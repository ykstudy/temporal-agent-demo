package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"agentstudio/internal/contracts"
)

// A trusted test-only proposal adds a deliberately failing test before P1.
// Thereafter the real controlled scaffold, sandboxed tools, settlement and
// retry entry points run unchanged. This is not a model repair test.
func recoveryFailedVerification(t *testing.T) (*Service, Run, contracts.ExecutionRequest) {
	return recoveryFailedVerificationVersion(t, "1.2.0")
}

func recoveryFailedVerificationVersion(t *testing.T, version string) (*Service, Run, contracts.ExecutionRequest) {
	t.Helper()
	s := testService(t)
	r := controlFixture(t, s, "verify_recovery")
	if version == "1.3.0" {
		r = newControlCompletionRun(t, s, "rules")
	}
	if version == "1.4.0" {
		r = runtimeFixture(t, s, "runtime_actual_failure")
	}

	result, err := s.ProcessCommand(context.Background(), Command{ID: "recovery_input", RunID: r.ID, Content: "创建 retry-service"})
	if err != nil {
		t.Fatal(err)
	}
	r, err = s.Store.Update(r.ID, func(run *Run) error {
		files, e := s.renderFrozenFiles(*run)
		if e != nil {
			return e
		}
		files["failure_test.go"] = "package main\nimport \"testing\"\nfunc TestDeliberateFailure(t *testing.T){t.Fatal(\"deliberate retry fixture\")}\n"
		sourceChanged(run)
		return s.makeCanonicalProposal(run, *result.Run.Control.Audit.Plan, files, time.Now().UTC())
	})
	if err != nil {
		t.Fatal(err)
	}
	r = controlAcceptCommand(t, s, r)
	next, err := s.PrepareControlAction(context.Background(), r.ID)
	if err != nil {
		t.Fatal(err)
	}
	r, _, err = s.ExecuteControlWorkspace(context.Background(), r.ID, *next.Execution)
	if err != nil {
		t.Fatal(err)
	}
	next, err = s.PrepareControlAction(context.Background(), r.ID)
	if err != nil {
		t.Fatal(err)
	}
	x := *next.Execution
	tools, err := s.ExecuteControlVerification(context.Background(), x)
	if err == nil || tools.ReapprovalRequired || len(tools.Receipts) != 2 || tools.Receipts[1].Status != "failed" {
		t.Fatalf("expected actual test failure: %v %+v", err, tools)
	}
	r, err = s.Store.Get(r.ID)
	if err != nil {
		t.Fatal(err)
	}
	return s, r, x
}

func TestControlRecoveryRealFailureReusesOriginalRequestAndExhaustsBudget(t *testing.T) {
	s, r, x := recoveryFailedVerification(t)
	source, approval, deadline := r.SourceVersion, r.Control.Audit.Approval.ApprovalHash, *r.Control.Audit.Budget.ExecutionDeadlineAt
	for attempt := 2; attempt <= 3; attempt++ {
		var err error
		r, err = s.RetryControlVerification(context.Background(), r.ID, x)
		if err != nil || r.State != "IMPLEMENT" || r.SourceVersion != source || r.Control.Audit.Approval.ApprovalHash != approval || *r.Control.Audit.Budget.ExecutionDeadlineAt != deadline {
			t.Fatalf("retry %d changed authority/deadline: %v %s", attempt, err, r.State)
		}
		next, err := s.PrepareControlAction(context.Background(), r.ID)
		if err != nil || next.Execution == nil || !equalContractValue(*next.Execution, x) {
			t.Fatalf("retry identity changed: %v %+v", err, next)
		}
		r, _ = s.Store.Get(r.ID)
		if r.State != "VERIFY" {
			t.Fatal("retry did not reenter actual verification stage")
		}
		if _, err = s.ExecuteControlVerification(context.Background(), x); err == nil {
			t.Fatal("deliberately failing source succeeded")
		}
	}
	r, err := s.RetryControlVerification(context.Background(), r.ID, x)
	if err == nil || r.State != "FAILED" || len(r.Control.Audit.Executions) != 2 || r.Control.Audit.Budget.Totals.Consumed.Action != 4 {
		t.Fatalf("budget not terminal or action replay lost identity: %v %s %+v", err, r.State, r.Control.Audit.Budget.Totals)
	}
	if r.Control.Audit.Budget.Totals.Consumed.Tool != 6 || r.Control.Audit.Budget.Totals.Reserved.Action != 0 || len(r.Control.Audit.ToolReceipts) != 6 {
		t.Fatal("failed attempts were dropped or duplicated")
	}
	for _, d := range r.Control.Audit.Decisions {
		if d.Advice.Kind == "diagnosis" && d.DecisionMode != "rules" {
			t.Fatal("diagnosis source misrepresented")
		}
	}
	if _, err = s.RetryControlVerification(context.Background(), r.ID, x); err == nil {
		t.Fatal("terminal run resumed")
	}
}

