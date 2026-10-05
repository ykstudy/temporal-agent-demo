package agent

import (
	"context"
	"encoding/json"
	"os"
	"sort"
	"sync"
	"time"

	"agentstudio/internal/contracts"
	"agentstudio/internal/governance"
)

// ControlToolsResult is an internal executor result. Receipts are the complete
// public contract; Candidate is never an approved manifest or a DONE decision.
type ControlToolsResult struct {
	Receipts           []contracts.ToolExecutionReceipt
	ReapprovalRequired bool
	Candidate          *ControlToolCandidate
}

type ControlToolCandidate struct {
	Directory    string
	ChangedPaths []string
	Files        map[string]string
}

// ExecuteControlTools consumes an already reserved VERIFY action attempt. It
// never modifies the approved directory. The scheduler owns the outer action
// settlement, approval lifecycle and subsequent state transition.
func (s *Service) ExecuteControlTools(ctx context.Context, supplied Run, request contracts.ExecutionRequest, commands []contracts.PostCommand, attemptID string) (ControlToolsResult, error) {
	result := ControlToolsResult{Receipts: []contracts.ToolExecutionReceipt{}}
	if !safeID.MatchString(supplied.ID) || !safeID.MatchString(attemptID) || supplied.ID != request.RunID {
		return result, fail("invalid_tool_attempt", "工具尝试必须绑定真实Run和固定父尝试")
	}
	s.mu.Lock()
	if s.commandLocks == nil {
		s.commandLocks = map[string]*sync.Mutex{}
	}
	key := "control-tools:" + supplied.ID
	lock := s.commandLocks[key]
	if lock == nil {
		lock = &sync.Mutex{}
		s.commandLocks[key] = lock
	}
	s.mu.Unlock()
	lock.Lock()
	defer lock.Unlock()
	r, err := s.Store.Get(supplied.ID)
	if err != nil {
		return result, err
	}
	if r.SourceVersion != supplied.SourceVersion {
		return result, fail("stale_source", "工具执行来源已改变")
	}
	tool, binary, err := controlGoIdentity(r)
	if err != nil {
		return result, err
	}
	if err = s.controlToolFacts(r, request, attemptID, tool); err != nil {
		return result, err
	}
	module, err := controlFrozenModuleEnvironment(r)
	if err != nil {
		return result, err
	}
	merged, err := governance.MergePostCommands(*r.FrozenDefinition, *r.Control.Audit.Plan)
	if err != nil {
		return result, err
	}
	if !equalContractValue(merged, commands) || len(commands) == 0 {
		return result, fail("invalid_tool_command", "必须完整执行冻结合并命令，不能改变参数、顺序或限制")
	}
	paths, err := s.controlToolPaths(r, request, attemptID)
	if err != nil {
		return result, err
	}
	approved, err := s.controlToolFiles(r, paths.approved)
	if err != nil {
		return result, err
	}
	if err = controlMatchApproved(r, approved); err != nil {
		return result, err
	}
	if err = paths.prepare(); err != nil {
		return result, err
	}
	// Only a brand new parent attempt may copy the approved source. An existing
	// candidate is preserved for receipt replay and crash/unknown examination.
	if _, err = os.Lstat(paths.candidate); os.IsNotExist(err) {
		if err = controlCopyFiles(paths.candidate, approved); err != nil {
			return result, err
		}
	} else if err != nil {
		return result, err
	}
	if _, err = controlSandboxCommand(ctx, paths, binary, nil, "go.test", module); err != nil {
		return result, err
	}
	for _, command := range commands {
		r, err = s.Store.Get(r.ID)
		if err != nil {
			return result, err
		}
		if err = s.controlToolFacts(r, request, attemptID, tool); err != nil {
			return result, err
		}
		actual, e := s.controlToolFiles(r, paths.approved)
		if e != nil {
			return result, e
		}
		if !equalContractValue(actual, approved) {
			return result, fail("artifact_source_mismatch", "工具执行期间批准工作区发生变化")
		}
		childHash, _ := contracts.HashValue([]string{request.RequestHash, attemptID, command.CommandID})
		childAttempt := "tool_" + childHash[:40]
		var previous *contracts.ToolExecutionReceipt
		var reservation *contracts.BudgetReservation
		for _, value := range r.Control.Audit.Budget.Reservations {
			if value.AttemptID == childAttempt {
				copy := value
				reservation = &copy
			}
		}
		for _, value := range r.Control.Audit.ToolReceipts {
			if value.AttemptID == childAttempt {
				copy := value
				previous = &copy
			}
		}
		if previous != nil {
			if reservation == nil || s.controlVerifyToolReceipt(r, request, command, tool, *previous, *reservation) != nil {
				return result, fail("tool_receipt_mismatch", "原工具回执不能证明当前准确尝试")
			}
			if err = s.controlSettleTool(r.ID, *reservation, *previous); err != nil {
				return result, err
			}
			result.Receipts = append(result.Receipts, *previous)
			if previous.Status != "succeeded" {
				return result, fail("tool_"+previous.Status, "原工具尝试未成功，不能以相同身份重放")
			}
			if previous.OutputManifestRef == nil {
				return s.controlCandidateResult(r, paths, approved, result)
			}
			continue
		}
		if reservation != nil {
			// A crash can occur on either side of process Start. Absence of a
			// receipt never proves the tool did not run and cannot release quota.
			_, _, settleErr := s.settleControlBudget(r.ID, governance.SettlementInput{ReservationID: reservation.ReservationID, AttemptID: childAttempt, Outcome: "unknown", MeasurementSource: "unknown", Now: time.Now().UTC()})
			if settleErr != nil {
				return result, settleErr
			}
			return result, fail("tool_attempt_unknown", "已有工具预留缺少真实回执，禁止盲重放；需核实或同源新尝试")
		}
		candidate, e := s.controlToolFiles(r, paths.candidate)
		if e != nil {
			return result, e
		}
		if !equalContractValue(candidate, approved) {
			return s.controlCandidateResult(r, paths, approved, result)
		}
		if err = ctx.Err(); err != nil {
			return result, err
		}
		var reserved contracts.BudgetReservation
		r, reserved, err = s.reserveControlBudget(r.ID, governance.ReservationInput{SourceVersion: request.SourceVersion, CommandID: request.CommandID, AttemptID: childAttempt, Kind: "tool", ScopeID: "tool:" + command.CommandID, Request: &request, Tool: &command, Now: time.Now().UTC()})
		if err != nil {
			return result, err
		}
		deadline, e := time.Parse(time.RFC3339Nano, *reserved.DeadlineAt)
		if e != nil {
			return result, e
		}
		commandCtx, cancel := context.WithDeadline(ctx, deadline)
		receipt, executionErr := s.controlRunTool(commandCtx, r, request, command, tool, binary, paths, reserved)
		cancel()
		if receipt.ReceiptID == "" {
			return result, executionErr
		}
		candidate, e = s.controlToolFiles(r, paths.candidate)
		if e == nil && equalContractValue(candidate, approved) {
			ref := controlManifestRef(*r.Control.Audit.Manifest)
			receipt.OutputManifestRef = &contracts.ToolExecutionReceiptOutputManifestRef{ObjectType: ref.ObjectType, ObjectID: ref.ObjectID, ObjectVersion: ref.ObjectVersion, SourceVersion: ref.SourceVersion, Hash: ref.Hash}
		}
		receipt.ReceiptHash, err = contracts.HashObject("ToolExecutionReceipt", receipt)
		if err != nil {
			return result, err
		}
		if err = contracts.Validate("ToolExecutionReceipt", receipt); err != nil {
			return result, err
		}
		_, err = s.Store.Update(r.ID, func(run *Run) error {
			for _, old := range run.Control.Audit.ToolReceipts {
				if old.ReceiptID == receipt.ReceiptID {
					if equalContractValue(old, receipt) {
						return nil
					}
					return fail("tool_receipt_mismatch", "工具回执身份不可覆盖")
				}
			}
			run.Control.Audit.ToolReceipts = append(run.Control.Audit.ToolReceipts, receipt)
			return nil
		})
		if err != nil {
			return result, err
		}
		if err = s.controlSettleTool(r.ID, reserved, receipt); err != nil {
			return result, err
		}
		result.Receipts = append(result.Receipts, receipt)
		if e != nil {
			return result, e
		}
		if !equalContractValue(candidate, approved) {
			return s.controlCandidateResult(r, paths, approved, result)
		}
		if executionErr != nil {
			return result, executionErr
		}
	}
	r, err = s.Store.Get(r.ID)
	if err != nil {
		return result, err
	}
	if err = s.controlToolFacts(r, request, attemptID, tool); err != nil {
		return result, err
	}
	actual, err := s.controlToolFiles(r, paths.approved)
	if err != nil {
		return result, err
	}
	if !equalContractValue(actual, approved) {
		return result, fail("artifact_source_mismatch", "批准内容在验证期间改变")
	}
	return result, nil
}

