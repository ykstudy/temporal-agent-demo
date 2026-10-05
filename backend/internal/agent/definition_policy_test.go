package agent

import (
	"errors"
	"runtime"
	"strings"
	"testing"
	"time"

	"agentstudio/internal/contracts"
	"agentstudio/internal/definitions"
)

// These tests isolate the completion gate. Check results are synthetic here;
// actual tool execution remains covered by the complete local-flow tests.
func publishedCompletionPolicyRun(t *testing.T, mode string, mutate func(*contracts.AgentDefinition)) Run {
	t.Helper()
	bundles, err := releaseTemplatePrograms()
	if err != nil {
		t.Fatal(err)
	}
	for i := range bundles {
		bundles[i].Version = "3.99.0"
	}
	platform := runtime.GOOS + "_" + runtime.GOARCH
	release, err := definitions.BuildDefault(definitions.PublishOptions{
		DefinitionVersion: "3.99.0", SkillVersion: "3.99.0", TemplateVersion: "3.99.0",
		SystemPrompt: defaultSystemPrompt, Templates: bundles,
		Tools: []contracts.ToolIdentity{{ToolID: "go", Version: runtime.Version(), Platform: &platform}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if mutate != nil {
		mutate(&release.Definition)
	}
	release.Definition.DefinitionHash, err = contracts.HashObject("AgentDefinition", release.Definition)
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
	snapshot, err := registry.Freeze(release.Definition.DefinitionID, release.Definition.DefinitionVersion, release.Definition.DefinitionHash, nil, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	r := Run{DeliveryMode: mode, SourceVersion: 7, FrozenDefinition: &snapshot}
	r.RequiredChecks = requiredChecksFor(r)
	return r
}

func markPolicyChecksPassed(r *Run) {
	for i := range r.RequiredChecks {
		r.RequiredChecks[i].Passed = true
		r.RequiredChecks[i].Status = "passed"
		r.RequiredChecks[i].SourceVersion = r.SourceVersion
	}
}

func TestFrozenCompletionRejectsUnsupportedCheckSemantics(t *testing.T) {
	for _, tc := range []struct {
		name, mode, checkID string
		mutate              func(*contracts.CheckDefinition)
	}{
		{"different_kind", "local", "check.go_test", func(c *contracts.CheckDefinition) { c.Kind = "file" }},
		{"additional_evidence", "local", "check.go_test", func(c *contracts.CheckDefinition) {
			c.RequiredEvidenceTypes = append(c.RequiredEvidenceTypes, "source_hash")
		}},
		{"substituted_evidence", "local", "check.go_test", func(c *contracts.CheckDefinition) {
			c.RequiredEvidenceTypes = []string{"file_hash"}
		}},
		{"missing_ci_evidence", "gitlab", "check.gitlab_ci", func(c *contracts.CheckDefinition) {
			c.RequiredEvidenceTypes = []string{"pipeline_jobs", "coverage_report"}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := publishedCompletionPolicyRun(t, tc.mode, func(def *contracts.AgentDefinition) {
				for i := range def.CompletionPolicy.RequiredChecks {
					if def.CompletionPolicy.RequiredChecks[i].CheckID == tc.checkID {
						tc.mutate(&def.CompletionPolicy.RequiredChecks[i])
					}
				}
			})
			found := false
			for _, check := range r.RequiredChecks {
				found = found || check.Name == "unsupported_check_policy:"+tc.checkID
			}
			if !found {
				t.Fatal("unimplemented frozen evidence requirements were discarded")
			}
			markPolicyChecksPassed(&r)
			var api *APIError
			if err := requireCompletion(r); !errors.As(err, &api) || api.Code != "verification_incomplete" {
				t.Fatalf("unsupported frozen policy must reject even forged passed results: %v", err)
			}
		})
	}
}

func TestFrozenCompletionAcceptsEvidenceSetOrderAndTitleChanges(t *testing.T) {
	for _, mode := range []string{"local", "gitlab"} {
		t.Run(mode, func(t *testing.T) {
			r := publishedCompletionPolicyRun(t, mode, func(def *contracts.AgentDefinition) {
				for i := range def.CompletionPolicy.RequiredChecks {
					check := &def.CompletionPolicy.RequiredChecks[i]
					check.Title = "Operator-facing title"
					if check.CheckID == "check.gitlab_ci" {
						check.RequiredEvidenceTypes = []string{"build_artifact", "pipeline_jobs", "coverage_report"}
					}
				}
			})
			if len(r.RequiredChecks) != len(requiredChecks(mode)) {
				t.Fatal("compatible policy changed its required safety check set")
			}
			for _, check := range r.RequiredChecks {
				if strings.Contains(check.Name, "check_policy:") {
					t.Fatal("evidence set order or title incorrectly changed execution semantics")
				}
			}
			markPolicyChecksPassed(&r)
			if err := requireCompletion(r); err != nil {
				t.Fatal(err)
			}
			r.RequiredChecks[0].SourceVersion--
			if err := requireCompletion(r); err == nil {
				t.Fatal("completion accepted evidence from another source version")
			}
		})
	}
}
