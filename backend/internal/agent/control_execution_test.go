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
)

func controlAcceptCommand(t *testing.T, s *Service, r Run) Run {
	t.Helper()
	p := r.Control.Audit.Proposal
	result, err := s.ProcessCommand(context.Background(), Command{ID: "accept_" + r.ID, RunID: r.ID, Action: "accept", ProposalID: p.ProposalID, ProposalVersion: int(p.ProposalVersion), SourceVersion: r.SourceVersion, SnapshotHash: p.SnapshotHash})
	if err != nil {
		t.Fatal(err)
	}
	return result.Run
}

func TestControlExecutionPreparesCompleteRequestAndAtomicallyAdmitsOnce(t *testing.T) {
	s := testService(t)
	r := controlFixture(t, s, "execution_admission")
	result, err := s.ProcessCommand(context.Background(), Command{ID: "execution_requirements", RunID: r.ID, Content: "创建 admitted-service"})
	if err != nil {
		t.Fatal(err)
	}
	r = controlAcceptCommand(t, s, result.Run)
	prepared, err := s.PrepareControlAction(context.Background(), r.ID)
	if err != nil {
		t.Fatal(err)
	}
	if prepared.Action == nil || prepared.Execution == nil || prepared.Operation == nil || prepared.Operation.Status != "ready" || prepared.OperationSequence != 1 {
		t.Fatal("no exact ready request", prepared)
	}
	x := *prepared.Execution
	after, _ := s.Store.Get(r.ID)
	if len(after.Control.Audit.Decisions) != 2 || len(after.Control.Audit.Executions) != 1 || after.Control.Audit.Approval.StartedAt != nil {
		t.Fatal("request preparation prematurely started application")
	}
	if after.SourceVersion != r.SourceVersion || after.Control.Audit.Budget.ExecutionDeadlineAt == nil {
		t.Fatal("source changed or fixed total deadline absent")
	}
	if _, e := os.Stat(filepath.Join(s.Config.DataDir, "workspaces", r.ID)); !os.IsNotExist(e) {
		t.Fatal("preparation wrote project files", e)
	}
	repeated, err := s.PrepareControlAction(context.Background(), r.ID)
	if err != nil || !equalContractValue(repeated.Execution, prepared.Execution) {
		t.Fatal("prepared request identity changed on retry", err)
	}
	tampered := x
	tampered.CommandID = "forged_command"
	tampered.RequestHash, _ = contracts.HashObject("ExecutionRequest", tampered)
	if _, _, err = s.beginControlExecution(r.ID, tampered); err == nil {
		t.Fatal("rehashing a forged request authorized it")
	}
	started, budget, err := s.beginControlExecution(r.ID, x)
	if err != nil {
		t.Fatal(err)
	}
	if budget.RequestHash == nil || *budget.RequestHash != x.RequestHash || started.Control.Audit.Approval.StartedAt == nil || started.Control.Audit.Approval.Status != "started" || started.Control.Audit.Operations[0].Status != "sent" || started.ActiveStep != x.ExecutionID {
		t.Fatal("admission not atomically bound")
	}
	if _, _, err = s.beginControlExecution(r.ID, x); err == nil {
		t.Fatal("sent request admitted twice")
	}
	unknown, err := s.PrepareControlAction(context.Background(), r.ID)
	if err != nil || !unknown.NeedsReconciliation || unknown.Action != nil || unknown.Execution.OperationID != x.OperationID {
		t.Fatal("sent result changed identity or retained write authority", err)
	}
	current, _ := s.Store.Get(r.ID)
	if current.Control.Audit.Budget.Totals.Reserved.Action != 1 || len(current.Control.Audit.Executions) != 1 {
		t.Fatal("unknown/replay created another effect attempt")
	}
}

