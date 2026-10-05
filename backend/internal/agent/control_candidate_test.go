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

// The extra unused requirement is trusted test input before P1. Every later
// candidate byte, receipt and settlement is produced by real controlled tools.
func controlCandidateFixture(t *testing.T, s *Service, mode string) (Run, contracts.ExecutionRequest) {
	t.Helper()
	r := newControlCompletionRun(t, s, mode)
	result, err := s.ProcessCommand(context.Background(), Command{ID: "candidate_input", RunID: r.ID, Content: "创建 candidate-service"})
	if err != nil {
		t.Fatal(err)
	}
	r, err = s.Store.Update(r.ID, func(run *Run) error {
		files, e := s.renderFrozenFiles(*run)
		if e != nil {
			return e
		}
		files["go.mod"] += "\nrequire example.com/unused v1.0.0\n"
		sourceChanged(run)
		return s.makeCanonicalProposal(run, *result.Run.Control.Audit.Plan, files, time.Now().UTC())
	})
	if err != nil {
		t.Fatal(err)
	}
	r = controlAcceptCommand(t, s, r)
	next, err := s.PrepareControlAction(context.Background(), r.ID)
	if err != nil || next.Execution == nil {
		t.Fatalf("prepare scaffold: %v", err)
	}
	if _, _, err = s.ExecuteControlWorkspace(context.Background(), r.ID, *next.Execution); err != nil {
		t.Fatal(err)
	}
	next, err = s.PrepareControlAction(context.Background(), r.ID)
	if err != nil || next.Execution == nil {
		t.Fatalf("prepare verify: %v", err)
	}
	request := *next.Execution
	tools, err := s.ExecuteControlVerification(context.Background(), request)
	if err != nil || !tools.ReapprovalRequired || len(tools.Receipts) != 1 || tools.Receipts[0].Status != "succeeded" {
		t.Fatalf("real tidy candidate: %v %+v", err, tools)
	}
	r, err = s.Store.Get(r.ID)
	if err != nil || r.State != "BLOCKED" || r.ResumeTo != "VERIFY" || len(r.Control.Audit.VerificationCandidates) != 1 {
		t.Fatalf("candidate not durably blocked: %v", err)
	}
	return r, request
}

func candidateReviseCommand(r Run, id string) Command {
	cmd := approvalCommand(r, id)
	cmd.Action = "revise"
	return cmd
}

