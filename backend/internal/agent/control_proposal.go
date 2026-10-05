package agent

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"agentstudio/internal/contracts"
)

const localActorID = "local-user"

func hashMatches(schema string, value any, expected string) bool {
	actual, err := contracts.HashObject(schema, value)
	return err == nil && actual == expected && contracts.Validate(schema, value) == nil
}

func equalContractValue(left, right any) bool {
	a, e1 := contracts.HashValue(left)
	b, e2 := contracts.HashValue(right)
	return e1 == nil && e2 == nil && a == b
}

func (s *Service) canonicalTargetAllowed(r Run, plan contracts.ProjectPlan) error {
	if r.DeliveryMode == "local" {
		var target contracts.LocalTarget
		if err := contracts.DecodeVersion("LocalTarget", r.FrozenDefinition.ContractVersion, plan.DeliveryTarget, &target); err != nil {
			return fail("target_mismatch", "本地Run只能使用准确本地交付目标")
		}
		return nil
	}
	var target contracts.GitLabTarget
	if err := contracts.DecodeVersion("GitLabTarget", r.FrozenDefinition.ContractVersion, plan.DeliveryTarget, &target); err != nil {
		return fail("target_mismatch", "远程Run需要完整准确的GitLab目标")
	}
	policy := r.FrozenDefinition.Definition.RepositoryPolicy
	if r.DeliveryMode != "gitlab" || target.InstanceURL != s.Config.GitLabURL || target.NamespaceID != s.Config.GitLabNamespace || target.ProjectPath != plan.ProjectName || target.Ref != policy.RequiredRef || target.Visibility != policy.Visibility {
		return fail("target_changed", "交付目标不在当前可信实例、namespace、项目和ref范围")
	}
	return nil
}

// proposalScope is a deterministic reduction of the exact selected capabilities.
// There is no permission, target, timeout or actor supplied by model prose.
func proposalScope(r Run, plan contracts.ProjectPlan) ([]string, []string, error) {
	wanted := []string{"scaffold", "verify", "package"}
	permissions := map[string]bool{"workspace_write": true, "workspace_read": true, "tool_go": true, "package_write": true}
	if len(plan.SelectedSkills) > 0 {
		wanted = append(wanted, "apply_skill")
	}
	for _, raw := range plan.SelectedSkills {
		var selection struct {
			SkillID string `json:"skill_id"`
		}
		if err := json.Unmarshal(raw, &selection); err != nil {
			return nil, nil, err
		}
		found := false
		for _, skill := range r.FrozenDefinition.Skills {
			if skill.SkillID == selection.SkillID {
				found = true
				for _, permission := range skill.RequiredPermissions {
					permissions[permission] = true
				}
			}
		}
		if !found {
			return nil, nil, fail("invalid_skill", "所选技能不在完整冻结定义内")
		}
	}
	if r.DeliveryMode == "gitlab" {
		wanted = append(wanted, "create_repository", "push_repository", "trigger_ci")
		for _, permission := range []string{"gitlab_create", "gitlab_push", "gitlab_ci"} {
			permissions[permission] = true
		}
	}
	available := map[string]bool{}
	for _, action := range r.FrozenDefinition.Definition.ActionProtocol.AllowedActions {
		available[action] = true
	}
	for _, action := range wanted {
		if !available[action] {
			return nil, nil, fail("action_not_allowed", "冻结定义不支持完成方案所需动作: "+action)
		}
	}
	sort.Strings(wanted)
	grants := make([]string, 0, len(permissions))
	for permission := range permissions {
		if r.DeliveryMode == "local" && strings.HasPrefix(permission, "gitlab_") {
			return nil, nil, fail("remote_permission_forbidden", "本地交付不能取得远程写权限")
		}
		grants = append(grants, permission)
	}
	sort.Strings(grants)
	return wanted, grants, nil
}

