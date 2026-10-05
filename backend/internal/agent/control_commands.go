package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"agentstudio/internal/contracts"
	"agentstudio/internal/conversation"
	"agentstudio/internal/governance"
)

// processControlCommand is called under the same per-Run command lock as the
// legacy processor. Budget reservations and original text precede deciders;
// the final command result and its settlement share a snapshot transaction.
func (s *Service) processControlCommand(ctx context.Context, cmd Command) (CommandResult, error) {
	r, err := s.bindCommand(cmd)
	if err != nil {
		return CommandResult{}, err
	}
	started := time.Now().UTC()
	var reservation contracts.BudgetReservation
	r, err = s.Store.Update(r.ID, func(run *Run) error {
		if err := s.verifyFrozenDefinition(*run); err != nil {
			return err
		}
		var e error
		run.Control.Audit.Budget, reservation, e = governance.ReserveBudget(run.FrozenDefinition.Definition.BudgetPolicy, run.Control.Audit.Budget, governance.ReservationInput{SourceVersion: int64(run.SourceVersion), CommandID: cmd.ID, AttemptID: newID("command_attempt_"), Kind: "command", ScopeID: cmd.ID, Now: started})
		if e != nil {
			return e
		}
		if cmd.Action == "" {
			if strings.TrimSpace(cmd.Content) == "" || len(cmd.Content) > 16000 {
				return fail("invalid_message", "消息须为1–16000字节")
			}
			if _, e = appendCanonicalInput(run, cmd, started); e != nil {
				return e
			}
		}
		if c := conversationState(*run); c != nil {
			receipt := c.Receipts[cmd.ID]
			if receipt.Status == "rejected" {
				receipt.Status = "accepted"
				receipt.RunRef = nil
				receipt.Error = nil
				receipt.UpdatedAt = now()
				receipt.ResultRefs = []contracts.ObjectReference{}
				if e = saveConversationReceipt(c, receipt); e != nil {
					return e
				}
			}
		}
		syncBudgetProjection(run)
		return nil
	})
	if err != nil {
		// A refusal before reservation must close an admitted receipt. No
		// attempt was consumed and a failed storage commit cannot become success.
		refusal := err
		updated, closeErr := s.Store.Update(r.ID, func(run *Run) error {
			c := conversationState(*run)
			if c == nil {
				return nil
			}
			receipt, ok := c.Receipts[cmd.ID]
			if !ok || receipt.Status == "processed" || receipt.Status == "result_unknown" {
				return nil
			}
			run.Error = "command_admission_failed: " + s.redact(refusal.Error())
			return completeConversationCommand(run, cmd, refusal)
		})
		if closeErr != nil {
			return CommandResult{Run: r}, errors.Join(refusal, closeErr)
		}
		return CommandResult{Run: updated}, refusal
	}
	var result CommandResult
	var workErr error
	if cmd.Action == "" {
		r, workErr = s.summarizeConversation(ctx, r, cmd)
	}
	if workErr == nil {
		result, workErr = s.controlCommandBody(ctx, r, cmd, reservation)
	}
	if workErr == nil {
		return result, nil
	}
	// A failed command did run. Never release its charge, even if no external
	// write was allowed. A commit failure leaves a pending reservation which
	// startup reconciles as unknown, never as a free retry.
	updated, settleErr := s.Store.Update(r.ID, func(run *Run) error {
		if e := settleControlInRun(run, reservation, "failed", time.Now().UTC()); e != nil {
			return e
		}
		var api *APIError
		message := "控制命令失败；保留原输入与已登记事实"
		if errors.As(workErr, &api) {
			message = api.Code + ": " + s.redact(api.Message)
		}
		run.Error = message
		history(run, "command_failed", message)
		return completeConversationCommand(run, cmd, workErr)
	})
	if settleErr != nil {
		return result, errors.Join(workErr, settleErr)
	}
	result.Run = updated
	return result, workErr
}

func actualControlSettlement(reservation contracts.BudgetReservation, outcome string, started, completed time.Time) governance.SettlementInput {
	elapsed := completed.Sub(started).Milliseconds()
	if elapsed < 0 {
		elapsed = 0
	}
	return governance.SettlementInput{ReservationID: reservation.ReservationID, AttemptID: reservation.AttemptID, Outcome: outcome, MeasurementSource: "actual", ElapsedMS: &elapsed, Now: completed}
}

func settleControlInRun(r *Run, reservation contracts.BudgetReservation, outcome string, completed time.Time) error {
	started, err := time.Parse(time.RFC3339Nano, reservation.ReservedAt)
	if err != nil {
		return err
	}
	ledger, _, err := governance.SettleBudget(r.FrozenDefinition.Definition.BudgetPolicy, r.Control.Audit.Budget, actualControlSettlement(reservation, outcome, started, completed))
	if err != nil {
		return err
	}
	r.Control.Audit.Budget = ledger
	syncBudgetProjection(r)
	return nil
}

