package agent

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"agentstudio/internal/contracts"
	"agentstudio/internal/governance"
)

// PrepareControlAction persists the complete allowed chain before returning a
// request. A scheduling result is not permission to perform the side effect;
// the executor must atomically revalidate and reserve via beginControlExecution.
func (s *Service) PrepareControlAction(ctx context.Context, id string) (ControlSchedule, error) {
	s.mu.Lock()
	lock := s.commandLocks[id]
	if lock == nil {
		lock = &sync.Mutex{}
		s.commandLocks[id] = lock
	}
	s.mu.Unlock()
	lock.Lock()
	defer lock.Unlock()
	r, err := s.Store.Get(id)
	if err != nil {
		return ControlSchedule{}, err
	}
	if r.Control == nil || r.ExecutionMode != s.Config.Mode {
		return ControlSchedule{}, fail("control_binding_mismatch", "只能调度当前执行模式的完整受控Run")
	}
	// Unknown writes retain their original read-only reconciliation identity,
	// even after a pause, cancellation or permission revocation. This result
	// contains no Action and cannot authorize a new write.
	for _, op := range r.Control.Audit.Operations {
		if op.Status == "sent" || op.Status == "unknown" {
			return NextControlAction(r, r.Control.Schedule)
		}
	}
	if r.State == "PAUSED" || r.State == "BLOCKED" || terminal(r.State) || r.PauseRequested || r.CancelRequested {
		return ControlSchedule{Stage: r.State, AwaitingEvidence: true}, nil
	}
	if err = s.recheckControlApproval(r, time.Now().UTC()); err != nil {
		return ControlSchedule{}, err
	}
	workspaceKey := controlWorkspaceKey(r)
	if r.Control.Schedule.Workspace.WorkspaceID == "" || r.Control.Schedule.Workspace.StorageKey != workspaceKey {
		r, err = s.Store.Update(id, func(run *Run) error {
			if e := s.recheckControlApproval(*run, time.Now().UTC()); e != nil {
				return e
			}
			key := controlWorkspaceKey(*run)
			hash, _ := contracts.HashValue([]string{run.ID, key})
			run.Control.Schedule.Workspace = contracts.WorkspaceReference{WorkspaceID: "workspace_" + hash[:32], RunID: run.ID, StorageKey: key}
			if run.Control.Schedule.Results == nil {
				run.Control.Schedule.Results = []CanonicalActionResult{}
			}
			run.Workspace = filepath.Join(s.Config.DataDir, "workspaces", filepath.FromSlash(key))
			return contracts.Validate("WorkspaceReference", run.Control.Schedule.Workspace)
		})
		if err != nil {
			return ControlSchedule{}, err
		}
	}
	next, err := NextControlAction(r, r.Control.Schedule)
	if err != nil {
		return next, err
	}
	if next.Execution != nil && !next.AwaitingEvidence && !next.NeedsReconciliation && r.State != next.Stage {
		_, err = s.Store.Update(id, func(run *Run) error {
			if e := s.recheckControlApproval(*run, time.Now().UTC()); e != nil {
				return e
			}
			current, e := NextControlAction(*run, run.Control.Schedule)
			if e != nil {
				return e
			}
			if current.Execution == nil || !equalContractValue(*current.Execution, *next.Execution) || current.AwaitingEvidence || current.NeedsReconciliation {
				return fail("stale_schedule", "原请求不再是可接纳的下一动作")
			}
			run.State = next.Stage
			return nil
		})
		if err != nil {
			return ControlSchedule{}, err
		}
	}
	if next.Execution != nil || next.Action == nil {
		return next, nil
	}
	// The persisted total execution window starts when the approved action
	// chain is admitted, including its mandatory model decision. It never moves
	// on retry/resume or source changes.
	r, err = s.Store.Update(id, func(run *Run) error {
		if e := s.recheckControlApproval(*run, time.Now().UTC()); e != nil {
			return e
		}
		current, e := NextControlAction(*run, run.Control.Schedule)
		if e != nil {
			return e
		}
		if !equalContractValue(current.Action, next.Action) || current.OperationSequence != next.OperationSequence || current.Execution != nil {
			return fail("stale_schedule", "下一动作已改变")
		}
		run.State = next.Stage
		run.Control.Audit.Budget, e = governance.StartExecution(run.FrozenDefinition.Definition.BudgetPolicy, run.Control.Audit.Budget, time.Now().UTC())
		return e
	})
	if err != nil {
		return next, err
	}
	hash, _ := contracts.HashValue([]any{r.ID, r.SourceVersion, r.Control.Audit.Approval.ApprovalID, next.OperationSequence, *next.Action})
	commandID := "schedule_" + hash[:32]
	var reserved *contracts.BudgetReservation
	if r.DecisionMode == "model" {
		timeout := r.FrozenDefinition.Definition.BudgetPolicy.MaxToolSeconds
		if timeout > 60 {
			timeout = 60
		}
		var b contracts.BudgetReservation
		r, b, err = s.reserveControlBudget(id, governance.ReservationInput{SourceVersion: int64(r.SourceVersion), CommandID: commandID, AttemptID: newID("model_attempt_"), Kind: "model", ScopeID: "decision:model", TimeoutSeconds: timeout, Now: time.Now().UTC()})
		if err != nil {
			return next, err
		}
		reserved = &b
	}
	called := false
	modelOutcome := "failed"
	failure := func(e error) (ControlSchedule, error) {
		if reserved != nil {
			start, _ := time.Parse(time.RFC3339Nano, reserved.ReservedAt)
			settlement := actualControlSettlement(*reserved, modelOutcome, start, time.Now().UTC())
			if !called {
				zero := int64(0)
				settlement.Outcome = "released"
				settlement.ElapsedMS = &zero
				settlement.ConfirmedNotStarted = true
			}
			_, _, se := s.settleControlBudget(id, settlement)
			if se != nil {
				e = errors.Join(e, se)
			}
		}
		return ControlSchedule{}, e
	}
	ordered, inputs, err := canonicalActionContext(r, *next.Action)
	if err != nil {
		return failure(err)
	}
	advice := contracts.DecisionAdvice{Kind: "action", Action: next.Action, Reason: "显式rules按已批准计划和真实执行证据选择下一动作"}
	usage := contracts.ModelUsage{Status: "not_applicable"}
	if r.DecisionMode == "model" {
		body, e := canonicalModelRequest(r, ordered)
		if e != nil {
			return failure(e)
		}
		deadline, _ := time.Parse(time.RFC3339Nano, *reserved.DeadlineAt)
		bounded, cancel := context.WithDeadline(ctx, deadline)
		response, e := s.callModelAdvice(bounded, r, body)
		called = response.Started
		cancel()
		if e != nil {
			return failure(e)
		}
		advice, usage, ordered = response.Advice, response.Usage, response.Messages
		modelOutcome = "succeeded"
	} else if r.DecisionMode != "rules" {
		return failure(fail("invalid_mode", "不支持的建议模式"))
	}
	in, record, err := bindCanonicalDecision(r, commandID, advice, usage, inputs, ordered, time.Now().UTC())
	if err != nil {
		return failure(err)
	}
	in.Facts, err = s.controlExecutionFacts(r, next, commandID, in.Facts.Now, "")
	if err != nil {
		return failure(err)
	}
	in.Facts.ObservedModelUsage = &usage
	validation, err := (governance.Validator{Definitions: s.Definitions}).Validate(in)
	if err != nil {
		return failure(err)
	}
	r, err = s.Store.Update(id, func(run *Run) error {
		if e := s.recordCanonicalDecision(run, in, record, validation); e != nil {
			return e
		}
		if reserved != nil {
			if e := settleControlInRun(run, *reserved, modelOutcome, time.Now().UTC()); e != nil {
				return e
			}
		}
		if validation.ExecutionRequest != nil {
			x := validation.ExecutionRequest
			kind, _, e := scheduleAction(x.Action)
			if e != nil {
				return e
			}
			target, e := governance.RequestTargetHash(*x)
			if e != nil {
				return e
			}
			run.Control.Audit.Operations = append(run.Control.Audit.Operations, contracts.OperationRecord{OperationID: x.OperationID, RunID: run.ID, SourceVersion: x.SourceVersion, ActionType: kind, OperationSequence: next.OperationSequence, RequestHash: x.RequestHash, TargetHash: target, Status: "ready", EvidenceRefs: []contracts.ObjectReference{}, CreatedAt: x.CreatedAt, UpdatedAt: x.CreatedAt})
		} else {
			run.Error = "当前模型或规则建议没有产生获准执行请求"
		}
		return nil
	})
	if err != nil {
		return failure(err)
	}
	reserved = nil
	if validation.ExecutionRequest == nil {
		return ControlSchedule{}, fail("advice_rejected", "调度建议未产生准确获准动作；没有执行副作用")
	}
	return NextControlAction(r, r.Control.Schedule)
}

