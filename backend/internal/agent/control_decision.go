package agent

import (
	"encoding/json"
	"fmt"
	"time"

	"agentstudio/internal/contracts"
	"agentstudio/internal/governance"
	"agentstudio/internal/storage"
)

// appendCanonicalInput happens before invoking either decider. Retrying a
// failed call reuses the same exact input envelope rather than inventing a new
// version, timestamp or message that the original call could not have seen.
func appendCanonicalInput(r *Run, cmd Command, received time.Time) (contracts.MessageEnvelope, error) {
	if r.Control == nil || received.IsZero() || cmd.RunID != r.ID || cmd.Content == "" || cmd.Action != "" {
		return contracts.MessageEnvelope{}, fail("invalid_input_message", "完整用户输入的身份或内容无效")
	}
	for _, message := range r.Control.Audit.Messages {
		if message.CommandID != nil && *message.CommandID == cmd.ID {
			if len(message.Parts) != 1 {
				return contracts.MessageEnvelope{}, fail("idempotency_conflict", "已登记输入内容不一致")
			}
			var part contracts.TextPart
			if json.Unmarshal(message.Parts[0], &part) != nil || part.Text != cmd.Content {
				return contracts.MessageEnvelope{}, fail("idempotency_conflict", "相同命令身份不可替换用户原文")
			}
			return message, nil
		}
	}
	// Text preserves the original input; the display fallback has its own
	// smaller character bound and cannot be used as the message source.
	fallback := []rune(cmd.Content)
	if len(fallback) > 4096 {
		fallback = fallback[:4096]
	}
	part := contracts.TextPart{PartID: "text", PartVersion: 1, Type: "text", Text: cmd.Content, Format: "plain", FallbackText: string(fallback)}
	partBody, err := json.Marshal(part)
	if err != nil {
		return contracts.MessageEnvelope{}, err
	}
	message := contracts.MessageEnvelope{SchemaVersion: 2, ContractVersion: r.FrozenDefinition.ContractVersion, RunID: r.ID, MessageID: cmd.ID, Sequence: int64(len(r.Control.Audit.Messages) + 1), SourceVersion: int64(r.SourceVersion), StateSequence: int64(r.StateSequence), Role: "user", CreatedAt: received.UTC().Format(time.RFC3339Nano), Modes: runModes(*r), CommandID: &cmd.ID, Parts: []contracts.Part{contracts.Part(partBody)}}
	if err = contracts.Validate("MessageEnvelope", message); err != nil {
		return message, err
	}
	r.Control.Audit.Messages = append(r.Control.Audit.Messages, message)
	r.Messages = append(r.Messages, Message{ID: cmd.ID, Role: "user", Content: cmd.Content, Kind: "text", CreatedAt: message.CreatedAt})
	return message, nil
}

