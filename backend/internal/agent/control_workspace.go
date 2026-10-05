package agent

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"agentstudio/internal/contracts"
)

// ExecuteControlWorkspace applies only exact approved bytes owned by this
// scaffold/Skill action. Admission and every published result use the complete
// immutable request; an uncertain write never starts another attempt.
func (s *Service) ExecuteControlWorkspace(ctx context.Context, id string, request contracts.ExecutionRequest) (Run, contracts.WorkspaceReceipt, error) {
	var empty contracts.WorkspaceReceipt
	r, err := s.Store.Get(id)
	if err != nil {
		return r, empty, err
	}
	kind, _, err := scheduleAction(request.Action)
	if err != nil || (kind != "scaffold" && kind != "apply_skill") {
		return r, empty, fail("invalid_workspace_action", "工作区执行器仅接受准确scaffold/apply_skill请求")
	}
	if r.Control == nil || request.RunID != id || request.SourceVersion != int64(r.SourceVersion) {
		return r, empty, fail("execution_binding_mismatch", "工作区请求不属于当前Run来源")
	}
	// A completed request is an immutable receipt replay, not permission to
	// perform its writes again. The scheduler independently checks all evidence.
	for _, old := range r.Control.Audit.WorkspaceReceipts {
		if old.ExecutionID != request.ExecutionID || old.Status != "succeeded" {
			continue
		}
		if !hashMatches("ExecutionRequest", request, request.RequestHash) || !hashMatches("WorkspaceReceipt", old, old.ReceiptHash) || old.RequestHash != request.RequestHash || old.ActionType != kind || old.SourceVersion != request.SourceVersion {
			return r, empty, fail("workspace_receipt_mismatch", "工作区回执不属于该不可变请求")
		}
		for _, operation := range r.Control.Audit.Operations {
			if operation.OperationID == request.OperationID && operation.RequestHash == request.RequestHash && operation.Status == "verified" && scheduleHasRef(operation.EvidenceRefs, contracts.ObjectReference{ObjectType: "workspace", ObjectID: old.ReceiptID, ObjectVersion: 1, SourceVersion: old.SourceVersion, Hash: old.ReceiptHash}) {
				return r, old, nil
			}
		}
		return r, empty, fail("workspace_receipt_mismatch", "成功回执缺少原操作核实")
	}
	files, err := controlWorkspaceOwnedFiles(r, request)
	if err != nil {
		return r, empty, err
	}
	if err = ctx.Err(); err != nil {
		return r, empty, err
	}
	r, reservation, err := s.beginControlExecution(id, request)
	if err != nil {
		return r, empty, err
	}
	started := time.Now().UTC()
	deadline, err := time.Parse(time.RFC3339Nano, *reservation.DeadlineAt)
	if err != nil {
		return s.controlWorkspaceFailed(r, request, reservation, started, nil, false, err)
	}
	bounded, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()
	root, full, err := s.controlWorkspaceRoot(r, request)
	if err != nil {
		return s.controlWorkspaceFailed(r, request, reservation, started, nil, false, err)
	}
	defer root.Close()
	before, err := s.controlToolFiles(r, full)
	if err != nil {
		return s.controlWorkspaceFailed(r, request, reservation, started, nil, false, err)
	}
	if err = controlWorkspaceMatchManifest(r.Control.Audit.Manifest, before); err != nil {
		return s.controlWorkspaceFailed(r, request, reservation, started, nil, false, err)
	}
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	written := []string{}
	touched := false
	for _, name := range names {
		if err = bounded.Err(); err != nil {
			return s.controlWorkspaceFailed(r, request, reservation, started, written, touched, err)
		}
		// Context changes are checked at each file boundary. Cancellation during
		// a write is observed after its fsync; it never claims a rollback.
		current, getErr := s.Store.Get(id)
		if getErr != nil {
			return s.controlWorkspaceFailed(r, request, reservation, started, written, touched, getErr)
		}
		if current.SourceVersion != r.SourceVersion || current.ActiveStep != request.ExecutionID || current.Control.Audit.Approval == nil || current.Control.Audit.Approval.ApprovalID != request.Approval.ApprovalID || current.Control.Audit.Approval.RevokedAt != nil || current.Control.Audit.PermissionSnapshot == nil || current.Control.Audit.PermissionSnapshot.Revoked {
			return s.controlWorkspaceFailed(r, request, reservation, started, written, touched, fail("permission_revoked", "执行中来源或批准已改变"))
		}
		if err = controlWorkspaceSafeRelative(root, name); err != nil {
			return s.controlWorkspaceFailed(r, request, reservation, started, written, touched, err)
		}
		if _, exists := before[name]; exists {
			allowedBefore := false
			for _, change := range r.Control.Audit.Proposal.Changes {
				if change.Path == name && ((change.Before != nil && *change.Before == before[name]) || files[name] == before[name]) {
					allowedBefore = true
				}
			}
			if !allowedBefore {
				return s.controlWorkspaceFailed(r, request, reservation, started, written, touched, fail("artifact_source_mismatch", "现有文件未被准确批准覆盖"))
			}
		}
		touched = true
		if err = controlWorkspaceAtomicWrite(root, name, files[name]); err != nil {
			return s.controlWorkspaceFailed(r, request, reservation, started, written, touched, err)
		}
		written = append(written, name)
	}
	if err = bounded.Err(); err != nil {
		return s.controlWorkspaceFailed(r, request, reservation, started, written, touched, err)
	}
	after, err := s.controlToolFiles(r, full)
	if err != nil {
		return s.controlWorkspaceFailed(r, request, reservation, started, written, touched, err)
	}
	expected := map[string]string{}
	for name, content := range before {
		expected[name] = content
	}
	for name, content := range files {
		expected[name] = content
	}
	if !equalContractValue(expected, after) {
		return s.controlWorkspaceFailed(r, request, reservation, started, written, touched, fail("artifact_source_mismatch", "写入后的真实集合与动作范围不符"))
	}
	manifest, err := controlWorkspaceManifest(r, request, after, time.Now().UTC())
	if err != nil {
		return s.controlWorkspaceFailed(r, request, reservation, started, written, touched, err)
	}
	completed := time.Now().UTC()
	receipt := controlWorkspaceReceipt(r, request, reservation, kind, started, completed, written, "succeeded")
	ref := controlManifestRef(manifest)
	receipt.OutputManifestRef = &contracts.WorkspaceReceiptOutputManifestRef{ObjectType: ref.ObjectType, ObjectID: ref.ObjectID, ObjectVersion: ref.ObjectVersion, SourceVersion: ref.SourceVersion, Hash: ref.Hash}
	receipt.ReceiptHash, err = contracts.HashObject("WorkspaceReceipt", receipt)
	if err != nil {
		return s.controlWorkspaceFailed(r, request, reservation, started, written, touched, err)
	}
	if err = contracts.Validate("WorkspaceReceipt", receipt); err != nil {
		return s.controlWorkspaceFailed(r, request, reservation, started, written, touched, err)
	}
	updated, err := s.Store.Update(id, func(run *Run) error {
		if run.SourceVersion != r.SourceVersion || run.ActiveStep != request.ExecutionID {
			return fail("stale_source", "写入后Run来源已改变，须核实原动作")
		}
		run.Control.Audit.Manifest = &manifest
		run.Control.Audit.ManifestHistory = append(run.Control.Audit.ManifestHistory, manifest)
		run.Control.Audit.WorkspaceReceipts = append(run.Control.Audit.WorkspaceReceipts, receipt)
		for i := range run.Control.Audit.Operations {
			op := &run.Control.Audit.Operations[i]
			if op.OperationID == request.OperationID {
				if op.Status != "sent" || op.RequestHash != request.RequestHash {
					return fail("operation_conflict", "原写操作状态已改变")
				}
				op.Status = "verified"
				op.UpdatedAt = completed.Format(time.RFC3339Nano)
				op.EvidenceRefs = append(op.EvidenceRefs, contracts.ObjectReference{ObjectType: "workspace", ObjectID: receipt.ReceiptID, ObjectVersion: 1, SourceVersion: receipt.SourceVersion, Hash: receipt.ReceiptHash})
			}
		}
		if e := settleControlInRun(run, reservation, "succeeded", time.Now().UTC()); e != nil {
			return e
		}
		run.ActiveStep = ""
		run.Error = ""
		run.Artifacts = []Artifact{}
		for _, file := range manifest.Files {
			run.Artifacts = append(run.Artifacts, Artifact{Path: file.Path, Size: file.Size, SHA256: file.SHA256})
		}
		controlWorkspaceBoundary(run)
		return nil
	})
	if err != nil {
		return s.controlWorkspaceFailed(r, request, reservation, started, written, touched, err)
	}
	return updated, receipt, nil
}

