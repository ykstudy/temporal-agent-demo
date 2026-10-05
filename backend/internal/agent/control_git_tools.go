package agent

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"agentstudio/internal/contracts"
	"agentstudio/internal/governance"
)

// Private launch facts, saved before Start. They are neither a successful tool
// receipt nor authority to restart a command after a process/worker failure.
type controlGitInvocation struct {
	ExecutionID string                 `json:"execution_id"`
	AttemptID   string                 `json:"attempt_id"`
	Parent      string                 `json:"parent_attempt"`
	CommandID   string                 `json:"command_id"`
	Tool        contracts.ToolIdentity `json:"tool"`
	Arguments   []string               `json:"arguments"`
	StartedAt   string                 `json:"started_at"`
}

func verifyControlGitInvocations(r Run) error {
	for key, in := range r.Control.GitInvocations {
		var x *contracts.ExecutionRequest
		for _, candidate := range r.Control.Audit.Executions {
			if candidate.ExecutionID == in.ExecutionID {
				copy := candidate
				x = &copy
			}
		}
		if x == nil || !contracts.DeliveryContract(x.ContractVersion) || key != in.AttemptID {
			return remoteReceiptError("Git launch original request")
		}
		kind, raw, err := scheduleAction(x.Action)
		var p contracts.PushRepositoryParameters
		if err != nil || kind != "push_repository" || contracts.DecodeVersion("PushRepositoryParameters", x.ContractVersion, raw, &p) != nil {
			return remoteReceiptError("Git launch action")
		}
		parent, err := controlGitLabAttempt(r, *x)
		if err != nil || in.Parent != parent.AttemptID || key != controlGitChild(*x, in.Parent, in.CommandID) {
			return remoteReceiptError("Git launch parent/child identity")
		}
		toolFound, commandFound, budgetFound := false, false, false
		for _, tool := range r.FrozenDefinition.ToolBindings {
			if tool.ToolID == "git" && chainEqual(tool, in.Tool) {
				toolFound = true
			}
		}
		for _, policy := range r.FrozenDefinition.Definition.RepositoryPolicy.GitExecution.Commands {
			if policy.CommandID == in.CommandID && chainEqual(controlGitArguments(policy, *x, p.Target), in.Arguments) {
				commandFound = true
			}
		}
		for _, b := range r.Control.Audit.Budget.Reservations {
			if b.AttemptID == key && b.Kind == "tool" && b.ScopeID == "tool:"+in.CommandID && remoteReservationBinds(b, *x) && b.DeadlineAt != nil && chainBefore(b.ReservedAt, in.StartedAt, false) && chainBefore(in.StartedAt, *b.DeadlineAt, false) {
				budgetFound = true
			}
		}
		if !toolFound || !commandFound || !budgetFound {
			return remoteReceiptError("Git launch frozen identity/command/budget")
		}
		if in.CommandID == "git.push" && !r.Control.RemoteWriteStarted[x.ExecutionID] {
			return remoteReceiptError("Git push launch missing write boundary")
		}
		for _, q := range r.Control.Audit.ToolReceipts {
			if q.AttemptID == key && (q.ExecutionID != x.ExecutionID || q.RequestHash != x.RequestHash || q.StartedAt != in.StartedAt || q.CommandID != in.CommandID || !chainEqual(q.Tool, in.Tool) || !chainEqual(q.Arguments, in.Arguments) || !hashMatches("ToolExecutionReceipt", q, q.ReceiptHash)) {
				return remoteReceiptError("Git receipt differs from original launch")
			}
		}
	}
	return nil
}

func controlGitChild(x contracts.ExecutionRequest, parent, command string) string {
	hash, _ := contracts.HashValue([]string{x.RequestHash, parent, command})
	return "tool_" + hash[:40]
}