// bindCanonicalDecision binds server-owned identity and observed transport
// usage. A provider can only return DecisionAdvice; it never supplies these
// fields or an ExecutionRequest. Malformed advice is rejected before binding.
func bindCanonicalDecision(r Run, commandID string, advice contracts.DecisionAdvice, usage contracts.ModelUsage, input []contracts.MessageEnvelope, ordered []contracts.ContextMessage, received time.Time) (governance.Input, contracts.ContextRecord, error) {
	var empty governance.Input
	var record contracts.ContextRecord
	if r.Control == nil || r.FrozenDefinition == nil || received.IsZero() {
		return empty, record, fail("control_binding_mismatch", "缺少完整建议来源")
	}
	if err := contracts.ValidateVersion("DecisionAdvice", r.FrozenDefinition.ContractVersion, advice); err != nil {
		return empty, record, fail("model_schema_invalid", "建议不符合冻结协议")
	}
	if err := contracts.ValidateVersion("ModelUsage", r.FrozenDefinition.ContractVersion, usage); err != nil {
		return empty, record, err
	}
	refs := make([]contracts.ObjectReference, 0, len(input))
	for _, message := range input {
		found := false
		ref, err := governance.MessageRef(message)
		if err != nil {
			return empty, record, err
		}
		for _, stored := range r.Control.Audit.Messages {
			storedRef, e := governance.MessageRef(stored)
			if e == nil && storedRef == ref {
				found = true
				break
			}
		}
		if !found {
			return empty, record, fail("input_provenance_mismatch", "建议引用未登记的输入原文")
		}
		refs = append(refs, ref)
	}
	contextHash, err := contracts.HashValue(ordered)
	if err != nil {
		return empty, record, err
	}
	decision := contracts.DecisionProposal{
		ContractVersion: r.FrozenDefinition.ContractVersion, DecisionID: newID("decision_"), RunID: r.ID, SourceVersion: int64(r.SourceVersion), StateSequence: int64(r.StateSequence),
		DefinitionSnapshotHash: r.FrozenDefinition.SnapshotHash, DecisionMode: r.DecisionMode, Advice: advice, ModelUsage: usage, InputMessageRefs: refs, InputContextHash: contextHash, CreatedAt: received.UTC().Format(time.RFC3339Nano),
	}
	decision.DecisionHash, err = contracts.HashObject("DecisionProposal", decision)
	if err != nil {
		return empty, record, err
	}
	if err = contracts.Validate("DecisionProposal", decision); err != nil {
		return empty, record, err
	}
	record = contracts.ContextRecord{ContractVersion: decision.ContractVersion, ContextID: "context_" + decision.DecisionID, RunID: r.ID, DecisionID: decision.DecisionID, SourceVersion: decision.SourceVersion, StateSequence: decision.StateSequence, DefinitionSnapshotHash: decision.DefinitionSnapshotHash, InputMessageRefs: []contracts.ContextRecordInputMessageRefsItem{}, Messages: ordered, InputContextHash: contextHash, CreatedAt: decision.CreatedAt}
	if c := conversationState(r); c != nil {
		record.SummaryRefs = []contracts.ObjectReference{}
		for _, summary := range contextSummaries(r) {
			included := false
			// These are the builder's explicit derived-material slots, before
			// the original input envelopes. A user's copied summary text cannot
			// introduce a summary reference through the original-input suffix.
			if len(ordered) < len(input)+2 {
				return empty, record, fail("summary_context_mismatch", "context authority prefix is missing")
			}
			for _, message := range ordered[2 : len(ordered)-len(input)] {
				if message.Role == "user" && message.Content == summaryContextText(summary) {
					included = true
				}
			}
			if !included {
				continue
			}
			ref, e := storage.Reference(r.ID, summary.SourceVersion, "SummaryRecord", summary)
			if e != nil {
				return empty, record, e
			}
			record.SummaryRefs = append(record.SummaryRefs, ref)
		}
	}
	for _, ref := range refs {
		record.InputMessageRefs = append(record.InputMessageRefs, contracts.ContextRecordInputMessageRefsItem(ref))
	}
	record.ContextHash, err = contracts.HashObject("ContextRecord", record)
	if err != nil {
		return empty, record, err
	}
	if err = contracts.Validate("ContextRecord", record); err != nil {
		return empty, record, err
	}
	a := r.Control.Audit
	facts := governance.Facts{RunID: r.ID, SourceVersion: int64(r.SourceVersion), StateSequence: int64(r.StateSequence), State: r.State, ActorID: localActorID, CommandID: commandID, Modes: runModes(r), FrozenDefinition: *r.FrozenDefinition, Plan: a.Plan, Proposal: a.Proposal, Approval: a.Approval, Permission: a.PermissionSnapshot, Manifest: a.Manifest, Now: received, ObservedModelUsage: &usage, AvailableTools: append([]contracts.ToolIdentity{}, r.FrozenDefinition.ToolBindings...)}
	return governance.Input{Facts: facts, Decision: decision, InputMessages: input, OrderedContext: ordered}, record, nil
}

func (s *Service) recordCanonicalDecision(r *Run, input governance.Input, record contracts.ContextRecord, validation contracts.ActionValidation) error {
	checked, err := (governance.Validator{Definitions: s.Definitions}).Validate(input)
	if err != nil {
		return err
	}
	if !equalContractValue(checked, validation) {
		return fail("decision_binding_mismatch", "提交的校验或执行请求并非当前确定性校验结果")
	}
	return appendCanonicalDecision(r, input, record, validation)
}

func appendCanonicalDecision(r *Run, input governance.Input, record contracts.ContextRecord, validation contracts.ActionValidation) error {
	if r.Control == nil || input.Decision.RunID != r.ID || input.Decision.SourceVersion != int64(r.SourceVersion) || input.Decision.StateSequence != int64(r.StateSequence) {
		return fail("stale_decision", "建议绑定的来源或进度已改变")
	}
	if validation.DecisionID != input.Decision.DecisionID || validation.RunID != r.ID || record.DecisionID != input.Decision.DecisionID || record.InputContextHash != input.Decision.InputContextHash {
		return fail("decision_binding_mismatch", "建议、上下文与校验结果未绑定同一身份")
	}
	if !hashMatches("DecisionProposal", input.Decision, input.Decision.DecisionHash) || !hashMatches("ActionValidation", validation, validation.ValidationHash) || !hashMatches("ContextRecord", record, record.ContextHash) {
		return fail("decision_binding_mismatch", "建议链结构或摘要不符")
	}
	for _, old := range r.Control.Audit.Decisions {
		if old.DecisionID == input.Decision.DecisionID {
			return fmt.Errorf("duplicate decision identity")
		}
	}
	candidate := clone(*r)
	a := &candidate.Control.Audit
	a.Decisions = append(a.Decisions, input.Decision)
	a.Contexts = append(a.Contexts, record)
	a.Validations = append(a.Validations, validation)
	if validation.ExecutionRequest != nil {
		a.Executions = append(a.Executions, *validation.ExecutionRequest)
	}
	candidate.Control.ValidationInputs = append(candidate.Control.ValidationInputs, input)
	if err := verifyControlChain(candidate); err != nil {
		return err
	}
	*r = candidate
	return nil
}