func TestControlCandidateRealRevisionReapprovalNewWorkspaceAndCompletion(t *testing.T) {
	s := testService(t)
	r, originalRequest := controlCandidateFixture(t, s, "rules")
	old := clone(r)
	candidate := r.Control.Audit.VerificationCandidates[0]
	if _, _, _, err := controlCandidateEvidence(r, candidate); err != nil {
		t.Fatal(err)
	}
	for _, mutation := range []string{"source", "operation", "tool", "environment", "output", "missing_settlement", "unknown", "failed_tool_success_budget", "parent_attempt", "parent_measurement", "files", "changed_paths"} {
		t.Run(mutation, func(t *testing.T) {
			changed := clone(old)
			c := changed.Control.Audit.VerificationCandidates[0]
			receipt := &changed.Control.Audit.ToolReceipts[0]
			switch mutation {
			case "source":
				receipt.SourceVersion++
			case "operation":
				receipt.OperationID += "_other"
			case "tool":
				receipt.Tool.Version += ".other"
			case "environment":
				receipt.CwdScope = "other"
			case "output":
				receipt.Output += " changed"
			case "missing_settlement":
				for i, settlement := range changed.Control.Audit.Budget.Settlements {
					if settlement.AttemptID == receipt.AttemptID {
						changed.Control.Audit.Budget.Settlements = append(changed.Control.Audit.Budget.Settlements[:i], changed.Control.Audit.Budget.Settlements[i+1:]...)
						break
					}
				}
			case "unknown", "parent_attempt", "parent_measurement":
				for i := range changed.Control.Audit.Budget.Settlements {
					settlement := &changed.Control.Audit.Budget.Settlements[i]
					if mutation == "unknown" && settlement.AttemptID == receipt.AttemptID {
						settlement.Outcome = "unknown"
					} else if settlement.AttemptID == c.AttemptID {
						if mutation == "parent_attempt" {
							settlement.AttemptID += "_other"
						} else if mutation == "parent_measurement" {
							settlement.MeasurementSource, settlement.ElapsedMS = "unknown", nil
						}
					}
					settlement.SettlementHash, _ = contracts.HashObject("BudgetSettlement", *settlement)
				}
			case "failed_tool_success_budget":
				receipt.Status = "failed"
				code := int64(1)
				receipt.ExitCode = &code
			case "files":
				c.Files[0].Content += " changed"
			case "changed_paths":
				c.ChangedPaths = []string{"README.md"}
			}
			// Rehash the forged bodies and references. The semantic check must
			// reject validly hashed but incorrectly bound evidence as well.
			receipt.ReceiptHash, _ = contracts.HashObject("ToolExecutionReceipt", *receipt)
			c.ToolReceiptRefs[0].Hash = receipt.ReceiptHash
			for i := range changed.Control.ObjectRefs {
				if changed.Control.ObjectRefs[i].ObjectID == receipt.ReceiptID {
					changed.Control.ObjectRefs[i].Hash = receipt.ReceiptHash
				}
			}
			c.CandidateHash, _ = contracts.HashObject("VerificationCandidate", c)
			if _, _, _, e := controlCandidateEvidence(changed, c); e == nil {
				t.Fatalf("forged candidate accepted: %s", mutation)
			}
		})
	}
	stale := candidateReviseCommand(r, "stale_candidate_revision")
	stale.SourceVersion++
	if _, err := s.ProcessCommand(context.Background(), stale); err == nil {
		t.Fatal("stale revision accepted")
	}
	if _, err := s.RetryControlVerification(context.Background(), r.ID, originalRequest); err == nil {
		t.Fatal("candidate changed content retried under original approval")
	}
	command := candidateReviseCommand(r, "candidate_revision")
	result, err := s.ProcessCommand(context.Background(), command)
	if err != nil {
		t.Fatal(err)
	}
	r = result.Run
	if result.Execute || r.State != "PLAN" || r.SourceVersion != old.SourceVersion+1 || r.Control.Audit.Approval != nil || r.Control.Audit.Manifest != nil || r.Workspace != "" || r.Control.Audit.Plan.VerificationCandidateRef == nil || !equalContractValue(r.Control.Audit.Plan.VerificationCandidateRef, controlCandidateRef(candidate)) || *r.Control.Audit.Budget.ExecutionDeadlineAt != *old.Control.Audit.Budget.ExecutionDeadlineAt || r.Control.Audit.Budget.Totals.Consumed.Tool != old.Control.Audit.Budget.Totals.Consumed.Tool {
		t.Fatal("revision failed to preserve budget and require new approval")
	}
	for _, change := range r.Control.Audit.Proposal.Changes {
		if change.Path == "go.mod" && (change.Operation != "update" || change.Before == nil || !strings.Contains(*change.Before, "unused") || change.After == nil || strings.Contains(*change.After, "unused")) {
			t.Fatal("revision did not expose exact before/after content")
		}
	}
	replay, err := s.ProcessCommand(context.Background(), command)
	if err != nil || replay.Run.Proposal.ID != r.Proposal.ID || replay.Run.SourceVersion != r.SourceVersion || replay.Run.Control.Audit.Budget.LedgerVersion != r.Control.Audit.Budget.LedgerVersion {
		t.Fatalf("revision replay generated new state: %v", err)
	}
	if _, err = s.ProcessCommand(context.Background(), approvalCommand(old, "stale_old_accept")); err == nil {
		t.Fatal("old approval accepted new source")
	}
	if _, err = s.ExecuteControlVerification(context.Background(), originalRequest); err == nil {
		t.Fatal("old request executed after revision")
	}
	// Restart at the pending P1 boundary; old source and candidate survive.
	s.Close()
	restored, err := New(s.Config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(restored.Close)
	accepted, err := restored.ProcessCommand(context.Background(), approvalCommand(r, "accept_revised_source"))
	if err != nil || !accepted.Execute {
		t.Fatalf("new P1: %v", err)
	}
	for _, step := range executionSteps {
		if _, err = restored.ExecuteStep(context.Background(), StepInput{RunID: r.ID, Step: step}); err != nil {
			t.Fatalf("controlled dispatcher %s: %v", step, err)
		}
	}
	finished, err := restored.Store.Get(r.ID)
	if err != nil || finished.State != "DONE" || finished.Control.Audit.VerificationReport == nil || finished.Control.Audit.VerificationReport.Outcome != "passed" {
		t.Fatalf("reapproved actual delivery: %v state=%s", err, finished.State)
	}
	if finished.Workspace == old.Workspace || len(finished.Control.Audit.ApprovalHistory) != 2 || len(finished.Control.Audit.VerificationCandidates) != 1 || len(finished.Control.Audit.ToolReceipts) != 4 {
		t.Fatal("new generation overwrote history or omitted actual verification")
	}
	oldMod, err := os.ReadFile(filepath.Join(old.Workspace, "go.mod"))
	if err != nil || !strings.Contains(string(oldMod), "unused") {
		t.Fatalf("old approved directory changed: %v", err)
	}
	newMod, err := os.ReadFile(filepath.Join(finished.Workspace, "go.mod"))
	if err != nil || strings.Contains(string(newMod), "unused") {
		t.Fatalf("new directory did not use approved candidate: %v", err)
	}
	writeCandidateTestEvidence(t, restored, finished, "rules-reapproval-done")
}

func TestControlCandidateModelFailurePreservesOriginalAuthority(t *testing.T) {
	var behavior atomic.Int64
	var calls atomic.Int64
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		calls.Add(1)
		if behavior.Load() == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		var request struct {
			Messages []contracts.ContextMessage `json:"messages"`
		}
		if json.NewDecoder(req.Body).Decode(&request) != nil || len(request.Messages) < 2 {
			t.Error("missing actual model context")
			w.WriteHeader(400)
			return
		}
		var facts struct {
			Plan     contracts.ProjectPlan `json:"draft_plan"`
			Expected string                `json:"expected"`
			Observed struct {
				Plan contracts.ProjectPlan `json:"draft_plan"`
			} `json:"observed_facts"`
		}
		if json.Unmarshal([]byte(request.Messages[1].Content), &facts) != nil {
			t.Error("invalid model authority context")
		}
		advice := contracts.DecisionAdvice{Kind: "plan", Reason: "actual fixture provider suggests observed scope only"}
		prefix := "Propose this exact next action; it remains subject to deterministic authorization: "
		if strings.HasPrefix(facts.Expected, prefix) {
			action := contracts.CandidateAction(strings.TrimPrefix(facts.Expected, prefix))
			advice.Kind, advice.Action = "action", &action
		} else if facts.Observed.Plan.VerificationCandidateRef != nil {
			advice.Plan = &facts.Observed.Plan
			if behavior.Load() == 2 {
				advice.Plan.Description += " model scope change"
			}
			advice.Plan.PlanHash, _ = contracts.HashObject("ProjectPlan", *advice.Plan)
		} else {
			facts.Plan.ProjectName = "candidate-service"
			facts.Plan.PlanHash, _ = contracts.HashObject("ProjectPlan", facts.Plan)
			advice.Plan = &facts.Plan
		}
		body, _ := json.Marshal(advice)
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": string(body)}}}, "usage": map[string]int{"prompt_tokens": 2, "completion_tokens": 3, "total_tokens": 5}})
	}))
	defer provider.Close()
	s := testService(t)
	s.Config.AIEndpoint, s.Config.AIModel, s.Config.AIKey = provider.URL, "candidate-fixture", "test-only-key"
	r, _ := controlCandidateFixture(t, s, "model")
	original := clone(r)
	for _, mode := range []int64{1, 2} {
		behavior.Store(mode)
		id := "revision_provider_failed"
		if mode == 2 {
			id = "revision_scope_rejected"
		}
		before := calls.Load()
		if _, err := s.ProcessCommand(context.Background(), candidateReviseCommand(original, id)); err == nil {
			t.Fatal("provider failure or expanded revision accepted")
		}
		r, _ = s.Store.Get(r.ID)
		if r.State != "BLOCKED" || r.ResumeTo != "VERIFY" || r.SourceVersion != original.SourceVersion || r.Control.Audit.Approval.ApprovalHash != original.Control.Audit.Approval.ApprovalHash || r.Workspace != original.Workspace || calls.Load() != before+1 || r.Control.Audit.Budget.Totals.Reserved.Model != 0 {
			t.Fatal("failed model revision changed authority or dropped charge")
		}
	}
	behavior.Store(0)
	result, err := s.ProcessCommand(context.Background(), candidateReviseCommand(original, "revision_model_success"))
	if err != nil || result.Run.State != "PLAN" || result.Run.SourceVersion != original.SourceVersion+1 || result.Run.Control.Audit.Approval != nil {
		t.Fatalf("real model candidate adoption: %v", err)
	}
	last := result.Run.Control.Audit.Decisions[len(result.Run.Control.Audit.Decisions)-1]
	if last.DecisionMode != "model" || last.Advice.Kind != "plan" || last.ModelUsage.Status != "known" || last.Advice.Plan.VerificationCandidateRef == nil {
		t.Fatal("model revision was fabricated as rules or lost actual usage")
	}
	writeCandidateTestEvidence(t, s, result.Run, "model-revision-plan")
}

func writeCandidateTestEvidence(t *testing.T, s *Service, r Run, name string) {
	t.Helper()
	directory := os.Getenv("CONTROL_RECOVERY_EVIDENCE_DIR")
	if directory == "" {
		return
	}
	audit, err := s.Store.CanonicalAudit(r.ID)
	if err != nil {
		t.Fatal(err)
	}
	body, err := json.MarshalIndent(map[string]any{"kind": "real_controlled_tools_and_revision", "initial_proposal": "trusted test-only unused dependency supplied before P1", "public_creation": false, "live_provider": false, "state": r.State, "audit": audit}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.MkdirAll(directory, 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(directory, name+".json"), append(body, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	if r.State == "DONE" {
		receipt := r.Control.Audit.PackageReceipts[len(r.Control.Audit.PackageReceipts)-1]
		zip, err := s.ReadControlPackage(r.ID, *scheduleRef(receipt.PackageRef))
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(directory, name+".zip"), zip, 0600); err != nil {
			t.Fatal(err)
		}
	}
}
