package agent

import (
	"context"
	"encoding/json"
	"errors"
	"go.temporal.io/sdk/temporal"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func assertCode(t *testing.T, err error, code string) {
	t.Helper()
	var api *APIError
	if !errors.As(err, &api) || api.Code != code {
		t.Fatalf("expected %s, got %v", code, err)
	}
}
func TestProgressAndPauseDoNotInvalidateProposal(t *testing.T) {
	s := testService(t)
	r := planRun(t, s, "axes", "创建 axes-service")
	source, seq, hash := r.SourceVersion, r.StateSequence, r.Proposal.SnapshotHash
	result, e := s.ProcessCommand(context.Background(), Command{ID: "progress_axes", RunID: r.ID, Content: "进度"})
	if e != nil {
		t.Fatal(e)
	}
	if result.Run.SourceVersion != source || result.Run.StateSequence <= seq || result.Run.Proposal.SnapshotHash != hash {
		t.Fatal("ordinary progress changed authorization")
	}
	if _, e = s.ProcessCommand(context.Background(), Command{ID: "pause_axes", RunID: r.ID, Action: "pause"}); e != nil {
		t.Fatal(e)
	}
	if _, e = s.ProcessCommand(context.Background(), Command{ID: "resume_axes", RunID: r.ID, Action: "resume"}); e != nil {
		t.Fatal(e)
	}
	if _, e = s.ProcessCommand(context.Background(), approvalCommand(r, "approve_axes")); e != nil {
		t.Fatal(e)
	}
}
func TestExpiredProposalAndFirstApplicationRecheck(t *testing.T) {
	for _, accepted := range []bool{false, true} {
		t.Run(map[bool]string{false: "before_accept", true: "before_apply"}[accepted], func(t *testing.T) {
			s := testService(t)
			r := planRun(t, s, "expiry", "创建 expiry-service")
			if accepted {
				if _, e := s.ProcessCommand(context.Background(), approvalCommand(r, "accept_expiry")); e != nil {
					t.Fatal(e)
				}
			}
			// The test changes the service's issued fixture, not a client request.
			r, e := s.Store.Update(r.ID, func(run *Run) error {
				run.Proposal.ExpiresAt = time.Now().Add(-time.Minute).UTC().Format(time.RFC3339Nano)
				run.Proposal.SnapshotHash = proposalDigest(*run, *run.Proposal)
				if run.Approval != nil {
					run.Approval.ExpiresAt = run.Proposal.ExpiresAt
					run.Approval.SnapshotHash = run.Proposal.SnapshotHash
				}
				return nil
			})
			if e != nil {
				t.Fatal(e)
			}
			if !accepted {
				_, e = s.ProcessCommand(context.Background(), approvalCommand(r, "expired_accept"))
				assertCode(t, e, "proposal_expired")
			} else {
				result, e := s.ExecuteStep(context.Background(), StepInput{RunID: r.ID, Step: "scaffold"})
				if e != nil || result.State != "PLAN" {
					t.Fatalf("expired first application: %+v %v", result, e)
				}
			}
			if _, e = os.Stat(filepath.Join(s.Config.DataDir, "workspaces", r.ID)); !os.IsNotExist(e) {
				t.Fatal("expired authorization wrote source")
			}
		})
	}
}
func TestStartedApprovalCompletesWithinOriginalScopeAfterExpiry(t *testing.T) {
	s := testService(t)
	r := planRun(t, s, "started", "创建 started-service")
	if _, e := s.ProcessCommand(context.Background(), approvalCommand(r, "accept_started")); e != nil {
		t.Fatal(e)
	}
	if _, e := s.ExecuteStep(context.Background(), StepInput{RunID: r.ID, Step: "scaffold"}); e != nil {
		t.Fatal(e)
	}
	r, e := s.Store.Update(r.ID, func(run *Run) error {
		run.Proposal.ExpiresAt = time.Now().Add(-time.Minute).UTC().Format(time.RFC3339Nano)
		run.Proposal.SnapshotHash = proposalDigest(*run, *run.Proposal)
		run.Approval.ExpiresAt = run.Proposal.ExpiresAt
		run.Approval.SnapshotHash = run.Proposal.SnapshotHash
		return nil
	})
	if e != nil {
		t.Fatal(e)
	}
	if e = s.verifyApproval(r, false); e != nil {
		t.Fatal("valid started scope cancelled by elapsed time", e)
	}
}
func TestSnapshotTamperingAndGoModTamperingStopExecution(t *testing.T) {
	s := testService(t)
	r := planRun(t, s, "tamper", "创建 tamper-service")
	if _, e := s.ProcessCommand(context.Background(), approvalCommand(r, "accept_tamper")); e != nil {
		t.Fatal(e)
	}
	if _, e := s.ExecuteStep(context.Background(), StepInput{RunID: r.ID, Step: "scaffold"}); e != nil {
		t.Fatal(e)
	}
	if _, e := s.ExecuteStep(context.Background(), StepInput{RunID: r.ID, Step: "skills"}); e != nil {
		t.Fatal(e)
	}
	r, _ = s.Store.Get(r.ID)
	if e := os.WriteFile(filepath.Join(r.Workspace, "go.mod"), []byte("module attacker\n\ngo 1.25\n"), 0600); e != nil {
		t.Fatal(e)
	}
	result, e := s.ExecuteStep(context.Background(), StepInput{RunID: r.ID, Step: "verify"})
	if e != nil {
		t.Fatal(e)
	}
	if result.State != "FAILED" {
		t.Fatal("tampered module not stopped", result)
	}
	r2 := planRun(t, s, "snapshot_bad", "创建 snapshot-bad")
	if _, e = s.ProcessCommand(context.Background(), approvalCommand(r2, "accept_snapshot_bad")); e != nil {
		t.Fatal(e)
	}
	_, _ = s.Store.Update(r2.ID, func(run *Run) error { run.Proposal.Changes[0].After += "\n// unapproved"; return nil })
	result, e = s.ExecuteStep(context.Background(), StepInput{RunID: r2.ID, Step: "scaffold"})
	if e != nil || result.State != "FAILED" {
		t.Fatal("modified snapshot applied", result, e)
	}
}
func TestCreateRetryAfterModelFailureAndBudgetIncludesFailure(t *testing.T) {
	calls := 0
	var s *Service
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.WriteHeader(503)
			return
		}
		run, err := s.Store.Get("run_retry_model")
		if err != nil {
			t.Error(err)
			w.WriteHeader(500)
			return
		}
		plan := run.Plan
		plan.ProjectName = "retry-model"
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": string(fixtureAdvice(t, run, plan))}}}})
	}))
	defer api.Close()
	s = testService(t)
	s.Config.DecisionMode = "model"
	s.Config.AIEndpoint = api.URL
	s.Config.AIKey = "provider-secret"
	s.Config.AIModel = "fixture-model"
	opts := CreateOptions{ID: "run_retry_model", Content: "创建 retry-model"}
	_, e := s.CreateWithOptions(context.Background(), opts)
	assertCode(t, e, "model_unavailable")
	r, e := s.CreateWithOptions(context.Background(), opts)
	if e != nil || r.State != "PLAN" || r.Proposal == nil {
		t.Fatal("create retry lost initial message", r.State, e)
	}
	if r.Usage.ModelAttempts != 2 || r.Usage.ModelFailures != 1 {
		t.Fatal("model failure uncharged", r.Usage)
	}
	_, e = s.CreateWithOptions(context.Background(), CreateOptions{ID: opts.ID, Content: "other"})
	assertCode(t, e, "idempotency_conflict")
	_, _ = s.Store.Update(r.ID, func(run *Run) error { run.Usage.ModelAttempts = run.Definition.Budget.MaxModelAttempts; return nil })
	_, e = s.ProcessCommand(context.Background(), Command{ID: "over_model_budget", RunID: r.ID, Content: "JSON 日志"})
	assertCode(t, e, "budget_exceeded")
	if calls != 2 {
		t.Fatal("provider called beyond budget")
	}
}
func TestModeAndSecretBoundaries(t *testing.T) {
	for _, url := range []string{"https://token@example.com", "https://example.com?token=x", "https://example.com#secret"} {
		_, e := New(Config{DataDir: t.TempDir(), Mode: "local", GitLabURL: url})
		assertCode(t, e, "invalid_endpoint")
	}
	s := testService(t)
	s.Config.AIKey = "k9"
	_, e := s.CreateWithOptions(context.Background(), CreateOptions{ID: "secret_title", Title: "k9"})
	assertCode(t, e, "secret_input")
	_, e = s.CreateWithOptions(context.Background(), CreateOptions{ID: "wrong_mode", ExecutionMode: "temporal"})
	assertCode(t, e, "invalid_mode")
	_, e = s.CreateWithOptions(context.Background(), CreateOptions{ID: "no_model", DecisionMode: "model", Content: "创建 no-model"})
	assertCode(t, e, "model_unavailable")
	if _, err := s.Store.Get("no_model"); err == nil {
		t.Fatal("unbound model identity was persisted")
	}
}
func TestStoreSingleWriterAndMalformedTranscript(t *testing.T) {
	s := testService(t)
	r := planRun(t, s, "storelock", "创建 storelock-service")
	_, e := NewStore(s.Config.DataDir)
	assertCode(t, e, "storage_locked")
	s.Store.Close()
	f, e := os.OpenFile(filepath.Join(s.Config.DataDir, "runs", r.ID+".jsonl"), os.O_APPEND|os.O_WRONLY, 0600)
	if e != nil {
		t.Fatal(e)
	}
	_, _ = f.WriteString("{truncated")
	_ = f.Close()
	_, e = NewStore(s.Config.DataDir)
	if e == nil || !strings.Contains(e.Error(), "JSONL") {
		t.Fatal("corrupt transcript silently accepted", e)
	}
}
func TestUnknownPipelineNeverPostsAgainWhileInvisible(t *testing.T) {
	s := testService(t)
	var posts int
	visible := false
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			posts++
			w.WriteHeader(503)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/pipelines") {
			if !visible {
				_ = json.NewEncoder(w).Encode([]any{})
			} else {
				_ = json.NewEncoder(w).Encode([]any{map[string]any{"id": 77, "sha": "original-sha", "ref": "main", "status": "success"}})
			}
			return
		}
		if strings.HasSuffix(r.URL.Path, "/jobs") {
			_ = json.NewEncoder(w).Encode([]any{})
			return
		}
		w.WriteHeader(404)
	}))
	defer api.Close()
	s.Config.GitLabURL = api.URL
	s.Config.GitLabToken = "fixture-token"
	s.Config.GitLabNamespace = "1"
	r := planRun(t, s, "unknown_pipeline", "创建 unknown-pipeline，GitLab")
	if _, e := s.ProcessCommand(context.Background(), approvalCommand(r, "approve_unknown")); e != nil {
		t.Fatal(e)
	}
	for _, step := range []string{"scaffold", "skills", "verify"} {
		if _, e := s.ExecuteStep(context.Background(), StepInput{RunID: r.ID, Step: step}); e != nil {
			t.Fatal(e)
		}
	}
	_, _ = s.Store.Update(r.ID, func(run *Run) error { run.RepoProjectID = 5; run.CommitSHA = "original-sha"; return nil })
	for i := 0; i < 3; i++ {
		e := s.VerifyGitLabPipeline(context.Background(), r.ID)
		var unknown *UnknownResultError
		if !errors.As(e, &unknown) {
			t.Fatal("lost result escaped unknown state", e)
		}
	}
	if posts != 1 {
		t.Fatal("unknown original request repeated", posts)
	}
	visible = true
	if e := s.VerifyGitLabPipeline(context.Background(), r.ID); e == nil {
		t.Fatal("pipeline success with missing required jobs accepted")
	}
	if posts != 1 {
		t.Fatal("reconciliation triggered another pipeline")
	}
}

