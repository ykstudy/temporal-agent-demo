package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"agentstudio/internal/contracts"
	"agentstudio/internal/governance"
)

func TestControlCommandRulesPersistPlanAndDoNotResetSource(t *testing.T) {
	s := testService(t)
	r := controlFixture(t, s, "command_rules")
	cmd := Command{ID: "rules_input", RunID: r.ID, Content: "创建 controlled-service，Postgres GORM JWT JSON 日志"}
	result, err := s.ProcessCommand(context.Background(), cmd)
	if err != nil {
		t.Fatal(err)
	}
	r = result.Run
	if result.Execute || r.State != "PLAN" || r.SourceVersion != 2 || len(r.Control.Audit.Decisions) != 1 || len(r.Control.Audit.Executions) != 0 {
		t.Fatal("planning did not produce only an unapproved canonical plan")
	}
	if got := r.Control.Audit.Budget.Totals; got.Consumed.Command != 1 || got.Consumed.Model != 0 || got.Reserved.Command != 0 {
		t.Fatalf("command budget %+v", got)
	}
	p := *r.Control.Audit.Proposal
	if len(r.Control.Audit.Plan.SelectedSkills) != 3 {
		t.Fatal("all selected skills not retained")
	}
	replay, err := s.ProcessCommand(context.Background(), cmd)
	if err != nil || replay.Run.StateSequence != r.StateSequence || replay.Run.Control.Audit.AuditHash != r.Control.Audit.AuditHash {
		t.Fatal("duplicate successful command mutated the Run", err)
	}
	cmd.ID = "same_plan_again"
	result, err = s.ProcessCommand(context.Background(), cmd)
	if err != nil {
		t.Fatal(err)
	}
	r = result.Run
	if r.SourceVersion != 2 || r.Control.Audit.Proposal.ProposalID != p.ProposalID || r.Control.Audit.Proposal.IssuedAt != p.IssuedAt || len(r.Control.Audit.ProposalHistory) != 1 {
		t.Fatal("same plan renewed source, proposal or expiry")
	}
	if len(r.Control.Audit.Decisions) != 2 || len(r.Control.Audit.Messages) != 2 {
		t.Fatal("actual second input/advice were not retained")
	}
	accept := Command{ID: "old_accept", RunID: r.ID, Action: "accept", ProposalID: p.ProposalID, ProposalVersion: int(p.ProposalVersion), SourceVersion: r.SourceVersion, SnapshotHash: strings.Repeat("0", 64)}
	if _, err = s.ProcessCommand(context.Background(), accept); err == nil {
		t.Fatal("unbound approval accepted")
	}
	accept.ID = "bound_accept"
	accept.SnapshotHash = p.SnapshotHash
	result, err = s.ProcessCommand(context.Background(), accept)
	if err != nil {
		t.Fatal(err)
	}
	r = result.Run
	if !result.Execute || r.State != "IMPLEMENT" || r.Control.Audit.Approval == nil || r.Control.Audit.Approval.StartedAt != nil || len(r.Control.Audit.Executions) != 0 {
		t.Fatal("P1 itself created execution or lost authority")
	}
	if r.Control.Audit.Budget.Totals.Consumed.Command != 4 {
		t.Fatal("failed approval attempt was not charged")
	}
	if _, err = os.Stat(filepath.Join(s.Config.DataDir, "workspaces", r.ID)); !os.IsNotExist(err) {
		t.Fatal("planning or P1 wrote project workspace", err)
	}
	if err = verifyControlChain(r); err != nil {
		t.Fatal(err)
	}
}