func TestControlExecutionActionInModelRunReallyCallsFrozenProvider(t *testing.T) {
	s := testService(t)
	var calls atomic.Int64
	var deny atomic.Bool
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		calls.Add(1)
		var input struct {
			Messages []contracts.ContextMessage `json:"messages"`
		}
		if e := json.NewDecoder(req.Body).Decode(&input); e != nil {
			t.Error(e)
		}
		var authority struct {
			Draft    contracts.ProjectPlan `json:"draft_plan"`
			Expected string                `json:"expected"`
		}
		if e := json.Unmarshal([]byte(input.Messages[1].Content), &authority); e != nil {
			t.Error(e)
		}
		var advice contracts.DecisionAdvice
		if strings.HasPrefix(authority.Expected, "Propose this exact next action;") {
			if deny.Load() {
				http.Error(w, "unavailable", 503)
				return
			}
			start := strings.Index(authority.Expected, "{")
			if start < 0 {
				t.Error("missing expected action")
				http.Error(w, "missing", 500)
				return
			}
			action := contracts.CandidateAction([]byte(authority.Expected[start:]))
			advice = contracts.DecisionAdvice{Kind: "action", Action: &action, Reason: "actual provider proposed the supplied action"}
		} else {
			authority.Draft.ProjectName = "model-action-service"
			authority.Draft.PlanHash = strings.Repeat("0", 64)
			advice = contracts.DecisionAdvice{Kind: "plan", Plan: &authority.Draft, Reason: "actual provider plan"}
		}
		body, _ := json.Marshal(advice)
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []map[string]any{{"message": map[string]string{"content": string(body)}}}})
	}))
	defer provider.Close()
	s.Config.AIEndpoint = provider.URL
	s.Config.AIKey = "real-advice-fixture-secret"
	s.Config.AIModel = "frozen-action-model"
	r := controlFixture(t, s, "execution_model", "model")
	result, err := s.ProcessCommand(context.Background(), Command{ID: "model_action_requirements", RunID: r.ID, Content: "创建 model-action-service"})
	if err != nil {
		t.Fatal(err)
	}
	r = controlAcceptCommand(t, s, result.Run)
	deny.Store(true)
	if _, err = s.PrepareControlAction(context.Background(), r.ID); err == nil {
		t.Fatal("failed model action silently became rules")
	}
	failed, _ := s.Store.Get(r.ID)
	if len(failed.Control.Audit.Executions) != 0 || failed.Control.Audit.Approval.StartedAt != nil {
		t.Fatal("failed model action gained authority")
	}
	deadline := *failed.Control.Audit.Budget.ExecutionDeadlineAt
	deny.Store(false)
	prepared, err := s.PrepareControlAction(context.Background(), r.ID)
	if err != nil {
		t.Fatal(err)
	}
	if prepared.Execution == nil || calls.Load() != 3 {
		t.Fatal("later action did not call actual provider")
	}
	r, _ = s.Store.Get(r.ID)
	d := r.Control.Audit.Decisions[len(r.Control.Audit.Decisions)-1]
	if d.DecisionMode != "model" || d.Advice.Kind != "action" || d.ModelUsage.Status != "unknown" || r.Control.Audit.Budget.Totals.Consumed.Model != 3 || *r.Control.Audit.Budget.ExecutionDeadlineAt != deadline {
		t.Fatal("mode, actual attempts or fixed deadline changed")
	}
	if _, err = s.PrepareControlAction(context.Background(), r.ID); err != nil || calls.Load() != 3 {
		t.Fatal("replay regenerated original action", err)
	}
}

func TestControlExecutionRechecksSourceAndRevocationBeforeAdmission(t *testing.T) {
	for _, revoked := range []bool{false, true} {
		t.Run(map[bool]string{false: "source_changed", true: "revoked"}[revoked], func(t *testing.T) {
			s := testService(t)
			r := controlFixture(t, s, "admission_recheck")
			result, err := s.ProcessCommand(context.Background(), Command{ID: "recheck_input", RunID: r.ID, Content: "创建 recheck-service"})
			if err != nil {
				t.Fatal(err)
			}
			r = controlAcceptCommand(t, s, result.Run)
			x, err := s.PrepareControlAction(context.Background(), r.ID)
			if err != nil {
				t.Fatal(err)
			}
			if revoked {
				_, err = s.Store.Update(r.ID, func(run *Run) error {
					p := *run.Control.Audit.PermissionSnapshot
					p.PermissionVersion++
					p.Revoked = true
					p.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
					p.PermissionHash, _ = contracts.HashObject("PermissionSnapshot", p)
					run.Control.Audit.PermissionSnapshot = &p
					run.Control.Audit.PermissionHistory = append(run.Control.Audit.PermissionHistory, p)
					return nil
				})
				if err != nil {
					t.Fatal(err)
				}
			} else {
				// The public immutable approval remains unchanged. Moving to a new
				// business source invalidates its current admission immediately.
				_, err = s.Store.Update(r.ID, func(run *Run) error { sourceChanged(run); return nil })
				if err != nil {
					t.Fatal(err)
				}
			}
			if _, _, err = s.beginControlExecution(r.ID, *x.Execution); err == nil {
				t.Fatal("changed authority admitted prepared request")
			}
			current, _ := s.Store.Get(r.ID)
			if current.Control.Audit.Budget.Totals.Reserved.Action != 0 || current.Control.Audit.Approval.StartedAt != nil {
				t.Fatal("rejected admission reserved or started application")
			}
		})
	}
}
