package agent

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"

	"agentstudio/internal/contracts"
)

// Transport fields are observations, never an authorization DTO. Missing
// booleans and pipeline identities must remain distinguishable from zero.
type controlCIPipeline struct {
	ID        int64  `json:"id"`
	ProjectID int64  `json:"project_id"`
	Ref       string `json:"ref"`
	SHA       string `json:"sha"`
	Status    string `json:"status"`
	Source    string `json:"source"`
}

func (p controlCIPipeline) fact() contracts.RemotePipelineObservation {
	return contracts.RemotePipelineObservation{ProjectID: p.ProjectID, PipelineID: p.ID, Ref: p.Ref, CommitSha: p.SHA, Status: p.Status, Source: p.Source}
}

func controlCIPipelineMatches(p controlCIPipeline, params contracts.TriggerCIParameters) bool {
	return p.ProjectID == params.ProjectID && p.ID > 0 && p.Ref == params.Target.Ref && p.SHA == params.CommitSha && p.Source == "api" && contracts.Validate("RemotePipelineObservation", p.fact()) == nil
}

func controlCIPipelinePath(project, pipeline int64) string {
	return remoteProjectReadPath(project) + "/pipelines/" + strconv.FormatInt(pipeline, 10)
}

// Pagination is complete only after a short terminal page. Never follow a
// remote Link URL or let a truncated/ambiguous list authorize a new write.
func controlCIPageEnd(headers http.Header, count, page int) (bool, error) {
	if count > 100 {
		return false, fail("ci_list_incomplete", "流水线或作业分页超出固定大小")
	}
	next := headers.Get("X-Next-Page")
	if next != "" && (next != strconv.Itoa(page+1) || count != 100) {
		return false, fail("ci_list_incomplete", "流水线或作业分页标记不一致")
	}
	return count < 100 && next == "", nil
}

func (s *Service) controlCIRead(ctx context.Context, id string, x contracts.ExecutionRequest, path, readKind string, output any, normalize func() (controlRemoteFact, error), headers *http.Header) (int, error) {
	return s.controlRemoteRead(ctx, id, x, path, readKind, output, func(bounded context.Context) (int, error) {
		return s.gitlabJSON(bounded, http.MethodGet, path, nil, output, headers)
	}, normalize)
}

func (s *Service) controlCIVariables(ctx context.Context, id string, x contracts.ExecutionRequest, p controlCIPipeline, readKind string) (bool, error) {
	var rows []struct {
		Key   string `json:"key"`
		Value string `json:"value"`
	}
	matched := false
	_, err := s.controlCIRead(ctx, id, x, controlCIPipelinePath(p.ProjectID, p.ID)+"/variables", readKind, &rows, func() (controlRemoteFact, error) {
		if rows == nil || len(rows) > 100 {
			return controlRemoteFact{}, fail("ci_variables_invalid", "流水线变量缺少完整有界集合")
		}
		values := map[string]string{}
		for _, row := range rows {
			if row.Key != "AGENTSTUDIO_OPERATION_ID" && row.Key != "AGENTSTUDIO_REQUEST_HASH" {
				continue
			}
			if _, exists := values[row.Key]; exists {
				return controlRemoteFact{}, fail("ci_variables_invalid", "流水线归属变量重复")
			}
			values[row.Key] = row.Value
		}
		matched = values["AGENTSTUDIO_OPERATION_ID"] == x.OperationID && values["AGENTSTUDIO_REQUEST_HASH"] == x.RequestHash
		// Unrelated variables may be secrets. No response list is archived.
		if !matched {
			return controlRemoteFact{"unavailable", nil}, nil
		}
		return controlRemoteFact{"pipeline_variables", contracts.RemotePipelineVariables{ProjectID: p.ProjectID, PipelineID: p.ID, OperationID: x.OperationID, RequestHash: x.RequestHash}}, nil
	}, nil)
	return matched, err
}

