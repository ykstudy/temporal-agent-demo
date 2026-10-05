package agent

import (
	"context"
	"errors"
	"time"

	"agentstudio/internal/contracts"
	"agentstudio/internal/governance"
)

// controlRetryEvidence proves a completed, unchanged VERIFY attempt. An error
// string, model diagnosis, or the absence of a receipt never proves this fact.
func controlRetryEvidence(r Run, x contracts.ExecutionRequest) ([]contracts.ObjectReference, error) {
	if r.Control == nil || r.Control.Audit.Manifest == nil || r.ActiveStep != "" || x.SourceVersion != int64(r.SourceVersion) {
		return nil, fail("retry_not_safe", "验证重试缺少当前完整来源或安全边界")
	}
	if kind, _, e := scheduleAction(x.Action); e != nil || kind != "verify" {
		return nil, fail("retry_not_safe", "只有已知完成的verify可按原批准重试")
	}
	registered, rejected := false, false
	for _, stored := range r.Control.Audit.Executions {
		if equalContractValue(stored, x) {
			registered = true
		}
	}
	for _, op := range r.Control.Audit.Operations {
		if op.Status == "sent" || op.Status == "unknown" {
			return nil, fail("reconciliation_required", "未知操作必须先按原身份核实")
		}
		if op.OperationID == x.OperationID && op.RequestHash == x.RequestHash && op.Status == "rejected" {
			rejected = true
		}
	}
	if !registered || !rejected || r.Control.Audit.Approval == nil || r.Control.Audit.Approval.ApprovalID != x.Approval.ApprovalID {
		return nil, fail("retry_not_safe", "缺少准确原请求与已知失败状态")
	}
	var parent *contracts.BudgetReservation
	for _, p := range r.Control.Audit.Budget.Reservations {
		if p.Kind == "action" && p.ScopeID == "verify" && p.RequestHash != nil && *p.RequestHash == x.RequestHash {
			copy := p
			parent = &copy
		}
	}
	if parent == nil {
		return nil, fail("retry_not_safe", "缺少原验证action预留")
	}
	settlements := map[string]contracts.BudgetSettlement{}
	for _, settled := range r.Control.Audit.Budget.Settlements {
		settlements[settled.ReservationID] = settled
	}
	settled, ok := settlements[parent.ReservationID]
	if !ok || settled.Outcome != "failed" || settled.MeasurementSource != "actual" {
		return nil, fail("retry_not_safe", "原验证尝试不是已知失败结算")
	}
	commands, err := governance.MergePostCommands(*r.FrozenDefinition, *r.Control.Audit.Plan)
	if err != nil {
		return nil, err
	}
	manifest := controlManifestRef(*r.Control.Audit.Manifest)
	refs := []contracts.ObjectReference{}
	failed, gap := false, false
	for _, command := range commands {
		identity, _ := contracts.HashValue([]string{x.RequestHash, parent.AttemptID, command.CommandID})
		attemptID := "tool_" + identity[:40]
		var reservation *contracts.BudgetReservation
		var receipt *contracts.ToolExecutionReceipt
		for _, p := range r.Control.Audit.Budget.Reservations {
			if p.AttemptID == attemptID && p.Kind == "tool" {
				copy := p
				reservation = &copy
			}
		}
		for _, value := range r.Control.Audit.ToolReceipts {
			if value.AttemptID == attemptID {
				if receipt != nil {
					return nil, fail("retry_not_safe", "同一工具尝试有多份回执")
				}
				copy := value
				receipt = &copy
			}
		}
		if reservation == nil && receipt == nil {
			gap = true
			continue
		}
		if gap || failed || reservation == nil || receipt == nil {
			return nil, fail("retry_not_safe", "工具预留、顺序或回执不完整")
		}
		settled, ok := settlements[reservation.ReservationID]
		if !ok || settled.MeasurementSource != "actual" || !chainContains([]string{"succeeded", "failed"}, settled.Outcome) || !chainContains([]string{"succeeded", "failed", "timed_out"}, receipt.Status) {
			return nil, fail("retry_not_safe", "工具存在未知、中断或未结算尝试")
		}
		toolBound := false
		for _, tool := range r.FrozenDefinition.ToolBindings {
			if equalContractValue(tool, receipt.Tool) && tool.ToolID == command.Executable {
				toolBound = true
			}
		}
		if !toolBound || !hashMatches("ToolExecutionReceipt", *receipt, receipt.ReceiptHash) || receipt.RunID != r.ID || receipt.SourceVersion != x.SourceVersion || receipt.ExecutionID != x.ExecutionID || receipt.OperationID != x.OperationID || receipt.RequestHash != x.RequestHash || receipt.ApprovalID != x.Approval.ApprovalID || receipt.ApprovalHash != r.Control.Audit.Approval.ApprovalHash || receipt.ProposalSnapshotHash != x.Approval.ProposalSnapshotHash || receipt.DefinitionSnapshotHash != x.DefinitionSnapshotHash || receipt.PlanHash != x.PlanHash || !equalContractValue(receipt.Workspace, x.Workspace) || receipt.CommandID != command.CommandID || !equalContractValue(receipt.Arguments, command.Arguments) || receipt.CwdScope != command.CwdScope || receipt.EnvironmentPolicy != command.EnvironmentPolicy || !equalContractValue(scheduleRef(receipt.InputManifestRef), &manifest) || !equalContractValue(scheduleRef(receipt.OutputManifestRef), &manifest) || receipt.OutputSHA256 != contracts.BytesHash([]byte(receipt.Output)) || !chainBefore(reservation.ReservedAt, receipt.StartedAt, false) || !chainBefore(receipt.StartedAt, receipt.CompletedAt, false) || !chainBefore(receipt.CompletedAt, settled.SettledAt, false) {
			return nil, fail("retry_not_safe", "工具回执未证明原命令及未变的批准内容")
		}
		ref := contracts.ObjectReference{ObjectType: "evidence", ObjectID: receipt.ReceiptID, ObjectVersion: 1, SourceVersion: receipt.SourceVersion, Hash: receipt.ReceiptHash}
		if !scheduleHasRef(r.Control.ObjectRefs, ref) {
			return nil, fail("retry_not_safe", "工具回执未登记不可变原体")
		}
		refs = append(refs, ref)
		failed = receipt.Status != "succeeded"
	}
	if !failed || len(refs) == 0 {
		return nil, fail("retry_not_safe", "缺少已知终止的工具失败")
	}
	return refs, nil
}

