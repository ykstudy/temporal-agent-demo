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
	"time"

	"agentstudio/internal/contracts"
)

func runtimeFixture(t *testing.T, s *Service, id string) Run {
	t.Helper()
	var def contracts.AgentDefinition
	defs, e := s.Definitions.List()
	if e != nil {
		t.Fatal(e)
	}
	for _, d := range defs {
		if d.DefinitionVersion == "3.3.0" {
			def = d
		}
	}
	r, e := s.CreateWithOptions(context.Background(), CreateOptions{ID: id, Title: id, Content: "创建 runtime-service", ExecutionMode: s.Config.Mode, DecisionMode: "rules", DeliveryMode: "local", DefinitionID: def.DefinitionID, DefinitionVersion: def.DefinitionVersion, DefinitionHash: def.DefinitionHash})
	if e != nil {
		t.Fatal(e)
	}
	return r
}
func TestRuntimeAuditMetricsReadinessAndCorruption(t *testing.T) {
	s := testService(t)
	r := runtimeFixture(t, s, "runtime_metrics")
	checkHealth := func(ready bool) {
		t.Helper()
		h, e := s.RuntimeHealth(context.Background(), true)
		if e != nil || h.Ready != ready {
			t.Fatalf("readiness want %v got %+v %v", ready, h, e)
		}
	}
	checkHealth(true)
	live, e := s.RuntimeHealth(context.Background(), false)
	if e != nil || !live.Live || live.Ready {
		t.Fatal("liveness claimed dependencies", e)
	}
	before := r.StateSequence
	a, e := s.RuntimeAudit(r.ID)
	if e != nil {
		t.Fatal(e)
	}
	if a.Coverage != "complete" || len(a.Events) < 6 {
		t.Fatal("missing audit events")
	}
	for _, event := range a.Events {
		if strings.Contains(event.Parameters.Action, "runtime-service") {
			t.Fatal("user text leaked into parameter summary")
		}
	}
	v, e := s.RuntimeView(context.Background(), r.ID)
	if e != nil {
		t.Fatal(e)
	}
	if v.Checkpoint.Availability != "not_applicable" || *v.Metrics.EffectiveBusinessRounds != 1 || *v.Metrics.SuccessfulRuns != 0 || len(v.ActivityAttempts) != 1 {
		t.Fatalf("incorrect observation %+v", v)
	}
	if got, _ := s.Store.Get(r.ID); got.StateSequence != before {
		t.Fatal("read-only observation mutated Run")
	}
	cmd := Command{ID: "status_only", RunID: r.ID, Content: "进度"}
	if _, e = s.Submit(context.Background(), cmd); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Submit(context.Background(), cmd); e != nil {
		t.Fatal(e)
	}
	if _, e = s.ProcessCommand(context.Background(), Command{ID: "pause_rt", RunID: r.ID, Action: "pause"}); e != nil {
		t.Fatal(e)
	}
	v, _ = s.RuntimeView(context.Background(), r.ID)
	if *v.Metrics.EffectiveBusinessRounds != 1 || *v.Metrics.FailedRuns != 0 || *v.Metrics.ActivityRetries != 0 {
		t.Fatal("controls counted as rounds/retries/failures")
	}
	if _, e = s.ProcessCommand(context.Background(), Command{ID: "cancel_rt", RunID: r.ID, Action: "cancel"}); e != nil {
		t.Fatal(e)
	}
	v, e = s.RuntimeView(context.Background(), r.ID)
	if e != nil || v.Metrics.FirstTerminalState == nil || *v.Metrics.FirstTerminalState != "CANCELLED" || *v.Metrics.FailedRuns != 0 || v.Diagnosis.Category != "user_cancelled" {
		t.Fatal("cancellation lost separate semantics", e)
	}
	elapsed := *v.Metrics.TotalElapsedMS
	time.Sleep(time.Millisecond * 10)
	v, _ = s.RuntimeView(context.Background(), r.ID)
	if *v.Metrics.TotalElapsedMS != elapsed {
		t.Fatal("terminal elapsed continued counting")
	}
	// A real metadata permission fault must make readiness false without new Runs.
	runs := filepath.Join(s.Config.DataDir, "runs")
	if e = os.Chmod(runs, 0500); e != nil {
		t.Fatal(e)
	}
	checkHealth(false)
	_ = os.Chmod(runs, 0700)
	checkHealth(true)
	current, _ := s.Store.Get(r.ID)
	path := filepath.Join(runs, r.ID+".json")
	original, _ := os.ReadFile(path)
	if e = os.WriteFile(path, []byte("{}"), 0600); e != nil {
		t.Fatal(e)
	}
	checkHealth(false)
	_ = os.WriteFile(path, original, 0600)
	checkHealth(true)
	cloneRun := clone(current)
	runtimeState(cloneRun).Events[1].Outcome = "forged"
	if validateRuntimeState(cloneRun) == nil {
		t.Fatal("audit tampering accepted")
	}
	// Counts use actual committed terminal verdicts; this is an explicit unit
	// transition, not evidence of real generated delivery.
	success := runtimeFixture(t, s, "runtime_success_metric")
	failed := runtimeFixture(t, s, "runtime_failed_metric")
	for _, x := range []struct{ id, state string }{{success.ID, "DONE"}, {failed.ID, "FAILED"}} {
		if _, e = s.Store.Update(x.id, func(r *Run) error { r.State = x.state; return nil }); e != nil {
			t.Log("completion gate correctly disallowed synthetic DONE", e)
			if x.state == "FAILED" {
				t.Fatal(e)
			}
		}
	}
	m, e := s.RuntimeMetrics()
	if e != nil || m.FailedRuns != 1 {
		t.Fatalf("metrics failed %v %+v", e, m)
	}
	s.BeginShutdown()
	checkHealth(false)
	if _, e = s.Submit(context.Background(), Command{ID: "stopping", RunID: r.ID, Content: "进度"}); e == nil {
		t.Fatal("new work admitted while stopping")
	}
}
func TestRuntimePublicSchemaNegativeAndOlderCoverage(t *testing.T) {
	s := testService(t)
	r := runtimeFixture(t, s, "runtime_http")
	mux := http.NewServeMux()
	s.Register(mux)
	for _, path := range []string{"/api/runtime/live", "/api/runtime/ready", "/api/runtime/metrics", "/api/v2/runs/" + r.ID + "/runtime", "/api/v2/runs/" + r.ID + "/audit"} {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
		if rec.Code != 200 {
			t.Fatalf("%s: %d %s", path, rec.Code, rec.Body.String())
		}
	}
	a, _ := s.RuntimeAudit(r.ID)
	raw, _ := json.Marshal(a)
	var object map[string]any
	_ = json.Unmarshal(raw, &object)
	for _, mutate := range []func(map[string]any){func(v map[string]any) { delete(v, "coverage") }, func(v map[string]any) { v["extra"] = true }, func(v map[string]any) { v["coverage"] = "partial" }, func(v map[string]any) { v["observed_at"] = "yesterday" }, func(v map[string]any) { v["run_ref"].(map[string]any)["object_version"] = true }} {
		var v map[string]any
		_ = json.Unmarshal(raw, &v)
		mutate(v)
		if contracts.ValidateRuntime("RuntimeAuditTrail", v) == nil {
			t.Fatal("malformed runtime payload accepted")
		}
	}
	// The pre-publication fixture removes the new private field on an offline
	// copy; reads must not reconstruct missing history from business objects.
	cfg := s.Config
	s.Close()
	path := filepath.Join(cfg.DataDir, "runs", r.ID+".json")
	raw, _ = os.ReadFile(path)
	var old map[string]any
	_ = json.Unmarshal(raw, &old)
	control := old["canonical_control"].(map[string]any)
	conv := control["conversation"].(map[string]any)
	delete(conv, "runtime")
	raw, _ = json.MarshalIndent(old, "", "  ")
	if e := os.WriteFile(path, raw, 0600); e != nil {
		t.Fatal(e)
	}
	restarted, e := New(cfg)
	if e != nil {
		t.Fatal(e)
	}
	defer restarted.Close()
	v, e := restarted.RuntimeView(context.Background(), r.ID)
	if e != nil || v.AuditCoverage != "unavailable" || v.Metrics.FailedRuns != nil {
		t.Fatal("old history fabricated", e)
	}
}
func TestRuntimePendingAttemptRecoveryPreservesIdentity(t *testing.T) {
	s := testService(t)
	r := runtimeFixture(t, s, "runtime_recovery")
	_, err := s.Store.Update(r.ID, func(run *Run) error {
		a := contracts.RuntimeActivityAttempt{AttemptID: "interrupted_attempt", RunID: r.ID, Mode: "local", ActivityID: "actual_local_dispatch", ActivityType: "ExecuteStep", Step: "verify", Attempt: 1, StartedAt: now(), Status: "running", HeartbeatSource: "process", HeartbeatStatus: "unavailable"}
		runtimeState(*run).Attempts = append(runtimeState(*run).Attempts, a)
		return runtimeEvent(run, contracts.RuntimeAuditEvent{Kind: "activity_started", Step: "verify", Outcome: "running", Correlation: contracts.RuntimeCorrelation{ActivityID: &a.ActivityID, AttemptID: &a.AttemptID, Attempt: &a.Attempt}})
	})
	if err != nil {
		t.Fatal(err)
	}
	a, _ := s.RuntimeAudit(r.ID)
	cfg := s.Config
	s.Close()
	restarted, e := New(cfg)
	if e != nil {
		t.Fatal(e)
	}
	defer restarted.Close()
	after, e := restarted.RuntimeAudit(r.ID)
	if e != nil {
		t.Fatal(e)
	}
	for i, event := range a.Events {
		if event.EventHash != after.Events[i].EventHash {
			t.Fatal("recovery rewrote existing audit")
		}
	}
	v, e := restarted.RuntimeView(context.Background(), r.ID)
	if e != nil {
		t.Fatal(e)
	}
	last := v.ActivityAttempts[len(v.ActivityAttempts)-1]
	if last.AttemptID != "interrupted_attempt" || last.Status != "unknown" {
		t.Fatal("pending attempt fabricated or lost")
	}
	if *v.Metrics.FailedRuns != 0 {
		t.Fatal("unknown attempt counted as failed Run")
	}
}