func TestHistoricalExecutionModeRemainsReadOnly(t *testing.T) {
	s := testService(t)
	r := planRun(t, s, "historical_mode", "创建 history-service")
	_, _ = s.Store.Update(r.ID, func(run *Run) error { run.ExecutionMode = "temporal"; return nil })
	cmd := Command{ID: "wrong_executor", RunID: r.ID, Content: "进度"}
	_, e := s.Submit(context.Background(), cmd)
	assertCode(t, e, "run_mode_mismatch")
	_, e = s.ProcessCommand(context.Background(), cmd)
	assertCode(t, e, "run_mode_mismatch")
	if _, e = s.ExecuteStep(context.Background(), StepInput{RunID: r.ID, Step: "scaffold"}); e == nil {
		t.Fatal("old run executed in another mode")
	}
}
func TestLocalRestartClearsInterruptedLatchAndPreservesApproval(t *testing.T) {
	s := testService(t)
	r := planRun(t, s, "restart", "创建 restart-service")
	if _, e := s.ProcessCommand(context.Background(), approvalCommand(r, "accept_restart")); e != nil {
		t.Fatal(e)
	}
	_, _ = s.Store.Update(r.ID, func(run *Run) error { run.ActiveStep = "scaffold"; run.State = "IMPLEMENT"; return nil })
	cfg := s.Config
	s.Close()
	restarted, e := New(cfg)
	if e != nil {
		t.Fatal(e)
	}
	defer restarted.Close()
	saved, _ := restarted.Store.Get(r.ID)
	if saved.State != "BLOCKED" || saved.ActiveStep != "" || saved.Approval == nil {
		t.Fatal("local interrupted run irrecoverable", saved.State, saved.ActiveStep)
	}
	result, e := restarted.ProcessCommand(context.Background(), Command{ID: "resume_restart", RunID: r.ID, Action: "resume"})
	if e != nil || !result.Execute {
		t.Fatal("restart resume failed", e)
	}
}