func finishControlCommand(r *Run, cmd Command, reservation contracts.BudgetReservation) error {
	if err := settleControlInRun(r, reservation, "succeeded", time.Now().UTC()); err != nil {
		return err
	}
	payload, err := json.Marshal(cmd)
	if err != nil {
		return err
	}
	r.Processed[cmd.ID] = string(payload)
	return completeConversationCommand(r, cmd, nil)
}

func (s *Service) controlCommandBody(ctx context.Context, r Run, cmd Command, commandBudget contracts.BudgetReservation) (CommandResult, error) {
	if cmd.Action == "" && progressQuery(cmd.Content) {
		updated, err := s.Store.Update(r.ID, func(run *Run) error {
			addMessage(run, "assistant", fmt.Sprintf("当前阶段：%s；%d个产物；执行/决策/交付：%s/%s/%s。", run.State, len(run.Artifacts), run.ExecutionMode, run.DecisionMode, run.DeliveryMode), "text", "")
			return finishControlCommand(run, cmd, commandBudget)
		})
		return CommandResult{Run: updated}, err
	}
	if cmd.Action == "retry" {
		if !terminal(r.State) {
			return CommandResult{Run: r}, fail("invalid_state", "重做仅适用于终止Run")
		}
		// A new identity deliberately receives a new definition and approval;
		// the old command's budget/history are retained on the terminal Run.
		next, err := s.createControlRetry(ctx, r, cmd)
		if err != nil {
			return CommandResult{Run: r}, err
		}
		_, err = s.Store.Update(r.ID, func(run *Run) error {
			history(run, "retried", next.ID)
			if err := finishControlCommand(run, cmd, commandBudget); err != nil {
				return err
			}
			if c := conversationState(*run); c != nil {
				n := conversationState(next)
				if n == nil || n.LatestSnapshot == nil || next.RetryOf != run.ID {
					return fail("retry_binding_mismatch", "new Run lacks its original retry relationship")
				}
				receipt := c.Receipts[cmd.ID]
				receipt.ResultRefs = append(receipt.ResultRefs, *n.LatestSnapshot)
				c.Receipts[cmd.ID] = receipt
				c.ReceiptHistory[len(c.ReceiptHistory)-1] = receipt
			}
			return nil
		})
		return CommandResult{Run: next}, err
	}
	if cmd.Action == "reconcile" {
		return s.controlReconcileCommand(ctx, r, cmd, commandBudget)
	}
	if terminal(r.State) {
		return CommandResult{Run: r}, fail("terminal_run", "Run已终止，请重做创建关联新Run")
	}
	if cmd.Action == "revise" && r.State == "IMPLEMENT" && r.Control.Audit.Approval != nil && r.Control.Audit.Approval.StartedAt == nil {
		return s.reissueExpiredControlPlan(r, cmd, commandBudget)
	}
	if cmd.Action == "revise" && (r.State == "VERIFY" || r.State == "BLOCKED" && r.ResumeTo == "VERIFY") {
		p := r.Control.Audit.Proposal
		if p == nil || cmd.ProposalID != p.ProposalID || int64(cmd.ProposalVersion) != p.ProposalVersion || int64(cmd.SourceVersion) != p.SourceVersion || p.SourceVersion != int64(r.SourceVersion) || cmd.SnapshotHash != p.SnapshotHash {
			return CommandResult{Run: r}, fail("stale_proposal", "验证修订未绑定原批准提案的准确身份、版本和摘要")
		}
		hasCandidate := false
		for _, candidate := range r.Control.Audit.VerificationCandidates {
			if candidate.SourceVersion == int64(r.SourceVersion) && r.Control.Audit.Approval != nil && candidate.ApprovalID == r.Control.Audit.Approval.ApprovalID {
				hasCandidate = true
			}
		}
		if !hasCandidate {
			return s.closeControlVerificationPlan(r, cmd, commandBudget)
		}
		updated, err := s.proposeControlCandidate(ctx, r.ID, func(run *Run) error {
			return finishControlCommand(run, cmd, commandBudget)
		})
		return CommandResult{Run: updated}, err
	}
	if cmd.Action != "" {
		return s.controlExplicitCommand(r, cmd, commandBudget)
	}
	if r.ActiveStep != "" || r.AcceptedProposal != "" || !chainContains([]string{"INTAKE", "CLARIFY", "PLAN"}, r.State) {
		return CommandResult{Run: r}, fail("run_busy", "需要在明确的规划或验证修订边界修改方案")
	}
	return s.controlPlanningCommand(ctx, r, cmd, commandBudget)
}

