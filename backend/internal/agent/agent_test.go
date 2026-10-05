package agent

import (
	"archive/zip"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func testService(t *testing.T) *Service {
	t.Helper()
	s, e := New(Config{DataDir: t.TempDir(), Mode: "local", GitLabURL: "https://gitlab.com"})
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(s.Close)
	return s
}
func approvalCommand(r Run, id string) Command {
	return Command{ID: id, RunID: r.ID, Action: "accept", ProposalID: r.Proposal.ID, SourceVersion: r.SourceVersion, ProposalVersion: r.Proposal.ProposalVersion, SnapshotHash: r.Proposal.SnapshotHash}
}
func planRun(t *testing.T, s *Service, name, text string) Run {
	t.Helper()
	r, e := s.Create(context.Background(), "run_"+name, "", "")
	if e != nil {
		t.Fatal(e)
	}
	result, e := s.ProcessCommand(context.Background(), Command{ID: "msg_" + name, RunID: r.ID, Content: text})
	if e != nil {
		t.Fatal(e)
	}
	return result.Run
}

func TestProposalBindingAndIdempotency(t *testing.T) {
	s := testService(t)
	r := planRun(t, s, "proposal", "创建 order-service，JWT 认证和 JSON 日志")
	if r.State != "PLAN" || r.Proposal == nil {
		t.Fatalf("missing preview: %+v", r)
	}
	if _, e := os.Stat(filepath.Join(s.Config.DataDir, "workspaces", r.ID)); !os.IsNotExist(e) {
		t.Fatal("preview wrote project files")
	}
	_, e := s.ProcessCommand(context.Background(), Command{ID: "stale", RunID: r.ID, Action: "accept", ProposalID: r.Proposal.ID, SourceVersion: r.Version - 1})
	if e == nil {
		t.Fatal("stale preview accepted")
	}
	cmd := Command{ID: "accept_one", RunID: r.ID, Action: "accept", ProposalID: r.Proposal.ID, SourceVersion: r.Version, ProposalVersion: r.Proposal.ProposalVersion, SnapshotHash: r.Proposal.SnapshotHash}
	result, e := s.ProcessCommand(context.Background(), cmd)
	if e != nil || !result.Execute {
		t.Fatalf("accept: %v", e)
	}
	again, e := s.ProcessCommand(context.Background(), cmd)
	if e != nil || again.Execute || len(again.Run.History) != len(result.Run.History) {
		t.Fatal("duplicate approval was reapplied")
	}
	cmd.Content = "changed payload"
	if _, e = s.ProcessCommand(context.Background(), cmd); e == nil {
		t.Fatal("same id with changed content accepted")
	}
}

func TestExecutionUsesReviewedSourceSnapshot(t *testing.T) {
	s := testService(t)
	r := planRun(t, s, "snapshot", "创建 snapshot-service")
	reviewed := "# Reviewed source from the approved definition version\n"
	r, e := s.Store.Update(r.ID, func(run *Run) error {
		for i := range run.Proposal.Changes {
			if run.Proposal.Changes[i].Path == "README.md" {
				run.Proposal.Changes[i].After = reviewed
			}
		}
		run.Proposal.SnapshotHash = proposalDigest(*run, *run.Proposal)
		return nil
	})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.ProcessCommand(context.Background(), Command{ID: "accept_snapshot", RunID: r.ID, Action: "accept", ProposalID: r.Proposal.ID, SourceVersion: r.Version, ProposalVersion: r.Proposal.ProposalVersion, SnapshotHash: r.Proposal.SnapshotHash}); e != nil {
		t.Fatal(e)
	}
	if _, e = s.ExecuteStep(context.Background(), StepInput{RunID: r.ID, Step: "scaffold"}); e != nil {
		t.Fatal(e)
	}
	r, _ = s.Store.Get(r.ID)
	source, e := os.ReadFile(filepath.Join(r.Workspace, "README.md"))
	if e != nil || string(source) != reviewed {
		t.Fatal("worker regenerated different contents after review")
	}
}

func TestRealGeneratedProjects(t *testing.T) {
	for _, auth := range []string{"jwt", "session"} {
		t.Run(auth, func(t *testing.T) {
			s := testService(t)
			r := planRun(t, s, auth, "创建 "+auth+"-service，"+auth+" 认证和 JSON 日志")
			result, e := s.ProcessCommand(context.Background(), Command{ID: "accept_" + auth, RunID: r.ID, Action: "accept", ProposalID: r.Proposal.ID, SourceVersion: r.Version, ProposalVersion: r.Proposal.ProposalVersion, SnapshotHash: r.Proposal.SnapshotHash})
			if e != nil || !result.Execute {
				t.Fatal(e)
			}
			for _, step := range executionSteps {
				result, e := s.ExecuteStep(context.Background(), StepInput{RunID: r.ID, Step: step})
				if e != nil {
					t.Fatal(e)
				}
				if !result.Continue && result.State != "DONE" {
					current, _ := s.Store.Get(r.ID)
					t.Fatalf("%s failed: %s", step, current.Error)
				}
			}
			current, _ := s.Store.Get(r.ID)
			if current.State != "DONE" || len(current.Artifacts) < 10 {
				t.Fatalf("invalid completion: %+v", current)
			}
			zipPath := filepath.Join(s.Config.DataDir, "downloads", r.ID+".zip")
			z, e := zip.OpenReader(zipPath)
			if e != nil {
				t.Fatal(e)
			}
			defer z.Close()
			if len(z.File) != len(current.Artifacts) {
				t.Fatal("zip lost files")
			}
			// Restore authority from disk, keeping the raw transcript and identity.
			s.Store.Close()
			restored, e := NewStore(s.Config.DataDir)
			if e != nil {
				t.Fatal(e)
			}
			defer restored.Close()
			saved, e := restored.Get(r.ID)
			if e != nil || saved.State != "DONE" || len(saved.Processed) != 2 {
				t.Fatal("snapshot did not retain execution identity")
			}
			if _, e = os.Stat(filepath.Join(s.Config.DataDir, "runs", r.ID+".jsonl")); e != nil {
				t.Fatal("raw transcript missing")
			}
		})
	}
}

func TestDatabaseTemplatesCompile(t *testing.T) {
	if testing.Short() {
		t.Skip("real ORM dependency build skipped in short mode")
	}
	for _, choice := range []struct{ db, orm string }{{"sqlite", "gorm"}, {"sqlite", "sqlx"}, {"postgres", "gorm"}, {"postgres", "sqlx"}, {"mysql", "gorm"}, {"mysql", "sqlx"}} {
		t.Run(choice.db+"_"+choice.orm, func(t *testing.T) {
			s := testService(t)
			p := defaultPlan()
			p.ProjectName = "database-service"
			p.Skills = []string{"add_database"}
			p.Database = choice.db
			p.ORM = choice.orm
			files, e := RenderFiles(p)
			if e != nil {
				t.Fatal(e)
			}
			root := t.TempDir()
			for path, content := range files {
				target := filepath.Join(root, path)
				if e = os.MkdirAll(filepath.Dir(target), 0755); e != nil {
					t.Fatal(e)
				}
				if e = os.WriteFile(target, []byte(content), 0644); e != nil {
					t.Fatal(e)
				}
			}
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
			defer cancel()
			checks, e := s.VerifyWorkspace(ctx, root)
			if e != nil {
				t.Fatalf("real generated database project: %v, checks=%+v", e, checks)
			}
		})
	}
}
func TestGitLabMissingCredentialsBlocks(t *testing.T) {
	s := testService(t)
	r := planRun(t, s, "gitlab", "创建 gitlab-service，GitLab")
	_, e := s.ProcessCommand(context.Background(), Command{ID: "accept_gitlab", RunID: r.ID, Action: "accept", ProposalID: r.Proposal.ID, SourceVersion: r.Version, ProposalVersion: r.Proposal.ProposalVersion, SnapshotHash: r.Proposal.SnapshotHash})
	if e != nil {
		t.Fatal(e)
	}
	for _, step := range []string{"scaffold", "skills", "verify", "repo"} {
		_, e = s.ExecuteStep(context.Background(), StepInput{RunID: r.ID, Step: step})
		if e != nil {
			t.Fatal(e)
		}
	}
	r, _ = s.Store.Get(r.ID)
	if r.State != "BLOCKED" || r.RepoURL != "" || r.PipelineURL != "" || !strings.Contains(r.Error, "GITLAB_TOKEN") {
		t.Fatalf("false GitLab success: %+v", r)
	}
}
func TestPauseCancelAndTerminalRetry(t *testing.T) {
	s := testService(t)
	r := planRun(t, s, "lifecycle", "创建 life-service")
	_, e := s.ProcessCommand(context.Background(), Command{ID: "pause", RunID: r.ID, Action: "pause"})
	if e != nil {
		t.Fatal(e)
	}
	_, e = s.ProcessCommand(context.Background(), Command{ID: "resume", RunID: r.ID, Action: "resume"})
	if e != nil {
		t.Fatal(e)
	}
	_, e = s.ProcessCommand(context.Background(), Command{ID: "cancel", RunID: r.ID, Action: "cancel"})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.ProcessCommand(context.Background(), Command{ID: "revive", RunID: r.ID, Action: "resume"}); e == nil {
		t.Fatal("terminal run revived")
	}
	newRun, e := s.ProcessCommand(context.Background(), Command{ID: "redo", RunID: r.ID, Action: "retry"})
	if e != nil || newRun.Run.ID == r.ID || newRun.Run.State != "PLAN" {
		t.Fatalf("retry did not create new run: %+v %v", newRun, e)
	}
}
func TestHTTPPathAndStableMessages(t *testing.T) {
	s := testService(t)
	mux := http.NewServeMux()
	s.Register(mux)
	request := func(method, path, body string) *httptest.ResponseRecorder {
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, httptest.NewRequest(method, path, strings.NewReader(body)))
		return rr
	}
	created := request("POST", "/api/runs", `{"id":"run_http","content":"创建 http-service"}`)
	if created.Code != 201 {
		t.Fatal(created.Body.String())
	}
	var r Run
	if e := json.Unmarshal(created.Body.Bytes(), &r); e != nil {
		t.Fatal(e)
	}
	bad := request("POST", "/api/runs/run_http/messages", `{"id":"duplicate","content":"JWT 认证"}`)
	if bad.Code != 200 {
		t.Fatal(bad.Body.String())
	}
	again := request("POST", "/api/runs/run_http/messages", `{"id":"duplicate","content":"JWT 认证"}`)
	if again.Code != 200 {
		t.Fatal(again.Body.String())
	}
	conflict := request("POST", "/api/runs/run_http/messages", `{"id":"duplicate","content":"session 认证"}`)
	if conflict.Code != 409 {
		t.Fatalf("conflicting duplicate: %d", conflict.Code)
	}
	traversal := request("GET", "/api/runs/run_http/artifacts?path=../../runs/run_http.json", "")
	if traversal.Code != 400 {
		t.Fatalf("path traversal status %d", traversal.Code)
	}
	if got := request("GET", "/api/runs/missing", ""); got.Code != 404 {
		t.Fatal("missing run not 404")
	}
}
func TestContextCompressionPreservesTranscript(t *testing.T) {
	s := testService(t)
	r := planRun(t, s, "context", "创建 context-service")
	for i := 0; i < 25; i++ {
		_, e := s.ProcessCommand(context.Background(), Command{ID: strings.Repeat("m", i+1), RunID: r.ID, Content: "JSON 日志"})
		if e != nil {
			t.Fatal(e)
		}
	}
	r, _ = s.Store.Get(r.ID)
	if !r.Context.Compressed || len(r.Messages) < 50 || r.Context.Summary == "" {
		t.Fatal("compression lost original messages")
	}
}
func TestValidation(t *testing.T) {
	p := defaultPlan()
	p.ProjectName = "../../bad"
	if ValidatePlan(p) == nil {
		t.Fatal("traversal name accepted")
	}
	p.ProjectName = "valid-project"
	p.Skills = []string{"arbitrary_shell"}
	if ValidatePlan(p) == nil {
		t.Fatal("unknown skill accepted")
	}
}
