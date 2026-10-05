package governance

import (
	"testing"
	"time"

	"agentstudio/internal/contracts"
	"agentstudio/internal/definitions"
)

func TestCandidatePlanRequiresRegisteredCurrentRevisionEvidence(t *testing.T) {
	registry, err := definitions.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	definition, err := registry.PublishBundled("3.2.0")
	if err != nil {
		t.Fatal(err)
	}
	frozen, err := registry.Freeze(definition.DefinitionID, definition.DefinitionVersion, definition.DefinitionHash, nil, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	ref := contracts.ProjectPlanVerificationCandidateRef{ObjectType: "verification_candidate", ObjectID: "candidate_original", ObjectVersion: 1, SourceVersion: 2, Hash: contracts.BytesHash([]byte("immutable candidate"))}
	plan := contracts.ProjectPlan{ContractVersion: "1.3.0", ProjectName: "revision-service", Description: "Existing scope", TechStack: "golang", SelectedSkills: []contracts.SkillSelection{}, TemplateDependencies: frozen.TemplateDependencies, DeliveryTarget: raw(t, contracts.LocalTarget{Provider: "local", Destination: "run_package"}), VerificationCandidateRef: &ref}
	plan.PlanHash = objectHash(t, "ProjectPlan", plan)
	facts := Facts{FrozenDefinition: frozen, State: "VERIFY", SourceVersion: 2, RevisionAllowed: true, AvailableEvidence: []contracts.ObjectReference{contracts.ObjectReference(ref)}, Modes: contracts.Modes{ExecutionMode: "local", DecisionMode: "rules", DeliveryMode: "local"}}
	validator := Validator{Definitions: registry}
	if err := validator.validatePlanAdvice(facts, plan); err != nil {
		t.Fatal(err)
	}
	for _, mutation := range []string{"planning", "revoked_boundary", "missing", "source", "hash", "identity"} {
		t.Run(mutation, func(t *testing.T) {
			changed := facts
			changed.AvailableEvidence = append([]contracts.ObjectReference{}, facts.AvailableEvidence...)
			switch mutation {
			case "planning":
				changed.State = "PLAN"
			case "revoked_boundary":
				changed.RevisionAllowed = false
			case "missing":
				changed.AvailableEvidence = nil
			case "source":
				changed.SourceVersion++
			case "hash":
				changed.AvailableEvidence[0].Hash = contracts.BytesHash([]byte("other"))
			case "identity":
				changed.AvailableEvidence[0].ObjectID = "candidate_other"
			}
			if err := validator.validatePlanAdvice(changed, plan); err == nil || err.Code != "candidate_not_available" {
				t.Fatalf("untrusted candidate reference accepted: %v", err)
			}
		})
	}
}