func (s *Service) reissueExpiredControlPlan(r Run, cmd Command, budget contracts.BudgetReservation) (CommandResult, error) {
	updated, err := s.Store.Update(r.ID, func(run *Run) error {
		a := &run.Control.Audit
		p, approval := a.Proposal, a.Approval
		if run.State != "IMPLEMENT" || run.ActiveStep != "" || run.PauseRequested || run.CancelRequested || p == nil || approval == nil || approval.StartedAt != nil || p.ProposalID != cmd.ProposalID || p.ProposalVersion != int64(cmd.ProposalVersion) || p.SourceVersion != int64(cmd.SourceVersion) || p.SourceVersion != int64(run.SourceVersion) || p.SnapshotHash != cmd.SnapshotHash {
			return fail("revision_not_allowed", "期限重提必须绑定未开始的准确原批准")
		}
		if e := verifyControlChain(*run); e != nil {
			return e
		}
		expiry, e := time.Parse(time.RFC3339Nano, approval.ExpiresAt)
		if e != nil || time.Now().UTC().Before(expiry) {
			return fail("revision_not_allowed", "原批准尚未到期")
		}
		for _, op := range a.Operations {
			if op.Status == "sent" || op.Status == "unknown" {
				return fail("reconciliation_required", "存在未知操作，不能期限重提")
			}
		}
		files := map[string]string{}
		for _, change := range p.Changes {
			if change.After != nil {
				files[change.Path] = *change.After
			}
		}
		plan := *a.Plan
		if e = s.makeCanonicalProposal(run, plan, files, time.Now().UTC()); e != nil {
			return e
		}
		run.State, run.ResumeTo, run.Error = "PLAN", "", ""
		history(run, "expired_reproposal", "同一完整计划签发新提案和服务端期限；原批准及版本保留")
		return finishControlCommand(run, cmd, budget)
	})
	return CommandResult{Run: updated}, err
}

// Explicitly closing a known failed plan never grants a repair action. The
// next original message must pass the ordinary complete planning protocol.
func (s *Service) closeControlVerificationPlan(r Run, cmd Command, budget contracts.BudgetReservation) (CommandResult, error) {
	updated, err := s.Store.Update(r.ID, func(run *Run) error {
		if run.ActiveStep != "" || run.PauseRequested || run.CancelRequested || !chainContains([]string{"VERIFY", "BLOCKED"}, run.State) || run.State == "BLOCKED" && run.ResumeTo != "VERIFY" || run.Control.Audit.Proposal == nil || run.Control.Audit.Proposal.SnapshotHash != cmd.SnapshotHash || run.SourceVersion != cmd.SourceVersion {
			return fail("revision_not_allowed", "验证修订边界已改变")
		}
		if err := s.recheckControlApproval(*run, time.Now().UTC()); err != nil {
			return err
		}
		if err := s.controlCandidateApprovedFiles(*run); err != nil {
			return err
		}
		found := false
		for i := len(run.Control.Audit.Executions) - 1; i >= 0; i-- {
			x := run.Control.Audit.Executions[i]
			kind, _, e := scheduleAction(x.Action)
			if e == nil && kind == "verify" && x.SourceVersion == int64(run.SourceVersion) {
				if _, e = controlRetryEvidence(*run, x); e != nil {
					return e
				}
				found = true
				break
			}
		}
		if !found {
			return fail("revision_not_allowed", "缺少当前准确已知失败的verify请求")
		}
		run.Control.Audit.Proposal.Status = "revision_requested"
		run.Proposal.Status = "revision_requested"
		run.Control.Audit.Approval = nil
		run.Approval = nil
		run.AcceptedProposal = ""
		run.State, run.ResumeTo, run.Error = "CLARIFY", "", ""
		history(run, "verification_replan", "关闭原批准；旧工件和执行历史保留，后续参数重新规划并确认")
		return finishControlCommand(run, cmd, budget)
	})
	return CommandResult{Run: updated}, err
}

// A reconcile command chooses the original outstanding request itself. It
// cannot carry a new target, revive a terminal Run or cause local dispatch.
func (s *Service) controlReconcileCommand(ctx context.Context, r Run, cmd Command, budget contracts.BudgetReservation) (CommandResult, error) {
	if !contracts.DeliveryContract(r.FrozenDefinition.ContractVersion) || cmd.Content != "" || cmd.ProposalID != "" || cmd.ProposalVersion != 0 || cmd.SourceVersion != 0 || cmd.SnapshotHash != "" {
		return CommandResult{Run: r}, fail("invalid_action", "reconcile仅接受支持契约的Run及稳定命令身份，不接受目标或批准参数")
	}
	if r.ActiveStep != "" {
		return CommandResult{Run: r}, fail("run_busy", "活跃执行尚未到安全边界，不能核实或结算其尝试")
	}
	next, err := NextControlAction(r, r.Control.Schedule)
	if err != nil {
		return CommandResult{Run: r}, err
	}
	if !next.NeedsReconciliation || next.Execution == nil {
		return CommandResult{Run: r}, fail("reconciliation_not_required", "没有已登记的sent或unknown原操作")
	}
	kind, _, err := scheduleAction(next.Execution.Action)
	if err != nil || !chainContains([]string{"create_repository", "push_repository", "trigger_ci"}, kind) {
		return CommandResult{Run: r}, fail("reconciliation_unavailable", "该原操作缺少已实现的只读核实入口，禁止重放")
	}
	r, err = s.ReconcileControlGitLab(ctx, r.ID, *next.Execution)
	if err != nil {
		return CommandResult{Run: r}, err
	}
	r, err = s.Store.Update(r.ID, func(run *Run) error {
		history(run, "reconciled", "按原操作身份完成只读核实；未授予写入或恢复权限")
		return finishControlCommand(run, cmd, budget)
	})
	return CommandResult{Run: r}, err
}

