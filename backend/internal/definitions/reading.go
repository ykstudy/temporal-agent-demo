package definitions

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"agentstudio/internal/contracts"
)

// List verifies every published closure before returning immutable definitions.
func (r *Registry) List() ([]contracts.AgentDefinition, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	files, e := filepath.Glob(filepath.Join(r.root, "definitions", "*", "*.json"))
	if e != nil {
		return nil, e
	}
	sort.Strings(files)
	out := make([]contracts.AgentDefinition, 0, len(files))
	for _, file := range files {
		rel, e := filepath.Rel(r.root, file)
		if e != nil {
			return nil, e
		}
		b, e := r.read(rel)
		if e != nil {
			return nil, e
		}
		var saved Release
		if e = json.Unmarshal(b, &saved); e != nil {
			return nil, e
		}
		d := saved.Definition
		release, e := r.load(d.DefinitionID, d.DefinitionVersion, d.DefinitionHash)
		if e != nil {
			return nil, e
		}
		out = append(out, release.Definition)
	}
	return out, nil
}

// Content allows only exact published content references present in the given
// snapshot. Knowing another blob's digest never grants access through this API.
func (r *Registry) Content(snapshot contracts.DefinitionSnapshot, objectType, objectID, hash string) ([]byte, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if e := r.verifySnapshot(snapshot); e != nil {
		return nil, e
	}
	switch objectType {
	case "template_content":
		for _, dep := range snapshot.TemplateDependencies {
			for _, file := range dep.Files {
				ref := file.ContentRef
				if ref.ObjectID == objectID && ref.Hash == hash && ref.ObjectType == objectType {
					return r.blob(file.SHA256)
				}
			}
		}
	case "schema":
		for _, dep := range snapshot.SchemaDependencies {
			if dep.SchemaURI == objectID && dep.SHA256 == hash {
				return r.blob(dep.SHA256)
			}
		}
	case "template":
		for _, dep := range snapshot.TemplateDependencies {
			if dep.TemplateID == objectID && dep.ManifestHash == hash {
				return canonical(dep)
			}
		}
	default:
		return nil, fmt.Errorf("unsupported published content reference type")
	}
	return nil, fmt.Errorf("content reference is not in bound definition")
}

// ExportReleaseAssets materializes an already verified complete publication as
// product-owned, reviewable source assets. This is a developer publishing API,
// never an HTTP endpoint and never called while an Agent Run is executing.
func ExportReleaseAssets(root string, release Release) error {
	if e := validateRelease(release, func(hash string) ([]byte, error) {
		if b, ok := release.TemplateContents[hash]; ok {
			return b, nil
		}
		for _, b := range release.Schemas {
			if contracts.BytesHash(b) == hash {
				return b, nil
			}
		}
		return nil, fmt.Errorf("publication blob missing")
	}); e != nil {
		return e
	}
	writeJSON := func(relative string, value any) error {
		data, e := json.MarshalIndent(value, "", "  ")
		if e != nil {
			return e
		}
		return writeAsset(root, relative, append(data, '\n'))
	}
	if e := writeJSON("definitions/agent.json", release.Definition); e != nil {
		return e
	}
	if e := writeJSON("definitions/release.json", release); e != nil {
		return e
	}
	for _, skill := range release.Skills {
		if e := writeJSON(filepath.Join("skills", skill.SkillID, "skill.json"), skill); e != nil {
			return e
		}
	}
	for _, dep := range release.TemplateDependencies {
		for _, file := range dep.Files {
			b, ok := release.TemplateContents[file.SHA256]
			if !ok {
				return fmt.Errorf("template publication bytes missing")
			}
			if e := writeAsset(root, filepath.Join("templates", "golang", dep.TemplateID, dep.TemplateVersion, file.Path), b); e != nil {
				return e
			}
		}
	}
	return nil
}
func writeAsset(root, relative string, b []byte) error {
	p := filepath.Join(root, relative)
	if e := os.MkdirAll(filepath.Dir(p), 0755); e != nil {
		return e
	}
	return os.WriteFile(p, b, 0644)
}
