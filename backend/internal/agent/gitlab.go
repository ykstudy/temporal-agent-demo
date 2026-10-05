package agent

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"agentstudio/internal/contracts"
)

type UnknownResultError struct{ Reason string }

func (e *UnknownResultError) Error() string { return e.Reason }

type InfrastructureError struct{ Reason string }

func (e *InfrastructureError) Error() string { return e.Reason }

type gitlabProject struct {
	ID            int    `json:"id"`
	WebURL        string `json:"web_url"`
	Description   string `json:"description"`
	Path          string `json:"path_with_namespace"`
	DefaultBranch string `json:"default_branch"`
	Visibility    string `json:"visibility"`
}
type gitlabPipeline struct {
	ID     int    `json:"id"`
	WebURL string `json:"web_url"`
	Status string `json:"status"`
	SHA    string `json:"sha"`
	Ref    string `json:"ref"`
}

func (s *Service) gitlab(ctx context.Context, method, path string, input, output any) (int, error) {
	return s.gitlabJSON(ctx, method, path, input, output, nil)
}

func (s *Service) gitlabJSON(ctx context.Context, method, path string, input, output any, headers *http.Header) (int, error) {
	var body io.Reader
	if input != nil {
		b, e := json.Marshal(input)
		if e != nil {
			return 0, e
		}
		body = bytes.NewReader(b)
	}
	base := strings.TrimRight(s.Config.GitLabURL, "/")
	req, e := http.NewRequestWithContext(ctx, method, base+"/api/v4"+path, body)
	if e != nil {
		return 0, e
	}
	req.Header.Set("PRIVATE-TOKEN", s.Config.GitLabToken)
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 40 * time.Second, CheckRedirect: func(req *http.Request, via []*http.Request) error { return http.ErrUseLastResponse }}
	resp, e := client.Do(req)
	if e != nil {
		if ctx.Err() != nil {
			return 0, ctx.Err()
		}
		return 0, &InfrastructureError{Reason: fmt.Sprintf("GitLab request failed (%s %s)", method, path)}
	}
	defer resp.Body.Close()
	if headers != nil {
		*headers = resp.Header.Clone()
	}
	b, e := io.ReadAll(io.LimitReader(resp.Body, (2<<20)+1))
	if e != nil {
		if ctx.Err() != nil {
			return resp.StatusCode, ctx.Err()
		}
		return resp.StatusCode, &InfrastructureError{Reason: fmt.Sprintf("GitLab response interrupted (%s %s)", method, path)}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if resp.StatusCode == 408 || resp.StatusCode == 429 || resp.StatusCode >= 500 {
			return resp.StatusCode, &InfrastructureError{Reason: fmt.Sprintf("GitLab %s %s returned HTTP %d", method, path, resp.StatusCode)}
		}
		return resp.StatusCode, fmt.Errorf("GitLab %s %s returned HTTP %d", method, path, resp.StatusCode)
	}
	if len(b) > 2<<20 {
		return resp.StatusCode, fmt.Errorf("GitLab response exceeds the bounded JSON size")
	}
	if output != nil && len(b) > 0 {
		if _, e = contracts.ParseJSON(b); e != nil {
			return resp.StatusCode, fmt.Errorf("invalid GitLab JSON response")
		}
		if e = json.Unmarshal(b, output); e != nil {
			return resp.StatusCode, fmt.Errorf("invalid GitLab response")
		}
	}
	return resp.StatusCode, nil
}