// canonicalContext is a minimal S2 context, built exclusively from frozen
// authority and real envelopes. It intentionally does not convert legacy text
// summaries into trusted message or authorization history.
func canonicalContext(r Run, instruction string, target contracts.DeliveryTarget) ([]contracts.ContextMessage, []contracts.MessageEnvelope, error) {
	if r.Control == nil || r.FrozenDefinition == nil {
		return nil, nil, fail("control_binding_mismatch", "缺少完整上下文来源")
	}
	draft, err := projectPlanFor(r, r.Plan)
	if err != nil {
		return nil, nil, err
	}
	if retry := r.Control.RetryOrigin; retry != nil && r.Control.Audit.Plan == nil && retry.Plan != nil {
		draft = *retry.Plan
		// Adoption evidence belongs to the original Run. A fresh proposal
		// regenerates all files from the frozen templates before a new P1.
		draft.VerificationCandidateRef = nil
	}
	draft.DeliveryTarget = target
	draft.PlanHash, err = contracts.HashObject("ProjectPlan", draft)
	if err != nil {
		return nil, nil, err
	}
	facts := map[string]any{"run_id": r.ID, "state": r.State, "source_version": r.SourceVersion, "state_sequence": r.StateSequence, "modes": runModes(r), "definition_snapshot_hash": r.FrozenDefinition.SnapshotHash, "draft_plan": draft, "skill_catalog": frozenSkillCatalog(r.FrozenDefinition), "expected": instruction}
	if r.Control.RetryOrigin != nil && r.Control.Audit.Plan == nil {
		origin := r.Control.RetryOrigin
		facts["retry_origin"] = map[string]any{"run_id": origin.RunID, "source_version": origin.SourceVersion, "definition_snapshot_hash": origin.DefinitionSnapshotHash, "plan_hash": origin.Plan.PlanHash}
		facts["expected"] = "Return the complete draft unchanged for this explicit retry. Regenerate frozen templates; require new P1."
		// This command only requests the original complete plan. Repeating a
		// catalog of alternative defaults would expand neither context nor scope.
		delete(facts, "skill_catalog")
	}
	return canonicalContextWithAuthority(r, facts)
}

func canonicalActionContext(r Run, action contracts.CandidateAction) ([]contracts.ContextMessage, []contracts.MessageEnvelope, error) {
	a := r.Control.Audit
	if a.Approval == nil || a.PermissionSnapshot == nil || a.Plan == nil {
		return nil, nil, fail("unapproved_execution", "动作上下文缺准确批准来源")
	}
	return canonicalContextWithAuthority(r, map[string]any{"run_id": r.ID, "state": r.State, "source_version": r.SourceVersion, "state_sequence": r.StateSequence, "modes": runModes(r), "definition_snapshot_hash": r.FrozenDefinition.SnapshotHash, "plan_hash": a.Plan.PlanHash, "approval_id": a.Approval.ApprovalID, "approval_hash": a.Approval.ApprovalHash, "permission_hash": a.PermissionSnapshot.PermissionHash, "delivery_target": a.Plan.DeliveryTarget, "expected": "Propose this exact next action; it remains subject to deterministic authorization: " + string(action)})
}

func canonicalContextWithAuthority(r Run, facts any) ([]contracts.ContextMessage, []contracts.MessageEnvelope, error) {
	authority, err := json.Marshal(facts)
	if err != nil {
		return nil, nil, err
	}
	core := []contracts.ContextMessage{{Role: "system", Content: r.FrozenDefinition.Definition.SystemPrompt.Content}, {Role: "system", Content: string(authority)}}
	if c := conversationState(r); c != nil {
		for _, summary := range contextSummaries(r) {
			core = append(core, contracts.ContextMessage{Role: "user", Content: summaryContextText(summary)})
		}
	}
	limit := int(r.FrozenDefinition.Definition.ContextPolicy.RecentMessages)
	if limit > 32 {
		limit = 32
	}
	if limit < 1 {
		limit = 1
	}
	start := len(r.Control.Audit.Messages) - limit
	if start < 0 {
		start = 0
	}
	inputs := append([]contracts.MessageEnvelope{}, r.Control.Audit.Messages[start:]...)
	if len(inputs) == 0 {
		return nil, nil, fail("input_provenance_mismatch", "建议需要真实已持久化输入")
	}
	protected := inputs[len(inputs)-1].MessageID
	for i := len(inputs) - 1; i >= 0; i-- {
		if inputs[i].Role == "user" {
			protected = inputs[i].MessageID
			break
		}
	}
	makeMessages := func(input []contracts.MessageEnvelope) ([]contracts.ContextMessage, error) {
		messages := append([]contracts.ContextMessage{}, core...)
		for _, envelope := range input {
			if conversationState(r) != nil {
				messages = append(messages, contracts.ContextMessage{Role: envelope.Role, Content: conversation.Text(envelope)})
				continue
			}
			if len(envelope.Parts) != 1 {
				return nil, fail("input_provenance_mismatch", "S2输入不是完整原文text")
			}
			var part contracts.TextPart
			if err := contracts.DecodeVersion("TextPart", envelope.ContractVersion, envelope.Parts[0], &part); err != nil {
				return nil, err
			}
			messages = append(messages, contracts.ContextMessage{Role: envelope.Role, Content: part.Text})
		}
		return messages, nil
	}
	for {
		messages, e := makeMessages(inputs)
		if e != nil {
			return nil, nil, e
		}
		if _, e = canonicalModelRequest(r, messages); e == nil {
			return messages, inputs, nil
		}
		if len(inputs) > 1 {
			for i, message := range inputs {
				if message.MessageID != protected {
					inputs = append(inputs[:i], inputs[i+1:]...)
					break
				}
			}
			continue
		}
		if len(core) > 2 {
			core = append(core[:2], core[3:]...)
			continue
		}
		return nil, nil, e
	}
}

