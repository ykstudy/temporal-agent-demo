package agent

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"

	"agentstudio/internal/contracts"
	"agentstudio/internal/governance"
)

// CanonicalScheduleState is private, durable executor state, never model input
// or a replacement for the public audit. Only the trusted executor may append
// Results, after observing the remote object or the actual package bytes.
type CanonicalScheduleState struct {
	Workspace contracts.WorkspaceReference `json:"workspace"`
	Results   []CanonicalActionResult      `json:"results"`
}

// CanonicalActionResult records facts for which the 1.2 public contract has no
// receipt carrier. It complements the original full ExecutionRequest and
// OperationRecord; a successful status or an external_object_id alone is not
// completion evidence. ResultHash detects corruption, not an untrusted writer.
type CanonicalActionResult struct {
	RunID                  string                     `json:"run_id"`
	SourceVersion          int64                      `json:"source_version"`
	DefinitionSnapshotHash string                     `json:"definition_snapshot_hash"`
	PlanHash               string                     `json:"plan_hash"`
	ApprovalID             string                     `json:"approval_id"`
	ApprovalHash           string                     `json:"approval_hash"`
	ProposalSnapshotHash   string                     `json:"proposal_snapshot_hash"`
	ExecutionID            string                     `json:"execution_id"`
	OperationID            string                     `json:"operation_id"`
	AttemptID              string                     `json:"attempt_id"`
	RequestHash            string                     `json:"request_hash"`
	TargetHash             string                     `json:"target_hash"`
	ActionType             string                     `json:"action_type"`
	ManifestRef            *contracts.ObjectReference `json:"manifest_ref"`
	ObservedTarget         *contracts.GitLabTarget    `json:"observed_target"`
	ProjectID              int64                      `json:"project_id"`
	CommitSHA              string                     `json:"commit_sha"`
	CIConfigSHA256         string                     `json:"ci_config_sha256"`
	PipelineID             int64                      `json:"pipeline_id"`
	PipelineStatus         string                     `json:"pipeline_status"`
	PackageSHA256          string                     `json:"package_sha256"`
	PackageBytes           int64                      `json:"package_bytes"`
	CompletedAt            string                     `json:"completed_at"`
	ResultHash             string                     `json:"result_hash"`
}

func HashCanonicalActionResult(result CanonicalActionResult) (string, error) {
	result.ResultHash = ""
	return contracts.HashValue(result)
}

// ControlSchedule is a scheduling decision, not an authorization. The caller
// must validate new advice with live governance facts and reserve budget before
// execution. Existing requests keep their original identities and hashes.
// sent/unknown requests have no Action and may only be reconciled read-only.
// ActionsExhausted never means DONE; the completion gate still owns that state.
type ControlSchedule struct {
	Stage               string
	Action              *contracts.CandidateAction
	Execution           *contracts.ExecutionRequest
	Operation           *contracts.OperationRecord
	OperationSequence   int64
	NeedsReconciliation bool
	AwaitingEvidence    bool
	ActionsExhausted    bool
}

type canonicalScheduler struct {
	r           Run
	state       CanonicalScheduleState
	ops         map[string]contracts.OperationRecord
	results     map[string]CanonicalActionResult
	requests    []contracts.ExecutionRequest
	manifest    *contracts.ArtifactManifest
	projectID   int64
	commitSHA   string
	ciSHA       string
	completedAt string
	sequence    int64
	position    int
}

func scheduleError(detail string) error {
	return fail("control_schedule_mismatch", "确定性调度证据不完整或不一致: "+detail)
}

