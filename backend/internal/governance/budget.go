package governance

import (
	"encoding/json"
	"fmt"
	"time"

	"agentstudio/internal/contracts"
)

// ReservationInput comes from the executor's trusted admission transaction.
// Tool must be the merged frozen PostCommand, never arguments from a model.
type ReservationInput struct {
	SourceVersion                       int64
	CommandID, AttemptID, Kind, ScopeID string
	Request                             *contracts.ExecutionRequest
	Tool                                *contracts.PostCommand
	GitTool                             *contracts.GitCommandPolicy
	TimeoutSeconds                      int64
	Now                                 time.Time
}

type SettlementInput struct {
	ReservationID, AttemptID, Outcome, MeasurementSource string
	ElapsedMS                                            *int64
	ConfirmedNotStarted                                  bool
	Now                                                  time.Time
}

func NewBudgetLedger(runID, definitionSnapshotHash string, policy contracts.BudgetPolicy, now time.Time) (contracts.BudgetLedger, error) {
	return NewBudgetLedgerVersion(contracts.ContractVersion, runID, definitionSnapshotHash, policy, now)
}

// NewBudgetLedgerVersion creates a ledger in the exact frozen definition's
// release; a newer codec never silently upgrades an existing Run's source.
func NewBudgetLedgerVersion(version, runID, definitionSnapshotHash string, policy contracts.BudgetPolicy, now time.Time) (contracts.BudgetLedger, error) {
	if now.IsZero() || contracts.Validate("BudgetPolicy", policy) != nil {
		return contracts.BudgetLedger{}, reject("invalid_budget_policy", "a complete frozen policy and trusted time are required")
	}
	policyHash, err := contracts.HashValue(policy)
	if err != nil {
		return contracts.BudgetLedger{}, err
	}
	ledger := contracts.BudgetLedger{ContractVersion: version, RunID: runID, DefinitionSnapshotHash: definitionSnapshotHash, PolicyHash: policyHash, LedgerVersion: 1, CIWindows: []contracts.BudgetWindow{}, ReconciliationWindows: []contracts.BudgetWindow{}, Reservations: []contracts.BudgetReservation{}, Settlements: []contracts.BudgetSettlement{}, UpdatedAt: stamp(now)}
	ledger.LedgerHash, err = contracts.HashObject("BudgetLedger", ledger)
	if err == nil {
		err = ValidateBudgetLedger(policy, ledger)
	}
	return ledger, err
}

// StartExecution is idempotent. Calling it on resume cannot move the original
// beginning or deadline, even after that deadline has already elapsed.
func StartExecution(policy contracts.BudgetPolicy, ledger contracts.BudgetLedger, now time.Time) (contracts.BudgetLedger, error) {
	if err := ValidateBudgetLedger(policy, ledger); err != nil {
		return ledger, err
	}
	if ledger.ExecutionStartedAt != nil {
		return ledger, nil
	}
	if err := budgetTime(ledger, now); err != nil {
		return ledger, err
	}
	out := copyLedger(ledger)
	start, end := stamp(now), stamp(now.Add(time.Duration(policy.MaxExecutionSeconds)*time.Second))
	out.ExecutionStartedAt, out.ExecutionDeadlineAt = &start, &end
	return finishLedger(policy, out, now)
}

