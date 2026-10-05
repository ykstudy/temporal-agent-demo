package agent

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"agentstudio/internal/contracts"
)

func controlGitPushFixture(t *testing.T, lose bool) (*Service, contracts.ExecutionRequest, string, *atomic.Int64) {
	return controlGitPushFixtureHook(t, lose, nil)
}

func controlGitPushFixtureHook(t *testing.T, lose bool, onPushed func()) (*Service, contracts.ExecutionRequest, string, *atomic.Int64) {
	return controlGitPushFixtureHTTP(t, lose, onPushed, nil)
}

func controlGitMockHTTP(t *testing.T, lose bool, onPushed func(), extra func(http.ResponseWriter, *http.Request) bool) (*controlGitLabMock, *atomic.Int64) {
	t.Helper()
	gitHTTP, bare, writes := controlGitHTTPServer(t)
	m := &controlGitLabMock{}
	git := func(args ...string) (string, error) {
		data, err := exec.Command("git", append([]string{"-C", bare}, args...)...).Output()
		return string(data), err
	}
	m.extra = func(w http.ResponseWriter, req *http.Request) bool {
		if extra != nil && extra(w, req) {
			return true
		}
		if strings.HasPrefix(req.URL.Path, "/schedule-group/create-service.git/") {
			if lose && req.Method == http.MethodPost {
				recorder := httptest.NewRecorder()
				gitHTTP.ServeHTTP(recorder, req)
				if onPushed != nil {
					onPushed()
				}
				http.Error(w, "response lost after actual push "+controlGitAuthorization("controlled-gitlab-test-secret"), 503)
			} else {
				gitHTTP.ServeHTTP(w, req)
			}
			return true
		}
		sha, err := git("rev-parse", "--verify", "refs/heads/main")
		sha = strings.TrimSpace(sha)
		if req.URL.Path == "/api/v4/projects/191" && err == nil {
			m.mu.Lock()
			if m.project != nil {
				empty, branch := false, "main"
				m.project.EmptyRepo, m.project.DefaultBranch = &empty, &branch
			}
			m.mu.Unlock()
			return false
		}
		prefix := "/api/v4/projects/191/repository/"
		if !strings.HasPrefix(req.URL.Path, prefix) {
			return false
		}
		if req.Method != http.MethodGet || req.Header.Get("PRIVATE-TOKEN") != "controlled-gitlab-test-secret" {
			http.Error(w, "bad read", 400)
			return true
		}
		if err != nil {
			http.Error(w, "not found", 404)
			return true
		}
		w.Header().Set("Content-Type", "application/json")
		path := strings.TrimPrefix(req.URL.Path, prefix)
		switch {
		case path == "branches/main":
			_ = json.NewEncoder(w).Encode(map[string]any{"name": "main", "commit": map[string]any{"id": sha}})
		case path == "tree":
			rows := []map[string]string{}
			if req.URL.Query().Get("page") == "1" {
				tree, e := git("ls-tree", "-r", "-z", "--full-tree", sha)
				if e != nil {
					http.Error(w, "tree failed", 500)
					return true
				}
				for _, row := range strings.Split(strings.TrimSuffix(tree, "\x00"), "\x00") {
					meta, name, _ := strings.Cut(row, "\t")
					fields := strings.Fields(meta)
					rows = append(rows, map[string]string{"id": fields[2], "type": fields[1], "mode": fields[0], "path": name})
				}
			}
			_ = json.NewEncoder(w).Encode(rows)
		case strings.HasPrefix(path, "files/"):
			name := strings.TrimPrefix(path, "files/")
			body, e := git("show", sha+":"+name)
			if e != nil {
				http.Error(w, "missing file", 404)
				return true
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"file_path": name, "encoding": "base64", "content": base64.StdEncoding.EncodeToString([]byte(body)), "size": len([]byte(body)), "blob_id": controlGitBlob(body), "commit_id": sha, "content_sha256": contracts.BytesHash([]byte(body))})
		case path == "commits/"+sha:
			message, e := git("show", "-s", "--format=%B", sha)
			if e != nil {
				http.Error(w, "missing commit", 404)
				return true
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"id": sha, "parent_ids": []string{}, "message": strings.TrimRight(message, "\n") + "\n"})
		default:
			http.Error(w, "unexpected read", 400)
		}
		return true
	}
	return m, writes
}

