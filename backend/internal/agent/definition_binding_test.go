package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"agentstudio/internal/contracts"
	"agentstudio/internal/definitions"
)

func fixtureAdvice(t *testing.T, r Run, p Plan) []byte {
	t.Helper()
	plan, err := projectPlanFor(r, p)
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(contracts.DecisionAdvice{Kind: "plan", Plan: &plan, Reason: "structured fixture suggestion"})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestFrozenRunRendersOriginalTemplatesAndPromptAcrossPublications(t *testing.T) {
	s := testService(t)
	old := planRun(t, s, "frozen_old", "创建 stable-service，Postgres GORM JWT JSON 日志")
	original, err := s.renderFrozenFiles(old)
	if err != nil {
		t.Fatal(err)
	}
	request, err := buildModelRequest(old, "继续", "fixture")
	if err != nil {
		t.Fatal(err)
	}
	bundles, err := releaseTemplatePrograms()
	if err != nil {
		t.Fatal(err)
	}
	for i := range bundles {
		bundles[i].Version = "3.1.0"
		if bundles[i].ID == "scaffold" {
			var program templateProgram
			if err = json.Unmarshal(bundles[i].Files["render-program.json"], &program); err != nil {
				t.Fatal(err)
			}
			for j := range program.Rules {
				if program.Rules[j].Path == "README.md" {
					program.Rules[j].Source += "\nNEW TEMPLATE VERSION\n"
				}
			}
			bundles[i].Files["render-program.json"], err = json.Marshal(program)
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	platform := runtime.GOOS + "_" + runtime.GOARCH
	def, err := s.Definitions.PublishDefault(definitions.PublishOptions{DefinitionVersion: "3.1.0", SkillVersion: "3.1.0", TemplateVersion: "3.1.0", SystemPrompt: defaultSystemPrompt + "\nNEW PROMPT VERSION", Tools: []contracts.ToolIdentity{{ToolID: "go", Version: runtime.Version(), Platform: &platform}}, Templates: bundles})
	if err != nil {
		t.Fatal(err)
	}
	s.DefaultDefinition = def
	after, err := s.renderFrozenFiles(old)
	if err != nil {
		t.Fatal(err)
	}
	if digest(original) != digest(after) {
		t.Fatal("old Run used new template bytes")
	}
	afterRequest, err := buildModelRequest(old, "继续", "fixture")
	if err != nil {
		t.Fatal(err)
	}
	if string(request) != string(afterRequest) {
		t.Fatal("old Run used new prompt")
	}
	newRun := planRun(t, s, "frozen_new", "创建 next-service")
	newFiles, err := s.renderFrozenFiles(newRun)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(newFiles["README.md"], "NEW TEMPLATE VERSION") || newRun.FrozenDefinition.DefinitionHash == old.FrozenDefinition.DefinitionHash {
		t.Fatal("new selection did not bind new publication")
	}
	persisted, err := s.Store.Get(old.ID)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.FrozenDefinition.SnapshotHash != old.FrozenDefinition.SnapshotHash {
		t.Fatal("old snapshot changed")
	}
	// Same Skill/template version cannot be relabeled with different content.
	bundles[0].Files["render-program.json"] = []byte("different")
	if _, err = s.Definitions.PublishDefault(definitions.PublishOptions{DefinitionVersion: "3.1.0", SkillVersion: "3.1.0", Templates: bundles, Tools: def.RequiredTools}); err == nil {
		t.Fatal("same version content replacement accepted")
	}
}

func TestFrozenTemplateAllVariantsMatchExistingGeneration(t *testing.T) {
	s := testService(t)
	r, err := s.Create(context.Background(), "run_variants", "", "")
	if err != nil {
		t.Fatal(err)
	}
	for _, db := range []string{"postgres", "mysql", "sqlite"} {
		for _, orm := range []string{"gorm", "sqlx"} {
			for _, auth := range []string{"jwt", "session"} {
				for _, output := range []string{"console", "file"} {
					p := r.Plan
					p.ProjectName = "frozen-variants"
					p.Skills = []string{"add_database", "add_auth", "add_logging"}
					p.Database = db
					p.ORM = orm
					p.Auth = auth
					p.LogOutput = output
					p.TokenTTL = "7h"
					p.LogPath = "logs/custom.log"
					r.Plan = p
					want, err := RenderFiles(p)
					if err != nil {
						t.Fatal(err)
					}
					got, err := s.renderFrozenFiles(r)
					if err != nil {
						t.Fatal(err)
					}
					if digest(want) != digest(got) {
						t.Fatalf("frozen renderer drift for %s/%s/%s/%s", db, orm, auth, output)
					}
				}
			}
		}
	}
}

func TestFrozenDefinitionPublicReadAndLegacyMissingSource(t *testing.T) {
	s := testService(t)
	r := planRun(t, s, "definition_public", "创建 read-service")
	mux := http.NewServeMux()
	s.Register(mux)
	read := func(path string) *httptest.ResponseRecorder {
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, httptest.NewRequest("GET", path, nil))
		return rr
	}
	response := read("/api/runs/" + r.ID + "/definition")
	if response.Code != 200 {
		t.Fatal(response.Code, response.Body.String())
	}
	if err := contracts.ValidateJSON("DefinitionSnapshot", response.Body.Bytes()); err != nil {
		t.Fatal(err)
	}
	dep := r.FrozenDefinition.TemplateDependencies[0].Files[0]
	content := read("/api/runs/" + r.ID + "/definition/content?object_type=template_content&object_id=" + dep.ContentRef.ObjectID + "&hash=" + dep.SHA256)
	if content.Code != 200 || contracts.BytesHash(content.Body.Bytes()) != dep.SHA256 {
		t.Fatal("public content mismatch")
	}
	if read("/api/runs/"+r.ID+"/definition/content?object_type=template_content&object_id=other&hash="+dep.SHA256).Code != 404 {
		t.Fatal("unbound content exposed")
	}
	_, err := s.Store.Update(r.ID, func(run *Run) error {
		run.FrozenDefinition = nil
		run.Definition.FrozenSnapshotHash = ""
		run.Definition.Hash = definitionDigest(run.Definition)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if response = read("/api/runs/" + r.ID + "/definition"); response.Code != 409 || !strings.Contains(response.Body.String(), "legacy_contract_incomplete") {
		t.Fatal("missing legacy history fabricated")
	}
	// Missing content after a restart fails closed, never reads current sources.
	if err = os.Remove(filepath.Join(s.Config.DataDir, "definitions", "blobs", dep.SHA256)); err != nil {
		t.Fatal(err)
	}
	if _, err = s.renderFrozenFiles(r); err == nil {
		t.Fatal("missing blob silently rendered from current code")
	}
}

func TestHTTPRejectsDuplicateJSONKeysBeforeEffects(t *testing.T) {
	s := testService(t)
	mux := http.NewServeMux()
	s.Register(mux)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest("POST", "/api/runs", strings.NewReader(`{"id":"run_one","id":"run_two","content":"create foo-service"}`)))
	if rr.Code != 400 || len(s.Store.List()) != 0 {
		t.Fatal("duplicate key created a Run")
	}
}