func TestFailedCommandRetainsOriginalBusinessIdentity(t *testing.T) {
	s := testService(t)
	// The provider identity can be frozen while its credential is unavailable.
	// No fabricated endpoint is inserted by the product to create this Run.
	s.Config.AIEndpoint, s.Config.AIModel = "https://model.invalid/v1", "fixture-model"
	r, e := s.CreateWithOptions(context.Background(), CreateOptions{ID: "failed_intent", DecisionMode: "model"})
	if e != nil {
		t.Fatal(e)
	}
	cmd := Command{ID: "failed_business_command", RunID: r.ID, Content: "创建 failed-service"}
	_, e = s.ProcessCommand(context.Background(), cmd)
	assertCode(t, e, "model_unavailable")
	cmd.Content = "changed payload"
	_, e = s.ProcessCommand(context.Background(), cmd)
	assertCode(t, e, "idempotency_conflict")
}

func TestConcurrentCreateIdentityIsAtomic(t *testing.T) {
	s := testService(t)
	results := make(chan error, 2)
	for _, title := range []string{"first title", "second title"} {
		go func(title string) {
			_, e := s.CreateWithOptions(context.Background(), CreateOptions{ID: "atomic_create", Title: title})
			results <- e
		}(title)
	}
	first, second := <-results, <-results
	if first == nil && second == nil {
		t.Fatal("concurrent different payloads both accepted")
	}
	if first != nil {
		assertCode(t, first, "idempotency_conflict")
	}
	if second != nil {
		assertCode(t, second, "idempotency_conflict")
	}
	if len(s.Store.List()) != 1 {
		t.Fatal("duplicate business run")
	}
}

func TestErrorCodesHaveSameLocalAndTemporalHTTPStatus(t *testing.T) {
	for _, code := range []string{"model_unavailable", "model_schema_invalid", "stale_proposal", "idempotency_conflict", "budget_exceeded"} {
		for _, err := range []error{fail(code, "fixture"), temporal.NewNonRetryableApplicationError("fixture", code, nil)} {
			rr := httptest.NewRecorder()
			writeError(rr, err)
			if rr.Code != errorStatus(code) {
				t.Fatal("transport changed error status", code, rr.Code)
			}
		}
	}
}
