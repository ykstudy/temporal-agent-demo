package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"agentstudio/internal/contracts"
)

func TestControlReconcileCommandReadsOriginalWriteWithoutRevivingTerminalRun(t *testing.T) {
	m := &controlGitLabMock{lostResponse: true}
	remote := httptest.NewServer(http.HandlerFunc(m.serve))
	defer remote.Close()
	s := testService(t)
	s.Config.GitLabURL, s.Config.GitLabNamespace, s.Config.GitLabToken = remote.URL, "17", "controlled-gitlab-test-secret"
	r := newControlCompletionRun(t, s, "rules")
	var err error
	r, err = s.Store.Update(r.ID, func(run *Run) error {
		run.DeliveryMode, run.Plan.RepoProvider = "gitlab", "gitlab"
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	planned, err := s.ProcessCommand(context.Background(), Command{ID: "remote_requirements", RunID: r.ID, Content: "创建 create-service"})
	if err != nil {
		t.Fatal(err)
	}
	r = controlAcceptCommand(t, s, planned.Run)
	for _, step := range []string{"scaffold", "skills", "verify"} {
		if _, err = s.ExecuteStep(context.Background(), StepInput{RunID: r.ID, Step: step}); err != nil {
			t.Fatalf("actual prerequisite %s: %v", step, err)
		}
	}
	next, err := s.PrepareControlAction(context.Background(), r.ID)
	if err != nil || next.Execution == nil {
		t.Fatalf("remote request: %v", err)
	}
	r, err = s.ExecuteControlGitLab(context.Background(), r.ID, *next.Execution)
	if err == nil || r.State != "BLOCKED" || m.posts != 1 {
		t.Fatalf("lost actual POST not unknown: %v", err)
	}
	if len(r.Control.Audit.RemoteReceipts) != 1 || r.Control.Audit.RemoteReceipts[0].Status != "unknown" || !r.Control.Audit.RemoteReceipts[0].WriteSent || len(r.Control.Audit.RemoteReceipts[0].Observations) != 2 || r.Control.Audit.RemoteReceipts[0].Project != nil {
		t.Fatal("unknown write lacks its complete original receipt")
	}
	unknownBytes, err := s.Store.CanonicalObject(r.ID, controlRemoteReceiptRef(r.Control.Audit.RemoteReceipts[0]))
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.ProcessCommand(context.Background(), Command{ID: "cancel_unknown", RunID: r.ID, Action: "cancel"})
	if err != nil {
		t.Fatal(err)
	}
	handler := http.NewServeMux()
	s.Register(handler)
	beforeGets := m.gets
	for _, suffix := range []string{"", "/decision-audit", "/budget"} {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/runs/"+r.ID+suffix, nil))
		if recorder.Code != 200 || m.gets != beforeGets || m.posts != 1 {
			t.Fatalf("GET caused remote reconciliation or invalid response: %d", recorder.Code)
		}
	}
	bad := httptest.NewRecorder()
	handler.ServeHTTP(bad, httptest.NewRequest(http.MethodPost, "/api/runs/"+r.ID+"/actions", strings.NewReader(`{"id":"bad_reconcile","action":"reconcile","proposal_id":"injected"}`)))
	if bad.Code == 200 || m.gets != beforeGets || m.posts != 1 {
		t.Fatal("reconcile accepted caller scope")
	}
	for i := 0; i < 2; i++ {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/runs/"+r.ID+"/actions", strings.NewReader(`{"id":"original_reconcile","action":"reconcile"}`)))
		if recorder.Code != 200 || m.gets != beforeGets+1 || m.posts != 1 {
			t.Fatalf("reconcile or replay changed write/read count: status=%d body=%s", recorder.Code, recorder.Body.String())
		}
	}
	r, err = s.Store.Get(r.ID)
	if err != nil || r.State != "CANCELLED" || r.ActiveStep != "" {
		t.Fatalf("terminal fact changed: %v", err)
	}
	assertControlCreateResult(t, r, *next.Execution, "unknown")
	if len(r.Control.Audit.RemoteReceipts) != 2 || r.Control.Audit.RemoteReceipts[1].Status != "succeeded" || len(r.Control.Audit.RemoteReceipts[1].Observations) != 3 {
		t.Fatal("reconciliation did not append one complete successful receipt")
	}
	stillUnknown, err := s.Store.CanonicalObject(r.ID, controlRemoteReceiptRef(r.Control.Audit.RemoteReceipts[0]))
	if err != nil || string(stillUnknown) != string(unknownBytes) {
		t.Fatal("reconciliation overwrote the old unknown receipt", err)
	}
	assertControlCreateReceiptNegatives(t, r, *next.Execution)
	if directory := os.Getenv("CONTROL_REMOTE_EVIDENCE_DIR"); directory != "" {
		audit, e := s.Store.CanonicalAudit(r.ID)
		if e != nil {
			t.Fatal(e)
		}
		body, e := json.MarshalIndent(map[string]any{"kind": "actual_controlled_workspace_tools_and_mock_gitlab_reconciliation", "public_creation": false, "live_provider": false, "state": r.State, "audit": audit}, "", "  ")
		if e != nil {
			t.Fatal(e)
		}
		if e = os.MkdirAll(directory, 0700); e != nil {
			t.Fatal(e)
		}
		if e = os.WriteFile(filepath.Join(directory, "cancelled-create-reconciled.json"), append(body, '\n'), 0600); e != nil {
			t.Fatal(e)
		}
	}
	if m.requestProblem != "" {
		t.Fatal(m.requestProblem)
	}
}

func assertControlCreateReceiptNegatives(t *testing.T, r Run, request contracts.ExecutionRequest) {
	t.Helper()
	receipt := r.Control.Audit.RemoteReceipts[len(r.Control.Audit.RemoteReceipts)-1]
	if err := ValidateControlRemoteReceipt(r, receipt); err != nil {
		t.Fatal("complete actual receipt rejected", err)
	}
	for _, mutation := range []string{"source", "request", "approval", "manifest", "project", "marker", "unobserved_project", "unknown_unobserved_project", "missing_read", "read_path", "read_facts_hash", "read_reservation", "duplicate_read", "write_not_sent", "attempt", "time"} {
		t.Run(mutation, func(t *testing.T) {
			changed := clone(r)
			q := changed.Control.Audit.RemoteReceipts[len(changed.Control.Audit.RemoteReceipts)-1]
			switch mutation {
			case "source":
				q.SourceVersion++
			case "request":
				q.RequestHash = strings.Repeat("d", 64)
			case "approval":
				q.ApprovalHash = strings.Repeat("d", 64)
			case "manifest":
				q.ManifestRef.Hash = strings.Repeat("d", 64)
			case "project":
				q.Project.ProjectID++
			case "marker":
				q.Project.OwnershipMarker = "another owner"
			case "unobserved_project":
				q.Project.EmptyRepository = false
			case "unknown_unobserved_project":
				q.Status = "unknown"
				q.Project.ProjectID++
			case "missing_read":
				q.Observations = q.Observations[:2]
			case "read_path":
				q.Observations[2].Path = "/projects/other"
			case "read_facts_hash":
				hash := strings.Repeat("d", 64)
				q.Observations[2].FactsHash = &hash
			case "read_reservation":
				q.Observations[2].ReservationRef.Hash = strings.Repeat("d", 64)
			case "duplicate_read":
				q.Observations = append(q.Observations, q.Observations[2])
			case "write_not_sent":
				q.WriteSent = false
			case "attempt":
				q.AttemptID = "different_attempt"
			case "time":
				q.CompletedAt = request.CreatedAt
			}
			q.ReceiptHash = chainRehash(t, "RemoteExecutionReceipt", q)
			if err := contracts.Validate("RemoteExecutionReceipt", q); err != nil {
				t.Fatal("negative fixture must remain full-schema-valid", err)
			}
			if err := ValidateControlRemoteReceipt(changed, q); err == nil {
				t.Fatal("rehashed forged remote receipt accepted")
			}
		})
	}
	missing := clone(r)
	missing.Control.Audit.RemoteReceipts = nil
	if _, err := NextControlAction(missing, missing.Control.Schedule); err == nil {
		t.Fatal("private success alone advanced 1.3 scheduler")
	}
	unlinked := clone(r)
	for i := range unlinked.Control.Audit.Operations {
		if unlinked.Control.Audit.Operations[i].OperationID == request.OperationID {
			unlinked.Control.Audit.Operations[i].EvidenceRefs = nil
		}
	}
	if err := verifyControlChain(unlinked); err == nil {
		t.Fatal("unlinked public receipt accepted")
	}
}

func TestControlReconcileCommandRejectsOldPublication(t *testing.T) {
	s := testService(t)
	r := controlFixture(t, s, "old_reconcile")
	result, err := s.ProcessCommand(context.Background(), Command{ID: "unsupported_reconcile", RunID: r.ID, Action: "reconcile"})
	if err == nil || result.Execute || result.Run.SourceVersion != r.SourceVersion {
		t.Fatal("old publication accepted new intent")
	}
}