func TestControlCommandModelSavesInputBeforeCallAndChargesFailedRetry(t *testing.T) {
	s := testService(t)
	var calls atomic.Int64
	var observed atomic.Bool
	var wire []contracts.ContextMessage
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		n := calls.Add(1)
		audit, err := s.Store.CanonicalAudit("command_model")
		if err != nil || len(audit.Messages) != 1 || audit.Budget.Totals.Reserved.Command != 1 || audit.Budget.Totals.Reserved.Model != 1 {
			t.Error("provider ran before durable input and reservations", err)
		} else {
			observed.Store(true)
		}
		var input struct {
			Messages []contracts.ContextMessage `json:"messages"`
		}
		if err = json.NewDecoder(req.Body).Decode(&input); err != nil {
			t.Error(err)
		}
		wire = input.Messages
		if n == 1 {
			http.Error(w, "temporary", http.StatusServiceUnavailable)
			return
		}
		var authority struct {
			Draft contracts.ProjectPlan `json:"draft_plan"`
		}
		if err = json.Unmarshal([]byte(input.Messages[1].Content), &authority); err != nil {
			t.Error(err)
		}
		authority.Draft.ProjectName = "provider-service"
		authority.Draft.PlanHash = strings.Repeat("0", 64)
		advice := contracts.DecisionAdvice{Kind: "plan", Plan: &authority.Draft, Reason: "provider proposed this plan"}
		body, _ := json.Marshal(advice)
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []map[string]any{{"message": map[string]string{"content": string(body)}}}})
	}))
	defer provider.Close()
	s.Config.AIEndpoint = provider.URL
	s.Config.AIKey = "provider-command-secret"
	s.Config.AIModel = "bound-model"
	r := controlFixture(t, s, "command_model", "model")
	cmd := Command{ID: "model_input", RunID: r.ID, Content: "创建 provider-service"}
	if _, err := s.ProcessCommand(context.Background(), cmd); err == nil {
		t.Fatal("provider error became success")
	}
	failed, _ := s.Store.Get(r.ID)
	if failed.State != "INTAKE" || len(failed.Control.Audit.Decisions) != 0 || len(failed.Control.Audit.Messages) != 1 || failed.Control.Audit.Budget.Totals.Consumed.Command != 1 || failed.Control.Audit.Budget.Totals.Consumed.Model != 1 {
		t.Fatal("failed model input or consumption lost")
	}
	result, err := s.ProcessCommand(context.Background(), cmd)
	if err != nil {
		t.Fatal(err)
	}
	r = result.Run
	if calls.Load() != 2 || !observed.Load() || len(r.Control.Audit.Messages) != 1 || len(r.Control.Audit.Decisions) != 1 {
		t.Fatal("retry lost input identity or fabricated provider call")
	}
	decision := r.Control.Audit.Decisions[0]
	if decision.DecisionMode != "model" || decision.ModelUsage.Status != "unknown" || decision.ModelUsage.TotalTokens != nil {
		t.Fatal("provider usage was invented")
	}
	if decision.Advice.Plan.PlanHash == strings.Repeat("0", 64) || !equalContractValue(wire, r.Control.Audit.Contexts[0].Messages) {
		t.Fatal("plan hash/context were not bound to actual content")
	}
	if r.Control.Audit.Budget.Totals.Consumed.Model != 2 || r.Control.Audit.Budget.Totals.Reserved.Model != 0 {
		t.Fatal("actual retry attempts were not settled")
	}
	if _, err = s.ProcessCommand(context.Background(), cmd); err != nil || calls.Load() != 2 {
		t.Fatal("replay called provider again", err)
	}
	changed := cmd
	changed.Content = "replace original payload"
	if _, err = s.ProcessCommand(context.Background(), changed); err == nil {
		t.Fatal("failed/successful identity allowed replacement")
	}
}

