package governance

import (
	"encoding/json"
	"errors"
	"sort"
	"testing"
	"time"

	"agentstudio/internal/contracts"
	"agentstudio/internal/definitions"
)

func fixture(t *testing.T) (Validator, Input) {
	t.Helper()
	opts := definitions.PublishOptions{ContractVersion: contracts.ContractVersion, DefinitionVersion: "5.0.0", SkillVersion: "5.0.0", TemplateVersion: "5.0.0", SystemPrompt: "Return DecisionAdvice only; approval is a deterministic control-plane decision.", Tools: []contracts.ToolIdentity{{ToolID: "go", Version: "go1.25.0"}}}
	for _, id := range []string{"scaffold", "database", "auth", "logging"} {
		opts.Templates = append(opts.Templates, definitions.TemplateBundle{ID: id, Version: "5.0.0", Files: map[string][]byte{"render-program.json": []byte(`{"format_version":1,"rules":[]}`)}})
	}
	release, err := definitions.BuildDefault(opts)
	if err != nil {
		t.Fatal(err)
	}
	registry, err := definitions.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err = registry.Publish(release); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)
	snapshot, err := registry.Freeze(release.Definition.DefinitionID, release.Definition.DefinitionVersion, release.Definition.DefinitionHash, nil, now.Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	version := snapshot.ContractVersion
	target := contracts.GitLabTarget{Provider: "gitlab", InstanceURL: "https://gitlab.example.com", NamespaceID: "12", NamespacePath: "team", ProjectPath: "governed-service", Ref: "main", Visibility: "private"}
	plan := contracts.ProjectPlan{ContractVersion: version, ProjectName: "governed-service", Description: "Governed project", TechStack: "golang", SelectedSkills: []contracts.SkillSelection{}, TemplateDependencies: snapshot.TemplateDependencies, DeliveryTarget: raw(t, target)}
	for _, skill := range snapshot.Skills {
		params, err := registry.NormalizeParameters(snapshot, skill.SkillID, map[string]any{})
		if err != nil {
			t.Fatal(err)
		}
		plan.SelectedSkills = append(plan.SelectedSkills, raw(t, map[string]any{"skill_id": skill.SkillID, "skill_version": skill.SkillVersion, "skill_hash": skill.SkillHash, "parameters": params}))
	}
	plan.PlanHash = objectHash(t, "ProjectPlan", plan)
	permissions := []string{"gitlab_ci", "gitlab_create", "gitlab_push", "package_write", "tool_go", "workspace_read", "workspace_write"}
	permission := contracts.PermissionSnapshot{PermissionVersion: 1, ActorID: "trusted_user", AllowedPermissions: permissions, DeliveryTarget: plan.DeliveryTarget, UpdatedAt: now.Add(-time.Minute).Format(time.RFC3339Nano)}
	permission.PermissionHash = objectHash(t, "PermissionSnapshot", permission)
	content := "stages: [test, build]\n"
	contentHash := contracts.BytesHash([]byte(content))
	issued, expires := now.Add(-time.Minute).Format(time.RFC3339Nano), now.Add(30*time.Minute).Format(time.RFC3339Nano)
	proposal := contracts.Proposal{ContractVersion: version, ProposalID: "proposal_one", RunID: "run_one", ProposalVersion: 1, SourceVersion: 2, DefinitionSnapshotHash: snapshot.SnapshotHash, PlanHash: plan.PlanHash, Title: "Approved project", Status: "accepted", Plan: plan, Changes: []contracts.FileChange{{Path: ".gitlab-ci.yml", Operation: "create", After: &content, AfterSHA256: &contentHash}}, Impact: []string{"Create only approved project files"}, AllowedActions: append([]string{}, snapshot.Definition.ActionProtocol.AllowedActions...), RequiredPermissionIDs: permissions, PermissionHash: permission.PermissionHash, Checks: []contracts.PreflightCheck{{CheckID: "preflight.safe", Passed: true, Detail: "Controlled fixture preflight"}}, IssuedAt: issued, ExpiresAt: expires}
	proposal.SnapshotHash = objectHash(t, "Proposal", proposal)
	approval := contracts.Approval{ContractVersion: version, ApprovalID: "approval_one", RunID: "run_one", ActorID: "trusted_user", ProposalID: proposal.ProposalID, ProposalVersion: 1, SourceVersion: 2, ProposalSnapshotHash: proposal.SnapshotHash, DefinitionSnapshotHash: snapshot.SnapshotHash, PlanHash: plan.PlanHash, PermissionHash: permission.PermissionHash, AllowedActions: proposal.AllowedActions, DeliveryTarget: plan.DeliveryTarget, IssuedAt: issued, ExpiresAt: expires, AcceptedAt: now.Add(-30 * time.Second).Format(time.RFC3339Nano), Status: "accepted"}
	approval.ApprovalHash = objectHash(t, "Approval", approval)
	manifest := contracts.ArtifactManifest{ContractVersion: version, ManifestID: "manifest_one", RunID: "run_one", SourceVersion: 2, ProposalID: proposal.ProposalID, ProposalVersion: 1, ProposalSnapshotHash: proposal.SnapshotHash, DefinitionSnapshotHash: snapshot.SnapshotHash, PlanHash: plan.PlanHash, Files: []contracts.ArtifactEntry{{ArtifactID: "artifact_ci", Path: ".gitlab-ci.yml", Size: int64(len(content)), SHA256: contentHash, MediaType: "text/yaml"}}, CreatedAt: now.Format(time.RFC3339Nano), ManifestVersion: 1}
	manifest.Hash = objectHash(t, "ArtifactManifest", manifest)
	deadline := now.Add(time.Hour)
	facts := Facts{RunID: "run_one", State: "IMPLEMENT", ActorID: "trusted_user", CommandID: "command_one", SourceVersion: 2, StateSequence: 8, OperationSequence: 1, Modes: contracts.Modes{ExecutionMode: "local", DecisionMode: "rules", DeliveryMode: "gitlab"}, FrozenDefinition: snapshot, Plan: &plan, Proposal: &proposal, Approval: &approval, Permission: &permission, Workspace: &contracts.WorkspaceReference{WorkspaceID: "workspace_one", RunID: "run_one", StorageKey: "workspaces/run_one/governed-service"}, Manifest: &manifest, RemoteProjectID: 41, CommitSHA: "0123456789012345678901234567890123456789", CIConfigSHA256: contentHash, Now: now, Budget: BudgetFacts{Available: true, RemainingAttempts: 3, ExecutionDeadline: &deadline, ToolDeadline: &deadline, CIDeadline: &deadline, ReconciliationDeadline: &deadline, ReconciliationAvailable: true}, ObservedModelUsage: &contracts.ModelUsage{Status: "not_applicable"}, AvailableTools: snapshot.ToolBindings, AvailableEvidence: []contracts.ObjectReference{manifestRef(manifest)}}
	message := contracts.MessageEnvelope{SchemaVersion: 2, ContractVersion: version, RunID: facts.RunID, MessageID: "message_one", Sequence: 1, SourceVersion: 2, StateSequence: 7, Role: "user", CreatedAt: now.Add(-time.Minute).Format(time.RFC3339Nano), Modes: facts.Modes, Parts: []contracts.Part{raw(t, contracts.TextPart{PartID: "part_one", PartVersion: 1, Type: "text", FallbackText: "Create the approved project", Text: "Create the approved project", Format: "plain"})}}
	ref, err := MessageRef(message)
	if err != nil {
		t.Fatal(err)
	}
	in := Input{Facts: facts, InputMessages: []contracts.MessageEnvelope{message}, OrderedContext: []ContextMessage{{Role: "system", Content: snapshot.Definition.SystemPrompt.Content}, {Role: "user", Content: "Create the approved project"}}}
	in.Decision = contracts.DecisionProposal{ContractVersion: version, DecisionID: "decision_one", RunID: facts.RunID, SourceVersion: 2, StateSequence: 8, DefinitionSnapshotHash: snapshot.SnapshotHash, DecisionMode: "rules", ModelUsage: *facts.ObservedModelUsage, InputMessageRefs: []contracts.ObjectReference{ref}, CreatedAt: now.Format(time.RFC3339Nano)}
	in.Decision.InputContextHash, _ = contracts.HashValue(in.OrderedContext)
	setAction(t, &in, "scaffold")
	return Validator{Definitions: registry}, in
}

