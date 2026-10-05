package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"agentstudio/internal/contracts"
)

// The earlier workspace history here is synthetic scheduling evidence. These
// review tests exercise real admission/settlement entry points, not a completed
// project or live GitLab acceptance.
func reviewPreparedVerification(t *testing.T) (*Service, Run, contracts.ExecutionRequest) {
	t.Helper()
	f := newScheduleFixture(t, "local")
	// The pure scheduler fixture deliberately uses an arbitrary valid workspace.
	// This integration imports its requests into the real dispatcher, so freeze
	// the dispatcher's actual identity before issuing any synthetic request.
	key := controlWorkspaceKey(f.r)
	hash, err := contracts.HashValue([]any{f.r.ID, key})
	if err != nil {
		t.Fatal(err)
	}
	f.state.Workspace = contracts.WorkspaceReference{WorkspaceID: "workspace_" + hash[:32], RunID: f.r.ID, StorageKey: key}
	for {
		next := f.next()
		kind, _, err := scheduleAction(*next.Action)
		if err != nil {
			t.Fatal(err)
		}
		if kind == "verify" {
			break
		}
		f.finish(f.issue(next))
	}
	f.r.Control.Schedule = f.state
	f.r.Workspace = filepath.Join(f.s.Config.DataDir, "workspaces", f.state.Workspace.StorageKey)
	// Store owns state_sequence and will not accept a fixture whose historical
	// requests appear in its future. Advance the fixture's earlier empty Store
	// snapshots before importing the explicitly synthetic scheduling history.
	persisted, err := f.s.Store.Get(f.r.ID)
	if err != nil {
		t.Fatal(err)
	}
	for persisted.StateSequence < f.r.StateSequence {
		persisted, err = f.s.Store.Update(f.r.ID, func(run *Run) error { return nil })
		if err != nil {
			t.Fatal(err)
		}
	}
	_, err = f.s.Store.Update(f.r.ID, func(run *Run) error { *run = clone(f.r); return nil })
	if err != nil {
		t.Fatal(err)
	}
	next, err := f.s.PrepareControlAction(context.Background(), f.r.ID)
	if err != nil || next.Execution == nil {
		t.Fatalf("prepare verification: %v %+v", err, next)
	}
	r, err := f.s.Store.Get(f.r.ID)
	if err != nil {
		t.Fatal(err)
	}
	return f.s, r, *next.Execution
}

func TestControlIntegrationReviewFatalBeforeToolsClosesOriginalAttempt(t *testing.T) {
	s, before, request := reviewPreparedVerification(t)
	_, err := s.ExecuteControlVerification(context.Background(), request)
	if err == nil {
		t.Fatal("missing actual approved workspace became success")
	}
	after, readErr := s.Store.Get(before.ID)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if after.Control.Audit.Budget.Totals.Reserved.Action != 0 || after.Control.Audit.Budget.Totals.Consumed.Action != before.Control.Audit.Budget.Totals.Consumed.Action+1 || after.ActiveStep != "" {
		t.Fatalf("fatal before tool launch left original action unclosed: active=%q totals=%+v error=%v", after.ActiveStep, after.Control.Audit.Budget.Totals, err)
	}
	op := after.Control.Audit.Operations[len(after.Control.Audit.Operations)-1]
	if op.OperationID != request.OperationID || op.Status != "rejected" || after.State != "BLOCKED" || len(after.Control.Audit.ToolReceipts) != 0 || after.Control.Audit.Budget.Totals.Consumed.Tool != 0 {
		t.Fatalf("fatal invented tool evidence or lost original scope: %+v", op)
	}
	if _, statErr := os.Stat(after.Workspace); !os.IsNotExist(statErr) {
		t.Fatalf("fatal preflight created approved files: %v", statErr)
	}
	if _, err = s.ExecuteControlVerification(context.Background(), request); err == nil {
		t.Fatal("closed fatal action blindly retried")
	}
	again, _ := s.Store.Get(before.ID)
	if again.Control.Audit.Budget.Totals.Consumed.Action != after.Control.Audit.Budget.Totals.Consumed.Action {
		t.Fatal("blocked replay charged another action")
	}
}

func TestControlIntegrationReviewAllSelectedSkillsFitFirstActionContext(t *testing.T) {
	s := testService(t)
	r := controlFixture(t, s, "integration_review_all_skills")
	result, err := s.ProcessCommand(context.Background(), Command{ID: "review_all_skills", RunID: r.ID, Content: "创建 all-skills-project，Postgres GORM JWT JSON 日志"})
	if err != nil {
		t.Fatal(err)
	}
	r = controlAcceptCommand(t, s, result.Run)
	if len(r.Control.Audit.Plan.SelectedSkills) != 3 {
		t.Fatal("review requires all three selected skills")
	}
	next, err := s.PrepareControlAction(context.Background(), r.ID)
	if err != nil || next.Execution == nil {
		t.Fatalf("valid approved three-skill plan cannot prepare its first actual action: %v", err)
	}
	kind, _, _ := scheduleAction(next.Execution.Action)
	if kind != "scaffold" {
		t.Fatalf("first action = %q", kind)
	}
}