// makeCanonicalProposal runs only after deterministic plan validation. Every
// byte in files is already available for preview; no project workspace is
// modified. source_version is decided by the orchestrator before this call.
func (s *Service) makeCanonicalProposal(r *Run, plan contracts.ProjectPlan, files map[string]string, issued time.Time) error {
	if r.Control == nil || r.FrozenDefinition == nil || issued.IsZero() {
		return fail("control_binding_mismatch", "缺少完整受控来源或服务端时间")
	}
	if !hashMatches("ProjectPlan", plan, plan.PlanHash) || plan.ContractVersion != r.FrozenDefinition.ContractVersion {
		return fail("plan_mismatch", "完整计划Schema或内容摘要不符")
	}
	if err := s.verifyFrozenDefinition(*r); err != nil {
		return err
	}
	if err := s.canonicalTargetAllowed(*r, plan); err != nil {
		return err
	}
	actions, permissions, err := proposalScope(*r, plan)
	if err != nil {
		return err
	}
	a := &r.Control.Audit
	grant := a.PermissionSnapshot
	if grant != nil {
		if grant.Revoked || grant.ActorID != localActorID || !hashMatches("PermissionSnapshot", *grant, grant.PermissionHash) {
			return fail("permission_revoked", "当前可信权限已撤销或不可核实；普通方案修订不能重新授予")
		}
		allowed := map[string]bool{}
		for _, permission := range grant.AllowedPermissions {
			allowed[permission] = true
		}
		for _, permission := range permissions {
			if !allowed[permission] {
				return fail("permission_scope_mismatch", "修订需要当前可信权限之外的能力，不能由候选计划扩权")
			}
		}
	}
	permissionHash, err := contracts.HashValue(struct {
		Permissions []string
		Target      contracts.DeliveryTarget
	}{permissions, plan.DeliveryTarget})
	if err != nil {
		return err
	}
	previousScope := ""
	if grant != nil {
		previousScope, err = contracts.HashValue(struct {
			Permissions []string
			Target      contracts.DeliveryTarget
		}{grant.AllowedPermissions, grant.DeliveryTarget})
	}
	if err != nil {
		return err
	}
	if grant == nil || previousScope != permissionHash {
		version := int64(1)
		if grant != nil {
			version = grant.PermissionVersion + 1
		}
		value := contracts.PermissionSnapshot{PermissionVersion: version, ActorID: localActorID, AllowedPermissions: permissions, DeliveryTarget: plan.DeliveryTarget, UpdatedAt: issued.UTC().Format(time.RFC3339Nano)}
		value.PermissionHash, err = contracts.HashObject("PermissionSnapshot", value)
		if err != nil {
			return err
		}
		if err = contracts.Validate("PermissionSnapshot", value); err != nil {
			return err
		}
		grant = &value
		a.PermissionHistory = append(a.PermissionHistory, value)
	}
	paths := make([]string, 0, len(files))
	var size int64
	for path, contents := range files {
		paths = append(paths, path)
		size += int64(len(contents))
	}
	if int64(len(paths)) > r.FrozenDefinition.Definition.BudgetPolicy.MaxFiles || size > r.FrozenDefinition.Definition.BudgetPolicy.MaxArtifactBytes {
		return fail("budget_exceeded", "方案文件或字节超出冻结预算")
	}
	sort.Strings(paths)
	changes := make([]contracts.FileChange, 0, len(paths))
	for _, path := range paths {
		content := files[path]
		if s.containsSecret(content) {
			return fail("secret_output", "候选工件包含服务端凭据，拒绝持久化")
		}
		hash := contracts.BytesHash([]byte(content))
		changes = append(changes, contracts.FileChange{Path: path, Operation: "create", After: &content, AfterSHA256: &hash})
	}
	if plan.VerificationCandidateRef != nil {
		found := false
		for _, candidate := range a.VerificationCandidates {
			if !equalContractValue(plan.VerificationCandidateRef, controlCandidateRef(candidate)) {
				continue
			}
			_, old, observed, e := controlCandidateEvidence(*r, candidate)
			if e != nil {
				return e
			}
			if candidate.SourceVersion >= int64(r.SourceVersion) || !equalContractValue(observed, files) {
				return fail("candidate_binding_mismatch", "修订来源必须递增且使用完整准确候选")
			}
			before := map[string]string{}
			for _, change := range old.Changes {
				if change.After != nil {
					before[change.Path] = *change.After
				}
			}
			for i := range changes {
				if content, exists := before[changes[i].Path]; exists {
					hash := contracts.BytesHash([]byte(content))
					changes[i].Before, changes[i].BeforeSHA256, changes[i].Operation = &content, &hash, "update"
					delete(before, changes[i].Path)
				}
			}
			for name, content := range before {
				hash := contracts.BytesHash([]byte(content))
				changes = append(changes, contracts.FileChange{Path: name, Operation: "delete", Before: &content, BeforeSHA256: &hash})
			}
			sort.Slice(changes, func(i, j int) bool { return changes[i].Path < changes[j].Path })
			found = true
			break
		}
		if !found {
			return fail("candidate_not_available", "修订必须引用已登记的完整真实候选")
		}
	}
	version := int64(1)
	if a.Proposal != nil {
		version = a.Proposal.ProposalVersion + 1
	}
	proposal := contracts.Proposal{
		ContractVersion: plan.ContractVersion, ProposalID: newID("proposal_"), RunID: r.ID, ProposalVersion: version, SourceVersion: int64(r.SourceVersion),
		DefinitionSnapshotHash: r.FrozenDefinition.SnapshotHash, PlanHash: plan.PlanHash, Title: plan.ProjectName + " 项目创建方案", Status: "pending", Plan: plan, Changes: changes,
		Impact:         []string{"只写入该Run批准的文件内容", "执行冻结Go工具和固定后置命令；未批准的依赖差异需重新预览确认", "按明确交付目标生成产物；不含客户数据库真实迁移和生产发布"},
		AllowedActions: actions, RequiredPermissionIDs: permissions, PermissionHash: grant.PermissionHash,
		Checks:   []contracts.PreflightCheck{{CheckID: "plan_schema", Passed: true, Detail: "完整冻结计划已通过结构和确定性参数校验"}, {CheckID: "secret_boundary", Passed: true, Detail: "候选内容未包含执行器凭据"}},
		IssuedAt: issued.UTC().Format(time.RFC3339Nano), ExpiresAt: issued.Add(s.Config.ApprovalTTL).UTC().Format(time.RFC3339Nano),
	}
	proposal.SnapshotHash, err = contracts.HashObject("Proposal", proposal)
	if err != nil {
		return err
	}
	if err = contracts.Validate("Proposal", proposal); err != nil {
		return err
	}
	a.Plan, a.Proposal, a.PermissionSnapshot, a.Approval = &plan, &proposal, grant, nil
	a.ProposalHistory = append(a.ProposalHistory, proposal)
	// A new proposal never advertises the preceding approval's files as its
	// own implementation. Immutable manifests and directories remain history.
	a.Manifest, a.VerificationReport = nil, nil
	r.Control.Schedule.Workspace = contracts.WorkspaceReference{}
	r.Workspace, r.Manifest, r.Artifacts = "", nil, []Artifact{}
	r.RepoProjectID, r.PipelineID, r.CommitSHA = 0, 0, ""
	r.RepoURL, r.PipelineURL = "", ""
	r.AcceptedProposal, r.Approval = "", nil
	r.Proposal = legacyProposal(proposal, r.Definition.Hash, s.target(*r), r.DeliveryMode)
	return nil
}

