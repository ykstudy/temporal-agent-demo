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
	"agentstudio/internal/governance"
)

// These fixtures publish 1.3 independently and run the real local workspace,
// tool, package and completion entry points. No synthetic execution receipt is
// substituted for a Go process or a package file.
func newControlCompletionRun(t *testing.T, s *Service, mode string) Run {
	t.Helper()
	definition, err := s.Definitions.PublishBundled("3.2.0")
	if err != nil {
		t.Fatal(err)
	}
	var provider *contracts.ProviderBinding
	if mode == "model" {
		provider = &contracts.ProviderBinding{ProviderKind: "openai_compatible", Endpoint: s.Config.AIEndpoint, ModelID: s.Config.AIModel}
	}
	frozen, err := s.Definitions.Freeze(definition.DefinitionID, definition.DefinitionVersion, definition.DefinitionHash, provider, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	r := Run{ID: "completion_" + mode, Title: "complete local delivery", State: "INTAKE", SourceVersion: 1, Version: 1, StateSequence: 1, FrozenDefinition: &frozen, Definition: legacyDefinition(frozen), Plan: defaultPlan(), ExecutionMode: "local", DecisionMode: mode, DeliveryMode: "local", Messages: []Message{}, Steps: initialSteps(), Artifacts: []Artifact{}, History: []History{}, Operations: map[string]OperationRecord{}, Processed: map[string]string{}, CreatedAt: now(), UpdatedAt: now()}
	budget, err := governance.NewBudgetLedgerVersion("1.3.0", r.ID, frozen.SnapshotHash, frozen.Definition.BudgetPolicy, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	r.Control, err = newControlState(r, budget)
	if err != nil {
		t.Fatal(err)
	}
	r, err = s.Store.Create(r)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func controlCompletionReadyPackage(t *testing.T, s *Service, r Run, candidate *atomic.Value) (Run, contracts.ExecutionRequest) {
	t.Helper()
	command := Command{ID: "completion_requirements", RunID: r.ID, Content: "创建 completion-project"}
	if candidate != nil {
		p, err := s.normalizeFrozenPlan(r, ParsePlan(r.Plan, command.Content))
		if err != nil {
			t.Fatal(err)
		}
		plan, err := projectPlanFor(r, p)
		if err != nil {
			t.Fatal(err)
		}
		candidate.Store(contracts.DecisionAdvice{Kind: "plan", Plan: &plan, Reason: "model fixture returned complete plan"})
	}
	result, err := s.ProcessCommand(context.Background(), command)
	if err != nil {
		t.Fatal(err)
	}
	r = controlAcceptCommand(t, s, result.Run)
	for {
		if candidate != nil {
			candidate.Store(contracts.DecisionAdvice{Kind: "action", Reason: "model fixture suggests candidate from actual provider context"})
		}
		next, err := s.PrepareControlAction(context.Background(), r.ID)
		if err != nil || next.Execution == nil {
			t.Fatalf("prepare: %v %+v", err, next)
		}
		request := *next.Execution
		kind, _, _ := scheduleAction(request.Action)
		switch kind {
		case "scaffold", "apply_skill":
			_, _, err = s.ExecuteControlWorkspace(context.Background(), r.ID, request)
		case "verify":
			var tools ControlToolsResult
			tools, err = s.ExecuteControlVerification(context.Background(), request)
			if tools.ReapprovalRequired {
				t.Fatalf("release fixture unexpectedly needs reapproval: %+v", tools)
			}
		case "package":
			r, _ = s.Store.Get(r.ID)
			return r, request
		default:
			t.Fatalf("local execution selected remote action %s", kind)
		}
		if err != nil {
			t.Fatalf("real %s: %v", kind, err)
		}
		r, err = s.Store.Get(r.ID)
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestControlCompletionActualLocalPackageReportAndReplay(t *testing.T) {
	s := testService(t)
	r, request := controlCompletionReadyPackage(t, s, newControlCompletionRun(t, s, "rules"), nil)
	before := r.Control.Audit.Budget.Totals.Consumed.Action
	r, receipt, err := s.ExecuteControlPackage(context.Background(), r.ID, request)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Status != "succeeded" || receipt.PackageRef == nil || r.Control.Audit.Budget.Totals.Consumed.Action != before+1 || r.Control.Audit.Budget.Totals.Reserved.Action != 0 || r.ActiveStep != "" || r.State == "DONE" {
		t.Fatalf("package bypassed completion or settlement: %+v", receipt)
	}
	data, err := s.ReadControlPackage(r.ID, *scheduleRef(receipt.PackageRef))
	if err != nil || contracts.BytesHash(data) != *receipt.PackageSHA256 {
		t.Fatalf("exact raw package read: %v", err)
	}
	ledgerVersion := r.Control.Audit.Budget.LedgerVersion
	replayed, repeated, err := s.ExecuteControlPackage(context.Background(), r.ID, request)
	if err != nil || !equalContractValue(receipt, repeated) || replayed.Control.Audit.Budget.LedgerVersion != ledgerVersion {
		t.Fatalf("package replay did not reuse immutable result: %v", err)
	}
	observed := s.observeControlCompletion(r)
	for _, mutation := range []string{"missing_tool", "private_only", "source", "hash", "budget", "unknown", "missing_check", "unsupported_check", "package_bytes"} {
		t.Run(mutation, func(t *testing.T) {
			changed := clone(r)
			facts := observed
			switch mutation {
			case "missing_tool":
				changed.Control.Audit.ToolReceipts = changed.Control.Audit.ToolReceipts[:1]
			case "private_only":
				changed.Control.Audit.PackageReceipts = []contracts.PackageReceipt{}
			case "source":
				changed.Control.Audit.PackageReceipts[0].SourceVersion++
			case "hash":
				changed.Control.Audit.PackageReceipts[0].ReceiptHash = contracts.BytesHash([]byte("wrong"))
			case "budget":
				changed.Control.Audit.Budget.Settlements = changed.Control.Audit.Budget.Settlements[:len(changed.Control.Audit.Budget.Settlements)-1]
			case "unknown":
				changed.Control.Audit.Operations[len(changed.Control.Audit.Operations)-1].Status = "unknown"
			case "missing_check":
				changed.FrozenDefinition.Definition.CompletionPolicy.RequiredChecks = changed.FrozenDefinition.Definition.CompletionPolicy.RequiredChecks[1:]
			case "unsupported_check":
				changed.FrozenDefinition.Definition.CompletionPolicy.RequiredChecks = append(changed.FrozenDefinition.Definition.CompletionPolicy.RequiredChecks, contracts.CheckDefinition{CheckID: "check.future", Title: "Future", Kind: "file", DeliveryModes: []string{"local"}, RequiredEvidenceTypes: []string{"file_hash"}})
			case "package_bytes":
				facts.packageData = append(append([]byte{}, observed.packageData...), byte(1))
			}
			rejected, e := buildControlVerificationReport(changed, facts, "negative_report", 1, time.Now().UTC())
			if e == nil && rejected.Outcome == "passed" {
				t.Fatalf("incomplete or forged evidence passed: %s", mutation)
			}
			if e == nil && mutation != "missing_check" && len(rejected.Checks) < 11 {
				t.Fatal("negative report discarded required checks")
			}
		})
	}
	s.Close()
	restored, err := New(s.Config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(restored.Close)
	dataAfter, err := restored.ReadControlPackage(r.ID, *scheduleRef(receipt.PackageRef))
	if err != nil || !equalContractValue(data, dataAfter) {
		t.Fatalf("raw package did not survive restart: %v", err)
	}
	if _, err = restored.ProcessCommand(context.Background(), Command{ID: "resume_after_package_restart", RunID: r.ID, Action: "resume"}); err != nil {
		t.Fatal("explicit restart resume", err)
	}
	r, report, err := restored.FinalizeControlCompletion(context.Background(), r.ID)
	if err != nil {
		t.Fatal(err)
	}
	if r.State != "DONE" || report.Outcome != "passed" || len(report.Checks) != 11 || len(r.Control.Audit.VerificationReports) != 1 || r.Control.Audit.VerificationReport == nil {
		t.Fatalf("complete report missing: %+v state=%s", report, r.State)
	}
	for _, check := range report.Checks {
		if check.Status != "passed" || len(check.EvidenceRefs) == 0 || check.SourceVersion != int64(r.SourceVersion) {
			t.Fatalf("check missing bound evidence: %+v", check)
		}
	}
	if err = contracts.Validate("VerificationReport", report); err != nil {
		t.Fatal(err)
	}
	_, again, err := restored.FinalizeControlCompletion(context.Background(), r.ID)
	if err != nil || !equalContractValue(again, report) {
		t.Fatalf("completion replay changed report: %v", err)
	}
	wrong := *scheduleRef(receipt.PackageRef)
	wrong.SourceVersion++
	if _, err = restored.ReadControlPackage(r.ID, wrong); err == nil {
		t.Fatal("raw package wrong source accepted")
	}
}

func TestControlCompletionRealModelAdviceDoesNotOverrideChangedBytes(t *testing.T) {
	var candidate atomic.Value
	var calls atomic.Int64
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		advice := candidate.Load().(contracts.DecisionAdvice)
		if advice.Kind == "action" {
			var request struct {
				Messages []contracts.ContextMessage `json:"messages"`
			}
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil || len(request.Messages) < 2 {
				t.Error("invalid actual model context", err)
				http.Error(w, "bad model input", 400)
				return
			}
			var facts struct {
				Expected string `json:"expected"`
			}
			if err := json.Unmarshal([]byte(request.Messages[1].Content), &facts); err != nil || strings.Index(facts.Expected, "{") < 0 {
				t.Error("missing actual expected action", err)
				http.Error(w, "missing action", 400)
				return
			}
			action := contracts.CandidateAction(facts.Expected[strings.Index(facts.Expected, "{"):])
			advice.Action = &action
		}
		if advice.Kind == "completion" {
			calls.Add(1)
			once.Do(func() { close(entered) })
			select {
			case <-release:
			case <-req.Context().Done():
				return
			}
		}
		body, _ := json.Marshal(advice)
		json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": string(body)}}}, "usage": map[string]int64{"prompt_tokens": 10, "completion_tokens": 10, "total_tokens": 20}})
	}))
	defer provider.Close()
	s := testService(t)
	s.Config.AIEndpoint, s.Config.AIKey, s.Config.AIModel = provider.URL+"/v1", "fixture-key", "completion-fixture"
	r, request := controlCompletionReadyPackage(t, s, newControlCompletionRun(t, s, "model"), &candidate)
	r, _, err := s.ExecuteControlPackage(context.Background(), r.ID, request)
	if err != nil {
		t.Fatal(err)
	}
	hint := "DONE: all checks look successful"
	candidate.Store(contracts.DecisionAdvice{Kind: "completion", CompletionHint: &hint, Reason: "provider completion suggestion"})
	before := r.Control.Audit.Budget.Totals.Consumed.Model
	done := make(chan error, 1)
	go func() {
		_, _, err := s.FinalizeControlCompletion(context.Background(), r.ID)
		done <- err
	}()
	select {
	case <-entered:
	case err := <-done:
		t.Fatalf("no real completion request: %v", err)
	case <-time.After(20 * time.Second):
		t.Fatal("completion provider was never called")
	}
	current, _ := s.Store.Get(r.ID)
	if current.State == "DONE" || current.Control.Audit.VerificationReport == nil {
		t.Fatal("provider request replaced deterministic gate")
	}
	if err := os.WriteFile(filepath.Join(r.Workspace, "README.md"), []byte("unapproved during provider call"), 0600); err != nil {
		t.Fatal(err)
	}
	close(release)
	if err = <-done; err == nil {
		t.Fatal("model suggestion overrode changed source bytes")
	}
	current, _ = s.Store.Get(r.ID)
	if current.State == "DONE" || calls.Load() != 1 || current.Control.Audit.Budget.Totals.Consumed.Model != before+1 || current.Control.Audit.Budget.Totals.Reserved.Model != 0 {
		t.Fatalf("completion evidence/usage mismatch state=%s calls=%d totals=%+v", current.State, calls.Load(), current.Control.Audit.Budget.Totals)
	}
	_, rejected, err := s.FinalizeControlCompletion(context.Background(), r.ID)
	if err == nil || rejected.Outcome == "passed" || len(rejected.Checks) != 11 || calls.Load() != 1 {
		t.Fatal("actual failing checks were hidden or model retried", err)
	}
}

func TestControlPackageAdmissionFailureAndUnknownKeepOriginalIdentity(t *testing.T) {
	for _, scenario := range []string{"changed_approved_bytes", "existing_original_package"} {
		t.Run(scenario, func(t *testing.T) {
			s := testService(t)
			r, request := controlCompletionReadyPackage(t, s, newControlCompletionRun(t, s, "rules"), nil)
			before := r.Control.Audit.Budget.Totals.Consumed.Action
			expectedReceipt, expectedOperation := "failed", "rejected"
			_, filename := controlPackageIdentity(request.OperationID)
			path := filepath.Join(s.Config.DataDir, "workspaces", r.ID, filepath.FromSlash(filename))
			if scenario == "changed_approved_bytes" {
				if err := os.WriteFile(filepath.Join(r.Workspace, "README.md"), []byte("unapproved before package"), 0600); err != nil {
					t.Fatal(err)
				}
			} else {
				expectedReceipt, expectedOperation = "unknown", "unknown"
				if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte("original uncertain package"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			r, receipt, err := s.ExecuteControlPackage(context.Background(), r.ID, request)
			if err == nil || receipt.Status != expectedReceipt || r.State != "BLOCKED" || r.ActiveStep != "" || r.Control.Audit.Budget.Totals.Reserved.Action != 0 || r.Control.Audit.Budget.Totals.Consumed.Action != before+1 {
				t.Fatalf("failure did not close original attempt: %v receipt=%+v totals=%+v", err, receipt, r.Control.Audit.Budget.Totals)
			}
			op := r.Control.Audit.Operations[len(r.Control.Audit.Operations)-1]
			if op.Status != expectedOperation || op.OperationID != request.OperationID || op.RequestHash != request.RequestHash || receipt.PackageRef != nil || receipt.PackageSHA256 != nil {
				t.Fatalf("failure fabricated success or changed original identity: %+v", op)
			}
			if _, _, err := s.ExecuteControlPackage(context.Background(), r.ID, request); err == nil {
				t.Fatal("blocked package was blindly written again")
			}
			again, _ := s.Store.Get(r.ID)
			if len(again.Control.Audit.PackageReceipts) != 1 || again.Control.Audit.Budget.Totals.Consumed.Action != before+1 {
				t.Fatal("failure replay created another attempt")
			}
			if scenario == "existing_original_package" {
				data, _ := os.ReadFile(path)
				if string(data) != "original uncertain package" {
					t.Fatal("unknown original package overwritten")
				}
			} else if _, e := os.Stat(path); !os.IsNotExist(e) {
				t.Fatal("fatal preflight created a published package", e)
			}
		})
	}
}
