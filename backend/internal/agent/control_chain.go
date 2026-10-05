package agent

import (
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"agentstudio/internal/contracts"
	"agentstudio/internal/governance"
)

// verifyControlChain checks persisted provenance without granting authority or
// reconstructing historical facts from prose. A caller must still validate a
// new candidate against its live registry, budget and executor observations.
// AuditHash and the durable object index belong to the surrounding Store check;
// this function is also usable before a candidate audit has been rehashed.
func verifyControlChain(r Run) error {
	if r.Control == nil {
		return nil
	}
	if r.FrozenDefinition == nil {
		return chainError("frozen definition missing")
	}
	c := controlChain{run: r, audit: &r.Control.Audit, proposals: map[string]contracts.Proposal{}, approvals: map[string]contracts.Approval{}, permissions: map[string]contracts.PermissionSnapshot{}, messages: map[string]contracts.MessageEnvelope{}, manifests: map[string]contracts.ArtifactManifest{}}
	if !chainEqual(c.audit.DefinitionSnapshot, *r.FrozenDefinition) || !hashMatches("DefinitionSnapshot", *r.FrozenDefinition, r.FrozenDefinition.SnapshotHash) {
		return chainError("frozen definition identity or hash")
	}
	if err := c.authorities(); err != nil {
		return err
	}
	if err := c.messageHistory(); err != nil {
		return err
	}
	if err := c.decisions(); err != nil {
		return err
	}
	if err := verifyControlRemoteObservations(r); err != nil {
		return err
	}
	if err := verifyControlGitInvocations(r); err != nil {
		return err
	}
	remoteIDs, remoteSuccess := map[string]bool{}, map[string]bool{}
	for _, receipt := range c.audit.RemoteReceipts {
		if remoteIDs[receipt.ReceiptID] || receipt.Status == "succeeded" && remoteSuccess[receipt.OperationID] {
			return remoteReceiptError("duplicate remote receipt or successful operation")
		}
		remoteIDs[receipt.ReceiptID] = true
		if receipt.Status == "succeeded" {
			remoteSuccess[receipt.OperationID] = true
		}
		if err := ValidateControlRemoteReceipt(r, receipt); err != nil {
			return err
		}
		linked := false
		for _, operation := range c.audit.Operations {
			if operation.OperationID == receipt.OperationID && scheduleHasRef(operation.EvidenceRefs, controlRemoteReceiptRef(receipt)) {
				linked = true
			}
		}
		if !linked {
			return remoteReceiptError("operation lacks exact public receipt reference")
		}
	}
	for _, candidate := range c.audit.VerificationCandidates {
		if _, _, _, err := controlCandidateEvidence(r, candidate); err != nil {
			return err
		}
	}
	for _, p := range c.audit.ProposalHistory {
		if p.Plan.VerificationCandidateRef == nil {
			continue
		}
		found := false
		for _, candidate := range c.audit.VerificationCandidates {
			if equalContractValue(p.Plan.VerificationCandidateRef, controlCandidateRef(candidate)) && candidate.SourceVersion < p.SourceVersion && chainBefore(candidate.CreatedAt, p.IssuedAt, false) {
				_, original, files, err := controlCandidateEvidence(r, candidate)
				if err != nil {
					return err
				}
				expected := original.Plan
				expected.VerificationCandidateRef = p.Plan.VerificationCandidateRef
				expected.PlanHash, err = contracts.HashObject("ProjectPlan", expected)
				if err != nil || !equalContractValue(expected, p.Plan) {
					return chainError("candidate revision changed the original business scope")
				}
				after := map[string]string{}
				for _, change := range p.Changes {
					if change.After != nil {
						after[change.Path] = *change.After
					}
				}
				found = equalContractValue(files, after)
			}
		}
		if !found {
			return chainError("candidate revision proposal is not the complete exact observed candidate")
		}
	}
	return nil
}

type controlChain struct {
	run         Run
	audit       *contracts.S2AuditBundle
	proposals   map[string]contracts.Proposal
	approvals   map[string]contracts.Approval
	permissions map[string]contracts.PermissionSnapshot
	messages    map[string]contracts.MessageEnvelope
	manifests   map[string]contracts.ArtifactManifest
}

