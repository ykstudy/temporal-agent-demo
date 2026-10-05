package agent

import (
	"context"
	"errors"
	"sync"
	"time"

	"agentstudio/internal/contracts"
	"agentstudio/internal/governance"
)

// prepareControlSemanticAdvice records diagnosis/completion advice with its
// actual context and transport usage. It grants no execution authority and
// never changes lifecycle state. Callers must independently recheck their
// deterministic evidence before acting on the returned validation.
func (s *Service) prepareControlSemanticAdvice(ctx context.Context, id, kind string, rules contracts.DecisionAdvice, facts any, available []contracts.ObjectReference) (contracts.ActionValidation, error) {
	s.mu.Lock()
	lock := s.commandLocks[id]
	if lock == nil {
		lock = &sync.Mutex{}
		s.commandLocks[id] = lock
	}
	s.mu.Unlock()
	lock.Lock()
	defer lock.Unlock()
	return s.controlSemanticAdvice(ctx, id, kind, rules, facts, available)
}

// controlSemanticAdvice requires the caller to hold commandLocks[id]. A plan
// here is limited to the explicit VERIFY revision boundary; general planning
// continues through controlPlanningCommand and its persisted user command.
func (s *Service) controlSemanticAdvice(ctx context.Context, id, kind string, rules contracts.DecisionAdvice, facts any, available []contracts.ObjectReference) (contracts.ActionValidation, error) {
	var empty contracts.ActionValidation
	if !chainContains([]string{"diagnosis", "completion", "plan"}, kind) || rules.Kind != kind {
		return empty, fail("invalid_advice_kind", "不支持该语义建议入口")
	}
	r, err := s.Store.Get(id)
	if err != nil {
		return empty, err
	}
	if r.Control == nil || r.ExecutionMode != s.Config.Mode || r.ActiveStep != "" || terminal(r.State) || r.State == "PAUSED" || r.CancelRequested || r.PauseRequested {
		return empty, fail("execution_stopped", "当前Run不接受新语义建议")
	}
	if err = verifyControlChain(r); err != nil {
		return empty, err
	}
	if kind == "plan" && (r.State != "VERIFY" || rules.Plan == nil || rules.Plan.VerificationCandidateRef == nil) {
		return empty, fail("revision_not_allowed", "修订建议必须绑定VERIFY候选来源")
	}
	for _, ref := range available {
		if !scheduleHasRef(r.Control.ObjectRefs, ref) {
			return empty, fail("evidence_unavailable", "语义建议只能引用当前Run已登记的完整证据")
		}
	}
	hash, err := contracts.HashValue([]any{r.ID, r.SourceVersion, kind, facts, available})
	if err != nil {
		return empty, err
	}
	commandID := "semantic_" + hash[:32]
	var reserved *contracts.BudgetReservation
	if r.DecisionMode == "model" {
		timeout := r.FrozenDefinition.Definition.BudgetPolicy.MaxToolSeconds
		if timeout > 60 {
			timeout = 60
		}
		var value contracts.BudgetReservation
		r, value, err = s.reserveControlBudget(id, governance.ReservationInput{SourceVersion: int64(r.SourceVersion), CommandID: commandID, AttemptID: newID("model_attempt_"), Kind: "model", ScopeID: "decision:model", TimeoutSeconds: timeout, Now: time.Now().UTC()})
		if err != nil {
			return empty, err
		}
		reserved = &value
	}
	started, outcome := false, "failed"
	failure := func(cause error) (contracts.ActionValidation, error) {
		if reserved != nil {
			begin, _ := time.Parse(time.RFC3339Nano, reserved.ReservedAt)
			settlement := actualControlSettlement(*reserved, outcome, begin, time.Now().UTC())
			if !started {
				zero := int64(0)
				settlement.Outcome, settlement.ElapsedMS, settlement.ConfirmedNotStarted = "released", &zero, true
			}
			_, _, settleErr := s.settleControlBudget(id, settlement)
			cause = errors.Join(cause, settleErr)
		}
		return empty, cause
	}
	ordered, input, err := canonicalContextWithAuthority(r, map[string]any{
		"run_id": r.ID, "source_version": r.SourceVersion, "state": r.State,
		"definition_snapshot_hash": r.FrozenDefinition.SnapshotHash,
		"instruction":              "Return only a " + kind + " DecisionAdvice. This is advice; it cannot grant permission, perform an action or decide DONE.",
		"observed_facts":           facts, "available_evidence": available,
	})
	if err != nil {
		return failure(err)
	}
	advice, usage := rules, (contracts.ModelUsage{Status: "not_applicable"})
	if r.DecisionMode == "model" {
		body, e := canonicalModelRequest(r, ordered)
		if e != nil {
			return failure(e)
		}
		deadline, _ := time.Parse(time.RFC3339Nano, *reserved.DeadlineAt)
		bounded, cancel := context.WithDeadline(ctx, deadline)
		response, e := s.callModelAdvice(bounded, r, body)
		started = response.Started
		cancel()
		if e != nil {
			return failure(e)
		}
		advice, usage, ordered, outcome = response.Advice, response.Usage, response.Messages, "succeeded"
	} else if r.DecisionMode != "rules" {
		return failure(fail("invalid_mode", "不支持的建议模式"))
	}
	in, record, err := bindCanonicalDecision(r, commandID, advice, usage, input, ordered, time.Now().UTC())
	if err != nil {
		return failure(err)
	}
	in.Facts.AvailableEvidence = append([]contracts.ObjectReference{}, available...)
	in.Facts.RevisionAllowed = kind == "plan"
	validation, err := (governance.Validator{Definitions: s.Definitions}).Validate(in)
	if err != nil {
		return failure(err)
	}
	_, err = s.Store.Update(id, func(run *Run) error {
		if e := s.recordCanonicalDecision(run, in, record, validation); e != nil {
			return e
		}
		if reserved != nil {
			return settleControlInRun(run, *reserved, outcome, time.Now().UTC())
		}
		return nil
	})
	if err != nil {
		return failure(err)
	}
	reserved = nil
	if advice.Kind != kind || validation.Verdict != "allowed" || validation.ExecutionRequest != nil {
		return validation, fail("advice_rejected", "实际建议不满足当前诊断或完成入口；未授予执行权")
	}
	return validation, nil
}