func controlWorkspaceOwnedFiles(r Run, request contracts.ExecutionRequest) (map[string]string, error) {
	if r.FrozenDefinition == nil || r.Control.Audit.Proposal == nil || r.Control.Audit.Plan == nil {
		return nil, fail("control_binding_mismatch", "缺少完整批准文件来源")
	}
	kind, parameters, err := scheduleAction(request.Action)
	if err != nil {
		return nil, err
	}
	selected := map[string]bool{}
	for _, raw := range r.Control.Audit.Plan.SelectedSkills {
		var value struct {
			SkillID string `json:"skill_id"`
		}
		if err = json.Unmarshal(raw, &value); err != nil {
			return nil, err
		}
		selected[value.SkillID] = true
	}
	owned := map[string]bool{}
	if kind == "apply_skill" {
		var p contracts.ApplySkillParameters
		if err = json.Unmarshal(parameters, &p); err != nil {
			return nil, err
		}
		var value struct {
			SkillID string `json:"skill_id"`
		}
		if err = json.Unmarshal(p.Selection, &value); err != nil {
			return nil, err
		}
		if !selected[value.SkillID] {
			return nil, fail("invalid_skill_selection", "未选择的Skill不能写文件")
		}
		for _, skill := range r.FrozenDefinition.Skills {
			if skill.SkillID == value.SkillID {
				for _, path := range skill.WritePaths {
					owned[path] = true
				}
			}
		}
	} else if kind == "scaffold" {
		for _, change := range r.Control.Audit.Proposal.Changes {
			if change.Operation == "delete" && change.After == nil {
				continue
			}
			owned[change.Path] = true
		}
		for _, skill := range r.FrozenDefinition.Skills {
			if selected[skill.SkillID] {
				for _, path := range skill.WritePaths {
					delete(owned, path)
				}
			}
		}
	} else {
		return nil, fail("invalid_workspace_action", "不支持的工作区动作")
	}
	files := map[string]string{}
	for _, change := range r.Control.Audit.Proposal.Changes {
		// A new approval/source writes a fresh generation. Deleted files are
		// absent from that full After snapshot; never delete the old source tree.
		if change.Operation == "delete" && change.After == nil {
			continue
		}
		if !owned[change.Path] {
			continue
		}
		if change.After == nil || change.AfterSHA256 == nil || contracts.BytesHash([]byte(*change.After)) != *change.AfterSHA256 {
			return nil, fail("invalid_approved_content", "工作区动作必须具有完整批准后内容")
		}
		if err = contracts.Validate("agent.schema.json#/$defs/RelativePath", change.Path); err != nil {
			return nil, err
		}
		files[change.Path] = *change.After
	}
	if len(files) == 0 {
		return nil, fail("invalid_workspace_action", "动作没有属于它的已批准文件")
	}
	return files, nil
}