func controlWorkspaceKey(r Run) string {
	name := r.Control.Audit.Plan.ProjectName
	if contracts.DeliveryContract(r.FrozenDefinition.ContractVersion) {
		// A new approval is a new empty generation. Old manifests and external
		// results retain their exact original references and directory bytes.
		return r.ID + "/sources/" + strconv.Itoa(r.SourceVersion) + "/" + r.Control.Audit.Approval.ApprovalID + "/" + name
	}
	return r.ID + "/" + name
}

func controlActionScope(action contracts.CandidateAction) (string, error) {
	kind, parameters, err := scheduleAction(action)
	if err != nil {
		return "", err
	}
	if kind == "apply_skill" {
		var p contracts.ApplySkillParameters
		if err = json.Unmarshal(parameters, &p); err != nil {
			return "", err
		}
		var selected struct {
			ID string `json:"skill_id"`
		}
		if err = json.Unmarshal(p.Selection, &selected); err != nil {
			return "", err
		}
		return kind + ":" + selected.ID, nil
	}
	return kind, nil
}

func (s *Service) controlExecutionFacts(r Run, next ControlSchedule, commandID string, at time.Time, ownAttempt string) (governance.Facts, error) {
	a := r.Control.Audit
	f := governance.Facts{RunID: r.ID, SourceVersion: int64(r.SourceVersion), StateSequence: int64(r.StateSequence), State: r.State, ActorID: localActorID, CommandID: commandID, OperationSequence: next.OperationSequence, Modes: runModes(r), FrozenDefinition: *r.FrozenDefinition, Plan: a.Plan, Proposal: a.Proposal, Approval: a.Approval, Permission: a.PermissionSnapshot, Workspace: &r.Control.Schedule.Workspace, Manifest: a.Manifest, Now: at, ExpectedAction: next.Action, Operation: next.Operation, AvailableEvidence: append([]contracts.ObjectReference{}, r.Control.ObjectRefs...)}
	if next.Action == nil {
		return f, fail("reconciliation_required", "没有可写下一动作")
	}
	// Probe the actual executable identity, rather than claiming the frozen
	// declaration is evidence that the installed tool matches it.
	tool, _, err := controlGoIdentity(r)
	if err != nil {
		return f, err
	}
	f.AvailableTools = []contracts.ToolIdentity{tool}
	if contracts.DeliveryContract(r.FrozenDefinition.ContractVersion) {
		git, _, e := controlGitIdentity(r)
		if e != nil {
			return f, e
		}
		http, _, e := controlGitHTTPIdentity(r)
		if e != nil {
			return f, e
		}
		f.AvailableTools = append(f.AvailableTools, git, http)
	}
	scope, err := controlActionScope(*next.Action)
	if err != nil {
		return f, err
	}
	remaining := r.FrozenDefinition.Definition.BudgetPolicy.MaxStepAttempts
	settled := map[string]contracts.BudgetSettlement{}
	for _, x := range a.Budget.Settlements {
		settled[x.ReservationID] = x
	}
	for _, x := range a.Budget.Reservations {
		if x.Kind != "action" || x.ScopeID != scope || x.AttemptID == ownAttempt {
			continue
		}
		if end, ok := settled[x.ReservationID]; !ok || end.ConsumedUnits != 0 {
			remaining--
		}
	}
	f.Budget = governance.BudgetFacts{Available: remaining > 0, RemainingAttempts: remaining}
	if a.Budget.ExecutionDeadlineAt != nil {
		deadline, e := time.Parse(time.RFC3339Nano, *a.Budget.ExecutionDeadlineAt)
		if e != nil {
			return f, e
		}
		f.Budget.ExecutionDeadline = &deadline
		f.Budget.ToolDeadline = &deadline
		f.Budget.CIDeadline = &deadline
	}
	for _, x := range r.Control.Schedule.Results {
		if x.SourceVersion != int64(r.SourceVersion) {
			continue
		}
		if x.ActionType == "create_repository" {
			f.RemoteProjectID = x.ProjectID
		}
		if x.ActionType == "push_repository" {
			f.CommitSHA = x.CommitSHA
			f.CIConfigSHA256 = x.CIConfigSHA256
		}
	}
	return f, nil
}

