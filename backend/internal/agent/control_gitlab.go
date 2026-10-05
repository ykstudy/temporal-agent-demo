package agent

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"agentstudio/internal/contracts"
	"agentstudio/internal/governance"
)

// Transport observations are private facts, not replacement public DTOs. A
// create result proves an empty project's identity, never a branch or commit.
type controlGitLabProject struct {
	ID                int64   `json:"id"`
	Path              string  `json:"path"`
	PathWithNamespace string  `json:"path_with_namespace"`
	Description       string  `json:"description"`
	Visibility        string  `json:"visibility"`
	DefaultBranch     *string `json:"default_branch"`
	EmptyRepo         *bool   `json:"empty_repo"`
	Namespace         struct {
		ID       int64  `json:"id"`
		FullPath string `json:"full_path"`
	} `json:"namespace"`
}

func controlGitLabMarker(x contracts.ExecutionRequest) string {
	return "agentstudio:" + x.OperationID + ":" + x.RequestHash
}

// Complete remote executors consume the original registered action; unsupported
// kinds fail before admission or network I/O.
func (s *Service) ExecuteControlGitLab(ctx context.Context, id string, x contracts.ExecutionRequest) (Run, error) {
	if kind, _, _ := scheduleAction(x.Action); kind == "trigger_ci" && contracts.DeliveryContract(x.ContractVersion) {
		return s.executeControlCI(ctx, id, x, false)
	}
	if kind, _, _ := scheduleAction(x.Action); kind == "push_repository" && contracts.DeliveryContract(x.ContractVersion) {
		return s.executeControlPush(ctx, id, x, false)
	}
	r, err := s.Store.Get(id)
	if err != nil {
		return r, err
	}
	target, op, err := s.controlGitLabRequest(r, x)
	if err != nil {
		return r, err
	}
	if op.Status == "sent" || op.Status == "unknown" {
		return r, fail("reconciliation_required", "原操作结果未知，只允许按原身份只读核实")
	}
	if op.Status == "verified" {
		if err = controlGitLabExistingResult(r, x); err == nil {
			return r, nil
		}
		return r, err
	}
	if op.Status != "ready" || r.ActiveStep != "" || r.PauseRequested || r.CancelRequested || terminal(r.State) || r.State == "PAUSED" || r.State == "BLOCKED" {
		return r, fail("execution_stopped", "当前Run不允许新GitLab写入")
	}
	if err = s.recheckControlApproval(r, time.Now().UTC()); err != nil {
		return r, err
	}
	next, err := NextControlAction(r, r.Control.Schedule)
	if err != nil {
		return r, err
	}
	if next.Execution == nil || next.Action == nil || next.NeedsReconciliation || !equalContractValue(*next.Execution, x) {
		return r, fail("execution_binding_mismatch", "GitLab请求不是准确下一动作")
	}
	// A namespace read must succeed before a precise project 404 is meaningful.
	var namespace struct {
		ID       int64  `json:"id"`
		FullPath string `json:"full_path"`
	}
	if _, err = s.controlGitLabRead(ctx, id, x, "/namespaces/"+url.PathEscape(target.NamespaceID), &namespace); err != nil {
		return s.controlGitLabBlock(id, x, false, "gitlab_namespace_unavailable", err)
	}
	if strconv.FormatInt(namespace.ID, 10) != target.NamespaceID || namespace.FullPath != target.NamespacePath {
		return s.controlGitLabBlock(id, x, false, "gitlab_namespace_mismatch", fail("target_mismatch", "GitLab namespace身份与批准目标不符"))
	}
	var existing controlGitLabProject
	status, readErr := s.controlGitLabRead(ctx, id, x, controlGitLabProjectPath(target), &existing)
	if status != http.StatusNotFound {
		if readErr == nil {
			readErr = fail("target_conflict", "准确路径已存在项目；未发送的操作不能接管既有项目")
		}
		return s.controlGitLabBlock(id, x, false, "gitlab_target_unavailable", readErr)
	}
	// Admission is the final authority check and durable sent/attempt boundary.
	// Losing the process after this point always requires read-only reconciliation.
	r, reservation, err := s.beginControlExecution(id, x)
	if err != nil {
		return r, err
	}
	bounded, cancel, err := controlGitLabDeadline(ctx, reservation)
	if err != nil {
		return s.controlGitLabBlock(id, x, true, "gitlab_write_unknown", err)
	}
	defer cancel()
	if contracts.DeliveryContract(x.ContractVersion) {
		if err = s.controlGitLabDispatch(id, x); err != nil {
			return s.controlGitLabBlock(id, x, true, "gitlab_dispatch_stopped", err)
		}
	}
	var created controlGitLabProject
	_, err = s.gitlab(bounded, http.MethodPost, "/projects", struct {
		NamespaceID string `json:"namespace_id"`
		Name        string `json:"name"`
		Path        string `json:"path"`
		Visibility  string `json:"visibility"`
		Description string `json:"description"`
		Initialize  bool   `json:"initialize_with_readme"`
	}{target.NamespaceID, target.ProjectPath, target.ProjectPath, target.Visibility, controlGitLabMarker(x), false}, &created)
	if err != nil {
		return s.controlGitLabBlock(id, x, true, "gitlab_write_unknown", err)
	}
	if created.ID <= 0 || created.ID > 9007199254740991 {
		return s.controlGitLabBlock(id, x, true, "gitlab_write_unknown", fail("invalid_gitlab_response", "建仓响应缺少准确项目身份"))
	}
	// Retain the returned numeric identity before the follow-up query. It is not
	// success evidence until the exact complete remote target is verified.
	r, err = s.Store.Update(id, func(run *Run) error {
		_, op, e := s.controlGitLabRequest(*run, x)
		if e != nil {
			return e
		}
		if op.Status == "verified" {
			return controlGitLabExistingResult(*run, x)
		}
		value := strconv.FormatInt(created.ID, 10)
		if op.ExternalObjectID != nil && *op.ExternalObjectID != value {
			return fail("operation_conflict", "原操作已登记另一外部项目身份")
		}
		op.ExternalObjectID = &value
		op.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
		return nil
	})
	if err != nil {
		return r, err // persistence failure cannot authorize another POST.
	}
	var observed controlGitLabProject
	if _, err = s.controlGitLabRead(ctx, id, x, "/projects/"+strconv.FormatInt(created.ID, 10), &observed); err != nil {
		return s.controlGitLabBlock(id, x, true, "gitlab_read_after_write_unknown", err)
	}
	if err = controlGitLabVerifyProject(target, x, observed); err != nil || observed.ID != created.ID {
		if err == nil {
			err = fail("target_mismatch", "GitLab返回了不同的项目身份")
		}
		return s.controlGitLabBlock(id, x, true, "gitlab_result_mismatch", err)
	}
	return s.controlGitLabComplete(id, x, target, observed)
}

