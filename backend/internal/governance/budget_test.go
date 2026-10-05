package governance

import (
	"encoding/json"
	"testing"
	"time"

	"agentstudio/internal/contracts"
)

func budgetFixture(t *testing.T) (contracts.BudgetPolicy, contracts.BudgetLedger, time.Time) {
	t.Helper()
	policy := contracts.BudgetPolicy{MaxCommands: 2, MaxModelAttempts: 2, MaxFiles: 128, MaxArtifactBytes: 2 << 20, MaxStepAttempts: 2, MaxToolSeconds: 3, MaxExecutionSeconds: 10, MaxCIWaitSeconds: 20, MaxCIPollAttempts: 2, MaxReconciliationSeconds: 15, MaxReconciliationAttempts: 2}
	now := time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)
	ledger, err := NewBudgetLedger("run_budget", contracts.BytesHash([]byte("frozen definition")), policy, now)
	if err != nil {
		t.Fatal(err)
	}
	return policy, ledger, now
}

func TestBudgetAttemptsSettleExactlyOnceAndUnknownIsConsumed(t *testing.T) {
	policy, ledger, now := budgetFixture(t)
	in := ReservationInput{SourceVersion: 1, CommandID: "command_one", AttemptID: "attempt_one", Kind: "command", ScopeID: "command_one", Now: now}
	next, reservation, err := ReserveBudget(policy, ledger, in)
	if err != nil {
		t.Fatal(err)
	}
	if len(ledger.Reservations) != 0 || next.Totals.Reserved.Command != 1 {
		t.Fatal("reservation mutated input ledger or was not counted")
	}
	in.Now = now.Add(time.Second)
	replayed, replay, err := ReserveBudget(policy, next, in)
	if err != nil || !equalJSON(replayed, next) || replay.ReservationHash != reservation.ReservationHash {
		t.Fatal("attempt replay renewed its reservation", err)
	}
	conflict := in
	conflict.CommandID = "command_changed"
	conflict.ScopeID = conflict.CommandID
	if _, _, err = ReserveBudget(policy, next, conflict); err == nil {
		t.Fatal("same attempt accepted changed command")
	}
	elapsed := int64(10)
	settle := SettlementInput{ReservationID: reservation.ReservationID, AttemptID: reservation.AttemptID, Outcome: "failed", MeasurementSource: "actual", ElapsedMS: &elapsed, Now: now.Add(time.Second)}
	next, settlement, err := SettleBudget(policy, next, settle)
	if err != nil {
		t.Fatal(err)
	}
	if next.Totals.Consumed.Command != 1 || next.Totals.Reserved.Command != 0 || next.Totals.MeasuredElapsedMS != 10 {
		t.Fatal("failed attempt consumption is incorrect")
	}
	settle.Now = now.Add(2 * time.Second)
	replayed, repeatedSettlement, err := SettleBudget(policy, next, settle)
	if err != nil || !equalJSON(replayed, next) || repeatedSettlement.SettlementHash != settlement.SettlementHash {
		t.Fatal("settlement replay charged twice", err)
	}
	in.CommandID, in.ScopeID, in.AttemptID, in.Now = "command_two", "command_two", "attempt_two", now.Add(2*time.Second)
	next, reservation, err = ReserveBudget(policy, next, in)
	if err != nil {
		t.Fatal(err)
	}
	next, _, err = SettleBudget(policy, next, SettlementInput{ReservationID: reservation.ReservationID, AttemptID: reservation.AttemptID, Outcome: "unknown", MeasurementSource: "unknown", Now: now.Add(3 * time.Second)})
	if err != nil {
		t.Fatal(err)
	}
	if next.Totals.Consumed.Command != 2 || next.Totals.Unknown.Command != 1 || next.Totals.UnknownElapsedAttempts != 1 {
		t.Fatal("unknown is not a consumed subset")
	}
	in.CommandID, in.ScopeID, in.AttemptID, in.Now = "command_three", "command_three", "attempt_three", now.Add(4*time.Second)
	if _, _, err = ReserveBudget(policy, next, in); err == nil {
		t.Fatal("failed/unknown attempts did not exhaust the command allowance")
	}
	zero := int64(0)
	if _, _, err = SettleBudget(policy, next, SettlementInput{ReservationID: reservation.ReservationID, AttemptID: reservation.AttemptID, Outcome: "released", MeasurementSource: "actual", ElapsedMS: &zero, ConfirmedNotStarted: true, Now: now.Add(4 * time.Second)}); err == nil {
		t.Fatal("unknown consumption was rewritten as released")
	}
}

