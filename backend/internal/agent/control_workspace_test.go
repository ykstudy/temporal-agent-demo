package agent

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"agentstudio/internal/contracts"
)

func controlWorkspacePlanFixture(t *testing.T) (*Service, Run) {
	t.Helper()
	s := testService(t)
	r := controlFixture(t, s, "workspace_execution")
	result, err := s.ProcessCommand(context.Background(), Command{ID: "workspace_requirements", RunID: r.ID, Content: "创建 workspace-service，Postgres GORM JWT，添加日志"})
	if err != nil {
		t.Fatal(err)
	}
	r = controlAcceptCommand(t, s, result.Run)
	if len(r.Control.Audit.Plan.SelectedSkills) != 3 {
		t.Fatal("fixture does not select all three frozen Skills")
	}
	return s, r
}

func TestControlWorkspaceWritesOnlyEachApprovedScopeAndSurvivesRestart(t *testing.T) {
	s, r := controlWorkspacePlanFixture(t)
	version := r.SourceVersion
	var last contracts.ExecutionRequest
	var lastReceipt contracts.WorkspaceReceipt
	for i := 0; i < 4; i++ {
		next, err := s.PrepareControlAction(context.Background(), r.ID)
		if err != nil {
			t.Fatalf("prepare step %d: %v", i, err)
		}
		if next.Execution == nil {
			t.Fatalf("step%d has no controlled request %+v", i, next)
		}
		x := *next.Execution
		current, _ := s.Store.Get(r.ID)
		owned, err := controlWorkspaceOwnedFiles(current, x)
		if err != nil {
			t.Fatal(err)
		}
		before := map[string]contracts.ArtifactEntry{}
		if current.Control.Audit.Manifest != nil {
			for _, entry := range current.Control.Audit.Manifest.Files {
				before[entry.Path] = entry
			}
		}
		var receipt contracts.WorkspaceReceipt
		r, receipt, err = s.ExecuteControlWorkspace(context.Background(), r.ID, x)
		if err != nil {
			t.Fatalf("step%d: %v %+v", i, err, receipt)
		}
		if receipt.Status != "succeeded" || len(receipt.WrittenPaths) != len(owned) || !hashMatches("WorkspaceReceipt", receipt, receipt.ReceiptHash) || receipt.OutputManifestRef == nil || r.ActiveStep != "" || r.SourceVersion != version {
			t.Fatal("incomplete successful action", receipt)
		}
		for _, name := range receipt.WrittenPaths {
			if _, ok := owned[name]; !ok {
				t.Fatal("action wrote another Skill path", name)
			}
		}
		for _, entry := range r.Control.Audit.Manifest.Files {
			if _, written := owned[entry.Path]; !written {
				if !equalContractValue(entry, before[entry.Path]) {
					t.Fatal("unchanged artifact identity changed", entry.Path)
				}
			}
		}
		if r.Control.Audit.Manifest.ManifestVersion != int64(i+1) {
			t.Fatal("manifest version not monotonic")
		}
		if i == 0 {
			if _, err = os.Stat(filepath.Join(r.Workspace, "go.mod")); !os.IsNotExist(err) {
				t.Fatal("scaffold stole selected database go.mod", err)
			}
		}
		last, lastReceipt = x, receipt
	}
	actual, err := s.controlToolFiles(r, r.Workspace)
	if err != nil {
		t.Fatal(err)
	}
	if err = controlMatchApproved(r, actual); err != nil {
		t.Fatal(err)
	}
	if r.Control.Audit.Budget.Totals.Consumed.Action != 4 || r.Control.Audit.Budget.Totals.Reserved.Action != 0 {
		t.Fatal("workspace actions not settled once")
	}
	ledgerVersion := r.Control.Audit.Budget.LedgerVersion
	repeated, receipt, err := s.ExecuteControlWorkspace(context.Background(), r.ID, last)
	if err != nil || !equalContractValue(receipt, lastReceipt) || repeated.Control.Audit.Budget.LedgerVersion != ledgerVersion {
		t.Fatal("completed request did not reuse original receipt", err)
	}
	next, err := s.PrepareControlAction(context.Background(), r.ID)
	if err != nil {
		t.Fatal(err)
	}
	kind, _, _ := scheduleAction(next.Execution.Action)
	if kind != "verify" {
		t.Fatal("real evidence did not advance to VERIFY", kind)
	}
	cfg := s.Config
	s.Close()
	reopened, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	restored, err := reopened.Store.Get(r.ID)
	if err != nil {
		t.Fatal(err)
	}
	if restored.Control.Audit.Manifest.Hash != r.Control.Audit.Manifest.Hash || len(restored.Control.Audit.WorkspaceReceipts) != 4 || restored.Control.Audit.Budget.Totals.Consumed.Action != 4 {
		t.Fatal("restart lost complete workspace evidence")
	}
	if _, err = reopened.Store.CanonicalAudit(r.ID); err != nil {
		t.Fatal("reopened exact archive", err)
	}
}

