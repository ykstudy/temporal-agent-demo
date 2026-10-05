package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"agentstudio/internal/contracts"
	"agentstudio/internal/conversation"
	"agentstudio/internal/storage"
)

// Only newly created 1.4 Runs receive this state. Absence means unavailable
// provenance, never an invitation to convert old display messages on read.
type ConversationState struct {
	Runtime        *RuntimeState                       `json:"runtime,omitempty"`
	WireCommands   map[string]publicWireCommand        `json:"wire_commands,omitempty"`
	LatestSnapshot *contracts.ObjectReference          `json:"latest_snapshot,omitempty"`
	MessageCount   int                                 `json:"message_count"`
	Summaries      []contracts.SummaryRecord           `json:"summaries"`
	Attempts       []contracts.SummaryAttempt          `json:"attempts"`
	History        []contracts.HistoryEvent            `json:"history"`
	Receipts       map[string]contracts.CommandReceipt `json:"receipts"`
	ReceiptHistory []contracts.CommandReceipt          `json:"receipt_history"`
	PendingSummary *summaryPending                     `json:"pending_summary,omitempty"`
}

type summaryPending struct {
	AttemptID     string                       `json:"attempt_id"`
	Source        contracts.SummarySource      `json:"source"`
	SourceVersion int64                        `json:"source_version"`
	StartedAt     string                       `json:"started_at"`
	Reservation   *contracts.BudgetReservation `json:"reservation"`
}

func newConversation() *ConversationState {
	return &ConversationState{Runtime: newRuntimeState(), Summaries: []contracts.SummaryRecord{}, Attempts: []contracts.SummaryAttempt{}, History: []contracts.HistoryEvent{}, Receipts: map[string]contracts.CommandReceipt{}, ReceiptHistory: []contracts.CommandReceipt{}}
}
func conversationState(r Run) *ConversationState {
	if r.Control == nil {
		return nil
	}
	return r.Control.Conversation
}
func boundedText(text string) string {
	r := []rune(text)
	if len(r) > 4096 {
		r = r[:4096]
	}
	return string(r)
}
func rawPart(value any) contracts.Part { b, _ := json.Marshal(value); return contracts.Part(b) }
func componentPart(id, kind, fallback string, payload any) contracts.Part {
	return rawPart(map[string]any{"part_id": id, "part_version": 1, "type": "component", "fallback_text": boundedText(fallback), "component_id": id, "component_version": 1, "component_type": kind, "component_schema": "urn:agentstudio:component:" + kind + ":1.0.0", "payload": payload})
}

func prepareConversation(r *Run) error {
	c := conversationState(*r)
	if c == nil {
		return nil
	}
	if r.FrozenDefinition.ContractVersion != "1.4.0" {
		return fail("conversation_version_mismatch", "conversation requires its frozen publication")
	}
	if c.MessageCount > len(r.Control.Audit.Messages) || len(c.History) > len(r.History) {
		return fail("conversation_truncated", "conversation history cannot shrink")
	}
	ids := map[string]bool{}
	for _, m := range r.Control.Audit.Messages {
		ids[m.MessageID] = true
	}
	for _, legacy := range r.Messages {
		if ids[legacy.ID] {
			continue
		}
		parts := []contracts.Part{rawPart(contracts.TextPart{PartID: "text_" + legacy.ID, PartVersion: 1, Type: "text", Text: legacy.Content, Format: "markdown", FallbackText: boundedText(legacy.Content)})}
		if legacy.Kind == "proposal" && legacy.ProposalID != "" {
			found := false
			for _, p := range r.Control.Audit.ProposalHistory {
				if p.ProposalID == legacy.ProposalID {
					ref, err := storage.Reference(r.ID, p.SourceVersion, "Proposal", p)
					if err != nil {
						return err
					}
					paths := []string{}
					for _, f := range p.Changes {
						paths = append(paths, f.Path)
					}
					parts = append(parts, componentPart("proposal_"+legacy.ID, "proposal_diff", legacy.Content, map[string]any{"proposal_ref": ref, "summary": boundedText(legacy.Content), "changed_paths": paths}))
					found = true
					break
				}
			}
			if !found {
				return fail("missing_message_proposal", "message has no exact original proposal")
			}
		} else if legacy.Role == "assistant" {
			parts = append(parts, componentPart("progress_"+legacy.ID, "progress", r.State, map[string]any{"state": r.State, "summary": boundedText(legacy.Content)}))
		}
		m := contracts.MessageEnvelope{ContractVersion: "1.4.0", SchemaVersion: 2, RunID: r.ID, MessageID: legacy.ID, Sequence: int64(len(r.Control.Audit.Messages) + 1), SourceVersion: int64(r.SourceVersion), StateSequence: int64(r.StateSequence), Role: legacy.Role, CreatedAt: legacy.CreatedAt, Modes: runModes(*r), Parts: parts}
		r.Control.Audit.Messages = append(r.Control.Audit.Messages, m)
		ids[legacy.ID] = true
	}
	// Newly staged envelopes acquire the sequence of this actual commit before
	// any immutable hash or original transcript byte is published.
	for i := c.MessageCount; i < len(r.Control.Audit.Messages); i++ {
		r.Control.Audit.Messages[i].StateSequence = int64(r.StateSequence)
	}
	for _, h := range r.History[len(c.History):] {
		c.History = append(c.History, contracts.HistoryEvent{EventID: h.ID, Action: h.Action, SourceVersion: int64(r.SourceVersion), StateSequence: int64(r.StateSequence), ObjectRefs: []contracts.ObjectReference{}, Detail: boundedText(h.Detail), CreatedAt: h.CreatedAt})
	}
	c.MessageCount = len(r.Control.Audit.Messages)
	return validateConversation(*r)
}

