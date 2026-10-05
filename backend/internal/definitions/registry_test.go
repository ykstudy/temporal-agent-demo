package definitions

import (
	"agentstudio/assets"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"agentstudio/internal/contracts"
)

func testOptions(version string) PublishOptions {
	platform := "test_amd64"
	opts := PublishOptions{DefinitionVersion: version, SkillVersion: version, TemplateVersion: version, SystemPrompt: "Only propose structured DecisionAdvice; never approve or execute.", Tools: []contracts.ToolIdentity{{ToolID: "go", Version: "go1.25.0", Platform: &platform, BinarySHA256: nil}}}
	for _, id := range []string{"scaffold", "database", "auth", "logging"} {
		opts.Templates = append(opts.Templates, TemplateBundle{ID: id, Version: version, Files: map[string][]byte{"render-program.json": []byte(`{"format_version":1,"rules":[{"path":"` + id + `.go","source":"package frozen"}]}`)}})
	}
	return opts
}
func publishTest(t *testing.T, r *Registry, opts PublishOptions) (Release, contracts.DefinitionSnapshot) {
	t.Helper()
	release, e := BuildDefault(opts)
	if e != nil {
		t.Fatal(e)
	}
	if e = r.Publish(release); e != nil {
		t.Fatal(e)
	}
	d := release.Definition
	s, e := r.Freeze(d.DefinitionID, d.DefinitionVersion, d.DefinitionHash, nil, time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC))
	if e != nil {
		t.Fatal(e)
	}
	return release, s
}
func openTest(t *testing.T) *Registry {
	t.Helper()
	r, e := Open(filepath.Join(t.TempDir(), "registry"))
	if e != nil {
		t.Fatal(e)
	}
	return r
}

func TestCompleteFrozenClosureAndRealHashes(t *testing.T) {
	r := openTest(t)
	release, snapshot := publishTest(t, r, testOptions("3.0.0"))
	if e := contracts.Validate("DefinitionSnapshot", snapshot); e != nil {
		t.Fatal(e)
	}
	if snapshot.DefinitionHash == snapshot.SnapshotHash || snapshot.DefinitionHash == strings.Repeat("0", 64) {
		t.Fatal("logical definition and closed snapshot need separate real identities")
	}
	if e := r.VerifySnapshot(snapshot); e != nil {
		t.Fatal(e)
	}
	for _, skill := range snapshot.Skills {
		if e := contracts.Validate("SkillDefinition", skill); e != nil {
			t.Fatal(e)
		}
		if skill.SkillHash == strings.Repeat("0", 64) {
			t.Fatal("placeholder skill hash")
		}
	}
	for _, dep := range snapshot.SchemaDependencies {
		b, e := r.SchemaBytes(snapshot, dep.SchemaURI)
		if e != nil {
			t.Fatal(e)
		}
		if contracts.BytesHash(b) != dep.SHA256 || contracts.BytesHash(release.Schemas[dep.SchemaURI]) != dep.SHA256 {
			t.Fatal("schema original bytes not retained")
		}
	}
	if snapshot.Definition.BudgetPolicy.MaxReconciliationAttempts != 64 || snapshot.Definition.BudgetPolicy.MaxExecutionSeconds != 7200 {
		t.Fatal("extended resource policy not frozen")
	}
	if e := r.Publish(release); e != nil {
		t.Fatal("identical release must be idempotent", e)
	}
}