// ReserveBudget returns the original reservation for a repeated identical
// attempt. Its deadline and charge are never renewed by a repeated call.
func ReserveBudget(policy contracts.BudgetPolicy, ledger contracts.BudgetLedger, in ReservationInput) (contracts.BudgetLedger, contracts.BudgetReservation, error) {
	if err := ValidateBudgetLedger(policy, ledger); err != nil {
		return ledger, contracts.BudgetReservation{}, err
	}
	if in.Now.IsZero() || in.SourceVersion < 1 || in.CommandID == "" || in.AttemptID == "" {
		return ledger, contracts.BudgetReservation{}, reject("invalid_reservation", "trusted attempt, command, source and time are required")
	}
	scope, maxAttempts, timeout, err := reservationScope(policy, in)
	if err != nil {
		return ledger, contracts.BudgetReservation{}, err
	}
	if in.ScopeID != scope {
		return ledger, contracts.BudgetReservation{}, reject("budget_scope_mismatch", "reservation scope differs from the stable published scope")
	}
	reservation := contracts.BudgetReservation{ContractVersion: ledger.ContractVersion, ReservationID: hashID("reservation_", ledger.RunID, in.AttemptID), RunID: ledger.RunID, SourceVersion: in.SourceVersion, DefinitionSnapshotHash: ledger.DefinitionSnapshotHash, PolicyHash: ledger.PolicyHash, CommandID: in.CommandID, AttemptID: in.AttemptID, Kind: in.Kind, ScopeID: scope, Units: 1, ReservedAt: stamp(in.Now)}
	if in.Request != nil {
		r := in.Request
		if r.RunID != ledger.RunID || r.SourceVersion != in.SourceVersion || r.DefinitionSnapshotHash != ledger.DefinitionSnapshotHash || r.CommandID != in.CommandID || r.ContractVersion != ledger.ContractVersion {
			return ledger, reservation, reject("budget_request_mismatch", "reservation does not bind this Run's exact execution request")
		}
		opID, executionID, requestHash := r.OperationID, r.ExecutionID, r.RequestHash
		reservation.OperationID, reservation.ExecutionID, reservation.RequestHash = &opID, &executionID, &requestHash
	}
	for _, existing := range ledger.Reservations {
		if existing.AttemptID == in.AttemptID {
			if existing.SourceVersion != reservation.SourceVersion || existing.CommandID != reservation.CommandID || existing.Kind != reservation.Kind || existing.ScopeID != reservation.ScopeID || !equalJSON(existing.OperationID, reservation.OperationID) || !equalJSON(existing.ExecutionID, reservation.ExecutionID) || !equalJSON(existing.RequestHash, reservation.RequestHash) {
				return ledger, reservation, reject("budget_attempt_conflict", "attempt identity is already bound to different work")
			}
			return ledger, existing, nil
		}
	}
	if err := budgetTime(ledger, in.Now); err != nil {
		return ledger, reservation, err
	}
	if len(ledger.Reservations) >= 4096 {
		return ledger, reservation, reject("budget_capacity_exceeded", "the immutable reservation ledger is full")
	}
	out := copyLedger(ledger)
	var deadline time.Time
	if in.Kind != "command" {
		if in.Kind == "model" {
			deadline = in.Now.Add(time.Duration(timeout) * time.Second)
		} else if in.Kind != "reconciliation_read" {
			if ledger.ExecutionDeadlineAt == nil {
				return ledger, reservation, reject("execution_not_started", "execution start and deadline must be persisted before action admission")
			}
		}
		if ledger.ExecutionDeadlineAt != nil && in.Kind != "reconciliation_read" {
			end, _ := parseStamp(*ledger.ExecutionDeadlineAt)
			deadline = minDeadline(deadline, end)
		}
		if in.Kind == "tool" {
			deadline = minDeadline(deadline, in.Now.Add(time.Duration(timeout)*time.Second))
		}
		if in.Kind == "ci_poll" || in.Kind == "reconciliation_read" {
			window, windowErr := ensureWindow(policy, &out, in)
			if windowErr != nil {
				return ledger, reservation, windowErr
			}
			end, _ := parseStamp(window.DeadlineAt)
			deadline = minDeadline(deadline, end)
		}
		if deadline.IsZero() || !in.Now.Before(deadline) {
			return ledger, reservation, reject("budget_deadline_exceeded", "the persisted deadline has been exhausted")
		}
		end := stamp(deadline)
		reservation.DeadlineAt = &end
	}
	if occupied(ledger, in.Kind, scope) >= maxAttempts {
		return ledger, reservation, reject("budget_exceeded", "reserved and consumed attempts reached the frozen limit")
	}
	reservation.ReservationHash, err = contracts.HashObject("BudgetReservation", reservation)
	if err != nil {
		return ledger, reservation, err
	}
	if err = contracts.Validate("BudgetReservation", reservation); err != nil {
		return ledger, reservation, reject("invalid_reservation", "reservation violates its complete published schema")
	}
	out.Reservations = append(out.Reservations, reservation)
	out, err = finishLedger(policy, out, in.Now)
	if err != nil {
		return ledger, reservation, err
	}
	return out, reservation, nil
}