func chainError(detail string) error {
	return fail("control_chain_mismatch", "受控决定链不完整或不一致: "+detail)
}
func chainEqual(a, b any) bool {
	x, e := contracts.HashValue(a)
	if e != nil {
		return false
	}
	y, e := contracts.HashValue(b)
	return e == nil && x == y
}
func chainTime(s string) (time.Time, error) {
	t, e := time.Parse(time.RFC3339Nano, s)
	if e != nil || t.IsZero() {
		return t, chainError("invalid timestamp")
	}
	return t, nil
}
func chainBefore(a, b string, strict bool) bool {
	x, e := chainTime(a)
	if e != nil {
		return false
	}
	y, e := chainTime(b)
	return e == nil && !x.After(y) && (!strict || x.Before(y))
}
func chainContains(values []string, value string) bool {
	for _, v := range values {
		if v == value {
			return true
		}
	}
	return false
}
func chainSubset(values, allowed []string) bool {
	for _, v := range values {
		if !chainContains(allowed, v) {
			return false
		}
	}
	return true
}
func proposalKey(id string, version int64) string { return fmt.Sprintf("%s/%d", id, version) }
func (c *controlChain) bound(version, runID string, source int64, definition string) bool {
	return version == c.run.FrozenDefinition.ContractVersion && runID == c.run.ID && source > 0 && source <= int64(c.run.SourceVersion) && definition == c.run.FrozenDefinition.SnapshotHash
}
func (c *controlChain) historyTime(stamp string) bool {
	return chainBefore(c.run.CreatedAt, stamp, false)
}
func (c *controlChain) authorities() error {
	var permissionVersion int64
	for _, p := range c.audit.PermissionHistory {
		if !hashMatches("PermissionSnapshot", p, p.PermissionHash) || p.PermissionVersion <= permissionVersion || c.permissions[p.PermissionHash].PermissionHash != "" || !c.historyTime(p.UpdatedAt) {
			return chainError("permission history")
		}
		permissionVersion = p.PermissionVersion
		c.permissions[p.PermissionHash] = p
	}
	var proposalVersion int64
	for _, p := range c.audit.ProposalHistory {
		key := proposalKey(p.ProposalID, p.ProposalVersion)
		grant, ok := c.permissions[p.PermissionHash]
		if !ok || !hashMatches("Proposal", p, p.SnapshotHash) || !c.bound(p.ContractVersion, p.RunID, p.SourceVersion, p.DefinitionSnapshotHash) || p.ProposalVersion <= proposalVersion || c.proposals[key].ProposalID != "" || !hashMatches("ProjectPlan", p.Plan, p.PlanHash) || p.PlanHash != p.Plan.PlanHash || p.Plan.ContractVersion != p.ContractVersion {
			return chainError("proposal history identity, plan or permission")
		}
		if grant.Revoked || !chainEqual(grant.DeliveryTarget, p.Plan.DeliveryTarget) || !chainSubset(p.RequiredPermissionIDs, grant.AllowedPermissions) || !chainSubset(p.AllowedActions, c.run.FrozenDefinition.Definition.ActionProtocol.AllowedActions) || !c.historyTime(p.IssuedAt) || !chainBefore(grant.UpdatedAt, p.IssuedAt, false) || !chainBefore(p.IssuedAt, p.ExpiresAt, true) {
			return chainError("proposal scope or validity interval")
		}
		for _, change := range p.Changes {
			if change.Before != nil && (change.BeforeSHA256 == nil || *change.BeforeSHA256 != contracts.BytesHash([]byte(*change.Before))) || change.After != nil && (change.AfterSHA256 == nil || *change.AfterSHA256 != contracts.BytesHash([]byte(*change.After))) {
				return chainError("proposal file content hash")
			}
		}
		proposalVersion = p.ProposalVersion
		c.proposals[key] = p
	}
	for _, a := range c.audit.ApprovalHistory {
		if c.approvals[a.ApprovalID].ApprovalID != "" {
			return chainError("duplicate approval identity")
		}
		if err := c.approval(a); err != nil {
			return err
		}
		c.approvals[a.ApprovalID] = a
	}
	for _, m := range c.audit.ManifestHistory {
		p, ok := c.proposals[proposalKey(m.ProposalID, m.ProposalVersion)]
		if !ok || !hashMatches("ArtifactManifest", m, m.Hash) || !c.bound(m.ContractVersion, m.RunID, m.SourceVersion, m.DefinitionSnapshotHash) || m.SourceVersion != p.SourceVersion || m.ProposalSnapshotHash != p.SnapshotHash || m.PlanHash != p.PlanHash || !chainBefore(p.IssuedAt, m.CreatedAt, false) || c.manifests[m.Hash].Hash != "" {
			return chainError("manifest history")
		}
		c.manifests[m.Hash] = m
	}
	if p := c.audit.Plan; p != nil {
		if !hashMatches("ProjectPlan", *p, p.PlanHash) || p.ContractVersion != c.run.FrozenDefinition.ContractVersion {
			return chainError("current plan hash")
		}
	}
	if p := c.audit.Proposal; p != nil {
		if !c.proposalProjection(*p) || c.audit.Plan == nil || !chainEqual(p.Plan, *c.audit.Plan) {
			return chainError("current proposal archive or plan")
		}
	}
	if p := c.audit.PermissionSnapshot; p != nil {
		if !chainEqual(c.permissions[p.PermissionHash], *p) || p.PermissionVersion != permissionVersion {
			return chainError("current permission archive")
		}
	} else if len(c.audit.PermissionHistory) != 0 {
		return chainError("latest permission projection missing")
	}
	if a := c.audit.Approval; a != nil {
		if !c.approvalProjection(*a) {
			return chainError("current approval archive or lifecycle")
		}
	}
	if m := c.audit.Manifest; m != nil {
		if !chainEqual(c.manifests[m.Hash], *m) {
			return chainError("current manifest archive")
		}
	}
	return nil
}
func (c *controlChain) proposalProjection(p contracts.Proposal) bool {
	old, ok := c.proposals[proposalKey(p.ProposalID, p.ProposalVersion)]
	if !ok || !hashMatches("Proposal", p, p.SnapshotHash) {
		return false
	}
	p.Status = old.Status
	return chainEqual(p, old)
}
func (c *controlChain) approval(a contracts.Approval) error {
	p, ok := c.proposals[proposalKey(a.ProposalID, a.ProposalVersion)]
	g, gok := c.permissions[a.PermissionHash]
	if !ok || !gok || !hashMatches("Approval", a, a.ApprovalHash) || !c.bound(a.ContractVersion, a.RunID, a.SourceVersion, a.DefinitionSnapshotHash) || a.SourceVersion != p.SourceVersion || a.ProposalSnapshotHash != p.SnapshotHash || a.PlanHash != p.PlanHash || a.PermissionHash != p.PermissionHash || a.ActorID != g.ActorID || g.Revoked || !chainEqual(a.DeliveryTarget, p.Plan.DeliveryTarget) || !chainSubset(a.AllowedActions, p.AllowedActions) || a.IssuedAt != p.IssuedAt || a.ExpiresAt != p.ExpiresAt {
		return chainError("approval history binding")
	}
	if !chainBefore(a.IssuedAt, a.AcceptedAt, false) || !chainBefore(a.AcceptedAt, a.ExpiresAt, true) {
		return chainError("approval acceptance time")
	}
	for _, check := range p.Checks {
		if !check.Passed {
			return chainError("approval of blocked proposal")
		}
	}
	if a.StartedAt != nil {
		if !chainBefore(a.AcceptedAt, *a.StartedAt, false) || !chainBefore(*a.StartedAt, a.ExpiresAt, true) {
			return chainError("approval start time")
		}
	}
	if a.RevokedAt != nil {
		if !chainBefore(a.AcceptedAt, *a.RevokedAt, false) || a.StartedAt != nil && !chainBefore(*a.StartedAt, *a.RevokedAt, false) {
			return chainError("approval revocation time")
		}
	}
	if a.Status == "accepted" && (a.StartedAt != nil || a.RevokedAt != nil) || a.Status == "started" && (a.StartedAt == nil || a.RevokedAt != nil) || a.Status == "revoked" && a.RevokedAt == nil || a.Status != "revoked" && a.RevokedAt != nil {
		return chainError("approval lifecycle")
	}
	return nil
}
func (c *controlChain) approvalProjection(a contracts.Approval) bool {
	old, ok := c.approvals[a.ApprovalID]
	if !ok || c.approval(a) != nil {
		return false
	}
	a.Status, a.StartedAt, a.RevokedAt = old.Status, old.StartedAt, old.RevokedAt
	return chainEqual(a, old)
}
func (c *controlChain) messageHistory() error {
	var sequence, source, state int64
	for _, m := range c.audit.Messages {
		if contracts.ValidateVersion("MessageEnvelope", c.run.FrozenDefinition.ContractVersion, m) != nil || m.RunID != c.run.ID || m.Sequence <= sequence || m.SourceVersion < source || m.SourceVersion > int64(c.run.SourceVersion) || m.StateSequence < state || m.StateSequence > int64(c.run.StateSequence) || m.Modes != runModes(c.run) || c.messages[m.MessageID].MessageID != "" || !c.historyTime(m.CreatedAt) {
			return chainError("registered message history")
		}
		sequence, source, state = m.Sequence, m.SourceVersion, m.StateSequence
		c.messages[m.MessageID] = m
	}
	return nil
}
func (c *controlChain) decisions() error {
	if len(c.audit.Decisions) != len(c.audit.Validations) || len(c.audit.Decisions) != len(c.audit.Contexts) || len(c.audit.Decisions) != len(c.run.Control.ValidationInputs) {
		return chainError("decision, validation, context and private input cardinality")
	}
	contexts := map[string]contracts.ContextRecord{}
	contextIDs := map[string]bool{}
	for _, v := range c.audit.Contexts {
		if contexts[v.DecisionID].ContextID != "" || contextIDs[v.ContextID] {
			return chainError("duplicate context")
		}
		contexts[v.DecisionID] = v
		contextIDs[v.ContextID] = true
	}
	validations := map[string]contracts.ActionValidation{}
	validationIDs := map[string]bool{}
	for _, v := range c.audit.Validations {
		if validations[v.DecisionID].ValidationID != "" || validationIDs[v.ValidationID] {
			return chainError("duplicate validation")
		}
		validations[v.DecisionID] = v
		validationIDs[v.ValidationID] = true
	}
	inputs := map[string]governance.Input{}
	for _, v := range c.run.Control.ValidationInputs {
		if inputs[v.Decision.DecisionID].Decision.DecisionID != "" {
			return chainError("duplicate validator input")
		}
		inputs[v.Decision.DecisionID] = v
	}
	executions := map[string]contracts.ExecutionRequest{}
	for _, v := range c.audit.Executions {
		if executions[v.ExecutionID].ExecutionID != "" {
			return chainError("duplicate execution")
		}
		executions[v.ExecutionID] = v
	}
	decisions := map[string]bool{}
	usedExecutions := map[string]bool{}
	for _, d := range c.audit.Decisions {
		if decisions[d.DecisionID] || !hashMatches("DecisionProposal", d, d.DecisionHash) || !c.bound(d.ContractVersion, d.RunID, d.SourceVersion, d.DefinitionSnapshotHash) || d.StateSequence > int64(c.run.StateSequence) || d.DecisionMode != c.run.DecisionMode || !c.historyTime(d.CreatedAt) {
			return chainError("decision identity or hash")
		}
		decisions[d.DecisionID] = true
		context, cok := contexts[d.DecisionID]
		v, vok := validations[d.DecisionID]
		in, iok := inputs[d.DecisionID]
		if !cok || !vok || !iok || !hashMatches("ContextRecord", context, context.ContextHash) || !hashMatches("ActionValidation", v, v.ValidationHash) {
			return chainError("missing or invalid decision children")
		}
		if context.ContractVersion != d.ContractVersion || context.RunID != d.RunID || context.SourceVersion != d.SourceVersion || context.StateSequence != d.StateSequence || context.DefinitionSnapshotHash != d.DefinitionSnapshotHash || context.CreatedAt != d.CreatedAt || context.InputContextHash != d.InputContextHash || !chainEqual(context.InputMessageRefs, d.InputMessageRefs) {
			return chainError("context binding")
		}
		actual, e := contracts.HashValue(context.Messages)
		if e != nil || actual != d.InputContextHash {
			return chainError("actual ordered context hash")
		}
		if v.ContractVersion != d.ContractVersion || v.RunID != d.RunID || v.SourceVersion != d.SourceVersion || v.StateSequence != d.StateSequence || v.DefinitionSnapshotHash != d.DefinitionSnapshotHash || v.DecisionKind != d.Advice.Kind || !chainBefore(d.CreatedAt, v.ValidatedAt, false) {
			return chainError("validation identity, source or time")
		}
		if v.Verdict == "allowed" && !chainEqual(v.NormalizedAdvice, &d.Advice) || v.Verdict == "needs_clarification" && d.Advice.Kind != "clarification" {
			return chainError("normalized advice or verdict")
		}
		for _, finding := range v.Findings {
			if v.Verdict != "rejected" && !finding.Passed {
				return chainError("allowed validation has failed finding")
			}
		}
		if err := c.input(in, d, context, v); err != nil {
			return err
		}
		if v.ExecutionRequest != nil {
			x := *v.ExecutionRequest
			if usedExecutions[x.ExecutionID] || !chainEqual(executions[x.ExecutionID], x) {
				return chainError("execution archive cardinality or body")
			}
			if err := c.execution(x, d, v, in.Facts); err != nil {
				return err
			}
			usedExecutions[x.ExecutionID] = true
		}
	}
	if len(usedExecutions) != len(executions) {
		return chainError("orphan execution")
	}
	return nil
}
func (c *controlChain) input(in governance.Input, d contracts.DecisionProposal, record contracts.ContextRecord, v contracts.ActionValidation) error {
	f := in.Facts
	if !chainEqual(in.Decision, d) || !chainEqual(in.OrderedContext, record.Messages) || f.RunID != d.RunID || f.SourceVersion != d.SourceVersion || f.StateSequence != d.StateSequence || f.Modes != runModes(c.run) || f.ActorID != localActorID || f.CommandID == "" || !chainEqual(f.FrozenDefinition, *c.run.FrozenDefinition) || f.ObservedModelUsage == nil || !chainEqual(*f.ObservedModelUsage, d.ModelUsage) || f.Now.IsZero() || f.Now.UTC().Format(time.RFC3339Nano) != v.ValidatedAt || contracts.Validate("RunState", f.State) != nil {
		return chainError("private validator facts do not bind public decision")
	}
	if len(in.InputMessages) != len(d.InputMessageRefs) {
		return chainError("validator input messages cardinality")
	}
	identity, err := contracts.HashValue([]any{f.RunID, f.SourceVersion, f.StateSequence, f.CommandID, d.DecisionID, d.DecisionHash})
	if err != nil || v.ValidationID != "validation_"+identity[:32] {
		return chainError("validation identity does not bind its original input")
	}
	var sequence int64
	for i, ref := range d.InputMessageRefs {
		m, ok := c.messages[ref.ObjectID]
		actual, e := governance.MessageRef(m)
		if !ok || e != nil || actual != ref || !chainEqual(m, in.InputMessages[i]) || m.SourceVersion > d.SourceVersion || m.StateSequence > d.StateSequence || m.Sequence <= sequence || !chainBefore(m.CreatedAt, d.CreatedAt, false) {
			return chainError("input reference is not the exact registered earlier message")
		}
		sequence = m.Sequence
	}
	if f.Plan != nil {
		found := false
		for _, p := range c.proposals {
			if chainEqual(p.Plan, *f.Plan) && chainBefore(p.IssuedAt, v.ValidatedAt, false) {
				found = true
				break
			}
		}
		if !found || !hashMatches("ProjectPlan", *f.Plan, f.Plan.PlanHash) {
			return chainError("private plan is absent from prior public proposals")
		}
	}
	if f.Proposal != nil {
		if !c.proposalProjection(*f.Proposal) || !chainBefore(f.Proposal.IssuedAt, v.ValidatedAt, false) {
			return chainError("private proposal has no prior archive")
		}
	}
	if f.Approval != nil {
		if !c.approvalProjection(*f.Approval) || !chainBefore(f.Approval.AcceptedAt, v.ValidatedAt, false) || f.Approval.StartedAt != nil && !chainBefore(*f.Approval.StartedAt, v.ValidatedAt, false) || f.Approval.RevokedAt != nil && !chainBefore(*f.Approval.RevokedAt, v.ValidatedAt, false) {
			return chainError("private approval archive or observed lifecycle time")
		}
	}
	if f.Permission != nil {
		if !chainEqual(c.permissions[f.Permission.PermissionHash], *f.Permission) || !chainBefore(f.Permission.UpdatedAt, v.ValidatedAt, false) {
			return chainError("private permission has no prior archive")
		}
	}
	if f.Manifest != nil {
		if !chainEqual(c.manifests[f.Manifest.Hash], *f.Manifest) || !chainBefore(f.Manifest.CreatedAt, v.ValidatedAt, false) {
			return chainError("private manifest has no prior archive")
		}
	}
	for _, tool := range f.AvailableTools {
		found := false
		for _, bound := range c.run.FrozenDefinition.ToolBindings {
			if chainEqual(tool, bound) {
				found = true
			}
		}
		if !found {
			return chainError("private tool identity not frozen")
		}
	}
	if f.Workspace != nil && (contracts.Validate("WorkspaceReference", *f.Workspace) != nil || f.Workspace.RunID != c.run.ID) {
		return chainError("private workspace ownership")
	}
	return nil
}
func (c *controlChain) execution(x contracts.ExecutionRequest, d contracts.DecisionProposal, v contracts.ActionValidation, f governance.Facts) error {
	if !hashMatches("ExecutionRequest", x, x.RequestHash) || x.ContractVersion != d.ContractVersion || x.RunID != d.RunID || x.SourceVersion != d.SourceVersion || x.StateSequenceAtValidation != d.StateSequence || x.DefinitionSnapshotHash != d.DefinitionSnapshotHash || x.ValidationID != v.ValidationID || x.ValidationHash != v.ValidationHash || x.CreatedAt != v.ValidatedAt || x.CommandID != f.CommandID || d.Advice.Action == nil || !chainEqual(x.Action, *d.Advice.Action) || f.ExpectedAction == nil || !chainEqual(x.Action, *f.ExpectedAction) || f.Workspace == nil || !chainEqual(x.Workspace, *f.Workspace) || x.Workspace.RunID != x.RunID {
		return chainError("execution identity, action or input binding")
	}
	a, ok := c.approvals[x.Approval.ApprovalID]
	p, pok := c.proposals[proposalKey(x.Approval.ProposalID, x.Approval.ProposalVersion)]
	grant, gok := c.permissions[x.Approval.PermissionHash]
	expected := contracts.ApprovalBinding{ApprovalID: a.ApprovalID, ProposalID: a.ProposalID, ProposalVersion: a.ProposalVersion, SourceVersion: a.SourceVersion, ProposalSnapshotHash: a.ProposalSnapshotHash, DefinitionSnapshotHash: a.DefinitionSnapshotHash, PermissionHash: a.PermissionHash, IssuedAt: a.IssuedAt, ExpiresAt: a.ExpiresAt}
	if !ok || !pok || !gok || !chainEqual(expected, x.Approval) || x.SourceVersion != a.SourceVersion || x.PlanHash != a.PlanHash || x.PlanHash != p.PlanHash || f.Plan == nil || !chainEqual(*f.Plan, p.Plan) || f.Proposal == nil || f.Proposal.Status != "accepted" || f.Proposal.SnapshotHash != p.SnapshotHash || f.Approval == nil || f.Approval.ApprovalID != a.ApprovalID || f.Permission == nil || !chainEqual(*f.Permission, grant) || grant.Revoked || f.Approval.RevokedAt != nil || !chainContains([]string{"accepted", "started"}, f.Approval.Status) || !chainBefore(a.AcceptedAt, x.CreatedAt, false) {
		return chainError("execution authority does not bind exact historical approval")
	}
	if f.Approval.StartedAt == nil && !chainBefore(x.CreatedAt, a.ExpiresAt, true) {
		return chainError("execution first application expired")
	}
	// A private copy of an old grant cannot erase a later trusted revocation or
	// scope change that had already happened when this request was validated.
	for _, permission := range c.audit.PermissionHistory {
		if permission.PermissionVersion > grant.PermissionVersion && chainBefore(permission.UpdatedAt, x.CreatedAt, false) {
			return chainError("execution used a permission superseded before validation")
		}
	}
	var action struct {
		ActionType string          `json:"action_type"`
		Parameters json.RawMessage `json:"parameters"`
	}
	if json.Unmarshal(x.Action, &action) != nil {
		return chainError("execution action")
	}
	opHash, err := contracts.HashValue([]any{f.RunID, action.ActionType, f.SourceVersion, f.OperationSequence})
	if err != nil || f.OperationSequence < 1 || x.OperationID != "op_"+opHash[:32] {
		return chainError("operation identity does not bind the original action")
	}
	executionHash, err := contracts.HashValue([]any{v.ValidationID, x.OperationID})
	if err != nil || x.ExecutionID != "execution_"+executionHash[:32] {
		return chainError("execution identity does not bind validation and operation")
	}
	stage := map[string]string{"scaffold": "IMPLEMENT", "apply_skill": "IMPLEMENT", "verify": "VERIFY", "create_repository": "DELIVER", "push_repository": "DELIVER", "trigger_ci": "DELIVER", "package": "DELIVER"}[action.ActionType]
	if f.State != stage {
		return chainError("execution contradicts its observed business stage")
	}
	if err := c.actionParameters(action.ActionType, action.Parameters, p, f); err != nil {
		return err
	}
	if !chainContains(a.AllowedActions, action.ActionType) || !chainContains(p.AllowedActions, action.ActionType) || !chainContains(c.run.FrozenDefinition.Definition.ActionProtocol.AllowedActions, action.ActionType) {
		return chainError("execution action not approved")
	}
	required := map[string][]string{"scaffold": {"workspace_write"}, "verify": {"tool_go", "workspace_read", "workspace_write"}, "create_repository": {"gitlab_create"}, "push_repository": {"gitlab_push", "workspace_read"}, "trigger_ci": {"gitlab_ci"}, "package": {"package_write", "workspace_read"}}[action.ActionType]
	if action.ActionType == "apply_skill" {
		var params contracts.ApplySkillParameters
		if json.Unmarshal(action.Parameters, &params) != nil {
			return chainError("skill parameters")
		}
		var selected struct {
			SkillID string `json:"skill_id"`
		}
		if json.Unmarshal(params.Selection, &selected) != nil {
			return chainError("skill identity")
		}
		for _, skill := range c.run.FrozenDefinition.Skills {
			if skill.SkillID == selected.SkillID {
				required = append([]string{}, skill.RequiredPermissions...)
			}
		}
	}
	sort.Strings(required)
	if len(required) == 0 || !chainEqual(required, x.RequiredPermissionIDs) || !chainSubset(required, grant.AllowedPermissions) || !chainSubset(required, p.RequiredPermissionIDs) {
		return chainError("execution permission set")
	}
	if c.run.DeliveryMode == "local" && (action.ActionType == "create_repository" || action.ActionType == "push_repository" || action.ActionType == "trigger_ci") {
		return chainError("local execution has remote authority")
	}
	if f.Operation != nil {
		target, e := governance.RequestTargetHash(x)
		if e != nil || f.Operation.RunID != x.RunID || f.Operation.SourceVersion != x.SourceVersion || f.Operation.OperationID != x.OperationID || f.Operation.OperationSequence != f.OperationSequence || f.Operation.RequestHash != x.RequestHash || f.Operation.TargetHash != target || f.Operation.ActionType != action.ActionType || f.Operation.Status != "ready" {
			return chainError("execution operation binding")
		}
	}
	return nil
}

