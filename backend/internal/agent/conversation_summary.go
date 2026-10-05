package agent

import (
	"context"
	"encoding/json"
	"time"

	"agentstudio/internal/contracts"
	"agentstudio/internal/conversation"
	"agentstudio/internal/governance"
	"agentstudio/internal/storage"
)

// Called under the existing per-Run command lock. The source range is frozen
// durably before a provider call; progress updates can append later messages
// without entering that range. No failure changes decision_mode or approval.
func (s *Service) summarizeConversation(ctx context.Context, r Run, cmd Command) (Run, error) {
	c := conversationState(r)
	if c == nil {
		return r, nil
	}
	policy := r.FrozenDefinition.Definition.ContextPolicy
	through := int64(0)
	if len(c.Summaries) > 0 {
		through = c.Summaries[len(c.Summaries)-1].Source.LastSequence
	}
	end := int64(len(r.Control.Audit.Messages)) - policy.RecentMessages
	if int64(len(r.Control.Audit.Messages))-through < policy.CompressionThreshold || end <= through {
		return r, nil
	}
	if end-through > 256 {
		end = through + 256
	}
	messages := append([]contracts.MessageEnvelope{}, r.Control.Audit.Messages[through:end]...)
	source, e := conversation.Source(messages)
	if e != nil {
		return r, e
	}
	pending := summaryPending{AttemptID: newID("summary_attempt_"), Source: source, SourceVersion: int64(r.SourceVersion), StartedAt: now()}
	r, e = s.Store.Update(r.ID, func(run *Run) error {
		if conversationState(*run).PendingSummary != nil {
			return fail("summary_pending", "previous summary outcome requires recovery")
		}
		if run.DecisionMode == "model" {
			timeout := policy.OutputReserveTokens
			if timeout > 60 {
				timeout = 60
			}
			if timeout > run.FrozenDefinition.Definition.BudgetPolicy.MaxToolSeconds {
				timeout = run.FrozenDefinition.Definition.BudgetPolicy.MaxToolSeconds
			}
			ledger, reservation, err := governance.ReserveBudget(run.FrozenDefinition.Definition.BudgetPolicy, run.Control.Audit.Budget, governance.ReservationInput{SourceVersion: pending.SourceVersion, CommandID: cmd.ID, AttemptID: pending.AttemptID, Kind: "model", ScopeID: "decision:model", TimeoutSeconds: timeout, Now: time.Now().UTC()})
			if err != nil {
				return err
			}
			run.Control.Audit.Budget = ledger
			pending.Reservation = &reservation
			syncBudgetProjection(run)
		}
		conversationState(*run).PendingSummary = &pending
		return nil
	})
	if e != nil {
		return r, e
	}
	advice := conversation.Rules(messages)
	usage := contracts.ModelUsage{Status: "not_applicable"}
	started := false
	if r.DecisionMode == "model" {
		input := make([]map[string]any, 0, len(messages))
		for _, m := range messages {
			input = append(input, map[string]any{"message_id": m.MessageID, "text": conversation.Text(m), "required": conversation.Required(m)})
		}
		body, _ := json.Marshal(input)
		request, _ := json.Marshal(map[string]any{"model": r.FrozenDefinition.ProviderBinding.ModelID, "messages": []contracts.ContextMessage{{Role: "system", Content: policy.SummaryPrompt.Content}, {Role: "user", Content: string(body)}}, "temperature": 0, "response_format": map[string]string{"type": "json_object"}, "max_tokens": policy.SummaryTokens})
		usage.Status = "unknown"
		if int64(len(request))+policy.SummaryTokens+policy.ProtocolReserveTokens > policy.MaxTokens {
			e = fail("summary_budget_exceeded", "frozen source and summary output exceed context budget")
		} else {
			deadline, _ := time.Parse(time.RFC3339Nano, *pending.Reservation.DeadlineAt)
			bounded, cancel := context.WithDeadline(ctx, deadline)
			result, err := s.callModelPayload(bounded, r, request, "SummaryAdvice")
			cancel()
			started = result.Started
			usage = result.Usage
			e = err
			if e == nil {
				e = json.Unmarshal(result.Payload, &advice)
			}
		}
	}
	var previous *contracts.SummaryRecord
	if len(c.Summaries) > 0 {
		previous = &c.Summaries[len(c.Summaries)-1]
	}
	var summary contracts.SummaryRecord
	if e == nil {
		summary, e = conversation.Record(r.ID, pending.SourceVersion, *r.FrozenDefinition, messages, previous, advice, time.Now().UTC())
	}
	workErr := e
	updated, e := s.Store.Update(r.ID, func(run *Run) error {
		c := conversationState(*run)
		if c.PendingSummary == nil || c.PendingSummary.AttemptID != pending.AttemptID {
			return fail("summary_binding_mismatch", "frozen summary attempt changed")
		}
		if pending.Reservation != nil {
			outcome := "succeeded"
			if workErr != nil {
				outcome = "failed"
			}
			begin, _ := time.Parse(time.RFC3339Nano, pending.Reservation.ReservedAt)
			in := actualControlSettlement(*pending.Reservation, outcome, begin, time.Now().UTC())
			if !started {
				zero := int64(0)
				in.Outcome = "released"
				in.ElapsedMS = &zero
				in.ConfirmedNotStarted = true
			}
			ledger, _, err := governance.SettleBudget(run.FrozenDefinition.Definition.BudgetPolicy, run.Control.Audit.Budget, in)
			if err != nil {
				return err
			}
			run.Control.Audit.Budget = ledger
			syncBudgetProjection(run)
		}
		g := contracts.SummaryGenerator{Mode: run.DecisionMode, GeneratorVersion: policy.SummaryGeneratorVersion}
		if run.DecisionMode == "model" {
			g.ModelID = &run.FrozenDefinition.ProviderBinding.ModelID
			g.PromptSHA256 = &policy.SummaryPrompt.SHA256
		}
		a := contracts.SummaryAttempt{ContractVersion: "1.4.0", AttemptID: pending.AttemptID, RunID: run.ID, SourceVersion: pending.SourceVersion, DefinitionSnapshotHash: run.FrozenDefinition.SnapshotHash, Source: pending.Source, Generator: g, Status: "succeeded", ModelUsage: usage, CreatedAt: pending.StartedAt, CompletedAt: now()}
		if workErr == nil {
			ref, err := storage.Reference(run.ID, summary.SourceVersion, "SummaryRecord", summary)
			if err != nil {
				return err
			}
			a.SummaryRef = &ref
			c.Summaries = append(c.Summaries, summary)
			history(run, "summary_created", "Derived summary version recorded; original transcript unchanged")
		} else {
			code := "summary_invalid"
			if api, ok := workErr.(*APIError); ok {
				code = api.Code
			}
			a.Status = "failed"
			a.FailureCode = &code
			history(run, "summary_failed", code+": prior summary and original transcript retained")
		}
		var err error
		a.AttemptHash, err = contracts.HashObject("SummaryAttempt", a)
		if err != nil {
			return err
		}
		if err = contracts.Validate("SummaryAttempt", a); err != nil {
			return err
		}
		c.Attempts = append(c.Attempts, a)
		c.PendingSummary = nil
		return nil
	})
	// A failed derived summary is visible but does not discard a valid current
	// user request. Context construction still enforces the complete input bound.
	return updated, e
}

