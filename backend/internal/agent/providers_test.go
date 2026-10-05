package agent

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// This verifies REST reconciliation against a contract server; it is not
// evidence of a live GitLab operation.
func TestGitLabReconcilesOriginalCommitAndPipeline(t *testing.T) {
	s := testService(t)
	var r Run
	created, committed := false, false
	createCount, commitCount, triggerCount := 0, 0, 0
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Header.Get("PRIVATE-TOKEN") != "test-token" {
			w.WriteHeader(401)
			return
		}
		path := req.URL.Path
		reply := func(value any) { _ = json.NewEncoder(w).Encode(value) }
		project := map[string]any{"id": 7, "web_url": "https://gitlab.example/tester/remote-service", "description": "[agentstudio:" + r.ID + "]", "path_with_namespace": "tester/remote-service", "visibility": "private"}
		pipeline := map[string]any{"id": 8, "web_url": "https://gitlab.example/pipelines/8", "status": "success", "sha": "sha-confirmed", "ref": "main"}
		switch {
		case path == "/api/v4/namespaces/1":
			reply(map[string]string{"full_path": "tester"})
		case path == "/api/v4/projects" && req.Method == "POST":
			createCount++
			created = true
			reply(project)
		case path == "/api/v4/projects/tester/remote-service" || path == "/api/v4/projects/7":
			if !created {
				w.WriteHeader(404)
			} else {
				reply(project)
			}
		case path == "/api/v4/projects/7/repository/branches/main":
			if !committed {
				w.WriteHeader(404)
			} else {
				reply(map[string]any{"name": "main", "commit": map[string]string{"id": "sha-confirmed"}})
			}
		case path == "/api/v4/projects/7/repository/commits" && req.Method == "POST":
			commitCount++
			committed = true
			reply(map[string]string{"id": "sha-confirmed"})
		case path == "/api/v4/projects/7/repository/commits":
			if !committed {
				reply([]any{})
			} else {
				reply([]map[string]string{{"id": "sha-confirmed", "message": "Agent Studio " + r.ID + " " + r.AcceptedProposal}})
			}
		case strings.HasPrefix(path, "/api/v4/projects/7/repository/files/"):
			if !committed {
				w.WriteHeader(404)
				return
			}
			file := strings.TrimPrefix(path, "/api/v4/projects/7/repository/files/")
			b, e := os.ReadFile(filepath.Join(r.Workspace, file))
			if e != nil {
				t.Error(e)
				w.WriteHeader(404)
				return
			}
			reply(map[string]string{"encoding": "base64", "content": base64.StdEncoding.EncodeToString(b)})
		case path == "/api/v4/projects/7/repository/tree":
			files, _ := ListArtifacts(r.Workspace)
			tree := []map[string]string{}
			for _, f := range files {
				tree = append(tree, map[string]string{"type": "blob", "path": f.Path})
			}
			reply(tree)
		case path == "/api/v4/projects/7/pipelines":
			reply([]any{pipeline})
		case path == "/api/v4/projects/7/pipelines/8":
			reply(pipeline)
		case path == "/api/v4/projects/7/pipelines/8/jobs":
			reply([]map[string]any{{"id": 10, "name": "test", "status": "success", "commit": map[string]string{"id": "sha-confirmed"}, "artifacts_file": map[string]string{"filename": "artifacts.zip"}}, {"id": 11, "name": "build", "status": "success", "commit": map[string]string{"id": "sha-confirmed"}, "artifacts_file": map[string]string{"filename": "artifacts.zip"}}})
		case path == "/api/v4/projects/7/jobs/10/artifacts/coverage.out":
			_, _ = w.Write([]byte("mode: atomic\nremote-service/test.go:1.1,2.1 1 1\n"))
		case path == "/api/v4/projects/7/jobs/11/artifacts/bin/server":
			_, _ = w.Write([]byte{0x7f, 'E', 'L', 'F', 1, 2, 3, 4})
		case path == "/api/v4/projects/7/pipeline":
			triggerCount++
			w.WriteHeader(500)
		default:
			t.Errorf("unexpected GitLab %s %s", req.Method, req.URL)
			w.WriteHeader(404)
		}
	}))
	defer api.Close()
	s.Config.GitLabURL = api.URL
	s.Config.GitLabToken = "test-token"
	s.Config.GitLabNamespace = "1"
	r = planRun(t, s, "remote", "创建 remote-service，GitLab")
	if _, e := s.ProcessCommand(context.Background(), approvalCommand(r, "approve_remote")); e != nil {
		t.Fatal(e)
	}
	for _, step := range []string{"scaffold", "skills", "verify"} {
		if _, e := s.ExecuteStep(context.Background(), StepInput{RunID: r.ID, Step: step}); e != nil {
			t.Fatal(e)
		}
	}
	r, _ = s.Store.Get(r.ID)
	for i := 0; i < 2; i++ {
		if e := s.PublishGitLab(context.Background(), r); e != nil {
			t.Fatal(e)
		}
		if e := s.VerifyGitLabPipeline(context.Background(), r.ID); e != nil {
			t.Fatal(e)
		}
	}
	if createCount != 1 || commitCount != 1 || triggerCount != 0 {
		t.Fatalf("duplicate writes %d/%d/%d", createCount, commitCount, triggerCount)
	}
	current, _ := s.Store.Get(r.ID)
	for _, check := range current.RequiredChecks {
		if strings.HasPrefix(check.Name, "remote_") || check.Name == "gitlab_ci" {
			if !check.Passed {
				t.Fatal("missing remote gate", check)
			}
		}
	}
	b, _ := os.ReadFile(filepath.Join(s.Config.DataDir, "runs", r.ID+".json"))
	if strings.Contains(string(b), "test-token") {
		t.Fatal("secret persisted")
	}
}
func TestTransientGitLabReadRemainsRetryable(t *testing.T) {
	s := testService(t)
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(503) }))
	defer api.Close()
	s.Config.GitLabURL = api.URL
	s.Config.GitLabToken = "test-token"
	s.Config.GitLabNamespace = "1"
	r := planRun(t, s, "retry_read", "创建 read-service，GitLab")
	if _, e := s.ProcessCommand(context.Background(), approvalCommand(r, "approve_read")); e != nil {
		t.Fatal(e)
	}
	for _, step := range []string{"scaffold", "skills", "verify"} {
		if _, e := s.ExecuteStep(context.Background(), StepInput{RunID: r.ID, Step: step}); e != nil {
			t.Fatal(e)
		}
	}
	_, e := s.ExecuteStep(context.Background(), StepInput{RunID: r.ID, Step: "repo"})
	var infrastructure *InfrastructureError
	if !errors.As(e, &infrastructure) {
		t.Fatalf("read not retryable: %v", e)
	}
}

