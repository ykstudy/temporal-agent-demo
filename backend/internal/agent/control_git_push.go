package agent

import (
	"context"
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"agentstudio/internal/contracts"
)

func controlGitBlob(content string) string {
	value := sha1.Sum([]byte(fmt.Sprintf("blob %d\x00%s", len([]byte(content)), content)))
	return hex.EncodeToString(value[:])
}

func controlGitTreeMatches(output string, files map[string]string) error {
	seen := map[string]bool{}
	if !strings.HasSuffix(output, "\x00") {
		return remoteReceiptError("unterminated Git tree")
	}
	for _, row := range strings.Split(strings.TrimSuffix(output, "\x00"), "\x00") {
		metadata, path, found := strings.Cut(row, "\t")
		body, exists := files[path]
		if !found || !exists || seen[path] || metadata != "100644 blob "+controlGitBlob(body) {
			return remoteReceiptError("Git tree path, mode or blob")
		}
		seen[path] = true
	}
	if len(seen) != len(files) {
		return remoteReceiptError("incomplete Git tree")
	}
	return nil
}

func (s *Service) executeControlPush(ctx context.Context, id string, x contracts.ExecutionRequest, reconcile bool) (Run, error) {
	s.mu.Lock()
	key := "control-git-execution:" + id
	if s.commandLocks == nil {
		s.commandLocks = map[string]*sync.Mutex{}
	}
	lock := s.commandLocks[key]
	if lock == nil {
		lock = &sync.Mutex{}
		s.commandLocks[key] = lock
	}
	s.mu.Unlock()
	lock.Lock()
	defer lock.Unlock()
	r, err := s.Store.Get(id)
	if err != nil {
		return r, err
	}
	target, op, err := s.controlGitLabRequest(r, x)
	if err != nil {
		return r, err
	}
	if op.Status == "verified" {
		return r, controlGitLabExistingResult(r, x)
	}
	_, raw, _ := scheduleAction(x.Action)
	var parameters contracts.PushRepositoryParameters
	if err = contracts.DecodeVersion("PushRepositoryParameters", x.ContractVersion, raw, &parameters); err != nil {
		return r, err
	}
	if reconcile {
		if op.Status != "sent" && op.Status != "unknown" {
			return r, fail("reconciliation_not_required", "推送核实需要原sent或unknown请求")
		}
		if err = s.recoverControlGitLaunch(id, x); err != nil {
			return s.controlGitLabBlock(id, x, true, "git_push_history_incomplete", err)
		}
		r, err = s.controlGitLabBlock(id, x, true, "git_push_reconciliation_required", nil)
		if err != nil {
			return r, err
		}
	} else {
		if op.Status == "sent" || op.Status == "unknown" {
			return r, fail("reconciliation_required", "原推送只能只读核实，不能再次启动Git")
		}
		if op.Status != "ready" || r.ActiveStep != "" || r.State != "DELIVER" || r.PauseRequested || r.CancelRequested {
			return r, fail("execution_stopped", "当前Run不允许新推送")
		}
		if err = s.recheckControlApproval(r, time.Now().UTC()); err != nil {
			return r, err
		}
		next, e := NextControlAction(r, r.Control.Schedule)
		if e != nil {
			return r, e
		}
		if next.Execution == nil || !chainEqual(*next.Execution, x) || next.NeedsReconciliation {
			return r, fail("execution_binding_mismatch", "推送不是准确下一请求")
		}
		var project controlGitLabProject
		if _, err = s.controlGitLabRead(ctx, id, x, remoteProjectReadPath(parameters.ProjectID), &project); err != nil {
			return s.controlGitLabBlock(id, x, false, "git_push_preflight_unavailable", err)
		}
		var created *contracts.RemoteExecutionReceipt
		for _, q := range r.Control.Audit.RemoteReceipts {
			if q.ActionType == "create_repository" && q.Status == "succeeded" && q.SourceVersion == x.SourceVersion && q.ApprovalID == x.Approval.ApprovalID && q.Project != nil && q.Project.ProjectID == parameters.ProjectID {
				copy := q
				created = &copy
			}
		}
		if created == nil || project.EmptyRepo == nil || !*project.EmptyRepo || project.ID != parameters.ProjectID || project.Description != created.Project.OwnershipMarker || !remoteProjectTarget(controlProjectFact(project), target) || parameters.ExpectedBranchSha != nil {
			return s.controlGitLabBlock(id, x, false, "git_push_target_conflict", fail("target_mismatch", "原创建项目不再是批准的空仓库"))
		}
		var branch any
		status, readErr := s.controlGitLabRead(ctx, id, x, remoteProjectReadPath(parameters.ProjectID)+"/repository/branches/"+url.PathEscape(target.Ref), &branch)
		if status != http.StatusNotFound {
			if readErr == nil {
				readErr = fail("target_conflict", "批准的新分支已存在")
			}
			return s.controlGitLabBlock(id, x, false, "git_push_branch_unavailable", readErr)
		}
		var parent contracts.BudgetReservation
		r, parent, err = s.beginControlExecution(id, x)
		if err != nil {
			return r, err
		}
		if err = s.executeControlGitTools(ctx, r, x, target, parent); err != nil {
			return s.controlGitLabBlock(id, x, true, "git_push_result_unknown", err)
		}
	}
	if err = s.observeControlPush(ctx, id, x, parameters); err != nil {
		return s.controlGitLabBlock(id, x, true, "git_push_observation_unknown", err)
	}
	return s.completeControlPush(id, x, parameters)
}