func (s *Service) controlGitToolFacts(r Run, x contracts.ExecutionRequest, parent string) error {
	_, op, err := s.controlGitLabRequest(r, x)
	if err != nil {
		return err
	}
	kind, _, _ := scheduleAction(x.Action)
	if kind != "push_repository" || r.State != "DELIVER" || r.ActiveStep != x.ExecutionID || op.Status != "sent" || r.PauseRequested || r.CancelRequested {
		return fail("execution_stopped", "固定Git命令只能执行当前已准入推送")
	}
	a, err := controlGitLabAttempt(r, x)
	if err != nil || a.AttemptID != parent || controlGitLabSettled(r, a) != nil {
		return fail("tool_parent_closed", "固定Git命令缺少原未结算父尝试")
	}
	for _, v := range r.Control.Audit.Validations {
		if v.ValidationID != x.ValidationID {
			continue
		}
		for _, original := range r.Control.ValidationInputs {
			if original.Decision.DecisionID != v.DecisionID {
				continue
			}
			facts, err := s.controlExecutionFacts(r, ControlSchedule{Action: &x.Action, OperationSequence: original.Facts.OperationSequence}, x.CommandID, time.Now().UTC(), parent)
			if err != nil {
				return err
			}
			// This exact owned, open action is already admitted, not a new write.
			facts.Operation = nil
			return (governance.Validator{Definitions: s.Definitions}).Revalidate(x, original, v, facts)
		}
	}
	return fail("execution_binding_mismatch", "固定Git执行缺少原决定和校验")
}