// SettleBudget only appends a first settlement. An unknown charge is final as a
// budget fact; later external reconciliation records separate operation facts.
func SettleBudget(policy contracts.BudgetPolicy, ledger contracts.BudgetLedger, in SettlementInput) (contracts.BudgetLedger, contracts.BudgetSettlement, error) {
	if err := ValidateBudgetLedger(policy, ledger); err != nil {
		return ledger, contracts.BudgetSettlement{}, err
	}
	var reservation *contracts.BudgetReservation
	for i := range ledger.Reservations {
		if ledger.Reservations[i].ReservationID == in.ReservationID {
			reservation = &ledger.Reservations[i]
		}
	}
	if reservation == nil || reservation.AttemptID != in.AttemptID {
		return ledger, contracts.BudgetSettlement{}, reject("reservation_not_found", "settlement must bind an existing reservation and exact attempt")
	}
	if !oneOf(in.Outcome, "succeeded", "failed", "interrupted", "released", "unknown") || !oneOf(in.MeasurementSource, "actual", "unknown") {
		return ledger, contracts.BudgetSettlement{}, reject("invalid_settlement", "outcome and measurement source must be explicit")
	}
	if in.Outcome == "released" && (!in.ConfirmedNotStarted || in.MeasurementSource != "actual" || in.ElapsedMS == nil || *in.ElapsedMS != 0) {
		return ledger, contracts.BudgetSettlement{}, reject("release_not_proven", "release requires trusted proof that no attempt started and zero actual duration")
	}
	if in.MeasurementSource == "actual" && (in.ElapsedMS == nil || *in.ElapsedMS < 0) || in.MeasurementSource == "unknown" && in.ElapsedMS != nil {
		return ledger, contracts.BudgetSettlement{}, reject("invalid_measurement", "unknown elapsed time cannot be presented as a measured number")
	}
	for _, existing := range ledger.Settlements {
		if existing.ReservationID == in.ReservationID {
			if existing.Outcome != in.Outcome || existing.MeasurementSource != in.MeasurementSource || !equalJSON(existing.ElapsedMS, in.ElapsedMS) {
				return ledger, existing, reject("settlement_conflict", "the original settlement is immutable, including unknown consumption")
			}
			return ledger, existing, nil
		}
	}
	if err := budgetTime(ledger, in.Now); err != nil {
		return ledger, contracts.BudgetSettlement{}, err
	}
	if len(ledger.Settlements) >= 4096 {
		return ledger, contracts.BudgetSettlement{}, reject("budget_capacity_exceeded", "the immutable settlement ledger is full")
	}
	reservedAt, _ := parseStamp(reservation.ReservedAt)
	if in.Now.Before(reservedAt) || in.ElapsedMS != nil && *in.ElapsedMS > in.Now.Sub(reservedAt).Milliseconds() {
		return ledger, contracts.BudgetSettlement{}, reject("invalid_measurement", "settlement cannot predate reservation or exceed its observed wall-clock interval")
	}
	consumed := int64(1)
	if in.Outcome == "released" {
		consumed = 0
	}
	var elapsed *int64
	if in.ElapsedMS != nil {
		value := *in.ElapsedMS
		elapsed = &value
	}
	settlement := contracts.BudgetSettlement{ContractVersion: ledger.ContractVersion, SettlementID: hashID("settlement_", ledger.RunID, reservation.ReservationID), ReservationID: reservation.ReservationID, ReservationHash: reservation.ReservationHash, AttemptID: reservation.AttemptID, RunID: ledger.RunID, SourceVersion: reservation.SourceVersion, ConsumedUnits: consumed, Outcome: in.Outcome, ElapsedMS: elapsed, MeasurementSource: in.MeasurementSource, SettledAt: stamp(in.Now)}
	var err error
	settlement.SettlementHash, err = contracts.HashObject("BudgetSettlement", settlement)
	if err != nil {
		return ledger, settlement, err
	}
	out := copyLedger(ledger)
	out.Settlements = append(out.Settlements, settlement)
	out, err = finishLedger(policy, out, in.Now)
	if err != nil {
		return ledger, settlement, err
	}
	return out, settlement, nil
}