// ReconcileControlGitLab never sends an external write. Even after a source,
// permission or terminal-state change it can observe the old immutable request,
// but it cannot restore authorization or move that Run back into execution.
func (s *Service) ReconcileControlGitLab(ctx context.Context, id string, x contracts.ExecutionRequest) (Run, error) {
	if kind, _, _ := scheduleAction(x.Action); kind == "trigger_ci" && contracts.DeliveryContract(x.ContractVersion) {
		return s.executeControlCI(ctx, id, x, true)
	}
	if kind, _, _ := scheduleAction(x.Action); kind == "push_repository" && contracts.DeliveryContract(x.ContractVersion) {
		return s.executeControlPush(ctx, id, x, true)
	}
	r, err := s.Store.Get(id)
	if err != nil {
		return r, err
	}
	target, op, err := s.controlGitLabRequest(r, x)
	if err != nil {
		return r, err
	}
	if op.Status == "verified" {
		return r, controlGitLabExistingResult(r, x)
	}
	if op.Status != "sent" && op.Status != "unknown" {
		return r, fail("reconciliation_not_required", "只读核实要求原sent或unknown操作")
	}
	// A crash can leave an unsettled reservation. Its charge becomes unknown,
	// never released; observation has its own separately metered attempt.
	if _, err = s.controlGitLabBlock(id, x, true, "gitlab_reconciliation_required", nil); err != nil {
		return r, err
	}
	path := controlGitLabProjectPath(target)
	if op.ExternalObjectID != nil {
		project, e := strconv.ParseInt(*op.ExternalObjectID, 10, 64)
		if e != nil || project <= 0 {
			return r, fail("operation_conflict", "登记的项目身份无效")
		}
		path = "/projects/" + strconv.FormatInt(project, 10)
	}
	var observed controlGitLabProject
	if _, err = s.controlGitLabRead(ctx, id, x, path, &observed); err != nil {
		return s.controlGitLabBlock(id, x, true, "gitlab_reconciliation_unknown", err)
	}
	if err = controlGitLabVerifyProject(target, x, observed); err != nil {
		return s.controlGitLabBlock(id, x, true, "gitlab_reconciliation_conflict", err)
	}
	if op.ExternalObjectID != nil && *op.ExternalObjectID != strconv.FormatInt(observed.ID, 10) {
		return s.controlGitLabBlock(id, x, true, "gitlab_reconciliation_conflict", fail("target_mismatch", "核实响应不是原登记项目"))
	}
	return s.controlGitLabComplete(id, x, target, observed)
}