func TestControlRecoveryRejectsMissingUnknownChangedAndTamperedEvidence(t *testing.T) {
	s, r, x := recoveryFailedVerification(t)
	if _, err := controlRetryEvidence(r, x); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*Run){
		func(v *Run) { v.Control.Audit.ToolReceipts = nil },
		func(v *Run) { v.Control.Audit.Operations[len(v.Control.Audit.Operations)-1].Status = "unknown" },
		func(v *Run) { v.Control.Audit.ToolReceipts[0].OutputManifestRef = nil },
		func(v *Run) { v.Control.Audit.ToolReceipts[0].Output = "altered" },
		func(v *Run) { v.SourceVersion++ },
		func(v *Run) { v.Control.Audit.Budget.Settlements = nil },
	} {
		bad := clone(r)
		mutate(&bad)
		if _, err := controlRetryEvidence(bad, x); err == nil {
			t.Fatal("unsafe evidence authorized retry")
		}
	}
	if err := os.WriteFile(filepath.Join(r.Workspace, "failure_test.go"), []byte("package main\n"), 0600); err != nil {
		t.Fatal(err)
	}
	before := r.Control.Audit.Budget.LedgerVersion
	if _, err := s.RetryControlVerification(context.Background(), r.ID, x); err == nil {
		t.Fatal("changed approved files permitted retry")
	}
	after, _ := s.Store.Get(r.ID)
	if after.Control.Audit.Budget.LedgerVersion != before || after.State != "BLOCKED" {
		t.Fatal("unsafe retry changed authority or charged unstarted tool")
	}
}

func TestControlSemanticAdviceCallsActualModelAndCannotChangeState(t *testing.T) {
	s := testService(t)
	var calls atomic.Int64
	var unavailable atomic.Bool
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		calls.Add(1)
		if unavailable.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		advice := contracts.DecisionAdvice{Kind: "diagnosis", Reason: "actual provider advice", Diagnosis: &contracts.Diagnosis{Category: "business", Code: "test_failure", Message: "建议修复测试", EvidenceRefs: []contracts.ObjectReference{}, SuggestedRemedy: "重新预览并批准"}}
		body, _ := json.Marshal(advice)
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": string(body)}, "finish_reason": "stop"}}})
	}))
	defer provider.Close()
	s.Config.AIEndpoint, s.Config.AIKey, s.Config.AIModel = provider.URL, "semantic_fixture_key", "semantic-model"
	r := controlFixture(t, s, "semantic_model", "model")
	r, err := s.Store.Update(r.ID, func(run *Run) error {
		_, e := appendCanonicalInput(run, Command{ID: "semantic_input", RunID: run.ID, Content: "请诊断验证失败"}, time.Now().UTC())
		run.State = "BLOCKED"
		return e
	})
	if err != nil {
		t.Fatal(err)
	}
	rules := contracts.DecisionAdvice{Kind: "diagnosis", Reason: "rules-only template", Diagnosis: &contracts.Diagnosis{Category: "business", Code: "rules_only", Message: "规则模板", EvidenceRefs: []contracts.ObjectReference{}, SuggestedRemedy: "规则模板"}}
	validation, err := s.prepareControlSemanticAdvice(context.Background(), r.ID, "diagnosis", rules, map[string]string{"failure": "known"}, nil)
	if err != nil || validation.ExecutionRequest != nil || validation.Verdict != "allowed" || calls.Load() != 1 {
		t.Fatal("actual model diagnosis failed", err, validation)
	}
	after, _ := s.Store.Get(r.ID)
	if after.State != "BLOCKED" || after.SourceVersion != r.SourceVersion || after.Control.Audit.Decisions[0].Advice.Diagnosis.Code != "test_failure" || after.Control.Audit.Decisions[0].ModelUsage.Status != "unknown" {
		t.Fatal("model advice changed authority or invented usage")
	}
	unavailable.Store(true)
	if _, err = s.prepareControlSemanticAdvice(context.Background(), r.ID, "diagnosis", rules, nil, nil); err == nil {
		t.Fatal("provider failure silently became rules")
	}
	after, _ = s.Store.Get(r.ID)
	if after.Control.Audit.Budget.Totals.Consumed.Model != 2 || len(after.Control.Audit.Decisions) != 1 || after.State != "BLOCKED" {
		t.Fatal("failed model attempt was not accurately recorded")
	}
}