func (s *Service) controlWorkspaceRoot(r Run, request contracts.ExecutionRequest) (*os.Root, string, error) {
	key := controlWorkspaceKey(r)
	if request.Workspace.RunID != r.ID || request.Workspace.StorageKey != key || !equalContractValue(request.Workspace, r.Control.Schedule.Workspace) {
		return nil, "", fail("invalid_path", "工作区必须绑定准确Run与项目")
	}
	data, err := filepath.Abs(s.Config.DataDir)
	if err != nil {
		return nil, "", err
	}
	data, err = filepath.EvalSymlinks(data)
	if err != nil {
		return nil, "", err
	}
	full := filepath.Join(data, "workspaces", filepath.FromSlash(key))
	if err = safeWorkspacePath(data, full); err != nil {
		return nil, "", err
	}
	anchor, err := os.OpenRoot(data)
	if err != nil {
		return nil, "", err
	}
	defer anchor.Close()
	if err = anchor.MkdirAll("workspaces/"+r.ID, 0700); err != nil {
		return nil, "", err
	}
	runRoot, err := anchor.OpenRoot("workspaces/" + r.ID)
	if err != nil {
		return nil, "", err
	}
	defer runRoot.Close()
	relative := strings.TrimPrefix(key, r.ID+"/")
	if relative == key || contracts.Validate("agent.schema.json#/$defs/RelativePath", relative) != nil {
		return nil, "", fail("invalid_path", "工作区代际必须位于准确Run目录内")
	}
	if err = runRoot.MkdirAll(relative, 0700); err != nil {
		return nil, "", err
	}
	if err = safeWorkspacePath(data, full); err != nil {
		return nil, "", err
	}
	root, err := runRoot.OpenRoot(relative)
	return root, full, err
}