func raw(t *testing.T, value any) []byte {
	t.Helper()
	body, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return body
}
func objectHash(t *testing.T, name string, value any) string {
	t.Helper()
	hash, err := contracts.HashObject(name, value)
	if err != nil {
		t.Fatal(err)
	}
	return hash
}
func cloneInput(t *testing.T, in Input) Input {
	t.Helper()
	var out Input
	if err := json.Unmarshal(raw(t, in), &out); err != nil {
		t.Fatal(err)
	}
	return out
}
func rehashDecision(t *testing.T, in *Input) {
	in.Decision.DecisionHash = objectHash(t, "DecisionProposal", in.Decision)
}
func setAction(t *testing.T, in *Input, kind string) {
	t.Helper()
	f := &in.Facts
	var parameters any
	switch kind {
	case "scaffold":
		f.State = "IMPLEMENT"
		parameters = contracts.ScaffoldParameters{PlanHash: f.Plan.PlanHash}
	case "apply_skill":
		f.State = "IMPLEMENT"
		parameters = contracts.ApplySkillParameters{PlanHash: f.Plan.PlanHash, Selection: f.Plan.SelectedSkills[0]}
	case "verify":
		f.State = "VERIFY"
		checks := []string{}
		for _, c := range f.FrozenDefinition.Definition.CompletionPolicy.RequiredChecks {
			if contains(c.DeliveryModes, f.Modes.DeliveryMode) {
				checks = append(checks, c.CheckID)
			}
		}
		sort.Strings(checks)
		parameters = contracts.VerifyParameters{ManifestRef: manifestRef(*f.Manifest), RequiredCheckIDs: checks}
	case "create_repository":
		f.State = "DELIVER"
		var target contracts.GitLabTarget
		_ = json.Unmarshal(f.Plan.DeliveryTarget, &target)
		parameters = contracts.CreateRepositoryParameters{Target: target}
	case "push_repository":
		f.State = "DELIVER"
		var target contracts.GitLabTarget
		_ = json.Unmarshal(f.Plan.DeliveryTarget, &target)
		parameters = contracts.PushRepositoryParameters{Target: target, ProjectID: f.RemoteProjectID, ManifestRef: manifestRef(*f.Manifest)}
	case "trigger_ci":
		f.State = "DELIVER"
		var target contracts.GitLabTarget
		_ = json.Unmarshal(f.Plan.DeliveryTarget, &target)
		parameters = contracts.TriggerCIParameters{Target: target, ProjectID: f.RemoteProjectID, CommitSha: f.CommitSHA, CIConfigSHA256: f.CIConfigSHA256}
	case "package":
		f.State = "DELIVER"
		parameters = contracts.PackageParameters{ManifestRef: manifestRef(*f.Manifest)}
	default:
		t.Fatal("unregistered fixture action")
	}
	action := contracts.CandidateAction(raw(t, map[string]any{"action_type": kind, "parameters": parameters}))
	f.ExpectedAction = &action
	in.Decision.Advice = contracts.DecisionAdvice{Kind: "action", Action: &action, Reason: "Controlled fixture suggestion"}
	rehashDecision(t, in)
}