// NextControlAction is pure: it never writes files, calls tools, reads a clock,
// changes stages, issues model advice, or manufactures successful evidence.
func NextControlAction(r Run, state CanonicalScheduleState) (ControlSchedule, error) {
	if r.Control == nil || r.FrozenDefinition == nil {
		return ControlSchedule{}, scheduleError("complete control and definition required")
	}
	if err := verifyControlChain(r); err != nil {
		return ControlSchedule{}, err
	}
	if err := governance.ValidateBudgetLedger(r.FrozenDefinition.Definition.BudgetPolicy, r.Control.Audit.Budget); err != nil {
		return ControlSchedule{}, err
	}
	s := canonicalScheduler{r: r, state: state, ops: map[string]contracts.OperationRecord{}, results: map[string]CanonicalActionResult{}}
	if err := s.index(); err != nil {
		return ControlSchedule{}, err
	}
	// Even an old source or revoked approval cannot make an outstanding write
	// disappear. Resolve the original identity before any new action is issued.
	for _, op := range r.Control.Audit.Operations {
		if op.Status == "sent" || op.Status == "unknown" {
			for _, request := range r.Control.Audit.Executions {
				if request.OperationID == op.OperationID {
					return scheduleExisting(request, &op, true, false), nil
				}
			}
		}
	}
	if err := s.authority(); err != nil {
		return ControlSchedule{}, err
	}
	if r.PauseRequested || r.CancelRequested || chainContains([]string{"PAUSED", "CANCELLED", "FAILED", "DONE", "BLOCKED"}, r.State) {
		return ControlSchedule{Stage: r.State, AwaitingEvidence: true}, nil
	}
	steps := []struct {
		kind       string
		parameters any
	}{{"scaffold", contracts.ScaffoldParameters{PlanHash: r.Control.Audit.Plan.PlanHash}}}
	for _, selection := range r.Control.Audit.Plan.SelectedSkills {
		steps = append(steps, struct {
			kind       string
			parameters any
		}{"apply_skill", contracts.ApplySkillParameters{PlanHash: r.Control.Audit.Plan.PlanHash, Selection: selection}})
	}
	for _, step := range steps {
		out, done, err := s.step(step.kind, step.parameters)
		if err != nil || !done {
			return out, err
		}
	}
	checks := []string{}
	for _, check := range r.FrozenDefinition.Definition.CompletionPolicy.RequiredChecks {
		if chainContains(check.DeliveryModes, r.DeliveryMode) {
			checks = append(checks, check.CheckID)
		}
	}
	sort.Strings(checks)
	out, done, err := s.step("verify", contracts.VerifyParameters{ManifestRef: scheduleManifestRef(*s.manifest), RequiredCheckIDs: checks})
	if err != nil || !done {
		return out, err
	}
	if r.DeliveryMode == "gitlab" {
		var target contracts.GitLabTarget
		if err := contracts.Decode("GitLabTarget", r.Control.Audit.Plan.DeliveryTarget, &target); err != nil {
			return ControlSchedule{}, err
		}
		out, done, err = s.step("create_repository", contracts.CreateRepositoryParameters{Target: target})
		if err != nil || !done {
			return out, err
		}
		out, done, err = s.step("push_repository", contracts.PushRepositoryParameters{Target: target, ProjectID: s.projectID, ManifestRef: scheduleManifestRef(*s.manifest)})
		if err != nil || !done {
			return out, err
		}
		out, done, err = s.step("trigger_ci", contracts.TriggerCIParameters{Target: target, ProjectID: s.projectID, CommitSha: s.commitSHA, CIConfigSHA256: s.ciSHA})
		if err != nil || !done {
			return out, err
		}
	}
	out, done, err = s.step("package", contracts.PackageParameters{ManifestRef: scheduleManifestRef(*s.manifest)})
	if err != nil || !done {
		return out, err
	}
	if s.position != len(s.requests) {
		return ControlSchedule{}, scheduleError("extra execution after package")
	}
	if err = s.currentManifest(); err != nil {
		return ControlSchedule{}, err
	}
	return ControlSchedule{Stage: "DELIVER", ActionsExhausted: true}, nil
}