func canonicalModelRequest(r Run, messages []contracts.ContextMessage) ([]byte, error) {
	policy := r.FrozenDefinition.Definition.ContextPolicy
	model := "rules"
	if r.FrozenDefinition.ProviderBinding != nil {
		model = r.FrozenDefinition.ProviderBinding.ModelID
	}
	body, err := json.Marshal(map[string]any{"model": model, "messages": messages, "temperature": 0.1, "response_format": map[string]string{"type": "json_object"}, "max_tokens": policy.OutputReserveTokens})
	if err != nil {
		return nil, err
	}
	if int64(len(body))+policy.OutputReserveTokens+policy.ProtocolReserveTokens > policy.MaxTokens {
		return nil, fail("context_budget_exceeded", "必要规则与完整当前原文超过冻结上下文预算；尚未调用模型")
	}
	return body, nil
}

func (s *Service) controlPlanningTarget(ctx context.Context, r Run) (contracts.DeliveryTarget, error) {
	if r.DeliveryMode == "local" {
		b, e := json.Marshal(contracts.LocalTarget{Provider: "local", Destination: "run_package"})
		return contracts.DeliveryTarget(b), e
	}
	if r.DeliveryMode != "gitlab" || s.Config.GitLabToken == "" || s.Config.GitLabNamespace == "" {
		return nil, fail("gitlab_unavailable", "精确GitLab目标需要已配置并可核实的namespace")
	}
	var ns struct {
		FullPath string `json:"full_path"`
	}
	bounded, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if _, e := s.gitlab(bounded, "GET", "/namespaces/"+url.PathEscape(s.Config.GitLabNamespace), nil, &ns); e != nil || ns.FullPath == "" {
		return nil, fail("gitlab_unavailable", "GitLab namespace不可核实")
	}
	p := r.FrozenDefinition.Definition.RepositoryPolicy
	body, err := json.Marshal(contracts.GitLabTarget{Provider: "gitlab", InstanceURL: s.Config.GitLabURL, NamespaceID: s.Config.GitLabNamespace, NamespacePath: ns.FullPath, ProjectPath: r.Plan.ProjectName, Ref: p.RequiredRef, Visibility: p.Visibility})
	return contracts.DeliveryTarget(body), err
}

func planTargetName(target contracts.DeliveryTarget, name string) contracts.DeliveryTarget {
	var remote contracts.GitLabTarget
	if json.Unmarshal(target, &remote) == nil && remote.Provider == "gitlab" {
		remote.ProjectPath = name
		body, _ := json.Marshal(remote)
		return contracts.DeliveryTarget(body)
	}
	return target
}