func controlWorkspaceSafeRelative(root *os.Root, name string) error {
	if err := contracts.Validate("agent.schema.json#/$defs/RelativePath", name); err != nil {
		return err
	}
	parts := strings.Split(name, "/")
	for i := 1; i <= len(parts); i++ {
		path := strings.Join(parts[:i], "/")
		info, err := root.Lstat(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 || i < len(parts) && !info.IsDir() || i == len(parts) && !info.Mode().IsRegular() {
			return fail("invalid_path", "批准路径包含符号链接或特殊文件")
		}
	}
	return nil
}

func controlWorkspaceAtomicWrite(root *os.Root, name, contents string) error {
	if err := controlWorkspaceSafeRelative(root, name); err != nil {
		return err
	}
	parent := filepath.ToSlash(filepath.Dir(name))
	if err := root.MkdirAll(parent, 0700); err != nil {
		return err
	}
	if err := controlWorkspaceSafeRelative(root, name); err != nil {
		return err
	}
	tmp := parent + "/.write-" + newID("pending_")
	f, err := root.OpenFile(tmp, os.O_CREATE|os.O_EXCL|os.O_WRONLY|syscall.O_NOFOLLOW, 0600)
	if err != nil {
		return err
	}
	defer root.Remove(tmp)
	_, err = f.WriteString(contents)
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
	if err = root.Rename(tmp, name); err != nil {
		return err
	}
	dir, err := root.Open(parent)
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}

func controlWorkspaceMatchManifest(manifest *contracts.ArtifactManifest, files map[string]string) error {
	if manifest == nil {
		if len(files) == 0 {
			return nil
		}
		return fail("artifact_source_mismatch", "首次动作发现未登记文件，禁止覆盖")
	}
	if !hashMatches("ArtifactManifest", *manifest, manifest.Hash) || len(manifest.Files) != len(files) {
		return fail("artifact_source_mismatch", "已有工作区与登记manifest不符")
	}
	for _, file := range manifest.Files {
		content, ok := files[file.Path]
		if !ok || int64(len(content)) != file.Size || contracts.BytesHash([]byte(content)) != file.SHA256 {
			return fail("artifact_source_mismatch", "输入文件与登记manifest不符")
		}
	}
	return nil
}

func controlWorkspaceManifest(r Run, request contracts.ExecutionRequest, files map[string]string, at time.Time) (contracts.ArtifactManifest, error) {
	version := int64(1)
	previous := map[string]contracts.ArtifactEntry{}
	for _, old := range r.Control.Audit.ManifestHistory {
		if old.ManifestVersion >= version {
			version = old.ManifestVersion + 1
		}
	}
	if r.Control.Audit.Manifest != nil {
		for _, entry := range r.Control.Audit.Manifest.Files {
			previous[entry.Path] = entry
		}
	}
	identity, _ := contracts.HashValue([]string{r.ID, "workspace_manifest"})
	m := contracts.ArtifactManifest{ContractVersion: request.ContractVersion, ManifestID: "manifest_" + identity[:32], ManifestVersion: version, RunID: r.ID, SourceVersion: request.SourceVersion, ProposalID: request.Approval.ProposalID, ProposalVersion: request.Approval.ProposalVersion, ProposalSnapshotHash: request.Approval.ProposalSnapshotHash, DefinitionSnapshotHash: request.DefinitionSnapshotHash, PlanHash: request.PlanHash, Files: []contracts.ArtifactEntry{}, CreatedAt: at.Format(time.RFC3339Nano)}
	for name, content := range files {
		hash := contracts.BytesHash([]byte(content))
		if old, exists := previous[name]; exists && old.SHA256 == hash && old.Size == int64(len(content)) {
			m.Files = append(m.Files, old)
			continue
		}
		id, _ := contracts.HashValue([]string{r.ID, name, hash})
		m.Files = append(m.Files, contracts.ArtifactEntry{ArtifactID: "artifact_" + id[:32], Path: name, Size: int64(len(content)), SHA256: hash, MediaType: "text/plain"})
	}
	sort.Slice(m.Files, func(i, j int) bool { return m.Files[i].Path < m.Files[j].Path })
	var err error
	m.Hash, err = contracts.HashObject("ArtifactManifest", m)
	if err == nil {
		err = contracts.Validate("ArtifactManifest", m)
	}
	return m, err
}

func controlWorkspaceReceipt(r Run, x contracts.ExecutionRequest, reservation contracts.BudgetReservation, kind string, started, completed time.Time, written []string, status string) contracts.WorkspaceReceipt {
	id, _ := contracts.HashValue([]string{x.RequestHash, reservation.AttemptID})
	receipt := contracts.WorkspaceReceipt{ContractVersion: x.ContractVersion, ReceiptID: "workspace_receipt_" + id[:32], RunID: x.RunID, SourceVersion: x.SourceVersion, DefinitionSnapshotHash: x.DefinitionSnapshotHash, ExecutionID: x.ExecutionID, OperationID: x.OperationID, AttemptID: reservation.AttemptID, RequestHash: x.RequestHash, ApprovalID: x.Approval.ApprovalID, ApprovalHash: r.Control.Audit.Approval.ApprovalHash, ProposalSnapshotHash: x.Approval.ProposalSnapshotHash, PlanHash: x.PlanHash, Workspace: x.Workspace, ActionType: kind, WrittenPaths: append([]string{}, written...), StartedAt: started.Format(time.RFC3339Nano), CompletedAt: completed.Format(time.RFC3339Nano), Status: status}
	if r.Control.Audit.Manifest != nil {
		ref := controlManifestRef(*r.Control.Audit.Manifest)
		receipt.InputManifestRef = &contracts.WorkspaceReceiptInputManifestRef{ObjectType: ref.ObjectType, ObjectID: ref.ObjectID, ObjectVersion: ref.ObjectVersion, SourceVersion: ref.SourceVersion, Hash: ref.Hash}
	}
	return receipt
}

func (s *Service) controlWorkspaceFailed(r Run, x contracts.ExecutionRequest, reservation contracts.BudgetReservation, started time.Time, written []string, touched bool, cause error) (Run, contracts.WorkspaceReceipt, error) {
	kind, _, _ := scheduleAction(x.Action)
	status := "failed"
	if touched {
		status = "unknown"
	}
	completed := time.Now().UTC()
	receipt := controlWorkspaceReceipt(r, x, reservation, kind, started, completed, written, status)
	receipt.ReceiptHash, _ = contracts.HashObject("WorkspaceReceipt", receipt)
	updated, err := s.Store.Update(r.ID, func(run *Run) error {
		if run.ActiveStep != x.ExecutionID {
			return fail("operation_conflict", "失败动作已经被另一尝试替换")
		}
		run.Control.Audit.WorkspaceReceipts = append(run.Control.Audit.WorkspaceReceipts, receipt)
		for i := range run.Control.Audit.Operations {
			op := &run.Control.Audit.Operations[i]
			if op.OperationID == x.OperationID {
				op.Status = "unknown"
				op.UpdatedAt = completed.Format(time.RFC3339Nano)
				op.EvidenceRefs = append(op.EvidenceRefs, contracts.ObjectReference{ObjectType: "workspace", ObjectID: receipt.ReceiptID, ObjectVersion: 1, SourceVersion: receipt.SourceVersion, Hash: receipt.ReceiptHash})
			}
		}
		if e := settleControlInRun(run, reservation, status, time.Now().UTC()); e != nil {
			return e
		}
		run.ActiveStep = ""
		run.ResumeTo = "IMPLEMENT"
		run.State = "BLOCKED"
		run.Error = s.redact(cause.Error())
		controlWorkspaceBoundary(run)
		return nil
	})
	return updated, receipt, errors.Join(cause, err)
}

func controlWorkspaceBoundary(run *Run) {
	if run.CancelRequested {
		run.State = "CANCELLED"
		run.CancelRequested = false
		run.PauseRequested = false
		run.Error = "用户取消；已发生文件事实与原操作保留"
	} else if run.PauseRequested {
		if run.State != "BLOCKED" {
			run.ResumeTo = run.State
			run.State = "PAUSED"
		}
		run.PauseRequested = false
	}
}
