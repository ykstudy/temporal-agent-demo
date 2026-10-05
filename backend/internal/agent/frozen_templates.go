package agent

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path"
	"sort"
	"strconv"
	"strings"
	"text/template"

	"agentstudio/internal/contracts"
	"agentstudio/internal/definitions"
)

// The immutable program is data, not executable code. Its only operations are
// matching declared enum values and rendering fixed relative file paths with
// pure text/template helpers. This interpreter remains versioned independently
// from the template bytes published into the definition registry.
type templateProgram struct {
	FormatVersion int            `json:"format_version"`
	Rules         []templateRule `json:"rules"`
}
type templateRule struct {
	Path    string            `json:"path"`
	Source  string            `json:"source"`
	Append  bool              `json:"append,omitempty"`
	Literal bool              `json:"literal,omitempty"`
	When    map[string]string `json:"when,omitempty"`
}

// releaseTemplatePrograms is used at publication only. Old Runs load programs
// from their registered immutable blobs and never call this function.
func releaseTemplatePrograms() ([]definitions.TemplateBundle, error) {
	programs := map[string]templateProgram{}
	base := baseTemplateSources()
	for name, source := range base {
		if name == "Dockerfile" {
			source = strings.ReplaceAll(source, "CGO_ENABLED=0", `CGO_ENABLED={{if and (skill "add_database") (eq .Database "sqlite")}}1{{else}}0{{end}}`)
			source = strings.ReplaceAll(source, "distroless/static-debian12", `distroless/{{if and (skill "add_database") (eq .Database "sqlite")}}base{{else}}static{{end}}-debian12`)
		}
		p := programs["scaffold"]
		p.Rules = append(p.Rules, templateRule{Path: name, Source: source})
		programs["scaffold"] = p
	}
	// Database variants contain no user-controlled free text. Freeze every
	// supported variant's actual source and pinned dependency versions once.
	for _, db := range []string{"mysql", "postgres", "sqlite"} {
		for _, orm := range []string{"gorm", "sqlx"} {
			p := defaultPlan()
			p.ProjectName = "template-release"
			p.Skills = []string{"add_database"}
			p.Database, p.ORM = db, orm
			files, err := RenderFiles(p)
			if err != nil {
				return nil, err
			}
			condition := map[string]string{"database": db, "orm": orm}
			program := programs["database"]
			for name, content := range files {
				if strings.HasPrefix(name, "internal/database/") || strings.HasPrefix(name, "migrations/") {
					program.Rules = append(program.Rules, templateRule{Path: name, Source: content, Literal: true, When: condition})
				}
			}
			prefix := "module template-release\n\ngo 1.25.0\n"
			if !strings.HasPrefix(files["go.mod"], prefix) {
				return nil, fmt.Errorf("unexpected module template")
			}
			program.Rules = append(program.Rules, templateRule{Path: "go.mod", Source: strings.TrimPrefix(files["go.mod"], prefix), Append: true, When: condition})
			programs["database"] = program
		}
	}
	programs["auth"] = templateProgram{Rules: []templateRule{
		{Path: "internal/auth/auth.go", Source: strings.ReplaceAll(jwtSource, "TTL_VALUE", "{{.TokenTTL}}"), When: map[string]string{"auth": "jwt"}},
		{Path: "internal/auth/auth_test.go", Source: jwtTests, Literal: true, When: map[string]string{"auth": "jwt"}},
		{Path: "internal/auth/auth.go", Source: strings.ReplaceAll(sessionSource, "TTL_VALUE", "{{.TokenTTL}}"), When: map[string]string{"auth": "session"}},
		{Path: "internal/auth/auth_test.go", Source: sessionTests, Literal: true, When: map[string]string{"auth": "session"}},
	}}
	programs["logging"] = templateProgram{Rules: []templateRule{
		{Path: "internal/logging/logging.go", Source: loggingSource},
		{Path: "internal/logging/logging_test.go", Source: loggingTests},
	}}
	bundles := []definitions.TemplateBundle{}
	for _, id := range []string{"scaffold", "database", "auth", "logging"} {
		program := programs[id]
		program.FormatVersion = 1
		sort.SliceStable(program.Rules, func(i, j int) bool {
			a, b := program.Rules[i], program.Rules[j]
			if a.Path != b.Path {
				return a.Path < b.Path
			}
			ak, _ := json.Marshal(a.When)
			bk, _ := json.Marshal(b.When)
			return string(ak) < string(bk)
		})
		body, err := json.Marshal(program)
		if err != nil {
			return nil, err
		}
		bundles = append(bundles, definitions.TemplateBundle{ID: id, Version: currentDefinitionVersion, Files: map[string][]byte{"render-program.json": body}})
	}
	return bundles, nil
}