func (s *Service) controlPlanningCommand(ctx context.Context, r Run, cmd Command, commandBudget contracts.BudgetReservation) (CommandResult, error) {
	target, err := s.controlPlanningTarget(ctx, r)
	if err != nil {
		return CommandResult{Run: r}, err
	}
	var modelBudget *contracts.BudgetReservation
	if r.DecisionMode == "model" {
		if !s.ModelConfigured() {
			return CommandResult{Run: r}, fail("model_unavailable", "冻结model模式缺配置，不会切换rules")
		}
		timeout := r.FrozenDefinition.Definition.BudgetPolicy.MaxToolSeconds
		if timeout > 60 {
			timeout = 60
		}
		var reserved contracts.BudgetReservation
		r, reserved, err = s.reserveControlBudget(r.ID, governance.ReservationInput{SourceVersion: int64(r.SourceVersion), CommandID: cmd.ID, AttemptID: newID("model_attempt_"), Kind: "model", ScopeID: "decision:model", TimeoutSeconds: timeout, Now: time.Now().UTC()})
		if err != nil {
			return CommandResult{Run: r}, err
		}
		modelBudget = &reserved
	}
	// Admission is not itself a provider attempt. A preflight failure can only
	// release a reservation while this invocation knows no call began.
	modelOutcome := "failed"
	modelSettled := false
	modelStarted := false
	failModel := func(e error) (CommandResult, error) {
		if modelBudget != nil && !modelSettled {
			start, _ := time.Parse(time.RFC3339Nano, modelBudget.ReservedAt)
			settlement := actualControlSettlement(*modelBudget, modelOutcome, start, time.Now().UTC())
			if !modelStarted {
				zero := int64(0)
				settlement.Outcome, settlement.ElapsedMS, settlement.ConfirmedNotStarted = "released", &zero, true
			}
			updated, _, se := s.settleControlBudget(r.ID, settlement)
			modelSettled = se == nil
			if se != nil {
				return CommandResult{Run: r}, errors.Join(e, se)
			}
			r = updated
		}
		return CommandResult{Run: r}, e
	}
	ordered, inputs, err := canonicalContext(r, "Return clarification or a complete plan for the current user input. No execution is authorized by this request.", target)
	if err != nil {
		return failModel(err)
	}
	var advice contracts.DecisionAdvice
	usage := contracts.ModelUsage{Status: "not_applicable"}
	if r.DecisionMode == "model" {
		request, e := canonicalModelRequest(r, ordered)
		if e != nil {
			return failModel(e)
		}
		deadline, _ := time.Parse(time.RFC3339Nano, *modelBudget.DeadlineAt)
		bounded, cancel := context.WithDeadline(ctx, deadline)
		response, e := s.callModelAdvice(bounded, r, request)
		modelStarted = response.Started
		cancel()
		if e != nil {
			return failModel(e)
		}
		advice, usage, ordered = response.Advice, response.Usage, response.Messages
		modelOutcome = "succeeded"
	} else if r.DecisionMode == "rules" {
		// The rules parser consumes the same persisted final input that was
		// included in ordered context, not a second unregistered text source.
		p := ParsePlan(r.Plan, ordered[len(ordered)-1].Content)
		retry := r.Control.RetryOrigin
		if retry != nil && r.Control.Audit.Plan == nil && retry.Plan != nil {
			p = r.Plan
		}
		p, err = s.normalizeFrozenPlan(r, p)
		if err != nil {
			return failModel(err)
		}
		if p.RepoProvider != r.DeliveryMode {
			return failModel(fail("invalid_mode", "交付模式已绑定当前Run；改变交付模式需要新Run明确选择"))
		}
		if p.ProjectName == "" {
			question := "请提供小写英文项目名，例如 order-service。"
			advice = contracts.DecisionAdvice{Kind: "clarification", Question: &question, Reason: "显式rules解析后缺少项目名"}
		} else {
			plan, e := projectPlanFor(r, p)
			if e != nil {
				return failModel(e)
			}
			plan.DeliveryTarget = planTargetName(target, p.ProjectName)
			plan.PlanHash, e = contracts.HashObject("ProjectPlan", plan)
			if e != nil {
				return failModel(e)
			}
			advice = contracts.DecisionAdvice{Kind: "plan", Plan: &plan, Reason: "按显式rules解析已持久化原文"}
			if retry != nil && r.Control.Audit.Plan == nil && retry.Plan != nil {
				plan = *retry.Plan
				plan.VerificationCandidateRef = nil
				plan.PlanHash, e = contracts.HashObject("ProjectPlan", plan)
				if e != nil {
					return failModel(e)
				}
				advice = contracts.DecisionAdvice{Kind: "plan", Plan: &plan, Reason: "按真实重做命令建议准确原完整计划，仍须新P1"}
			}
		}
	} else {
		return failModel(fail("invalid_mode", "无效决策模式"))
	}
	var proposed Plan
	if advice.Kind == "plan" {
		expected := planTargetName(target, advice.Plan.ProjectName)
		proposed, err = s.compatibilityPlan(r, *advice.Plan, expected)
		if err != nil {
			return failModel(err)
		}
		canonical, e := projectPlanFor(r, proposed)
		if e != nil {
			return failModel(e)
		}
		canonical.DeliveryTarget = expected
		canonical.PlanHash, e = contracts.HashObject("ProjectPlan", canonical)
		if e != nil {
			return failModel(e)
		}
		advice.Plan = &canonical
		if retry := r.Control.RetryOrigin; retry != nil && r.Control.Audit.Plan == nil && retry.Plan != nil {
			expected := *retry.Plan
			expected.VerificationCandidateRef = nil
			expected.PlanHash, e = contracts.HashObject("ProjectPlan", expected)
			if e != nil || !equalContractValue(expected, canonical) {
				return failModel(fail("advice_rejected", "重做建议改变了准确原完整计划；必须新原文明确修改范围"))
			}
		}
	}
	in, record, err := bindCanonicalDecision(r, cmd.ID, advice, usage, inputs, ordered, time.Now().UTC())
	if err != nil {
		return failModel(err)
	}
	validation, err := (governance.Validator{Definitions: s.Definitions}).Validate(in)
	if err != nil {
		return failModel(err)
	}
	rejected := validation.Verdict == "rejected"
	updated, err := s.Store.Update(r.ID, func(run *Run) error {
		if e := s.recordCanonicalDecision(run, in, record, validation); e != nil {
			return e
		}
		if modelBudget != nil {
			if e := settleControlInRun(run, *modelBudget, modelOutcome, time.Now().UTC()); e != nil {
				return e
			}
		}
		if rejected {
			run.Error = "建议未获准：" + validation.Findings[0].Code
			history(run, "advice_rejected", run.Error)
			// Persist the rejected full chain, but allow an explicit same-input
			// retry to obtain a new, charged provider response.
			return nil
		}
		switch advice.Kind {
		case "clarification":
			if run.Control.Audit.Proposal != nil {
				run.Control.Audit.Proposal.Status = "revision_requested"
				run.Proposal.Status = "revision_requested"
			}
			run.State = "CLARIFY"
			run.Error = ""
			addMessage(run, "assistant", *advice.Question, "text", "")
		case "plan":
			old := run.Control.Audit.Plan
			changed := old == nil || old.PlanHash != advice.Plan.PlanHash
			if changed {
				sourceChanged(run)
			}
			run.Plan = proposed
			run.Title = proposed.ProjectName
			run.Error = ""
			p := run.Control.Audit.Proposal
			refresh := changed || p == nil || p.Status != "pending"
			if p != nil {
				expiry, e := time.Parse(time.RFC3339Nano, p.ExpiresAt)
				if e != nil || !time.Now().UTC().Before(expiry) {
					refresh = true
				}
			}
			if refresh {
				files, e := s.renderFrozenFiles(*run)
				if e != nil {
					return e
				}
				if e = s.makeCanonicalProposal(run, *advice.Plan, files, time.Now().UTC()); e != nil {
					return e
				}
				run.RequiredChecks = requiredChecksFor(*run)
			}
			run.State = "PLAN"
			addMessage(run, "assistant", advice.Reason+"\n请审查当前准确方案后确认。", "proposal", run.Proposal.ID)
		default:
			return fail("invalid_state", "规划入口未授予动作、诊断或完成权限")
		}
		return finishControlCommand(run, cmd, commandBudget)
	})
	if err != nil {
		return failModel(err)
	}
	modelSettled = true
	if rejected {
		return CommandResult{Run: updated}, fail("advice_rejected", updated.Error)
	}
	return CommandResult{Run: updated}, nil
}