func controlGitLabProjectPath(target contracts.GitLabTarget) string {
	return "/projects/" + url.PathEscape(target.NamespacePath+"/"+target.ProjectPath)
}

func controlGitLabVerifyProject(target contracts.GitLabTarget, x contracts.ExecutionRequest, p controlGitLabProject) error {
	if p.ID <= 0 || p.ID > 9007199254740991 || p.Path != target.ProjectPath || p.PathWithNamespace != target.NamespacePath+"/"+target.ProjectPath || strconv.FormatInt(p.Namespace.ID, 10) != target.NamespaceID || p.Namespace.FullPath != target.NamespacePath || p.Visibility != target.Visibility || p.Description != controlGitLabMarker(x) || p.EmptyRepo == nil || !*p.EmptyRepo || p.DefaultBranch != nil && *p.DefaultBranch != "" && *p.DefaultBranch != target.Ref {
		return fail("target_mismatch", "项目身份、归属或空仓库状态与原创建操作不符")
	}
	return nil
}

func (s *Service) controlGitLabRequest(r Run, x contracts.ExecutionRequest) (contracts.GitLabTarget, *contracts.OperationRecord, error) {
	var target contracts.GitLabTarget
	kind, raw, err := scheduleAction(x.Action)
	if err != nil {
		return target, nil, err
	}
	switch {
	case kind == "create_repository":
		var parameters contracts.CreateRepositoryParameters
		err = contracts.DecodeVersion("CreateRepositoryParameters", x.ContractVersion, raw, &parameters)
		target = parameters.Target
	case kind == "push_repository" && contracts.DeliveryContract(x.ContractVersion):
		var parameters contracts.PushRepositoryParameters
		err = contracts.DecodeVersion("PushRepositoryParameters", x.ContractVersion, raw, &parameters)
		target = parameters.Target
	case kind == "trigger_ci" && contracts.DeliveryContract(x.ContractVersion):
		var parameters contracts.TriggerCIParameters
		err = contracts.DecodeVersion("TriggerCIParameters", x.ContractVersion, raw, &parameters)
		target = parameters.Target
	default:
		return target, nil, fail("control_gitlab_action_not_ready", "此受控GitLab动作尚未接入执行器")
	}
	if err != nil {
		return target, nil, err
	}
	if r.Control == nil || r.FrozenDefinition == nil || r.ID != x.RunID || r.DeliveryMode != "gitlab" || r.ExecutionMode != s.Config.Mode || x.DefinitionSnapshotHash != r.FrozenDefinition.SnapshotHash || !hashMatches("ExecutionRequest", x, x.RequestHash) {
		return target, nil, fail("execution_binding_mismatch", "GitLab请求没有完整Run冻结来源")
	}
	if s.Config.GitLabToken == "" || target.InstanceURL != s.Config.GitLabURL || target.NamespaceID != s.Config.GitLabNamespace {
		return target, nil, fail("target_changed", "可信GitLab实例、凭据或namespace配置与原目标不同")
	}
	if err = verifyControlChain(r); err != nil {
		return target, nil, err
	}
	found := false
	for _, original := range r.Control.Audit.Executions {
		if original.ExecutionID == x.ExecutionID && equalContractValue(original, x) {
			found = true
		}
	}
	if !found {
		return target, nil, fail("execution_binding_mismatch", "必须消费原文完全相同的已登记执行请求")
	}
	for i := range r.Control.Audit.Operations {
		op := &r.Control.Audit.Operations[i]
		targetHash, e := governance.RequestTargetHash(x)
		if e != nil {
			return target, nil, e
		}
		if op.OperationID == x.OperationID && op.RequestHash == x.RequestHash && op.SourceVersion == x.SourceVersion && op.TargetHash == targetHash && op.ActionType == kind {
			return target, op, nil
		}
	}
	return target, nil, fail("operation_not_found", "原请求没有完整操作登记")
}

