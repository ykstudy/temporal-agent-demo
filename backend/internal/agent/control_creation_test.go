package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"agentstudio/internal/contracts"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/testsuite"
	"go.temporal.io/sdk/workflow"
)

func controlCreateOptions(t *testing.T, s *Service, id, mode string) CreateOptions {
	t.Helper()
	d, err := s.Definitions.PublishBundled("3.2.0")
	if err != nil {
		t.Fatal(err)
	}
	return CreateOptions{ID: id, Content: "创建 new-service，SQLite + GORM、JWT 认证和 JSON 日志", DecisionMode: mode, DeliveryMode: "local", DefinitionID: d.DefinitionID, DefinitionVersion: d.DefinitionVersion, DefinitionHash: d.DefinitionHash}
}

func TestControlPublicSelectionAndTerminalRedo(t *testing.T) {
	s := testService(t)
	opts := controlCreateOptions(t, s, "public_s2_creation", "rules")
	r, err := s.CreateWithOptions(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	if r.State != "PLAN" || r.Control == nil || r.Control.Audit.ContractVersion != "1.3.0" || r.Control.Audit.Approval != nil || len(r.Control.Audit.Decisions) != 1 || len(r.Control.Audit.Executions) != 0 || len(r.Control.Audit.Plan.SelectedSkills) != 3 {
		t.Fatal("public selection did not create the complete controlled plan")
	}
	replayed, err := s.CreateWithOptions(context.Background(), opts)
	if err != nil || !equalContractValue(replayed.Control.Audit, r.Control.Audit) {
		t.Fatal("create replay", err)
	}
	cancelled, err := s.ProcessCommand(context.Background(), Command{ID: "cancel_original", RunID: r.ID, Action: "cancel"})
	if err != nil {
		t.Fatal(err)
	}
	cmd := Command{ID: "redo_original", RunID: r.ID, Action: "retry"}
	redone, err := s.ProcessCommand(context.Background(), cmd)
	if err != nil {
		t.Fatal(err)
	}
	n := redone.Run
	if redone.Execute || n.ID == r.ID || n.RetryOf != r.ID || n.State != "PLAN" || n.Control == nil || n.Control.Audit.Approval != nil || n.Workspace != "" || len(n.Control.Audit.Executions) != 0 || len(n.Control.Audit.Budget.Reservations) != 1 || n.Control.Audit.Budget.ExecutionDeadlineAt != nil || !equalContractValue(n.Control.Audit.Plan, r.Control.Audit.Plan) {
		t.Fatal("redo inherited authority or reduced the complete plan")
	}
	input, _ := json.Marshal(cmd)
	var part contracts.TextPart
	if json.Unmarshal(n.Control.Audit.Messages[0].Parts[0], &part) != nil || part.Text != string(input) {
		t.Fatal("redo fabricated user input")
	}
	old, err := s.Store.Get(r.ID)
	if err != nil || old.State != cancelled.Run.State || old.Control.Audit.Plan.PlanHash != r.Control.Audit.Plan.PlanHash {
		t.Fatal("old terminal Run changed", err)
	}
	again, err := s.ProcessCommand(context.Background(), cmd)
	if err != nil || !equalContractValue(again.Run.Control.Audit, n.Control.Audit) {
		t.Fatal("redo replay changed successor", err)
	}
	if err = contracts.Validate("S2AuditBundle", n.Control.Audit); err != nil {
		t.Fatal(err)
	}
}

func TestControlModelRedoFailureKeepsStableSuccessor(t *testing.T) {
	var calls atomic.Int32
	var failRetry atomic.Bool
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		calls.Add(1)
		var payload struct {
			Messages []contracts.ContextMessage `json:"messages"`
		}
		if json.NewDecoder(req.Body).Decode(&payload) != nil || len(payload.Messages) < 2 {
			t.Fatal("missing real context")
		}
		var facts struct {
			Draft contracts.ProjectPlan `json:"draft_plan"`
			Retry json.RawMessage       `json:"retry_origin"`
		}
		if json.Unmarshal([]byte(payload.Messages[1].Content), &facts) != nil {
			t.Fatal("bad authority context")
		}
		if len(facts.Retry) > 0 && failRetry.CompareAndSwap(true, false) {
			http.Error(w, "explicit mock provider failure", 503)
			return
		}
		facts.Draft.ProjectName = "new-service"
		advice := contracts.DecisionAdvice{Kind: "plan", Plan: &facts.Draft, Reason: "explicit model fixture proposes full draft"}
		if err := contracts.ValidateVersion("DecisionAdvice", "1.3.0", advice); err != nil {
			t.Fatal(err)
		}
		body, _ := json.Marshal(advice)
		writeJSON(w, 200, map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": string(body)}}}})
	}))
	defer provider.Close()
	s := testService(t)
	s.Config.AIEndpoint, s.Config.AIKey, s.Config.AIModel = provider.URL, "controlled-fixture-key", "mock-s2"
	r, err := s.CreateWithOptions(context.Background(), controlCreateOptions(t, s, "model_redo_origin", "model"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.ProcessCommand(context.Background(), Command{ID: "cancel_model_origin", RunID: r.ID, Action: "cancel"}); err != nil {
		t.Fatal(err)
	}
	cmd := Command{ID: "redo_model_origin", RunID: r.ID, Action: "retry"}
	failRetry.Store(true)
	if _, err = s.ProcessCommand(context.Background(), cmd); err == nil {
		t.Fatal("provider failure became success")
	}
	failed, err := s.Store.Get(retryRunID(r.ID, cmd.ID))
	if err != nil || failed.Control == nil || len(failed.Control.Audit.Messages) != 1 || failed.Control.Audit.Plan != nil {
		t.Fatal("failed redo lost initialized successor", err)
	}
	result, err := s.ProcessCommand(context.Background(), cmd)
	if err != nil || result.Run.State != "PLAN" || result.Run.ID != failed.ID || calls.Load() != 3 || len(result.Run.Control.Audit.Messages) != 1 {
		t.Fatal("redo retry changed identity or skipped real provider", err, calls.Load())
	}
	if len(result.Run.Control.Audit.Budget.Settlements) < 4 || result.Run.Control.Audit.Decisions[0].DecisionMode != "model" {
		t.Fatal("failed attempts or model identity missing")
	}
	if _, err = s.ProcessCommand(context.Background(), cmd); err != nil || calls.Load() != 3 {
		t.Fatal("successful redo replay called provider", err)
	}
}

func TestControlStartupConsumesLostActionAndRejectsResume(t *testing.T) {
	s := testService(t)
	r, err := s.CreateWithOptions(context.Background(), controlCreateOptions(t, s, "lost_original_action", "rules"))
	if err != nil {
		t.Fatal(err)
	}
	r = controlAcceptCommand(t, s, r)
	next, err := s.PrepareControlAction(context.Background(), r.ID)
	if err != nil || next.Execution == nil {
		t.Fatal(err)
	}
	r, reservation, err := s.beginControlExecution(r.ID, *next.Execution)
	if err != nil {
		t.Fatal(err)
	}
	deadline := *r.Control.Audit.Budget.ExecutionDeadlineAt
	cfg := s.Config
	s.Close()
	restarted, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.Close()
	r, err = restarted.Store.Get(r.ID)
	if err != nil || r.State != "BLOCKED" || r.ResumeTo != "IMPLEMENT" || r.ActiveStep != "" || *r.Control.Audit.Budget.ExecutionDeadlineAt != deadline || r.Control.Audit.Budget.Totals.Unknown.Action != 1 || r.Control.Audit.Budget.Totals.Reserved.Action != 0 || len(r.Control.Audit.WorkspaceReceipts) != 0 {
		t.Fatal("lost action recovery invented or released facts", err)
	}
	settled := false
	for _, q := range r.Control.Audit.Budget.Settlements {
		if q.ReservationID == reservation.ReservationID {
			settled = q.Outcome == "unknown" && q.ConsumedUnits == 1 && q.ElapsedMS == nil
		}
	}
	if !settled {
		t.Fatal("lost action charge missing")
	}
	if _, err = restarted.ProcessCommand(context.Background(), Command{ID: "resume_lost_action", RunID: r.ID, Action: "resume"}); err == nil {
		t.Fatal("unknown action allowed resume")
	}
	if _, _, err = restarted.ExecuteControlWorkspace(context.Background(), r.ID, *next.Execution); err == nil {
		t.Fatal("unknown action replayed write")
	}
	current, err := restarted.Store.Get(r.ID)
	if err != nil || current.State != "BLOCKED" || current.Control.Audit.Budget.Totals.Consumed.Action != 1 || len(current.Control.Audit.WorkspaceReceipts) != 0 {
		t.Fatal("rejected replay charged another action or changed state", err)
	}
}

func TestTemporalControlRefusalDoesNotCallModelAgain(t *testing.T) {
	var calls atomic.Int32
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		var body struct {
			Messages []contracts.ContextMessage `json:"messages"`
		}
		if json.NewDecoder(req.Body).Decode(&body) != nil {
			t.Fatal("provider context")
		}
		var facts map[string]json.RawMessage
		if json.Unmarshal([]byte(body.Messages[1].Content), &facts) != nil {
			t.Fatal("facts")
		}
		var advice contracts.DecisionAdvice
		if calls.Add(1) == 1 {
			var plan contracts.ProjectPlan
			if json.Unmarshal(facts["draft_plan"], &plan) != nil {
				t.Fatal("draft")
			}
			plan.ProjectName = "refusal-project"
			advice = contracts.DecisionAdvice{Kind: "plan", Plan: &plan, Reason: "explicit planning fixture"}
		} else {
			action := contracts.CandidateAction([]byte(`{"action_type":"scaffold","parameters":{"plan_hash":"ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"}}`))
			advice = contracts.DecisionAdvice{Kind: "action", Action: &action, Reason: "valid-schema deliberate wrong plan hash"}
		}
		if err := contracts.ValidateVersion("DecisionAdvice", "1.3.0", advice); err != nil {
			t.Fatal(err)
		}
		data, _ := json.Marshal(advice)
		writeJSON(w, 200, map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": string(data)}}}})
	}))
	defer provider.Close()
	s := testService(t)
	s.Config.AIEndpoint, s.Config.AIKey, s.Config.AIModel = provider.URL, "refusal-fixture-key", "mock-refusal"
	opts := controlCreateOptions(t, s, "temporal_refusal", "model")
	opts.Content = "创建 refusal-project"
	r, err := s.CreateWithOptions(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	r = controlAcceptCommand(t, s, r)
	var suite testsuite.WorkflowTestSuite
	env := suite.NewTestWorkflowEnvironment()
	env.SetTestTimeout(2 * time.Minute)
	env.RegisterActivityWithOptions(s.TemporalExecuteStep, activity.RegisterOptions{Name: "ExecuteStep"})
	env.ExecuteWorkflow(func(ctx workflow.Context) error {
		ctx = workflow.WithActivityOptions(ctx, workflow.ActivityOptions{StartToCloseTimeout: time.Minute, RetryPolicy: &temporal.RetryPolicy{MaximumAttempts: 3}})
		var result StepResult
		return workflow.ExecuteActivity(ctx, "ExecuteStep", StepInput{RunID: r.ID, Step: "scaffold"}).Get(ctx, &result)
	})
	if env.GetWorkflowError() == nil || calls.Load() != 2 {
		t.Fatal("business refusal retried provider", env.GetWorkflowError(), calls.Load())
	}
	current, err := s.Store.Get(r.ID)
	if err != nil || len(current.Control.Audit.Executions) != 0 || current.Control.Audit.Budget.Totals.Consumed.Action != 0 || len(current.Control.Audit.Decisions) != 2 {
		t.Fatal("refusal created side effects or lost rejected chain", err)
	}
}