func TestVersionsCoexistAndOldSnapshotReadsOldBytes(t *testing.T) {
	r := openTest(t)
	_, old := publishTest(t, r, testOptions("3.0.0"))
	oldFiles, e := r.TemplateFiles(old, "scaffold")
	if e != nil {
		t.Fatal(e)
	}
	opts := testOptions("3.1.0")
	opts.SystemPrompt = "New immutable prompt."
	opts.Templates[0].Files["render-program.json"] = []byte(`{"format_version":1,"rules":[{"path":"new.go","source":"package changed"}]}`)
	_, newSnapshot := publishTest(t, r, opts)
	if old.DefinitionHash == newSnapshot.DefinitionHash {
		t.Fatal("changed definition has old identity")
	}
	reopened, e := Open(r.root)
	if e != nil {
		t.Fatal(e)
	}
	oldAgain, e := reopened.Freeze(old.DefinitionID, old.DefinitionVersion, old.DefinitionHash, nil, time.Date(2026, 10, 3, 0, 0, 0, 0, time.UTC))
	if e != nil {
		t.Fatal(e)
	}
	if oldAgain.Definition.SystemPrompt.Content != old.Definition.SystemPrompt.Content {
		t.Fatal("old prompt upgraded")
	}
	files, e := reopened.TemplateFiles(old, "scaffold")
	if e != nil {
		t.Fatal(e)
	}
	if string(files["render-program.json"]) != string(oldFiles["render-program.json"]) {
		t.Fatal("old template upgraded")
	}
	files["render-program.json"][0] = 'x'
	filesAgain, e := reopened.TemplateFiles(old, "scaffold")
	if e != nil || filesAgain["render-program.json"][0] == 'x' {
		t.Fatal("template reader aliases caller memory")
	}
}

func TestSameVersionDifferentDefinitionSkillOrTemplateRefused(t *testing.T) {
	for _, kind := range []string{"definition", "skill", "template"} {
		t.Run(kind, func(t *testing.T) {
			r := openTest(t)
			publishTest(t, r, testOptions("3.0.0"))
			opts := testOptions("3.1.0")
			switch kind {
			case "definition":
				opts.DefinitionVersion = "3.0.0"
				opts.SystemPrompt = "changed prompt"
			case "skill":
				opts.SkillVersion = "3.0.0"
			case "template":
				opts.Templates[0].Version = "3.0.0"
				opts.Templates[0].Files["render-program.json"] = []byte("changed bytes")
			}
			release, e := BuildDefault(opts)
			if e != nil {
				t.Fatal(e)
			}
			if e = r.Publish(release); e == nil || !strings.Contains(e.Error(), "conflict") {
				t.Fatalf("same-version replacement accepted: %v", e)
			}
		})
	}
}

func TestMissingOrTamperedDependencyBlocksWithoutFallback(t *testing.T) {
	for _, kind := range []string{"template", "schema", "skill_index", "missing"} {
		t.Run(kind, func(t *testing.T) {
			r := openTest(t)
			release, snapshot := publishTest(t, r, testOptions("3.0.0"))
			file := filepath.Join(r.root, "blobs", snapshot.TemplateDependencies[0].Files[0].SHA256)
			if kind == "schema" {
				file = filepath.Join(r.root, "blobs", snapshot.SchemaDependencies[0].SHA256)
			}
			if kind == "skill_index" {
				p, _ := pairPath("skills", release.Skills[0].SkillID, release.Skills[0].SkillVersion)
				file = filepath.Join(r.root, p)
			}
			var e error
			if kind == "missing" {
				e = os.Remove(file)
			} else {
				e = os.WriteFile(file, []byte("tampered"), 0600)
			}
			if e != nil {
				t.Fatal(e)
			}
			if e = r.VerifySnapshot(snapshot); e == nil {
				t.Fatal("incomplete/tampered frozen closure accepted")
			}
			if _, e = r.TemplateFiles(snapshot, "scaffold"); e == nil {
				t.Fatal("template loader silently fell back")
			}
		})
	}
}

func TestPublicationRejectsHashScopeAndUnboundTool(t *testing.T) {
	for _, kind := range []string{"prompt", "skill_hash", "template_bytes", "schema_bytes", "command", "tool"} {
		t.Run(kind, func(t *testing.T) {
			r := openTest(t)
			release, e := BuildDefault(testOptions("3.0.0"))
			if e != nil {
				t.Fatal(e)
			}
			switch kind {
			case "prompt":
				release.Definition.SystemPrompt.Content += " altered"
			case "skill_hash":
				release.Skills[0].SkillHash = strings.Repeat("a", 64)
			case "template_bytes":
				release.TemplateContents[release.TemplateDependencies[0].Files[0].SHA256] = []byte("bad")
			case "schema_bytes":
				release.Schemas["agent.schema.json"] = []byte("{}")
			case "command":
				release.Definition.PostCommands[0].Arguments = contracts.PostCommandArguments(`["build","./..."]`)
				release.Definition.DefinitionHash, _ = contracts.HashObject("AgentDefinition", release.Definition)
			case "tool":
				release.ToolBindings[0].Version = "go1.26.0"
			}
			if e = r.Publish(release); e == nil {
				t.Fatal("invalid publication accepted")
			}
		})
	}
}