func (s *Service) executeControlGitTools(ctx context.Context, r Run, x contracts.ExecutionRequest, target contracts.GitLabTarget, parent contracts.BudgetReservation) error {
	tool, binary, err := controlGitIdentity(r)
	if err != nil {
		return err
	}
	_, helper, err := controlGitHTTPIdentity(r)
	if err != nil {
		return err
	}
	paths, err := s.controlToolPaths(r, x, parent.AttemptID)
	if err != nil {
		return err
	}
	approved, err := s.controlToolFiles(r, paths.approved)
	if err != nil {
		return err
	}
	if err = controlMatchApproved(r, approved); err != nil {
		return err
	}
	for name := range approved {
		for _, part := range strings.Split(name, "/") {
			if strings.EqualFold(part, ".git") {
				return fail("artifact_source_mismatch", "批准源文件不能包含Git私有元数据")
			}
		}
	}
	if err = paths.prepare(); err != nil {
		return err
	}
	// There is deliberately no resume/replay branch for a Git working copy.
	if err = controlCopyFiles(paths.candidate, approved); err != nil {
		return err
	}
	for _, policy := range r.FrozenDefinition.Definition.RepositoryPolicy.GitExecution.Commands {
		if err = ctx.Err(); err != nil {
			return err
		}
		actual, e := s.controlToolFiles(r, paths.approved)
		if e != nil {
			return e
		}
		candidate, e := s.controlSourceFiles(r, paths.candidate, true)
		if e != nil {
			return e
		}
		if !chainEqual(actual, approved) || !chainEqual(candidate, approved) {
			return fail("artifact_source_mismatch", "推送前批准文件与实际Git副本不同")
		}
		var reserved contracts.BudgetReservation
		r, err = s.Store.Update(r.ID, func(run *Run) error {
			if e := s.controlGitToolFacts(*run, x, parent.AttemptID); e != nil {
				return e
			}
			var e error
			child := controlGitChild(x, parent.AttemptID, policy.CommandID)
			for _, old := range run.Control.Audit.Budget.Reservations {
				if old.AttemptID == child {
					return fail("tool_attempt_unknown", "已有Git尝试禁止重放")
				}
			}
			run.Control.Audit.Budget, reserved, e = governance.ReserveBudget(run.FrozenDefinition.Definition.BudgetPolicy, run.Control.Audit.Budget, governance.ReservationInput{SourceVersion: x.SourceVersion, CommandID: x.CommandID, AttemptID: child, Kind: "tool", ScopeID: "tool:" + policy.CommandID, Request: &x, GitTool: &policy, Now: time.Now().UTC()})
			syncBudgetProjection(run)
			return e
		})
		if err != nil {
			return err
		}
		bounded, cancel, e := controlGitLabDeadline(ctx, reserved)
		if e != nil {
			return e
		}
		cmd, e := controlGitSandboxCommand(bounded, paths, binary, helper, x, target, policy, s.Config.GitLabToken)
		if e != nil {
			cancel()
			return e
		}
		invocation := controlGitInvocation{ExecutionID: x.ExecutionID, AttemptID: reserved.AttemptID, Parent: parent.AttemptID, CommandID: policy.CommandID, Tool: tool, Arguments: controlGitArguments(policy, x, target)}
		r, err = s.Store.Update(r.ID, func(run *Run) error {
			if e := s.controlGitToolFacts(*run, x, parent.AttemptID); e != nil {
				return e
			}
			if run.Control.GitInvocations == nil {
				run.Control.GitInvocations = map[string]controlGitInvocation{}
			}
			if _, found := run.Control.GitInvocations[reserved.AttemptID]; found {
				return fail("tool_attempt_unknown", "原工具发送边界已登记")
			}
			invocation.StartedAt = time.Now().UTC().Format(time.RFC3339Nano)
			run.Control.GitInvocations[reserved.AttemptID] = invocation
			if policy.CommandID == "git.push" {
				if run.Control.RemoteWriteStarted[x.ExecutionID] {
					return fail("reconciliation_required", "原Git push已进入发送边界")
				}
				if run.Control.RemoteWriteStarted == nil {
					run.Control.RemoteWriteStarted = map[string]bool{}
				}
				run.Control.RemoteWriteStarted[x.ExecutionID] = true
			}
			return nil
		})
		if err != nil {
			cancel()
			return err
		}
		output := newControlToolOutput([]string{s.Config.GitLabToken, s.Config.AIKey, controlGitAuthorization(s.Config.GitLabToken), strings.TrimPrefix(controlGitAuthorization(s.Config.GitLabToken), "Basic ")})
		cmd.Stdout, cmd.Stderr = output, output
		executionErr := cmd.Run()
		end := time.Now().UTC()
		status := "succeeded"
		var code *int64
		if cmd.ProcessState != nil {
			value := int64(cmd.ProcessState.ExitCode())
			code = &value
		}
		if bounded.Err() != nil {
			status = "interrupted"
		} else if code == nil {
			status = "unknown"
		} else if *code != 0 {
			status = "failed"
		}
		cancel()
		text, truncated := output.finish()
		receipt, e := controlGitReceipt(r, x, invocation, status, code, text, truncated, end)
		if e != nil {
			return e
		}
		candidate, e = s.controlSourceFiles(r, paths.candidate, true)
		if e == nil && chainEqual(candidate, approved) {
			ref := *receipt.InputManifestRef
			receipt.OutputManifestRef = &contracts.ToolExecutionReceiptOutputManifestRef{ObjectType: ref.ObjectType, ObjectID: ref.ObjectID, ObjectVersion: ref.ObjectVersion, SourceVersion: ref.SourceVersion, Hash: ref.Hash}
		}
		if err = s.persistControlGitReceipt(r.ID, x, reserved, receipt, false); err != nil {
			return err
		}
		if e != nil {
			return e
		}
		if executionErr != nil {
			return fail("tool_"+status, "固定Git命令未成功；原尝试不可重放")
		}
		if policy.CommandID == "git.tree" {
			if truncated || controlGitTreeMatches(text, approved) != nil {
				return fail("git_tree_mismatch", "实际Git tree不等于完整批准文件")
			}
		}
	}
	return nil
}