func (s *Service) controlCIFind(ctx context.Context, id string, x contracts.ExecutionRequest, params contracts.TriggerCIParameters, readKind string) (*controlCIPipeline, error) {
	seen := map[int64]bool{}
	var found *controlCIPipeline
	for page := 1; page <= 16; page++ {
		var rows []controlCIPipeline
		var headers http.Header
		path := remoteProjectReadPath(params.ProjectID) + "/pipelines?ref=" + url.QueryEscape(params.Target.Ref) + "&sha=" + url.QueryEscape(params.CommitSha) + "&source=api&per_page=100&page=" + strconv.Itoa(page)
		_, err := s.controlCIRead(ctx, id, x, path, readKind, &rows, nil, &headers)
		if err != nil {
			return nil, err
		}
		if rows == nil {
			return nil, fail("ci_list_incomplete", "流水线列表缺少完整集合")
		}
		end, err := controlCIPageEnd(headers, len(rows), page)
		if err != nil {
			return nil, err
		}
		for _, p := range rows {
			if seen[p.ID] || !controlCIPipelineMatches(p, params) {
				return nil, fail("ci_list_invalid", "流水线列表身份重复或不符")
			}
			seen[p.ID] = true
			matches, err := s.controlCIVariables(ctx, id, x, p, readKind)
			if err != nil {
				return nil, err
			}
			if matches {
				if found != nil {
					return nil, fail("ci_pipeline_ambiguous", "多条流水线匹配原operation/request")
				}
				copy := p
				found = &copy
			}
		}
		if end {
			return found, nil
		}
	}
	return nil, fail("ci_list_incomplete", "流水线分页未在固定界限内结束")
}