func (s *Service) controlToolFacts(r Run, request contracts.ExecutionRequest, attemptID string, tool contracts.ToolIdentity) error {
	if r.Control == nil || r.FrozenDefinition == nil || r.State != "VERIFY" || r.PauseRequested || r.CancelRequested || r.Control.Audit.Manifest == nil {
		return fail("invalid_tool_stage", "完整VERIFY来源和已登记manifest必需")
	}
	if r.ActiveStep != "" && r.ActiveStep != request.ExecutionID {
		return fail("execution_binding_mismatch", "当前活跃尝试不属于此VERIFY请求")
	}
	if err := s.verifyFrozenDefinition(r); err != nil {
		return err
	}
	if err := verifyControlChain(r); err != nil {
		return err
	}
	if err := governance.ValidateBudgetLedger(r.FrozenDefinition.Definition.BudgetPolicy, r.Control.Audit.Budget); err != nil {
		return err
	}
	var action struct {
		ActionType string `json:"action_type"`
	}
	if json.Unmarshal(request.Action, &action) != nil || action.ActionType != "verify" {
		return fail("invalid_tool_stage", "Go后置命令只能执行受控verify请求")
	}
	var original *governance.Input
	var validation *contracts.ActionValidation
	registered := false
	for _, x := range r.Control.Audit.Executions {
		if x.ExecutionID == request.ExecutionID {
			if !equalContractValue(x, request) {
				return fail("execution_binding_mismatch", "请求与不可变登记内容不符")
			}
			registered = true
		}
	}
	if !registered {
		return fail("execution_binding_mismatch", "执行请求未持久登记")
	}
	for i := range r.Control.Audit.Validations {
		if r.Control.Audit.Validations[i].ValidationID == request.ValidationID {
			validation = &r.Control.Audit.Validations[i]
		}
	}
	if validation != nil {
		for i := range r.Control.ValidationInputs {
			if r.Control.ValidationInputs[i].Decision.DecisionID == validation.DecisionID {
				original = &r.Control.ValidationInputs[i]
			}
		}
	}
	if original == nil || validation == nil {
		return fail("execution_binding_mismatch", "请求缺少原始决定和确定性校验来源")
	}
	ledger := r.Control.Audit.Budget
	var owned *contracts.BudgetReservation
	for i := range ledger.Reservations {
		p := &ledger.Reservations[i]
		if p.AttemptID == attemptID && p.Kind == "action" && p.ScopeID == "verify" && p.RequestHash != nil && *p.RequestHash == request.RequestHash && p.OperationID != nil && *p.OperationID == request.OperationID && p.ExecutionID != nil && *p.ExecutionID == request.ExecutionID && p.CommandID == request.CommandID && p.SourceVersion == request.SourceVersion {
			owned = p
		}
	}
	if owned == nil || ledger.ExecutionDeadlineAt == nil {
		return fail("tool_parent_unreserved", "工具需要当前VERIFY父尝试的持久预算预留")
	}
	for _, settled := range ledger.Settlements {
		if settled.ReservationID == owned.ReservationID {
			return fail("tool_parent_closed", "父尝试已结算，不能增加工具执行")
		}
	}
	operationFound := false
	for _, operation := range r.Control.Audit.Operations {
		if operation.OperationID != request.OperationID {
			continue
		}
		target, _ := governance.RequestTargetHash(request)
		if operation.ActionType != "verify" || operation.RunID != request.RunID || operation.SourceVersion != request.SourceVersion || operation.RequestHash != request.RequestHash || operation.TargetHash != target || (operation.Status != "ready" && operation.Status != "sent") {
			return fail("operation_conflict", "当前操作不允许启动此候选工具尝试")
		}
		operationFound = true
	}
	if !operationFound {
		return fail("operation_conflict", "VERIFY父操作未登记")
	}
	facts := original.Facts
	facts.State, facts.SourceVersion, facts.StateSequence, facts.Now = r.State, int64(r.SourceVersion), int64(r.StateSequence), time.Now().UTC()
	facts.Modes, facts.FrozenDefinition = runModes(r), *r.FrozenDefinition
	facts.Plan, facts.Proposal, facts.Approval, facts.Permission, facts.Manifest = r.Control.Audit.Plan, r.Control.Audit.Proposal, r.Control.Audit.Approval, r.Control.Audit.PermissionSnapshot, r.Control.Audit.Manifest
	facts.AvailableTools = []contracts.ToolIdentity{tool}
	if contracts.DeliveryContract(r.FrozenDefinition.ContractVersion) {
		git, _, err := controlGitIdentity(r)
		if err != nil {
			return err
		}
		http, _, err := controlGitHTTPIdentity(r)
		if err != nil {
			return err
		}
		facts.AvailableTools = append(facts.AvailableTools, git, http)
	}
	// The exact pending parent reservation is admission already paid for. A
	// matching sent VERIFY describes this local attempt, not a new remote write.
	facts.Operation = nil
	deadline, err := time.Parse(time.RFC3339Nano, *ledger.ExecutionDeadlineAt)
	if err != nil {
		return err
	}
	facts.Budget = governance.BudgetFacts{Available: true, RemainingAttempts: 1, ExecutionDeadline: &deadline, ToolDeadline: &deadline}
	return (governance.Validator{Definitions: s.Definitions}).Revalidate(request, *original, *validation, facts)
}