func controlGitReceipt(r Run, x contracts.ExecutionRequest, in controlGitInvocation, status string, code *int64, output string, truncated bool, completed time.Time) (contracts.ToolExecutionReceipt, error) {
	approval, manifest := controlGitLabProvenance(r, x)
	if approval == nil || manifest == nil {
		return contracts.ToolExecutionReceipt{}, remoteReceiptError("original Git provenance")
	}
	args, err := json.Marshal(in.Arguments)
	q := contracts.ToolExecutionReceipt{ContractVersion: x.ContractVersion, ReceiptID: "receipt_" + strings.TrimPrefix(in.AttemptID, "tool_"), RunID: x.RunID, SourceVersion: x.SourceVersion, DefinitionSnapshotHash: x.DefinitionSnapshotHash, ExecutionID: x.ExecutionID, OperationID: x.OperationID, AttemptID: in.AttemptID, RequestHash: x.RequestHash, ApprovalID: approval.ApprovalID, ApprovalHash: approval.ApprovalHash, ProposalSnapshotHash: x.Approval.ProposalSnapshotHash, PlanHash: x.PlanHash, Workspace: x.Workspace, Tool: in.Tool, CommandID: in.CommandID, Arguments: args, CwdScope: "run_workspace", EnvironmentPolicy: "isolated_git", InputManifestRef: &contracts.ToolExecutionReceiptInputManifestRef{ObjectType: manifest.ObjectType, ObjectID: manifest.ObjectID, ObjectVersion: manifest.ObjectVersion, SourceVersion: manifest.SourceVersion, Hash: manifest.Hash}, StartedAt: in.StartedAt, CompletedAt: completed.Format(time.RFC3339Nano), ExitCode: code, Status: status, Output: output, OutputSHA256: contracts.BytesHash([]byte(output)), OutputTruncated: truncated}
	return q, err
}

func (s *Service) persistControlGitReceipt(id string, x contracts.ExecutionRequest, reservation contracts.BudgetReservation, q contracts.ToolExecutionReceipt, unknown bool) error {
	var err error
	q.ReceiptHash, err = contracts.HashObject("ToolExecutionReceipt", q)
	if err != nil {
		return err
	}
	if err = contracts.Validate("ToolExecutionReceipt", q); err != nil {
		return err
	}
	_, err = s.Store.Update(id, func(r *Run) error {
		for _, prior := range r.Control.Audit.ToolReceipts {
			if prior.AttemptID == q.AttemptID {
				if chainEqual(prior, q) {
					return nil
				}
				return remoteReceiptError("immutable Git attempt result")
			}
		}
		start, _ := time.Parse(time.RFC3339Nano, q.StartedAt)
		end, _ := time.Parse(time.RFC3339Nano, q.CompletedAt)
		elapsed := end.Sub(start).Milliseconds()
		in := governance.SettlementInput{ReservationID: reservation.ReservationID, AttemptID: reservation.AttemptID, Outcome: q.Status, ElapsedMS: &elapsed, MeasurementSource: "actual", Now: time.Now().UTC()}
		if unknown {
			in.Outcome, in.MeasurementSource, in.ElapsedMS = "unknown", "unknown", nil
		}
		var e error
		r.Control.Audit.Budget, _, e = governance.SettleBudget(r.FrozenDefinition.Definition.BudgetPolicy, r.Control.Audit.Budget, in)
		if e != nil {
			return e
		}
		r.Control.Audit.ToolReceipts = append(r.Control.Audit.ToolReceipts, q)
		for i := range r.Control.Audit.Operations {
			if r.Control.Audit.Operations[i].OperationID == x.OperationID {
				r.Control.Audit.Operations[i].EvidenceRefs = append(r.Control.Audit.Operations[i].EvidenceRefs, contracts.ObjectReference{ObjectType: "evidence", ObjectID: q.ReceiptID, ObjectVersion: 1, SourceVersion: q.SourceVersion, Hash: q.ReceiptHash})
			}
		}
		syncBudgetProjection(r)
		return nil
	})
	return err
}