func TestModelOutputIsSchemaChecked(t *testing.T) {
	s := testService(t)
	var r Run
	valid := defaultPlan()
	valid.ProjectName = "model-service"
	bad := false
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path != "/v1/chat/completions" || req.Header.Get("Authorization") != "Bearer trusted-key" {
			t.Error("model request boundary mismatch")
		}
		p := valid
		if bad {
			p.ProjectName = "../../escape"
		}
		result := fixtureAdvice(t, r, p)
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []map[string]any{{"message": map[string]string{"content": string(result)}}}})
	}))
	defer api.Close()
	s.Config.AIEndpoint = api.URL + "/v1"
	s.Config.AIKey = "trusted-key"
	s.Config.AIModel = "configured-model"
	var err error
	r, err = s.CreateWithOptions(context.Background(), CreateOptions{ID: "model", DecisionMode: "model"})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, e := s.ModelPlan(context.Background(), r, "continue"); e != nil {
		t.Fatal(e)
	}
	bad = true
	if _, _, e := s.ModelPlan(context.Background(), r, "unsafe suggestion"); e == nil {
		t.Fatal("invalid model plan was accepted")
	}
	if strings.Contains(fmt.Sprint(r.Messages), "trusted-key") {
		t.Fatal("model credential entered conversation")
	}
}

func TestModelContextBudgetKeepsCurrentIdentityAndMessage(t *testing.T) {
	s := testService(t)
	r, err := s.Create(context.Background(), "run_budget", "", "")
	if err != nil {
		t.Fatal(err)
	}
	r.State = "PLAN"
	r.Version = 9
	r.SourceVersion = 9
	r.Context.Summary = strings.Repeat("早期说明", 2000)
	r.Messages = []Message{}
	r.Plan.ProjectName = "budget-service"
	r.Plan.Description = strings.Repeat("描述", 4000)
	for i := 0; i < 10; i++ {
		r.Messages = append(r.Messages, Message{Role: "user", Kind: "text", Content: strings.Repeat("历史内容", 1000)})
	}
	request, e := buildModelRequest(r, "当前完整需求：保留 JWT 认证", "test-model")
	if e != nil {
		t.Fatal(e)
	}
	if len(request)+1800+128 > 8000 {
		t.Fatalf("context budget exceeded: %d", len(request))
	}
	if !strings.Contains(string(request), "run_budget") || !strings.Contains(string(request), "source_version 9") || !strings.Contains(string(request), "当前完整需求：保留 JWT 认证") || !strings.Contains(string(request), "budget-service") {
		t.Fatal("necessary identity or current message lost to trimming")
	}
	if _, e = buildModelRequest(r, strings.Repeat("大", 5000), "test-model"); e == nil {
		t.Fatal("oversized current message silently accepted")
	}
	if len(r.Messages) != 10 || len(r.Plan.Description) < 10000 {
		t.Fatal("budget trimming changed authoritative source")
	}
}