func TestRegisteredParameterNormalizationAndSemanticBounds(t *testing.T) {
	r := openTest(t)
	_, s := publishTest(t, r, testOptions("3.0.0"))
	for _, id := range []string{"add_database", "add_auth", "add_logging"} {
		if _, e := r.NormalizeParameters(s, id, nil); e != nil {
			t.Fatal(id, e)
		}
	}
	p, e := r.NormalizeParameters(s, "add_database", map[string]any{"database": "sqlite", "orm": "sqlx"})
	if e != nil || p["database"] != "sqlite" {
		t.Fatalf("legal combination: %v", e)
	}
	cases := []struct {
		id     string
		params map[string]any
	}{
		{"add_database", map[string]any{"database": true}}, {"add_database", map[string]any{"shell": "echo unsafe"}},
		{"add_auth", map[string]any{"token_ttl": "0s"}}, {"add_auth", map[string]any{"token_ttl": "721h"}}, {"add_auth", map[string]any{"token_ttl": nil}},
		{"add_logging", map[string]any{"log_max_size": true}}, {"add_logging", map[string]any{"log_max_backups": 101}},
		{"add_logging", map[string]any{"log_max_age": "30m"}}, {"add_logging", map[string]any{"log_path": "logs/../go.mod"}}, {"add_logging", map[string]any{"log_path": "logs/a//b"}},
	}
	for _, c := range cases {
		if _, e = r.NormalizeParameters(s, c.id, c.params); e == nil {
			t.Errorf("invalid parameters accepted: %s %v", c.id, c.params)
		}
	}
	if e = r.ValidateSelection(s, []string{"add_auth", "add_auth"}); e == nil {
		t.Fatal("duplicate selection accepted")
	}
	if e = r.ValidateSelection(s, []string{"dynamic_shell"}); e == nil {
		t.Fatal("unknown selection accepted")
	}
	if e = r.ValidateSelection(s, []string{"add_auth", "add_database", "add_logging"}); e != nil {
		t.Fatal(e)
	}
}

func TestSnapshotTamperingAndProviderBindingRefused(t *testing.T) {
	r := openTest(t)
	_, s := publishTest(t, r, testOptions("3.0.0"))
	s.Definition.SystemPrompt.Content = "mutated"
	s.SnapshotHash, _ = contracts.HashObject("DefinitionSnapshot", s)
	if e := r.VerifySnapshot(s); e == nil {
		t.Fatal("rehashed altered closure accepted")
	}
	_, e := r.Freeze(s.DefinitionID, s.DefinitionVersion, s.DefinitionHash, &contracts.ProviderBinding{ProviderKind: "openai_compatible", Endpoint: "https://user:secret@example.invalid", ModelID: "model"}, time.Now())
	if e == nil {
		t.Fatal("credential endpoint accepted")
	}
}

func TestPublicContentReaderBindsSnapshot(t *testing.T) {
	r := openTest(t)
	_, a := publishTest(t, r, testOptions("3.0.0"))
	opts := testOptions("3.1.0")
	opts.Templates[0].Files["render-program.json"] = []byte("distinct newer template")
	_, b := publishTest(t, r, opts)
	list, e := r.List()
	if e != nil || len(list) != 2 {
		t.Fatalf("published list: %d %v", len(list), e)
	}
	ref := b.TemplateDependencies[0].Files[0].ContentRef
	// Pick a changed source explicitly, as sorting puts auth first.
	for _, dep := range b.TemplateDependencies {
		if dep.TemplateID == "scaffold" {
			ref = dep.Files[0].ContentRef
		}
	}
	if _, e = r.Content(a, ref.ObjectType, ref.ObjectID, ref.Hash); e == nil {
		t.Fatal("unbound content readable through old snapshot")
	}
	if _, e = r.Content(b, ref.ObjectType, ref.ObjectID, ref.Hash); e != nil {
		t.Fatal(e)
	}
	schema := a.SchemaDependencies[0]
	if _, e = r.Content(a, "schema", schema.SchemaURI, schema.SHA256); e != nil {
		t.Fatal(e)
	}
	if _, e = r.Content(a, "template_content", "../outside", schema.SHA256); e == nil {
		t.Fatal("arbitrary reference accepted")
	}
}