func (s *canonicalScheduler) index() error {
	requests := map[string]contracts.ExecutionRequest{}
	for _, x := range s.r.Control.Audit.Executions {
		if _, exists := requests[x.OperationID]; exists {
			return scheduleError("multiple requests for one operation")
		}
		requests[x.OperationID] = x
	}
	for _, in := range s.r.Control.ValidationInputs {
		if in.Facts.OperationSequence > s.sequence {
			s.sequence = in.Facts.OperationSequence
		}
	}
	for _, op := range s.r.Control.Audit.Operations {
		x, exists := requests[op.OperationID]
		kind, _, err := scheduleAction(x.Action)
		target, hashErr := governance.RequestTargetHash(x)
		identity, identityErr := contracts.HashValue([]any{op.RunID, op.ActionType, op.SourceVersion, op.OperationSequence})
		if !exists || err != nil || hashErr != nil || identityErr != nil || contracts.Validate("OperationRecord", op) != nil || s.ops[op.OperationID].OperationID != "" || op.OperationID != "op_"+identity[:32] || op.RunID != x.RunID || op.SourceVersion != x.SourceVersion || op.RequestHash != x.RequestHash || op.TargetHash != target || op.ActionType != kind || !chainBefore(x.CreatedAt, op.CreatedAt, false) || !chainBefore(op.CreatedAt, op.UpdatedAt, false) {
			return scheduleError("operation request, scope, target or identity")
		}
		s.ops[op.OperationID] = op
		if op.OperationSequence > s.sequence {
			s.sequence = op.OperationSequence
		}
	}
	for _, result := range s.state.Results {
		x, exists := requests[result.OperationID]
		op := s.ops[result.OperationID]
		hash, err := HashCanonicalActionResult(result)
		var approval *contracts.Approval
		for i := range s.r.Control.Audit.ApprovalHistory {
			a := &s.r.Control.Audit.ApprovalHistory[i]
			if a.ApprovalID == x.Approval.ApprovalID {
				approval = a
			}
		}
		if !exists || approval == nil || err != nil || hash != result.ResultHash || op.OperationID == "" || s.results[result.OperationID].OperationID != "" || result.RunID != x.RunID || result.SourceVersion != x.SourceVersion || result.DefinitionSnapshotHash != x.DefinitionSnapshotHash || result.PlanHash != x.PlanHash || result.ApprovalID != x.Approval.ApprovalID || result.ApprovalHash != approval.ApprovalHash || result.ProposalSnapshotHash != x.Approval.ProposalSnapshotHash || result.ExecutionID != x.ExecutionID || result.RequestHash != x.RequestHash || result.TargetHash != op.TargetHash || result.ActionType != op.ActionType || !chainBefore(x.CreatedAt, result.CompletedAt, false) || !chainBefore(result.CompletedAt, op.UpdatedAt, false) {
			return scheduleError("executor result identity, hash or provenance")
		}
		if !chainContains([]string{"create_repository", "push_repository", "trigger_ci", "package"}, result.ActionType) {
			return scheduleError("unexpected private result kind")
		}
		if contracts.DeliveryContract(x.ContractVersion) && result.ActionType != "package" {
			if err := controlRemoteResultReceipt(s.r, x, result); err != nil {
				return err
			}
		}
		s.results[result.OperationID] = result
	}
	return nil
}

func (s *canonicalScheduler) authority() error {
	a := s.r.Control.Audit
	if a.Plan == nil || a.Proposal == nil || a.Approval == nil || a.PermissionSnapshot == nil {
		return scheduleError("approved scope required")
	}
	p, approval, grant := a.Proposal, a.Approval, a.PermissionSnapshot
	if contracts.Validate("WorkspaceReference", s.state.Workspace) != nil || s.state.Workspace.RunID != s.r.ID || p.RunID != s.r.ID || p.SourceVersion != int64(s.r.SourceVersion) || approval.SourceVersion != p.SourceVersion || p.Status != "accepted" || !chainContains([]string{"accepted", "started"}, approval.Status) || approval.RevokedAt != nil || grant.Revoked || approval.ActorID != grant.ActorID || approval.PermissionHash != grant.PermissionHash || p.PermissionHash != grant.PermissionHash || approval.ProposalSnapshotHash != p.SnapshotHash || approval.PlanHash != a.Plan.PlanHash || !chainEqual(grant.DeliveryTarget, a.Plan.DeliveryTarget) || !chainEqual(approval.DeliveryTarget, a.Plan.DeliveryTarget) {
		return scheduleError("current workspace, source, grant or approval scope")
	}
	if s.r.DeliveryMode != "local" && s.r.DeliveryMode != "gitlab" {
		return scheduleError("unsupported delivery mode")
	}
	if s.r.DeliveryMode == "local" {
		for _, permission := range grant.AllowedPermissions {
			if len(permission) >= 7 && permission[:7] == "gitlab_" {
				return scheduleError("remote permission in local scope")
			}
		}
		for _, action := range append(append([]string{}, p.AllowedActions...), approval.AllowedActions...) {
			if chainContains([]string{"create_repository", "push_repository", "trigger_ci"}, action) {
				return scheduleError("remote action in local scope")
			}
		}
	}
	for _, x := range a.Executions {
		if x.SourceVersion == int64(s.r.SourceVersion) && x.Approval.ApprovalID == approval.ApprovalID {
			if !chainEqual(x.Workspace, s.state.Workspace) || x.PlanHash != a.Plan.PlanHash {
				return scheduleError("execution workspace or plan changed")
			}
			s.requests = append(s.requests, x)
		}
	}
	return nil
}