func controlProjectFact(p controlGitLabProject) contracts.RemoteProjectObservation {
	empty := false
	if p.EmptyRepo != nil {
		empty = *p.EmptyRepo
	}
	return contracts.RemoteProjectObservation{ProjectID: p.ID, NamespaceID: strconv.FormatInt(p.Namespace.ID, 10), NamespacePath: p.Namespace.FullPath, ProjectPath: p.Path, PathWithNamespace: p.PathWithNamespace, Visibility: p.Visibility, OwnershipMarker: p.Description, EmptyRepository: empty, DefaultBranch: p.DefaultBranch}
}

func (s *Service) recoverControlGitLaunch(id string, x contracts.ExecutionRequest) error {
	r, err := s.Store.Get(id)
	if err != nil {
		return err
	}
	parent, err := controlGitLabAttempt(r, x)
	if err != nil {
		return err
	}
	for _, reserved := range r.Control.Audit.Budget.Reservations {
		if reserved.ExecutionID == nil || *reserved.ExecutionID != x.ExecutionID || reserved.Kind != "tool" {
			continue
		}
		found := false
		for _, q := range r.Control.Audit.ToolReceipts {
			if q.AttemptID == reserved.AttemptID {
				found = true
			}
		}
		if found {
			continue
		}
		launch, exists := r.Control.GitInvocations[reserved.AttemptID]
		if !exists || launch.Parent != parent.AttemptID {
			return fail("git_history_incomplete", "原Git启动记录缺失，不能补造历史或重启命令")
		}
		q, e := controlGitReceipt(r, x, launch, "unknown", nil, "", true, time.Now().UTC())
		if e != nil {
			return e
		}
		if err = s.persistControlGitReceipt(id, x, reserved, q, true); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) observeControlPush(ctx context.Context, id string, x contracts.ExecutionRequest, p contracts.PushRepositoryParameters) error {
	r, err := s.Store.Get(id)
	if err != nil {
		return err
	}
	parent, err := controlGitLabAttempt(r, x)
	if err != nil {
		return err
	}
	sha := ""
	for _, q := range r.Control.Audit.ToolReceipts {
		if q.ExecutionID == x.ExecutionID && q.AttemptID == controlGitChild(x, parent.AttemptID, "git.head") && q.Status == "succeeded" && !q.OutputTruncated {
			sha = strings.TrimSpace(q.Output)
		}
	}
	if !r.Control.RemoteWriteStarted[x.ExecutionID] || contracts.ValidateVersion("RemoteBranchObservation", "1.3.0", contracts.RemoteBranchObservation{ProjectID: p.ProjectID, Ref: p.Target.Ref, CommitSha: sha}) != nil {
		return fail("git_history_incomplete", "未记录原push发送及本地准确SHA，禁止猜测远端提交")
	}
	return s.observeControlCommit(ctx, id, x, p.ProjectID, p.Target, sha, "reconciliation_read")
}

func (s *Service) observeControlCommit(ctx context.Context, id string, x contracts.ExecutionRequest, projectID int64, target contracts.GitLabTarget, sha, readKind string) error {
	r, err := s.Store.Get(id)
	if err != nil {
		return err
	}
	read := func(path string, output any, normalize func() (controlRemoteFact, error)) (int, error) {
		return s.controlRemoteRead(ctx, id, x, path, readKind, output, func(bounded context.Context) (int, error) {
			return s.gitlab(bounded, http.MethodGet, path, nil, output)
		}, normalize)
	}
	base := remoteProjectReadPath(projectID)
	var project controlGitLabProject
	if _, err = read(base, &project, nil); err != nil {
		return err
	}
	var branch struct {
		Name   string `json:"name"`
		Commit struct {
			ID string `json:"id"`
		} `json:"commit"`
	}
	if _, err = read(base+"/repository/branches/"+url.PathEscape(target.Ref), &branch, func() (controlRemoteFact, error) {
		return controlRemoteFact{"branch", contracts.RemoteBranchObservation{ProjectID: projectID, Ref: branch.Name, CommitSha: branch.Commit.ID}}, nil
	}); err != nil {
		return err
	}
	if branch.Name != target.Ref || branch.Commit.ID != sha {
		return fail("target_mismatch", "远端分支不指向原本地提交")
	}
	type entry struct {
		ID   string `json:"id"`
		Path string `json:"path"`
		Type string `json:"type"`
		Mode string `json:"mode"`
	}
	listed := map[string]entry{}
	// Count directories too; stop a malicious/repeating page within the frozen
	// read budget. Completion requires a final short page, not a guessed count.
	complete := false
	for page := 1; page <= 16; page++ {
		var rows []entry
		path := base + "/repository/tree?ref=" + url.QueryEscape(sha) + "&recursive=true&per_page=100&page=" + strconv.Itoa(page)
		if _, err = read(path, &rows, nil); err != nil {
			return err
		}
		for _, row := range rows {
			if contracts.Validate("agent.schema.json#/$defs/RelativePath", row.Path) != nil {
				return fail("remote_tree_invalid", "远端树路径非法")
			}
			if row.Type == "tree" && row.Mode == "040000" {
				continue
			}
			if _, duplicate := listed[row.Path]; duplicate || row.Type != "blob" || row.Mode != "100644" || int64(len(listed)+1) > r.FrozenDefinition.Definition.BudgetPolicy.MaxFiles {
				return fail("remote_tree_invalid", "远端文件树重复、类型或预算不符")
			}
			listed[row.Path] = row
		}
		if len(rows) < 100 {
			complete = true
			break
		}
	}
	if !complete || len(listed) == 0 {
		return fail("remote_tree_incomplete", "未观察到完整远端文件树")
	}
	names := make([]string, 0, len(listed))
	for name := range listed {
		names = append(names, name)
	}
	sort.Strings(names)
	files := []contracts.RemoteFileEvidence{}
	for _, name := range names {
		var file struct {
			Path     string `json:"file_path"`
			Encoding string `json:"encoding"`
			Content  string `json:"content"`
			Size     int64  `json:"size"`
			Blob     string `json:"blob_id"`
			Commit   string `json:"commit_id"`
			SHA256   string `json:"content_sha256"`
		}
		var fact contracts.RemoteFileEvidence
		path := base + "/repository/files/" + url.PathEscape(name) + "?ref=" + url.QueryEscape(sha)
		if _, err = read(path, &file, func() (controlRemoteFact, error) {
			body, e := base64.StdEncoding.DecodeString(strings.ReplaceAll(file.Content, "\n", ""))
			if e != nil || file.Encoding != "base64" || file.Path != name || file.Commit != sha || file.Size != int64(len(body)) || file.Blob != controlGitBlob(string(body)) || file.Blob != listed[name].ID || file.SHA256 != contracts.BytesHash(body) {
				return controlRemoteFact{}, fail("remote_file_invalid", "远端文件原文与准确提交、blob、size或hash不符")
			}
			fact = contracts.RemoteFileEvidence{Path: name, Size: file.Size, SHA256: contracts.BytesHash(body), GitBlobSha1: file.Blob}
			return controlRemoteFact{"file", fact}, nil
		}); err != nil {
			return err
		}
		files = append(files, fact)
	}
	// A commit observation combines the actual commit metadata with the complete
	// tree/per-file reads immediately above; it never copies local manifest bytes.
	var commit struct {
		ID      string   `json:"id"`
		Parents []string `json:"parent_ids"`
		Message string   `json:"message"`
	}
	_, err = read(base+"/repository/commits/"+sha, &commit, func() (controlRemoteFact, error) {
		if commit.ID != sha || commit.Parents == nil {
			return controlRemoteFact{}, fail("remote_commit_invalid", "远端提交缺少原SHA或完整parent集合")
		}
		return controlRemoteFact{"commit", contracts.RemoteCommitObservation{ProjectID: projectID, CommitSha: commit.ID, ParentShas: commit.Parents, Message: strings.TrimSuffix(commit.Message, "\n"), Files: files}}, nil
	})
	return err
}

func (s *Service) completeControlPush(id string, x contracts.ExecutionRequest, p contracts.PushRepositoryParameters) (Run, error) {
	return s.Store.Update(id, func(r *Run) error {
		_, op, err := s.controlGitLabRequest(*r, x)
		if err != nil {
			return err
		}
		if op.Status == "verified" {
			return controlGitLabExistingResult(*r, x)
		}
		if op.Status != "sent" && op.Status != "unknown" {
			return fail("operation_conflict", "原推送未发送")
		}
		parent, err := controlGitLabAttempt(*r, x)
		if err != nil {
			return err
		}
		completed := time.Now().UTC()
		if controlGitLabSettled(*r, parent) == nil {
			if err = settleControlInRun(r, parent, "succeeded", completed); err != nil {
				return err
			}
		}
		if err = appendControlRemoteReceipt(r, x, "succeeded", completed); err != nil {
			return err
		}
		q := r.Control.Audit.RemoteReceipts[len(r.Control.Audit.RemoteReceipts)-1]
		manifest := contracts.ObjectReference(q.ManifestRef)
		result := CanonicalActionResult{RunID: x.RunID, SourceVersion: x.SourceVersion, DefinitionSnapshotHash: x.DefinitionSnapshotHash, PlanHash: x.PlanHash, ApprovalID: q.ApprovalID, ApprovalHash: q.ApprovalHash, ProposalSnapshotHash: q.ProposalSnapshotHash, ExecutionID: x.ExecutionID, OperationID: x.OperationID, AttemptID: parent.AttemptID, RequestHash: x.RequestHash, TargetHash: op.TargetHash, ActionType: "push_repository", ManifestRef: &manifest, ObservedTarget: &p.Target, ProjectID: p.ProjectID, CommitSHA: q.Commit.CommitSha, CIConfigSHA256: *q.CIConfigSHA256, CompletedAt: q.CompletedAt}
		result.ResultHash, err = HashCanonicalActionResult(result)
		if err != nil {
			return err
		}
		r.Control.Schedule.Results = append(r.Control.Schedule.Results, result)
		commit := q.Commit.CommitSha
		op.Status, op.ExternalObjectID, op.UpdatedAt = "verified", &commit, q.CompletedAt
		if r.ActiveStep == x.ExecutionID {
			r.ActiveStep = ""
		}
		syncBudgetProjection(r)
		return nil
	})
}