func (s *Service) controlExplicitCommand(r Run, cmd Command, budget contracts.BudgetReservation) (CommandResult, error) {
	execute := false
	updated, err := s.Store.Update(r.ID, func(run *Run) error {
		switch cmd.Action {
		case "accept":
			if err := s.acceptCanonical(run, cmd, time.Now().UTC()); err != nil {
				return err
			}
			execute = true
		case "reject", "revise":
			p := run.Control.Audit.Proposal
			if run.State != "PLAN" || p == nil || p.Status != "pending" || run.Control.Audit.Approval != nil {
				return fail("proposal_closed", "当前提案不接受关闭请求")
			}
			if cmd.ProposalID != p.ProposalID || int64(cmd.ProposalVersion) != p.ProposalVersion || int64(cmd.SourceVersion) != p.SourceVersion || p.SourceVersion != int64(run.SourceVersion) || cmd.SnapshotHash != p.SnapshotHash || !hashMatches("Proposal", *p, p.SnapshotHash) {
				return fail("stale_proposal", "关闭请求未绑定当前完整提案身份、来源与摘要")
			}
			status := "rejected"
			if cmd.Action == "revise" {
				status = "revision_requested"
			}
			p.Status = status
			run.Proposal.Status = status
			run.State = "CLARIFY"
		case "pause":
			if run.State == "PAUSED" || run.State == "BLOCKED" {
				return fail("invalid_state", "Run已暂停或阻塞")
			}
			if run.ActiveStep != "" {
				run.PauseRequested = true
			} else {
				run.ResumeTo = run.State
				run.State = "PAUSED"
			}
		case "cancel":
			if run.ActiveStep != "" {
				run.CancelRequested = true
			} else {
				run.State = "CANCELLED"
				run.Error = "用户取消；已发生事实与未知操作保留"
				run.PauseRequested = false
			}
		case "resume":
			if run.State != "PAUSED" && run.State != "BLOCKED" {
				return fail("invalid_state", "只有暂停或阻塞Run可恢复")
			}
			if run.ActiveStep != "" {
				return fail("run_busy", "原操作未到安全边界")
			}
			for _, op := range run.Control.Audit.Operations {
				if op.Status == "sent" || op.Status == "unknown" {
					return fail("reconciliation_required", "未知原操作只允许准确身份核实；恢复命令不能重放")
				}
			}
			next := run.ResumeTo
			if next == "" {
				next = "CLARIFY"
			}
			if run.Control.Audit.Approval != nil {
				if err := s.recheckControlApproval(*run, time.Now().UTC()); err != nil {
					return err
				}
				if !chainContains([]string{"IMPLEMENT", "VERIFY", "DELIVER"}, next) {
					return fail("invalid_state", "缺少准确恢复阶段")
				}
				execute = true
			}
			run.State = next
			run.ResumeTo = ""
			run.PauseRequested = false
			run.CancelRequested = false
			run.Error = ""
		default:
			return fail("invalid_action", "未知控制命令")
		}
		history(run, cmd.Action, "控制命令按完整来源和当前授权处理")
		return finishControlCommand(run, cmd, budget)
	})
	return CommandResult{Run: updated, Execute: execute}, err
}