func scheduleAction(action contracts.CandidateAction) (string, json.RawMessage, error) {
	var value struct {
		ActionType string          `json:"action_type"`
		Parameters json.RawMessage `json:"parameters"`
	}
	if err := contracts.Validate("CandidateAction", action); err != nil {
		return "", nil, err
	}
	err := json.Unmarshal(action, &value)
	return value.ActionType, value.Parameters, err
}

func scheduleStage(kind string) string {
	if kind == "scaffold" || kind == "apply_skill" {
		return "IMPLEMENT"
	}
	if kind == "verify" {
		return "VERIFY"
	}
	return "DELIVER"
}

func scheduleExisting(x contracts.ExecutionRequest, op *contracts.OperationRecord, reconcile, waiting bool) ControlSchedule {
	kind, _, _ := scheduleAction(x.Action)
	// Deep-copy the public request so the returned action cannot mutate history.
	encoded, _ := json.Marshal(x)
	var copy contracts.ExecutionRequest
	_ = json.Unmarshal(encoded, &copy)
	out := ControlSchedule{Stage: scheduleStage(kind), Execution: &copy, NeedsReconciliation: reconcile, AwaitingEvidence: waiting}
	if op != nil {
		value := *op
		value.EvidenceRefs = append([]contracts.ObjectReference{}, op.EvidenceRefs...)
		out.Operation = &value
		out.OperationSequence = op.OperationSequence
	}
	if !reconcile && !waiting {
		action := contracts.CandidateAction(append([]byte{}, copy.Action...))
		out.Action = &action
	}
	return out
}

func (s *canonicalScheduler) step(kind string, parameters any) (ControlSchedule, bool, error) {
	body, err := json.Marshal(struct {
		ActionType string `json:"action_type"`
		Parameters any    `json:"parameters"`
	}{kind, parameters})
	if err != nil {
		return ControlSchedule{}, false, err
	}
	action := contracts.CandidateAction(body)
	if err = contracts.Validate("CandidateAction", action); err != nil {
		return ControlSchedule{}, false, err
	}
	a := s.r.Control.Audit
	if !chainContains(a.Proposal.AllowedActions, kind) || !chainContains(a.Approval.AllowedActions, kind) || !chainContains(s.r.FrozenDefinition.Definition.ActionProtocol.AllowedActions, kind) {
		return ControlSchedule{}, false, scheduleError("action outside current approval")
	}
	if s.position == len(s.requests) {
		if err = s.currentManifest(); err != nil {
			return ControlSchedule{}, false, err
		}
		if s.sequence >= 9007199254740991 {
			return ControlSchedule{}, false, scheduleError("operation sequence exhausted")
		}
		return ControlSchedule{Stage: scheduleStage(kind), Action: &action, OperationSequence: s.sequence + 1}, false, nil
	}
	x := s.requests[s.position]
	if s.completedAt != "" && !chainBefore(s.completedAt, x.CreatedAt, false) {
		return ControlSchedule{}, false, scheduleError("request issued before predecessor completed")
	}
	if !chainEqual(x.Action, action) {
		return ControlSchedule{}, false, scheduleError("execution is not the next exact approved action")
	}
	op, exists := s.ops[x.OperationID]
	if !exists || op.Status == "ready" {
		if err := s.currentManifest(); err != nil {
			return ControlSchedule{}, false, err
		}
		var pointer *contracts.OperationRecord
		if exists {
			pointer = &op
		}
		out := scheduleExisting(x, pointer, false, false)
		if !exists {
			for _, input := range s.r.Control.ValidationInputs {
				identity, identityErr := contracts.HashValue([]any{x.RunID, kind, x.SourceVersion, input.Facts.OperationSequence})
				if identityErr == nil && input.Facts.CommandID == x.CommandID && input.Decision.StateSequence == x.StateSequenceAtValidation && "op_"+identity[:32] == x.OperationID {
					out.OperationSequence = input.Facts.OperationSequence
				}
			}
		}
		return out, false, nil
	}
	if op.Status != "verified" {
		return scheduleExisting(x, &op, false, true), false, nil
	}
	complete := false
	switch kind {
	case "scaffold", "apply_skill":
		complete, err = s.workspace(x, op)
	case "verify":
		complete, err = s.tools(x, op)
	default:
		complete, err = s.result(x, op)
	}
	if err != nil {
		return ControlSchedule{}, false, err
	}
	if !complete {
		return scheduleExisting(x, &op, false, true), false, nil
	}
	s.position++
	return ControlSchedule{}, true, nil
}

