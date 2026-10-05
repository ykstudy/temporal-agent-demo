package governance

import (
	"encoding/json"
	"sort"
	"strings"
	"time"

	"agentstudio/internal/contracts"
)

func (v Validator) validateAction(f Facts, action contracts.CandidateAction) ([]string, *Error) {
	if contracts.Validate("CandidateAction", action) != nil {
		return nil, reject("invalid_action", "action violates its fixed parameter schema")
	}
	if f.ExpectedAction == nil || !equalJSON(action, *f.ExpectedAction) {
		return nil, reject("unexpected_action", "candidate differs from the deterministic scheduler's next action and exact parameters")
	}
	var candidate struct {
		ActionType string          `json:"action_type"`
		Parameters json.RawMessage `json:"parameters"`
	}
	if json.Unmarshal(action, &candidate) != nil {
		return nil, reject("invalid_action", "action is not a JSON object")
	}
	kind := candidate.ActionType
	stages := map[string]string{"scaffold": "IMPLEMENT", "apply_skill": "IMPLEMENT", "verify": "VERIFY", "create_repository": "DELIVER", "push_repository": "DELIVER", "trigger_ci": "DELIVER", "package": "DELIVER"}
	if f.State != stages[kind] {
		return nil, reject("invalid_state", "action is not legal in the current business stage")
	}
	if f.Plan == nil || f.Proposal == nil || f.Approval == nil || f.Permission == nil || f.Workspace == nil {
		return nil, reject("unapproved_execution", "action requires the actual plan, proposal, approval, permission and workspace")
	}
	if err := v.validatePlan(f, *f.Plan); err != nil {
		return nil, err
	}
	if ref := f.Plan.VerificationCandidateRef; ref != nil {
		if !contracts.DeliveryContract(f.Plan.ContractVersion) || ref.SourceVersion >= f.SourceVersion || !containsRef(f.AvailableEvidence, contracts.ObjectReference(*ref)) {
			return nil, reject("candidate_not_available", "executing a revision requires an approved new source and its registered original candidate")
		}
	}
	if err := validateAuthorization(f, kind); err != nil {
		return nil, err
	}
	if f.OperationSequence < 1 {
		return nil, reject("invalid_operation", "operation sequence must be assigned by the trusted control plane")
	}
	if contracts.Validate("WorkspaceReference", *f.Workspace) != nil || f.Workspace.RunID != f.RunID || !safePath(f.Workspace.StorageKey) {
		return nil, reject("workspace_mismatch", "workspace is not an isolated relative storage reference for this Run")
	}
	permissions := map[string][]string{
		"scaffold": {"workspace_write"}, "verify": {"tool_go", "workspace_read", "workspace_write"},
		"create_repository": {"gitlab_create"}, "push_repository": {"gitlab_push", "workspace_read"},
		"trigger_ci": {"gitlab_ci"}, "package": {"package_write", "workspace_read"},
	}[kind]
	switch kind {
	case "scaffold":
		var params contracts.ScaffoldParameters
		if json.Unmarshal(candidate.Parameters, &params) != nil || params.PlanHash != f.Plan.PlanHash {
			return nil, reject("plan_mismatch", "scaffold does not bind the approved plan")
		}
	case "apply_skill":
		var params contracts.ApplySkillParameters
		if json.Unmarshal(candidate.Parameters, &params) != nil || params.PlanHash != f.Plan.PlanHash {
			return nil, reject("plan_mismatch", "skill action does not bind the approved plan")
		}
		var selection selectionFields
		if json.Unmarshal(params.Selection, &selection) != nil {
			return nil, reject("invalid_skill_selection", "skill selection could not be parsed")
		}
		selected := false
		for _, item := range f.Plan.SelectedSkills {
			if equalJSON(item, params.Selection) {
				selected = true
			}
		}
		if !selected {
			return nil, reject("skill_scope_mismatch", "action skill and parameters are not an exact approved selection")
		}
		skill := findSkill(f.FrozenDefinition, selection.SkillID)
		if skill == nil {
			return nil, reject("invalid_skill_selection", "skill is absent from the frozen definition")
		}
		permissions = append([]string{}, skill.RequiredPermissions...)
		for _, p := range skill.WritePaths {
			if !safePath(p) {
				return nil, reject("invalid_skill_path", "skill write path is not a confined relative path")
			}
		}
		if err := checkTools(f, skill.RequiredTools); err != nil {
			return nil, err
		}
	case "verify":
		var params contracts.VerifyParameters
		if json.Unmarshal(candidate.Parameters, &params) != nil {
			return nil, reject("invalid_action", "verification parameters could not be parsed")
		}
		if err := validateManifest(f, params.ManifestRef); err != nil {
			return nil, err
		}
		checks := []string{}
		for _, check := range f.FrozenDefinition.Definition.CompletionPolicy.RequiredChecks {
			if contains(check.DeliveryModes, f.Modes.DeliveryMode) {
				checks = append(checks, check.CheckID)
			}
		}
		sort.Strings(checks)
		if !equalJSON(checks, params.RequiredCheckIDs) {
			return nil, reject("check_scope_mismatch", "verify must bind the complete frozen check set for this delivery mode")
		}
	case "create_repository":
		var params contracts.CreateRepositoryParameters
		if json.Unmarshal(candidate.Parameters, &params) != nil || !equalJSON(params.Target, f.Plan.DeliveryTarget) {
			return nil, reject("target_mismatch", "repository create target is not the exact approved target")
		}
	case "push_repository":
		var params contracts.PushRepositoryParameters
		if json.Unmarshal(candidate.Parameters, &params) != nil || !equalJSON(params.Target, f.Plan.DeliveryTarget) {
			return nil, reject("target_mismatch", "repository push target is not the exact approved target")
		}
		if f.RemoteProjectID <= 0 || params.ProjectID != f.RemoteProjectID {
			return nil, reject("project_unverified", "push must use the actually verified repository identity")
		}
		if params.ExpectedBranchSha != nil {
			return nil, reject("branch_scope_mismatch", "initial project creation cannot overwrite an existing branch")
		}
		if err := validateManifest(f, params.ManifestRef); err != nil {
			return nil, err
		}
	case "trigger_ci":
		var params contracts.TriggerCIParameters
		if json.Unmarshal(candidate.Parameters, &params) != nil || !equalJSON(params.Target, f.Plan.DeliveryTarget) {
			return nil, reject("target_mismatch", "CI target is not the exact approved target")
		}
		if f.RemoteProjectID <= 0 || params.ProjectID != f.RemoteProjectID || f.CommitSHA == "" || params.CommitSha != f.CommitSHA || f.CIConfigSHA256 == "" || params.CIConfigSHA256 != f.CIConfigSHA256 {
			return nil, reject("commit_unverified", "CI must bind the actually verified project, commit and CI configuration")
		}
		if f.Manifest == nil {
			return nil, reject("manifest_mismatch", "CI configuration requires a same-source verified manifest")
		}
		if err := validateManifest(f, manifestRef(*f.Manifest)); err != nil {
			return nil, err
		}
		found := false
		for _, file := range f.Manifest.Files {
			if file.Path == ".gitlab-ci.yml" && file.SHA256 == params.CIConfigSHA256 {
				found = true
			}
		}
		if !found {
			return nil, reject("ci_configuration_mismatch", "CI config hash is not present in the approved manifest")
		}
	case "package":
		var params contracts.PackageParameters
		if json.Unmarshal(candidate.Parameters, &params) != nil {
			return nil, reject("invalid_action", "package parameters could not be parsed")
		}
		if err := validateManifest(f, params.ManifestRef); err != nil {
			return nil, err
		}
	}
	if strings.HasPrefix(kind, "create_repository") || kind == "push_repository" || kind == "trigger_ci" {
		if f.Modes.DeliveryMode != "gitlab" {
			return nil, reject("remote_action_forbidden", "local delivery cannot authorize remote actions")
		}
	}
	sort.Strings(permissions)
	for _, permission := range permissions {
		if !contains(f.Permission.AllowedPermissions, permission) || !contains(f.Proposal.RequiredPermissionIDs, permission) {
			return nil, reject("permission_denied", "required permission is absent from the current grant or approved scope")
		}
	}
	if contains(permissions, "tool_go") {
		if err := checkTools(f, f.FrozenDefinition.Definition.RequiredTools); err != nil {
			return nil, err
		}
	}
	if f.Operation != nil {
		op := f.Operation
		if contracts.Validate("OperationRecord", *op) != nil || op.RunID != f.RunID || op.SourceVersion != f.SourceVersion || op.ActionType != kind || op.OperationSequence != f.OperationSequence || op.OperationID != operationID(f, kind) {
			return nil, reject("operation_conflict", "persisted operation identity differs from the requested business action")
		}
		if op.Status == "sent" || op.Status == "unknown" {
			return nil, reject("reconciliation_required", "the original write may have happened; only bounded read-only reconciliation is permitted")
		}
		if op.Status != "ready" {
			return nil, reject("operation_closed", "closed operations cannot authorize another write")
		}
	}
	if err := validateBudget(f, kind); err != nil {
		return nil, err
	}
	return permissions, nil
}