func reservationScope(policy contracts.BudgetPolicy, in ReservationInput) (string, int64, int64, error) {
	if in.Kind == "command" || in.Kind == "model" {
		if in.Request != nil || in.Tool != nil || in.GitTool != nil {
			return "", 0, 0, reject("invalid_reservation", "command and model reservations cannot claim execution identity")
		}
		if in.Kind == "command" {
			return in.CommandID, policy.MaxCommands, 0, nil
		}
		if in.TimeoutSeconds < 1 || in.TimeoutSeconds > policy.MaxToolSeconds {
			return "", 0, 0, reject("invalid_attempt_timeout", "model attempt timeout must be explicit and within the frozen single-call bound")
		}
		return "decision:model", policy.MaxModelAttempts, in.TimeoutSeconds, nil
	}
	if in.Request == nil {
		return "", 0, 0, reject("budget_request_missing", "this reservation kind requires its complete frozen execution request")
	}
	if err := checkHash("ExecutionRequest", *in.Request, in.Request.RequestHash); err != nil {
		return "", 0, 0, err
	}
	var action struct {
		ActionType string          `json:"action_type"`
		Parameters json.RawMessage `json:"parameters"`
	}
	if err := json.Unmarshal(in.Request.Action, &action); err != nil {
		return "", 0, 0, err
	}
	switch in.Kind {
	case "action":
		scope := action.ActionType
		if scope == "apply_skill" {
			var params contracts.ApplySkillParameters
			if json.Unmarshal(action.Parameters, &params) != nil {
				return "", 0, 0, reject("invalid_action", "invalid skill action")
			}
			var selection selectionFields
			if json.Unmarshal(params.Selection, &selection) != nil {
				return "", 0, 0, reject("invalid_action", "invalid skill selection")
			}
			scope += ":" + selection.SkillID
		}
		return scope, policy.MaxStepAttempts, 0, nil
	case "tool":
		if in.GitTool != nil {
			if in.Tool != nil || !contracts.DeliveryContract(in.Request.ContractVersion) || contracts.ValidateVersion("GitCommandPolicy", in.Request.ContractVersion, *in.GitTool) != nil {
				return "", 0, 0, reject("invalid_tool_command", "Git requires its complete frozen command policy and 1.3 request")
			}
			if action.ActionType != "push_repository" {
				return "", 0, 0, reject("invalid_tool_stage", "fixed Git commands only run in controlled push_repository")
			}
			return "tool:" + in.GitTool.CommandID, minInt(policy.MaxStepAttempts, in.GitTool.MaxAttempts), minInt(policy.MaxToolSeconds, in.GitTool.TimeoutSeconds), nil
		}
		if in.Tool == nil || contracts.Validate("PostCommand", *in.Tool) != nil {
			return "", 0, 0, reject("invalid_tool_command", "tool reservation requires a complete frozen PostCommand")
		}
		fixed := map[string][]string{"go.mod_tidy": {"mod", "tidy"}, "go.test": {"test", "./..."}, "go.build": {"build", "./..."}}
		if in.Tool.Executable != "go" || in.Tool.CwdScope != "run_workspace" || in.Tool.EnvironmentPolicy != "isolated" || !equalJSON(in.Tool.Arguments, fixed[in.Tool.CommandID]) || fixed[in.Tool.CommandID] == nil {
			return "", 0, 0, reject("invalid_tool_command", "tool command ID and exact argv/environment do not match the fixed executor")
		}
		if action.ActionType != "verify" {
			return "", 0, 0, reject("invalid_tool_stage", "merged post commands only run in the controlled verify action")
		}
		return "tool:" + in.Tool.CommandID, minInt(policy.MaxStepAttempts, in.Tool.MaxAttempts), minInt(policy.MaxToolSeconds, in.Tool.TimeoutSeconds), nil
	case "ci_poll":
		if action.ActionType != "trigger_ci" {
			return "", 0, 0, reject("invalid_read_scope", "CI polling requires the original trigger_ci request")
		}
		return "ci_poll:" + in.Request.OperationID, policy.MaxCIPollAttempts, 0, nil
	case "reconciliation_read":
		if !oneOf(action.ActionType, "create_repository", "push_repository", "trigger_ci") {
			return "", 0, 0, reject("invalid_read_scope", "external reconciliation requires the original remote write request")
		}
		return "reconciliation_read:" + in.Request.OperationID, policy.MaxReconciliationAttempts, 0, nil
	}
	return "", 0, 0, reject("invalid_budget_kind", "unknown resource reservation kind")
}