func TestControlProgressQueriesPreserveProposalAndAuthority(t *testing.T) {
	s := testService(t)
	opts := controlCreateOptions(t, s, "control_progress_query", "rules")
	r, err := s.CreateWithOptions(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	original := clone(r)
	for i, content := range []string{"查看进度", "查询进度", "progress"} {
		cmd := Command{RunID: r.ID, ID: fmt.Sprintf("progress_%d", i), Content: content}
		result, err := s.ProcessCommand(context.Background(), cmd)
		if err != nil {
			t.Fatal(err)
		}
		r = result.Run
		if r.SourceVersion != original.SourceVersion || r.StateSequence <= original.StateSequence || !chainEqual(r.Control.Audit.Plan, original.Control.Audit.Plan) || !chainEqual(r.Control.Audit.Proposal, original.Control.Audit.Proposal) || r.Control.Audit.Approval != nil || len(r.Control.Audit.Executions) != 0 || len(r.Control.Audit.Decisions) != len(original.Control.Audit.Decisions) {
			t.Fatal("progress query changed business scope or authority")
		}
		before := r.StateSequence
		replay, err := s.ProcessCommand(context.Background(), cmd)
		if err != nil || replay.Run.StateSequence != before || len(replay.Run.Control.Audit.Messages) != len(r.Control.Audit.Messages) {
			t.Fatal("progress command replay changed original input or state", err)
		}
	}
}