func (s *Service) executeControlCI(ctx context.Context, id string, x contracts.ExecutionRequest, reconcile bool) (Run, error) {
	s.mu.Lock()
	key := "control-ci-execution:" + id
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
	_, op, err := s.controlGitLabRequest(r, x)
	if err != nil {
		return r, err
	}
	if op.Status == "verified" {
		return r, controlGitLabExistingResult(r, x)
	}
	_, raw, _ := scheduleAction(x.Action)
	var p contracts.TriggerCIParameters
	if err = contracts.DecodeVersion("TriggerCIParameters", x.ContractVersion, raw, &p); err != nil {
		return r, err
	}
	readKind := "ci_poll"
	if reconcile {
		readKind = "reconciliation_read"
		if op.Status != "sent" && op.Status != "unknown" {
			return r, fail("reconciliation_not_required", "CI核实需要原sent或unknown请求")
		}
		if r.ActiveStep != "" {
			return r, fail("execution_active", "活跃CI执行不能同时核实")
		}
		r, err = s.controlGitLabBlock(id, x, true, "ci_reconciliation_required", nil)
		if err != nil {
			return r, err
		}
	} else {
		if op.Status == "sent" || op.Status == "unknown" {
			return r, fail("reconciliation_required", "原CI操作只能只读核实，不能再POST")
		}
		if op.Status != "ready" || r.ActiveStep != "" || r.State != "DELIVER" || r.PauseRequested || r.CancelRequested {
			return r, fail("execution_stopped", "当前Run不允许触发CI")
		}
		if err = s.recheckControlApproval(r, time.Now().UTC()); err != nil {
			return r, err
		}
		next, e := NextControlAction(r, r.Control.Schedule)
		if e != nil {
			return r, e
		}
		if next.Execution == nil || !chainEqual(*next.Execution, x) || next.NeedsReconciliation {
			return r, fail("execution_binding_mismatch", "CI不是准确下一请求")
		}
		if err = s.observeControlCommit(ctx, id, x, p.ProjectID, p.Target, p.CommitSha, readKind); err != nil {
			return s.controlGitLabBlock(id, x, false, "ci_source_unavailable", err)
		}
		if err = s.controlCICodeMatches(id, x); err != nil {
			return s.controlGitLabBlock(id, x, false, "ci_source_mismatch", err)
		}
		found, e := s.controlCIFind(ctx, id, x, p, readKind)
		if e != nil {
			return s.controlGitLabBlock(id, x, false, "ci_preflight_unavailable", e)
		}
		if found != nil {
			return s.controlGitLabBlock(id, x, false, "ci_original_send_missing", fail("ci_history_incomplete", "发现原身份流水线但缺少原发送记录，禁止再次触发"))
		}
		var reserved contracts.BudgetReservation
		r, reserved, err = s.beginControlExecution(id, x)
		if err != nil {
			return r, err
		}
		bounded, cancel, e := controlGitLabDeadline(ctx, reserved)
		if e != nil {
			return s.controlGitLabBlock(id, x, true, "ci_write_unknown", e)
		}
		defer cancel()
		if err = s.controlGitLabDispatch(id, x); err != nil {
			return s.controlGitLabBlock(id, x, true, "ci_dispatch_stopped", err)
		}
		var created controlCIPipeline
		_, err = s.gitlab(bounded, http.MethodPost, remoteProjectReadPath(p.ProjectID)+"/pipeline", struct {
			Ref       string              `json:"ref"`
			Variables []map[string]string `json:"variables"`
		}{p.Target.Ref, []map[string]string{{"key": "AGENTSTUDIO_OPERATION_ID", "value": x.OperationID, "variable_type": "env_var"}, {"key": "AGENTSTUDIO_REQUEST_HASH", "value": x.RequestHash, "variable_type": "env_var"}}}, &created)
		if err != nil {
			return s.controlGitLabBlock(id, x, true, "ci_write_unknown", err)
		}
		if !controlCIPipelineMatches(created, p) {
			return s.controlGitLabBlock(id, x, true, "ci_write_identity_unknown", fail("ci_pipeline_invalid", "CI创建响应身份或源不符"))
		}
		if err = s.controlCIRecordID(id, x, created.ID); err != nil {
			return r, err
		}
	}
	r, err = s.Store.Get(id)
	if err != nil {
		return r, err
	}
	_, op, err = s.controlGitLabRequest(r, x)
	if err != nil {
		return r, err
	}
	pipeline := int64(0)
	if op.ExternalObjectID != nil {
		pipeline, err = strconv.ParseInt(*op.ExternalObjectID, 10, 64)
	}
	if err != nil || op.ExternalObjectID != nil && pipeline <= 0 {
		return s.controlGitLabBlock(id, x, true, "ci_history_incomplete", fail("ci_pipeline_invalid", "原流水线ID非法"))
	}
	if pipeline == 0 {
		found, e := s.controlCIFind(ctx, id, x, p, readKind)
		if e != nil {
			return s.controlGitLabBlock(id, x, true, "ci_reconciliation_unknown", e)
		}
		if found == nil {
			return s.controlGitLabBlock(id, x, true, "ci_reconciliation_unknown", fail("ci_pipeline_unknown", "原触发尚无唯一可归属流水线"))
		}
		pipeline = found.ID
		if err = s.controlCIRecordID(id, x, pipeline); err != nil {
			return r, err
		}
	}
	for {
		current, e := s.Store.Get(id)
		if e != nil {
			return current, e
		}
		if !reconcile && (current.PauseRequested || current.CancelRequested || terminal(current.State) || current.State == "PAUSED") {
			return s.controlGitLabBlock(id, x, true, "ci_wait_stopped", fail("execution_stopped", "用户控制停止CI等待"))
		}
		var observed controlCIPipeline
		_, err = s.controlCIRead(ctx, id, x, controlCIPipelinePath(p.ProjectID, pipeline), readKind, &observed, func() (controlRemoteFact, error) {
			if observed.ID != pipeline || !controlCIPipelineMatches(observed, p) {
				return controlRemoteFact{}, fail("ci_pipeline_invalid", "读取不是原准确流水线")
			}
			return controlRemoteFact{"pipeline", observed.fact()}, nil
		}, nil)
		if err != nil {
			return s.controlGitLabBlock(id, x, true, "ci_read_unknown", err)
		}
		matches, e := s.controlCIVariables(ctx, id, x, observed, readKind)
		if e != nil || !matches {
			return s.controlGitLabBlock(id, x, true, "ci_variables_mismatch", errors.Join(e, fail("ci_variables_mismatch", "原流水线归属变量不匹配")))
		}
		if observed.Status == "success" {
			break
		}
		if !chainContains([]string{"created", "waiting_for_resource", "preparing", "pending", "running", "scheduled"}, observed.Status) {
			return s.controlGitLabBlock(id, x, true, "ci_terminal_failed", fail("ci_failed", "原流水线未通过必需终态"))
		}
		if reconcile {
			return s.controlGitLabBlock(id, x, true, "ci_not_ready", fail("ci_not_ready", "原流水线尚未完成"))
		}
		heartbeat(ctx, "ci_poll")
		timer := time.NewTimer(2 * time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return s.controlGitLabBlock(id, x, true, "ci_wait_interrupted", ctx.Err())
		case <-timer.C:
		}
	}
	if err = s.observeControlCIJobs(ctx, id, x, p, pipeline, readKind); err != nil {
		return s.controlGitLabBlock(id, x, true, "ci_jobs_unverified", err)
	}
	// Re-observe the original remote code for terminal reconciliation too.
	if err = s.observeControlCommit(ctx, id, x, p.ProjectID, p.Target, p.CommitSha, readKind); err != nil {
		return s.controlGitLabBlock(id, x, true, "ci_source_unverified", err)
	}
	return s.completeControlCI(id, x, p, pipeline)
}

