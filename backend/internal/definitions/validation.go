package definitions

import (
	"encoding/json"
	"fmt"
	"path"
	"sort"
	"strings"
	"time"

	"agentstudio/internal/contracts"
)

func validateRelease(release Release, readBlob func(string) ([]byte, error)) error {
	d := release.Definition
	version := d.ContractVersion
	trustedSchemas, schemaErr := contracts.SchemaFilesFor(version)
	if schemaErr != nil {
		return fmt.Errorf("unsupported definition contract: %w", schemaErr)
	}
	if e := contracts.ValidateVersion("AgentDefinition", version, d); e != nil {
		return fmt.Errorf("definition schema: %w", e)
	}
	hash, e := contracts.HashObject("AgentDefinition", d)
	if e != nil || hash != d.DefinitionHash {
		return fmt.Errorf("definition content hash mismatch")
	}
	if contracts.BytesHash([]byte(d.SystemPrompt.Content)) != d.SystemPrompt.SHA256 {
		return fmt.Errorf("system prompt content hash mismatch")
	}
	schemaHashes := map[string]string{}
	schemaDocs := map[string]map[string]any{}
	previous := ""
	for _, dep := range release.SchemaDependencies {
		if dep.SchemaURI <= previous || schemaHashes[dep.SchemaURI] != "" {
			return fmt.Errorf("duplicate or unsorted schema dependency")
		}
		previous = dep.SchemaURI
		if dep.ContractVersion != version {
			return fmt.Errorf("schema contract mismatch")
		}
		b, e := readBlob(dep.SHA256)
		if e != nil {
			return e
		}
		if contracts.BytesHash(b) != dep.SHA256 {
			return fmt.Errorf("schema content hash mismatch")
		}
		if trusted, ok := trustedSchemas[dep.SchemaURI]; !ok || contracts.BytesHash(trusted) != dep.SHA256 {
			return fmt.Errorf("schema differs from immutable contract publication")
		}
		var doc map[string]any
		if e = json.Unmarshal(b, &doc); e != nil {
			return e
		}
		expectedID := "https://agentstudio.invalid/contracts/" + version + "/" + dep.SchemaURI
		if doc["$id"] != expectedID {
			return fmt.Errorf("schema stable identity mismatch")
		}
		schemaHashes[dep.SchemaURI] = dep.SHA256
		schemaDocs[dep.SchemaURI] = doc
	}
	for _, name := range []string{"agent.schema.json", "common.schema.json", "message.schema.json"} {
		if schemaHashes[name] == "" {
			return fmt.Errorf("missing schema dependency: %s", name)
		}
	}
	if len(schemaHashes) != 3 {
		return fmt.Errorf("unexpected schema dependency")
	}
	checkRef := func(ref contracts.SchemaReference) error {
		file := schemaName(ref.SchemaRef)
		if schemaHashes[file] != ref.FileSHA256 {
			return fmt.Errorf("schema reference hash mismatch")
		}
		parts := strings.Split(ref.SchemaRef, "#/$defs/")
		if len(parts) != 2 {
			return fmt.Errorf("unsupported schema reference")
		}
		defs, _ := schemaDocs[file]["$defs"].(map[string]any)
		if _, ok := defs[parts[1]]; !ok {
			return fmt.Errorf("unresolved schema reference")
		}
		return nil
	}
	expectedProtocol := map[string]string{
		d.ActionProtocol.ModelOutputSchema.SchemaRef:      "agent.schema.json#/$defs/DecisionAdvice",
		d.ActionProtocol.DecisionProposalSchema.SchemaRef: "agent.schema.json#/$defs/DecisionProposal",
		d.ActionProtocol.ActionValidationSchema.SchemaRef: "agent.schema.json#/$defs/ActionValidation",
		d.ActionProtocol.ExecutionRequestSchema.SchemaRef: "agent.schema.json#/$defs/ExecutionRequest",
	}
	if version == "1.4.0" {
		p := d.ContextPolicy
		if contracts.BytesHash([]byte(p.SummaryPrompt.Content)) != p.SummaryPrompt.SHA256 || p.SummaryOutputSchema.SchemaRef != "agent.schema.json#/$defs/SummaryAdvice" {
			return fmt.Errorf("summary protocol differs from frozen publication")
		}
		if err := checkRef(p.SummaryOutputSchema); err != nil {
			return err
		}
	}
	if len(expectedProtocol) != 4 {
		return fmt.Errorf("action protocol schemas must bind distinct published objects")
	}
	for actual, expected := range expectedProtocol {
		if actual != expected {
			return fmt.Errorf("action protocol schema role mismatch")
		}
	}
	for _, ref := range []contracts.SchemaReference{d.ActionProtocol.ModelOutputSchema, d.ActionProtocol.DecisionProposalSchema, d.ActionProtocol.ActionValidationSchema, d.ActionProtocol.ExecutionRequestSchema} {
		if e = checkRef(ref); e != nil {
			return e
		}
	}
	templates := map[string]contracts.TemplateDependency{}
	previous = ""
	for _, dep := range release.TemplateDependencies {
		key := dep.TemplateID + "@" + dep.TemplateVersion
		if key <= previous || templates[key].TemplateID != "" {
			return fmt.Errorf("duplicate or unsorted template dependency")
		}
		previous = key
		if e = contracts.ValidateVersion("TemplateDependency", version, dep); e != nil {
			return e
		}
		hash, e := contracts.HashObject("TemplateDependency", dep)
		if e != nil || hash != dep.ManifestHash {
			return fmt.Errorf("template manifest hash mismatch")
		}
		priorPath := ""
		for _, file := range dep.Files {
			if file.Path <= priorPath {
				return fmt.Errorf("duplicate or unsorted template file")
			}
			priorPath = file.Path
			if path.Clean(file.Path) != file.Path || strings.HasPrefix(file.Path, "/") {
				return fmt.Errorf("unsafe template file identity")
			}
			ref := file.ContentRef
			if ref.ObjectType != "template_content" || ref.ObjectID != "template_content_"+file.SHA256 || ref.ObjectVersion != 1 || ref.SourceVersion != 1 || ref.Hash != file.SHA256 {
				return fmt.Errorf("template content reference mismatch")
			}
			b, e := readBlob(file.SHA256)
			if e != nil {
				return e
			}
			if contracts.BytesHash(b) != file.SHA256 {
				return fmt.Errorf("template content hash mismatch")
			}
		}
		templates[key] = dep
	}
	if len(templates) == 0 {
		return fmt.Errorf("missing template dependencies")
	}
	tools := map[string]contracts.ToolIdentity{}
	previous = ""
	for _, tool := range release.ToolBindings {
		platform := ""
		if tool.Platform != nil {
			platform = *tool.Platform
		}
		key := tool.ToolID + "@" + platform
		if key <= previous {
			return fmt.Errorf("duplicate or unsorted tool binding")
		}
		previous = key
		if e = contracts.ValidateVersion("ToolIdentity", version, tool); e != nil {
			return e
		}
		tools[key] = tool
	}
	checkTools := func(required []contracts.ToolIdentity) error {
		for _, tool := range required {
			platform := ""
			if tool.Platform != nil {
				platform = *tool.Platform
			}
			actual, ok := tools[tool.ToolID+"@"+platform]
			if !ok || !equalJSON(actual, tool) {
				return fmt.Errorf("unbound tool identity")
			}
		}
		return nil
	}
	if e = checkTools(d.RequiredTools); e != nil {
		return e
	}
	if e = validateCommands(d.PostCommands); e != nil {
		return e
	}
	completion := map[string]bool{}
	previous = ""
	for _, c := range d.CompletionPolicy.RequiredChecks {
		if c.CheckID <= previous {
			return fmt.Errorf("duplicate or unsorted required check")
		}
		previous = c.CheckID
		completion[c.CheckID] = true
	}
	skills := map[string]contracts.SkillDefinition{}
	previous = ""
	for _, skill := range release.Skills {
		if skill.SkillID <= previous {
			return fmt.Errorf("duplicate or unsorted skills")
		}
		previous = skill.SkillID
		if e = contracts.ValidateVersion("SkillDefinition", version, skill); e != nil {
			return fmt.Errorf("skill schema: %w", e)
		}
		hash, e := contracts.HashObject("SkillDefinition", skill)
		if e != nil || hash != skill.SkillHash {
			return fmt.Errorf("skill content hash mismatch")
		}
		expectedInput := map[string]string{"add_database": "DatabaseParameters", "add_auth": "AuthenticationParameters", "add_logging": "LoggingParameters"}[skill.SkillID]
		if skill.InputContract.SchemaRef != "agent.schema.json#/$defs/"+expectedInput || skill.OutputContract.SchemaRef != "agent.schema.json#/$defs/ArtifactManifest" {
			return fmt.Errorf("skill input/output schema role mismatch")
		}
		if e = checkRef(skill.InputContract); e != nil {
			return e
		}
		if e = checkRef(skill.OutputContract); e != nil {
			return e
		}
		for _, dep := range skill.TemplateDependencies {
			actual, ok := templates[dep.TemplateID+"@"+dep.TemplateVersion]
			if !ok || !equalJSON(actual, dep) {
				return fmt.Errorf("skill template dependency mismatch")
			}
		}
		if e = checkTools(skill.RequiredTools); e != nil {
			return e
		}
		if e = validateCommands(skill.PostCommands); e != nil {
			return e
		}
		if !sortedUnique(skill.RequiredPermissions) || !sortedUnique(skill.RequiredCheckIDs) || !sortedUnique(skill.WritePaths) {
			return fmt.Errorf("skill sets must be sorted and unique")
		}
		for _, id := range skill.RequiredCheckIDs {
			if !completion[id] {
				return fmt.Errorf("skill requires unknown completion check")
			}
		}
		defaults, e := objectMap(skill.DefaultParameters)
		if e != nil {
			return e
		}
		if e = validateParameters(skill, defaults); e != nil {
			return fmt.Errorf("invalid skill defaults: %w", e)
		}
		parameterSchema, e := objectMap(skill.ParametersSchema)
		if e != nil {
			return e
		}
		props, _ := parameterSchema["properties"].(map[string]any)
		for name, defaultValue := range defaults {
			prop, ok := props[name].(map[string]any)
			if !ok || !equalJSON(prop["default"], defaultValue) {
				return fmt.Errorf("parameter schema default differs from registered default")
			}
		}
		skills[skill.SkillID] = skill
	}
	if len(skills) != 3 || len(d.AvailableSkills) != 3 {
		return fmt.Errorf("definition requires exactly three registered skills")
	}
	previous = ""
	for _, ref := range d.AvailableSkills {
		if ref.SkillID <= previous {
			return fmt.Errorf("duplicate or unsorted skill references")
		}
		previous = ref.SkillID
		s, ok := skills[ref.SkillID]
		if !ok || s.SkillVersion != ref.SkillVersion || s.SkillHash != ref.SkillHash {
			return fmt.Errorf("definition skill binding mismatch")
		}
	}
	// Dependencies must resolve to immutable exact versions and be acyclic.
	colors := map[string]int{}
	var visit func(string) error
	visit = func(id string) error {
		if colors[id] == 1 {
			return fmt.Errorf("skill dependency cycle")
		}
		if colors[id] == 2 {
			return nil
		}
		colors[id] = 1
		s := skills[id]
		seen := map[string]bool{}
		for _, dep := range s.Dependencies {
			actual, ok := skills[dep.SkillID]
			if !ok || actual.SkillVersion != dep.SkillVersion || actual.SkillHash != dep.SkillHash || seen[dep.SkillID] {
				return fmt.Errorf("invalid skill dependency binding")
			}
			seen[dep.SkillID] = true
			for _, conflict := range s.Conflicts {
				if conflict == dep.SkillID {
					return fmt.Errorf("skill dependency conflicts with required skill")
				}
			}
			if e := visit(dep.SkillID); e != nil {
				return e
			}
		}
		colors[id] = 2
		return nil
	}
	for id, s := range skills {
		for _, conflict := range s.Conflicts {
			if conflict == id || skills[conflict].SkillID == "" {
				return fmt.Errorf("invalid skill conflict")
			}
		}
		if e = visit(id); e != nil {
			return e
		}
	}
	return nil
}