func controlGitLabDeadline(ctx context.Context, reservation contracts.BudgetReservation) (context.Context, context.CancelFunc, error) {
	if reservation.DeadlineAt == nil {
		return nil, nil, fail("budget_deadline_missing", "GitLab尝试缺少持久期限")
	}
	deadline, err := time.Parse(time.RFC3339Nano, *reservation.DeadlineAt)
	if err != nil || !time.Now().UTC().Before(deadline) {
		return nil, nil, fail("budget_deadline_exceeded", "原GitLab尝试期限已耗尽")
	}
	bounded, cancel := context.WithDeadline(ctx, deadline)
	return bounded, cancel, nil
}

func (s *Service) controlGitLabRead(ctx context.Context, id string, x contracts.ExecutionRequest, path string, output any) (int, error) {
	return s.controlGitLabReadFact(ctx, id, x, path, output, nil)
}

func (s *Service) controlGitLabReadFact(ctx context.Context, id string, x contracts.ExecutionRequest, path string, output any, normalize func() (controlRemoteFact, error)) (int, error) {
	return s.controlRemoteRead(ctx, id, x, path, "reconciliation_read", output, func(bounded context.Context) (int, error) {
		return s.gitlab(bounded, http.MethodGet, path, nil, output)
	}, normalize)
}