func (s *Service) recheckControlApproval(r Run, at time.Time) error {
	if err := verifyControlChain(r); err != nil {
		return err
	}
	if err := s.verifyFrozenDefinition(r); err != nil {
		return err
	}
	a := r.Control.Audit
	if a.Approval == nil || a.PermissionSnapshot == nil || a.Proposal == nil || a.Plan == nil {
		return fail("unapproved_execution", "缺少完整批准")
	}
	ap := a.Approval
	p := a.Proposal
	grant := a.PermissionSnapshot
	if ap.SourceVersion != int64(r.SourceVersion) || p.SourceVersion != int64(r.SourceVersion) || ap.ProposalID != p.ProposalID || ap.ProposalVersion != p.ProposalVersion || ap.ProposalSnapshotHash != p.SnapshotHash || ap.PlanHash != a.Plan.PlanHash || ap.PermissionHash != grant.PermissionHash || ap.ActorID != localActorID || grant.ActorID != localActorID || grant.Revoked || ap.Status == "revoked" || ap.RevokedAt != nil || p.Status != "accepted" {
		return fail("approval_binding_mismatch", "当前来源、权限或批准已失效")
	}
	if err := s.canonicalTargetAllowed(r, *a.Plan); err != nil {
		return err
	}
	expires, e := time.Parse(time.RFC3339Nano, ap.ExpiresAt)
	if e != nil || ap.StartedAt == nil && !at.Before(expires) {
		return fail("proposal_expired", "首次执行批准已过期")
	}
	return nil
}

// Recover only observations whose owning process is definitively gone. An
// active request must never use this to settle another live attempt.
func recoverControlCommandBudgets(r *Run, at time.Time) error {
	if r.Control == nil {
		return nil
	}
	settled := map[string]bool{}
	for _, x := range r.Control.Audit.Budget.Settlements {
		settled[x.ReservationID] = true
	}
	for _, x := range r.Control.Audit.Budget.Reservations {
		if settled[x.ReservationID] {
			continue
		}
		ledger, _, err := governance.SettleBudget(r.FrozenDefinition.Definition.BudgetPolicy, r.Control.Audit.Budget, governance.SettlementInput{ReservationID: x.ReservationID, AttemptID: x.AttemptID, Outcome: "unknown", MeasurementSource: "unknown", Now: at})
		if err != nil {
			return err
		}
		r.Control.Audit.Budget = ledger
		if c := conversationState(*r); c != nil && x.Kind == "command" {
			if receipt, ok := c.Receipts[x.CommandID]; ok && receipt.Status != "processed" {
				receipt.Status = "result_unknown"
				receipt.RunRef = nil
				receipt.Error = nil
				receipt.UpdatedAt = at.UTC().Format(time.RFC3339Nano)
				if err = saveConversationReceipt(c, receipt); err != nil {
					return err
				}
			}
		}
		if x.Kind == "action" || x.Kind == "tool" {
			for i := range r.Control.Audit.Operations {
				op := &r.Control.Audit.Operations[i]
				if op.OperationID == *x.OperationID && op.Status == "sent" {
					op.Status, op.UpdatedAt = "unknown", at.UTC().Format(time.RFC3339Nano)
				}
			}
			if !terminal(r.State) && r.State != "PAUSED" && r.State != "BLOCKED" {
				r.ResumeTo, r.State = r.State, "BLOCKED"
				r.Error = "进程中断的原执行没有完整终止事实；原身份已按unknown消费，禁止盲重放"
			}
			r.ActiveStep = ""
		}
	}
	syncBudgetProjection(r)
	return recoverPendingSummary(r, at)
}

func controlHasPendingCommands(r Run) bool {
	if r.Control == nil {
		return false
	}
	if c := conversationState(r); c != nil && c.PendingSummary != nil {
		return true
	}
	settled := map[string]bool{}
	for _, x := range r.Control.Audit.Budget.Settlements {
		settled[x.ReservationID] = true
	}
	for _, x := range r.Control.Audit.Budget.Reservations {
		if !settled[x.ReservationID] {
			return true
		}
	}
	return false
}
