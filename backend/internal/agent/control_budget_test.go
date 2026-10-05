package agent

import (
	"testing"
	"time"

	"agentstudio/internal/governance"
)

func TestControlBudgetPersistsFailedAttemptAndOriginalSource(t *testing.T) {
	s := testService(t)
	s.Config.AIEndpoint = "http://127.0.0.1:1"
	s.Config.AIModel = "budget-fixture"
	r := controlFixture(t, s, "canonical_budget", "model")
	started := time.Now().UTC()
	input := governance.ReservationInput{SourceVersion: 1, CommandID: "request_model", AttemptID: "model_attempt_1", Kind: "model", ScopeID: "decision:model", TimeoutSeconds: 60, Now: started}
	r, reservation, err := s.reserveControlBudget(r.ID, input)
	if err != nil {
		t.Fatal(err)
	}
	firstVersion := r.Control.Audit.Budget.LedgerVersion
	input.Now = time.Now().UTC()
	r, repeated, err := s.reserveControlBudget(r.ID, input)
	if err != nil || repeated.ReservationHash != reservation.ReservationHash || r.Control.Audit.Budget.LedgerVersion != firstVersion || r.Usage.ModelAttempts != 1 {
		t.Fatal("repeated reservation changed identity or charged twice", err)
	}
	r, err = s.Store.Update(r.ID, func(run *Run) error { sourceChanged(run); return nil })
	if err != nil {
		t.Fatal(err)
	}
	// A failed attempt whose precise elapsed time was lost still consumes one
	// unit. Source changes cannot erase the original reservation provenance.
	settle := governance.SettlementInput{ReservationID: reservation.ReservationID, AttemptID: reservation.AttemptID, Outcome: "failed", MeasurementSource: "unknown", Now: time.Now().UTC()}
	r, settlement, err := s.settleControlBudget(r.ID, settle)
	if err != nil {
		t.Fatal(err)
	}
	if settlement.SourceVersion != 1 || r.SourceVersion != 2 || r.Control.Audit.Budget.Totals.Consumed.Model != 1 || r.Control.Audit.Budget.Totals.Reserved.Model != 0 || r.Usage.ModelAttempts != 1 || r.Usage.ModelFailures != 1 {
		t.Fatal("failure accounting or original source was lost")
	}
	ledgerVersion := r.Control.Audit.Budget.LedgerVersion
	settle.Now = time.Now().UTC()
	r, _, err = s.settleControlBudget(r.ID, settle)
	if err != nil || r.Control.Audit.Budget.LedgerVersion != ledgerVersion {
		t.Fatal("settlement replay charged twice", err)
	}
	cfg := s.Config
	s.Close()
	reopened, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	restored, ref, err := reopened.Store.CanonicalBudget(r.ID)
	if err != nil || restored.LedgerHash != r.Control.Audit.Budget.LedgerHash || ref.SourceVersion != 2 {
		t.Fatal("budget did not retain complete source-bound ledger", err)
	}
	if restored.Reservations[0].SourceVersion != 1 || restored.Settlements[0].SourceVersion != 1 {
		t.Fatal("restart rewrote attempt source")
	}
	zero := int64(0)
	settle.Outcome = "released"
	settle.MeasurementSource = "actual"
	settle.ElapsedMS = &zero
	settle.ConfirmedNotStarted = true
	if _, _, err = reopened.settleControlBudget(r.ID, settle); err == nil {
		t.Fatal("failed consumption was released")
	}
}
