package agent

// These deterministic checks belong to the product's completion gate. They
// never import or invoke the external validation scenario runner.
import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

const manifestName = "AGENT-MANIFEST.json"

func (s *Service) verifyApprovedSources(r Run) error {
	if r.Proposal == nil {
		return fail("invalid_proposal", "没有批准文件快照")
	}
	for _, change := range r.Proposal.Changes {
		path := filepath.Join(r.Workspace, change.Path)
		if e := safeWorkspacePath(s.Config.DataDir, path); e != nil {
			return e
		}
		actual, e := os.ReadFile(path)
		if e != nil {
			return e
		}
		if change.Path == "go.mod" {
			tidied := false
			for _, check := range r.RequiredChecks {
				if check.Name == "go mod tidy" && check.Passed && check.SourceVersion == r.SourceVersion {
					tidied = true
				}
			}
			if !tidied && !bytes.Equal(actual, []byte(change.After)) {
				return fail("artifact_source_mismatch", "tidy前go.mod必须等于批准内容")
			}
			if tidied {
				if e = s.verifyModuleMetadata(r, []byte(change.After), actual); e != nil {
					return e
				}
			}
			continue
		}
		if !bytes.Equal(actual, []byte(change.After)) {
			return fail("artifact_source_mismatch", "文件与批准快照不同: "+change.Path)
		}
	}
	return nil
}
func (s *Service) registerManifest(r Run) error {
	var readErr error
	r, readErr = s.Store.Get(r.ID)
	if readErr != nil {
		return readErr
	}
	if e := s.verifyApprovedSources(r); e != nil {
		return e
	}
	files, e := ListArtifacts(r.Workspace)
	if e != nil {
		return e
	}
	allowed := map[string]bool{"go.sum": true, manifestName: true}
	for _, c := range r.Proposal.Changes {
		allowed[c.Path] = true
	}
	clean := []Artifact{}
	var total int64
	for _, f := range files {
		if !allowed[f.Path] {
			return fail("unexpected_artifact", "未获准工件: "+f.Path)
		}
		if f.Path != manifestName {
			clean = append(clean, f)
			total += f.Size
		}
	}
	if len(clean) > r.Definition.Budget.MaxFiles || total > r.Definition.Budget.MaxArtifactBytes {
		return fail("budget_exceeded", "实际工件数量或大小超出冻结预算")
	}
	m := ArtifactManifest{RunID: r.ID, SourceVersion: r.SourceVersion, ProposalID: r.Proposal.ID, ProposalVersion: r.Proposal.ProposalVersion, SnapshotHash: r.Proposal.SnapshotHash, DefinitionHash: r.Definition.Hash, PlanHash: digest(r.Plan), Files: clean, CreatedAt: now()}
	m.Hash = manifestDigest(m)
	b, e := json.MarshalIndent(m, "", "  ")
	if e != nil {
		return e
	}
	if e = writeAtomic(filepath.Join(r.Workspace, manifestName), b, 0600); e != nil {
		return e
	}
	_, e = s.Store.Update(r.ID, func(run *Run) error {
		run.Manifest = &m
		recordChecks(run, []Check{{Name: "approved_sources", Passed: true, Detail: "不可变源代码与批准快照一致；依赖元数据由获准工具整理"}, {Name: "artifact_manifest", Passed: true, Detail: m.Hash}})
		return nil
	})
	return e
}
func manifestDigest(m ArtifactManifest) string { m.Hash = ""; return digest(m) }
func (s *Service) verifyManifest(r Run) error {
	m := r.Manifest
	if m == nil || r.Proposal == nil || m.RunID != r.ID || m.SourceVersion != r.SourceVersion || m.SnapshotHash != r.Proposal.SnapshotHash || m.DefinitionHash != r.Definition.Hash || m.PlanHash != digest(r.Plan) || m.Hash != manifestDigest(*m) {
		return fail("manifest_mismatch", "工件manifest缺失或来源不匹配")
	}
	for _, f := range m.Files {
		path := filepath.Join(r.Workspace, f.Path)
		if e := safeWorkspacePath(s.Config.DataDir, path); e != nil {
			return e
		}
		b, e := os.ReadFile(path)
		if e != nil {
			return e
		}
		if int64(len(b)) != f.Size || bytesDigest(b) != f.SHA256 {
			return fail("manifest_mismatch", "工件内容已变化: "+f.Path)
		}
	}
	actual, e := ListArtifacts(r.Workspace)
	if e != nil {
		return e
	}
	if len(actual) != len(m.Files)+1 {
		return fail("manifest_mismatch", "工件集合与manifest不一致")
	}
	data, e := os.ReadFile(filepath.Join(r.Workspace, manifestName))
	if e != nil {
		return e
	}
	var saved ArtifactManifest
	if json.Unmarshal(data, &saved) != nil || saved.Hash != m.Hash || manifestDigest(saved) != m.Hash {
		return fail("manifest_mismatch", "落盘manifest与权威记录不同")
	}
	return s.verifyApprovedSources(r)
}
func bytesDigest(b []byte) string { return fmt.Sprintf("%x", sha256.Sum256(b)) }

type goModuleMetadata struct {
	Module    struct{ Path string }
	Go        string
	Toolchain string
	Require   []struct {
		Path, Version string
		Indirect      bool
	}
	Replace []json.RawMessage
	Exclude []json.RawMessage
}

func (s *Service) parseModule(r Run, data []byte) (goModuleMetadata, error) {
	var out goModuleMetadata
	dir := filepath.Join(s.Config.DataDir, "module-checks")
	if e := os.MkdirAll(dir, 0700); e != nil {
		return out, e
	}
	f, e := os.CreateTemp(dir, "approved-*.mod")
	if e != nil {
		return out, e
	}
	path := f.Name()
	defer os.Remove(path)
	if _, e = f.Write(data); e != nil {
		f.Close()
		return out, e
	}
	if e = f.Close(); e != nil {
		return out, e
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", "mod", "edit", "-json", path)
	cmd.Env = append(isolatedGoEnv(), "GOTOOLCHAIN="+r.Definition.Tools["go"])
	b, e := cmd.Output()
	if e != nil {
		return out, fail("artifact_source_mismatch", "go.mod无法按固定工具解析")
	}
	if json.Unmarshal(b, &out) != nil {
		return out, fail("artifact_source_mismatch", "go.mod结构解析失败")
	}
	return out, nil
}
func (s *Service) verifyModuleMetadata(r Run, approved, actual []byte) error {
	a, e := s.parseModule(r, approved)
	if e != nil {
		return e
	}
	b, e := s.parseModule(r, actual)
	if e != nil {
		return e
	}
	if a.Module.Path != b.Module.Path || a.Go != b.Go || b.Toolchain != "" || len(b.Replace) > 0 || len(b.Exclude) > 0 {
		return fail("artifact_source_mismatch", "tidy后module/go/replace/exclude/toolchain不允许改变")
	}
	versions := map[string]string{}
	for _, required := range a.Require {
		versions[required.Path] = required.Version
	}
	found := map[string]string{}
	for _, required := range b.Require {
		found[required.Path] = required.Version
		if _, exists := versions[required.Path]; !exists && !required.Indirect {
			return fail("artifact_source_mismatch", "tidy不能新增未批准直接依赖")
		}
	}
	for path, version := range versions {
		if found[path] != version {
			return fail("artifact_source_mismatch", "批准直接依赖版本被修改: "+path)
		}
	}
	return nil
}