func validateCommands(commands []contracts.PostCommand) error {
	allowed := map[string][]string{"go.mod_tidy": {"mod", "tidy"}, "go.test": {"test", "./..."}, "go.build": {"build", "./..."}}
	seen := map[string]bool{}
	for _, command := range commands {
		expected, ok := allowed[command.CommandID]
		var actual []string
		if !ok || seen[command.CommandID] || json.Unmarshal(command.Arguments, &actual) != nil || !equalJSON(expected, actual) {
			return fmt.Errorf("post-command identity does not match fixed arguments")
		}
		seen[command.CommandID] = true
	}
	return nil
}

func validateParameters(skill contracts.SkillDefinition, params map[string]any) error {
	if e := contracts.ValidateDocumentVersion(skill.ContractVersion, skill.ParametersSchema, params); e != nil {
		return e
	}
	parameterType := map[string]string{"add_database": "DatabaseParameters", "add_auth": "AuthenticationParameters", "add_logging": "LoggingParameters"}[skill.SkillID]
	if parameterType == "" {
		return fmt.Errorf("unknown skill")
	}
	if e := contracts.ValidateVersion(parameterType, skill.ContractVersion, params); e != nil {
		return e
	}
	switch skill.SkillID {
	case "add_auth":
		raw, _ := params["token_ttl"].(string)
		d, e := time.ParseDuration(raw)
		if e != nil || d <= 0 || d > 30*24*time.Hour {
			return fmt.Errorf("token_ttl must be between 1ns and 720h")
		}
	case "add_logging":
		p, _ := params["log_path"].(string)
		if !strings.HasPrefix(p, "logs/") || path.Clean(p) != p || len(p) > 160 || strings.ContainsAny(p, "\\\"\n\r\t") {
			return fmt.Errorf("log_path must remain in a separate logs directory")
		}
		for _, part := range strings.Split(p, "/") {
			if part == "" || part == "." || part == ".." {
				return fmt.Errorf("invalid log path component")
			}
		}
		raw, _ := params["log_max_age"].(string)
		age, e := time.ParseDuration(raw)
		if e != nil || age < time.Hour || age > 365*24*time.Hour {
			return fmt.Errorf("log_max_age must be between 1h and 8760h")
		}
	}
	return nil
}