func TestSevenActionsProduceCompleteDeterministicRequests(t *testing.T) {
	v, base := fixture(t)
	for _, kind := range []string{"scaffold", "apply_skill", "verify", "create_repository", "push_repository", "trigger_ci", "package"} {
		t.Run(kind, func(t *testing.T) {
			in := cloneInput(t, base)
			setAction(t, &in, kind)
			result, err := v.Validate(in)
			if err != nil || result.Verdict != "allowed" || result.ExecutionRequest == nil {
				t.Fatalf("%s: result=%+v error=%v", kind, result, err)
			}
			again, err := v.Validate(in)
			if err != nil || !equalJSON(result, again) {
				t.Fatal("same trusted inputs did not reproduce identical IDs and hashes", err)
			}
			if err = contracts.Validate("ActionValidation", result); err != nil {
				t.Fatal(err)
			}
			current := cloneInput(t, in).Facts
			current.StateSequence += 4
			current.Now = current.Now.Add(time.Second)
			started := in.Facts.Now.Format(time.RFC3339Nano)
			current.Approval.StartedAt = &started
			current.Approval.Status = "started"
			if err = v.Revalidate(*result.ExecutionRequest, in, result, current); err != nil {
				t.Fatal("ordinary progress invalidated the authorized action", err)
			}
		})
	}
}