func TestControlIntegrationReviewInvalidSuccessResultCannotLeaveReservedAttempt(t *testing.T) {
	s, before, request := reviewPreparedVerification(t)
	_, reserved, err := s.beginControlExecution(before.ID, request)
	if err != nil {
		t.Fatal(err)
	}
	err = s.finishControlVerification(before.ID, request, reserved, ControlToolsResult{}, nil)
	if err == nil {
		t.Fatal("empty caller result was accepted as verified")
	}
	after, readErr := s.Store.Get(before.ID)
	if readErr != nil {
		t.Fatal(readErr)
	}
	op := after.Control.Audit.Operations[len(after.Control.Audit.Operations)-1]
	if after.Control.Audit.Budget.Totals.Reserved.Action != 0 || after.Control.Audit.Budget.Totals.Consumed.Action != before.Control.Audit.Budget.Totals.Consumed.Action+1 || after.ActiveStep != "" || op.Status == "verified" || op.Status == "sent" {
		t.Fatalf("invalid success result did not settle original attempted work: op=%s active=%q totals=%+v error=%v", op.Status, after.ActiveStep, after.Control.Audit.Budget.Totals, err)
	}
}

type reviewModelFixture struct {
	s       *Service
	r       Run
	calls   atomic.Int64
	mode    atomic.Value
	entered chan struct{}
}

func reviewModelRun(t *testing.T) *reviewModelFixture {
	t.Helper()
	f := &reviewModelFixture{s: testService(t), entered: make(chan struct{}, 8)}
	f.mode.Store("success")
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		f.calls.Add(1)
		var input struct {
			Messages []contracts.ContextMessage `json:"messages"`
		}
		if err := json.NewDecoder(req.Body).Decode(&input); err != nil || len(input.Messages) < 2 {
			t.Error("actual model request missing frozen context", err)
			http.Error(w, "invalid", 400)
			return
		}
		var authority struct {
			Draft    contracts.ProjectPlan `json:"draft_plan"`
			Expected string                `json:"expected"`
		}
		if err := json.Unmarshal([]byte(input.Messages[1].Content), &authority); err != nil {
			t.Error(err)
			http.Error(w, "invalid", 400)
			return
		}
		var advice contracts.DecisionAdvice
		if strings.HasPrefix(authority.Expected, "Propose this exact next action;") {
			f.entered <- struct{}{}
			switch f.mode.Load().(string) {
			case "http503":
				http.Error(w, "fixture unavailable", 503)
				return
			case "wait":
				select {
				case <-req.Context().Done():
				case <-time.After(3 * time.Second):
				}
				return
			}
			position := strings.Index(authority.Expected, "{")
			if position < 0 {
				t.Error("request omitted deterministic next action")
				http.Error(w, "invalid", 400)
				return
			}
			action := contracts.CandidateAction(authority.Expected[position:])
			advice = contracts.DecisionAdvice{Kind: "action", Action: &action, Reason: "independent provider fixture returned actual requested action"}
		} else {
			authority.Draft.ProjectName = "review-model-project"
			authority.Draft.PlanHash = strings.Repeat("0", 64)
			advice = contracts.DecisionAdvice{Kind: "plan", Plan: &authority.Draft, Reason: "independent provider fixture returned actual plan"}
		}
		body, _ := json.Marshal(advice)
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []map[string]any{{"message": map[string]string{"content": string(body)}}}})
	}))
	t.Cleanup(provider.Close)
	f.s.Config.AIEndpoint = provider.URL
	f.s.Config.AIKey = "review-provider-secret"
	f.s.Config.AIModel = "review-frozen-model"
	f.r = controlFixture(t, f.s, "integration_review_model", "model")
	result, err := f.s.ProcessCommand(context.Background(), Command{ID: "review_plan", RunID: f.r.ID, Content: "创建 review-model-project"})
	if err != nil {
		t.Fatal(err)
	}
	f.r = controlAcceptCommand(t, f.s, result.Run)
	return f
}

func TestControlIntegrationReviewModelPreflightNeverClaimsProviderAttempt(t *testing.T) {
	f := reviewModelRun(t)
	before, _ := f.s.Store.Get(f.r.ID)
	for _, mode := range []string{"missing key", "changed frozen model"} {
		t.Run(mode, func(t *testing.T) {
			f.s.Config.AIKey = "review-provider-secret"
			f.s.Config.AIModel = "review-frozen-model"
			if mode == "missing key" {
				f.s.Config.AIKey = ""
			} else {
				f.s.Config.AIModel = "different-model"
			}
			_, err := f.s.PrepareControlAction(context.Background(), f.r.ID)
			if err == nil {
				t.Fatal("provider preflight failure became action")
			}
			after, readErr := f.s.Store.Get(f.r.ID)
			if readErr != nil {
				t.Fatal(readErr)
			}
			if f.calls.Load() != 1 || len(after.Control.Audit.Executions) != 0 || after.Control.Audit.Approval.StartedAt != nil || after.Control.Audit.Budget.Totals.Reserved.Model != 0 || after.Control.Audit.Budget.Totals.Consumed.Model != before.Control.Audit.Budget.Totals.Consumed.Model {
				t.Fatalf("certainly uncalled provider counted as a consumed attempt: calls=%d totals=%+v error=%v", f.calls.Load(), after.Control.Audit.Budget.Totals, err)
			}
		})
	}
}