func validateAuthorization(f Facts, kind string) *Error {
	p, a, grant := f.Proposal, f.Approval, f.Permission
	for _, check := range []struct {
		name  string
		value any
		hash  string
	}{{"Proposal", *p, p.SnapshotHash}, {"Approval", *a, a.ApprovalHash}, {"PermissionSnapshot", *grant, grant.PermissionHash}} {
		if err := checkHash(check.name, check.value, check.hash); err != nil {
			return err
		}
	}
	if p.RunID != f.RunID || a.RunID != f.RunID || p.SourceVersion != f.SourceVersion || a.SourceVersion != f.SourceVersion || p.DefinitionSnapshotHash != f.FrozenDefinition.SnapshotHash || a.DefinitionSnapshotHash != f.FrozenDefinition.SnapshotHash || p.PlanHash != f.Plan.PlanHash || a.PlanHash != f.Plan.PlanHash || !equalJSON(p.Plan, *f.Plan) {
		return reject("approval_binding_mismatch", "approval and proposal are not bound to this Run's current frozen plan and source")
	}
	if a.ProposalID != p.ProposalID || a.ProposalVersion != p.ProposalVersion || a.ProposalSnapshotHash != p.SnapshotHash || a.IssuedAt != p.IssuedAt || a.ExpiresAt != p.ExpiresAt || p.Status != "accepted" {
		return reject("approval_binding_mismatch", "approval does not bind the exact accepted proposal and server-issued validity interval")
	}
	if a.ActorID != f.ActorID || grant.ActorID != f.ActorID || grant.Revoked || a.RevokedAt != nil || !oneOf(a.Status, "accepted", "started") {
		return reject("permission_revoked", "trusted actor or current approval/permission lifecycle does not authorize execution")
	}
	if p.PermissionHash != grant.PermissionHash || a.PermissionHash != grant.PermissionHash || !equalJSON(grant.DeliveryTarget, f.Plan.DeliveryTarget) || !equalJSON(a.DeliveryTarget, f.Plan.DeliveryTarget) {
		return reject("permission_scope_mismatch", "permission hash or exact delivery target differs from the approval")
	}
	if !contains(f.FrozenDefinition.Definition.ActionProtocol.AllowedActions, kind) || !contains(p.AllowedActions, kind) || !contains(a.AllowedActions, kind) {
		return reject("action_not_allowed", "action is not in the definition, proposal and approval intersection")
	}
	for _, permitted := range a.AllowedActions {
		if !contains(p.AllowedActions, permitted) || !contains(f.FrozenDefinition.Definition.ActionProtocol.AllowedActions, permitted) {
			return reject("approval_scope_mismatch", "approval expands the proposal or frozen action scope")
		}
	}
	if !sortedStrings(grant.AllowedPermissions) || !sortedStrings(p.RequiredPermissionIDs) {
		return reject("permission_scope_mismatch", "permission sets must be normalized and unique")
	}
	grantTime, grantTimeErr := time.Parse(time.RFC3339Nano, grant.UpdatedAt)
	if grantTimeErr != nil || grantTime.After(f.Now) {
		return reject("permission_time_mismatch", "current permission must have a valid already-issued timestamp")
	}
	for _, permission := range p.RequiredPermissionIDs {
		if !contains(grant.AllowedPermissions, permission) {
			return reject("permission_denied", "proposal requires a permission absent from the current bound grant")
		}
	}
	if f.Modes.DeliveryMode == "local" {
		for _, permission := range grant.AllowedPermissions {
			if strings.HasPrefix(permission, "gitlab_") {
				return reject("remote_permission_forbidden", "local delivery cannot hold remote write permissions")
			}
		}
		for _, allowed := range append(append([]string{}, p.AllowedActions...), a.AllowedActions...) {
			if oneOf(allowed, "create_repository", "push_repository", "trigger_ci") {
				return reject("remote_permission_forbidden", "local proposal and approval cannot include remote actions")
			}
		}
	}
	issued, e1 := time.Parse(time.RFC3339Nano, a.IssuedAt)
	expires, e2 := time.Parse(time.RFC3339Nano, a.ExpiresAt)
	accepted, e3 := time.Parse(time.RFC3339Nano, a.AcceptedAt)
	if e1 != nil || e2 != nil || e3 != nil || expires.Sub(issued) < time.Minute || expires.Sub(issued) > 24*time.Hour || accepted.Before(issued) || !accepted.Before(expires) || accepted.After(f.Now) {
		return reject("invalid_approval_time", "approval time ordering is invalid")
	}
	if a.StartedAt == nil {
		if a.Status != "accepted" || !f.Now.Before(expires) {
			return reject("approval_expired", "first application must begin before the server-issued expiry")
		}
	} else {
		started, err := time.Parse(time.RFC3339Nano, *a.StartedAt)
		if err != nil || a.Status != "started" || started.Before(accepted) || !started.Before(expires) || started.After(f.Now) {
			return reject("invalid_approval_time", "started approval has no valid in-scope first application time")
		}
	}
	for _, check := range p.Checks {
		if !check.Passed {
			return reject("proposal_blocked", "the approved proposal contains a failed preflight check")
		}
	}
	previous := ""
	for _, change := range p.Changes {
		if !safePath(change.Path) || change.Path <= previous {
			return reject("invalid_change_scope", "approved paths must be confined, unique and normalized")
		}
		previous = change.Path
		if change.After != nil && (change.AfterSHA256 == nil || *change.AfterSHA256 != contracts.BytesHash([]byte(*change.After))) || change.Before != nil && (change.BeforeSHA256 == nil || *change.BeforeSHA256 != contracts.BytesHash([]byte(*change.Before))) {
			return reject("artifact_hash_mismatch", "approved file content hashes do not match their exact bytes")
		}
	}
	return nil
}