func controlGitPushFixtureHTTP(t *testing.T, lose bool, onPushed func(), extra func(http.ResponseWriter, *http.Request) bool) (*Service, contracts.ExecutionRequest, string, *atomic.Int64) {
	m, writes := controlGitMockHTTP(t, lose, onPushed, extra)
	s, create, id := controlGitLabFixtureVersion(t, m, "1.3.0")
	r, err := s.ExecuteControlGitLab(context.Background(), id, create)
	if err != nil {
		t.Fatal(err)
	}
	next, err := s.PrepareControlAction(context.Background(), id)
	if err != nil || next.Execution == nil {
		t.Fatal("prepare push", err)
	}
	// Only workspace/Go prerequisites are synthetic. Place their exact approved
	// bytes in the approved directory; all six Git processes/remote bytes below
	// are real and must independently agree with these complete public objects.
	x := *next.Execution
	path := filepath.Join(s.Config.DataDir, "workspaces", filepath.FromSlash(x.Workspace.StorageKey))
	if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{}
	for _, change := range r.Control.Audit.Proposal.Changes {
		if change.After != nil {
			files[change.Path] = *change.After
		}
	}
	if err = controlCopyFiles(path, files); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Store.Update(id, func(r *Run) error { r.Workspace = path; return nil }); err != nil {
		t.Fatal(err)
	}
	return s, x, id, writes
}

// The saved checkpoint is captured from the real Store while the actual Git
// server has committed the write but the client has not received its response.
// A second Store resumes that exact checkpoint without running another tool.
func TestControlGitPushOriginalLaunchRecovery(t *testing.T) {
	checkpoint := make(chan Run, 1)
	objects := t.TempDir()
	var s *Service
	var id string
	var x contracts.ExecutionRequest
	var writes *atomic.Int64
	s, x, id, writes = controlGitPushFixtureHook(t, true, func() {
		r, err := s.Store.Get(id)
		if err != nil {
			t.Error(err)
			return
		}
		if err = os.CopyFS(objects, os.DirFS(filepath.Join(s.Config.DataDir, "objects"))); err != nil {
			t.Error(err)
			return
		}
		checkpoint <- r
	})
	if _, err := s.ExecuteControlGitLab(context.Background(), id, x); err == nil {
		t.Fatal("lost response unexpectedly succeeded")
	}
	var before Run
	select {
	case before = <-checkpoint:
	default:
		t.Fatal("real original launch checkpoint missing")
	}
	parent, err := controlGitLabAttempt(before, x)
	if err != nil {
		t.Fatal(err)
	}
	child := controlGitChild(x, parent.AttemptID, "git.push")
	if before.Control.GitInvocations[child].StartedAt == "" {
		t.Fatal("original launch not durable before external effect")
	}
	for _, q := range before.Control.Audit.ToolReceipts {
		if q.AttemptID == child {
			t.Fatal("checkpoint already has a completed push receipt")
		}
	}
	for _, removeLaunch := range []bool{true, false} {
		name := "original_launch"
		if removeLaunch {
			name = "missing_launch"
		}
		t.Run(name, func(t *testing.T) {
			recovered := testService(t)
			recovered.Config.GitLabURL, recovered.Config.GitLabNamespace, recovered.Config.GitLabToken = s.Config.GitLabURL, s.Config.GitLabNamespace, s.Config.GitLabToken
			if _, err := recovered.Definitions.PublishBundled("3.2.0"); err != nil {
				t.Fatal(err)
			}
			if err := os.CopyFS(filepath.Join(recovered.Config.DataDir, "objects"), os.DirFS(objects)); err != nil {
				t.Fatal(err)
			}
			r := clone(before)
			if removeLaunch {
				delete(r.Control.GitInvocations, child)
			}
			if _, err := recovered.Store.Create(r); err != nil {
				t.Fatal("restore real checkpoint", err)
			}
			after, err := recovered.ReconcileControlGitLab(context.Background(), id, x)
			if removeLaunch {
				if err == nil || after.State != "BLOCKED" {
					t.Fatal("missing launch invented history", err)
				}
				for _, q := range after.Control.Audit.ToolReceipts {
					if q.AttemptID == child {
						t.Fatal("missing original launch created a tool receipt")
					}
				}
			} else {
				if err != nil {
					t.Fatal("read-only checkpoint recovery", err)
				}
				q := after.Control.Audit.RemoteReceipts[len(after.Control.Audit.RemoteReceipts)-1]
				if q.Status != "succeeded" || ValidateControlRemoteReceipt(after, q) != nil {
					t.Fatal("recovery did not prove original push")
				}
				found := false
				for _, tool := range after.Control.Audit.ToolReceipts {
					if tool.AttemptID == child {
						found = tool.Status == "unknown" && tool.ExitCode == nil && tool.Output == "" && tool.OutputTruncated && tool.OutputManifestRef == nil
					}
				}
				if !found {
					t.Fatal("recovery fabricated a successful push process or output")
				}
				for _, b := range after.Control.Audit.Budget.Reservations {
					if b.AttemptID == child {
						settled := controlGitLabSettled(after, b)
						if settled == nil || settled.Outcome != "unknown" || settled.ConsumedUnits != 1 || settled.MeasurementSource != "unknown" {
							t.Fatal("original unknown process charge lost")
						}
					}
				}
			}
			if writes.Load() != 1 {
				t.Fatal("recovery repeated remote push")
			}
		})
	}
}

