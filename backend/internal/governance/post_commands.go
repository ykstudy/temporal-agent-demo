package governance

import "agentstudio/internal/contracts"

// MergePostCommands resolves the approved definition and selected Skill command
// declarations. First-declaration order is preserved; duplicate commands are
// executed once with the strictest bound. Definition closure verification and
// action authorization still belong to Validator, before calling the executor.
func MergePostCommands(snapshot contracts.DefinitionSnapshot, plan contracts.ProjectPlan) ([]contracts.PostCommand, error) {
	if err := checkHash("DefinitionSnapshot", snapshot, snapshot.SnapshotHash); err != nil {
		return nil, err
	}
	if err := checkHash("ProjectPlan", plan, plan.PlanHash); err != nil {
		return nil, err
	}
	if snapshot.ContractVersion != plan.ContractVersion || !equalJSON(snapshot.TemplateDependencies, plan.TemplateDependencies) {
		return nil, reject("plan_dependency_mismatch", "post commands require the approved frozen plan dependencies")
	}
	declarations := append([]contracts.PostCommand{}, snapshot.Definition.PostCommands...)
	for _, raw := range plan.SelectedSkills {
		var selection selectionFields
		if err := selection.UnmarshalJSON(raw); err != nil {
			return nil, err
		}
		skill := findSkill(snapshot, selection.SkillID)
		if skill == nil || skill.SkillVersion != selection.SkillVersion || skill.SkillHash != selection.SkillHash {
			return nil, reject("invalid_skill_selection", "post command skill differs from the selected frozen skill")
		}
		declarations = append(declarations, skill.PostCommands...)
	}
	fixed := map[string][]string{"go.mod_tidy": {"mod", "tidy"}, "go.test": {"test", "./..."}, "go.build": {"build", "./..."}}
	out := []contracts.PostCommand{}
	indexes := map[string]int{}
	for _, command := range declarations {
		if contracts.Validate("PostCommand", command) != nil || command.Executable != "go" || command.CwdScope != "run_workspace" || command.EnvironmentPolicy != "isolated" || fixed[command.CommandID] == nil || !equalJSON(command.Arguments, fixed[command.CommandID]) {
			return nil, reject("invalid_tool_command", "command ID, exact argv, workspace and environment must match the registered command")
		}
		command.Arguments = append(contracts.PostCommandArguments(nil), command.Arguments...)
		command.TimeoutSeconds = minInt(command.TimeoutSeconds, snapshot.Definition.BudgetPolicy.MaxToolSeconds)
		command.MaxAttempts = minInt(command.MaxAttempts, snapshot.Definition.BudgetPolicy.MaxStepAttempts)
		if index, exists := indexes[command.CommandID]; exists {
			previous := out[index]
			if command.Executable != previous.Executable || command.CwdScope != previous.CwdScope || command.EnvironmentPolicy != previous.EnvironmentPolicy || !equalJSON(command.Arguments, previous.Arguments) {
				return nil, reject("post_command_conflict", "same command identity has conflicting execution semantics")
			}
			out[index].TimeoutSeconds = minInt(previous.TimeoutSeconds, command.TimeoutSeconds)
			out[index].MaxAttempts = minInt(previous.MaxAttempts, command.MaxAttempts)
		} else {
			indexes[command.CommandID] = len(out)
			out = append(out, command)
		}
	}
	return out, nil
}