func ensureWindow(policy contracts.BudgetPolicy, ledger *contracts.BudgetLedger, in ReservationInput) (contracts.BudgetWindow, error) {
	windows := &ledger.CIWindows
	seconds, attempts := policy.MaxCIWaitSeconds, policy.MaxCIPollAttempts
	if in.Kind == "reconciliation_read" {
		windows = &ledger.ReconciliationWindows
		seconds, attempts = policy.MaxReconciliationSeconds, policy.MaxReconciliationAttempts
	}
	for _, window := range *windows {
		if window.OperationID == in.Request.OperationID {
			if window.ScopeID != in.ScopeID || window.SourceVersion != in.SourceVersion {
				return window, reject("budget_window_conflict", "original operation window has a different scope or source")
			}
			return window, nil
		}
	}
	if len(*windows) >= 256 {
		return contracts.BudgetWindow{}, reject("budget_capacity_exceeded", "the persisted operation window registry is full")
	}
	window := contracts.BudgetWindow{WindowID: hashID("window_", ledger.RunID, in.Kind, in.Request.OperationID), Kind: in.Kind, OperationID: in.Request.OperationID, ScopeID: in.ScopeID, SourceVersion: in.SourceVersion, StartedAt: stamp(in.Now), DeadlineAt: stamp(in.Now.Add(time.Duration(seconds) * time.Second)), MaxAttempts: attempts}
	*windows = append(*windows, window)
	return window, nil
}

func finishLedger(policy contracts.BudgetPolicy, ledger contracts.BudgetLedger, now time.Time) (contracts.BudgetLedger, error) {
	ledger.LedgerVersion++
	ledger.UpdatedAt = stamp(now)
	totals, err := budgetTotals(ledger)
	if err != nil {
		return ledger, err
	}
	ledger.Totals = totals
	ledger.LedgerHash, err = contracts.HashObject("BudgetLedger", ledger)
	if err == nil {
		err = ValidateBudgetLedger(policy, ledger)
	}
	return ledger, err
}

func budgetTotals(ledger contracts.BudgetLedger) (contracts.BudgetTotals, error) {
	totals := contracts.BudgetTotals{}
	settled := map[string]contracts.BudgetSettlement{}
	for _, s := range ledger.Settlements {
		if _, found := settled[s.ReservationID]; found {
			return totals, reject("duplicate_settlement", "reservation has more than one settlement")
		}
		settled[s.ReservationID] = s
	}
	for _, r := range ledger.Reservations {
		s, ok := settled[r.ReservationID]
		if !ok {
			increment(&totals.Reserved, r.Kind)
			continue
		}
		if s.Outcome == "released" {
			increment(&totals.Released, r.Kind)
		} else {
			increment(&totals.Consumed, r.Kind)
			if s.Outcome == "unknown" {
				increment(&totals.Unknown, r.Kind)
			}
		}
		if s.MeasurementSource == "actual" && s.ElapsedMS != nil {
			totals.MeasuredElapsedMS += *s.ElapsedMS
		} else {
			totals.UnknownElapsedAttempts++
		}
	}
	return totals, nil
}