// Both JSON and raw-artifact GETs consume the same durable read boundary.
// A failed fetch is retained as unavailable, with no response body persisted.
func (s *Service) controlRemoteRead(ctx context.Context, id string, x contracts.ExecutionRequest, path, kind string, output any, fetch func(context.Context) (int, error), normalize func() (controlRemoteFact, error)) (int, error) {
	var reserved contracts.BudgetReservation
	_, err := s.Store.Update(id, func(r *Run) error {
		if _, _, e := s.controlGitLabRequest(*r, x); e != nil {
			return e
		}
		if e := s.verifyFrozenDefinition(*r); e != nil {
			return e
		}
		var e error
		r.Control.Audit.Budget, reserved, e = governance.ReserveBudget(r.FrozenDefinition.Definition.BudgetPolicy, r.Control.Audit.Budget, governance.ReservationInput{SourceVersion: x.SourceVersion, CommandID: x.CommandID, AttemptID: newID("gitlab_read_"), Kind: kind, ScopeID: kind + ":" + x.OperationID, Request: &x, Now: time.Now().UTC()})
		syncBudgetProjection(r)
		return e
	})
	if err != nil {
		return 0, err
	}
	bounded, cancel, err := controlGitLabDeadline(ctx, reserved)
	started := time.Now().UTC()
	status := 0
	if err == nil {
		status, err = fetch(bounded)
		cancel()
	}
	if err == nil && normalize != nil {
		fact, e := normalize()
		err, output = e, &fact
	}
	completed := time.Now().UTC()
	var observation contracts.RemoteReadObservation
	if contracts.DeliveryContract(x.ContractVersion) {
		var observationErr error
		observation, observationErr = s.controlGitLabObservation(reserved, path, output, status, err, started, completed)
		err = errors.Join(err, observationErr)
	}
	outcome := "succeeded"
	if err != nil {
		outcome = "failed"
	}
	_, settleErr := s.Store.Update(id, func(r *Run) error {
		if e := settleControlInRun(r, reserved, outcome, time.Now().UTC()); e != nil {
			return e
		}
		if contracts.DeliveryContract(x.ContractVersion) {
			if r.Control.RemoteObservations == nil {
				r.Control.RemoteObservations = map[string][]contracts.RemoteReadObservation{}
			}
			r.Control.RemoteObservations[x.ExecutionID] = append(r.Control.RemoteObservations[x.ExecutionID], observation)
		}
		return nil
	})
	// A failed persistence operation must never turn an HTTP 404 into permission
	// to write; status zero prevents the caller from treating it as absence.
	if settleErr != nil {
		return 0, errors.Join(err, settleErr)
	}
	var infrastructure *InfrastructureError
	if err != nil && (errors.As(err, &infrastructure) || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)) {
		return 0, err
	}
	return status, err
}

func controlGitLabAttempt(r Run, x contracts.ExecutionRequest) (contracts.BudgetReservation, error) {
	var result contracts.BudgetReservation
	for _, v := range r.Control.Audit.Budget.Reservations {
		if v.Kind == "action" && v.ExecutionID != nil && *v.ExecutionID == x.ExecutionID && v.OperationID != nil && *v.OperationID == x.OperationID && v.RequestHash != nil && *v.RequestHash == x.RequestHash && v.SourceVersion == x.SourceVersion {
			if result.ReservationID != "" {
				return result, fail("operation_conflict", "原操作含多个不可区分的发送尝试")
			}
			result = v
		}
	}
	if result.ReservationID == "" {
		return result, fail("operation_conflict", "原操作没有持久发送尝试")
	}
	return result, nil
}

func controlGitLabSettled(r Run, reservation contracts.BudgetReservation) *contracts.BudgetSettlement {
	for _, settlement := range r.Control.Audit.Budget.Settlements {
		if settlement.ReservationID == reservation.ReservationID {
			copy := settlement
			return &copy
		}
	}
	return nil
}

func (s *Service) controlGitLabBlock(id string, x contracts.ExecutionRequest, unknown bool, reason string, cause error) (Run, error) {
	r, err := s.Store.Update(id, func(r *Run) error {
		_, op, e := s.controlGitLabRequest(*r, x)
		if e != nil {
			return e
		}
		if op.Status == "verified" {
			return controlGitLabExistingResult(*r, x)
		}
		if unknown {
			reservation, e := controlGitLabAttempt(*r, x)
			if e != nil {
				return e
			}
			if controlGitLabSettled(*r, reservation) == nil {
				r.Control.Audit.Budget, _, e = governance.SettleBudget(r.FrozenDefinition.Definition.BudgetPolicy, r.Control.Audit.Budget, governance.SettlementInput{ReservationID: reservation.ReservationID, AttemptID: reservation.AttemptID, Outcome: "unknown", MeasurementSource: "unknown", Now: time.Now().UTC()})
				if e != nil {
					return e
				}
			}
			op.Status = "unknown"
			op.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
		}
		if r.ActiveStep == x.ExecutionID {
			r.ActiveStep = ""
		}
		if !terminal(r.State) {
			r.ResumeTo = "DELIVER"
			switch {
			case r.CancelRequested:
				r.State = "CANCELLED"
			case r.PauseRequested:
				r.State = "PAUSED"
			default:
				r.State = "BLOCKED"
			}
		}
		r.Error = reason
		syncBudgetProjection(r)
		if unknown && contracts.DeliveryContract(x.ContractVersion) {
			return appendControlRemoteReceipt(r, x, "unknown", time.Now().UTC())
		}
		return nil
	})
	if err != nil {
		return r, errors.Join(cause, err)
	}
	return r, cause
}