func scheduleManifestRef(m contracts.ArtifactManifest) contracts.ObjectReference {
	return contracts.ObjectReference{ObjectType: "manifest", ObjectID: m.ManifestID, ObjectVersion: m.ManifestVersion, SourceVersion: m.SourceVersion, Hash: m.Hash}
}

func (s *canonicalScheduler) manifestAt(ref contracts.ObjectReference) (*contracts.ArtifactManifest, error) {
	for _, m := range s.r.Control.Audit.ManifestHistory {
		p := s.r.Control.Audit.Proposal
		if scheduleManifestRef(m) == ref {
			if !hashMatches("ArtifactManifest", m, m.Hash) || m.RunID != s.r.ID || m.SourceVersion != int64(s.r.SourceVersion) || m.ProposalID != p.ProposalID || m.ProposalVersion != p.ProposalVersion || m.ProposalSnapshotHash != p.SnapshotHash || m.PlanHash != p.PlanHash || m.DefinitionSnapshotHash != s.r.FrozenDefinition.SnapshotHash {
				return nil, scheduleError("manifest scope")
			}
			previous := ""
			for _, f := range m.Files {
				if f.Path <= previous {
					return nil, scheduleError("manifest paths not unique and sorted")
				}
				previous = f.Path
			}
			return &m, nil
		}
	}
	return nil, scheduleError("unregistered exact manifest reference")
}

func (s *canonicalScheduler) currentManifest() error {
	if !chainEqual(s.manifest, s.r.Control.Audit.Manifest) {
		return scheduleError("current manifest has no complete predecessor receipt chain")
	}
	return nil
}

func scheduleRef(value any) *contracts.ObjectReference {
	if value == nil {
		return nil
	}
	encoded, err := json.Marshal(value)
	if err != nil || string(encoded) == "null" {
		return nil
	}
	var ref contracts.ObjectReference
	if json.Unmarshal(encoded, &ref) != nil {
		return nil
	}
	return &ref
}

func (s *canonicalScheduler) receiptBinding(x contracts.ExecutionRequest, run string, source int64, definition, execution, operation, request, approval, approvalHash, proposal, plan string, workspace contracts.WorkspaceReference, started, completed string) bool {
	a := s.r.Control.Audit.Approval
	return run == x.RunID && source == x.SourceVersion && definition == x.DefinitionSnapshotHash && execution == x.ExecutionID && operation == x.OperationID && request == x.RequestHash && approval == a.ApprovalID && approvalHash == a.ApprovalHash && proposal == x.Approval.ProposalSnapshotHash && plan == x.PlanHash && chainEqual(workspace, x.Workspace) && chainBefore(x.CreatedAt, started, false) && chainBefore(started, completed, false)
}

func (s *canonicalScheduler) settled(x contracts.ExecutionRequest, attempt, kind, scope, started, completed string) bool {
	for _, reservation := range s.r.Control.Audit.Budget.Reservations {
		if reservation.AttemptID != attempt || reservation.Kind != kind || reservation.ScopeID != scope || reservation.SourceVersion != x.SourceVersion || reservation.CommandID != x.CommandID || reservation.OperationID == nil || *reservation.OperationID != x.OperationID || reservation.ExecutionID == nil || *reservation.ExecutionID != x.ExecutionID || reservation.RequestHash == nil || *reservation.RequestHash != x.RequestHash || !chainBefore(reservation.ReservedAt, started, false) {
			continue
		}
		for _, settlement := range s.r.Control.Audit.Budget.Settlements {
			if settlement.ReservationID == reservation.ReservationID && settlement.ReservationHash == reservation.ReservationHash && settlement.AttemptID == attempt && (settlement.Outcome == "succeeded" && chainBefore(completed, settlement.SettledAt, false) || settlement.Outcome == "unknown") {
				return true
			}
		}
	}
	return false
}

