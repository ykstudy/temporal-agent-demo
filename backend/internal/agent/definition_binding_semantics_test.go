package agent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"agentstudio/internal/contracts"
	"agentstudio/internal/definitions"
)

// Publish every changed policy through the real registry. Tests must not make
// an altered in-memory snapshot appear to have been frozen by the service.
func publishSemanticDefinition(t *testing.T, s *Service, version string, changeTemplates func([]definitions.TemplateBundle), changeDefinition func(*contracts.AgentDefinition)) contracts.AgentDefinition {
	t.Helper()
	bundles, err := releaseTemplatePrograms()
	if err != nil {
		t.Fatal(err)
	}
	for i := range bundles {
		bundles[i].Version = version
	}
	if changeTemplates != nil {
		changeTemplates(bundles)
	}
	release, err := definitions.BuildDefault(definitions.PublishOptions{
		DefinitionVersion: version, SkillVersion: version, TemplateVersion: version,
		SystemPrompt: defaultSystemPrompt, Tools: s.DefaultDefinition.RequiredTools, Templates: bundles,
	})
	if err != nil {
		t.Fatal(err)
	}
	if changeDefinition != nil {
		changeDefinition(&release.Definition)
	}
	release.Definition.DefinitionHash, err = contracts.HashObject("AgentDefinition", release.Definition)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Definitions.Publish(release); err != nil {
		t.Fatal(err)
	}
	s.DefaultDefinition = release.Definition
	return release.Definition
}

func editSemanticTemplate(t *testing.T, bundles []definitions.TemplateBundle, id string, edit func(*templateProgram)) {
	t.Helper()
	for i := range bundles {
		if bundles[i].ID != id {
			continue
		}
		var program templateProgram
		if err := json.Unmarshal(bundles[i].Files["render-program.json"], &program); err != nil {
			t.Fatal(err)
		}
		edit(&program)
		contents, err := json.Marshal(program)
		if err != nil {
			t.Fatal(err)
		}
		bundles[i].Files["render-program.json"] = contents
		return
	}
	t.Fatalf("template %s missing", id)
}