func controlGitLabExistingResult(r Run, x contracts.ExecutionRequest) error {
	var target struct {
		Target contracts.GitLabTarget `json:"target"`
	}
	kind, raw, err := scheduleAction(x.Action)
	if err != nil || json.Unmarshal(raw, &target) != nil {
		return fail("operation_evidence_missing", "已核实操作缺少准确目标")
	}
	approval, manifest := controlGitLabProvenance(r, x)
	reservation, err := controlGitLabAttempt(r, x)
	if err != nil || approval == nil || manifest == nil {
		return fail("operation_evidence_missing", "已核实操作缺少原批准、工件或发送尝试")
	}
	settlement := controlGitLabSettled(r, reservation)
	if settlement == nil || settlement.ConsumedUnits != 1 || settlement.Outcome != "succeeded" && settlement.Outcome != "unknown" {
		return fail("operation_evidence_missing", "已核实操作缺少原尝试消费记录")
	}
	var op *contracts.OperationRecord
	for i := range r.Control.Audit.Operations {
		if r.Control.Audit.Operations[i].OperationID == x.OperationID {
			op = &r.Control.Audit.Operations[i]
		}
	}
	if op == nil || op.Status != "verified" || op.ExternalObjectID == nil {
		return fail("operation_evidence_missing", "操作尚未核实完整外部身份")
	}
	count := 0
	for _, result := range r.Control.Schedule.Results {
		if result.OperationID != x.OperationID {
			continue
		}
		count++
		external := strconv.FormatInt(result.ProjectID, 10)
		if kind == "push_repository" {
			external = result.CommitSHA
		}
		if kind == "trigger_ci" {
			external = strconv.FormatInt(result.PipelineID, 10)
		}
		hash, err := HashCanonicalActionResult(result)
		if err != nil || hash != result.ResultHash || result.ExecutionID != x.ExecutionID || result.RequestHash != x.RequestHash || result.RunID != x.RunID || result.SourceVersion != x.SourceVersion || result.DefinitionSnapshotHash != x.DefinitionSnapshotHash || result.PlanHash != x.PlanHash || result.ApprovalID != approval.ApprovalID || result.ApprovalHash != approval.ApprovalHash || result.ProposalSnapshotHash != x.Approval.ProposalSnapshotHash || result.TargetHash != op.TargetHash || result.AttemptID != reservation.AttemptID || result.ActionType != kind || result.ProjectID <= 0 || *op.ExternalObjectID != external || !chainEqual(result.ObservedTarget, &target.Target) || !chainEqual(result.ManifestRef, manifest) || kind == "create_repository" && (result.CommitSHA != "" || result.CIConfigSHA256 != "" || result.PipelineID != 0 || result.PipelineStatus != "") || result.PackageSHA256 != "" || result.PackageBytes != 0 || !chainBefore(reservation.ReservedAt, result.CompletedAt, false) || !chainBefore(result.CompletedAt, op.UpdatedAt, false) || settlement.Outcome == "succeeded" && !chainBefore(result.CompletedAt, settlement.SettledAt, false) {
			return fail("operation_evidence_missing", "已核实操作的结果来源或内容不完整")
		}
	}
	if count == 1 {
		if contracts.DeliveryContract(x.ContractVersion) {
			for _, result := range r.Control.Schedule.Results {
				if result.OperationID == x.OperationID {
					return controlRemoteResultReceipt(r, x, result)
				}
			}
		}
		return nil
	}
	return fail("operation_evidence_missing", "已核实操作缺少完整结果记录")
}