// Writes have a stable business identity. A sent/unknown record permits only
// reconciliation: absence from an eventually consistent list is not evidence
// that the original write failed.
func (s *Service) operation(r Run, kind string) (OperationRecord, error) {
	id := operationID(r, kind)
	fingerprint := digest(struct {
		Target   ExternalTarget
		Manifest string
		Kind     string
	}{r.Proposal.Target, r.Manifest.Hash, kind})
	var result OperationRecord
	_, e := s.Store.Update(r.ID, func(current *Run) error {
		if old, ok := current.Operations[id]; ok {
			if old.Fingerprint != fingerprint {
				return fail("operation_conflict", "外部操作身份绑定不同内容")
			}
			result = old
			return nil
		}
		result = OperationRecord{ID: id, Kind: kind, SourceVersion: r.SourceVersion, Fingerprint: fingerprint, Status: "ready"}
		current.Operations[id] = result
		return nil
	})
	return result, e
}
func (s *Service) setOperation(id string, op OperationRecord, status, external string) error {
	_, e := s.Store.Update(id, func(r *Run) error {
		op.Status = status
		op.ExternalID = external
		r.Operations[op.ID] = op
		history(r, "external_"+status, op.Kind+" "+op.ID)
		return nil
	})
	return e
}
func (s *Service) PublishGitLab(ctx context.Context, r Run) error {
	var e error
	r, e = s.Store.Get(r.ID)
	if e != nil {
		return e
	}
	if e = s.verifyApproval(r, false); e != nil {
		return e
	}
	if e = s.verifyManifest(r); e != nil {
		return e
	}
	namespace, e := strconv.Atoi(r.Proposal.Target.NamespaceID)
	if e != nil || namespace <= 0 {
		return fmt.Errorf("GITLAB_NAMESPACE_ID must name an explicit positive namespace")
	}
	var ns struct {
		FullPath string `json:"full_path"`
	}
	if _, e = s.gitlab(ctx, "GET", "/namespaces/"+strconv.Itoa(namespace), nil, &ns); e != nil {
		return e
	}
	if ns.FullPath == "" {
		return fmt.Errorf("GitLab namespace lookup returned no path")
	}
	path := ns.FullPath + "/" + r.Plan.ProjectName
	marker := "[agentstudio:" + r.ID + "]"
	var project gitlabProject
	op, e := s.operation(r, "project")
	if e != nil {
		return e
	}
	lookup := "/projects/" + url.PathEscape(path)
	if r.RepoProjectID > 0 {
		lookup = "/projects/" + strconv.Itoa(r.RepoProjectID)
	}
	status, e := s.gitlab(ctx, "GET", lookup, nil, &project)
	if e != nil && status != 404 {
		return e
	}
	if status == 404 {
		if op.Status != "ready" {
			return &UnknownResultError{Reason: "原项目创建调用尚不可见，继续只读核实；禁止再次POST"}
		}
		if e = s.setOperation(r.ID, op, "sent", ""); e != nil {
			return e
		}
		_, e = s.gitlab(ctx, "POST", "/projects", map[string]any{"name": r.Plan.ProjectName, "path": r.Plan.ProjectName, "namespace_id": namespace, "description": r.Plan.Description + "\n" + marker, "visibility": "private", "initialize_with_readme": false}, &project)
		if e != nil {
			_ = s.setOperation(r.ID, op, "unknown", "")
			if _, qe := s.gitlab(ctx, "GET", "/projects/"+url.PathEscape(path), nil, &project); qe != nil {
				return &UnknownResultError{Reason: "项目创建结果未知，按原namespace/path只读核实"}
			}
		}
	}
	if project.ID <= 0 || project.WebURL == "" || project.Path != path || project.Visibility != "private" || !strings.Contains(project.Description, marker) {
		return fmt.Errorf("GitLab项目身份、私有性或Run所有权不匹配，拒绝写入")
	}
	if e = s.setOperation(r.ID, op, "verified", strconv.Itoa(project.ID)); e != nil {
		return e
	}
	_, e = s.Store.Update(r.ID, func(current *Run) error {
		current.RepoProjectID = project.ID
		current.RepoURL = project.WebURL
		recordChecks(current, []Check{{Name: "remote_repository", Passed: true, Detail: "精确namespace/path、Run所有权与private可见性已核实"}})
		return nil
	})
	if e != nil {
		return e
	}
	message := "Agent Studio " + r.ID + " " + r.AcceptedProposal
	commitOp, e := s.operation(r, "commit")
	if e != nil {
		return e
	}
	var commits []struct {
		ID      string `json:"id"`
		Message string `json:"message"`
	}
	status, e = s.gitlab(ctx, "GET", fmt.Sprintf("/projects/%d/repository/commits?ref_name=main&per_page=100", project.ID), nil, &commits)
	if e != nil && status != 404 {
		return e
	}
	for _, c := range commits {
		if strings.TrimSpace(c.Message) == message {
			return s.verifyRemoteCommit(ctx, r, project.ID, c.ID, commitOp)
		}
	}
	if commitOp.Status != "ready" {
		return &UnknownResultError{Reason: "原提交调用尚不可见，继续只读核实；禁止再次POST"}
	}
	var branch struct {
		Commit struct {
			ID string `json:"id"`
		} `json:"commit"`
	}
	status, e = s.gitlab(ctx, "GET", fmt.Sprintf("/projects/%d/repository/branches/main", project.ID), nil, &branch)
	if e == nil {
		return fail("branch_changed", "目标main已有不属于获准操作的提交，拒绝覆盖")
	}
	if status != 404 {
		return e
	}
	files, e := ListArtifacts(r.Workspace)
	if e != nil {
		return e
	}
	actions := []map[string]any{}
	for _, f := range files {
		b, e := os.ReadFile(filepath.Join(r.Workspace, f.Path))
		if e != nil {
			return e
		}
		actions = append(actions, map[string]any{"action": "create", "file_path": f.Path, "encoding": "base64", "content": base64.StdEncoding.EncodeToString(b)})
	}
	if e = s.setOperation(r.ID, commitOp, "sent", ""); e != nil {
		return e
	}
	var commit struct {
		ID string `json:"id"`
	}
	_, e = s.gitlab(ctx, "POST", fmt.Sprintf("/projects/%d/repository/commits", project.ID), map[string]any{"branch": "main", "commit_message": message, "actions": actions}, &commit)
	if e != nil || commit.ID == "" {
		_ = s.setOperation(r.ID, commitOp, "unknown", "")
		var found []struct {
			ID      string `json:"id"`
			Message string `json:"message"`
		}
		if _, qe := s.gitlab(ctx, "GET", fmt.Sprintf("/projects/%d/repository/commits?ref_name=main&per_page=100", project.ID), nil, &found); qe == nil {
			for _, c := range found {
				if strings.TrimSpace(c.Message) == message {
					return s.verifyRemoteCommit(ctx, r, project.ID, c.ID, commitOp)
				}
			}
		}
		return &UnknownResultError{Reason: "提交结果未知；保留原调用身份，只读核实提交与文件"}
	}
	return s.verifyRemoteCommit(ctx, r, project.ID, commit.ID, commitOp)
}
func (s *Service) verifyRemoteCommit(ctx context.Context, r Run, project int, sha string, op OperationRecord) error {
	if sha == "" {
		return &UnknownResultError{Reason: "提交SHA缺失"}
	}
	files, e := ListArtifacts(r.Workspace)
	if e != nil {
		return e
	}
	expected := map[string]bool{}
	for _, f := range files {
		expected[f.Path] = true
		var remote struct {
			Encoding string `json:"encoding"`
			Content  string `json:"content"`
		}
		if _, e = s.gitlab(ctx, "GET", fmt.Sprintf("/projects/%d/repository/files/%s?ref=%s", project, url.PathEscape(f.Path), url.QueryEscape(sha)), nil, &remote); e != nil {
			return e
		}
		if remote.Encoding != "base64" {
			return fmt.Errorf("远程文件encoding无效")
		}
		b, e := base64.StdEncoding.DecodeString(strings.ReplaceAll(remote.Content, "\n", ""))
		if e != nil || bytesDigest(b) != f.SHA256 {
			return fmt.Errorf("远程提交文件不匹配批准manifest: %s", f.Path)
		}
	}
	seen := map[string]bool{}
	for page := 1; page <= 2; page++ {
		var tree []struct {
			Path string `json:"path"`
			Type string `json:"type"`
		}
		if _, e = s.gitlab(ctx, "GET", fmt.Sprintf("/projects/%d/repository/tree?ref=%s&recursive=true&per_page=100&page=%d", project, url.QueryEscape(sha), page), nil, &tree); e != nil {
			return e
		}
		for _, item := range tree {
			if item.Type == "blob" {
				if !expected[item.Path] {
					return fmt.Errorf("远程提交包含未批准文件: %s", item.Path)
				}
				seen[item.Path] = true
			}
		}
		if len(tree) < 100 {
			break
		}
	}
	if len(seen) != len(expected) {
		return fmt.Errorf("远程提交文件集合不完整")
	}
	if e = s.setOperation(r.ID, op, "verified", sha); e != nil {
		return e
	}
	_, e = s.Store.Update(r.ID, func(current *Run) error {
		current.CommitSHA = sha
		recordChecks(current, []Check{{Name: "remote_commit", Passed: true, Detail: "SHA " + sha + " 文件集合和SHA256逐项核实"}})
		history(current, "commit_verified", sha)
		return nil
	})
	return e
}
func (s *Service) VerifyGitLabPipeline(ctx context.Context, id string) error {
	r, e := s.Store.Get(id)
	if e != nil {
		return e
	}
	if e = s.verifyApproval(r, false); e != nil {
		return e
	}
	if e = s.verifyManifest(r); e != nil {
		return e
	}
	if r.RepoProjectID <= 0 || r.CommitSHA == "" {
		return fmt.Errorf("GitLab repository/commit not verified")
	}
	op, e := s.operation(r, "pipeline")
	if e != nil {
		return e
	}
	var pipeline gitlabPipeline
	exact := func(p gitlabPipeline) bool { return p.ID > 0 && p.SHA == r.CommitSHA && p.Ref == "main" }
	if r.PipelineID > 0 {
		_, e = s.gitlab(ctx, "GET", fmt.Sprintf("/projects/%d/pipelines/%d", r.RepoProjectID, r.PipelineID), nil, &pipeline)
		if e != nil {
			return e
		}
	} else {
		var existing []gitlabPipeline
		if _, e = s.gitlab(ctx, "GET", fmt.Sprintf("/projects/%d/pipelines?sha=%s&ref=main", r.RepoProjectID, url.QueryEscape(r.CommitSHA)), nil, &existing); e != nil {
			return e
		}
		for _, p := range existing {
			if exact(p) {
				pipeline = p
				break
			}
		}
		if pipeline.ID == 0 {
			if op.Status != "ready" {
				return &UnknownResultError{Reason: "原pipeline触发尚不可见，保持BLOCKED，只读核实不再次POST"}
			}
			if e = s.setOperation(id, op, "sent", ""); e != nil {
				return e
			}
			_, e = s.gitlab(ctx, "POST", fmt.Sprintf("/projects/%d/pipeline", r.RepoProjectID), map[string]any{"ref": "main", "variables": []map[string]string{{"key": "AGENT_OPERATION_ID", "value": op.ID}}}, &pipeline)
			if e != nil || !exact(pipeline) {
				_ = s.setOperation(id, op, "unknown", "")
				if _, qe := s.gitlab(ctx, "GET", fmt.Sprintf("/projects/%d/pipelines?sha=%s&ref=main", r.RepoProjectID, url.QueryEscape(r.CommitSHA)), nil, &existing); qe == nil {
					for _, p := range existing {
						if exact(p) {
							pipeline = p
							break
						}
					}
				}
				if !exact(pipeline) {
					return &UnknownResultError{Reason: "pipeline触发结果未知；按原项目/SHA/ref核实，禁止重复触发"}
				}
			}
		}
	}
	if !exact(pipeline) {
		return fmt.Errorf("pipeline身份不匹配确认提交")
	}
	if e = s.setOperation(id, op, "verified", strconv.Itoa(pipeline.ID)); e != nil {
		return e
	}
	_, e = s.Store.Update(id, func(current *Run) error {
		current.PipelineID = pipeline.ID
		current.PipelineURL = pipeline.WebURL
		history(current, "pipeline_recorded", fmt.Sprintf("pipeline %d", pipeline.ID))
		return nil
	})
	if e != nil {
		return e
	}
	deadline := time.NewTimer(8 * time.Minute)
	defer deadline.Stop()
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()
	for {
		if !exact(pipeline) {
			return fmt.Errorf("pipeline SHA/ref changed")
		}
		heartbeat(ctx, fmt.Sprintf("pipeline %d: %s", pipeline.ID, pipeline.Status))
		switch pipeline.Status {
		case "success":
			return s.verifyCIJobs(ctx, r, pipeline)
		case "failed", "canceled", "skipped":
			return fmt.Errorf("pipeline %d status=%s", pipeline.ID, pipeline.Status)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			return &UnknownResultError{Reason: "已登记pipeline未结束，恢复只查询同一ID"}
		case <-ticker.C:
		}
		if _, e = s.gitlab(ctx, "GET", fmt.Sprintf("/projects/%d/pipelines/%d", r.RepoProjectID, pipeline.ID), nil, &pipeline); e != nil {
			return e
		}
	}
}