func occupied(ledger contracts.BudgetLedger, kind, scope string) int64 {
	released := map[string]bool{}
	for _, settlement := range ledger.Settlements {
		if settlement.Outcome == "released" {
			released[settlement.ReservationID] = true
		}
	}
	var count int64
	for _, r := range ledger.Reservations {
		if r.Kind == kind && (kind == "command" || kind == "model" || r.ScopeID == scope) && !released[r.ReservationID] {
			count++
		}
	}
	return count
}

func increment(c *contracts.BudgetCounters, kind string) {
	switch kind {
	case "command":
		c.Command++
	case "model":
		c.Model++
	case "action":
		c.Action++
	case "tool":
		c.Tool++
	case "ci_poll":
		c.CIPoll++
	case "reconciliation_read":
		c.ReconciliationRead++
	}
}
func copyLedger(in contracts.BudgetLedger) contracts.BudgetLedger {
	out := in
	out.ExecutionStartedAt, out.ExecutionDeadlineAt = copyString(in.ExecutionStartedAt), copyString(in.ExecutionDeadlineAt)
	out.Reservations = append([]contracts.BudgetReservation{}, in.Reservations...)
	out.Settlements = append([]contracts.BudgetSettlement{}, in.Settlements...)
	out.CIWindows = append([]contracts.BudgetWindow{}, in.CIWindows...)
	out.ReconciliationWindows = append([]contracts.BudgetWindow{}, in.ReconciliationWindows...)
	for i := range out.Reservations {
		r := &out.Reservations[i]
		r.OperationID, r.ExecutionID, r.RequestHash, r.DeadlineAt = copyString(r.OperationID), copyString(r.ExecutionID), copyString(r.RequestHash), copyString(r.DeadlineAt)
	}
	for i := range out.Settlements {
		if out.Settlements[i].ElapsedMS != nil {
			elapsed := *out.Settlements[i].ElapsedMS
			out.Settlements[i].ElapsedMS = &elapsed
		}
	}
	return out
}
func copyString(in *string) *string {
	if in == nil {
		return nil
	}
	value := *in
	return &value
}
func stamp(now time.Time) string                 { return now.UTC().Format(time.RFC3339Nano) }
func parseStamp(value string) (time.Time, error) { return time.Parse(time.RFC3339Nano, value) }
func minInt(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}
func minDeadline(a, b time.Time) time.Time {
	if a.IsZero() || b.Before(a) {
		return b
	}
	return a
}
func budgetTime(ledger contracts.BudgetLedger, now time.Time) error {
	last, err := parseStamp(ledger.UpdatedAt)
	if err != nil || now.IsZero() || now.Before(last) {
		return reject("invalid_budget_time", "ledger updates require a nondecreasing trusted clock")
	}
	return nil
}