func controlManifestRef(m contracts.ArtifactManifest) contracts.ObjectReference {
	return contracts.ObjectReference{ObjectType: "manifest", ObjectID: m.ManifestID, ObjectVersion: m.ManifestVersion, SourceVersion: m.SourceVersion, Hash: m.Hash}
}

func (s *Service) controlSettleTool(id string, reservation contracts.BudgetReservation, receipt contracts.ToolExecutionReceipt) error {
	start, _ := time.Parse(time.RFC3339Nano, receipt.StartedAt)
	end, _ := time.Parse(time.RFC3339Nano, receipt.CompletedAt)
	elapsed := end.Sub(start).Milliseconds()
	outcome := receipt.Status
	if outcome == "timed_out" {
		outcome = "failed"
	}
	_, _, err := s.settleControlBudget(id, governance.SettlementInput{ReservationID: reservation.ReservationID, AttemptID: reservation.AttemptID, Outcome: outcome, ElapsedMS: &elapsed, MeasurementSource: "actual", Now: time.Now().UTC()})
	return err
}

func (s *Service) controlVerifyToolReceipt(r Run, request contracts.ExecutionRequest, command contracts.PostCommand, tool contracts.ToolIdentity, receipt contracts.ToolExecutionReceipt, reservation contracts.BudgetReservation) error {
	if !hashMatches("ToolExecutionReceipt", receipt, receipt.ReceiptHash) || receipt.RunID != r.ID || receipt.SourceVersion != request.SourceVersion || receipt.DefinitionSnapshotHash != request.DefinitionSnapshotHash || receipt.ExecutionID != request.ExecutionID || receipt.OperationID != request.OperationID || receipt.RequestHash != request.RequestHash || receipt.ApprovalID != request.Approval.ApprovalID || receipt.ApprovalHash != r.Control.Audit.Approval.ApprovalHash || receipt.ProposalSnapshotHash != request.Approval.ProposalSnapshotHash || receipt.PlanHash != request.PlanHash || !equalContractValue(receipt.Workspace, request.Workspace) || !equalContractValue(receipt.Tool, tool) || receipt.CommandID != command.CommandID || !equalContractValue(receipt.Arguments, command.Arguments) || receipt.AttemptID != reservation.AttemptID || reservation.RequestHash == nil || *reservation.RequestHash != request.RequestHash || reservation.ScopeID != "tool:"+command.CommandID || receipt.OutputSHA256 != contracts.BytesHash([]byte(receipt.Output)) || len(receipt.Output) > 65536 || !chainBefore(reservation.ReservedAt, receipt.StartedAt, false) || !chainBefore(receipt.StartedAt, receipt.CompletedAt, false) {
		return fail("tool_receipt_mismatch", "回执来源、命令、计时或输出不符")
	}
	ref := controlManifestRef(*r.Control.Audit.Manifest)
	if receipt.InputManifestRef == nil || !equalContractValue(*receipt.InputManifestRef, ref) || (receipt.OutputManifestRef != nil && !equalContractValue(*receipt.OutputManifestRef, ref)) {
		return fail("tool_receipt_mismatch", "回执manifest不是当前已登记批准内容")
	}
	return nil
}

func (s *Service) controlCandidateResult(r Run, paths controlToolPaths, approved map[string]string, result ControlToolsResult) (ControlToolsResult, error) {
	files, err := s.controlToolFiles(r, paths.candidate)
	if err != nil {
		return result, err
	}
	changed := map[string]bool{}
	for name, content := range approved {
		if value, ok := files[name]; !ok || value != content {
			changed[name] = true
		}
	}
	for name, content := range files {
		if value, ok := approved[name]; !ok || value != content {
			changed[name] = true
		}
	}
	if len(changed) == 0 {
		return result, fail("candidate_evidence_mismatch", "缺少输出manifest的原回执不能以当前文件替代历史验证")
	}
	names := make([]string, 0, len(changed))
	for name := range changed {
		names = append(names, name)
	}
	sort.Strings(names)
	result.ReapprovalRequired = true
	result.Candidate = &ControlToolCandidate{Directory: paths.candidate, ChangedPaths: names, Files: files}
	return result, nil
}