type gitlabJob struct {
	ID     int    `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"`
	Commit struct {
		ID string `json:"id"`
	} `json:"commit"`
	Artifact struct {
		Filename string `json:"filename"`
	} `json:"artifacts_file"`
}

func (s *Service) gitlabArtifact(ctx context.Context, path string) ([]byte, error) {
	req, e := http.NewRequestWithContext(ctx, "GET", strings.TrimRight(s.Config.GitLabURL, "/")+"/api/v4"+path, nil)
	if e != nil {
		return nil, e
	}
	req.Header.Set("PRIVATE-TOKEN", s.Config.GitLabToken)
	client := http.Client{Timeout: 40 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, e := client.Do(req)
	if e != nil {
		return nil, &InfrastructureError{Reason: "CI artifact request interrupted"}
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		if resp.StatusCode == 408 || resp.StatusCode == 429 || resp.StatusCode >= 500 {
			return nil, &InfrastructureError{Reason: fmt.Sprintf("CI artifact HTTP %d", resp.StatusCode)}
		}
		return nil, fmt.Errorf("CI artifact HTTP %d", resp.StatusCode)
	}
	b, e := io.ReadAll(io.LimitReader(resp.Body, (16<<20)+1))
	if e != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, &InfrastructureError{Reason: "CI artifact response interrupted"}
	}
	if len(b) == 0 || len(b) > 16<<20 {
		return nil, fmt.Errorf("CI artifact empty or too large")
	}
	return b, nil
}
func (s *Service) verifyCIJobs(ctx context.Context, r Run, pipeline gitlabPipeline) error {
	var jobs []gitlabJob
	if _, e := s.gitlab(ctx, "GET", fmt.Sprintf("/projects/%d/pipelines/%d/jobs?per_page=100", r.RepoProjectID, pipeline.ID), nil, &jobs); e != nil {
		return e
	}
	required := map[string]gitlabJob{}
	for _, job := range jobs {
		if job.Name == "test" || job.Name == "build" {
			if _, exists := required[job.Name]; exists {
				return fmt.Errorf("required CI job ambiguous")
			}
			if job.ID <= 0 || job.Status != "success" || job.Commit.ID != r.CommitSHA || job.Artifact.Filename == "" {
				return fmt.Errorf("CI required job %s missing/scope mismatch", job.Name)
			}
			required[job.Name] = job
		}
	}
	if len(required) != 2 {
		return fmt.Errorf("CI required test/build jobs missing")
	}
	coverage, e := s.gitlabArtifact(ctx, fmt.Sprintf("/projects/%d/jobs/%d/artifacts/coverage.out", r.RepoProjectID, required["test"].ID))
	if e != nil {
		return e
	}
	if !(bytes.HasPrefix(coverage, []byte("mode: set\n")) || bytes.HasPrefix(coverage, []byte("mode: count\n")) || bytes.HasPrefix(coverage, []byte("mode: atomic\n"))) {
		return fmt.Errorf("CI coverage report invalid")
	}
	binary, e := s.gitlabArtifact(ctx, fmt.Sprintf("/projects/%d/jobs/%d/artifacts/bin/server", r.RepoProjectID, required["build"].ID))
	if e != nil {
		return e
	}
	if len(binary) < 4 || !(bytes.Equal(binary[:4], []byte{0x7f, 'E', 'L', 'F'}) || bytes.Equal(binary[:4], []byte{0xcf, 0xfa, 0xed, 0xfe}) || bytes.Equal(binary[:4], []byte{0xfe, 0xed, 0xfa, 0xcf})) {
		return fmt.Errorf("CI build artifact lacks supported executable header")
	}
	_, e = s.Store.Update(r.ID, func(current *Run) error {
		recordChecks(current, []Check{{Name: "gitlab_ci", Passed: true, Detail: fmt.Sprintf("pipeline %d exact SHA/ref; test/build jobs success; coverage SHA256=%s binary SHA256=%s", pipeline.ID, bytesDigest(coverage), bytesDigest(binary))}})
		return nil
	})
	return e
}