func TestControlWorkspaceInputTamperBlocksOriginalOperationWithoutReplay(t *testing.T) {
	s, r := controlWorkspacePlanFixture(t)
	first, err := s.PrepareControlAction(context.Background(), r.ID)
	if err != nil {
		t.Fatal(err)
	}
	r, _, err = s.ExecuteControlWorkspace(context.Background(), r.ID, *first.Execution)
	if err != nil {
		t.Fatal(err)
	}
	next, err := s.PrepareControlAction(context.Background(), r.ID)
	if err != nil {
		t.Fatal(err)
	}
	path := r.Control.Audit.Manifest.Files[0].Path
	if err = os.WriteFile(filepath.Join(r.Workspace, filepath.FromSlash(path)), []byte("unapproved change"), 0600); err != nil {
		t.Fatal(err)
	}
	r, receipt, err := s.ExecuteControlWorkspace(context.Background(), r.ID, *next.Execution)
	if err == nil || r.State != "BLOCKED" || r.ActiveStep != "" || receipt.Status != "failed" || receipt.OutputManifestRef != nil {
		t.Fatal("unapproved input was used", err, receipt)
	}
	if r.Control.Audit.Operations[len(r.Control.Audit.Operations)-1].Status != "unknown" {
		t.Fatal("original operation not retained for examination")
	}
	consumed := r.Control.Audit.Budget.Totals.Consumed.Action
	if _, _, err = s.ExecuteControlWorkspace(context.Background(), r.ID, *next.Execution); err == nil {
		t.Fatal("blocked original operation replayed writes")
	}
	stored, _ := s.Store.Get(r.ID)
	if stored.Control.Audit.Budget.Totals.Consumed.Action != consumed {
		t.Fatal("rejected replay consumed new attempt")
	}
	content, _ := os.ReadFile(filepath.Join(r.Workspace, filepath.FromSlash(path)))
	if string(content) != "unapproved change" {
		t.Fatal("executor overwrote unexpected content")
	}
}

func TestControlWorkspaceFullAfterSnapshotOmitsDeletedPaths(t *testing.T) {
	s, r := controlWorkspacePlanFixture(t)
	next, err := s.PrepareControlAction(context.Background(), r.ID)
	if err != nil || next.Execution == nil {
		t.Fatal("prepare", err)
	}
	// Exercise only the scope/full-snapshot projection on a detached copy.
	// This is not a rehashed approval or an attempt to execute revised content.
	before := "retired source"
	hash := contracts.BytesHash([]byte(before))
	r.Control.Audit.Proposal.Changes = append(r.Control.Audit.Proposal.Changes, contracts.FileChange{Path: "retired.go", Operation: "delete", Before: &before, BeforeSHA256: &hash})
	files, err := controlWorkspaceOwnedFiles(r, *next.Execution)
	if err != nil {
		t.Fatal(err)
	}
	if _, exists := files["retired.go"]; exists {
		t.Fatal("deleted source scheduled as new generation write")
	}
	full := map[string]string{}
	manifest := contracts.ArtifactManifest{Files: []contracts.ArtifactEntry{}}
	for _, change := range r.Control.Audit.Proposal.Changes {
		if change.After == nil {
			continue
		}
		full[change.Path] = *change.After
		manifest.Files = append(manifest.Files, contracts.ArtifactEntry{Path: change.Path, SHA256: contracts.BytesHash([]byte(*change.After)), Size: int64(len(*change.After))})
	}
	r.Control.Audit.Manifest = &manifest
	if err = controlMatchApproved(r, full); err != nil {
		t.Fatal("complete After snapshot rejected deletion", err)
	}
	full["retired.go"] = before
	if err = controlMatchApproved(r, full); err == nil {
		t.Fatal("old deleted content allowed into new manifest")
	}
}

func TestControlWorkspaceRootRejectsSymlinkAndAtomicWritesStayAnchored(t *testing.T) {
	directory := t.TempDir()
	outside := t.TempDir()
	canary := filepath.Join(outside, "canary")
	if err := os.WriteFile(canary, []byte("private"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(directory, "linked")); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	if err = controlWorkspaceAtomicWrite(root, "linked/canary", "changed"); err == nil {
		t.Fatal("symlink escaped root")
	}
	if err = controlWorkspaceAtomicWrite(root, "../outside", "changed"); err == nil {
		t.Fatal("traversal escaped root")
	}
	if err = controlWorkspaceAtomicWrite(root, "safe/nested.txt", "approved"); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(canary)
	if string(data) != "private" {
		t.Fatal("outside file changed")
	}
	data, _ = os.ReadFile(filepath.Join(directory, "safe/nested.txt"))
	if string(data) != "approved" {
		t.Fatal("atomic approved write missing")
	}
}
