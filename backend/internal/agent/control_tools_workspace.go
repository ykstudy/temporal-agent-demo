package agent

import (
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unicode/utf8"

	"agentstudio/internal/contracts"
)

type controlToolPaths struct{ runRoot, approved, candidate, cache, home, temporary string }

func (s *Service) controlToolPaths(r Run, request contracts.ExecutionRequest, attempt string) (controlToolPaths, error) {
	var out controlToolPaths
	data, err := filepath.EvalSymlinks(s.Config.DataDir)
	if err != nil {
		return out, err
	}
	data, err = filepath.Abs(data)
	if err != nil {
		return out, err
	}
	root := filepath.Join(data, "workspaces")
	out.runRoot = filepath.Join(root, r.ID)
	if request.Workspace.RunID != r.ID || contracts.Validate("WorkspaceReference", request.Workspace) != nil {
		return out, fail("invalid_path", "工作区引用不属于当前Run")
	}
	resolved := filepath.Join(root, filepath.FromSlash(request.Workspace.StorageKey))
	actual, err := filepath.Abs(r.Workspace)
	if err != nil {
		return out, err
	}
	actual, err = filepath.EvalSymlinks(actual)
	if err != nil {
		return out, err
	}
	if !within(out.runRoot, resolved) || filepath.Clean(resolved) != actual || resolved == out.runRoot {
		return out, fail("invalid_path", "工具工作区必须准确绑定Run内独立项目目录")
	}
	if err = safeWorkspacePath(data, resolved); err != nil {
		return out, err
	}
	out.approved = resolved
	identity, _ := contracts.HashValue([]string{request.RequestHash, attempt})
	control := filepath.Join(out.runRoot, ".control-tools")
	out.candidate = filepath.Join(control, identity[:40], "workspace")
	out.cache = filepath.Join(control, "cache")
	out.home = filepath.Join(control, "home")
	out.temporary = filepath.Join(control, "tmp")
	for _, path := range []string{out.candidate, out.cache, out.home, out.temporary} {
		if within(out.approved, path) || within(path, out.approved) {
			return out, fail("invalid_path", "候选目录与批准目录不能重叠")
		}
		if err = safeWorkspacePath(data, path); err != nil {
			return out, err
		}
	}
	return out, nil
}

func (p controlToolPaths) prepare() error {
	for _, path := range []string{filepath.Dir(p.candidate), p.cache, filepath.Join(p.cache, "build"), filepath.Join(p.cache, "mod"), p.home, p.temporary} {
		if err := safeWorkspacePath(p.runRoot, path); err != nil {
			return err
		}
		if err := os.MkdirAll(path, 0700); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) controlToolFiles(r Run, root string) (map[string]string, error) {
	return s.controlSourceFiles(r, root, false)
}

// Only the fixed Git executor may exclude its own private root .git directory.
// Approved sources and Go candidates continue to reject/inspect every entry.
func (s *Service) controlSourceFiles(r Run, root string, gitMetadata bool) (map[string]string, error) {
	files := map[string]string{}
	budget := r.FrozenDefinition.Definition.BudgetPolicy
	var total int64
	info, err := os.Lstat(root)
	if err != nil {
		return files, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return files, fail("invalid_path", "工作区根不能是符号链接或普通文件")
	}
	anchored, err := os.OpenRoot(root)
	if err != nil {
		return files, err
	}
	defer anchored.Close()
	err = fs.WalkDir(anchored.FS(), ".", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fail("invalid_path", "工具工作区禁止符号链接")
		}
		if gitMetadata && path == ".git" && entry.IsDir() {
			return fs.SkipDir
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fail("invalid_path", "工具工作区禁止特殊文件")
		}
		if int64(len(files)+1) > budget.MaxFiles || info.Size() > budget.MaxArtifactBytes-total {
			return fail("artifact_budget_exceeded", "候选文件数量或字节超过冻结预算")
		}
		rel := path
		if err = contracts.Validate("agent.schema.json#/$defs/RelativePath", rel); err != nil {
			return err
		}
		f, err := anchored.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
		if err != nil {
			return err
		}
		opened, statErr := f.Stat()
		if statErr != nil || !opened.Mode().IsRegular() {
			f.Close()
			return fail("invalid_path", "工作区文件读取时类型发生变化")
		}
		// LimitReader also bounds a concurrently growing file. The executor's
		// run lock and sandbox keep its own child away from the approved tree.
		data, err := io.ReadAll(io.LimitReader(f, budget.MaxArtifactBytes-total+1))
		closeErr := f.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return closeErr
		}
		if int64(len(data)) > budget.MaxArtifactBytes-total {
			return fail("artifact_budget_exceeded", "候选内容超过冻结预算")
		}
		total += int64(len(data))
		if !utf8.Valid(data) {
			return fail("invalid_candidate_content", "候选源文件必须是可重新预览的UTF-8内容")
		}
		for _, secret := range []string{s.Config.GitLabToken, s.Config.AIKey} {
			if secret != "" && strings.Contains(string(data), secret) {
				return fail("secret_output", "候选内容包含执行器凭据")
			}
		}
		files[rel] = string(data)
		return nil
	})
	return files, err
}

func controlMatchApproved(r Run, files map[string]string) error {
	m := r.Control.Audit.Manifest
	p := r.Control.Audit.Proposal
	if m == nil || p == nil || len(m.Files) != len(files) {
		return fail("artifact_source_mismatch", "真实文件集合与批准manifest不符")
	}
	for _, entry := range m.Files {
		value, ok := files[entry.Path]
		if !ok || int64(len(value)) != entry.Size || contracts.BytesHash([]byte(value)) != entry.SHA256 {
			return fail("artifact_source_mismatch", "真实文件与已登记manifest摘要不符")
		}
	}
	approved := map[string]string{}
	for _, change := range p.Changes {
		if change.After != nil {
			approved[change.Path] = *change.After
		}
	}
	if !equalContractValue(approved, files) {
		return fail("artifact_source_mismatch", "manifest未覆盖准确批准文件原文")
	}
	return nil
}

func controlCopyFiles(root string, files map[string]string) error {
	if _, err := os.Lstat(root); !os.IsNotExist(err) {
		return fail("candidate_exists", "候选目标必须不存在")
	}
	stage, err := os.MkdirTemp(filepath.Dir(root), ".candidate-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	for name, content := range files {
		path := filepath.Join(stage, filepath.FromSlash(name))
		if err := safeWorkspacePath(stage, path); err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			return err
		}
		f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			return err
		}
		_, err = io.WriteString(f, content)
		if err == nil {
			err = f.Sync()
		}
		closeErr := f.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return closeErr
		}
	}
	dir, err := os.Open(stage)
	if err != nil {
		return err
	}
	err = dir.Sync()
	dir.Close()
	if err != nil {
		return err
	}
	if err = os.Rename(stage, root); err != nil {
		return err
	}
	parent, err := os.Open(filepath.Dir(root))
	if err != nil {
		return err
	}
	defer parent.Close()
	return parent.Sync()
}
