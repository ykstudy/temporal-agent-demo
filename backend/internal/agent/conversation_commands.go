package agent

import (
	"encoding/json"
	"sort"
	"strings"

	"agentstudio/internal/contracts"
	"agentstudio/internal/storage"
)

func acceptConversationCommand(r *Run, cmd Command) error {
	c := conversationState(*r)
	if c == nil {
		return nil
	}
	b, e := json.Marshal(cmd)
	if e != nil {
		return e
	}
	hash, e := contracts.HashValue(json.RawMessage(b))
	if e != nil {
		return e
	}
	if wire, ok := c.WireCommands[cmd.ID]; ok {
		if !equalContractValue(wire.Command, cmd) {
			return fail("idempotency_conflict", "wire request maps to a different command")
		}
		hash = wire.Fingerprint
	}
	if old, ok := c.Receipts[cmd.ID]; ok {
		if old.CommandFingerprint != hash {
			return fail("idempotency_conflict", "command receipt fingerprint changed")
		}
		return nil
	}
	receipt := contracts.CommandReceipt{ContractVersion: "1.4.0", SchemaVersion: 2, CommandID: cmd.ID, RunID: r.ID, CommandFingerprint: hash, Status: "accepted", ResultRefs: []contracts.ObjectReference{}, AcceptedAt: now(), UpdatedAt: now()}
	return saveConversationReceipt(c, receipt)
}
func saveConversationReceipt(c *ConversationState, receipt contracts.CommandReceipt) error {
	if e := contracts.Validate("CommandReceipt", receipt); e != nil {
		return e
	}
	c.Receipts[receipt.CommandID] = receipt
	c.ReceiptHistory = append(c.ReceiptHistory, receipt)
	return nil
}
func completeConversationCommand(r *Run, cmd Command, workErr error) error {
	c := conversationState(*r)
	if c == nil {
		return nil
	}
	receipt, ok := c.Receipts[cmd.ID]
	if !ok {
		return fail("receipt_missing", "command lacks persisted admission")
	}
	receipt.Status = "processed"
	receipt.RunRef = nil // The atomic final snapshot supplies the exact result.
	receipt.Error = nil
	receipt.UpdatedAt = now()
	receipt.ResultRefs = []contracts.ObjectReference{}
	if workErr != nil {
		code := "command_failed"
		if api, ok := workErr.(*APIError); ok {
			code = api.Code
		}
		receipt.Status = "rejected"
		receipt.Error = &contracts.Failure{Code: code, Category: failureCategory(code), Message: boundedText(workErr.Error()), EvidenceRefs: []contracts.ObjectReference{}}
	} else {
		for _, in := range r.Control.ValidationInputs {
			if in.Facts.CommandID == cmd.ID {
				ref, e := storage.Reference(r.ID, in.Decision.SourceVersion, "DecisionProposal", in.Decision)
				if e != nil {
					return e
				}
				receipt.ResultRefs = append(receipt.ResultRefs, ref)
			}
		}
	}
	return saveConversationReceipt(c, receipt)
}
func failureCategory(code string) string {
	for _, entry := range []struct {
		words    []string
		category string
	}{{[]string{"unknown", "reconciliation"}, "unknown_result"}, {[]string{"budget", "deadline"}, "budget"}, {[]string{"permission", "approval", "unapproved"}, "permission"}, {[]string{"unavailable", "interrupted", "storage", "model_failed"}, "infrastructure"}, {[]string{"invalid", "schema", "stale", "conflict"}, "input"}} {
		for _, w := range entry.words {
			if strings.Contains(code, w) {
				return entry.category
			}
		}
	}
	return "business"
}
func currentReceipts(c *ConversationState) []contracts.CommandReceipt {
	ids := make([]string, 0, len(c.Receipts))
	for id := range c.Receipts {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out := make([]contracts.CommandReceipt, 0, len(ids))
	for _, id := range ids {
		out = append(out, c.Receipts[id])
	}
	return out
}
