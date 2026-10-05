package agent

import (
	"agentstudio/internal/contracts"
	"agentstudio/internal/governance"
)

// reserveControlBudget runs inside the single-writer snapshot transaction. The
// caller may start an attempt only after this method returns successfully; an
// object/archive/persistence error leaves the operation unstarted.
func (s *Service) reserveControlBudget(id string, input governance.ReservationInput) (Run, contracts.BudgetReservation, error) {
	var reservation contracts.BudgetReservation
	r, err := s.Store.Update(id, func(run *Run) error {
		if run.Control == nil || run.FrozenDefinition == nil {
			return fail("legacy_contract_incomplete", "当前Run缺少完整冻结预算账本")
		}
		if int64(run.SourceVersion) != input.SourceVersion {
			return fail("stale_source", "预算预留来源已改变")
		}
		if input.Kind == "model" && run.DecisionMode != "model" {
			return fail("invalid_mode", "rules模式不能预留模型调用")
		}
		if err := s.verifyFrozenDefinition(*run); err != nil {
			return err
		}
		ledger, value, err := governance.ReserveBudget(run.FrozenDefinition.Definition.BudgetPolicy, run.Control.Audit.Budget, input)
		if err != nil {
			return err
		}
		run.Control.Audit.Budget, reservation = ledger, value
		syncBudgetProjection(run)
		return nil
	})
	return r, reservation, err
}

// A settlement retains its original reservation source even if an accepted
// planning result has since changed source_version. It can never release an
// unknown or failed attempt as though the call had not happened.
func (s *Service) settleControlBudget(id string, input governance.SettlementInput) (Run, contracts.BudgetSettlement, error) {
	var settlement contracts.BudgetSettlement
	r, err := s.Store.Update(id, func(run *Run) error {
		if run.Control == nil || run.FrozenDefinition == nil {
			return fail("legacy_contract_incomplete", "当前Run缺少完整预算来源")
		}
		ledger, value, err := governance.SettleBudget(run.FrozenDefinition.Definition.BudgetPolicy, run.Control.Audit.Budget, input)
		if err != nil {
			return err
		}
		run.Control.Audit.Budget, settlement = ledger, value
		syncBudgetProjection(run)
		return nil
	})
	return r, settlement, err
}

func syncBudgetProjection(r *Run) {
	budget := r.Control.Audit.Budget
	r.Usage.Commands = int(budget.Totals.Consumed.Command + budget.Totals.Reserved.Command)
	r.Usage.ModelAttempts = int(budget.Totals.Consumed.Model + budget.Totals.Reserved.Model)
	r.Usage.ModelFailures = 0
	model := map[string]bool{}
	for _, reservation := range budget.Reservations {
		if reservation.Kind == "model" {
			model[reservation.ReservationID] = true
		}
	}
	for _, settlement := range budget.Settlements {
		if model[settlement.ReservationID] && settlement.Outcome != "succeeded" && settlement.Outcome != "released" {
			r.Usage.ModelFailures++
		}
	}
}