func validateBudget(f Facts, kind string) *Error {
	if !f.Budget.Available || f.Budget.RemainingAttempts <= 0 {
		return reject("budget_exceeded", "trusted resource ledger has no available execution reservation")
	}
	if f.Budget.ExecutionDeadline == nil || !f.Now.Before(*f.Budget.ExecutionDeadline) {
		return reject("execution_deadline_exceeded", "the persisted total execution deadline is absent or exhausted")
	}
	if kind == "verify" || kind == "apply_skill" {
		if f.Budget.ToolDeadline == nil || !f.Now.Before(*f.Budget.ToolDeadline) {
			return reject("tool_deadline_exceeded", "the bounded tool deadline is absent or exhausted")
		}
	}
	if kind == "trigger_ci" {
		if f.Budget.CIDeadline == nil || !f.Now.Before(*f.Budget.CIDeadline) {
			return reject("ci_deadline_exceeded", "the persisted CI deadline is absent or exhausted")
		}
	}
	return nil
}

func validateManifest(f Facts, ref contracts.ObjectReference) *Error {
	m := f.Manifest
	if m == nil {
		return reject("manifest_mismatch", "action requires an actual same-source manifest")
	}
	if err := checkHash("ArtifactManifest", *m, m.Hash); err != nil {
		return err
	}
	if m.RunID != f.RunID || m.SourceVersion != f.SourceVersion || m.ProposalID != f.Proposal.ProposalID || m.ProposalVersion != f.Proposal.ProposalVersion || m.ProposalSnapshotHash != f.Proposal.SnapshotHash || m.DefinitionSnapshotHash != f.FrozenDefinition.SnapshotHash || m.PlanHash != f.Plan.PlanHash || ref != manifestRef(*m) {
		return reject("manifest_mismatch", "manifest reference and provenance differ from the approved source")
	}
	previous := ""
	for _, entry := range m.Files {
		if !safePath(entry.Path) || entry.Path <= previous {
			return reject("manifest_mismatch", "manifest paths must be confined, unique and sorted")
		}
		previous = entry.Path
	}
	return nil
}

