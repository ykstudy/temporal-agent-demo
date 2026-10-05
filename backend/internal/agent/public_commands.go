package agent

import (
	"agentstudio/internal/contracts"
	"encoding/json"
)

// Private admission provenance. Public DTOs are generated from the release;
// these records only bind the complete received wire request to its command.
type publicWireCommand struct {
	Schema      string          `json:"schema"`
	Body        json.RawMessage `json:"body"`
	Fingerprint string          `json:"fingerprint"`
	Command     Command         `json:"command"`
}

func initialCreateCommand(opts CreateOptions, id, content string) Command {
	if opts.wire != nil {
		return opts.wire.Command
	}
	return Command{ID: "initial_" + digest(id)[:20], RunID: id, Content: content}
}

func parseWire(schema string, body []byte, id string) (publicWireCommand, *contracts.ObjectReference, *contracts.ObjectReference, error) {
	w := publicWireCommand{Schema: schema}
	var target, proposal *contracts.ObjectReference
	switch schema {
	case "CreateRunRequest":
		var v contracts.CreateRunRequest
		if e := contracts.DecodeVersion(schema, "1.4.0", body, &v); e != nil {
			return w, nil, nil, fail("invalid_request", "请求不符合已发布的完整 v2 Schema")
		}
		if v.RunID != id {
			return w, nil, nil, fail("invalid_identity", "路径与创建身份不同")
		}
		w.Command = Command{ID: v.CommandID, RunID: id, Content: v.InitialContent}
	case "MessageRequest":
		var v contracts.MessageRequest
		if e := contracts.DecodeVersion(schema, "1.4.0", body, &v); e != nil {
			return w, nil, nil, fail("invalid_request", "消息不符合已发布的完整 v2 Schema")
		}
		t := contracts.ObjectReference(v.Target)
		target = &t
		w.Command = Command{ID: v.CommandID, RunID: id, Content: v.Content}
	case "ControlRequest":
		var v contracts.ControlRequest
		if e := contracts.DecodeVersion(schema, "1.4.0", body, &v); e != nil {
			return w, nil, nil, fail("invalid_request", "动作不符合已发布的完整 v2 Schema")
		}
		t := contracts.ObjectReference(v.Target)
		target = &t
		w.Command = Command{ID: v.CommandID, RunID: id, Action: v.Action}
		if v.RequiredProposalRef != nil {
			p := contracts.ObjectReference(*v.RequiredProposalRef)
			proposal = &p
			w.Command.ProposalID = p.ObjectID
			w.Command.ProposalVersion = int(p.ObjectVersion)
			w.Command.SourceVersion = int(p.SourceVersion)
			w.Command.SnapshotHash = p.Hash
		}
	default:
		return w, nil, nil, fail("invalid_request", "未知请求类型")
	}
	if target != nil && (target.ObjectType != "run" || target.ObjectID != id) {
		return w, nil, nil, fail("invalid_reference", "请求目标不是当前 Run")
	}
	b, e := contracts.Canonicalize(body)
	if e != nil {
		return w, nil, nil, e
	}
	w.Body = b
	w.Fingerprint = contracts.BytesHash(b)
	return w, target, proposal, nil
}

func validateWireCommand(r Run, cmd Command, admission bool) error {
	c := conversationState(r)
	if c == nil {
		return nil
	}
	w, ok := c.WireCommands[cmd.ID]
	if !ok {
		return nil
	}
	parsed, target, proposal, e := parseWire(w.Schema, w.Body, r.ID)
	if e != nil || parsed.Fingerprint != w.Fingerprint || !equalContractValue(parsed.Command, cmd) || !equalContractValue(w.Command, cmd) {
		return fail("wire_binding_mismatch", "原请求指纹或内部命令绑定损坏")
	}
	refs := map[contracts.ObjectReference]bool{}
	for _, ref := range r.Control.ObjectRefs {
		refs[ref] = true
	}
	if target != nil && (!refs[*target] || admission && target.SourceVersion != int64(r.SourceVersion)) {
		return fail("stale_target", "目标快照不存在或授权来源已变化")
	}
	if proposal != nil {
		if !refs[*proposal] {
			return fail("stale_proposal", "提案历史引用不存在")
		}
		p := r.Control.Audit.Proposal
		if admission && (p == nil || proposal.ObjectID != p.ProposalID || proposal.ObjectVersion != p.ProposalVersion || proposal.SourceVersion != int64(r.SourceVersion) || proposal.Hash != p.SnapshotHash) {
			return fail("stale_proposal", "此卡片已不是当前可操作方案")
		}
	}
	return nil
}

func (s *Service) bindWire(w publicWireCommand) error {
	if s.containsSecret(string(w.Body)) {
		return fail("secret_input", "请求包含服务端凭据，拒绝持久化")
	}
	if r, err := s.Store.Get(w.Command.RunID); err == nil {
		if c := conversationState(r); c != nil {
			if old, ok := c.WireCommands[w.Command.ID]; ok {
				if old.Schema != w.Schema || old.Fingerprint != w.Fingerprint {
					return fail("idempotency_conflict", "相同命令身份已绑定不同完整请求")
				}
				return validateWireCommand(r, w.Command, false)
			}
		}
	}
	_, e := s.Store.Update(w.Command.RunID, func(r *Run) error {
		c := conversationState(*r)
		if c == nil || c.LatestSnapshot == nil {
			return fail("legacy_contract_incomplete", "旧 Run 缺少完整原始来源")
		}
		if old, ok := c.WireCommands[w.Command.ID]; ok {
			if old.Schema != w.Schema || old.Fingerprint != w.Fingerprint {
				return fail("idempotency_conflict", "相同命令身份已绑定不同完整请求")
			}
			return nil
		}
		if _, ok := c.Receipts[w.Command.ID]; ok {
			return fail("idempotency_conflict", "命令身份已由另一个入口使用")
		}
		if len(c.WireCommands) >= r.Definition.Budget.MaxCommands {
			return fail("budget_exceeded", "Run 命令身份预算已耗尽")
		}
		if c.WireCommands == nil {
			c.WireCommands = map[string]publicWireCommand{}
		}
		c.WireCommands[w.Command.ID] = w
		return validateWireCommand(*r, w.Command, true)
	})
	return e
}

func (s *Store) PublicReceipt(id, command string) (contracts.CommandReceipt, error) {
	r, e := s.Get(id)
	if e != nil {
		return contracts.CommandReceipt{}, e
	}
	c := conversationState(r)
	if c == nil || c.LatestSnapshot == nil {
		return contracts.CommandReceipt{}, fail("legacy_contract_incomplete", "旧 Run 缺少完整回执来源")
	}
	receipt, ok := c.Receipts[command]
	if !ok {
		return receipt, ErrNotFound
	}
	if e = contracts.Validate("CommandReceipt", receipt); e != nil {
		return receipt, e
	}
	if receipt.RunRef == nil {
		return receipt, fail("object_corrupt", "命令回执未绑定提交快照")
	}
	_, e = s.CanonicalObject(id, contracts.ObjectReference(*receipt.RunRef))
	return receipt, e
}