func validateConversation(r Run) error {
	c := conversationState(r)
	if c == nil {
		return nil
	}
	refs := map[contracts.ObjectReference]bool{}
	for _, ref := range r.Control.ObjectRefs {
		refs[ref] = true
	}
	if err := visitCanonicalBodies(r, func(schema string, source int64, body any) error {
		ref, e := storage.Reference(r.ID, source, schema, body)
		if e == nil {
			refs[ref] = true
		}
		return e
	}); err != nil {
		return err
	}
	ids := map[string]bool{}
	components := map[string]string{}
	for i, m := range r.Control.Audit.Messages {
		if m.Sequence != int64(i+1) || m.RunID != r.ID || ids[m.MessageID] || m.SourceVersion > int64(r.SourceVersion) || m.StateSequence > int64(r.StateSequence) {
			return fail("message_binding_mismatch", "message identity or authoritative sequence invalid")
		}
		ids[m.MessageID] = true
		if err := conversation.ValidateMessage(m, func(ref contracts.ObjectReference) bool { return refs[ref] }); err != nil {
			return err
		}
		for _, p := range m.Parts {
			var v map[string]any
			_ = json.Unmarshal(p, &v)
			if v["type"] != "component" {
				continue
			}
			key := fmt.Sprint(v["component_id"], "/", v["component_version"])
			h := contracts.BytesHash(p)
			if old := components[key]; old != "" && old != h {
				return fail("component_conflict", "component identity changed content")
			}
			components[key] = h
		}
	}
	for i, s := range c.Summaries {
		if s.Generator.Mode != r.DecisionMode || s.Generator.GeneratorVersion != r.FrozenDefinition.Definition.ContextPolicy.SummaryGeneratorVersion {
			return fail("summary_generator_mismatch", "summary generator differs from frozen mode")
		}
		if r.DecisionMode == "model" && (s.Generator.ModelID == nil || s.Generator.PromptSHA256 == nil || *s.Generator.ModelID != r.FrozenDefinition.ProviderBinding.ModelID || *s.Generator.PromptSHA256 != r.FrozenDefinition.Definition.ContextPolicy.SummaryPrompt.SHA256) {
			return fail("summary_generator_mismatch", "summary provider or frozen prompt changed")
		}
		if s.RunID != r.ID || s.DefinitionSnapshotHash != r.FrozenDefinition.SnapshotHash || s.SummaryVersion != int64(i+1) || s.Source.FirstSequence < 1 || s.Source.LastSequence > int64(len(r.Control.Audit.Messages)) || s.Source.FirstSequence > s.Source.LastSequence || !hashMatches("SummaryRecord", s, s.SummaryHash) || s.ContentSHA256 != contracts.BytesHash([]byte(s.Content)) {
			return fail("summary_binding_mismatch", "summary identity, hash or source invalid")
		}
		source, e := conversation.Source(r.Control.Audit.Messages[s.Source.FirstSequence-1 : s.Source.LastSequence])
		if e != nil || !equalContractValue(source, s.Source) {
			return fail("summary_source_mismatch", "summary original range hash changed")
		}
		if i == 0 {
			if s.PreviousSummaryRef != nil {
				return fail("summary_chain_mismatch", "first summary has predecessor")
			}
		} else {
			prev, e := storage.Reference(r.ID, c.Summaries[i-1].SourceVersion, "SummaryRecord", c.Summaries[i-1])
			if e != nil || s.PreviousSummaryRef == nil || *s.PreviousSummaryRef != prev || s.Source.FirstSequence != c.Summaries[i-1].Source.LastSequence+1 {
				return fail("summary_chain_mismatch", "summary version or range has gap")
			}
		}
	}
	for _, a := range c.Attempts {
		if !hashMatches("SummaryAttempt", a, a.AttemptHash) || a.RunID != r.ID || a.DefinitionSnapshotHash != r.FrozenDefinition.SnapshotHash {
			return fail("summary_attempt_mismatch", "summary attempt provenance invalid")
		}
		if a.Generator.Mode != r.DecisionMode || a.Source.FirstSequence < 1 || a.Source.LastSequence > int64(len(r.Control.Audit.Messages)) || a.Source.LastSequence < a.Source.FirstSequence {
			return fail("summary_attempt_mismatch", "attempt mode or source range invalid")
		}
		source, e := conversation.Source(r.Control.Audit.Messages[a.Source.FirstSequence-1 : a.Source.LastSequence])
		if e != nil || !equalContractValue(source, a.Source) {
			return fail("summary_source_mismatch", "attempt original range changed")
		}
		if a.Status == "succeeded" {
			if a.SummaryRef == nil || !refs[*a.SummaryRef] {
				return fail("summary_attempt_mismatch", "successful attempt has no exact summary")
			}
		}
	}
	for _, context := range r.Control.Audit.Contexts {
		for _, ref := range context.SummaryRefs {
			found := false
			for _, summary := range c.Summaries {
				known, e := storage.Reference(r.ID, summary.SourceVersion, "SummaryRecord", summary)
				if e == nil && known == ref && summary.SourceVersion <= context.SourceVersion && chainBefore(summary.CreatedAt, context.CreatedAt, false) {
					for _, message := range context.Messages {
						if message.Role == "user" && message.Content == summaryContextText(summary) {
							found = true
						}
					}
				}
			}
			if !found {
				return fail("summary_context_mismatch", "context does not bind the actual historical summary")
			}
		}
	}
	for _, h := range c.History {
		if e := contracts.ValidateVersion("HistoryEvent", "1.4.0", h); e != nil {
			return e
		}
		if h.SourceVersion > int64(r.SourceVersion) || h.StateSequence > int64(r.StateSequence) {
			return fail("history_binding_mismatch", "history newer than snapshot")
		}
	}
	for _, receipt := range c.ReceiptHistory {
		if e := contracts.Validate("CommandReceipt", receipt); e != nil {
			return e
		}
		if receipt.RunID != r.ID {
			return fail("receipt_binding_mismatch", "foreign command receipt")
		}
		if receipt.RunRef != nil {
			ref := contracts.ObjectReference(*receipt.RunRef)
			if ref.ObjectID != r.ID || ref.ObjectType != "run" || !refs[ref] {
				return fail("receipt_binding_mismatch", "receipt does not reference its exact archived Run snapshot")
			}
		}
	}
	for id, wire := range c.WireCommands {
		if id != wire.Command.ID {
			return fail("wire_binding_mismatch", "wire command identity changed")
		}
		if err := validateWireCommand(r, wire.Command, false); err != nil {
			return err
		}
		if receipt, ok := c.Receipts[id]; ok && receipt.CommandFingerprint != wire.Fingerprint {
			return fail("wire_binding_mismatch", "receipt fingerprint differs from complete wire request")
		}
	}
	return nil
}