func TestBudgetModelFailureAndReleaseProof(t *testing.T) {
	policy, ledger, now := budgetFixture(t)
	in := ReservationInput{SourceVersion: 1, CommandID: "command_one", AttemptID: "model_one", Kind: "model", ScopeID: "decision:model", TimeoutSeconds: 2, Now: now}
	ledger, reservation, err := ReserveBudget(policy, ledger, in)
	if err != nil {
		t.Fatal(err)
	}
	if reservation.DeadlineAt == nil || *reservation.DeadlineAt != stamp(now.Add(2*time.Second)) {
		t.Fatal("pre-execution model call has no explicit bounded deadline")
	}
	zero := int64(0)
	settle := SettlementInput{ReservationID: reservation.ReservationID, AttemptID: reservation.AttemptID, Outcome: "released", MeasurementSource: "actual", ElapsedMS: &zero, Now: now.Add(time.Second)}
	if _, _, err = SettleBudget(policy, ledger, settle); err == nil {
		t.Fatal("reservation released without trusted not-started proof")
	}
	settle.ConfirmedNotStarted = true
	ledger, _, err = SettleBudget(policy, ledger, settle)
	if err != nil {
		t.Fatal(err)
	}
	if ledger.Totals.Released.Model != 1 || ledger.Totals.Consumed.Model != 0 {
		t.Fatal("release did not free unused reservation")
	}
	for i, attempt := range []string{"model_two", "model_three"} {
		in.AttemptID, in.SourceVersion, in.Now = attempt, int64(i+2), now.Add(time.Duration(i+2)*time.Second)
		ledger, reservation, err = ReserveBudget(policy, ledger, in)
		if err != nil {
			t.Fatal(err)
		}
		ledger, _, err = SettleBudget(policy, ledger, SettlementInput{ReservationID: reservation.ReservationID, AttemptID: reservation.AttemptID, Outcome: "failed", MeasurementSource: "unknown", Now: in.Now})
		if err != nil {
			t.Fatal(err)
		}
	}
	in.AttemptID, in.SourceVersion, in.Now = "model_four", 4, now.Add(5*time.Second)
	if _, _, err = ReserveBudget(policy, ledger, in); err == nil {
		t.Fatal("new source reset failed model attempts")
	}
}

func executionBudgetFixture(t *testing.T, kind string) (contracts.BudgetPolicy, contracts.BudgetLedger, contracts.ExecutionRequest, time.Time) {
	t.Helper()
	v, in := fixture(t)
	setAction(t, &in, kind)
	result, err := v.Validate(in)
	if err != nil || result.ExecutionRequest == nil {
		t.Fatal(result, err)
	}
	policy, _, now := budgetFixture(t)
	request := *result.ExecutionRequest
	ledger, err := NewBudgetLedger(request.RunID, request.DefinitionSnapshotHash, policy, now)
	if err != nil {
		t.Fatal(err)
	}
	ledger, err = StartExecution(policy, ledger, now)
	if err != nil {
		t.Fatal(err)
	}
	return policy, ledger, request, now
}

func TestBudgetExecutionScopesAndDeadlineSurviveSourceAndRestart(t *testing.T) {
	policy, ledger, request, now := executionBudgetFixture(t, "scaffold")
	for i := 0; i < 2; i++ {
		request.SourceVersion = int64(i + 2)
		request.Approval.SourceVersion = request.SourceVersion
		request.RequestHash = objectHash(t, "ExecutionRequest", request)
		in := ReservationInput{SourceVersion: request.SourceVersion, CommandID: request.CommandID, AttemptID: []string{"action_one", "action_two"}[i], Kind: "action", ScopeID: "scaffold", Request: &request, Now: now.Add(time.Duration(i) * time.Second)}
		var reservation contracts.BudgetReservation
		var err error
		ledger, reservation, err = ReserveBudget(policy, ledger, in)
		if err != nil {
			t.Fatal(err)
		}
		ledger, _, err = SettleBudget(policy, ledger, SettlementInput{ReservationID: reservation.ReservationID, AttemptID: reservation.AttemptID, Outcome: "interrupted", MeasurementSource: "unknown", Now: in.Now})
		if err != nil {
			t.Fatal(err)
		}
	}
	var persisted contracts.BudgetLedger
	if err := json.Unmarshal(raw(t, ledger), &persisted); err != nil {
		t.Fatal(err)
	}
	if err := ValidateBudgetLedger(policy, persisted); err != nil {
		t.Fatal(err)
	}
	resumed, err := StartExecution(policy, persisted, now.Add(time.Hour))
	if err != nil || !equalJSON(resumed, ledger) {
		t.Fatal("resume renewed an expired execution window", err)
	}
	request.SourceVersion++
	request.Approval.SourceVersion = request.SourceVersion
	request.RequestHash = objectHash(t, "ExecutionRequest", request)
	if _, _, err = ReserveBudget(policy, persisted, ReservationInput{SourceVersion: request.SourceVersion, CommandID: request.CommandID, AttemptID: "action_three", Kind: "action", ScopeID: "scaffold", Request: &request, Now: now.Add(3 * time.Second)}); err == nil {
		t.Fatal("source revision reset stable action scope")
	}
}

