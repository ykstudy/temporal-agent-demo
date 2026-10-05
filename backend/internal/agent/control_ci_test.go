package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"agentstudio/internal/contracts"
)

type controlCIMock struct {
	mu      sync.Mutex
	x       contracts.ExecutionRequest
	sha     string
	mode    string
	posts   int
	gets    int
	created bool
	binary  []byte
}

func (m *controlCIMock) serve(w http.ResponseWriter, r *http.Request) bool {
	if !strings.Contains(r.URL.Path, "/pipelines") && !strings.Contains(r.URL.Path, "/pipeline") && !strings.Contains(r.URL.Path, "/jobs/") {
		return false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if r.Header.Get("PRIVATE-TOKEN") != "controlled-gitlab-test-secret" {
		http.Error(w, "unauthorized", 401)
		return true
	}
	p := controlCIPipeline{ID: 291, ProjectID: 191, Ref: "main", SHA: m.sha, Status: "success", Source: "api"}
	if m.mode == "failed_pipeline" {
		p.Status = "failed"
	}
	w.Header().Set("Content-Type", "application/json")
	if r.Method == http.MethodPost {
		m.posts++
		var body struct {
			Ref       string              `json:"ref"`
			Variables []map[string]string `json:"variables"`
		}
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		if r.URL.Path != "/api/v4/projects/191/pipeline" || decoder.Decode(&body) != nil || body.Ref != "main" || len(body.Variables) != 2 || body.Variables[0]["key"] != "AGENTSTUDIO_OPERATION_ID" || body.Variables[0]["value"] != m.x.OperationID || body.Variables[1]["key"] != "AGENTSTUDIO_REQUEST_HASH" || body.Variables[1]["value"] != m.x.RequestHash {
			http.Error(w, "unexpected scope", 400)
			return true
		}
		m.created = true
		if m.mode == "lost_response" {
			http.Error(w, "lost after CI created", 503)
			return true
		}
		if m.mode == "wrong_sha" {
			p.SHA = strings.Repeat("a", 40)
		}
		w.WriteHeader(201)
		_ = json.NewEncoder(w).Encode(p)
		return true
	}
	if r.Method != http.MethodGet {
		http.Error(w, "unexpected method", 400)
		return true
	}
	m.gets++
	switch {
	case r.URL.Path == "/api/v4/projects/191/pipelines":
		if r.URL.Query().Get("ref") != "main" || r.URL.Query().Get("sha") != m.sha || r.URL.Query().Get("source") != "api" || r.URL.Query().Get("per_page") != "100" {
			http.Error(w, "unexpected query", 400)
			return true
		}
		rows := []controlCIPipeline{}
		if m.created {
			rows = append(rows, p)
		}
		if m.mode == "incomplete_page" {
			w.Header().Set("X-Next-Page", "2")
		}
		_ = json.NewEncoder(w).Encode(rows)
	case r.URL.Path == "/api/v4/projects/191/pipelines/291":
		_ = json.NewEncoder(w).Encode(p)
	case r.URL.Path == "/api/v4/projects/191/pipelines/291/variables":
		operation := m.x.OperationID
		if m.mode == "wrong_variables" {
			operation = "another_operation"
		}
		_ = json.NewEncoder(w).Encode([]map[string]string{{"key": "AGENTSTUDIO_OPERATION_ID", "value": operation}, {"key": "AGENTSTUDIO_REQUEST_HASH", "value": m.x.RequestHash}, {"key": "PRIVATE_SECRET", "value": "secret-not-for-audit"}})
	case r.URL.Path == "/api/v4/projects/191/pipelines/291/jobs":
		rows := []map[string]any{}
		if r.URL.Query().Get("page") == "1" {
			for i, name := range []string{"test", "build"} {
				rows = append(rows, map[string]any{"id": 391 + i, "name": name, "ref": "main", "status": "success", "allow_failure": false, "commit": map[string]string{"id": m.sha}, "pipeline": p})
			}
			if m.mode == "duplicate_job" {
				rows = append(rows, map[string]any{"id": 393, "name": "test", "ref": "main", "status": "success", "allow_failure": false, "commit": map[string]string{"id": m.sha}, "pipeline": p})
			}
		}
		_ = json.NewEncoder(w).Encode(rows)
	case r.URL.Path == "/api/v4/projects/191/jobs/391/artifacts/coverage.out":
		w.Header().Set("Content-Type", "application/octet-stream")
		if m.mode == "malformed_coverage" {
			_, _ = w.Write([]byte("mode: set\nnot coverage\n"))
		} else {
			_, _ = w.Write([]byte("mode: set\nexample.test/service/main.go:2.13,2.15 1 1\n"))
		}
	case r.URL.Path == "/api/v4/projects/191/jobs/392/artifacts/bin/server":
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write(m.binary)
	default:
		http.Error(w, "unexpected CI path", 400)
	}
	return true
}

func controlCIActualBinary(t *testing.T) []byte {
	t.Helper()
	dir := t.TempDir()
	source := filepath.Join(dir, "main.go")
	binary := filepath.Join(dir, "server")
	if err := os.WriteFile(source, []byte("package main\nfunc main() {}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	command := exec.Command("go", "build", "-ldflags=-s -w", "-o", binary, source)
	command.Env = append(os.Environ(), "CGO_ENABLED=0")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("actual executable fixture: %v %s", err, output)
	}
	data, err := os.ReadFile(binary)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func controlCIClone(t *testing.T, base *Service, checkpoint Run) *Service {
	t.Helper()
	s := testService(t)
	s.Config.GitLabURL, s.Config.GitLabNamespace, s.Config.GitLabToken = base.Config.GitLabURL, base.Config.GitLabNamespace, base.Config.GitLabToken
	if _, err := s.Definitions.PublishBundled("3.2.0"); err != nil {
		t.Fatal(err)
	}
	if err := os.CopyFS(filepath.Join(s.Config.DataDir, "objects"), os.DirFS(filepath.Join(base.Config.DataDir, "objects"))); err != nil {
		t.Fatal(err)
	}
	r := clone(checkpoint)
	r.Workspace = filepath.Join(s.Config.DataDir, "workspaces", filepath.FromSlash(r.Control.Audit.Executions[len(r.Control.Audit.Executions)-1].Workspace.StorageKey))
	files := map[string]string{}
	for _, change := range r.Control.Audit.Proposal.Changes {
		if change.After != nil {
			files[change.Path] = *change.After
		}
	}
	if err := os.MkdirAll(filepath.Dir(r.Workspace), 0700); err != nil {
		t.Fatal(err)
	}
	if err := controlCopyFiles(r.Workspace, files); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Store.Create(r); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestControlCIActualReadsRecoveryAndCompletion(t *testing.T) {
	m := &controlCIMock{binary: controlCIActualBinary(t)}
	base, push, id, writes := controlGitPushFixtureHTTP(t, false, nil, m.serve)
	r, err := base.ExecuteControlGitLab(context.Background(), id, push)
	if err != nil {
		t.Fatal(err)
	}
	next, err := base.PrepareControlAction(context.Background(), id)
	if err != nil || next.Execution == nil {
		t.Fatal(err)
	}
	x := *next.Execution
	r, err = base.Store.Get(id)
	if err != nil {
		t.Fatal(err)
	}
	m.x = x
	m.sha = r.Control.Schedule.Results[len(r.Control.Schedule.Results)-1].CommitSHA
	for _, mode := range []string{"success", "lost_response", "wrong_sha", "wrong_variables", "duplicate_job", "malformed_coverage", "failed_pipeline", "incomplete_page"} {
		t.Run(mode, func(t *testing.T) {
			m.mu.Lock()
			m.mode, m.created, m.posts, m.gets = mode, false, 0, 0
			m.mu.Unlock()
			s := controlCIClone(t, base, r)
			after, e := s.ExecuteControlGitLab(context.Background(), id, x)
			if mode != "success" && mode != "lost_response" {
				if e == nil || after.State != "BLOCKED" {
					t.Fatalf("invalid CI accepted: %v %s", e, after.State)
				}
				beforePosts := m.posts
				_, _ = s.ExecuteControlGitLab(context.Background(), id, x)
				if m.posts != beforePosts || after.Control.Audit.Budget.Totals.Reserved.Action != 0 || after.Control.Audit.Budget.Totals.Reserved.CIPoll != 0 {
					t.Fatal("failure replay repeated write or leaked budget")
				}
				if mode == "incomplete_page" && m.posts != 0 {
					t.Fatal("incomplete preflight authorized POST")
				}
				return
			}
			if mode == "lost_response" {
				if e == nil || after.State != "BLOCKED" || m.posts != 1 {
					t.Fatal("lost result not blocked", e)
				}
				unknown, _ := json.Marshal(after.Control.Audit.RemoteReceipts[len(after.Control.Audit.RemoteReceipts)-1])
				cancelled, e := s.ProcessCommand(context.Background(), Command{ID: "cancel_ci", RunID: id, Action: "cancel"})
				if e != nil || cancelled.Run.State != "CANCELLED" {
					t.Fatal("cancel", e)
				}
				s.Store.Close()
				s.Store, err = NewStore(s.Config.DataDir)
				if err != nil {
					t.Fatal(err)
				}
				result, e := s.ProcessCommand(context.Background(), Command{ID: "reconcile_ci", RunID: id, Action: "reconcile"})
				after = result.Run
				if e != nil || result.Execute || after.State != "CANCELLED" || m.posts != 1 {
					t.Fatal("read-only terminal reconciliation", e, after.State)
				}
				preserved, _ := json.Marshal(after.Control.Audit.RemoteReceipts[len(after.Control.Audit.RemoteReceipts)-2])
				if !bytes.Equal(unknown, preserved) {
					t.Fatal("unknown CI history rewritten")
				}
				gets := m.gets
				_, e = s.ProcessCommand(context.Background(), Command{ID: "reconcile_ci", RunID: id, Action: "reconcile"})
				if e != nil || m.gets != gets {
					t.Fatal("reconcile command replay made network calls", e)
				}
			} else if e != nil {
				t.Fatal("complete CI", e)
			}
			q := after.Control.Audit.RemoteReceipts[len(after.Control.Audit.RemoteReceipts)-1]
			if q.Status != "succeeded" || q.ActionType != "trigger_ci" || q.Pipeline == nil || q.Pipeline.PipelineID != 291 || len(q.Jobs) != 2 || m.posts != 1 || writes.Load() != 1 {
				t.Fatal("missing exact complete CI result")
			}
			if err := ValidateControlRemoteReceipt(after, q); err != nil {
				t.Fatal(err)
			}
			assertControlCINegatives(t, after, q)
			body, _ := json.Marshal(after)
			if strings.Contains(string(body), "secret-not-for-audit") || strings.Contains(string(body), s.Config.GitLabToken) {
				t.Fatal("CI response leaked credentials or unrelated variables")
			}
			version := after.Control.Audit.Budget.LedgerVersion
			gets := m.gets
			if _, e = s.ExecuteControlGitLab(context.Background(), id, x); e != nil || m.posts != 1 || m.gets != gets {
				t.Fatal("CI execution replay changed side effects", e)
			}
			again, _ := s.Store.Get(id)
			if again.Control.Audit.Budget.LedgerVersion != version {
				t.Fatal("CI replay charged again")
			}
			for _, job := range q.Jobs {
				for _, artifact := range job.Artifacts {
					ref := contracts.ObjectReference(artifact.ArtifactRef)
					data, e := s.ReadControlCIArtifact(id, ref)
					if e != nil || contracts.BytesHash(data) != artifact.SHA256 {
						t.Fatal("actual immutable artifact read", e)
					}
					query := url.Values{"object_type": {ref.ObjectType}, "object_id": {ref.ObjectID}, "object_version": {"1"}, "source_version": {strconv.FormatInt(ref.SourceVersion, 10)}, "hash": {ref.Hash}}
					request := httptest.NewRequest("GET", "/api/runs/"+id+"/objects?"+query.Encode(), nil)
					response := httptest.NewRecorder()
					mux := http.NewServeMux()
					s.Register(mux)
					mux.ServeHTTP(response, request)
					if response.Code != 200 || !bytes.Equal(response.Body.Bytes(), data) || response.Header().Get("X-Content-SHA256") != ref.Hash {
						t.Fatal("public raw artifact read failed", response.Code)
					}
				}
			}
			if mode == "success" {
				next, e := s.PrepareControlAction(context.Background(), id)
				if e != nil || next.Execution == nil {
					t.Fatal("package after exact CI", e)
				}
				after, _, e = s.ExecuteControlPackage(context.Background(), id, *next.Execution)
				if e != nil {
					t.Fatal(e)
				}
				delivered := clone(after)
				after, report, e := s.FinalizeControlCompletion(context.Background(), id)
				if e != nil || after.State != "DONE" || report.Outcome != "passed" || len(report.Checks) != 14 {
					t.Fatal("remote deterministic completion", e, after.State, report.Outcome, len(report.Checks))
				}
				artifact := q.Jobs[0].Artifacts[0]
				_, filename := controlCIArtifactIdentity(x, 291, q.Jobs[0].JobID, artifact.Path)
				if e = os.WriteFile(filepath.Join(s.Config.DataDir, "workspaces", id, filepath.FromSlash(filename)), []byte("corrupt"), 0600); e != nil {
					t.Fatal(e)
				}
				if _, e = s.ReadControlCIArtifact(id, contracts.ObjectReference(artifact.ArtifactRef)); e == nil {
					t.Fatal("changed raw artifact accepted")
				}
				observed := s.observeControlCompletion(delivered)
				report, e = buildControlVerificationReport(delivered, observed, "corrupt_artifact_report", 1, time.Now().UTC())
				if e != nil {
					t.Fatal(e)
				}
				for _, check := range report.Checks {
					if check.CheckID == "check.gitlab_ci" && check.Status == "passed" {
						t.Fatal("completion ignored changed raw artifact")
					}
				}
			}
			if dir := os.Getenv("CONTROL_CI_EVIDENCE_DIR"); dir != "" {
				audit, e := s.Store.CanonicalAudit(id)
				if e != nil {
					t.Fatal(e)
				}
				data, _ := json.MarshalIndent(map[string]any{"kind": "actual_fixed_git_and_mock_ci_with_real_executable", "public_creation": false, "live_provider": false, "initial_proposal": "synthetic workspace/Go prerequisites; actual Git, artifact download, package and deterministic completion", "audit": audit}, "", "  ")
				if e = os.MkdirAll(dir, 0700); e != nil {
					t.Fatal(e)
				}
				if e = os.WriteFile(filepath.Join(dir, mode+".json"), data, 0600); e != nil {
					t.Fatal(e)
				}
			}
		})
	}
}

func assertControlCINegatives(t *testing.T, r Run, original contracts.RemoteExecutionReceipt) {
	t.Helper()
	for index, mutate := range []func(*contracts.RemoteExecutionReceipt){
		func(q *contracts.RemoteExecutionReceipt) { q.Pipeline.PipelineID++ },
		func(q *contracts.RemoteExecutionReceipt) { q.Pipeline.CommitSha = strings.Repeat("a", 40) },
		func(q *contracts.RemoteExecutionReceipt) { q.PipelineVariables.OperationID = "other_operation" },
		func(q *contracts.RemoteExecutionReceipt) { q.Jobs = q.Jobs[:1] },
		func(q *contracts.RemoteExecutionReceipt) { q.Jobs[0].AllowFailure = true },
		func(q *contracts.RemoteExecutionReceipt) { q.Jobs[0].Artifacts[0].Validated = false },
		func(q *contracts.RemoteExecutionReceipt) { q.Jobs[0].Artifacts[0].SHA256 = strings.Repeat("b", 64) },
		func(q *contracts.RemoteExecutionReceipt) { q.Observations = q.Observations[:len(q.Observations)-1] },
	} {
		body, _ := json.Marshal(original)
		var q contracts.RemoteExecutionReceipt
		_ = json.Unmarshal(body, &q)
		mutate(&q)
		q.ReceiptHash, _ = contracts.HashObject("RemoteExecutionReceipt", q)
		if ValidateControlRemoteReceipt(r, q) == nil {
			t.Fatal("rehashed fake CI accepted", index)
		}
	}
}

func TestControlCIArtifactFormatAndPaginationNegatives(t *testing.T) {
	for _, data := range []string{"mode: set\n", "mode: set\ninvalid\n", "mode: set\np.go:3.1,2.1 1 1\n", "mode: set\np.go:1.1,2.1 1 2\n", "mode: set\np.go:1.1,2.1 0 0\n"} {
		if _, err := controlCIArtifactFormat("coverage.out", []byte(data)); err == nil {
			t.Fatal("invalid coverage accepted")
		}
	}
	for _, data := range [][]byte{{0x7f, 'E', 'L', 'F'}, {0xcf, 0xfa, 0xed, 0xfe}} {
		if _, err := controlCIArtifactFormat("bin/server", data); err == nil {
			t.Fatal("magic-only executable accepted")
		}
	}
	for _, header := range []http.Header{{"X-Next-Page": {"3"}}, {"X-Next-Page": {"2"}}} {
		if _, err := controlCIPageEnd(header, 1, 1); err == nil {
			t.Fatal("incomplete/invalid pagination accepted")
		}
	}
}