func (s *Store) transcriptPath(id string) string {
	return filepath.Join(s.dir, "runs", id+".envelopes.jsonl")
}
func (s *Store) recoverConversationTransactions() error {
	paths, err := filepath.Glob(filepath.Join(s.dir, "runs", ".pending-*.json"))
	if err != nil {
		return err
	}
	for _, p := range paths {
		b, e := os.ReadFile(p)
		if e != nil {
			return e
		}
		if _, e = contracts.ParseJSON(b); e != nil {
			return e
		}
		var d diskRun
		if e = json.Unmarshal(b, &d); e != nil {
			return e
		}
		r := d.value()
		if conversationState(r) == nil || !safeID.MatchString(r.ID) || filepath.Base(p) != ".pending-"+r.ID+".json" {
			return fail("invalid_pending_snapshot", "unrecognized pending conversation transaction")
		}
		if e = s.verifyCanonical(r); e != nil {
			return e
		}
		if oldBody, readErr := os.ReadFile(filepath.Join(s.dir, "runs", r.ID+".json")); readErr == nil {
			var old diskRun
			if json.Unmarshal(oldBody, &old) != nil || old.StateSequence+1 != r.StateSequence || old.CreateFingerprint != r.CreateFingerprint {
				return fail("pending_snapshot_conflict", "pending transaction does not extend the committed snapshot")
			}
		} else if !os.IsNotExist(readErr) {
			return readErr
		}
		if e = s.commitConversationLogs(r); e != nil {
			return e
		}
		if e = os.Rename(p, filepath.Join(s.dir, "runs", r.ID+".json")); e != nil {
			return e
		}
		if e = syncDirectory(filepath.Dir(p)); e != nil {
			return e
		}
	}
	return nil
}