func TestBudgetCIAndReconciliationWindowsNeverRestart(t *testing.T) {
	policy, ledger, request, now := executionBudgetFixture(t, "trigger_ci")
	in := ReservationInput{SourceVersion: request.SourceVersion, CommandID: request.CommandID, AttemptID: "poll_one", Kind: "ci_poll", ScopeID: "ci_poll:" + request.OperationID, Request: &request, Now: now.Add(time.Second)}
	ledger, first, err := ReserveBudget(policy, ledger, in)
	if err != nil {
		t.Fatal(err)
	}
	if *first.DeadlineAt != stamp(now.Add(10*time.Second)) || ledger.CIWindows[0].DeadlineAt != stamp(now.Add(21*time.Second)) {
		t.Fatal("CI did not preserve both total and operation deadlines")
	}
	in.AttemptID, in.Now = "poll_two", now.Add(11*time.Second)
	if _, _, err = ReserveBudget(policy, ledger, in); err == nil {
		t.Fatal("CI resumed past total execution deadline")
	}
	in.Kind, in.ScopeID, in.AttemptID = "reconciliation_read", "reconciliation_read:"+request.OperationID, "read_one"
	ledger, first, err = ReserveBudget(policy, ledger, in)
	if err != nil {
		t.Fatal("execution exhaustion incorrectly forbids bounded unknown-write observation", err)
	}
	if *first.DeadlineAt != stamp(now.Add(26*time.Second)) {
		t.Fatal("reconciliation deadline is not anchored at its original read")
	}
	in.AttemptID, in.Now = "read_two", now.Add(20*time.Second)
	ledger, second, err := ReserveBudget(policy, ledger, in)
	if err != nil || *second.DeadlineAt != *first.DeadlineAt || len(ledger.ReconciliationWindows) != 1 {
		t.Fatal("resume restarted the original reconciliation window", err)
	}
	in.AttemptID, in.Now = "read_three", now.Add(21*time.Second)
	if _, _, err = ReserveBudget(policy, ledger, in); err == nil {
		t.Fatal("pending read reservations did not consume quota")
	}
	in.AttemptID, in.Now = "read_four", now.Add(27*time.Second)
	if _, _, err = ReserveBudget(policy, ledger, in); err == nil {
		t.Fatal("expired unknown-write window restarted")
	}
}

func TestBudgetToolUsesFrozenCommandAndShortestLimits(t *testing.T) {
	policy, ledger, request, now := executionBudgetFixture(t, "verify")
	tool := contracts.PostCommand{CommandID: "go.test", Executable: "go", Arguments: raw(t, []string{"test", "./..."}), CwdScope: "run_workspace", EnvironmentPolicy: "isolated", TimeoutSeconds: 2, MaxAttempts: 1}
	in := ReservationInput{SourceVersion: request.SourceVersion, CommandID: request.CommandID, AttemptID: "tool_one", Kind: "tool", ScopeID: "tool:go.test", Request: &request, Tool: &tool, Now: now}
	ledger, reservation, err := ReserveBudget(policy, ledger, in)
	if err != nil {
		t.Fatal(err)
	}
	if *reservation.DeadlineAt != stamp(now.Add(2*time.Second)) {
		t.Fatal("tool did not take the stricter frozen timeout")
	}
	in.AttemptID = "tool_two"
	if _, _, err = ReserveBudget(policy, ledger, in); err == nil {
		t.Fatal("tool ignored its stricter declared max_attempts")
	}
	tool.Arguments = raw(t, []string{"build", "./..."})
	in.AttemptID = "tool_wrong_argv"
	if _, _, err = ReserveBudget(policy, ledger, in); err == nil {
		t.Fatal("tool command ID allowed a different fixed argument array")
	}
}

func TestBudgetLedgerRejectsRehashedSemanticTampering(t *testing.T) {
	policy, ledger, now := budgetFixture(t)
	ledger, reservation, err := ReserveBudget(policy, ledger, ReservationInput{SourceVersion: 1, CommandID: "command_one", AttemptID: "attempt_one", Kind: "command", ScopeID: "command_one", Now: now})
	if err != nil {
		t.Fatal(err)
	}
	ledger, _, err = SettleBudget(policy, ledger, SettlementInput{ReservationID: reservation.ReservationID, AttemptID: reservation.AttemptID, Outcome: "unknown", MeasurementSource: "unknown", Now: now})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		mutate func(*contracts.BudgetLedger)
	}{
		{"totals", func(l *contracts.BudgetLedger) { l.Totals.Consumed.Command = 0 }},
		{"duplicate_attempt", func(l *contracts.BudgetLedger) { l.Reservations = append(l.Reservations, l.Reservations[0]) }},
		{"wrong_policy", func(l *contracts.BudgetLedger) { l.PolicyHash = contracts.BytesHash([]byte("changed policy")) }},
		{"new_settlement_scope", func(l *contracts.BudgetLedger) {
			l.Settlements[0].SourceVersion++
			l.Settlements[0].SettlementHash = objectHash(t, "BudgetSettlement", l.Settlements[0])
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bad := copyLedger(ledger)
			tc.mutate(&bad)
			bad.LedgerHash = objectHash(t, "BudgetLedger", bad)
			if ValidateBudgetLedger(policy, bad) == nil {
				t.Fatal("correct self-hash concealed invalid resource history")
			}
		})
	}
}