func TestFiveAdviceKindsNeverGrantExecutionToNonActions(t *testing.T) {
	v, base := fixture(t)
	for _, kind := range []string{"clarification", "plan", "action", "diagnosis", "completion"} {
		t.Run(kind, func(t *testing.T) {
			in := cloneInput(t, base)
			advice := contracts.DecisionAdvice{Kind: kind, Reason: "Test each semantic branch"}
			switch kind {
			case "clarification":
				in.Facts.State = "CLARIFY"
				q := "Which project name?"
				advice.Question = &q
			case "plan":
				in.Facts.State = "PLAN"
				advice.Plan = in.Facts.Plan
			case "action":
				advice = in.Decision.Advice
			case "diagnosis":
				in.Facts.State = "VERIFY"
				advice.Diagnosis = &contracts.Diagnosis{Category: "business", Code: "test_failure", Message: "A test failed", EvidenceRefs: in.Facts.AvailableEvidence, SuggestedRemedy: "Request a new plan; this does not approve it"}
			case "completion":
				in.Facts.State = "DELIVER"
				hint := "The deterministic completion gate must decide"
				advice.CompletionHint = &hint
			}
			in.Decision.Advice = advice
			rehashDecision(t, &in)
			result, err := v.Validate(in)
			if err != nil || result.Verdict == "rejected" || result.DecisionKind != kind {
				t.Fatalf("%+v %v", result, err)
			}
			if (result.ExecutionRequest != nil) != (kind == "action") {
				t.Fatal("non-action advice obtained execution authority")
			}
			if kind == "clarification" && (result.Verdict != "needs_clarification" || result.NormalizedAdvice != nil) {
				t.Fatal("clarification shape violated published contract")
			}
		})
	}
}

func TestAdviceRejectsBrokenProvenanceAndAuthority(t *testing.T) {
	v, base := fixture(t)
	for _, tc := range []struct {
		name   string
		mutate func(*Input)
	}{
		{"missing_approval", func(in *Input) { in.Facts.Approval = nil }},
		{"fake_approval_hash", func(in *Input) { in.Facts.Approval.ActorID = "attacker" }},
		{"expired_first_application", func(in *Input) { in.Facts.Now = in.Facts.Now.Add(30 * time.Minute) }},
		{"revoked_permission", func(in *Input) {
			in.Facts.Permission.Revoked = true
			in.Facts.Permission.PermissionHash = objectHash(t, "PermissionSnapshot", *in.Facts.Permission)
		}},
		{"wrong_stage", func(in *Input) { in.Facts.State = "PLAN" }},
		{"wrong_source", func(in *Input) { in.Decision.SourceVersion++; rehashDecision(t, in) }},
		{"wrong_context", func(in *Input) { in.OrderedContext[1].Content = "Changed actual input" }},
		{"wrong_message", func(in *Input) { in.InputMessages[0].MessageID = "another_message" }},
		{"self_reported_usage", func(in *Input) { in.Facts.ObservedModelUsage = nil }},
		{"wrong_plan_hash", func(in *Input) { in.Facts.Plan.Description = "Not approved" }},
		{"wrong_workspace", func(in *Input) { in.Facts.Workspace.RunID = "other_run" }},
		{"empty_budget", func(in *Input) { in.Facts.Budget.Available = false }},
		{"expired_execution_deadline", func(in *Input) { in.Facts.Budget.ExecutionDeadline = &in.Facts.Now }},
		{"action_not_approved", func(in *Input) {
			in.Facts.Approval.AllowedActions = []string{"verify"}
			in.Facts.Approval.ApprovalHash = objectHash(t, "Approval", *in.Facts.Approval)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			in := cloneInput(t, base)
			tc.mutate(&in)
			result, err := v.Validate(in)
			if err != nil {
				t.Fatal(err)
			}
			if result.Verdict != "rejected" || result.ExecutionRequest != nil || result.NormalizedAdvice != nil {
				t.Fatalf("invalid authority accepted: %+v", result)
			}
		})
	}
}

func TestExecutionRejectsForgedRequestAndChangedCurrentAuthority(t *testing.T) {
	v, in := fixture(t)
	result, err := v.Validate(in)
	if err != nil || result.ExecutionRequest == nil {
		t.Fatal(result, err)
	}
	for _, tc := range []struct {
		name   string
		mutate func(*contracts.ExecutionRequest, *Facts)
	}{
		{"request_rehashed", func(request *contracts.ExecutionRequest, current *Facts) {
			request.CommandID = "forged_command"
			request.RequestHash = objectHash(t, "ExecutionRequest", *request)
		}},
		{"wrong_source", func(request *contracts.ExecutionRequest, current *Facts) { current.SourceVersion++ }},
		{"new_approval_same_id", func(request *contracts.ExecutionRequest, current *Facts) {
			current.Approval.AllowedActions = []string{"scaffold"}
			current.Approval.ApprovalHash = objectHash(t, "Approval", *current.Approval)
		}},
		{"revoked_after_validation", func(request *contracts.ExecutionRequest, current *Facts) {
			stamp := current.Now.Format(time.RFC3339Nano)
			current.Approval.RevokedAt = &stamp
			current.Approval.Status = "revoked"
		}},
		{"wrong_current_stage", func(request *contracts.ExecutionRequest, current *Facts) { current.State = "DELIVER" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			request := *result.ExecutionRequest
			current := cloneInput(t, in).Facts
			tc.mutate(&request, &current)
			if v.Revalidate(request, in, result, current) == nil {
				t.Fatal("forged/stale execution accepted")
			}
		})
	}
}