// RetryControlVerification deliberately reuses the immutable request and
// operation. Only the new attempt is fresh; source, approval and all budget
// windows remain unchanged. Unknown attempts cannot use this entry point.
func (s *Service) RetryControlVerification(ctx context.Context, id string, x contracts.ExecutionRequest) (Run, error) {
	r, err := s.Store.Get(id)
	if err != nil {
		return r, err
	}
	if !chainContains([]string{"VERIFY", "BLOCKED"}, r.State) || r.ResumeTo != "VERIFY" && r.State == "BLOCKED" || r.CancelRequested || r.PauseRequested {
		return r, fail("execution_stopped", "当前状态不能启动验证安全重试")
	}
	refs, err := controlRetryEvidence(r, x)
	if err != nil {
		return r, err
	}
	if err = s.recheckControlApproval(r, time.Now().UTC()); err != nil {
		return r, err
	}
	files, err := s.controlToolFiles(r, r.Workspace)
	if err != nil {
		return r, err
	}
	if err = controlMatchApproved(r, files); err != nil {
		return r, err
	}
	// An exhausted deterministic limit does not depend on obtaining another
	// model response (which may itself have no remaining allowance).
	if limitErr := controlRetryAllowance(r, x, time.Now().UTC()); limitErr != nil {
		if !controlRetryBudgetExhausted(limitErr) {
			return r, limitErr
		}
		updated, saveErr := s.Store.Update(id, func(run *Run) error {
			if !chainContains([]string{"VERIFY", "BLOCKED"}, run.State) || run.PauseRequested || run.CancelRequested {
				return fail("execution_stopped", "预算终止前Run已停止")
			}
			if _, e := controlRetryEvidence(*run, x); e != nil {
				return e
			}
			if e := s.recheckControlApproval(*run, time.Now().UTC()); e != nil {
				return e
			}
			if e := controlRetryAllowance(*run, x, time.Now().UTC()); !controlRetryBudgetExhausted(e) {
				return fail("stale_budget", "预算耗尽事实已改变")
			}
			controlRetryFailed(run)
			return nil
		})
		return updated, errors.Join(limitErr, saveErr)
	}
	diagnosis := contracts.Diagnosis{Category: "business", Code: "verification_failed", Message: "固定工具返回已知失败，批准内容未变化", EvidenceRefs: refs, SuggestedRemedy: "仅在剩余原预算内对同一批准内容重新验证；内容修复另行提案批准"}
	advice := contracts.DecisionAdvice{Kind: "diagnosis", Diagnosis: &diagnosis, Reason: "规则依据完整工具回执识别原批准内的有限重试边界"}
	if _, err = s.prepareControlSemanticAdvice(ctx, id, "diagnosis", advice, map[string]any{"execution_id": x.ExecutionID, "operation_id": x.OperationID, "request_hash": x.RequestHash, "manifest_ref": controlManifestRef(*r.Control.Audit.Manifest), "known_result": "failed_with_unchanged_approved_files"}, refs); err != nil {
		return r, err
	}
	var exhausted error
	r, err = s.Store.Update(id, func(run *Run) error {
		if !chainContains([]string{"VERIFY", "BLOCKED"}, run.State) || run.PauseRequested || run.CancelRequested {
			return fail("execution_stopped", "语义建议期间Run已停止")
		}
		if _, e := controlRetryEvidence(*run, x); e != nil {
			return e
		}
		if e := s.recheckControlApproval(*run, time.Now().UTC()); e != nil {
			return e
		}
		// A dry reservation validates the original deadline and cumulative
		// action allowance without consuming a speculative attempt.
		e := controlRetryAllowance(*run, x, time.Now().UTC())
		if e != nil {
			if !controlRetryBudgetExhausted(e) {
				return e
			}
			exhausted = e
			controlRetryFailed(run)
			return nil
		}
		for i := range run.Control.Audit.Operations {
			if run.Control.Audit.Operations[i].OperationID == x.OperationID {
				run.Control.Audit.Operations[i].Status = "ready"
				run.Control.Audit.Operations[i].UpdatedAt = now()
			}
		}
		run.State, run.ResumeTo, run.Error = "IMPLEMENT", "", ""
		history(run, "verification_retry", "原请求与原批准内容的有界新尝试；失败与预算事实保留")
		return nil
	})
	return r, errors.Join(err, exhausted)
}