func legacyProposal(p contracts.Proposal, definitionHash string, target ExternalTarget, delivery string) *Proposal {
	changes := make([]Change, 0, len(p.Changes))
	for _, c := range p.Changes {
		before, after := "", ""
		if c.Before != nil {
			before = *c.Before
		}
		if c.After != nil {
			after = *c.After
		}
		changes = append(changes, Change{Path: c.Path, Before: before, After: after})
	}
	checks := make([]Check, 0, len(p.Checks))
	for _, c := range p.Checks {
		checks = append(checks, Check{Name: c.CheckID, Passed: c.Passed, Detail: c.Detail})
	}
	return &Proposal{ID: p.ProposalID, ProposalVersion: int(p.ProposalVersion), SourceVersion: int(p.SourceVersion), DefinitionHash: definitionHash, SnapshotHash: p.SnapshotHash, Title: p.Title, Status: p.Status, Changes: changes, Impact: append([]string{}, p.Impact...), Checks: checks, IssuedAt: p.IssuedAt, ExpiresAt: p.ExpiresAt, DeliveryMode: delivery, Target: target}
}

func (s *Service) acceptCanonical(r *Run, cmd Command, accepted time.Time) error {
	if cmd.RunID != r.ID || cmd.Action != "accept" || !safeID.MatchString(cmd.ID) {
		return fail("invalid_identity", "确认必须绑定当前Run和稳定命令身份")
	}
	if r.Control == nil || r.Control.Audit.Proposal == nil || r.Control.Audit.Plan == nil || r.Control.Audit.PermissionSnapshot == nil {
		return fail("proposal_missing", "缺少完整当前提案或权限")
	}
	a := &r.Control.Audit
	p, grant := a.Proposal, a.PermissionSnapshot
	if cmd.ProposalID != p.ProposalID || int64(cmd.ProposalVersion) != p.ProposalVersion || int64(cmd.SourceVersion) != p.SourceVersion || cmd.SnapshotHash != p.SnapshotHash {
		return fail("stale_proposal", "确认未绑定当前完整提案的身份、版本和摘要")
	}
	if r.State != "PLAN" || p.Status != "pending" {
		return fail("proposal_closed", "当前提案不再接受新确认")
	}
	if err := s.verifyFrozenDefinition(*r); err != nil {
		return err
	}
	for _, check := range []struct {
		schema string
		value  any
		hash   string
	}{{"ProjectPlan", *a.Plan, a.Plan.PlanHash}, {"Proposal", *p, p.SnapshotHash}, {"PermissionSnapshot", *grant, grant.PermissionHash}} {
		if !hashMatches(check.schema, check.value, check.hash) {
			return fail("approval_binding_mismatch", "当前完整对象Schema或摘要损坏")
		}
	}
	if p.RunID != r.ID || p.SourceVersion != int64(r.SourceVersion) || p.DefinitionSnapshotHash != r.FrozenDefinition.SnapshotHash || p.PlanHash != a.Plan.PlanHash || p.PermissionHash != grant.PermissionHash || grant.Revoked || grant.ActorID != localActorID {
		return fail("approval_binding_mismatch", "当前来源或权限不能签发批准")
	}
	if !equalContractValue(p.Plan, *a.Plan) || !equalContractValue(grant.DeliveryTarget, a.Plan.DeliveryTarget) {
		return fail("approval_binding_mismatch", "提案计划和当前准确授权目标不符")
	}
	if err := s.canonicalTargetAllowed(*r, *a.Plan); err != nil {
		return err
	}
	actions, permissions, scopeErr := proposalScope(*r, *a.Plan)
	if scopeErr != nil {
		return scopeErr
	}
	if !equalContractValue(actions, p.AllowedActions) || !equalContractValue(permissions, p.RequiredPermissionIDs) {
		return fail("approval_scope_mismatch", "确认动作或必需权限与确定性方案不符")
	}
	allowed := map[string]bool{}
	for _, permission := range grant.AllowedPermissions {
		allowed[permission] = true
	}
	for _, permission := range permissions {
		if !allowed[permission] {
			return fail("permission_scope_mismatch", "当前可信权限不足以批准本方案")
		}
	}
	issued, e1 := time.Parse(time.RFC3339Nano, p.IssuedAt)
	expires, e2 := time.Parse(time.RFC3339Nano, p.ExpiresAt)
	if e1 != nil || e2 != nil || accepted.Before(issued) || !accepted.Before(expires) {
		return fail("proposal_expired", "服务端签发的提案有效期已过或时间不符")
	}
	for _, check := range p.Checks {
		if !check.Passed {
			return fail("proposal_blocked", "当前方案包含未通过的前置校验")
		}
	}
	approval := contracts.Approval{
		ContractVersion: p.ContractVersion, ApprovalID: newID("approval_"), RunID: r.ID, ActorID: localActorID,
		ProposalID: p.ProposalID, ProposalVersion: p.ProposalVersion, SourceVersion: p.SourceVersion, ProposalSnapshotHash: p.SnapshotHash,
		DefinitionSnapshotHash: p.DefinitionSnapshotHash, PlanHash: p.PlanHash, PermissionHash: p.PermissionHash, AllowedActions: append([]string{}, p.AllowedActions...),
		DeliveryTarget: a.Plan.DeliveryTarget, IssuedAt: p.IssuedAt, ExpiresAt: p.ExpiresAt, AcceptedAt: accepted.UTC().Format(time.RFC3339Nano), Status: "accepted",
	}
	var err error
	approval.ApprovalHash, err = contracts.HashObject("Approval", approval)
	if err != nil {
		return err
	}
	if err = contracts.Validate("Approval", approval); err != nil {
		return fmt.Errorf("approval schema: %w", err)
	}
	a.Approval = &approval
	a.ApprovalHistory = append(a.ApprovalHistory, approval)
	p.Status = "accepted"
	r.Proposal.Status = "accepted"
	r.AcceptedProposal, r.State, r.Error = p.ProposalID, "IMPLEMENT", ""
	r.Approval = &ApprovalRecord{UserID: localActorID, ProposalID: p.ProposalID, ProposalVersion: int(p.ProposalVersion), SourceVersion: int(p.SourceVersion), SnapshotHash: p.SnapshotHash, DefinitionHash: r.Definition.Hash, IssuedAt: p.IssuedAt, ExpiresAt: p.ExpiresAt, AcceptedAt: approval.AcceptedAt, Actions: append([]string{}, r.Definition.AllowedActions...)}
	return nil
}