func manifestRef(m contracts.ArtifactManifest) contracts.ObjectReference {
	return contracts.ObjectReference{ObjectType: "manifest", ObjectID: m.ManifestID, ObjectVersion: m.ManifestVersion, SourceVersion: m.SourceVersion, Hash: m.Hash}
}
func operationID(f Facts, kind string) string {
	return hashID("op_", f.RunID, kind, f.SourceVersion, f.OperationSequence)
}

// RequestTargetHash follows the 1.2 publication: all seven action types hash
// their complete normalized action.parameters value, with no exclusions.
func RequestTargetHash(request contracts.ExecutionRequest) (string, error) {
	var action struct {
		Parameters json.RawMessage `json:"parameters"`
	}
	if err := json.Unmarshal(request.Action, &action); err != nil {
		return "", err
	}
	return contracts.HashValue(action.Parameters)
}
func checkTools(f Facts, required []contracts.ToolIdentity) *Error {
	for _, tool := range required {
		bound, observed := false, false
		for _, actual := range f.FrozenDefinition.ToolBindings {
			bound = bound || equalJSON(actual, tool)
		}
		for _, actual := range f.AvailableTools {
			observed = observed || equalJSON(actual, tool)
		}
		if !bound || !observed {
			return reject("tool_identity_mismatch", "required tool identity is not both frozen and actually available")
		}
	}
	return nil
}

