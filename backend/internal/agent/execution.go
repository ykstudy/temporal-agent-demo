package agent

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"go.temporal.io/sdk/activity"
)

var executionSteps = []string{"scaffold", "skills", "verify", "repo", "ci", "deliver"}

type StepInput struct {
	RunID string `json:"run_id"`
	Step  string `json:"step"`
}
type StepResult struct {
	State         string `json:"state"`
	Continue      bool   `json:"continue"`
	SourceVersion int    `json:"source_version"`
	StateSequence int    `json:"state_sequence"`
}

func (s *Service) StartLocal(id string) {
	s.mu.Lock()
	if _, ok := s.active[id]; ok {
		s.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.active[id] = cancel
	s.localWG.Add(1)
	s.mu.Unlock()
	go func() {
		defer s.localWG.Done()
		defer func() { s.mu.Lock(); delete(s.active, id); s.mu.Unlock(); cancel() }()
		for _, step := range executionSteps {
			var result StepResult
			var e error
			for attempt := 0; attempt < 3; attempt++ {
				result, e = s.ExecuteStep(ctx, StepInput{RunID: id, Step: step})
				var infrastructure *InfrastructureError
				if e == nil || !errors.As(e, &infrastructure) || attempt == 2 {
					break
				}
				timer := time.NewTimer(time.Duration(1<<attempt) * time.Second)
				select {
				case <-ctx.Done():
					timer.Stop()
					e = ctx.Err()
					attempt = 3
				case <-timer.C:
				}
			}
			if e != nil {
				_ = s.RecordExecutionFailure(context.Background(), id, e.Error())
				return
			}
			if !result.Continue {
				return
			}
		}
	}()
}
func heartbeat(ctx context.Context, detail string) {
	if activity.IsActivity(ctx) {
		activity.RecordHeartbeat(ctx, detail)
	}
}
func (s *Service) executeStep(ctx context.Context, input StepInput) (StepResult, error) {
	if activity.IsActivity(ctx) {
		done := make(chan struct{})
		defer close(done)
		go func() {
			ticker := time.NewTicker(15 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-done:
					return
				case <-ctx.Done():
					return
				case <-ticker.C:
					activity.RecordHeartbeat(ctx, input.Step)
				}
			}
		}()
	}
	r, e := s.Store.Get(input.RunID)
	if e != nil {
		return StepResult{}, e
	}
	if r.ExecutionMode != s.Config.Mode {
		return StepResult{}, fail("run_mode_mismatch", "当前服务不可执行其他模式Run")
	}
	if r.Control != nil {
		return s.ExecuteControlStep(ctx, input)
	}
	if r.State == "PAUSED" || r.State == "BLOCKED" || terminal(r.State) {
		return StepResult{SourceVersion: r.SourceVersion, StateSequence: r.StateSequence, State: r.State}, nil
	}
	index := -1
	for i, step := range executionSteps {
		if input.Step == step {
			index = i
		}
	}
	if index < 0 {
		return StepResult{}, fail("invalid_step", "unknown execution step")
	}
	allowed := false
	for _, step := range r.Definition.AllowedActions {
		if step == input.Step {
			allowed = true
		}
	}
	if !allowed {
		return StepResult{}, fail("action_not_allowed", "步骤不在冻结定义的允许动作范围")
	}
	if e = s.verifyApproval(r, false); e != nil {
		var api *APIError
		if errors.As(e, &api) && api.Code == "target_changed" && r.Approval != nil && r.Approval.StartedAt != "" {
			return s.block(r.ID, input.Step, "执行期间外部目标配置改变；恢复原批准目标后核实并恢复，或取消后重做。保留已应用事实。")
		}
		if errors.As(e, &api) && (api.Code == "proposal_expired" || api.Code == "target_changed") {
			updated, saveErr := s.Store.Update(r.ID, func(run *Run) error {
				run.Proposal.Status = "expired"
				sourceChanged(run)
				run.AcceptedProposal = ""
				run.Approval = nil
				run.State = "PLAN"
				run.Error = api.Message
				run.ActiveStep = ""
				run.RequiredChecks = requiredChecksFor(*run)
				if err := s.makeProposal(run); err != nil {
					return err
				}
				history(run, "approval_recheck", api.Message)
				addMessage(run, "assistant", "执行尚未开始或目标已改变，请确认新的不可变方案。", "proposal", run.Proposal.ID)
				return nil
			})
			return StepResult{SourceVersion: updated.SourceVersion, StateSequence: updated.StateSequence, State: updated.State}, saveErr
		}
		return s.failStep(r.ID, input.Step, e)
	}
	for _, step := range r.Steps {
		if step.ID == input.Step && (step.State == "completed" || step.State == "skipped") {
			if index >= 2 {
				if e = s.verifyManifest(r); e != nil {
					return s.failStep(r.ID, input.Step, e)
				}
			}
			return StepResult{SourceVersion: r.SourceVersion, StateSequence: r.StateSequence, State: r.State, Continue: true}, nil
		}
	}
	for _, prior := range executionSteps[:index] {
		for _, step := range r.Steps {
			if step.ID == prior && step.State != "completed" && step.State != "skipped" {
				return s.failStep(r.ID, input.Step, fail("invalid_step_order", "前置步骤尚未完成: "+prior))
			}
		}
	}
	if r.Workspace == "" {
		r, e = s.Store.Update(r.ID, func(run *Run) error {
			run.Workspace = filepath.Join(s.Config.DataDir, "workspaces", run.ID, run.Plan.ProjectName)
			return nil
		})
		if e != nil {
			return StepResult{}, e
		}
	}
	if !within(filepath.Join(s.Config.DataDir, "workspaces", r.ID), r.Workspace) {
		return s.failStep(r.ID, input.Step, fail("invalid_workspace", "工作区越界"))
	}
	if e = safeWorkspacePath(s.Config.DataDir, r.Workspace); e != nil {
		return s.failStep(r.ID, input.Step, e)
	}
	r, e = s.Store.Update(r.ID, func(run *Run) error {
		if run.State == "PAUSED" || run.State == "BLOCKED" || terminal(run.State) {
			return fail("execution_stopped", "在安全边界停止执行")
		}
		if err := s.verifyApproval(*run, false); err != nil {
			return err
		}
		if run.Usage.StepAttempts == nil {
			run.Usage.StepAttempts = map[string]int{}
		}
		readOnlyRecovery := false
		if input.Step == "repo" || input.Step == "ci" {
			for _, op := range run.Operations {
				if op.SourceVersion == run.SourceVersion && (op.Status == "sent" || op.Status == "unknown") {
					readOnlyRecovery = true
				}
			}
		}
		if !readOnlyRecovery && run.Usage.StepAttempts[input.Step] >= run.Definition.Budget.MaxStepAttempts {
			return fail("budget_exceeded", "该阶段累计尝试预算已耗尽")
		}
		if !readOnlyRecovery {
			run.Usage.StepAttempts[input.Step]++
		}
		if run.Approval.StartedAt == "" {
			run.Approval.StartedAt = now()
			history(run, "application_started", "首次应用重新校验来源、内容hash与有效期")
		}
		run.ActiveStep = input.Step
		setStep(run, input.Step, "running", "实际执行中")
		if input.Step == "verify" {
			run.State = "VERIFY"
		} else if index >= 3 {
			run.State = "DELIVER"
		} else {
			run.State = "IMPLEMENT"
		}
		history(run, "step_started", input.Step)
		return nil
	})
	if e != nil {
		var api *APIError
		if errors.As(e, &api) && api.Code == "execution_stopped" {
			current, readErr := s.Store.Get(input.RunID)
			return StepResult{State: current.State, SourceVersion: current.SourceVersion, StateSequence: current.StateSequence}, readErr
		}
		return StepResult{}, e
	}
	heartbeat(ctx, input.Step)
	detail := ""
	switch input.Step {
	case "scaffold", "skills":
		if len(r.Proposal.Changes) == 0 {
			e = fail("invalid_proposal", "批准方案缺文件快照")
		}
		seen := map[string]bool{}
		for _, change := range r.Proposal.Changes {
			if e != nil {
				break
			}
			if seen[change.Path] {
				e = fail("invalid_proposal", "重复文件路径")
				break
			}
			seen[change.Path] = true
			isSkill := strings.HasPrefix(change.Path, "internal/database/") || strings.HasPrefix(change.Path, "internal/auth/") || strings.HasPrefix(change.Path, "internal/logging/") || strings.HasPrefix(change.Path, "migrations/")
			if (input.Step == "skills") != isSkill {
				continue
			}
			if filepath.IsAbs(change.Path) || strings.Contains(change.Path, "\\") {
				e = fail("invalid_path", "非法模板路径")
				break
			}
			target := filepath.Join(r.Workspace, change.Path)
			if e = safeWorkspacePath(s.Config.DataDir, target); e != nil {
				break
			}
			if !within(r.Workspace, target) {
				e = fail("invalid_path", "模板越界")
				break
			}
			if e = os.MkdirAll(filepath.Dir(target), 0755); e == nil {
				e = writeAtomic(target, []byte(change.After), 0644)
			}
		}
		if e == nil && input.Step == "skills" {
			e = s.registerManifest(r)
		}
		detail = "已应用经批准文件快照，模板升级不会重新渲染旧方案"
	case "verify":
		if e = s.verifyApprovedSources(r); e == nil {
			var checks []Check
			checks, e = s.verifyWorkspaceVersion(ctx, r.Workspace, r.Definition.Tools["go"])
			_, saveErr := s.Store.Update(r.ID, func(run *Run) error { recordChecks(run, checks); return nil })
			if e == nil {
				e = saveErr
			}
		}
		if e == nil {
			e = s.registerManifest(r)
		}
		detail = "固定工具版本的真实测试/构建与工件秘密检查通过"
	case "repo":
		if e = s.verifyManifest(r); e == nil {
			if r.DeliveryMode == "local" {
				detail = "delivery_mode=local，远程仓库不适用"
			} else if s.Config.GitLabToken == "" || s.Config.GitLabNamespace == "" {
				return s.block(r.ID, input.Step, "需要 GITLAB_TOKEN 与 GITLAB_NAMESPACE_ID；尚未创建远程仓库，不会改为本地DONE")
			} else {
				e = s.PublishGitLab(ctx, r)
				detail = "已核实精确GitLab仓库与本次提交"
			}
		}
	case "ci":
		if e = s.verifyManifest(r); e == nil {
			if r.DeliveryMode == "local" {
				detail = "delivery_mode=local，远程CI不适用"
			} else {
				e = s.VerifyGitLabPipeline(ctx, r.ID)
				detail = "真实pipeline与批准提交SHA一致并成功"
			}
		}
	case "deliver":
		if e = s.verifyManifest(r); e == nil {
			e = s.Package(r)
		}
		if e == nil {
			e = s.verifyPackage(r)
		}
		if e == nil {
			_, e = s.Store.Update(r.ID, func(run *Run) error {
				recordChecks(run, []Check{{Name: "zip_integrity", Passed: true, Detail: "ZIP逐文件内容/hash与同源manifest一致"}})
				return requireCompletion(*run)
			})
		}
		detail = "ZIP内容已独立核验，固定required_checks齐全"
	}
	if e != nil {
		if errors.Is(e, context.Canceled) || errors.Is(e, context.DeadlineExceeded) {
			return s.block(r.ID, input.Step, "阶段中断/超时；已有副作用保留，恢复按原操作身份核实")
		}
		var unknown *UnknownResultError
		if errors.As(e, &unknown) {
			return s.block(r.ID, input.Step, unknown.Error())
		}
		var infrastructure *InfrastructureError
		if errors.As(e, &infrastructure) {
			return StepResult{SourceVersion: r.SourceVersion, StateSequence: r.StateSequence, State: r.State}, e
		}
		return s.failStep(r.ID, input.Step, e)
	}
	r, e = s.Store.Update(r.ID, func(run *Run) error {
		run.ActiveStep = ""
		state := "completed"
		if ((input.Step == "repo" || input.Step == "ci") && run.DeliveryMode == "local") || (input.Step == "skills" && len(run.Plan.Skills) == 0) {
			state = "skipped"
		}
		setStep(run, input.Step, state, detail)
		history(run, "step_completed", input.Step+": "+detail)
		artifacts, err := ListArtifacts(run.Workspace)
		if err != nil {
			return err
		}
		run.Artifacts = artifacts
		if run.CancelRequested {
			run.State = "CANCELLED"
			run.Error = "用户取消，已发生副作用保留"
			run.CancelRequested = false
			run.PauseRequested = false
			history(run, "cancel_applied", "安全边界终止")
		} else if run.PauseRequested && input.Step != "deliver" {
			run.ResumeTo = run.State
			run.State = "PAUSED"
			run.PauseRequested = false
			history(run, "pause_applied", "安全边界暂停")
		} else if input.Step == "deliver" {
			if err = requireCompletion(*run); err != nil {
				return err
			}
			run.State = "DONE"
			run.Error = ""
			run.PauseRequested = false
			addMessage(run, "assistant", fmt.Sprintf("%s已完成，%d个真实文件与ZIP通过验证。交付模式：%s。", run.Plan.ProjectName, len(run.Artifacts), run.DeliveryMode), "result", run.AcceptedProposal)
		}
		return nil
	})
	if e != nil {
		return StepResult{}, e
	}
	return StepResult{SourceVersion: r.SourceVersion, StateSequence: r.StateSequence, State: r.State, Continue: r.State != "PAUSED" && r.State != "BLOCKED" && !terminal(r.State)}, nil
}
func (s *Service) failStep(id, step string, reason error) (StepResult, error) {
	r, e := s.Store.Update(id, func(run *Run) error {
		if terminal(run.State) {
			return nil
		}
		run.ActiveStep = ""
		run.State = "FAILED"
		run.Error = s.redact(reason.Error())
		setStep(run, step, "failed", run.Error)
		history(run, "step_failed", step+": "+run.Error)
		addMessage(run, "assistant", "执行失败："+run.Error+"；产物和外部事实保留。", "result", run.AcceptedProposal)
		return nil
	})
	return StepResult{SourceVersion: r.SourceVersion, StateSequence: r.StateSequence, State: r.State}, e
}
func (s *Service) block(id, step, reason string) (StepResult, error) {
	r, e := s.Store.Update(id, func(r *Run) error {
		if terminal(r.State) || r.State == "PAUSED" {
			return nil
		}
		r.ActiveStep = ""
		r.ResumeTo = r.State
		r.State = "BLOCKED"
		r.Error = reason
		setStep(r, step, "pending", reason)
		history(r, "blocked", step+": "+reason)
		addMessage(r, "assistant", reason, "result", r.AcceptedProposal)
		return nil
	})
	return StepResult{SourceVersion: r.SourceVersion, StateSequence: r.StateSequence, State: r.State}, e
}
func within(root, path string) bool {
	relative, e := filepath.Rel(root, path)
	return e == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) && !filepath.IsAbs(relative)
}
func writeAtomic(path string, data []byte, mode os.FileMode) error {
	f, e := os.CreateTemp(filepath.Dir(path), ".agent-write-*")
	if e != nil {
		return e
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if e = f.Chmod(mode); e == nil {
		_, e = f.Write(data)
	}
	if e == nil {
		e = f.Sync()
	}
	ce := f.Close()
	if e == nil {
		e = ce
	}
	if e != nil {
		return e
	}
	return os.Rename(tmp, path)
}
func ListArtifacts(root string) ([]Artifact, error) {
	out := []Artifact{}
	e := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Type()&os.ModeSymlink != 0 {
			return fail("invalid_artifact", "symlink artifact rejected")
		}
		if strings.HasPrefix(d.Name(), ".agent-write-") {
			return nil
		}
		info, e := d.Info()
		if e != nil {
			return e
		}
		relative, e := filepath.Rel(root, path)
		if e != nil {
			return e
		}
		data, e := os.ReadFile(path)
		if e != nil {
			return e
		}
		out = append(out, Artifact{Path: filepath.ToSlash(relative), Size: info.Size(), SHA256: bytesDigest(data)})
		return nil
	})
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, e
}
func (s *Service) VerifyWorkspace(ctx context.Context, workspace string) ([]Check, error) {
	return s.verifyWorkspaceVersion(ctx, workspace, runtime.Version())
}
func (s *Service) verifyWorkspaceVersion(ctx context.Context, workspace, goVersion string) ([]Check, error) {
	checks := []Check{}
	for _, args := range [][]string{{"mod", "tidy"}, {"test", "./..."}, {"build", "./..."}} {
		heartbeat(ctx, strings.Join(args, " "))
		cmd := exec.CommandContext(ctx, "go", args...)
		cmd.Dir = workspace
		cmd.Env = append(isolatedGoEnv(), "GOTOOLCHAIN="+goVersion)
		var output limitedBuffer
		cmd.Stdout = &output
		cmd.Stderr = &output
		e := cmd.Run()
		if e != nil && ctx.Err() != nil {
			return checks, ctx.Err()
		}
		detail := strings.TrimSpace(output.String())
		if detail == "" {
			detail = "exit status 0"
		}
		checks = append(checks, Check{Name: "go " + strings.Join(args, " "), Passed: e == nil, Detail: s.redact(detail)})
		if e != nil {
			return checks, fmt.Errorf("go %s: %s", strings.Join(args, " "), s.redact(detail))
		}
	}
	for _, required := range []string{"go.mod", ".gitignore", ".gitlab-ci.yml", "README.md"} {
		_, e := os.Stat(filepath.Join(workspace, required))
		if e != nil {
			return checks, e
		}
		checks = append(checks, Check{Name: "file:" + required, Passed: true, Detail: "存在"})
	}
	ignore, e := os.ReadFile(filepath.Join(workspace, ".gitignore"))
	if e != nil {
		return checks, e
	}
	if !strings.Contains(string(ignore), ".env") || !strings.Contains(string(ignore), "*.key") {
		return checks, fmt.Errorf("secret ignore rules missing")
	}
	artifacts, e := ListArtifacts(workspace)
	if e != nil {
		return checks, e
	}
	for _, a := range artifacts {
		b, e := os.ReadFile(filepath.Join(workspace, a.Path))
		if e != nil {
			return checks, e
		}
		for _, secret := range []string{s.Config.GitLabToken, s.Config.AIKey} {
			if len(secret) >= 8 && bytes.Contains(b, []byte(secret)) {
				return checks, fmt.Errorf("secret detected in artifact %s", a.Path)
			}
		}
	}
	checks = append(checks, Check{Name: "secret_boundary", Passed: true, Detail: "忽略规则存在，执行器凭据未进入产物"})
	return checks, nil
}
func isolatedGoEnv() []string {
	allowed := map[string]bool{"PATH": true, "HOME": true, "TMPDIR": true, "GOPATH": true, "GOCACHE": true, "GOPROXY": true, "GOSUMDB": true, "GOTOOLCHAIN": true, "CC": true, "CGO_ENABLED": true, "SSL_CERT_FILE": true, "SSL_CERT_DIR": true}
	env := []string{}
	for _, kv := range os.Environ() {
		name, _, _ := strings.Cut(kv, "=")
		if allowed[name] {
			env = append(env, kv)
		}
	}
	return append(env, "GOWORK=off")
}