func (s *canonicalScheduler) workspace(x contracts.ExecutionRequest, op contracts.OperationRecord) (bool, error) {
	kind, parameters, _ := scheduleAction(x.Action)
	allowed := map[string]bool{}
	for _, change := range s.r.Control.Audit.Proposal.Changes {
		if change.After != nil {
			allowed[change.Path] = true
		}
	}
	scope := kind
	if kind == "apply_skill" {
		var p contracts.ApplySkillParameters
		_ = json.Unmarshal(parameters, &p)
		var selection struct {
			SkillID string `json:"skill_id"`
		}
		_ = json.Unmarshal(p.Selection, &selection)
		scope += ":" + selection.SkillID
		skillPaths := map[string]bool{}
		for _, skill := range s.r.FrozenDefinition.Skills {
			if skill.SkillID == selection.SkillID {
				for _, path := range skill.WritePaths {
					skillPaths[path] = true
				}
			}
		}
		for path := range allowed {
			if !skillPaths[path] {
				delete(allowed, path)
			}
		}
	} else {
		// Scaffold owns the base paths; selected skill paths must be evidenced by
		// their own subsequent action, even if a legacy renderer wrote all files.
		for _, selection := range s.r.Control.Audit.Plan.SelectedSkills {
			var value struct {
				SkillID string `json:"skill_id"`
			}
			_ = json.Unmarshal(selection, &value)
			for _, skill := range s.r.FrozenDefinition.Skills {
				if skill.SkillID == value.SkillID {
					for _, path := range skill.WritePaths {
						delete(allowed, path)
					}
				}
			}
		}
	}
	var successful *contracts.WorkspaceReceipt
	for _, receipt := range s.r.Control.Audit.WorkspaceReceipts {
		if receipt.ExecutionID != x.ExecutionID {
			continue
		}
		if !hashMatches("WorkspaceReceipt", receipt, receipt.ReceiptHash) || receipt.ActionType != kind || !s.receiptBinding(x, receipt.RunID, receipt.SourceVersion, receipt.DefinitionSnapshotHash, receipt.ExecutionID, receipt.OperationID, receipt.RequestHash, receipt.ApprovalID, receipt.ApprovalHash, receipt.ProposalSnapshotHash, receipt.PlanHash, receipt.Workspace, receipt.StartedAt, receipt.CompletedAt) {
			return false, scheduleError("workspace receipt hash or binding")
		}
		previous := ""
		for _, path := range receipt.WrittenPaths {
			if path <= previous || !allowed[path] {
				return false, scheduleError("workspace write outside approved skill scope")
			}
			previous = path
		}
		if receipt.Status != "succeeded" {
			continue
		}
		if successful != nil {
			return false, scheduleError("multiple successful workspace effects")
		}
		copy := receipt
		successful = &copy
	}
	if successful == nil {
		return false, nil
	}
	receipt := *successful
	var expected *contracts.ObjectReference
	if s.manifest != nil {
		ref := scheduleManifestRef(*s.manifest)
		expected = &ref
	}
	if !chainEqual(scheduleRef(receipt.InputManifestRef), expected) || receipt.OutputManifestRef == nil {
		return false, scheduleError("workspace manifest continuity")
	}
	m, err := s.manifestAt(*scheduleRef(receipt.OutputManifestRef))
	if err != nil {
		return false, err
	}
	if !chainBefore(m.CreatedAt, receipt.CompletedAt, false) || !chainBefore(receipt.CompletedAt, op.UpdatedAt, false) {
		return false, scheduleError("workspace receipt timing")
	}
	// Each claimed write must occur in the actual registered output and match
	// the exact approved bytes. A renamed/empty success receipt is insufficient.
	for _, path := range receipt.WrittenPaths {
		matched := false
		for _, f := range m.Files {
			for _, change := range s.r.Control.Audit.Proposal.Changes {
				if f.Path == path && change.Path == path && change.After != nil && change.AfterSHA256 != nil && f.SHA256 == *change.AfterSHA256 && f.Size == int64(len([]byte(*change.After))) {
					matched = true
				}
			}
		}
		if !matched {
			return false, scheduleError("written path not proven by approved output")
		}
	}
	if len(receipt.WrittenPaths) == 0 {
		return false, scheduleError("empty successful workspace effect")
	}
	if len(receipt.WrittenPaths) != len(allowed) {
		return false, scheduleError("incomplete successful workspace effect")
	}
	before := map[string]contracts.ArtifactEntry{}
	if s.manifest != nil {
		for _, f := range s.manifest.Files {
			before[f.Path] = f
		}
	}
	for _, f := range m.Files {
		if !allowed[f.Path] {
			old, exists := before[f.Path]
			if !exists || !chainEqual(old, f) {
				return false, scheduleError("output changed an unreported path")
			}
		}
		delete(before, f.Path)
	}
	if len(before) != 0 {
		return false, scheduleError("output removed a previous file")
	}
	if !s.settled(x, receipt.AttemptID, "action", scope, receipt.StartedAt, receipt.CompletedAt) {
		return false, nil
	}
	ref := contracts.ObjectReference{ObjectType: "workspace", ObjectID: receipt.ReceiptID, ObjectVersion: 1, SourceVersion: receipt.SourceVersion, Hash: receipt.ReceiptHash}
	if !scheduleHasRef(op.EvidenceRefs, ref) {
		return false, scheduleError("operation does not reference workspace receipt")
	}
	s.manifest = m
	s.completedAt = receipt.CompletedAt
	return true, nil
}