func TestUnknownWriteOnlyReturnsBudgetedReconciliation(t *testing.T) {
	v, in := fixture(t)
	setAction(t, &in, "create_repository")
	result, err := v.Validate(in)
	if err != nil || result.ExecutionRequest == nil {
		t.Fatal(result, err)
	}
	request := *result.ExecutionRequest
	for _, status := range []string{"sent", "unknown"} {
		t.Run(status, func(t *testing.T) {
			current := cloneInput(t, in).Facts
			targetHash, _ := RequestTargetHash(request)
			current.Operation = &contracts.OperationRecord{OperationID: request.OperationID, RunID: current.RunID, SourceVersion: current.SourceVersion, ActionType: "create_repository", OperationSequence: 1, RequestHash: request.RequestHash, TargetHash: targetHash, Status: status, EvidenceRefs: []contracts.ObjectReference{}, CreatedAt: current.Now.Format(time.RFC3339Nano), UpdatedAt: current.Now.Format(time.RFC3339Nano)}
			current.Budget.Available = false // Exhausted writes must not disable separately bounded observation.
			var reconciliation *ReconciliationRequiredError
			if err := v.Revalidate(request, in, result, current); !errors.As(err, &reconciliation) || reconciliation.OperationID != request.OperationID {
				t.Fatal("unknown write did not enter original-identity reconciliation", err)
			}
			candidate := cloneInput(t, in)
			candidate.Facts = current
			blocked, err := v.Validate(candidate)
			if err != nil || blocked.Verdict != "rejected" || blocked.ExecutionRequest != nil || blocked.Findings[0].Code != "reconciliation_required" {
				t.Fatal(blocked, err)
			}
			current.Budget.ReconciliationAvailable = false
			if err := v.Revalidate(request, in, result, current); err == nil || errors.As(err, &reconciliation) {
				t.Fatal("exhausted read-only allowance still authorized reconciliation", err)
			}
		})
	}
}

func TestEveryActionRejectsWrongExactScope(t *testing.T) {
	v, base := fixture(t)
	for _, kind := range []string{"scaffold", "apply_skill", "verify", "create_repository", "push_repository", "trigger_ci", "package"} {
		t.Run(kind, func(t *testing.T) {
			in := cloneInput(t, base)
			setAction(t, &in, kind)
			var action map[string]any
			if err := json.Unmarshal(*in.Decision.Advice.Action, &action); err != nil {
				t.Fatal(err)
			}
			params := action["parameters"].(map[string]any)
			switch kind {
			case "scaffold":
				params["plan_hash"] = contracts.BytesHash([]byte("different plan"))
			case "apply_skill":
				selection := params["selection"].(map[string]any)
				values := selection["parameters"].(map[string]any)
				values["auth"] = "session"
			case "verify":
				params["required_check_ids"] = []string{"check.go_test"}
			case "create_repository":
				params["target"].(map[string]any)["namespace_id"] = "99"
			case "push_repository":
				params["project_id"] = int64(999)
			case "trigger_ci":
				params["commit_sha"] = "1111111111111111111111111111111111111111"
			case "package":
				params["manifest_ref"].(map[string]any)["source_version"] = 1
			}
			candidate := contracts.CandidateAction(raw(t, action))
			in.Decision.Advice.Action = &candidate
			// Even a broken scheduler's matching expectation is insufficient: the
			// independent parameter/approval rules must still reject the action.
			in.Facts.ExpectedAction = &candidate
			rehashDecision(t, &in)
			result, err := v.Validate(in)
			if err != nil {
				t.Fatal(err)
			}
			if result.Verdict != "rejected" || result.ExecutionRequest != nil {
				t.Fatalf("wrong exact %s scope accepted", kind)
			}
		})
	}
}

