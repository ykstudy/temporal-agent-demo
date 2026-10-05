package governance

import (
	"agentstudio/internal/contracts"
)

// Revalidate verifies the immutable original chain and then checks current facts.
// Ordinary state_sequence progress is allowed; source, plan, approval, permission,
// workspace, allowed stage and resource limits are always checked again.
func (v Validator) Revalidate(request contracts.ExecutionRequest, original Input, validation contracts.ActionValidation, current Facts) error {
	if err := checkHash("ExecutionRequest", request, request.RequestHash); err != nil {
		return err
	}
	if err := checkHash("ActionValidation", validation, validation.ValidationHash); err != nil {
		return err
	}
	rebuilt, err := v.Validate(original)
	if err != nil {
		return err
	}
	if rebuilt.Verdict != "allowed" || rebuilt.DecisionKind != "action" || rebuilt.ExecutionRequest == nil || !equalJSON(rebuilt, validation) || !equalJSON(*rebuilt.ExecutionRequest, request) {
		return reject("execution_binding_mismatch", "execution request was not produced by the original allowed validation")
	}
	if current.Now.Before(original.Facts.Now) || current.StateSequence < request.StateSequenceAtValidation || current.RunID != request.RunID || current.SourceVersion != request.SourceVersion || current.CommandID != request.CommandID || current.ActorID != original.Facts.ActorID || current.OperationSequence != original.Facts.OperationSequence || !equalJSON(current.Modes, original.Facts.Modes) || current.FrozenDefinition.SnapshotHash != request.DefinitionSnapshotHash {
		return reject("execution_binding_mismatch", "current identity, source or frozen mode differs from the allowed request")
	}
	if v.Definitions == nil || v.Definitions.VerifySnapshot(current.FrozenDefinition) != nil {
		return reject("definition_unavailable", "current frozen definition closure cannot be verified")
	}
	if current.Plan == nil || current.Plan.PlanHash != request.PlanHash || current.Approval == nil || current.Approval.ApprovalID != request.Approval.ApprovalID || current.Approval.ApprovalHash != original.Facts.Approval.ApprovalHash || current.Proposal == nil || current.Proposal.SnapshotHash != original.Facts.Proposal.SnapshotHash || current.Permission == nil || current.Permission.PermissionHash != request.Approval.PermissionHash || current.Workspace == nil || !equalJSON(*current.Workspace, request.Workspace) {
		return reject("execution_binding_mismatch", "current plan, approval or workspace differs from the allowed request")
	}
	targetHash, targetErr := RequestTargetHash(request)
	if targetErr != nil {
		return targetErr
	}
	if current.Operation != nil && (current.Operation.RequestHash != request.RequestHash || current.Operation.TargetHash != targetHash) {
		return reject("operation_conflict", "persisted operation belongs to another execution request")
	}
	permissions, problem := v.validateAction(current, request.Action)
	if problem != nil {
		if problem.Code == "reconciliation_required" {
			if !current.Budget.ReconciliationAvailable || current.Budget.ReconciliationDeadline == nil || !current.Now.Before(*current.Budget.ReconciliationDeadline) {
				return reject("reconciliation_budget_exceeded", "the original operation remains unknown and its read-only reconciliation budget is exhausted")
			}
			return &ReconciliationRequiredError{OperationID: request.OperationID}
		}
		return problem
	}
	if !equalJSON(permissions, request.RequiredPermissionIDs) {
		return reject("permission_scope_mismatch", "current required permissions differ from the original allowed request")
	}
	return nil
}