func (c *controlChain) actionParameters(kind string, parameters json.RawMessage, proposal contracts.Proposal, facts governance.Facts) error {
	manifest := func(ref contracts.ObjectReference) bool {
		m, ok := c.manifests[ref.Hash]
		return ok && ref.ObjectType == "manifest" && ref.ObjectID == m.ManifestID && ref.ObjectVersion == m.ManifestVersion && ref.SourceVersion == m.SourceVersion && m.SourceVersion == proposal.SourceVersion && m.ProposalID == proposal.ProposalID && m.ProposalVersion == proposal.ProposalVersion && m.ProposalSnapshotHash == proposal.SnapshotHash && m.PlanHash == proposal.PlanHash && facts.Manifest != nil && chainEqual(*facts.Manifest, m)
	}
	switch kind {
	case "scaffold":
		var p contracts.ScaffoldParameters
		if json.Unmarshal(parameters, &p) != nil || p.PlanHash != proposal.PlanHash {
			return chainError("scaffold parameter plan")
		}
	case "apply_skill":
		var p contracts.ApplySkillParameters
		if json.Unmarshal(parameters, &p) != nil || p.PlanHash != proposal.PlanHash {
			return chainError("skill parameter plan")
		}
		selected := false
		for _, s := range proposal.Plan.SelectedSkills {
			selected = selected || chainEqual(s, p.Selection)
		}
		if !selected {
			return chainError("skill parameters not in approved selections")
		}
	case "verify":
		var p contracts.VerifyParameters
		if json.Unmarshal(parameters, &p) != nil || !manifest(p.ManifestRef) {
			return chainError("verify parameter manifest")
		}
		checks := []string{}
		for _, check := range c.run.FrozenDefinition.Definition.CompletionPolicy.RequiredChecks {
			if chainContains(check.DeliveryModes, c.run.DeliveryMode) {
				checks = append(checks, check.CheckID)
			}
		}
		sort.Strings(checks)
		if !chainEqual(checks, p.RequiredCheckIDs) {
			return chainError("verify parameter checks")
		}
	case "create_repository":
		var p contracts.CreateRepositoryParameters
		if json.Unmarshal(parameters, &p) != nil || !chainEqual(p.Target, proposal.Plan.DeliveryTarget) {
			return chainError("repository target not approved")
		}
	case "push_repository":
		var p contracts.PushRepositoryParameters
		if json.Unmarshal(parameters, &p) != nil || !chainEqual(p.Target, proposal.Plan.DeliveryTarget) || !manifest(p.ManifestRef) || p.ExpectedBranchSha != nil || facts.RemoteProjectID <= 0 || p.ProjectID != facts.RemoteProjectID {
			return chainError("push target, manifest or project mismatch")
		}
	case "trigger_ci":
		var p contracts.TriggerCIParameters
		if json.Unmarshal(parameters, &p) != nil || !chainEqual(p.Target, proposal.Plan.DeliveryTarget) || facts.RemoteProjectID <= 0 || p.ProjectID != facts.RemoteProjectID || p.CommitSha != facts.CommitSHA || p.CIConfigSHA256 != facts.CIConfigSHA256 || facts.Manifest == nil {
			return chainError("CI target, commit or configuration mismatch")
		}
		found := false
		for _, file := range facts.Manifest.Files {
			found = found || file.Path == ".gitlab-ci.yml" && file.SHA256 == p.CIConfigSHA256
		}
		if !found || facts.Manifest.SourceVersion != proposal.SourceVersion || facts.Manifest.ProposalSnapshotHash != proposal.SnapshotHash {
			return chainError("CI configuration absent from approved manifest")
		}
	case "package":
		var p contracts.PackageParameters
		if json.Unmarshal(parameters, &p) != nil || !manifest(p.ManifestRef) {
			return chainError("package parameter manifest")
		}
	default:
		return chainError("unsupported execution action")
	}
	return nil
}