func (s *Service) renderFrozenFiles(r Run) (map[string]string, error) {
	if r.FrozenDefinition == nil {
		return nil, fail("legacy_contract_incomplete", "旧Run缺完整模板来源；请创建关联新Run")
	}
	if err := s.Definitions.VerifySnapshot(*r.FrozenDefinition); err != nil {
		return nil, fail("definition_mismatch", "冻结定义或依赖内容不可核实")
	}
	p, err := s.normalizeFrozenPlan(r, r.Plan)
	if err != nil {
		return nil, err
	}
	check := p
	if err = ValidatePlan(check); err != nil {
		return nil, err
	}
	files := map[string]string{}
	for _, id := range []string{"scaffold", "database", "auth", "logging"} {
		if id != "scaffold" && !hasSkill(p, "add_"+id) {
			continue
		}
		contents, err := s.Definitions.TemplateFiles(*r.FrozenDefinition, id)
		if err != nil {
			return nil, fail("definition_mismatch", "冻结模板内容缺失或损坏")
		}
		body, ok := contents["render-program.json"]
		if !ok {
			return nil, fail("definition_mismatch", "冻结模板缺渲染程序")
		}
		// Strict JSON prevents duplicate keys or unknown fields from changing
		// the interpretation of a content-addressed program after review.
		if _, err = contracts.Canonicalize(body); err != nil {
			return nil, err
		}
		var program templateProgram
		decoder := json.NewDecoder(bytes.NewReader(body))
		decoder.DisallowUnknownFields()
		if err = decoder.Decode(&program); err != nil {
			return nil, err
		}
		if program.FormatVersion != 1 || len(program.Rules) > 128 {
			return nil, fail("unsupported_template", "不支持的冻结模板程序")
		}
		for _, rule := range program.Rules {
			matches := true
			for key, value := range rule.When {
				actual := ""
				switch key {
				case "database":
					actual = p.Database
				case "orm":
					actual = p.ORM
				case "auth":
					actual = p.Auth
				default:
					return nil, fail("invalid_template", "非法模板条件")
				}
				if value != actual {
					matches = false
				}
			}
			if !matches {
				continue
			}
			if rule.Path == "" || path.IsAbs(rule.Path) || path.Clean(rule.Path) != rule.Path || strings.HasPrefix(rule.Path, "../") || strings.ContainsAny(rule.Path, "\\\x00") {
				return nil, fail("invalid_template", "模板输出路径越界")
			}
			if !templateGroupAllowsFor(r.FrozenDefinition.ContractVersion, id, rule.Path, rule.Append) {
				return nil, fail("invalid_template", "模板输出超出登记技能路径")
			}
			if id != "scaffold" {
				declared := false
				for _, skill := range r.FrozenDefinition.Skills {
					if skill.SkillID != "add_"+id {
						continue
					}
					for _, allowed := range skill.WritePaths {
						if string(allowed) == rule.Path {
							declared = true
						}
					}
				}
				if !declared {
					return nil, fail("invalid_template", "模板输出未在冻结技能路径中声明")
				}
			}
			_, exists := files[rule.Path]
			if rule.Append && !exists || !rule.Append && exists {
				return nil, fail("invalid_template", "模板写入顺序或路径冲突")
			}
			if rule.Literal {
				if rule.Append {
					files[rule.Path] += rule.Source
				} else {
					files[rule.Path] = rule.Source
				}
				continue
			}
			t, err := template.New(rule.Path).Option("missingkey=error").Funcs(template.FuncMap{"quote": strconv.Quote, "skill": func(id string) bool { return hasSkill(p, id) }}).Parse(rule.Source)
			if err != nil {
				return nil, err
			}
			var out bytes.Buffer
			if err = t.Execute(&out, p); err != nil {
				return nil, err
			}
			if rule.Append {
				files[rule.Path] += out.String()
			} else {
				files[rule.Path] = out.String()
			}
		}
	}
	return files, nil
}

func templateGroupAllows(group, name string, appendContent bool) bool {
	switch group {
	case "database":
		return name == "go.mod" && appendContent || !appendContent && (strings.HasPrefix(name, "internal/database/") || strings.HasPrefix(name, "migrations/"))
	case "auth":
		return !appendContent && strings.HasPrefix(name, "internal/auth/")
	case "logging":
		return !appendContent && strings.HasPrefix(name, "internal/logging/")
	case "scaffold":
		if appendContent {
			return false
		}
		for _, allowed := range []string{"go.mod", "cmd/server/main.go", "internal/server/server.go", "internal/server/server_test.go", ".gitignore", ".env.example", ".gitlab-ci.yml", "Dockerfile", "README.md"} {
			if name == allowed {
				return true
			}
		}
	}
	return false
}

// Contract 1.2 publications may freeze exact go.sum bytes before approval. The
// original 1.1 interpreter never gains a new write path from a newer Worker.
func templateGroupAllowsFor(contractVersion, group, name string, appendContent bool) bool {
	if (contractVersion == "1.2.0" || contracts.DeliveryContract(contractVersion)) && name == "go.sum" {
		return group == "scaffold" && !appendContent || group == "database" && appendContent
	}
	return templateGroupAllows(group, name, appendContent)
}