// beginControlExecution is the sole admission transaction for a new side-effect
// attempt. It rejects unknown/sent operations before reserving anything and
// freezes first application time together with the original operation identity.
func (s *Service) beginControlExecution(id string, request contracts.ExecutionRequest) (Run, contracts.BudgetReservation, error) {
	var reserved contracts.BudgetReservation
	r, err := s.Store.Update(id, func(run *Run) error {
		if run.Control == nil || run.ActiveStep != "" || run.PauseRequested || run.CancelRequested || terminal(run.State) || run.State == "PAUSED" || run.State == "BLOCKED" {
			return fail("execution_stopped", "当前Run不允许开始新尝试")
		}
		if e := s.recheckControlApproval(*run, time.Now().UTC()); e != nil {
			return e
		}
		next, e := NextControlAction(*run, run.Control.Schedule)
		if e != nil {
			return e
		}
		if next.NeedsReconciliation || next.Action == nil || next.Execution == nil || !equalContractValue(*next.Execution, request) {
			return fail("execution_binding_mismatch", "请求不是当前已登记的准确下一动作；未知写必须原身份核实")
		}
		var input *governance.Input
		var validation *contracts.ActionValidation
		for i := range run.Control.ValidationInputs {
			p := &run.Control.ValidationInputs[i]
			if p.Decision.DecisionID == "" {
				continue
			}
			for j := range run.Control.Audit.Validations {
				v := &run.Control.Audit.Validations[j]
				if v.ValidationID == request.ValidationID && v.DecisionID == p.Decision.DecisionID {
					input = p
					validation = v
				}
			}
		}
		if input == nil || validation == nil {
			return fail("execution_binding_mismatch", "缺少原始完整建议和校验输入")
		}
		facts, e := s.controlExecutionFacts(*run, next, request.CommandID, time.Now().UTC(), "")
		if e != nil {
			return e
		}
		if e = (governance.Validator{Definitions: s.Definitions}).Revalidate(request, *input, *validation, facts); e != nil {
			return e
		}
		scope, e := controlActionScope(request.Action)
		if e != nil {
			return e
		}
		at := time.Now().UTC()
		run.Control.Audit.Budget, reserved, e = governance.ReserveBudget(run.FrozenDefinition.Definition.BudgetPolicy, run.Control.Audit.Budget, governance.ReservationInput{SourceVersion: request.SourceVersion, CommandID: request.CommandID, AttemptID: newID("action_attempt_"), Kind: "action", ScopeID: scope, Request: &request, Now: at})
		if e != nil {
			return e
		}
		if run.Control.Audit.Approval.StartedAt == nil {
			stamp := at.Format(time.RFC3339Nano)
			run.Control.Audit.Approval.StartedAt = &stamp
			run.Control.Audit.Approval.Status = "started"
			run.Approval.StartedAt = stamp
		}
		for i := range run.Control.Audit.Operations {
			op := &run.Control.Audit.Operations[i]
			if op.OperationID == request.OperationID {
				op.Status = "sent"
				op.UpdatedAt = at.Format(time.RFC3339Nano)
			}
		}
		run.ActiveStep = request.ExecutionID
		syncBudgetProjection(run)
		return nil
	})
	return r, reserved, err
}