func TestBundledPublicationAssetsHaveCompleteActualHashes(t *testing.T) {
	data, e := assets.Sources.ReadFile("definitions/release.json")
	if e != nil {
		t.Fatal(e)
	}
	var release Release
	if e = json.Unmarshal(data, &release); e != nil {
		t.Fatal(e)
	}
	release.Schemas, e = contracts.SchemaFilesFor(release.Definition.ContractVersion)
	if e != nil {
		t.Fatal(e)
	}
	release.TemplateContents = map[string][]byte{}
	for _, dep := range release.TemplateDependencies {
		for _, file := range dep.Files {
			p := filepath.ToSlash(filepath.Join("templates", "golang", dep.TemplateID, dep.TemplateVersion, file.Path))
			b, e := assets.Sources.ReadFile(p)
			if e != nil {
				t.Fatal(e)
			}
			release.TemplateContents[file.SHA256] = b
		}
	}
	r := openTest(t)
	if e = r.Publish(release); e != nil {
		t.Fatal(e)
	}
	for _, skill := range release.Skills {
		b, e := assets.Sources.ReadFile("skills/" + skill.SkillID + "/skill.json")
		if e != nil {
			t.Fatal(e)
		}
		var actual contracts.SkillDefinition
		if e = contracts.Decode("SkillDefinition", b, &actual); e != nil {
			t.Fatal(e)
		}
		if !equalJSON(skill, actual) {
			t.Fatal("standalone skill asset differs from published release")
		}
	}
	b, e := assets.Sources.ReadFile("definitions/agent.json")
	if e != nil {
		t.Fatal(e)
	}
	var def contracts.AgentDefinition
	if e = contracts.Decode("AgentDefinition", b, &def); e != nil {
		t.Fatal(e)
	}
	if !equalJSON(def, release.Definition) {
		t.Fatal("standalone definition differs from published release")
	}
}

func TestRevokedDependencyBlocksBoundSnapshotAndSurvivesRestart(t *testing.T) {
	for _, kind := range []string{"definition", "skill", "template", "schema"} {
		t.Run(kind, func(t *testing.T) {
			r := openTest(t)
			_, snapshot := publishTest(t, r, testOptions("3.0.0"))
			id, version := snapshot.DefinitionID, snapshot.DefinitionVersion
			switch kind {
			case "skill":
				id, version = snapshot.Skills[0].SkillID, snapshot.Skills[0].SkillVersion
			case "template":
				id, version = snapshot.TemplateDependencies[0].TemplateID, snapshot.TemplateDependencies[0].TemplateVersion
			case "schema":
				id, version = snapshot.SchemaDependencies[0].SchemaURI, snapshot.SchemaDependencies[0].ContractVersion
			}
			if e := r.Revoke(kind, id, version, "test dependency withdrawal"); e != nil {
				t.Fatal(e)
			}
			reopened, e := Open(r.root)
			if e != nil {
				t.Fatal(e)
			}
			if e = reopened.VerifySnapshot(snapshot); e == nil || !strings.Contains(e.Error(), "revoked") {
				t.Fatalf("revoked identity accepted: %v", e)
			}
			if _, e = reopened.Freeze(snapshot.DefinitionID, snapshot.DefinitionVersion, snapshot.DefinitionHash, nil, time.Now()); e == nil {
				t.Fatal("new snapshot from revoked dependency")
			}
		})
	}
}

func TestPersistedPublicationRejectsDuplicateJSONKeys(t *testing.T) {
	r := openTest(t)
	_, s := publishTest(t, r, testOptions("3.0.0"))
	p, _ := pairPath("definitions", s.DefinitionID, s.DefinitionVersion)
	file := filepath.Join(r.root, p)
	data, e := os.ReadFile(file)
	if e != nil {
		t.Fatal(e)
	}
	data = bytes.Replace(data, []byte(`"definition":`), []byte(`"definition":null,"definition":`), 1)
	if e = os.WriteFile(file, data, 0600); e != nil {
		t.Fatal(e)
	}
	if e = r.VerifySnapshot(s); e == nil {
		t.Fatal("duplicate JSON key accepted in persisted publication")
	}
}