func buildRequest(f Facts, validation contracts.ActionValidation, permissions []string) (contracts.ExecutionRequest, error) {
	var action struct {
		ActionType string `json:"action_type"`
	}
	if err := json.Unmarshal(*validation.NormalizedAdvice.Action, &action); err != nil {
		return contracts.ExecutionRequest{}, err
	}
	opID := operationID(f, action.ActionType)
	a := f.Approval
	out := contracts.ExecutionRequest{
		ContractVersion: validation.ContractVersion, ExecutionID: hashID("execution_", validation.ValidationID, opID), OperationID: opID, CommandID: f.CommandID, RunID: f.RunID,
		SourceVersion: f.SourceVersion, StateSequenceAtValidation: f.StateSequence, DefinitionSnapshotHash: f.FrozenDefinition.SnapshotHash, PlanHash: f.Plan.PlanHash,
		ValidationID: validation.ValidationID, ValidationHash: validation.ValidationHash,
		Approval:  contracts.ApprovalBinding{ApprovalID: a.ApprovalID, ProposalID: a.ProposalID, ProposalVersion: a.ProposalVersion, SourceVersion: a.SourceVersion, ProposalSnapshotHash: a.ProposalSnapshotHash, DefinitionSnapshotHash: a.DefinitionSnapshotHash, PermissionHash: a.PermissionHash, IssuedAt: a.IssuedAt, ExpiresAt: a.ExpiresAt},
		Workspace: *f.Workspace, Action: append(contracts.CandidateAction(nil), (*validation.NormalizedAdvice.Action)...), RequiredPermissionIDs: append([]string{}, permissions...), CreatedAt: validation.ValidatedAt,
	}
	var err error
	out.RequestHash, err = contracts.HashObject("ExecutionRequest", out)
	if err == nil {
		err = contracts.Validate("ExecutionRequest", out)
	}
	return out, err
}