func (s *Store) ConversationAudit(id string) (contracts.ConversationAuditBundle, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.runs[id]
	if !ok {
		return contracts.ConversationAuditBundle{}, ErrNotFound
	}
	c := conversationState(r)
	if c == nil {
		return contracts.ConversationAuditBundle{}, fail("legacy_contract_incomplete", "original conversation provenance unavailable")
	}
	if e := validateConversation(r); e != nil {
		return contracts.ConversationAuditBundle{}, e
	}
	b, e := storage.ReadTranscript(s.transcriptPath(id), r.Control.Audit.Messages)
	if e != nil {
		return contracts.ConversationAuditBundle{}, e
	}
	a := contracts.ConversationAuditBundle{ContractVersion: "1.4.0", SchemaVersion: 2, RunID: id, SourceVersion: int64(r.SourceVersion), StateSequence: int64(r.StateSequence), DefinitionSnapshotHash: r.FrozenDefinition.SnapshotHash, Messages: r.Control.Audit.Messages, Summaries: c.Summaries, SummaryAttempts: c.Attempts, CommandReceipts: currentReceipts(c), History: c.History, TranscriptSHA256: contracts.BytesHash(b)}
	a.AuditHash, e = contracts.HashObject("ConversationAuditBundle", a)
	if e != nil {
		return a, e
	}
	return a, contracts.Validate("ConversationAuditBundle", a)
}
func (s *Store) ConversationTranscript(id, first, last string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.runs[id]
	if !ok {
		return nil, ErrNotFound
	}
	if conversationState(r) == nil {
		return nil, fail("legacy_contract_incomplete", "original conversation provenance unavailable")
	}
	if _, e := storage.ReadTranscript(s.transcriptPath(id), r.Control.Audit.Messages); e != nil {
		return nil, e
	}
	start, end := 1, len(r.Control.Audit.Messages)
	var e error
	if first != "" {
		start, e = strconv.Atoi(first)
		if e != nil {
			return nil, fail("invalid_range", "invalid first sequence")
		}
	}
	if last != "" {
		end, e = strconv.Atoi(last)
		if e != nil {
			return nil, fail("invalid_range", "invalid last sequence")
		}
	}
	if start < 1 || end < start || end > len(r.Control.Audit.Messages) {
		return nil, fail("invalid_range", "range is not committed and contiguous")
	}
	return storage.TranscriptBytes(r.Control.Audit.Messages[start-1 : end])
}