func recoverPendingSummary(r *Run, at time.Time) error {
	c := conversationState(*r)
	if c == nil || c.PendingSummary == nil {
		return nil
	}
	p := c.PendingSummary
	policy := r.FrozenDefinition.Definition.ContextPolicy
	g := contracts.SummaryGenerator{Mode: r.DecisionMode, GeneratorVersion: policy.SummaryGeneratorVersion}
	usage := contracts.ModelUsage{Status: "not_applicable"}
	if r.DecisionMode == "model" {
		g.ModelID = &r.FrozenDefinition.ProviderBinding.ModelID
		g.PromptSHA256 = &policy.SummaryPrompt.SHA256
		usage.Status = "unknown"
	}
	code := "summary_result_unknown"
	a := contracts.SummaryAttempt{ContractVersion: "1.4.0", AttemptID: p.AttemptID, RunID: r.ID, SourceVersion: p.SourceVersion, DefinitionSnapshotHash: r.FrozenDefinition.SnapshotHash, Source: p.Source, Generator: g, Status: "unknown", ModelUsage: usage, FailureCode: &code, CreatedAt: p.StartedAt, CompletedAt: at.UTC().Format(time.RFC3339Nano)}
	var err error
	a.AttemptHash, err = contracts.HashObject("SummaryAttempt", a)
	if err != nil {
		return err
	}
	c.Attempts = append(c.Attempts, a)
	c.PendingSummary = nil
	history(r, "summary_interrupted", "Original summary attempt outcome unknown; no automatic provider replay")
	return nil
}