func TestControlGitPushActualAndLostResponse(t *testing.T) {
	for _, lost := range []bool{false, true} {
		name := "success"
		if lost {
			name = "lost_response"
		}
		t.Run(name, func(t *testing.T) {
			s, x, id, writes := controlGitPushFixture(t, lost)
			r, err := s.ExecuteControlGitLab(context.Background(), id, x)
			if lost {
				if err == nil || r.State != "BLOCKED" || writes.Load() != 1 {
					t.Fatalf("lost push did not preserve unknown: %v state=%s writes=%d", err, r.State, writes.Load())
				}
				unknown, _ := json.Marshal(r.Control.Audit.RemoteReceipts[len(r.Control.Audit.RemoteReceipts)-1])
				cancelled, e := s.ProcessCommand(context.Background(), Command{ID: "cancel_after_push", RunID: id, Action: "cancel"})
				if e != nil || cancelled.Run.State != "CANCELLED" {
					t.Fatal("cancel", e)
				}
				r, err = s.ReconcileControlGitLab(context.Background(), id, x)
				if err != nil || r.State != "CANCELLED" {
					t.Fatal("read-only original push reconciliation", err, r.State)
				}
				preserved, _ := json.Marshal(r.Control.Audit.RemoteReceipts[len(r.Control.Audit.RemoteReceipts)-2])
				if string(unknown) != string(preserved) {
					t.Fatal("unknown history rewritten")
				}
			} else if err != nil {
				t.Fatal("actual fixed push", err)
			}
			q := r.Control.Audit.RemoteReceipts[len(r.Control.Audit.RemoteReceipts)-1]
			if q.Status != "succeeded" || q.ActionType != "push_repository" || q.Commit == nil || len(q.ToolReceiptRefs) != 6 || writes.Load() != 1 {
				t.Fatal("complete push receipt missing or repeated")
			}
			if err = ValidateControlRemoteReceipt(r, q); err != nil {
				t.Fatal(err)
			}
			assertControlPushNegatives(t, r, q)
			version := r.Control.Audit.Budget.LedgerVersion
			s.Store.Close()
			s.Store, err = NewStore(s.Config.DataDir)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(s.Store.Close)
			r, err = s.ExecuteControlGitLab(context.Background(), id, x)
			if err != nil || writes.Load() != 1 || r.Control.Audit.Budget.LedgerVersion != version {
				t.Fatal("push replay changed effect or budget", err)
			}
			if !lost {
				next, e := NextControlAction(r, r.Control.Schedule)
				if e != nil || next.Action == nil {
					t.Fatal("next CI", e)
				}
				kind, _, _ := scheduleAction(*next.Action)
				if kind != "trigger_ci" {
					t.Fatal("push skipped CI", kind)
				}
			}
			body, _ := json.Marshal(r)
			for _, secret := range []string{s.Config.GitLabToken, controlGitAuthorization(s.Config.GitLabToken), strings.TrimPrefix(controlGitAuthorization(s.Config.GitLabToken), "Basic ")} {
				if strings.Contains(string(body), secret) {
					t.Fatal("credential in persisted state")
				}
			}
			if dir := os.Getenv("CONTROL_PUSH_EVIDENCE_DIR"); dir != "" {
				audit, e := s.Store.CanonicalAudit(id)
				if e != nil {
					t.Fatal(e)
				}
				body, e = json.MarshalIndent(map[string]any{"kind": "actual_fixed_git_and_local_bare_remote", "public_creation": false, "live_provider": false, "initial_proposal": "synthetic workspace/Go prerequisite receipts; exact approved files supplied by test; all Git processes and remote bytes are real", "state": r.State, "audit": audit}, "", "  ")
				if e != nil {
					t.Fatal(e)
				}
				if err = os.MkdirAll(dir, 0700); err != nil {
					t.Fatal(err)
				}
				if err = os.WriteFile(filepath.Join(dir, name+".json"), body, 0600); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}

func assertControlPushNegatives(t *testing.T, r Run, original contracts.RemoteExecutionReceipt) {
	t.Helper()
	for _, change := range []string{"missing_tool", "sha", "file", "ci_hash", "read_path", "write", "parent", "tool_tree", "tool_args", "tool_attempt"} {
		t.Run(change, func(t *testing.T) {
			bad := clone(r)
			q := original
			encoded, _ := json.Marshal(original)
			_ = json.Unmarshal(encoded, &q)
			switch change {
			case "missing_tool":
				q.ToolReceiptRefs = q.ToolReceiptRefs[1:]
			case "sha":
				q.Commit.CommitSha = strings.Repeat("b", 40)
			case "file":
				q.Commit.Files = q.Commit.Files[1:]
			case "ci_hash":
				v := strings.Repeat("a", 64)
				q.CIConfigSHA256 = &v
			case "read_path":
				for i := range q.Observations {
					if q.Observations[i].Kind == "file" {
						q.Observations[i].Path += "wrong"
						break
					}
				}
			case "write":
				q.WriteSent = false
			case "parent":
				q.Commit.ParentShas = []string{strings.Repeat("a", 40)}
			default:
				for i := range bad.Control.Audit.ToolReceipts {
					tool := &bad.Control.Audit.ToolReceipts[i]
					if tool.ExecutionID != q.ExecutionID || tool.CommandID != "git.tree" {
						continue
					}
					switch change {
					case "tool_tree":
						tool.Output = "100644 blob " + strings.Repeat("a", 40) + "\twrong\x00"
						tool.OutputSHA256 = contracts.BytesHash([]byte(tool.Output))
					case "tool_args":
						tool.Arguments = []byte(`["ls-tree","HEAD"]`)
					case "tool_attempt":
						tool.AttemptID += "other"
					}
					tool.ReceiptHash = chainRehash(t, "ToolExecutionReceipt", *tool)
					for j := range q.ToolReceiptRefs {
						if q.ToolReceiptRefs[j].ObjectID == tool.ReceiptID {
							q.ToolReceiptRefs[j].Hash = tool.ReceiptHash
						}
					}
				}
			}
			q.ReceiptHash = chainRehash(t, "RemoteExecutionReceipt", q)
			if ValidateControlRemoteReceipt(bad, q) == nil {
				t.Fatal("forged push evidence accepted", change)
			}
		})
	}
}
