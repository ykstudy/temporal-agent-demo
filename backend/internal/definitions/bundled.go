package definitions

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"path"

	"agentstudio/assets"
	"agentstudio/internal/contracts"
)

// LoadBundledRelease reads an exact product publication. It does not rebuild a
// version from today's template code, adjust tool identity, or select a default.
func LoadBundledRelease(version string) (Release, error) {
	return loadBundledRelease(assets.Sources, version)
}

func bundledPrefix(version string) (string, error) {
	switch version {
	case "3.0.1":
		return "", nil
	case "3.1.0", "3.1.1", "3.1.2", "3.2.0", "3.3.0":
		return "releases/" + version, nil
	default:
		return "", fmt.Errorf("unknown bundled publication %q", version)
	}
}

func loadBundledRelease(source fs.FS, version string) (Release, error) {
	var release Release
	prefix, err := bundledPrefix(version)
	if err != nil {
		return release, err
	}
	read := func(relative string) ([]byte, error) { return fs.ReadFile(source, path.Join(prefix, relative)) }
	data, err := read("definitions/release.json")
	if err != nil {
		return release, err
	}
	if _, err = contracts.ParseJSON(data); err != nil {
		return release, err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&release); err != nil {
		return release, err
	}
	if release.Definition.DefinitionVersion != version {
		return release, fmt.Errorf("bundled definition version mismatch")
	}
	release.Schemas, err = contracts.SchemaFilesFor(release.Definition.ContractVersion)
	if err != nil {
		return release, err
	}
	data, err = read("definitions/agent.json")
	if err != nil {
		return release, err
	}
	var definition contracts.AgentDefinition
	if err = contracts.DecodeVersion("AgentDefinition", release.Definition.ContractVersion, data, &definition); err != nil {
		return release, err
	}
	if !equalJSON(definition, release.Definition) {
		return release, fmt.Errorf("bundled agent declaration differs from release")
	}
	for _, skill := range release.Skills {
		if !safeName.MatchString(skill.SkillID) {
			return release, fmt.Errorf("bundled Skill identity mismatch")
		}
		skillPrefix, e := bundledPrefix(skill.SkillVersion)
		if e != nil {
			return release, e
		}
		data, err = fs.ReadFile(source, path.Join(skillPrefix, "skills", skill.SkillID, "skill.json"))
		if err != nil {
			return release, err
		}
		var declared contracts.SkillDefinition
		if err = contracts.DecodeVersion("SkillDefinition", release.Definition.ContractVersion, data, &declared); err != nil {
			return release, err
		}
		if !equalJSON(declared, skill) {
			return release, fmt.Errorf("bundled Skill declaration differs from release")
		}
	}
	release.TemplateContents = map[string][]byte{}
	for _, dep := range release.TemplateDependencies {
		if !safeName.MatchString(dep.TemplateID) {
			return release, fmt.Errorf("bundled template identity mismatch")
		}
		templatePrefix, e := bundledPrefix(dep.TemplateVersion)
		if e != nil {
			return release, e
		}
		for _, file := range dep.Files {
			if !fs.ValidPath(file.Path) {
				return release, fmt.Errorf("bundled template path invalid")
			}
			body, err := fs.ReadFile(source, path.Join(templatePrefix, "templates/golang", dep.TemplateID, dep.TemplateVersion, file.Path))
			if err != nil {
				return release, err
			}
			if contracts.BytesHash(body) != file.SHA256 {
				return release, fmt.Errorf("bundled template bytes differ from publication")
			}
			release.TemplateContents[file.SHA256] = body
		}
	}
	if err = validateRelease(release, func(hash string) ([]byte, error) {
		if b, ok := release.TemplateContents[hash]; ok {
			return b, nil
		}
		for _, b := range release.Schemas {
			if contracts.BytesHash(b) == hash {
				return b, nil
			}
		}
		return nil, fmt.Errorf("bundled publication blob missing")
	}); err != nil {
		return Release{}, err
	}
	return release, nil
}

// PublishBundled registers this exact verified closure, preserving all existing
// definitions and already frozen Runs. Choosing it as default is a separate act.
func (r *Registry) PublishBundled(version string) (contracts.AgentDefinition, error) {
	release, err := LoadBundledRelease(version)
	if err != nil {
		return contracts.AgentDefinition{}, err
	}
	if err = r.Publish(release); err != nil {
		return contracts.AgentDefinition{}, err
	}
	return release.Definition, nil
}
