package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"agentstudio/internal/contracts"
	"agentstudio/internal/governance"
)

// A real full-contract publication, message/decision/approval chain and manifest
// wrap small approved source files. No external model/GitLab or fixture executor.
func controlToolsFixture(t *testing.T, files map[string]string) (*Service, Run, contracts.ExecutionRequest, []contracts.PostCommand, string) {
	t.Helper()
	s := testService(t)
	r := controlFixture(t, s, "controlled_tools")
	input := Command{ID: "tools_input", RunID: r.ID, Content: "创建 tools-service"}
	var message contracts.MessageEnvelope
	r, err := s.Store.Update(r.ID, func(run *Run) error {
		var e error
		message, e = appendCanonicalInput(run, input, time.Now().UTC())
		return e
	})
	if err != nil {
		t.Fatal(err)
	}
	p := defaultPlan()
	p.ProjectName = "tools-service"
	plan, err := projectPlanFor(r, p)
	if err != nil {
		t.Fatal(err)
	}
	ordered := []contracts.ContextMessage{{Role: "system", Content: r.FrozenDefinition.Definition.SystemPrompt.Content}, {Role: "user", Content: input.Content}}
	in, record, err := bindCanonicalDecision(r, input.ID, contracts.DecisionAdvice{Kind: "plan", Plan: &plan, Reason: "real parsed fixture input"}, contracts.ModelUsage{Status: "not_applicable"}, []contracts.MessageEnvelope{message}, ordered, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	validation, err := (governance.Validator{Definitions: s.Definitions}).Validate(in)
	if err != nil || validation.Verdict != "allowed" {
		t.Fatalf("plan: %v %+v", err, validation.Findings)
	}
	r, err = s.Store.Update(r.ID, func(run *Run) error {
		if e := s.recordCanonicalDecision(run, in, record, validation); e != nil {
			return e
		}
		run.Plan = p
		sourceChanged(run)
		if e := s.makeCanonicalProposal(run, plan, files, time.Now().UTC()); e != nil {
			return e
		}
		run.State = "PLAN"
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	proposal := r.Control.Audit.Proposal
	approve := Command{ID: "tools_approve", RunID: r.ID, Action: "accept", ProposalID: proposal.ProposalID, ProposalVersion: int(proposal.ProposalVersion), SourceVersion: r.SourceVersion, SnapshotHash: proposal.SnapshotHash}
	r, err = s.Store.Update(r.ID, func(run *Run) error { return s.acceptCanonical(run, approve, time.Now().UTC()) })
	if err != nil {
		t.Fatal(err)
	}
	workspace := filepath.Join(s.Config.DataDir, "workspaces", r.ID, p.ProjectName)
	if err = os.MkdirAll(filepath.Dir(workspace), 0700); err != nil {
		t.Fatal(err)
	}
	if err = controlCopyFiles(workspace, files); err != nil {
		t.Fatal(err)
	}
	manifest := contracts.ArtifactManifest{ContractVersion: "1.2.0", ManifestID: "manifest_tools", RunID: r.ID, SourceVersion: int64(r.SourceVersion), ProposalID: proposal.ProposalID, ProposalVersion: proposal.ProposalVersion, ProposalSnapshotHash: proposal.SnapshotHash, DefinitionSnapshotHash: r.FrozenDefinition.SnapshotHash, PlanHash: plan.PlanHash, Files: []contracts.ArtifactEntry{}, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano), ManifestVersion: 1}
	for path, contents := range files {
		hash := contracts.BytesHash([]byte(contents))
		manifest.Files = append(manifest.Files, contracts.ArtifactEntry{ArtifactID: "artifact_" + contracts.BytesHash([]byte(path))[:24], Path: path, Size: int64(len(contents)), SHA256: hash, MediaType: "text/plain"})
	}
	sort.Slice(manifest.Files, func(i, j int) bool { return manifest.Files[i].Path < manifest.Files[j].Path })
	manifest.Hash = chainRehash(t, "ArtifactManifest", manifest)
	r, err = s.Store.Update(r.ID, func(run *Run) error {
		run.Workspace = workspace
		run.State = "VERIFY"
		run.Control.Audit.Manifest = &manifest
		run.Control.Audit.ManifestHistory = append(run.Control.Audit.ManifestHistory, manifest)
		var e error
		run.Control.Audit.Budget, e = governance.StartExecution(run.FrozenDefinition.Definition.BudgetPolicy, run.Control.Audit.Budget, time.Now().UTC())
		return e
	})
	if err != nil {
		t.Fatal(err)
	}
	checks := []string{}
	for _, check := range r.FrozenDefinition.Definition.CompletionPolicy.RequiredChecks {
		if chainContains(check.DeliveryModes, "local") {
			checks = append(checks, check.CheckID)
		}
	}
	sort.Strings(checks)
	actionRaw, _ := json.Marshal(map[string]any{"action_type": "verify", "parameters": contracts.VerifyParameters{ManifestRef: controlManifestRef(manifest), RequiredCheckIDs: checks}})
	action := contracts.CandidateAction(actionRaw)
	in, record, err = bindCanonicalDecision(r, "tools_verify", contracts.DecisionAdvice{Kind: "action", Action: &action, Reason: "fixed VERIFY step"}, contracts.ModelUsage{Status: "not_applicable"}, []contracts.MessageEnvelope{message}, ordered, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	in.Facts.Workspace = &contracts.WorkspaceReference{WorkspaceID: "workspace_tools", RunID: r.ID, StorageKey: r.ID + "/" + p.ProjectName}
	in.Facts.Manifest = &manifest
	in.Facts.ExpectedAction = &action
	in.Facts.OperationSequence = 1
	deadline, _ := time.Parse(time.RFC3339Nano, *r.Control.Audit.Budget.ExecutionDeadlineAt)
	in.Facts.Budget = governance.BudgetFacts{Available: true, RemainingAttempts: 3, ExecutionDeadline: &deadline, ToolDeadline: &deadline}
	validation, err = (governance.Validator{Definitions: s.Definitions}).Validate(in)
	if err != nil || validation.Verdict != "allowed" {
		t.Fatalf("verify: %v %+v", err, validation.Findings)
	}
	r, err = s.Store.Update(r.ID, func(run *Run) error { return appendCanonicalDecision(run, in, record, validation) })
	if err != nil {
		t.Fatal(err)
	}
	request := *validation.ExecutionRequest
	r, err = s.Store.Update(r.ID, func(run *Run) error {
		target, e := governance.RequestTargetHash(request)
		if e != nil {
			return e
		}
		run.Control.Audit.Operations = append(run.Control.Audit.Operations, contracts.OperationRecord{OperationID: request.OperationID, RunID: r.ID, SourceVersion: request.SourceVersion, ActionType: "verify", OperationSequence: 1, RequestHash: request.RequestHash, TargetHash: target, Status: "sent", EvidenceRefs: []contracts.ObjectReference{}, CreatedAt: request.CreatedAt, UpdatedAt: time.Now().UTC().Format(time.RFC3339Nano)})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	attempt := "verify_attempt_1"
	r, _, err = s.reserveControlBudget(r.ID, governance.ReservationInput{SourceVersion: request.SourceVersion, CommandID: request.CommandID, AttemptID: attempt, Kind: "action", ScopeID: "verify", Request: &request, Now: time.Now().UTC()})
	if err != nil {
		t.Fatal(err)
	}
	commands, err := governance.MergePostCommands(*r.FrozenDefinition, plan)
	if err != nil {
		t.Fatal(err)
	}
	return s, r, request, commands, attempt
}

func toolSourceFiles() map[string]string {
	return map[string]string{"go.mod": "module example.com/controlled\n\ngo 1.25.0\n", "sample.go": "package controlled\nfunc Add(a,b int) int { return a+b }\n", "sample_test.go": "package controlled\nimport \"testing\"\nfunc TestAdd(t *testing.T){ if Add(1,2)!=3 { t.Fatal(\"wrong\") } }\n"}
}

func TestControlToolsActualSandboxExecutionAndReplay(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("supported process sandbox only")
	}
	files := toolSourceFiles()
	s, r, request, commands, attempt := controlToolsFixture(t, files)
	result, err := s.ExecuteControlTools(context.Background(), r, request, commands, attempt)
	if err != nil {
		t.Fatalf("execute: %v receipts=%+v", err, result.Receipts)
	}
	if result.ReapprovalRequired || len(result.Receipts) != 3 {
		t.Fatalf("unexpected result %+v", result)
	}
	for _, receipt := range result.Receipts {
		if err = contracts.Validate("ToolExecutionReceipt", receipt); err != nil {
			t.Fatal(err)
		}
		if receipt.Status != "succeeded" || receipt.ExitCode == nil || *receipt.ExitCode != 0 || receipt.OutputManifestRef == nil || !hashMatches("ToolExecutionReceipt", receipt, receipt.ReceiptHash) {
			t.Fatalf("not genuine success %+v", receipt)
		}
	}
	stored, err := s.Store.Get(r.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Control.Audit.Budget.Totals.Consumed.Tool != 3 || stored.Control.Audit.Budget.Totals.Reserved.Tool != 0 {
		t.Fatal("tools not settled")
	}
	version := stored.Control.Audit.Budget.LedgerVersion
	replay, err := s.ExecuteControlTools(context.Background(), stored, request, commands, attempt)
	if err != nil {
		t.Fatal(err)
	}
	if !equalContractValue(replay, result) {
		t.Fatal("replay rewrote evidence")
	}
	stored, _ = s.Store.Get(r.ID)
	if stored.Control.Audit.Budget.LedgerVersion != version {
		t.Fatal("replay consumed twice")
	}
	paths, err := s.controlToolPaths(stored, request, attempt)
	if err != nil {
		t.Fatal(err)
	}
	actual, err := s.controlToolFiles(stored, paths.approved)
	if err != nil || !equalContractValue(actual, files) {
		t.Fatal("approved workspace changed", err)
	}
	// Reuse this Run's compiled standard library cache, but test the sandbox
	// directly with a distinct disposable source package and no product receipt.
	outside := filepath.Join(s.Config.DataDir, "outside-canary")
	if err = os.WriteFile(outside, []byte("private-canary"), 0600); err != nil {
		t.Fatal(err)
	}
	probe := fmt.Sprintf(`package controlled
import("os";"os/exec";"net";"testing")
func TestSandbox(t *testing.T){
 if os.Getenv("CONTROL_PARENT_SECRET")!="" { t.Fatal("environment inherited") }
 if _,e:=os.ReadFile(%q);e==nil {t.Fatal("read outside sandbox")}
 if e:=os.WriteFile(%q,[]byte("changed"),0600);e==nil {t.Fatal("write outside sandbox")}
 if e:=exec.Command("/bin/sh","-c","exit 0").Run();e==nil {t.Fatal("shell allowed")}
 listener,e:=net.Listen("tcp","127.0.0.1:0");if e!=nil {t.Fatal(e)};listener.Close()
}
`, outside, outside)
	if err = os.WriteFile(filepath.Join(paths.candidate, "sandbox_test.go"), []byte(probe), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CONTROL_PARENT_SECRET", "do-not-inherit")
	_, binary, err := controlGoIdentity(stored)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd, err := controlSandboxCommand(ctx, paths, binary, []string{"test", "./..."}, "go.test")
	if err != nil {
		t.Fatal(err)
	}
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("sandbox probe: %v %s", err, output)
	}
	canary, _ := os.ReadFile(outside)
	if string(canary) != "private-canary" {
		t.Fatal("sandbox changed outside canary")
	}
}

func TestControlToolsTidyDifferenceRequiresReapproval(t *testing.T) {
	files := toolSourceFiles()
	files["go.mod"] += "\nrequire example.com/unused v1.0.0\n"
	s, r, request, commands, attempt := controlToolsFixture(t, files)
	result, err := s.ExecuteControlTools(context.Background(), r, request, commands, attempt)
	if err != nil {
		t.Fatalf("tidy: %v %+v", err, result.Receipts)
	}
	if !result.ReapprovalRequired || result.Candidate == nil || len(result.Receipts) != 1 || result.Receipts[0].ExitCode == nil || *result.Receipts[0].ExitCode != 0 || result.Receipts[0].OutputManifestRef != nil || !equalContractValue(result.Candidate.ChangedPaths, []string{"go.mod"}) {
		t.Fatalf("unapproved tidy result %+v", result)
	}
	approved, _ := os.ReadFile(filepath.Join(r.Workspace, "go.mod"))
	if string(approved) != files["go.mod"] {
		t.Fatal("tidy modified approved source")
	}
	if strings.Contains(result.Candidate.Files["go.mod"], "unused") {
		t.Fatal("candidate does not contain genuine tidy output")
	}
	stored, _ := s.Store.Get(r.ID)
	if stored.Control.Audit.Budget.Totals.Consumed.Tool != 1 {
		t.Fatal("tidy actual call not charged")
	}
	replay, err := s.ExecuteControlTools(context.Background(), stored, request, commands, attempt)
	if err != nil || !equalContractValue(result, replay) {
		t.Fatal("candidate replay changed result", err)
	}
}

func TestControlToolsRejectBeforeProcessAndRecoverUnknown(t *testing.T) {
	s, r, request, commands, attempt := controlToolsFixture(t, toolSourceFiles())
	bad := request
	bad.RunID = "another"
	if _, err := s.ExecuteControlTools(context.Background(), r, bad, commands, attempt); err == nil {
		t.Fatal("wrong run accepted")
	}
	changed := append([]contracts.PostCommand{}, commands...)
	changed[0].TimeoutSeconds--
	if _, err := s.ExecuteControlTools(context.Background(), r, request, changed, attempt); err == nil {
		t.Fatal("changed command accepted")
	}
	if _, err := s.ExecuteControlTools(context.Background(), r, request, commands, "unreserved_attempt"); err == nil {
		t.Fatal("unreserved parent accepted")
	}
	childHash, _ := contracts.HashValue([]string{request.RequestHash, attempt, commands[0].CommandID})
	child := "tool_" + childHash[:40]
	_, _, err := s.reserveControlBudget(r.ID, governance.ReservationInput{SourceVersion: request.SourceVersion, CommandID: request.CommandID, AttemptID: child, Kind: "tool", ScopeID: "tool:" + commands[0].CommandID, Request: &request, Tool: &commands[0], Now: time.Now().UTC()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.ExecuteControlTools(context.Background(), r, request, commands, attempt); err == nil {
		t.Fatal("missing receipt replayed tool")
	}
	stored, _ := s.Store.Get(r.ID)
	if stored.Control.Audit.Budget.Totals.Consumed.Tool != 1 || len(stored.Control.Audit.ToolReceipts) != 0 {
		t.Fatal("unknown was not consumed or fake receipt created")
	}
	if len(stored.Control.Audit.Budget.Settlements) != 1 || stored.Control.Audit.Budget.Settlements[0].Outcome != "unknown" {
		t.Fatal("lost result not unknown")
	}
}

func TestControlToolsBoundedStreamingRedaction(t *testing.T) {
	b := newControlToolOutput([]string{"boundary-secret"})
	for _, part := range []string{"start boundary-", "secret end ", strings.Repeat("界", 24000)} {
		_, _ = b.Write([]byte(part))
	}
	out, truncated := b.finish()
	if strings.Contains(out, "boundary-secret") || !strings.Contains(out, "[redacted]") || len(out) > 65536 || !utf8.ValidString(out) || !truncated {
		t.Fatalf("unsafe output %d %v", len(out), truncated)
	}
}

func TestControlToolsTimeoutConsumesAttemptAndPreservesSources(t *testing.T) {
	files := toolSourceFiles()
	files["sample_test.go"] = "package controlled\nimport (\"testing\";\"time\")\nfunc TestLong(t *testing.T){time.Sleep(time.Minute)}\n"
	s, r, request, commands, attempt := controlToolsFixture(t, files)
	paths, err := s.controlToolPaths(r, request, attempt)
	if err != nil {
		t.Fatal(err)
	}
	if err = paths.prepare(); err != nil {
		t.Fatal(err)
	}
	if err = controlCopyFiles(paths.candidate, files); err != nil {
		t.Fatal(err)
	}
	tool, binary, err := controlGoIdentity(r)
	if err != nil {
		t.Fatal(err)
	}
	command := commands[1]
	childHash, _ := contracts.HashValue([]string{request.RequestHash, attempt, command.CommandID})
	r, reservation, err := s.reserveControlBudget(r.ID, governance.ReservationInput{SourceVersion: request.SourceVersion, CommandID: request.CommandID, AttemptID: "tool_" + childHash[:40], Kind: "tool", ScopeID: "tool:" + command.CommandID, Request: &request, Tool: &command, Now: time.Now().UTC()})
	if err != nil {
		t.Fatal(err)
	}
	// Start this deadline at the process boundary, after expensive schema and
	// durable-ledger work. It must bound the real child and all its descendants.
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	started := time.Now()
	receipt, err := s.controlRunTool(ctx, r, request, command, tool, binary, paths, reservation)
	if err == nil || time.Since(started) > 5*time.Second || receipt.Status != "interrupted" || receipt.ExitCode == nil {
		t.Fatalf("process deadline not enforced: %v elapsed=%s receipt=%+v", err, time.Since(started), receipt)
	}
	receipt.ReceiptHash = chainRehash(t, "ToolExecutionReceipt", receipt)
	if err = contracts.Validate("ToolExecutionReceipt", receipt); err != nil {
		t.Fatal(err)
	}
	_, err = s.Store.Update(r.ID, func(run *Run) error {
		run.Control.Audit.ToolReceipts = append(run.Control.Audit.ToolReceipts, receipt)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.controlSettleTool(r.ID, reservation, receipt); err != nil {
		t.Fatal(err)
	}
	stored, _ := s.Store.Get(r.ID)
	if stored.Control.Audit.Budget.Totals.Consumed.Tool != 1 || stored.Control.Audit.Budget.Totals.Reserved.Tool != 0 {
		t.Fatal("interruption was not charged")
	}
	actual, e := s.controlToolFiles(stored, r.Workspace)
	if e != nil || !equalContractValue(actual, files) {
		t.Fatal("timeout modified approved files", e)
	}
}

func TestControlToolsRejectPathToolAndCurrentAuthority(t *testing.T) {
	s, r, request, commands, attempt := controlToolsFixture(t, toolSourceFiles())
	wrong := clone(r)
	wrong.FrozenDefinition.ToolBindings[0].Version = "go0.0.1"
	if _, _, err := controlGoIdentity(wrong); err == nil {
		t.Fatal("wrong actual Go version accepted")
	}
	wrong = clone(r)
	platform := "wrong_arch"
	wrong.FrozenDefinition.ToolBindings[0].Platform = &platform
	if _, _, err := controlGoIdentity(wrong); err == nil {
		t.Fatal("wrong platform accepted")
	}
	bad := request
	bad.Workspace.StorageKey = "other-run/tools-service"
	if _, err := s.controlToolPaths(r, bad, attempt); err == nil {
		t.Fatal("cross-run workspace accepted")
	}
	link := filepath.Join(r.Workspace, "linked")
	if err := os.Symlink(s.Config.DataDir, link); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ExecuteControlTools(context.Background(), r, request, commands, attempt); err == nil {
		t.Fatal("symlink accepted")
	}
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	_, err := s.Store.Update(r.ID, func(run *Run) error { run.PauseRequested = true; return nil })
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.ExecuteControlTools(context.Background(), r, request, commands, attempt); err == nil {
		t.Fatal("pause did not stop new tool")
	}
	stored, _ := s.Store.Get(r.ID)
	if stored.Control.Audit.Budget.Totals.Consumed.Tool != 0 || len(stored.Control.Audit.ToolReceipts) != 0 {
		t.Fatal("rejected attempt invoked process")
	}
}

// An explicitly enabled live public dependency probe. Ordinary unit/race runs
// remain offline; this proves the restricted tidy proxy and per-Run cache path.
func TestControlToolsPublicProxy(t *testing.T) {
	if os.Getenv("CONTROL_TOOLS_NETWORK_TEST") != "1" {
		t.Skip("explicit public proxy probe")
	}
	files := toolSourceFiles()
	files["go.mod"] += "\nrequire github.com/google/uuid v1.6.0\n"
	files["go.sum"] = "github.com/google/uuid v1.6.0 h1:NIvaJDMOsjHA8n1jAhLSgzrAzy1Hgr+hNrb57e+94F0=\ngithub.com/google/uuid v1.6.0/go.mod h1:TIyPZe4MgqvfeYDBFedMoGGpEw/LqOeaOT+nhxU+yHo=\n"
	files["sample.go"] = "package controlled\nimport \"github.com/google/uuid\"\nfunc Add(a,b int)int{return a+b}\nfunc Identifier()string{return uuid.NewString()}\n"
	s, r, request, commands, attempt := controlToolsFixture(t, files)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	result, err := s.ExecuteControlTools(ctx, r, request, commands, attempt)
	if err != nil || result.ReapprovalRequired || len(result.Receipts) != 3 {
		t.Fatalf("public dependency sandbox probe: %v %+v", err, result)
	}
}