// The newly read remote code must still be the complete approved push before
// the POST boundary. A later completion refusal cannot undo an invalid trigger.
func (s *Service) controlCICodeMatches(id string, x contracts.ExecutionRequest) error {
	r, err := s.Store.Get(id)
	if err != nil {
		return err
	}
	var project *contracts.RemoteProjectObservation
	var commit *contracts.RemoteCommitObservation
	for _, read := range r.Control.RemoteObservations[x.ExecutionID] {
		switch read.Kind {
		case "project":
			var value contracts.RemoteProjectObservation
			if err = contracts.Decode("RemoteProjectObservation", read.Payload, &value); err != nil {
				return err
			}
			project = &value
		case "commit":
			var value contracts.RemoteCommitObservation
			if err = contracts.Decode("RemoteCommitObservation", read.Payload, &value); err != nil {
				return err
			}
			commit = &value
		}
	}
	for _, q := range r.Control.Audit.RemoteReceipts {
		if q.ActionType == "push_repository" && q.Status == "succeeded" && q.SourceVersion == x.SourceVersion && q.ApprovalID == x.Approval.ApprovalID && q.PlanHash == x.PlanHash {
			if ValidateControlRemoteReceipt(r, q) != nil || !chainEqual(project, q.Project) || !chainEqual(commit, q.Commit) {
				return fail("ci_source_mismatch", "CI发送前远端项目或完整提交已改变")
			}
			return nil
		}
	}
	return fail("ci_source_mismatch", "CI缺少完整原推送来源")
}

func (s *Service) controlCIRecordID(id string, x contracts.ExecutionRequest, pipeline int64) error {
	_, err := s.Store.Update(id, func(r *Run) error {
		_, op, err := s.controlGitLabRequest(*r, x)
		if err != nil {
			return err
		}
		if op.Status != "sent" && op.Status != "unknown" {
			return fail("operation_conflict", "原CI尚未发送")
		}
		value := strconv.FormatInt(pipeline, 10)
		if op.ExternalObjectID != nil && *op.ExternalObjectID != value {
			return fail("operation_conflict", "CI原ID不能替换")
		}
		op.ExternalObjectID = &value
		op.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
		return nil
	})
	return err
}

type controlCIJob struct {
	ID           int64  `json:"id"`
	Name         string `json:"name"`
	Ref          string `json:"ref"`
	Status       string `json:"status"`
	AllowFailure *bool  `json:"allow_failure"`
	Commit       struct {
		ID string `json:"id"`
	} `json:"commit"`
	Pipeline controlCIPipeline `json:"pipeline"`
}