// ValidateBudgetLedger rechecks hashes, identity closure, immutable windows and
// reconstructed counters. A correct self-hash alone cannot authorize resources.
func ValidateBudgetLedger(policy contracts.BudgetPolicy, ledger contracts.BudgetLedger) error {
	if contracts.Validate("BudgetPolicy", policy) != nil {
		return reject("invalid_budget_policy", "the frozen policy violates its schema")
	}
	if err := checkHash("BudgetLedger", ledger, ledger.LedgerHash); err != nil {
		return err
	}
	policyHash, err := contracts.HashValue(policy)
	if err != nil || policyHash != ledger.PolicyHash {
		return reject("budget_policy_mismatch", "ledger is not bound to the complete frozen budget policy")
	}
	updated, _ := parseStamp(ledger.UpdatedAt)
	var executionStart, executionEnd time.Time
	if ledger.ExecutionStartedAt != nil {
		executionStart, err = parseStamp(*ledger.ExecutionStartedAt)
		if err != nil {
			return err
		}
		executionEnd, err = parseStamp(*ledger.ExecutionDeadlineAt)
		if err != nil || !executionEnd.Equal(executionStart.Add(time.Duration(policy.MaxExecutionSeconds)*time.Second)) || executionStart.After(updated) {
			return reject("budget_deadline_mismatch", "execution deadline does not match its original frozen start and duration")
		}
	}
	windows := map[string]contracts.BudgetWindow{}
	for _, collection := range [][]contracts.BudgetWindow{ledger.CIWindows, ledger.ReconciliationWindows} {
		for _, w := range collection {
			key := w.Kind + ":" + w.OperationID
			if _, found := windows[key]; found {
				return reject("duplicate_budget_window", "an operation cannot receive a second window")
			}
			seconds, limit := policy.MaxCIWaitSeconds, policy.MaxCIPollAttempts
			if w.Kind == "reconciliation_read" {
				seconds, limit = policy.MaxReconciliationSeconds, policy.MaxReconciliationAttempts
			}
			start, e1 := parseStamp(w.StartedAt)
			end, e2 := parseStamp(w.DeadlineAt)
			if e1 != nil || e2 != nil || w.ScopeID != key || w.WindowID != hashID("window_", ledger.RunID, w.Kind, w.OperationID) || w.MaxAttempts != limit || !end.Equal(start.Add(time.Duration(seconds)*time.Second)) || start.After(updated) {
				return reject("budget_window_mismatch", "operation window identity, limit or original deadline is inconsistent")
			}
			windows[key] = w
		}
	}
	reservations := map[string]contracts.BudgetReservation{}
	attempts := map[string]bool{}
	earliest := map[string]string{}
	previous := time.Time{}
	for _, r := range ledger.Reservations {
		if err := checkHash("BudgetReservation", r, r.ReservationHash); err != nil {
			return err
		}
		if _, found := reservations[r.ReservationID]; found || attempts[r.AttemptID] || r.ReservationID != hashID("reservation_", ledger.RunID, r.AttemptID) || r.RunID != ledger.RunID || r.DefinitionSnapshotHash != ledger.DefinitionSnapshotHash || r.PolicyHash != ledger.PolicyHash || r.ContractVersion != ledger.ContractVersion {
			return reject("budget_reservation_mismatch", "reservation identity or immutable budget binding is inconsistent")
		}
		reserved, e := parseStamp(r.ReservedAt)
		if e != nil || reserved.Before(previous) || reserved.After(updated) {
			return reject("invalid_budget_time", "reservation order and ledger time disagree")
		}
		previous = reserved
		if !validStoredScope(r) {
			return reject("budget_scope_mismatch", "stored reservation changed its stable cross-source scope")
		}
		if r.Kind != "command" {
			if r.DeadlineAt == nil {
				return reject("budget_deadline_missing", "timed attempts require a persisted absolute deadline")
			}
			deadline, e := parseStamp(*r.DeadlineAt)
			if e != nil || !reserved.Before(deadline) {
				return reject("budget_deadline_mismatch", "attempt deadline is not after its reservation")
			}
			if r.Kind != "model" && r.Kind != "reconciliation_read" && (executionStart.IsZero() || reserved.Before(executionStart)) {
				return reject("execution_not_started", "action resources were reserved without a persisted execution start")
			}
			if r.Kind != "reconciliation_read" && !executionStart.IsZero() && !reserved.Before(executionStart) && deadline.After(executionEnd) {
				return reject("budget_deadline_mismatch", "attempt extends the total execution deadline")
			}
			if (r.Kind == "tool" || r.Kind == "model") && deadline.After(reserved.Add(time.Duration(policy.MaxToolSeconds)*time.Second)) {
				return reject("budget_deadline_mismatch", "single-call deadline exceeds the frozen bound")
			}
			if r.Kind == "ci_poll" || r.Kind == "reconciliation_read" {
				w, found := windows[r.ScopeID]
				end, _ := parseStamp(w.DeadlineAt)
				if !found || w.SourceVersion != r.SourceVersion || r.OperationID == nil || w.OperationID != *r.OperationID || reserved.Before(mustTime(w.StartedAt)) || deadline.After(end) {
					return reject("budget_window_mismatch", "read reservation is not inside its original operation window")
				}
				if earliest[r.ScopeID] == "" {
					earliest[r.ScopeID] = r.ReservedAt
				}
			}
		}
		reservations[r.ReservationID] = r
		attempts[r.AttemptID] = true
	}
	for key, w := range windows {
		if earliest[key] != w.StartedAt {
			return reject("budget_window_mismatch", "window must retain the first recorded read attempt's beginning")
		}
	}
	settlementIDs := map[string]bool{}
	settledAttempts := map[string]bool{}
	previous = time.Time{}
	for _, s := range ledger.Settlements {
		if err := checkHash("BudgetSettlement", s, s.SettlementHash); err != nil {
			return err
		}
		r, found := reservations[s.ReservationID]
		if !found || settlementIDs[s.SettlementID] || settledAttempts[s.AttemptID] || s.SettlementID != hashID("settlement_", ledger.RunID, r.ReservationID) || s.RunID != ledger.RunID || s.SourceVersion != r.SourceVersion || s.ReservationHash != r.ReservationHash || s.AttemptID != r.AttemptID || s.ContractVersion != ledger.ContractVersion {
			return reject("budget_settlement_mismatch", "settlement does not bind a unique exact prior reservation")
		}
		settled, e := parseStamp(s.SettledAt)
		reserved, _ := parseStamp(r.ReservedAt)
		if e != nil || settled.Before(reserved) || settled.Before(previous) || settled.After(updated) || s.ElapsedMS != nil && *s.ElapsedMS > settled.Sub(reserved).Milliseconds() {
			return reject("invalid_budget_time", "settlement timing or measured duration is inconsistent")
		}
		previous = settled
		if s.Outcome == "released" && (s.MeasurementSource != "actual" || s.ElapsedMS == nil || *s.ElapsedMS != 0) {
			return reject("release_not_proven", "released attempts cannot contain consumed or unknown elapsed execution")
		}
		settlementIDs[s.SettlementID] = true
		settledAttempts[s.AttemptID] = true
	}
	totals, err := budgetTotals(ledger)
	if err != nil {
		return err
	}
	if !equalJSON(totals, ledger.Totals) {
		return reject("budget_totals_mismatch", "displayed totals do not equal the immutable reservation and settlement facts")
	}
	for _, r := range ledger.Reservations {
		limit := policy.MaxStepAttempts
		switch r.Kind {
		case "command":
			limit = policy.MaxCommands
		case "model":
			limit = policy.MaxModelAttempts
		case "ci_poll":
			limit = policy.MaxCIPollAttempts
		case "reconciliation_read":
			limit = policy.MaxReconciliationAttempts
		}
		if occupied(ledger, r.Kind, r.ScopeID) > limit {
			return reject("budget_exceeded", fmt.Sprintf("stored %s reservations exceed the frozen limit", r.Kind))
		}
	}
	return nil
}

func validStoredScope(r contracts.BudgetReservation) bool {
	switch r.Kind {
	case "command":
		return r.ScopeID == r.CommandID
	case "model":
		return r.ScopeID == "decision:model"
	case "action":
		return oneOf(r.ScopeID, "scaffold", "verify", "create_repository", "push_repository", "trigger_ci", "package", "apply_skill:add_database", "apply_skill:add_auth", "apply_skill:add_logging")
	case "tool":
		return oneOf(r.ScopeID, "tool:go.mod_tidy", "tool:go.test", "tool:go.build") || contracts.DeliveryContract(r.ContractVersion) && oneOf(r.ScopeID, "tool:git.init", "tool:git.add", "tool:git.commit", "tool:git.head", "tool:git.tree", "tool:git.push")
	case "ci_poll", "reconciliation_read":
		return r.OperationID != nil && r.ScopeID == r.Kind+":"+*r.OperationID
	}
	return false
}
func mustTime(value string) time.Time { out, _ := parseStamp(value); return out }