func TestRuntimeActualToolFailureReportsAndBudgetExhaustion(t *testing.T) {
	s, r, x := recoveryFailedVerificationVersion(t, "1.4.0")
	for attempt := 1; attempt <= 3; attempt++ {
		if attempt > 1 {
			var e error
			r, e = s.RetryControlVerification(context.Background(), r.ID, x)
			if e != nil {
				t.Fatal(e)
			}
			next, e := s.PrepareControlAction(context.Background(), r.ID)
			if e != nil || next.Execution == nil || !equalContractValue(*next.Execution, x) {
				t.Fatal("retry changed original execution", e)
			}
			if _, e = s.ExecuteControlVerification(context.Background(), x); e == nil {
				t.Fatal("deliberate tool failure accepted")
			}
		}
		v, e := s.RuntimeView(context.Background(), r.ID)
		if e != nil {
			t.Fatal(e)
		}
		if *v.Metrics.VerificationFailures != int64(attempt) || v.Diagnosis == nil || v.Diagnosis.Category != "business_verification" {
			t.Fatalf("failure report metrics lost: %+v", v.Metrics)
		}
	}
	r, e := s.RetryControlVerification(context.Background(), r.ID, x)
	if e == nil || r.State != "FAILED" {
		t.Fatal("budget did not terminate")
	}
	v, e := s.RuntimeView(context.Background(), r.ID)
	if e != nil || *v.Metrics.FailedRuns != 1 || *v.Metrics.VerificationFailures != 3 {
		t.Fatal("actual failure metrics incorrect", e)
	}
}
