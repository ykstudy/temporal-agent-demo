package definitions

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"agentstudio/assets"
	"agentstudio/internal/contracts"
)

func TestContractReleasesCoexistAndFreezeAfterReopen(t *testing.T) {
	r := openTest(t)
	oldRelease, old := publishTest(t, r, testOptions("3.0.1"))
	if old.ContractVersion != "1.1.0" {
		t.Fatal("default publication silently moved an old definition to the new contract")
	}
	oldPath, err := pairPath("definitions", old.DefinitionID, old.DefinitionVersion)
	if err != nil {
		t.Fatal(err)
	}
	oldBytes, err := r.read(oldPath)
	if err != nil {
		t.Fatal(err)
	}
	opts := testOptions("3.2.0")
	opts.ContractVersion = "1.2.0"
	opts.SystemPrompt = "A new immutable 1.2.0 publication."
	newRelease, newer := publishTest(t, r, opts)
	if newer.ContractVersion != "1.2.0" || newer.Definition.ContractVersion != "1.2.0" {
		t.Fatal("explicit publication did not bind the requested release")
	}
	for _, skill := range newer.Skills {
		if skill.ContractVersion != newer.ContractVersion {
			t.Fatal("new definition mixed contract versions")
		}
	}
	if err = r.Publish(oldRelease); err != nil {
		t.Fatalf("new publication made identical old publication invalid: %v", err)
	}
	stored, err := r.read(oldPath)
	if err != nil || !bytes.Equal(oldBytes, stored) {
		t.Fatal("old publication bytes were overwritten")
	}
	reopened, err := Open(r.root)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		snapshot contracts.DefinitionSnapshot
		release  Release
	}{{old, oldRelease}, {newer, newRelease}} {
		snapshot := tc.snapshot
		if err := reopened.VerifySnapshot(snapshot); err != nil {
			t.Fatalf("cannot verify persisted %s closure: %v", snapshot.ContractVersion, err)
		}
		boundAt, err := time.Parse(time.RFC3339Nano, snapshot.BoundAt)
		if err != nil {
			t.Fatal(err)
		}
		again, err := reopened.Freeze(snapshot.DefinitionID, snapshot.DefinitionVersion, snapshot.DefinitionHash, nil, boundAt)
		if err != nil || !equalJSON(snapshot, again) {
			t.Fatalf("restart changed %s frozen bytes/hash: %v", snapshot.ContractVersion, err)
		}
		for _, dependency := range snapshot.SchemaDependencies {
			if dependency.ContractVersion != snapshot.ContractVersion {
				t.Fatal("schema dependency upgraded")
			}
			contents, err := reopened.SchemaBytes(snapshot, dependency.SchemaURI)
			if err != nil || !bytes.Equal(contents, tc.release.Schemas[dependency.SchemaURI]) {
				t.Fatalf("bound schema read selected another release: %v", err)
			}
		}
	}
	oldSchema, err := reopened.SchemaBytes(old, "agent.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	newSchema, err := reopened.SchemaBytes(newer, "agent.schema.json")
	if err != nil || bytes.Equal(oldSchema, newSchema) {
		t.Fatal("different schema releases lost their separate bytes")
	}
}

func TestPublishedMixedVersionClosuresAreRejected(t *testing.T) {
	for _, change := range []struct {
		name   string
		mutate func(*Release)
	}{
		{"old_skill_inside_new_definition", func(r *Release) {
			r.Skills[0].ContractVersion = "1.1.0"
			r.Skills[0].SkillHash, _ = contracts.HashObject("SkillDefinition", r.Skills[0])
			for i := range r.Definition.AvailableSkills {
				if r.Definition.AvailableSkills[i].SkillID == r.Skills[0].SkillID {
					r.Definition.AvailableSkills[i].SkillHash = r.Skills[0].SkillHash
				}
			}
		}},
		{"schema_version_mismatch", func(r *Release) { r.SchemaDependencies[0].ContractVersion = "1.1.0" }},
		{"old_schema_bytes_with_new_identity", func(r *Release) {
			old, _ := contracts.SchemaFilesFor("1.1.0")
			r.Schemas = old
			for i := range r.SchemaDependencies {
				r.SchemaDependencies[i].SHA256 = contracts.BytesHash(old[r.SchemaDependencies[i].SchemaURI])
			}
		}},
		{"unknown_contract", func(r *Release) { r.Definition.ContractVersion = "9.9.9" }},
	} {
		t.Run(change.name, func(t *testing.T) {
			r := openTest(t)
			opts := testOptions("3.2.0")
			opts.ContractVersion = "1.2.0"
			release, err := BuildDefault(opts)
			if err != nil {
				t.Fatal(err)
			}
			change.mutate(&release)
			release.Definition.DefinitionHash, err = contracts.HashObject("AgentDefinition", release.Definition)
			if err != nil {
				t.Fatal(err)
			}
			if err = r.Publish(release); err == nil {
				t.Fatal("content hashes concealed a mixed release closure")
			}
		})
	}
	opts := testOptions("3.2.0")
	// 1.4.0 is now an actual frozen publication. Keep the unknown-version
	// rejection independent of the next legitimate release number.
	opts.ContractVersion = "9.9.9"
	if _, err := BuildDefault(opts); err == nil {
		t.Fatal("unknown contract version silently used the current schema")
	}
}

func TestOriginalBundledPublicationRemainsByteIdentical(t *testing.T) {
	data, err := assets.Sources.ReadFile("definitions/release.json")
	if err != nil {
		t.Fatal(err)
	}
	const originalBytesHash = "650b88ffbf1fb54fdef59d3cd1c613b40f22e39fa10303036133cc5d7938987f"
	if contracts.BytesHash(data) != originalBytesHash {
		t.Fatal("original 3.0.1 publication bytes changed; publish new versions separately")
	}
	var original Release
	if err = json.Unmarshal(data, &original); err != nil {
		t.Fatal(err)
	}
	d := original.Definition
	opts := PublishOptions{DefinitionID: d.DefinitionID, DefinitionVersion: d.DefinitionVersion, SkillVersion: original.Skills[0].SkillVersion, TemplateVersion: original.TemplateDependencies[0].TemplateVersion, SystemPrompt: d.SystemPrompt.Content, Tools: d.RequiredTools}
	for _, dep := range original.TemplateDependencies {
		bundle := TemplateBundle{ID: dep.TemplateID, Version: dep.TemplateVersion, Files: map[string][]byte{}}
		for _, file := range dep.Files {
			content, err := assets.Sources.ReadFile(filepath.ToSlash(filepath.Join("templates", "golang", dep.TemplateID, dep.TemplateVersion, file.Path)))
			if err != nil {
				t.Fatal(err)
			}
			bundle.Files[file.Path] = content
		}
		opts.Templates = append(opts.Templates, bundle)
	}
	rebuilt, err := BuildDefault(opts)
	if err != nil {
		t.Fatal(err)
	}
	if rebuilt.Definition.ContractVersion != "1.1.0" || !equalJSON(original, rebuilt) {
		t.Fatal("the default builder changed original publication content or hashes")
	}
	if rebuilt.Definition.DefinitionHash != "45cfaa983628b2ca23c9e2eb9a537d7428a4ce1d6078ab68eae0f8e36de01608" {
		t.Fatal("the original definition hash changed")
	}
}