type limitedBuffer struct{ bytes.Buffer }

func (b *limitedBuffer) Write(p []byte) (int, error) {
	n := len(p)
	if b.Len() < 64000 {
		remaining := 64000 - b.Len()
		if len(p) > remaining {
			p = p[:remaining]
		}
		_, _ = b.Buffer.Write(p)
	}
	return n, nil
}
func (s *Service) Package(r Run) error {
	artifacts, e := ListArtifacts(r.Workspace)
	if e != nil {
		return e
	}
	out := filepath.Join(s.Config.DataDir, "downloads")
	if e = os.MkdirAll(out, 0700); e != nil {
		return e
	}
	f, e := os.CreateTemp(out, ".zip-*")
	if e != nil {
		return e
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	writer := zip.NewWriter(f)
	for _, a := range artifacts {
		dst, e := writer.Create(r.Plan.ProjectName + "/" + a.Path)
		if e != nil {
			writer.Close()
			f.Close()
			return e
		}
		src, e := os.Open(filepath.Join(r.Workspace, a.Path))
		if e != nil {
			writer.Close()
			f.Close()
			return e
		}
		_, e = io.Copy(dst, src)
		src.Close()
		if e != nil {
			writer.Close()
			f.Close()
			return e
		}
	}
	if e = writer.Close(); e == nil {
		e = f.Sync()
	}
	ce := f.Close()
	if e == nil {
		e = ce
	}
	if e != nil {
		return e
	}
	return os.Rename(tmp, filepath.Join(out, r.ID+".zip"))
}
func (s *Service) verifyPackage(r Run) error {
	z, e := zip.OpenReader(filepath.Join(s.Config.DataDir, "downloads", r.ID+".zip"))
	if e != nil {
		return e
	}
	defer z.Close()
	expected := map[string]string{}
	for _, f := range r.Manifest.Files {
		expected[r.Plan.ProjectName+"/"+f.Path] = f.SHA256
	}
	metadata, e := os.ReadFile(filepath.Join(r.Workspace, manifestName))
	if e != nil {
		return e
	}
	expected[r.Plan.ProjectName+"/"+manifestName] = bytesDigest(metadata)
	if len(z.File) != len(expected) {
		return fail("zip_mismatch", "ZIP工件集合与manifest不同")
	}
	for _, f := range z.File {
		want, exists := expected[f.Name]
		if !exists {
			return fail("zip_mismatch", "ZIP含未登记文件")
		}
		body, e := f.Open()
		if e != nil {
			return e
		}
		data, e := io.ReadAll(io.LimitReader(body, 16<<20))
		closeErr := body.Close()
		if e != nil {
			return e
		}
		if closeErr != nil {
			return closeErr
		}
		if bytesDigest(data) != want {
			return fail("zip_mismatch", "ZIP工件hash或CRC不一致")
		}
		delete(expected, f.Name)
	}
	return nil
}
func (s *Service) redact(text string) string {
	for _, secret := range []string{s.Config.GitLabToken, s.Config.AIKey} {
		if secret != "" {
			text = strings.ReplaceAll(text, secret, "[redacted]")
		}
	}
	return text
}

// Activity options cap work per stage; cancellation never claims rollback.
var _ = time.Minute