func (s *Service) observeControlCIJobs(ctx context.Context, id string, x contracts.ExecutionRequest, p contracts.TriggerCIParameters, pipeline int64, readKind string) error {
	jobs := []contracts.CIJobEvidence{}
	seen := map[int64]bool{}
	names := map[string]bool{}
	complete := false
	base := controlCIPipelinePath(p.ProjectID, pipeline) + "/jobs"
	for page := 1; page <= 16; page++ {
		var rows []controlCIJob
		var headers http.Header
		_, err := s.controlCIRead(ctx, id, x, base+"?per_page=100&page="+strconv.Itoa(page), readKind, &rows, func() (controlRemoteFact, error) {
			if rows == nil {
				return controlRemoteFact{}, fail("ci_jobs_invalid", "job列表缺少完整集合")
			}
			end, e := controlCIPageEnd(headers, len(rows), page)
			if e != nil {
				return controlRemoteFact{}, e
			}
			for _, job := range rows {
				if seen[job.ID] || job.ID <= 0 {
					return controlRemoteFact{}, fail("ci_jobs_invalid", "job身份缺失或重复")
				}
				seen[job.ID] = true
				if job.Name != "test" && job.Name != "build" {
					continue
				}
				if names[job.Name] || job.AllowFailure == nil || *job.AllowFailure || job.Status != "success" || job.Ref != p.Target.Ref || job.Commit.ID != p.CommitSha || job.Pipeline.ID != pipeline || job.Pipeline.ProjectID != p.ProjectID || job.Pipeline.Ref != p.Target.Ref || job.Pipeline.SHA != p.CommitSha {
					return controlRemoteFact{}, fail("ci_jobs_invalid", "必需job重复、未成功或来源不符")
				}
				names[job.Name] = true
				jobs = append(jobs, contracts.CIJobEvidence{ProjectID: p.ProjectID, PipelineID: pipeline, JobID: job.ID, Name: job.Name, Ref: job.Ref, CommitSha: job.Commit.ID, Status: job.Status, AllowFailure: *job.AllowFailure, Artifacts: []contracts.RemoteArtifactEvidence{}})
			}
			if !end {
				return controlRemoteFact{"unavailable", nil}, nil
			}
			complete = true
			// The final page joins earlier actual page reads; all pages retain
			// their own consumed reservations. No guessed/truncated collection.
			return controlRemoteFact{"jobs", contracts.RemoteJobListObservation{ProjectID: p.ProjectID, PipelineID: pipeline, Jobs: jobs}}, nil
		}, &headers)
		if err != nil {
			return err
		}
		if complete {
			break
		}
	}
	if !complete || !names["test"] || !names["build"] {
		return fail("ci_jobs_missing", "缺少完整唯一test/build job")
	}
	for _, job := range jobs {
		path := "coverage.out"
		if job.Name == "build" {
			path = "bin/server"
		}
		if err := s.controlCIDownload(ctx, id, x, p.ProjectID, pipeline, job.JobID, path, readKind); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) completeControlCI(id string, x contracts.ExecutionRequest, p contracts.TriggerCIParameters, pipeline int64) (Run, error) {
	return s.Store.Update(id, func(r *Run) error {
		_, op, err := s.controlGitLabRequest(*r, x)
		if err != nil {
			return err
		}
		if op.Status == "verified" {
			return controlGitLabExistingResult(*r, x)
		}
		if op.Status != "sent" && op.Status != "unknown" {
			return fail("operation_conflict", "原CI未发送")
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
		if q.Pipeline == nil || q.Pipeline.PipelineID != pipeline {
			return fail("ci_pipeline_invalid", "完成回执缺少原流水线")
		}
		manifest := contracts.ObjectReference(q.ManifestRef)
		result := CanonicalActionResult{RunID: x.RunID, SourceVersion: x.SourceVersion, DefinitionSnapshotHash: x.DefinitionSnapshotHash, PlanHash: x.PlanHash, ApprovalID: q.ApprovalID, ApprovalHash: q.ApprovalHash, ProposalSnapshotHash: q.ProposalSnapshotHash, ExecutionID: x.ExecutionID, OperationID: x.OperationID, AttemptID: parent.AttemptID, RequestHash: x.RequestHash, TargetHash: op.TargetHash, ActionType: "trigger_ci", ManifestRef: &manifest, ObservedTarget: &p.Target, ProjectID: p.ProjectID, CommitSHA: q.Commit.CommitSha, CIConfigSHA256: *q.CIConfigSHA256, PipelineID: pipeline, PipelineStatus: q.Pipeline.Status, CompletedAt: q.CompletedAt}
		result.ResultHash, err = HashCanonicalActionResult(result)
		if err != nil {
			return err
		}
		r.Control.Schedule.Results = append(r.Control.Schedule.Results, result)
		value := strconv.FormatInt(pipeline, 10)
		op.Status, op.ExternalObjectID, op.UpdatedAt = "verified", &value, q.CompletedAt
		if r.ActiveStep == x.ExecutionID {
			r.ActiveStep = ""
		}
		syncBudgetProjection(r)
		return nil
	})
}

// Keep error bodies/unknown variable lists away from persistent model context.
func controlCIArtifactIdentity(x contracts.ExecutionRequest, pipeline, job int64, path string) (string, string) {
	hash := contracts.BytesHash([]byte(fmt.Sprintf("%s:%d:%d:%s", x.OperationID, pipeline, job, path)))
	id := "ci_artifact_" + hash[:40]
	return id, ".control-ci-artifacts/" + id + ".bin"
}