func TestSchedulerExpectationAndObservedToolCannotBeReplaced(t *testing.T) {
	v, in := fixture(t)
	setAction(t, &in, "verify")
	missing := cloneInput(t, in)
	missing.Facts.ExpectedAction = nil
	result, err := v.Validate(missing)
	if err != nil || result.Verdict != "rejected" {
		t.Fatal(result, err)
	}
	missing = cloneInput(t, in)
	missing.Facts.AvailableTools = nil
	result, err = v.Validate(missing)
	if err != nil || result.Verdict != "rejected" {
		t.Fatal(result, err)
	}
	forged := cloneInput(t, in)
	setAction(t, &forged, "package")
	forged.Facts.ExpectedAction = in.Facts.ExpectedAction
	result, err = v.Validate(forged)
	if err != nil || result.Verdict != "rejected" || result.Findings[0].Code != "unexpected_action" {
		t.Fatal("candidate chose a different scheduler action", result, err)
	}
}

func TestModelUsageRequiresObservedProviderProvenance(t *testing.T) {
	v, base := fixture(t)
	registry := v.Definitions.(*definitions.Registry)
	bound := base.Facts.FrozenDefinition
	snapshot, err := registry.Freeze(bound.DefinitionID, bound.DefinitionVersion, bound.DefinitionHash, &contracts.ProviderBinding{ProviderKind: "openai_compatible", Endpoint: "https://model.example.com/v1", ModelID: "model_fixture"}, base.Facts.Now.Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	base.Facts.FrozenDefinition = snapshot
	base.Facts.Modes.DecisionMode = "model"
	base.InputMessages[0].Modes = base.Facts.Modes
	base.Decision.DefinitionSnapshotHash = snapshot.SnapshotHash
	base.Decision.DecisionMode = "model"
	base.Decision.InputMessageRefs[0], err = MessageRef(base.InputMessages[0])
	if err != nil {
		t.Fatal(err)
	}
	base.Facts.Proposal.DefinitionSnapshotHash = snapshot.SnapshotHash
	base.Facts.Proposal.SnapshotHash = objectHash(t, "Proposal", *base.Facts.Proposal)
	base.Facts.Approval.DefinitionSnapshotHash = snapshot.SnapshotHash
	base.Facts.Approval.ProposalSnapshotHash = base.Facts.Proposal.SnapshotHash
	base.Facts.Approval.ApprovalHash = objectHash(t, "Approval", *base.Facts.Approval)
	input, output, total := int64(11), int64(3), int64(14)
	for _, usage := range []contracts.ModelUsage{{Status: "unknown"}, {Status: "known", InputTokens: &input, OutputTokens: &output, TotalTokens: &total}} {
		in := cloneInput(t, base)
		in.Decision.ModelUsage = usage
		in.Facts.ObservedModelUsage = &usage
		rehashDecision(t, &in)
		result, err := v.Validate(in)
		if err != nil || result.Verdict != "allowed" {
			t.Fatal(result, err)
		}
		in.Facts.ObservedModelUsage = &contracts.ModelUsage{Status: "not_applicable"}
		result, err = v.Validate(in)
		if err != nil || result.Verdict != "rejected" {
			t.Fatal("provider usage was replaced by candidate claims", result, err)
		}
	}
}

func TestMergedFrozenPostCommandsRemainFixedAndUnique(t *testing.T) {
	_, in := fixture(t)
	commands, err := MergePostCommands(in.Facts.FrozenDefinition, *in.Facts.Plan)
	if err != nil {
		t.Fatal(err)
	}
	if len(commands) != 3 {
		t.Fatalf("definition/three Skill duplicates produced %d commands", len(commands))
	}
	for i, id := range []string{"go.mod_tidy", "go.test", "go.build"} {
		if commands[i].CommandID != id || commands[i].TimeoutSeconds > in.Facts.FrozenDefinition.Definition.BudgetPolicy.MaxToolSeconds || commands[i].MaxAttempts > in.Facts.FrozenDefinition.Definition.BudgetPolicy.MaxStepAttempts {
			t.Fatal("merged command order or frozen limits changed")
		}
	}
	commands[0].Arguments[0] = 'x'
	again, err := MergePostCommands(in.Facts.FrozenDefinition, *in.Facts.Plan)
	if err != nil || again[0].Arguments[0] != '[' {
		t.Fatal("output command bytes aliased the frozen declaration", err)
	}
}