func controlRetryFailed(run *Run) {
	run.State, run.ResumeTo, run.Error = "FAILED", "", "原验证执行预算已耗尽；重做需要新Run身份和新批准"
	history(run, "verification_budget_exhausted", run.Error)
}

func controlRetryBudgetExhausted(err error) bool {
	var budgetError *governance.Error
	return errors.As(err, &budgetError) && chainContains([]string{"budget_exceeded", "budget_deadline_exceeded", "budget_capacity_exceeded"}, budgetError.Code)
}

func controlRetryAllowance(r Run, x contracts.ExecutionRequest, at time.Time) error {
	policy, ledger := r.FrozenDefinition.Definition.BudgetPolicy, r.Control.Audit.Budget
	_, _, err := governance.ReserveBudget(policy, ledger, governance.ReservationInput{SourceVersion: x.SourceVersion, CommandID: x.CommandID, AttemptID: newID("retry_probe_"), Kind: "action", ScopeID: "verify", Request: &x, Now: at})
	if err != nil {
		return err
	}
	commands, err := governance.MergePostCommands(*r.FrozenDefinition, *r.Control.Audit.Plan)
	if err != nil {
		return err
	}
	for _, command := range commands {
		_, _, err = governance.ReserveBudget(policy, ledger, governance.ReservationInput{SourceVersion: x.SourceVersion, CommandID: x.CommandID, AttemptID: newID("retry_tool_probe_"), Kind: "tool", ScopeID: "tool:" + command.CommandID, Request: &x, Tool: &command, Now: at})
		if err != nil {
			return err
		}
	}
	return nil
}
