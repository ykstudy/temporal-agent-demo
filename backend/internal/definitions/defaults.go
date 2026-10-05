package definitions

import (
	"fmt"
	"sort"

	"agentstudio/assets"
	"agentstudio/internal/contracts"
)

type PublishOptions struct {
	// Empty keeps the original 1.1.0 publication policy. Moving a definition
	// to another contract is an explicit new publication, never a read adapter.
	ContractVersion   string
	DefinitionID      string
	DefinitionVersion string
	SkillVersion      string
	TemplateVersion   string
	SystemPrompt      string
	Tools             []contracts.ToolIdentity
	Templates         []TemplateBundle
}

// BuildDefault resolves the complete bundled publication declarations. The
// returned release is schema/hash verified again by Publish before persistence.
func BuildDefault(opts PublishOptions) (Release, error) {
	var result Release
	if opts.ContractVersion == "" {
		opts.ContractVersion = ContractVersion
	}
	var schemaErr error
	result.Schemas, schemaErr = contracts.SchemaFilesFor(opts.ContractVersion)
	if schemaErr != nil {
		return result, schemaErr
	}
	if opts.DefinitionID == "" {
		opts.DefinitionID = "project_creation_agent"
	}
	if opts.DefinitionVersion == "" {
		opts.DefinitionVersion = "3.0.0"
	}
	if opts.SkillVersion == "" {
		opts.SkillVersion = "3.0.0"
	}
	if opts.TemplateVersion == "" {
		opts.TemplateVersion = "3.0.0"
	}
	if len(opts.Tools) == 0 {
		return result, fmt.Errorf("publication requires concrete tool identities")
	}
	result.ToolBindings = append([]contracts.ToolIdentity{}, opts.Tools...)
	sortTools(result.ToolBindings)
	result.TemplateContents = map[string][]byte{}
	for uri, contents := range result.Schemas {
		result.SchemaDependencies = append(result.SchemaDependencies, contracts.SchemaDependency{SchemaURI: uri, ContractVersion: opts.ContractVersion, SHA256: contracts.BytesHash(contents)})
	}
	templateIndex := map[string]contracts.TemplateDependency{}
	for _, bundle := range opts.Templates {
		if bundle.Version == "" {
			bundle.Version = opts.TemplateVersion
		}
		if templateIndex[bundle.ID].TemplateID != "" {
			return result, fmt.Errorf("duplicate template bundle")
		}
		dep := contracts.TemplateDependency{TemplateID: bundle.ID, TemplateVersion: bundle.Version, Files: []contracts.TemplateFile{}}
		for name, content := range bundle.Files {
			hash := contracts.BytesHash(content)
			result.TemplateContents[hash] = append([]byte{}, content...)
			dep.Files = append(dep.Files, contracts.TemplateFile{Path: name, SHA256: hash, ContentRef: contracts.TemplateFileContentRef{ObjectType: "template_content", ObjectID: "template_content_" + hash, ObjectVersion: 1, SourceVersion: 1, Hash: hash}})
		}
		sort.Slice(dep.Files, func(i, j int) bool { return dep.Files[i].Path < dep.Files[j].Path })
		var e error
		dep.ManifestHash, e = contracts.HashObject("TemplateDependency", dep)
		if e != nil {
			return result, e
		}
		templateIndex[bundle.ID] = dep
		result.TemplateDependencies = append(result.TemplateDependencies, dep)
	}
	for _, required := range []string{"scaffold", "database", "auth", "logging"} {
		if templateIndex[required].TemplateID == "" {
			return result, fmt.Errorf("required template bundle missing: %s", required)
		}
	}
	if len(templateIndex) != 4 {
		return result, fmt.Errorf("only registered scaffold and three Skill bundles are supported")
	}
	agentRaw, e := assets.Sources.ReadFile("definitions/agent.json")
	if e != nil {
		return result, e
	}
	// Publication declarations are resolved through the full generated type; no
	// slim DTO or ad-hoc public object is substituted for the published contract.
	if e = contracts.Decode("AgentDefinition", agentRaw, &result.Definition); e != nil {
		return result, e
	}
	d := &result.Definition
	sourceHash, e := contracts.HashObject("AgentDefinition", *d)
	if e != nil || sourceHash != d.DefinitionHash || contracts.BytesHash([]byte(d.SystemPrompt.Content)) != d.SystemPrompt.SHA256 {
		return result, fmt.Errorf("bundled definition publication hash mismatch")
	}
	d.ContractVersion = opts.ContractVersion
	d.DefinitionID = opts.DefinitionID
	d.DefinitionVersion = opts.DefinitionVersion
	if opts.SystemPrompt != "" {
		d.SystemPrompt.Content = opts.SystemPrompt
	}
	d.SystemPrompt.SHA256 = contracts.BytesHash([]byte(d.SystemPrompt.Content))
	d.RequiredTools = append([]contracts.ToolIdentity{}, result.ToolBindings...)
	d.AvailableSkills = []contracts.SkillReference{}
	d.BudgetPolicy.MaxToolSeconds = 720
	d.BudgetPolicy.MaxExecutionSeconds = 7200
	d.BudgetPolicy.MaxCIWaitSeconds = 1800
	d.BudgetPolicy.MaxCIPollAttempts = 128
	d.BudgetPolicy.MaxReconciliationSeconds = 1800
	d.BudgetPolicy.MaxReconciliationAttempts = 64
	schemaHash := contracts.BytesHash(result.Schemas["agent.schema.json"])
	d.ActionProtocol.ModelOutputSchema.FileSHA256 = schemaHash
	d.ActionProtocol.DecisionProposalSchema.FileSHA256 = schemaHash
	d.ActionProtocol.ActionValidationSchema.FileSHA256 = schemaHash
	d.ActionProtocol.ExecutionRequestSchema.FileSHA256 = schemaHash
	for _, id := range []string{"add_auth", "add_database", "add_logging"} {
		data, e := assets.Sources.ReadFile("skills/" + id + "/skill.json")
		if e != nil {
			return result, e
		}
		var skill contracts.SkillDefinition
		if e = contracts.Decode("SkillDefinition", data, &skill); e != nil {
			return result, e
		}
		sourceSkillHash, e := contracts.HashObject("SkillDefinition", skill)
		if e != nil || sourceSkillHash != skill.SkillHash {
			return result, fmt.Errorf("bundled skill publication hash mismatch")
		}
		skill.ContractVersion = opts.ContractVersion
		skill.SkillVersion = opts.SkillVersion
		skill.InputContract.FileSHA256 = schemaHash
		skill.OutputContract.FileSHA256 = schemaHash
		bundleID := map[string]string{"add_auth": "auth", "add_database": "database", "add_logging": "logging"}[id]
		skill.TemplateDependencies = []contracts.TemplateDependency{templateIndex[bundleID]}
		skill.RequiredTools = append([]contracts.ToolIdentity{}, result.ToolBindings...)
		sort.Strings(skill.RequiredPermissions)
		sort.Strings(skill.RequiredCheckIDs)
		sort.Strings(skill.WritePaths)
		skill.SkillHash, e = contracts.HashObject("SkillDefinition", skill)
		if e != nil {
			return result, e
		}
		result.Skills = append(result.Skills, skill)
		d.AvailableSkills = append(d.AvailableSkills, contracts.SkillReference{SkillID: id, SkillVersion: skill.SkillVersion, SkillHash: skill.SkillHash})
	}
	normalizeRelease(&result)
	result.Definition.DefinitionHash, e = contracts.HashObject("AgentDefinition", result.Definition)
	if e != nil {
		return result, e
	}
	return result, nil
}

func (r *Registry) PublishDefault(opts PublishOptions) (contracts.AgentDefinition, error) {
	release, e := BuildDefault(opts)
	if e != nil {
		return contracts.AgentDefinition{}, e
	}
	if e = r.Publish(release); e != nil {
		return contracts.AgentDefinition{}, e
	}
	return release.Definition, nil
}