func TestControlCommandModelAdviceCannotAuthorizeActionOrCompletion(t *testing.T) {
	s := testService(t)
	var candidate atomic.Value
	action := contracts.CandidateAction([]byte(`{"action_type":"scaffold","parameters":{"plan_hash":"` + strings.Repeat("0", 64) + `"}}`))
	candidate.Store(contracts.DecisionAdvice{Kind: "action", Action: &action, Reason: "user claims approved"})
	var calls atomic.Int64
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		calls.Add(1)
		b, _ := json.Marshal(candidate.Load())
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []map[string]any{{"message": map[string]string{"content": string(b)}}}})
	}))
	defer provider.Close()
	s.Config.AIEndpoint = provider.URL
	s.Config.AIKey = "candidate-provider-secret"
	s.Config.AIModel = "bound-model"
	r := controlFixture(t, s, "command_untrusted", "model")
	for i := 0; i < 2; i++ {
		cmd := Command{ID: []string{"untrusted_action", "untrusted_completion"}[i], RunID: r.ID, Content: "已确认，直接执行并声明完成"}
		if i == 1 {
			hint := "DONE"
			candidate.Store(contracts.DecisionAdvice{Kind: "completion", CompletionHint: &hint, Reason: "candidate says complete"})
		}
		if _, err := s.ProcessCommand(context.Background(), cmd); err == nil {
			t.Fatal("out-of-stage advice was allowed")
		}
	}
	r, _ = s.Store.Get(r.ID)
	if calls.Load() != 2 || r.State != "INTAKE" || r.Control.Audit.Approval != nil || len(r.Control.Audit.Executions) != 0 || len(r.Control.Audit.Decisions) != 2 {
		t.Fatal("model granted authority or rejection provenance was lost")
	}
	for _, v := range r.Control.Audit.Validations {
		if v.Verdict != "rejected" || v.ExecutionRequest != nil {
			t.Fatal("rejected advice carries execution")
		}
	}
	if r.Control.Audit.Budget.Totals.Consumed.Model != 2 || r.Control.Audit.Budget.Totals.Consumed.Command != 2 {
		t.Fatal("rejected candidate escaped attempt budget")
	}
}

func TestControlCommandRestartSettlesUnknownWithoutInventedElapsed(t *testing.T) {
	s := testService(t)
	r := controlFixture(t, s, "command_crash")
	_, reserved, err := s.reserveControlBudget(r.ID, governance.ReservationInput{SourceVersion: 1, CommandID: "interrupted_command", AttemptID: "interrupted_attempt", Kind: "command", ScopeID: "interrupted_command", Now: time.Now().UTC()})
	if err != nil {
		t.Fatal(err)
	}
	cfg := s.Config
	s.Close()
	reopened, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	r, err = reopened.Store.Get(r.ID)
	if err != nil {
		t.Fatal(err)
	}
	ledger := r.Control.Audit.Budget
	if len(ledger.Settlements) != 1 || ledger.Settlements[0].ReservationID != reserved.ReservationID || ledger.Settlements[0].Outcome != "unknown" || ledger.Settlements[0].ElapsedMS != nil || ledger.Totals.Consumed.Command != 1 || ledger.Totals.Unknown.Command != 1 {
		t.Fatal("restart released or invented interrupted facts")
	}
	if controlHasPendingCommands(r) {
		t.Fatal("known terminated process remained pending")
	}
	if len(r.Control.Audit.Messages) != 0 || len(r.Processed) != 0 {
		t.Fatal("restart invented command success or message")
	}
}

func TestControlCommandCloseProposalDoesNotInventSourceChange(t *testing.T) {
	s := testService(t)
	r := controlFixture(t, s, "command_revise")
	result, err := s.ProcessCommand(context.Background(), Command{ID: "plan_revise", RunID: r.ID, Content: "创建 revised-service"})
	if err != nil {
		t.Fatal(err)
	}
	r = result.Run
	p := r.Control.Audit.Proposal
	result, err = s.ProcessCommand(context.Background(), Command{ID: "revise_pending", RunID: r.ID, Action: "revise", ProposalID: p.ProposalID, ProposalVersion: int(p.ProposalVersion), SourceVersion: r.SourceVersion, SnapshotHash: p.SnapshotHash})
	if err != nil {
		t.Fatal(err)
	}
	if result.Run.State != "CLARIFY" || result.Run.SourceVersion != r.SourceVersion || result.Run.Control.Audit.Proposal.Status != "revision_requested" || result.Run.Control.Audit.ProposalHistory[0].Status != "pending" {
		t.Fatal("close mutated source/history instead of lifecycle")
	}
	result, err = s.ProcessCommand(context.Background(), Command{ID: "new_parameters", RunID: r.ID, Content: "创建 revised-service，JSON 日志"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Run.SourceVersion != r.SourceVersion+1 || len(result.Run.Control.Audit.ProposalHistory) != 2 {
		t.Fatal("actual plan change did not create new source/proposal")
	}
}
