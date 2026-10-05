package agent

import (
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// Bind all conditional branches and embedded template source, including
// branches absent from the representative rendered bundle below.
//
//go:embed templates.go
var templateSourceSnapshot string

func digest(value any) string {
	b, _ := json.Marshal(value)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
func sourceChanged(r *Run) { r.SourceVersion++; r.Version = r.SourceVersion }
func definitionSnapshot() (DefinitionSnapshot, error) {
	d := DefinitionSnapshot{ID: "project-creation-agent", Version: "2.0.0", TemplateVersion: "2.0.0", ActionSchemaVersion: "2.0.0", SkillVersions: map[string]string{"add_database": "2.0.0", "add_auth": "2.0.0", "add_logging": "2.0.0"}, Tools: map[string]string{"go": runtime.Version(), "temporal_sdk": "v1.38.0"}, AllowedActions: []string{"scaffold", "skills", "verify", "repo", "ci", "deliver"}, Budget: BudgetPolicy{MaxCommands: 256, MaxModelAttempts: 32, MaxFiles: 128, MaxArtifactBytes: 2 << 20, MaxStepAttempts: 3}}
	bundle := map[string]map[string]string{}
	for _, db := range []string{"postgres", "mysql", "sqlite"} {
		for _, orm := range []string{"gorm", "sqlx"} {
			for _, auth := range []string{"jwt", "session"} {
				for _, output := range []string{"console", "file"} {
					p := defaultPlan()
					p.ProjectName = "template-baseline"
					p.Skills = []string{"add_database", "add_auth", "add_logging"}
					p.Database = db
					p.ORM = orm
					p.Auth = auth
					p.LogOutput = output
					files, e := RenderFiles(p)
					if e != nil {
						return d, e
					}
					bundle[db+"/"+orm+"/"+auth+"/"+output] = files
				}
			}
		}
	}
	d.TemplateHash = digest(struct {
		Rendered                      map[string]map[string]string
		Source, Logging, LoggingTests string
	}{bundle, templateSourceSnapshot, loggingSource, loggingTests})
	d.Hash = definitionDigest(d)
	return d, nil
}
func definitionDigest(d DefinitionSnapshot) string { d.Hash = ""; return digest(d) }
func proposalDigest(r Run, p Proposal) string {
	return digest(struct {
		Plan                           Plan
		Definition                     DefinitionSnapshot
		SourceVersion, ProposalVersion int
		DeliveryMode                   string
		Identity, IssuedAt, ExpiresAt  string
		Target                         ExternalTarget
		Changes                        []Change
	}{r.Plan, r.Definition, p.SourceVersion, p.ProposalVersion, p.DeliveryMode, p.ID, p.IssuedAt, p.ExpiresAt, p.Target, p.Changes})
}
func (s *Service) target(r Run) ExternalTarget {
	if r.DeliveryMode != "gitlab" {
		return ExternalTarget{}
	}
	return ExternalTarget{GitLabURL: strings.TrimRight(s.Config.GitLabURL, "/"), NamespaceID: s.Config.GitLabNamespace, ProjectPath: r.Plan.ProjectName, Ref: "main"}
}
func (s *Service) verifyApproval(r Run, first bool) error {
	if err := s.verifyFrozenDefinition(r); err != nil {
		return err
	}
	p := r.Proposal
	a := r.Approval
	if p == nil || a == nil || p.Status != "accepted" || r.AcceptedProposal != p.ID || a.ProposalID != p.ID {
		return fail("unapproved_execution", "执行需要当前具体方案的批准")
	}
	if a.IssuedAt != p.IssuedAt || a.ExpiresAt != p.ExpiresAt || digest(a.Actions) != digest(r.Definition.AllowedActions) {
		return fail("snapshot_mismatch", "授权有效期或动作范围已改变")
	}
	if a.SourceVersion != r.SourceVersion || p.SourceVersion != r.SourceVersion || a.ProposalVersion != p.ProposalVersion {
		return fail("stale_proposal", "业务来源或提案版本已经改变，必须重新预览确认")
	}
	if definitionDigest(r.Definition) != r.Definition.Hash || a.DefinitionHash != r.Definition.Hash {
		return fail("definition_mismatch", "运行定义快照不一致")
	}
	if a.SnapshotHash != p.SnapshotHash || p.SnapshotHash != proposalDigest(r, *p) {
		return fail("snapshot_mismatch", "批准文件或参数快照已变化")
	}
	if p.Target != s.target(r) {
		return fail("target_changed", "配置中的外部目标与批准不一致，必须重新预览")
	}
	if first || a.StartedAt == "" {
		expires, e := time.Parse(time.RFC3339Nano, a.ExpiresAt)
		if e != nil || !time.Now().UTC().Before(expires) {
			return fail("proposal_expired", "方案确认已过期，请重新生成预览")
		}
	}
	if r.Usage.Commands > r.Definition.Budget.MaxCommands || r.Usage.ModelAttempts > r.Definition.Budget.MaxModelAttempts {
		return fail("budget_exceeded", "Run已超冻结预算")
	}
	if e := checkProposalBudget(r); e != nil {
		return e
	}
	return ValidatePlan(r.Plan)
}
func checkProposalBudget(r Run) error {
	if r.Proposal == nil {
		return nil
	}
	var size int64
	for _, c := range r.Proposal.Changes {
		size += int64(len(c.After))
	}
	if len(r.Proposal.Changes) > r.Definition.Budget.MaxFiles || size > r.Definition.Budget.MaxArtifactBytes {
		return fail("budget_exceeded", "批准文件数量或大小超预算")
	}
	return nil
}
func commandBinding(r Run, c Command) error {
	p := r.Proposal
	if p == nil || p.ID != c.ProposalID || p.SourceVersion != c.SourceVersion || r.SourceVersion != c.SourceVersion || p.ProposalVersion != c.ProposalVersion {
		return fail("stale_proposal", "预览来源或提案版本已改变，请刷新当前提案")
	}
	if c.SnapshotHash == "" || c.SnapshotHash != p.SnapshotHash || p.SnapshotHash != proposalDigest(r, *p) {
		return fail("snapshot_mismatch", "确认内容摘要与当前不可变提案不同")
	}
	return nil
}
func (s *Service) containsSecret(text string) bool {
	for _, secret := range []string{s.Config.GitLabToken, s.Config.AIKey} {
		if secret != "" && strings.Contains(text, secret) {
			return true
		}
	}
	return false
}
func safeWorkspacePath(root, path string) error {
	if !within(root, path) {
		return fail("invalid_path", "路径越出工作区")
	}
	rel, e := filepath.Rel(root, path)
	if e != nil {
		return e
	}
	current := root
	for _, part := range append([]string{""}, strings.Split(rel, string(filepath.Separator))...) {
		current = filepath.Join(current, part)
		info, e := os.Lstat(current)
		if os.IsNotExist(e) {
			continue
		}
		if e != nil {
			return e
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fail("invalid_path", "工作区路径包含未批准的符号链接")
		}
	}
	return nil
}
func requiredChecks(mode string) []Check {
	names := []string{"approved_sources", "artifact_manifest", "go mod tidy", "go test ./...", "go build ./...", "file:go.mod", "file:.gitignore", "file:.gitlab-ci.yml", "file:README.md", "secret_boundary", "zip_integrity"}
	if mode == "gitlab" {
		names = append(names, "remote_repository", "remote_commit", "gitlab_ci")
	}
	out := make([]Check, 0, len(names))
	for _, name := range names {
		out = append(out, Check{Name: name, Status: "not_ready", Detail: "等待真实检查证据"})
	}
	return out
}

// requiredChecksFor projects the bound completion policy into the existing
// check runner. Legacy recovery keeps its historical fixed checks. Unsupported
// or weakened frozen policies remain visible and can never authorize DONE.
func requiredChecksFor(r Run) []Check {
	if r.FrozenDefinition == nil {
		return requiredChecks(r.DeliveryMode)
	}
	bindings := map[string]struct {
		name, kind string
		evidence   []string
	}{
		"check.approved_sources":  {"approved_sources", "source", []string{"source_hash"}},
		"check.artifact_manifest": {"artifact_manifest", "manifest", []string{"manifest_hash"}},
		"check.go_mod_tidy":       {"go mod tidy", "tool", []string{"tool_exit"}},
		"check.go_test":           {"go test ./...", "tool", []string{"tool_exit"}},
		"check.go_build":          {"go build ./...", "tool", []string{"tool_exit"}},
		"check.go_mod_file":       {"file:go.mod", "file", []string{"file_hash"}},
		"check.gitignore_file":    {"file:.gitignore", "file", []string{"file_hash"}},
		"check.ci_config_file":    {"file:.gitlab-ci.yml", "file", []string{"file_hash"}},
		"check.readme_file":       {"file:README.md", "file", []string{"file_hash"}},
		"check.secret_boundary":   {"secret_boundary", "secret_boundary", []string{"secret_scan"}},
		"check.zip_integrity":     {"zip_integrity", "package", []string{"zip_integrity"}},
		"check.remote_repository": {"remote_repository", "remote_repository", []string{"remote_identity"}},
		"check.remote_commit":     {"remote_commit", "remote_commit", []string{"commit_tree"}},
		"check.gitlab_ci":         {"gitlab_ci", "ci", []string{"pipeline_jobs", "coverage_report", "build_artifact"}},
	}
	out := []Check{}
	seen := map[string]bool{}
	for _, definition := range r.FrozenDefinition.Definition.CompletionPolicy.RequiredChecks {
		applicable := false
		for _, mode := range definition.DeliveryModes {
			if mode == r.DeliveryMode {
				applicable = true
			}
		}
		if !applicable {
			continue
		}
		binding, supported := bindings[definition.CheckID]
		name := binding.name
		detail := "等待冻结完成策略要求的真实检查证据"
		if !supported {
			name = "unsupported_check:" + definition.CheckID
			detail = "冻结定义要求尚无受控执行器的检查，禁止完成"
		} else {
			// Evidence requirements are a set. A familiar check ID must not
			// silently substitute another kind or discard additional evidence.
			evidence := map[string]bool{}
			for _, kind := range definition.RequiredEvidenceTypes {
				evidence[kind] = true
			}
			matches := definition.Kind == binding.kind && len(evidence) == len(binding.evidence) && len(evidence) == len(definition.RequiredEvidenceTypes)
			for _, kind := range binding.evidence {
				matches = matches && evidence[kind]
			}
			if !matches {
				name = "unsupported_check_policy:" + definition.CheckID
				detail = "冻结检查类型或证据要求与受控执行器不一致，禁止完成"
			}
		}
		if seen[name] {
			name = "duplicate_check:" + definition.CheckID
			detail = "冻结完成策略含重复检查身份，禁止完成"
		}
		seen[name] = true
		out = append(out, Check{Name: name, Status: "not_ready", Detail: detail})
	}
	for _, baseline := range requiredChecks(r.DeliveryMode) {
		if !seen[baseline.Name] {
			out = append(out, Check{Name: "policy_missing:" + baseline.Name, Status: "not_ready", Detail: "冻结完成策略缺少本版本必需安全检查，禁止完成"})
		}
	}
	return out
}

func recordChecks(r *Run, checks []Check) {
	for _, c := range checks {
		c.SourceVersion = r.SourceVersion
		if c.Passed {
			c.Status = "passed"
		} else {
			c.Status = "failed"
		}
		for i := range r.RequiredChecks {
			if r.RequiredChecks[i].Name == c.Name {
				r.RequiredChecks[i] = c
			}
		}
	}
}
func requireCompletion(r Run) error {
	expectedChecks := requiredChecksFor(r)
	if len(r.RequiredChecks) != len(expectedChecks) {
		return fail("verification_incomplete", "必需检查集合不完整")
	}
	actual := map[string]Check{}
	for _, c := range r.RequiredChecks {
		if _, duplicate := actual[c.Name]; duplicate {
			return fail("verification_incomplete", "必需检查身份重复")
		}
		actual[c.Name] = c
	}
	for _, expected := range expectedChecks {
		if strings.HasPrefix(expected.Name, "unsupported_check:") || strings.HasPrefix(expected.Name, "unsupported_check_policy:") || strings.HasPrefix(expected.Name, "duplicate_check:") || strings.HasPrefix(expected.Name, "policy_missing:") {
			return fail("verification_incomplete", expected.Detail+": "+expected.Name)
		}
		c, ok := actual[expected.Name]
		statusOK := c.Status == "passed" || r.FrozenDefinition == nil && c.Status == ""
		if !ok || !c.Passed || !statusOK || c.SourceVersion != r.SourceVersion {
			return fail("verification_incomplete", fmt.Sprintf("缺少同源通过证据: %s", expected.Name))
		}
	}
	return nil
}

func operationID(r Run, kind string) string {
	return "op_" + digest([]any{r.ID, kind, r.SourceVersion, 1})[:32]
}
