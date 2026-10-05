package definitions

import (
	"io/fs"
	"testing"
	"testing/fstest"
	"time"

	"agentstudio/assets"
	"agentstudio/internal/contracts"
)

func TestBundled310PublishesExactClosureWithoutChangingOldVersion(t *testing.T) {
	old, err := LoadBundledRelease("3.0.1")
	if err != nil {
		t.Fatal(err)
	}
	if old.Definition.DefinitionHash != "45cfaa983628b2ca23c9e2eb9a537d7428a4ce1d6078ab68eae0f8e36de01608" {
		t.Fatal("original publication changed")
	}
	release, err := LoadBundledRelease("3.1.0")
	if err != nil {
		t.Fatal(err)
	}
	if release.Definition.ContractVersion != "1.2.0" || release.Definition.DefinitionVersion != "3.1.0" {
		t.Fatal("wrong release")
	}
	if !equalJSON(release.ToolBindings, old.ToolBindings) || !equalJSON(release.Definition.PostCommands, old.Definition.PostCommands) {
		t.Fatal("publication silently changed tools or commands")
	}
	for i, skill := range release.Skills {
		if skill.SkillVersion != "3.1.0" || skill.ContractVersion != "1.2.0" || !equalJSON(skill.RequiredPermissions, old.Skills[i].RequiredPermissions) || !equalJSON(skill.PostCommands, old.Skills[i].PostCommands) {
			t.Fatal("skill changed tool authority")
		}
		if skill.SkillID == "add_database" {
			found := false
			for _, p := range skill.WritePaths {
				found = found || p == "go.sum"
			}
			if !found {
				t.Fatal("database sum output not declared")
			}
		}
	}
	r := openTest(t)
	if err = r.Publish(old); err != nil {
		t.Fatal(err)
	}
	d, err := r.PublishBundled("3.1.0")
	if err != nil {
		t.Fatal(err)
	}
	frozen, err := r.Freeze(d.DefinitionID, d.DefinitionVersion, d.DefinitionHash, nil, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(r.root)
	if err != nil {
		t.Fatal(err)
	}
	if err = reopened.VerifySnapshot(frozen); err != nil {
		t.Fatal(err)
	}
	for _, dep := range frozen.TemplateDependencies {
		bodies, err := reopened.TemplateFiles(frozen, dep.TemplateID)
		if err != nil {
			t.Fatal(err)
		}
		for _, file := range dep.Files {
			if contracts.BytesHash(bodies[file.Path]) != file.SHA256 {
				t.Fatal("restart changed template bytes")
			}
		}
	}
	if _, err = reopened.Freeze(old.Definition.DefinitionID, "3.0.1", old.Definition.DefinitionHash, nil, time.Now().UTC()); err != nil {
		t.Fatal("old source lost", err)
	}
	if _, err = LoadBundledRelease("../3.1.0"); err == nil {
		t.Fatal("path alias accepted")
	}
	if _, err = LoadBundledRelease("9.9.9"); err == nil {
		t.Fatal("unknown version fell back")
	}
}
func TestBundled310RejectsMissingOrChangedPublicationBytes(t *testing.T) {
	source := fstest.MapFS{}
	err := fs.WalkDir(assets.Sources, "releases/3.1.0", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		body, err := assets.Sources.ReadFile(p)
		if err != nil {
			return err
		}
		source[p] = &fstest.MapFile{Data: body}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"definitions/agent.json", "definitions/release.json", "skills/add_database/skill.json", "templates/golang/database/3.1.0/render-program.json"} {
		t.Run(name, func(t *testing.T) {
			path := "releases/3.1.0/" + name
			original := source[path]
			delete(source, path)
			if _, err := loadBundledRelease(source, "3.1.0"); err == nil {
				t.Fatal("missing source accepted")
			}
			source[path] = &fstest.MapFile{Data: []byte(`{"unexpected":true}`)}
			if _, err := loadBundledRelease(source, "3.1.0"); err == nil {
				t.Fatal("modified source accepted")
			}
			source[path] = original
		})
	}
}

func TestBundled311RevisesOnlyAgentAndReusesExact310Dependencies(t *testing.T) {
	previous, err := LoadBundledRelease("3.1.0")
	if err != nil {
		t.Fatal(err)
	}
	current, err := LoadBundledRelease("3.1.1")
	if err != nil {
		t.Fatal(err)
	}
	expected := previous.Definition
	expected.DefinitionVersion = current.Definition.DefinitionVersion
	expected.DefinitionHash = current.Definition.DefinitionHash
	expected.SystemPrompt = current.Definition.SystemPrompt
	if !equalJSON(expected, current.Definition) || current.Definition.DefinitionVersion != "3.1.1" || current.Definition.ContractVersion != "1.2.0" {
		t.Fatal("agent-only revision changed policies or identities")
	}
	if current.Definition.SystemPrompt.Content == previous.Definition.SystemPrompt.Content || current.Definition.DefinitionHash == previous.Definition.DefinitionHash {
		t.Fatal("new prompt was not independently versioned")
	}
	if !equalJSON(previous.Skills, current.Skills) || !equalJSON(previous.TemplateDependencies, current.TemplateDependencies) || !equalJSON(previous.ToolBindings, current.ToolBindings) || !equalJSON(previous.SchemaDependencies, current.SchemaDependencies) || !equalJSON(previous.Schemas, current.Schemas) || !equalJSON(previous.TemplateContents, current.TemplateContents) {
		t.Fatal("agent revision replaced exact published dependencies")
	}
	r := openTest(t)
	for _, version := range []string{"3.0.1", "3.1.0", "3.1.1"} {
		if _, err = r.PublishBundled(version); err != nil {
			t.Fatal(err)
		}
	}
	reopened, err := Open(r.root)
	if err != nil {
		t.Fatal(err)
	}
	for _, version := range []string{"3.0.1", "3.1.0", "3.1.1"} {
		release, err := LoadBundledRelease(version)
		if err != nil {
			t.Fatal(err)
		}
		snapshot, err := reopened.Freeze(release.Definition.DefinitionID, version, release.Definition.DefinitionHash, nil, time.Now().UTC())
		if err != nil {
			t.Fatal(err)
		}
		if err = reopened.VerifySnapshot(snapshot); err != nil {
			t.Fatal(err)
		}
	}
	source := fstest.MapFS{}
	err = fs.WalkDir(assets.Sources, "releases", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		body, err := assets.Sources.ReadFile(p)
		if err != nil {
			return err
		}
		source[p] = &fstest.MapFile{Data: body}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	delete(source, "releases/3.1.0/templates/golang/database/3.1.0/render-program.json")
	if _, err = loadBundledRelease(source, "3.1.1"); err == nil {
		t.Fatal("Agent-only revision silently replaced a missing exact 3.1.0 dependency")
	}
}