func scheduleHasRef(refs []contracts.ObjectReference, ref contracts.ObjectReference) bool {
	for _, value := range refs {
		if value == ref {
			return true
		}
	}
	return false
}

func (s *canonicalScheduler) tools(x contracts.ExecutionRequest, op contracts.OperationRecord) (bool, error) {
	commands, err := governance.MergePostCommands(*s.r.FrozenDefinition, *s.r.Control.Audit.Plan)
	if err != nil {
		return false, err
	}
	if len(commands) == 0 {
		return false, scheduleError("verification has no frozen post commands")
	}
	// One successful VERIFY attempt must contain the complete command order.
	// Mixing tidy from one candidate copy with build from another is insufficient.
	for _, parent := range s.r.Control.Audit.Budget.Reservations {
		if parent.Kind != "action" || parent.ScopeID != "verify" || parent.RequestHash == nil || *parent.RequestHash != x.RequestHash {
			continue
		}
		complete, err := s.toolAttempt(x, op, commands, parent)
		if err != nil || complete {
			return complete, err
		}
	}
	return false, nil
}

func (s *canonicalScheduler) toolAttempt(x contracts.ExecutionRequest, op contracts.OperationRecord, commands []contracts.PostCommand, parent contracts.BudgetReservation) (bool, error) {
	ref := scheduleManifestRef(*s.manifest)
	lastCompleted := parent.ReservedAt
	for _, command := range commands {
		passed := false
		identity, _ := contracts.HashValue([]string{x.RequestHash, parent.AttemptID, command.CommandID})
		attemptID := "tool_" + identity[:40]
		for _, receipt := range s.r.Control.Audit.ToolReceipts {
			if receipt.ExecutionID != x.ExecutionID || receipt.CommandID != command.CommandID || receipt.AttemptID != attemptID {
				continue
			}
			boundTool := false
			for _, tool := range s.r.FrozenDefinition.ToolBindings {
				if tool.ToolID == command.Executable && chainEqual(tool, receipt.Tool) {
					boundTool = true
				}
			}
			if !hashMatches("ToolExecutionReceipt", receipt, receipt.ReceiptHash) || !boundTool || !chainEqual(command.Arguments, receipt.Arguments) || receipt.CwdScope != command.CwdScope || receipt.EnvironmentPolicy != command.EnvironmentPolicy || receipt.OutputSHA256 != contracts.BytesHash([]byte(receipt.Output)) || len([]byte(receipt.Output)) > 65536 || !s.receiptBinding(x, receipt.RunID, receipt.SourceVersion, receipt.DefinitionSnapshotHash, receipt.ExecutionID, receipt.OperationID, receipt.RequestHash, receipt.ApprovalID, receipt.ApprovalHash, receipt.ProposalSnapshotHash, receipt.PlanHash, receipt.Workspace, receipt.StartedAt, receipt.CompletedAt) {
				return false, scheduleError("tool receipt hash, tool, argv or binding")
			}
			if receipt.Status != "succeeded" {
				continue
			}
			if receipt.OutputManifestRef == nil {
				continue
			} // candidate changes require a new approval
			if !chainEqual(scheduleRef(receipt.InputManifestRef), &ref) || !chainEqual(scheduleRef(receipt.OutputManifestRef), &ref) || receipt.ExitCode == nil || *receipt.ExitCode != 0 || !chainBefore(receipt.CompletedAt, op.UpdatedAt, false) {
				return false, scheduleError("tool receipt manifest, exit or timing")
			}
			if !chainBefore(lastCompleted, receipt.StartedAt, false) {
				return false, scheduleError("tool command order changed")
			}
			if !s.settled(x, receipt.AttemptID, "tool", "tool:"+command.CommandID, receipt.StartedAt, receipt.CompletedAt) {
				continue
			}
			evidence := contracts.ObjectReference{ObjectType: "evidence", ObjectID: receipt.ReceiptID, ObjectVersion: 1, SourceVersion: receipt.SourceVersion, Hash: receipt.ReceiptHash}
			if !scheduleHasRef(op.EvidenceRefs, evidence) {
				return false, scheduleError("operation does not reference tool receipt")
			}
			passed = true
			lastCompleted = receipt.CompletedAt
		}
		if !passed {
			return false, nil
		}
	}
	complete := s.settled(x, parent.AttemptID, "action", "verify", parent.ReservedAt, lastCompleted)
	if complete {
		s.completedAt = lastCompleted
	}
	return complete, nil
}

