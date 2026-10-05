package agent

import (
	"encoding/json"
	"sort"
	"strings"

	"agentstudio/internal/contracts"
)

func projectPlanFor(r Run, p Plan) (contracts.ProjectPlan, error) {
	if r.FrozenDefinition == nil {
		return contracts.ProjectPlan{}, fail("legacy_contract_incomplete", "缺少完整冻结定义，不调用当前模型补造历史")
	}
	x := contracts.ProjectPlan{ContractVersion: r.FrozenDefinition.ContractVersion, ProjectName: p.ProjectName, Description: p.Description, TechStack: p.TechStack, SelectedSkills: []contracts.SkillSelection{}, TemplateDependencies: r.FrozenDefinition.TemplateDependencies}
	target, _ := json.Marshal(contracts.LocalTarget{Provider: "local", Destination: "run_package"})
	x.DeliveryTarget = contracts.DeliveryTarget(target)
	for _, id := range p.Skills {
		found := false
		for _, skill := range r.FrozenDefinition.Skills {
			if skill.SkillID != id {
				continue
			}
			found = true
			selection := map[string]any{"skill_id": id, "skill_version": skill.SkillVersion, "skill_hash": skill.SkillHash, "parameters": planSkillParameters(p, id)}
			b, err := json.Marshal(selection)
			if err != nil {
				return x, err
			}
			x.SelectedSkills = append(x.SelectedSkills, contracts.SkillSelection(b))
		}
		if !found {
			return x, fail("invalid_skill", "技能未登记")
		}
	}
	sort.Slice(x.SelectedSkills, func(i, j int) bool {
		var a, b struct {
			SkillID string `json:"skill_id"`
		}
		_ = json.Unmarshal(x.SelectedSkills[i], &a)
		_ = json.Unmarshal(x.SelectedSkills[j], &b)
		return a.SkillID < b.SkillID
	})
	var err error
	x.PlanHash, err = contracts.HashObject("ProjectPlan", x)
	return x, err
}

// A provider proposes content; the service verifies every dependency against
// the bound definition and derives the compatibility Plan itself.
func (s *Service) compatibilityPlan(r Run, proposed contracts.ProjectPlan, expectedTarget contracts.DeliveryTarget) (Plan, error) {
	p := r.Plan
	p.ProjectName, p.Description, p.TechStack = proposed.ProjectName, proposed.Description, proposed.TechStack
	p.Skills = []string{}
	if err := contracts.Validate("ProjectPlan", proposed); err != nil {
		return p, fail("model_schema_invalid", "模型方案结构非法")
	}
	if proposed.VerificationCandidateRef != nil {
		return p, fail("revision_not_allowed", "普通规划不能自报验证候选来源；必须使用验证修订入口")
	}
	a, err := contracts.HashValue(proposed.TemplateDependencies)
	if err != nil {
		return p, err
	}
	b, err := contracts.HashValue(r.FrozenDefinition.TemplateDependencies)
	if err != nil {
		return p, err
	}
	if a != b {
		return p, fail("model_schema_invalid", "模型改变了冻结模板引用")
	}
	a, err = contracts.HashValue(proposed.DeliveryTarget)
	if err != nil {
		return p, err
	}
	b, err = contracts.HashValue(expectedTarget)
	if err != nil {
		return p, err
	}
	if a != b {
		return p, fail("model_schema_invalid", "模型不能替换精确交付目标")
	}
	for _, raw := range proposed.SelectedSkills {
		var fields struct {
			SkillID      string         `json:"skill_id"`
			SkillVersion string         `json:"skill_version"`
			SkillHash    string         `json:"skill_hash"`
			Parameters   map[string]any `json:"parameters"`
		}
		if err = json.Unmarshal(raw, &fields); err != nil {
			return p, err
		}
		found := false
		for _, skill := range r.FrozenDefinition.Skills {
			if skill.SkillID == fields.SkillID && skill.SkillVersion == fields.SkillVersion && skill.SkillHash == fields.SkillHash {
				found = true
			}
		}
		if !found {
			return p, fail("model_schema_invalid", "模型技能引用与绑定定义不一致")
		}
		p.Skills = append(p.Skills, fields.SkillID)
		params, err := s.Definitions.NormalizeParameters(*r.FrozenDefinition, fields.SkillID, fields.Parameters)
		if err != nil {
			return p, fail("model_schema_invalid", "模型参数与技能Schema不一致")
		}
		data, err := json.Marshal(params)
		if err != nil {
			return p, err
		}
		if err = json.Unmarshal(data, &p); err != nil {
			return p, err
		}
	}
	p, err = s.normalizeFrozenPlan(r, p)
	if err != nil {
		return p, err
	}
	if err = ValidatePlan(p); err != nil {
		return p, fail("model_schema_invalid", err.Error())
	}
	// The model-supplied hash is never accepted as content identity.
	canonical, err := projectPlanFor(r, p)
	if err != nil {
		return p, err
	}
	canonical.DeliveryTarget = expectedTarget
	canonical.PlanHash, err = contracts.HashObject("ProjectPlan", canonical)
	return p, err
}

func frozenSkillCatalog(snapshot *contracts.DefinitionSnapshot) any {
	items := []map[string]any{}
	for _, skill := range snapshot.Skills {
		items = append(items, map[string]any{"skill_id": skill.SkillID, "skill_version": skill.SkillVersion, "skill_hash": skill.SkillHash, "defaults": json.RawMessage(skill.DefaultParameters)})
	}
	return items
}

func strictModelContent(content string) string {
	// Protocol responses are JSON only. Markdown fences are not silently
	// removed because doing so would accept a different wire format.
	return strings.TrimSpace(content)
}