func TestControlIntegrationReviewModelTransportFailuresSettleAndConcurrentReplayReuses(t *testing.T) {
	f := reviewModelRun(t)
	initial, _ := f.s.Store.Get(f.r.ID)
	f.mode.Store("http503")
	if _, err := f.s.PrepareControlAction(context.Background(), f.r.ID); err == nil {
		t.Fatal("provider HTTP 503 gained authority")
	}
	<-f.entered
	failed, _ := f.s.Store.Get(f.r.ID)
	if f.calls.Load() != 2 || failed.Control.Audit.Budget.Totals.Consumed.Model != 2 || failed.Control.Audit.Budget.Totals.Reserved.Model != 0 || len(failed.Control.Audit.Executions) != 0 {
		t.Fatal("failed actual provider attempt not durably charged")
	}
	deadline := *failed.Control.Audit.Budget.ExecutionDeadlineAt
	f.mode.Store("wait")
	ctx, cancel := context.WithCancel(context.Background())
	finished := make(chan error, 1)
	go func() { _, err := f.s.PrepareControlAction(ctx, f.r.ID); finished <- err }()
	select {
	case <-f.entered:
		cancel()
	case <-time.After(5 * time.Second):
		cancel()
		t.Fatal("model transport did not start")
	}
	select {
	case err := <-finished:
		if err == nil {
			t.Fatal("cancelled provider call gained authority")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("provider cancellation did not bound action preparation")
	}
	after, _ := f.s.Store.Get(f.r.ID)
	if after.Control.Audit.Budget.Totals.Consumed.Model != 3 || after.Control.Audit.Budget.Totals.Reserved.Model != 0 || len(after.Control.Audit.Executions) != 0 || after.SourceVersion != initial.SourceVersion || after.Control.Audit.Approval.StartedAt != nil || *after.Control.Audit.Budget.ExecutionDeadlineAt != deadline {
		t.Fatal("cancelled call lost budget/authority/window provenance")
	}
	f.mode.Store("success")
	var wg sync.WaitGroup
	results := make(chan ControlSchedule, 3)
	errorsSeen := make(chan error, 3)
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			out, err := f.s.PrepareControlAction(context.Background(), f.r.ID)
			results <- out
			errorsSeen <- err
		}()
	}
	wg.Wait()
	close(results)
	close(errorsSeen)
	for err := range errorsSeen {
		if err != nil {
			t.Fatal(err)
		}
	}
	id, hash := "", ""
	for out := range results {
		if out.Execution == nil {
			t.Fatal("actual provider response did not yield full request")
		}
		if id == "" {
			id, hash = out.Execution.ExecutionID, out.Execution.RequestHash
		} else if out.Execution.ExecutionID != id || out.Execution.RequestHash != hash {
			t.Fatal("concurrent retries replaced original request")
		}
	}
	after, _ = f.s.Store.Get(f.r.ID)
	if f.calls.Load() != 4 || len(after.Control.Audit.Executions) != 1 || len(after.Control.Audit.Operations) != 1 || after.Control.Audit.Budget.Totals.Consumed.Model != 4 || after.Control.Audit.Budget.Totals.Reserved.Action != 0 || after.Control.Audit.Approval.StartedAt != nil {
		t.Fatal("concurrent action preparation doubled provider/request or started side effects")
	}
}

func TestControlIntegrationReviewPrivateScheduleCannotReplaceAuthority(t *testing.T) {
	s := testService(t)
	r := controlFixture(t, s, "integration_review_private")
	_, err := s.Store.Update(r.ID, func(run *Run) error {
		run.Control.Schedule = CanonicalScheduleState{Workspace: contracts.WorkspaceReference{WorkspaceID: "workspace_fake", RunID: r.ID, StorageKey: r.ID}, Results: []CanonicalActionResult{{RunID: r.ID, SourceVersion: 1, ActionType: "package", PackageSHA256: strings.Repeat("a", 64), PackageBytes: 500, ResultHash: strings.Repeat("b", 64)}}}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	out, err := s.PrepareControlAction(context.Background(), r.ID)
	if err == nil || out.Action != nil || out.ActionsExhausted {
		t.Fatal("private scheduler facts manufactured execution authority")
	}
	after, _ := s.Store.Get(r.ID)
	if len(after.Control.Audit.Decisions) != 0 || len(after.Control.Audit.Executions) != 0 || after.Control.Audit.Budget.Totals.Consumed.Action != 0 || after.Control.Audit.Budget.Totals.Reserved.Action != 0 {
		t.Fatal("unapproved private state consumed/issued an action")
	}
	if _, err = os.Stat(filepath.Join(s.Config.DataDir, "workspaces", r.ID)); !os.IsNotExist(err) {
		t.Fatal("unapproved private state wrote files")
	}
}