func (s *canonicalScheduler) result(x contracts.ExecutionRequest, op contracts.OperationRecord) (bool, error) {
	r, exists := s.results[x.OperationID]
	if !exists {
		return false, nil
	}
	kind, parameters, _ := scheduleAction(x.Action)
	if r.ManifestRef == nil || *r.ManifestRef != scheduleManifestRef(*s.manifest) {
		return false, scheduleError("executor result manifest")
	}
	// Match the original persisted reservation start; request creation precedes
	// admission and cannot be used as a fabricated executor start timestamp.
	settled := false
	for _, reservation := range s.r.Control.Audit.Budget.Reservations {
		if reservation.AttemptID == r.AttemptID && chainBefore(reservation.ReservedAt, r.CompletedAt, false) {
			settled = s.settled(x, r.AttemptID, "action", kind, reservation.ReservedAt, r.CompletedAt)
		}
	}
	if !settled {
		return false, nil
	}
	if kind != "package" {
		var target contracts.GitLabTarget
		_ = json.Unmarshal(s.r.Control.Audit.Plan.DeliveryTarget, &target)
		if r.ObservedTarget == nil || !chainEqual(*r.ObservedTarget, target) || r.ProjectID <= 0 {
			return false, scheduleError("actual remote target or project missing")
		}
	}
	switch kind {
	case "create_repository":
		if op.ExternalObjectID == nil || *op.ExternalObjectID != strconv.FormatInt(r.ProjectID, 10) {
			return false, scheduleError("created project identity")
		}
		s.projectID = r.ProjectID
	case "push_repository":
		if r.ProjectID != s.projectID || len(r.CommitSHA) != 40 || op.ExternalObjectID == nil || *op.ExternalObjectID != r.CommitSHA {
			return false, scheduleError("pushed project or commit identity")
		}
		for _, c := range r.CommitSHA {
			if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
				return false, scheduleError("invalid commit SHA")
			}
		}
		found := false
		for _, f := range s.manifest.Files {
			if f.Path == ".gitlab-ci.yml" && f.SHA256 == r.CIConfigSHA256 {
				found = true
			}
		}
		if !found {
			return false, scheduleError("pushed CI configuration outside manifest")
		}
		s.commitSHA, s.ciSHA = r.CommitSHA, r.CIConfigSHA256
	case "trigger_ci":
		var p contracts.TriggerCIParameters
		_ = json.Unmarshal(parameters, &p)
		if r.ProjectID != p.ProjectID || r.CommitSHA != p.CommitSha || r.CIConfigSHA256 != p.CIConfigSHA256 || r.PipelineID <= 0 || op.ExternalObjectID == nil || *op.ExternalObjectID != strconv.FormatInt(r.PipelineID, 10) {
			return false, scheduleError("CI result does not bind original project, commit, configuration or pipeline")
		}
		if r.PipelineStatus != "success" {
			return false, nil
		}
	case "package":
		if contracts.Validate("Hash", r.PackageSHA256) != nil || r.PackageBytes <= 0 || r.ObservedTarget != nil || r.ProjectID != 0 || r.PipelineID != 0 || op.ExternalObjectID == nil || *op.ExternalObjectID != r.PackageSHA256 {
			return false, scheduleError("package byte identity")
		}
	default:
		return false, fmt.Errorf("unsupported scheduler result %q", kind)
	}
	s.completedAt = r.CompletedAt
	return true, nil
}