func controlGitLabProvenance(r Run, x contracts.ExecutionRequest) (*contracts.Approval, *contracts.ObjectReference) {
	var approval *contracts.Approval
	for _, a := range r.Control.Audit.ApprovalHistory {
		if a.ApprovalID == x.Approval.ApprovalID {
			copy := a
			approval = &copy
		}
	}
	var manifest *contracts.ObjectReference
	for _, in := range r.Control.ValidationInputs {
		if in.Facts.CommandID == x.CommandID && in.Facts.Manifest != nil {
			ref := scheduleManifestRef(*in.Facts.Manifest)
			manifest = &ref
		}
	}
	return approval, manifest
}

func (s *Service) controlGitLabComplete(id string, x contracts.ExecutionRequest, target contracts.GitLabTarget, observed controlGitLabProject) (Run, error) {
	return s.Store.Update(id, func(r *Run) error {
		_, op, err := s.controlGitLabRequest(*r, x)
		if err != nil {
			return err
		}
		if op.Status == "verified" {
			return controlGitLabExistingResult(*r, x)
		}
		if op.Status != "sent" && op.Status != "unknown" {
			return fail("operation_conflict", "只有原发送操作可登记观察结果")
		}
		if err = controlGitLabVerifyProject(target, x, observed); err != nil {
			return err
		}
		reservation, err := controlGitLabAttempt(*r, x)
		if err != nil {
			return err
		}
		completed := time.Now().UTC()
		settlement := controlGitLabSettled(*r, reservation)
		if settlement == nil {
			if err = settleControlInRun(r, reservation, "succeeded", completed); err != nil {
				return err
			}
		} else if settlement.Outcome != "unknown" {
			return fail("operation_conflict", "原发送尝试结算与观察结果冲突")
		}
		approval, manifest := controlGitLabProvenance(*r, x)
		if approval == nil || manifest == nil {
			return fail("operation_evidence_missing", "原批准和工件来源不可核实")
		}
		// Ref is the approved future branch. No commit, branch, CI or pipeline
		// fact is claimed by this create-only result for an observed empty repo.
		result := CanonicalActionResult{RunID: x.RunID, SourceVersion: x.SourceVersion, DefinitionSnapshotHash: x.DefinitionSnapshotHash, PlanHash: x.PlanHash, ApprovalID: approval.ApprovalID, ApprovalHash: approval.ApprovalHash, ProposalSnapshotHash: x.Approval.ProposalSnapshotHash, ExecutionID: x.ExecutionID, OperationID: x.OperationID, AttemptID: reservation.AttemptID, RequestHash: x.RequestHash, TargetHash: op.TargetHash, ActionType: "create_repository", ManifestRef: manifest, ObservedTarget: &target, ProjectID: observed.ID, CompletedAt: completed.Format(time.RFC3339Nano)}
		result.ResultHash, err = HashCanonicalActionResult(result)
		if err != nil {
			return err
		}
		r.Control.Schedule.Results = append(r.Control.Schedule.Results, result)
		project := strconv.FormatInt(observed.ID, 10)
		op.Status, op.ExternalObjectID, op.UpdatedAt = "verified", &project, result.CompletedAt
		if contracts.DeliveryContract(x.ContractVersion) {
			if err = appendControlRemoteReceipt(r, x, "succeeded", completed); err != nil {
				return err
			}
		}
		if r.ActiveStep == x.ExecutionID {
			r.ActiveStep = ""
		}
		// State transitions and renewed authority belong to the orchestrator.
		// In particular a resolved old write does not resume a BLOCKED Run.
		syncBudgetProjection(r)
		return nil
	})
}
