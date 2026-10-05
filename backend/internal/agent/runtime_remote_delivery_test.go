package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"
)

// Complete 1.4 creation and actual local Go/Git/package execution. GitLab HTTP
// and Runner job/artifact responses are explicitly mock, never live evidence.
func TestRuntime14ActualGitAndMockCIFullDelivery(t *testing.T) {
	ci := &controlCIMock{binary: controlCIActualBinary(t)}
	remote, writes := controlGitMockHTTP(t, false, nil, ci.serve)
	server := httptest.NewServer(http.HandlerFunc(remote.serve))
	defer server.Close()
	s := testService(t)
	s.Config.GitLabURL, s.Config.GitLabNamespace, s.Config.GitLabToken = server.URL, "17", "controlled-gitlab-test-secret"
	d, e := s.Definitions.PublishBundled("3.3.0")
	if e != nil {
		t.Fatal(e)
	}
	r, e := s.CreateWithOptions(context.Background(), CreateOptions{ID: "runtime_remote_delivery", Title: "actual Go/Git; mock GitLab Runner", Content: "创建 create-service", ExecutionMode: "local", DecisionMode: "rules", DeliveryMode: "gitlab", DefinitionID: d.DefinitionID, DefinitionVersion: d.DefinitionVersion, DefinitionHash: d.DefinitionHash})
	if e != nil {
		t.Fatal(e)
	}
	r = controlAcceptCommand(t, s, r)
	for _, step := range executionSteps {
		if step == "ci" {
			next, e := s.PrepareControlAction(context.Background(), r.ID)
			if e != nil || next.Execution == nil {
				t.Fatal("prepare CI", e)
			}
			current, _ := s.Store.Get(r.ID)
			ci.x = *next.Execution
			ci.sha = current.Control.Schedule.Results[len(current.Control.Schedule.Results)-1].CommitSHA
		}
		if _, e = s.ExecuteStep(context.Background(), StepInput{RunID: r.ID, Step: step}); e != nil {
			t.Fatal(step, e)
		}
	}
	r, e = s.Store.Get(r.ID)
	if e != nil || r.State != "DONE" || len(r.Control.Audit.VerificationReport.Checks) != 14 || ci.posts != 1 || writes.Load() != 1 {
		t.Fatal("incomplete current remote delivery", e, r.State, ci.posts, writes.Load())
	}
	if e = s.Store.verifyCanonical(r); e != nil {
		t.Fatal(e)
	}
	if dir := os.Getenv("RUNTIME_REMOTE_EVIDENCE_DIR"); dir != "" {
		if e = os.MkdirAll(filepath.Join(dir, "content"), 0700); e != nil {
			t.Fatal(e)
		}
		mux := http.NewServeMux()
		s.Register(mux)
		read := func(path string) []byte {
			t.Helper()
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
			if w.Code != 200 {
				t.Fatal("public export", path, w.Code, w.Body.String())
			}
			return w.Body.Bytes()
		}
		base := "/api/runs/" + r.ID
		v2 := "/api/v2/runs/" + r.ID
		export := map[string]any{"provenance": "public HTTP responses from current product integration; actual Go/Git/package; mock GitLab/Runner; no synthetic prerequisite receipts"}
		for key, path := range map[string]string{"snapshot": v2, "audit": base + "/decision-audit", "conversation": base + "/conversation-audit", "runtime": v2 + "/runtime", "runtime_audit": v2 + "/audit"} {
			export[key] = json.RawMessage(read(path))
		}
		for _, dep := range r.FrozenDefinition.TemplateDependencies {
			for _, file := range dep.Files {
				ref := file.ContentRef
				query := url.Values{"object_type": {ref.ObjectType}, "object_id": {ref.ObjectID}, "hash": {ref.Hash}}
				if e = os.WriteFile(filepath.Join(dir, "content", ref.Hash), read(base+"/definition/content?"+query.Encode()), 0600); e != nil {
					t.Fatal(e)
				}
			}
		}
		for _, dep := range r.FrozenDefinition.SchemaDependencies {
			query := url.Values{"object_type": {"schema"}, "object_id": {dep.SchemaURI}, "hash": {dep.SHA256}}
			if e = os.WriteFile(filepath.Join(dir, "content", dep.SHA256), read(base+"/definition/content?"+query.Encode()), 0600); e != nil {
				t.Fatal(e)
			}
		}
		raw, _ := json.MarshalIndent(export, "", "  ")
		if e = os.WriteFile(filepath.Join(dir, "public-objects.json"), raw, 0600); e != nil {
			t.Fatal(e)
		}
	}
}