func TestFrozenDefinitionSurvivesStoreReopenAndLaterPublication(t *testing.T) {
	cfg := Config{DataDir: t.TempDir(), Mode: "local", GitLabURL: "https://gitlab.com"}
	s, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	reopen := func() {
		t.Helper()
		s.Close()
		reopened, err := New(cfg)
		if err != nil {
			t.Fatal(err)
		}
		s = reopened
	}
	old := planRun(t, s, "restart_frozen_old", "创建 restart-old-service，JWT 认证")
	originalFiles, err := s.renderFrozenFiles(old)
	if err != nil {
		t.Fatal(err)
	}
	originalTemplate, err := s.Definitions.TemplateFiles(*old.FrozenDefinition, "scaffold")
	if err != nil {
		t.Fatal(err)
	}
	originalPrompt, err := buildModelRequest(old, "继续", "fixture")
	if err != nil {
		t.Fatal(err)
	}
	assertOldBinding := func() {
		t.Helper()
		saved, err := s.Store.Get(old.ID)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(saved.FrozenDefinition, old.FrozenDefinition) {
			t.Fatal("store reopen or publication changed the complete frozen snapshot")
		}
		if err = s.Definitions.VerifySnapshot(*saved.FrozenDefinition); err != nil {
			t.Fatal(err)
		}
		files, err := s.renderFrozenFiles(saved)
		if err != nil || !reflect.DeepEqual(files, originalFiles) {
			t.Fatalf("old rendered files changed: %v", err)
		}
		contents, err := s.Definitions.TemplateFiles(*saved.FrozenDefinition, "scaffold")
		if err != nil || !reflect.DeepEqual(contents, originalTemplate) {
			t.Fatalf("old raw template contents changed: %v", err)
		}
		prompt, err := buildModelRequest(saved, "继续", "fixture")
		if err != nil || string(prompt) != string(originalPrompt) {
			t.Fatalf("old frozen prompt changed: %v", err)
		}
	}

	reopen()
	assertOldBinding()
	const marker = "SECOND PUBLISHED TEMPLATE"
	publishSemanticDefinition(t, s, "3.8.1", func(bundles []definitions.TemplateBundle) {
		editSemanticTemplate(t, bundles, "scaffold", func(program *templateProgram) {
			for i := range program.Rules {
				if program.Rules[i].Path == "README.md" {
					program.Rules[i].Source += "\n" + marker + "\n"
				}
			}
		})
	}, func(definition *contracts.AgentDefinition) {
		definition.SystemPrompt.Content += "\nSECOND PUBLISHED PROMPT"
		definition.SystemPrompt.SHA256 = contracts.BytesHash([]byte(definition.SystemPrompt.Content))
	})
	newRun := planRun(t, s, "restart_frozen_new", "创建 restart-new-service")
	newFiles, err := s.renderFrozenFiles(newRun)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(newFiles["README.md"], marker) || newRun.FrozenDefinition.DefinitionHash == old.FrozenDefinition.DefinitionHash {
		t.Fatal("new Run did not bind the second publication")
	}
	assertOldBinding()
	reopen()
	assertOldBinding()
	savedNew, err := s.Store.Get(newRun.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(savedNew.FrozenDefinition, newRun.FrozenDefinition) {
		t.Fatal("restart changed the second publication's snapshot")
	}
	if files, err := s.renderFrozenFiles(savedNew); err != nil || !reflect.DeepEqual(files, newFiles) {
		t.Fatalf("second publication's persisted content did not survive restart: %v", err)
	}
}

func TestLegacyPendingWithoutFrozenSourceCannotAcquireApproval(t *testing.T) {
	s := testService(t)
	r := planRun(t, s, "legacy_pending_source", "创建 legacy-pending-service")
	legacy, err := definitionSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	// Model the historical legacy-only disk representation, including its own
	// coherent proposal digest. Rejection must concern missing frozen sources,
	// rather than an incidental digest mismatch in a damaged modern snapshot.
	r, err = s.Store.Update(r.ID, func(run *Run) error {
		run.FrozenDefinition = nil
		run.Definition = legacy
		run.Proposal.DefinitionHash = legacy.Hash
		run.Proposal.SnapshotHash = proposalDigest(*run, *run.Proposal)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	command := approvalCommand(r, "accept_missing_frozen_source")
	if err = commandBinding(r, command); err != nil {
		t.Fatalf("legacy fixture has invalid proposal binding: %v", err)
	}
	_, err = s.ProcessCommand(context.Background(), command)
	assertCode(t, err, "legacy_contract_incomplete")
	saved, err := s.Store.Get(r.ID)
	if err != nil {
		t.Fatal(err)
	}
	if saved.State != "PLAN" || saved.Proposal.Status != "pending" || saved.Approval != nil || saved.AcceptedProposal != "" || saved.Workspace != "" {
		t.Fatal("legacy pending Run acquired approval or execution state")
	}
	if _, err = os.Stat(filepath.Join(s.Config.DataDir, "workspaces", r.ID)); !os.IsNotExist(err) {
		t.Fatalf("legacy pending Run created a workspace: %v", err)
	}
}

func TestPublishedTemplateCannotWriteUndeclaredFrozenSkillPath(t *testing.T) {
	s := testService(t)
	publishSemanticDefinition(t, s, "3.8.2", func(bundles []definitions.TemplateBundle) {
		editSemanticTemplate(t, bundles, "auth", func(program *templateProgram) {
			program.Rules = append(program.Rules, templateRule{Path: "internal/auth/undeclared.go", Source: "package auth\n", Literal: true})
		})
	}, nil)
	r, err := s.Create(context.Background(), "run_undeclared_frozen_path", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Definitions.VerifySnapshot(*r.FrozenDefinition); err != nil {
		t.Fatalf("test publication is not a valid frozen dependency closure: %v", err)
	}
	_, err = s.ProcessCommand(context.Background(), Command{ID: "plan_undeclared_frozen_path", RunID: r.ID, Content: "创建 undeclared-path-service，JWT 认证"})
	assertCode(t, err, "invalid_template")
	if !strings.Contains(err.Error(), "冻结技能路径") {
		t.Fatalf("template was rejected for an unrelated reason: %v", err)
	}
	saved, err := s.Store.Get(r.ID)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Proposal != nil || saved.Approval != nil || saved.Workspace != "" || len(saved.Artifacts) != 0 {
		t.Fatal("undeclared path entered an approvable proposal or workspace")
	}
}

func TestFrozenActionPolicyDoesNotGrantOtherExecutionSteps(t *testing.T) {
	s := testService(t)
	publishSemanticDefinition(t, s, "3.8.3", nil, func(definition *contracts.AgentDefinition) {
		definition.ActionProtocol.AllowedActions = []contracts.ActionType{"scaffold"}
	})
	r := planRun(t, s, "restricted_frozen_actions", "创建 restricted-actions-service，JWT 认证")
	accepted, err := s.ProcessCommand(context.Background(), approvalCommand(r, "accept_restricted_frozen_actions"))
	if err != nil {
		t.Fatal(err)
	}
	if !accepted.Execute || accepted.Run.Approval == nil || !reflect.DeepEqual(accepted.Run.Approval.Actions, []string{"scaffold"}) || !reflect.DeepEqual(accepted.Run.Definition.AllowedActions, []string{"scaffold"}) {
		t.Fatal("approval granted actions outside the published action protocol")
	}
	result, err := s.ExecuteStep(context.Background(), StepInput{RunID: r.ID, Step: "scaffold"})
	if err != nil || !result.Continue {
		t.Fatalf("the permitted scaffold step failed: %+v, %v", result, err)
	}
	before, err := s.Store.Get(r.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, step := range []string{"skills", "verify", "repo", "ci", "deliver"} {
		_, err = s.ExecuteStep(context.Background(), StepInput{RunID: r.ID, Step: step})
		assertCode(t, err, "action_not_allowed")
	}
	after, err := s.Store.Get(r.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, after) {
		t.Fatal("denied execution steps changed Run state or consumed attempts")
	}
	if _, err = os.Stat(filepath.Join(after.Workspace, "internal", "auth", "auth.go")); !os.IsNotExist(err) {
		t.Fatalf("denied Skill step wrote authentication files: %v", err)
	}
}

func TestFrozenUnknownRequiredCheckCannotCompleteRun(t *testing.T) {
	s := testService(t)
	const checkID = "check.unimplemented_gate"
	publishSemanticDefinition(t, s, "3.8.4", nil, func(definition *contracts.AgentDefinition) {
		check := definition.CompletionPolicy.RequiredChecks[0]
		check.CheckID, check.Title = checkID, "Unimplemented required product check"
		check.DeliveryModes = []contracts.DeliveryMode{"gitlab", "local"}
		definition.CompletionPolicy.RequiredChecks = append(definition.CompletionPolicy.RequiredChecks, check)
		sort.Slice(definition.CompletionPolicy.RequiredChecks, func(i, j int) bool {
			return definition.CompletionPolicy.RequiredChecks[i].CheckID < definition.CompletionPolicy.RequiredChecks[j].CheckID
		})
	})
	r := planRun(t, s, "unsupported_frozen_check", "创建 required-check-service")
	accepted, err := s.ProcessCommand(context.Background(), approvalCommand(r, "accept_unsupported_frozen_check"))
	if err != nil || !accepted.Execute {
		t.Fatalf("valid published definition could not be approved: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	for _, step := range []string{"scaffold", "skills", "verify", "repo", "ci"} {
		result, err := s.ExecuteStep(ctx, StepInput{RunID: r.ID, Step: step})
		if err != nil || !result.Continue {
			current, _ := s.Store.Get(r.ID)
			t.Fatalf("prerequisite %s failed before the completion gate: %v, %s", step, err, current.Error)
		}
	}
	result, err := s.ExecuteStep(ctx, StepInput{RunID: r.ID, Step: "deliver"})
	if err != nil || result.State != "FAILED" || result.Continue {
		t.Fatalf("unknown required check did not stop completion: %+v, %v", result, err)
	}
	saved, err := s.Store.Get(r.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(saved.Error, "unsupported_check:"+checkID) {
		t.Fatalf("completion failed for an unrelated reason: %s", saved.Error)
	}
	assertCode(t, requireCompletion(saved), "verification_incomplete")
	found := false
	for _, check := range saved.RequiredChecks {
		if check.Name == "unsupported_check:"+checkID {
			found = true
			if check.Passed || check.Status != "not_ready" {
				t.Fatal("unsupported required check acquired fabricated pass evidence")
			}
		} else if check.Name != "zip_integrity" && (!check.Passed || check.Status != "passed" || check.SourceVersion != saved.SourceVersion) {
			t.Fatalf("baseline check did not actually pass: %+v", check)
		}
	}
	if !found {
		t.Fatal("unknown check disappeared from the frozen completion policy")
	}
	if _, err = os.Stat(filepath.Join(s.Config.DataDir, "downloads", r.ID+".zip")); err != nil {
		t.Fatalf("the real package step did not reach the completion gate: %v", err)
	}
}