func (r *Registry) NormalizeParameters(snapshot contracts.DefinitionSnapshot, skillID string, input map[string]any) (map[string]any, error) {
	if e := r.VerifySnapshot(snapshot); e != nil {
		return nil, e
	}
	for _, skill := range snapshot.Skills {
		if skill.SkillID == skillID {
			params, e := objectMap(skill.DefaultParameters)
			if e != nil {
				return nil, e
			}
			for key, value := range input {
				if _, ok := params[key]; !ok {
					return nil, fmt.Errorf("unknown parameter %s", key)
				}
				params[key] = value
			}
			if e = validateParameters(skill, params); e != nil {
				return nil, e
			}
			return params, nil
		}
	}
	return nil, fmt.Errorf("skill is not in bound definition")
}

// ValidateSelection verifies dependency/conflict semantics for the chosen set.
func (r *Registry) ValidateSelection(snapshot contracts.DefinitionSnapshot, ids []string) error {
	if e := r.VerifySnapshot(snapshot); e != nil {
		return e
	}
	selected := map[string]bool{}
	available := map[string]contracts.SkillDefinition{}
	for _, s := range snapshot.Skills {
		available[s.SkillID] = s
	}
	for _, id := range ids {
		if selected[id] || available[id].SkillID == "" {
			return fmt.Errorf("unknown or duplicate selected skill")
		}
		selected[id] = true
	}
	for id := range selected {
		skill := available[id]
		for _, dep := range skill.Dependencies {
			if !selected[dep.SkillID] {
				return fmt.Errorf("selected skill dependency missing")
			}
		}
		for _, conflict := range skill.Conflicts {
			if selected[conflict] {
				return fmt.Errorf("selected skills conflict")
			}
		}
	}
	return nil
}

func normalizeRelease(release *Release) {
	sort.Slice(release.Skills, func(i, j int) bool { return release.Skills[i].SkillID < release.Skills[j].SkillID })
	sort.Slice(release.Definition.AvailableSkills, func(i, j int) bool {
		return release.Definition.AvailableSkills[i].SkillID < release.Definition.AvailableSkills[j].SkillID
	})
	sort.Slice(release.SchemaDependencies, func(i, j int) bool {
		return release.SchemaDependencies[i].SchemaURI < release.SchemaDependencies[j].SchemaURI
	})
	sort.Slice(release.TemplateDependencies, func(i, j int) bool {
		a, b := release.TemplateDependencies[i], release.TemplateDependencies[j]
		if a.TemplateID != b.TemplateID {
			return a.TemplateID < b.TemplateID
		}
		return a.TemplateVersion < b.TemplateVersion
	})
	sortTools(release.ToolBindings)
	sortTools(release.Definition.RequiredTools)
	sort.Slice(release.Definition.CompletionPolicy.RequiredChecks, func(i, j int) bool {
		return release.Definition.CompletionPolicy.RequiredChecks[i].CheckID < release.Definition.CompletionPolicy.RequiredChecks[j].CheckID
	})
}
