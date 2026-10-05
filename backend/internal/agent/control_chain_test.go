package agent

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"agentstudio/internal/contracts"
	"agentstudio/internal/governance"
)

func chainRehash(t *testing.T, name string, v any) string {
	t.Helper()
	h, e := contracts.HashObject(name, v)
	if e != nil {
		t.Fatal(e)
	}
	return h
}
func assertChainRejected(t *testing.T, r Run) {
	t.Helper()
	if err := verifyControlChain(r); err == nil {
		t.Fatal("inconsistent chain accepted")
	}
}
func controlChainFixture(t *testing.T) (*Service, Run) {
	t.Helper()
	s := testService(t)
	r := controlledPlan(t, s, controlFixture(t, s, "chain_integrity"))
	if err := verifyControlChain(r); err != nil {
		t.Fatal("valid chain", err)
	}
	return s, r
}

func TestControlChainBindsPersistedPlanningAndPrivateFacts(t *testing.T) {
	_, original := controlChainFixture(t)
	cases := []struct {
		name   string
		change func(*Run)
	}{
		{"validation source", func(r *Run) {
			v := &r.Control.Audit.Validations[0]
			v.SourceVersion++
			v.ValidationHash = chainRehash(t, "ActionValidation", v)
		}},
		{"validation sequence", func(r *Run) {
			v := &r.Control.Audit.Validations[0]
			v.StateSequence++
			v.ValidationHash = chainRehash(t, "ActionValidation", v)
		}},
		{"validation definition", func(r *Run) {
			v := &r.Control.Audit.Validations[0]
			v.DefinitionSnapshotHash = strings.Repeat("0", 64)
			v.ValidationHash = chainRehash(t, "ActionValidation", v)
		}},
		{"validation kind", func(r *Run) {
			v := &r.Control.Audit.Validations[0]
			v.DecisionKind = "completion"
			v.ValidationHash = chainRehash(t, "ActionValidation", v)
		}},
		{"validation earlier than decision", func(r *Run) {
			v := &r.Control.Audit.Validations[0]
			v.ValidatedAt = "2020-01-01T00:00:00Z"
			v.ValidationHash = chainRehash(t, "ActionValidation", v)
		}},
		{"validation advice differs", func(r *Run) {
			v := &r.Control.Audit.Validations[0]
			v.NormalizedAdvice.Reason = "rewritten"
			v.ValidationHash = chainRehash(t, "ActionValidation", v)
		}},
		{"context source", func(r *Run) {
			v := &r.Control.Audit.Contexts[0]
			v.SourceVersion++
			v.ContextHash = chainRehash(t, "ContextRecord", v)
		}},
		{"context run", func(r *Run) {
			v := &r.Control.Audit.Contexts[0]
			v.RunID = "another_run"
			v.ContextHash = chainRehash(t, "ContextRecord", v)
		}},
		{"context time", func(r *Run) {
			v := &r.Control.Audit.Contexts[0]
			v.CreatedAt = "2020-01-01T00:00:00Z"
			v.ContextHash = chainRehash(t, "ContextRecord", v)
		}},
		{"actual ordered context differs", func(r *Run) {
			v := &r.Control.Audit.Contexts[0]
			v.Messages[0].Content = "different prompt"
			v.ContextHash = chainRehash(t, "ContextRecord", v)
		}},
		{"context refs differ", func(r *Run) {
			v := &r.Control.Audit.Contexts[0]
			v.InputMessageRefs[0].Hash = strings.Repeat("0", 64)
			v.ContextHash = chainRehash(t, "ContextRecord", v)
		}},
		{"registered message changed", func(r *Run) { r.Control.Audit.Messages[0].CreatedAt = "2020-01-01T00:00:00Z" }},
		{"missing registered message", func(r *Run) { r.Control.Audit.Messages = nil }},
		{"duplicate decision", func(r *Run) {
			r.Control.Audit.Decisions = append(r.Control.Audit.Decisions, r.Control.Audit.Decisions[0])
		}},
		{"missing private input", func(r *Run) { r.Control.ValidationInputs = nil }},
		{"private actor", func(r *Run) { r.Control.ValidationInputs[0].Facts.ActorID = "untrusted" }},
		{"private time", func(r *Run) { r.Control.ValidationInputs[0].Facts.Now = time.Now().Add(time.Hour) }},
		{"private source", func(r *Run) { r.Control.ValidationInputs[0].Facts.SourceVersion++ }},
		{"private frozen content", func(r *Run) {
			r.Control.ValidationInputs[0].Facts.FrozenDefinition.Definition.SystemPrompt.Content = "forged"
		}},
		{"private model usage", func(r *Run) { r.Control.ValidationInputs[0].Facts.ObservedModelUsage.Status = "unknown" }},
		{"private ordered context", func(r *Run) { r.Control.ValidationInputs[0].OrderedContext[0].Content = "forged" }},
		{"private input message", func(r *Run) { r.Control.ValidationInputs[0].InputMessages[0].MessageID = "forged" }},
		{"private permission without prior history", func(r *Run) { r.Control.ValidationInputs[0].Facts.Permission = r.Control.Audit.PermissionSnapshot }},
		{"private unbound tool", func(r *Run) { r.Control.ValidationInputs[0].Facts.AvailableTools[0].Version = "different" }},
		{"missing permission history", func(r *Run) { r.Control.Audit.PermissionHistory = nil }},
		{"proposal embedded plan hash", func(r *Run) {
			v := &r.Control.Audit.ProposalHistory[0]
			v.Plan.ProjectName = "different"
			v.SnapshotHash = chainRehash(t, "Proposal", v)
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			candidate := clone(original)
			tc.change(&candidate)
			assertChainRejected(t, candidate)
		})
	}
}

func controlledActionChain(t *testing.T) (*Service, Run) {
	t.Helper()
	s, r := controlChainFixture(t)
	p := r.Control.Audit.Proposal
	command := Command{ID: "approve_chain", RunID: r.ID, Action: "accept", ProposalID: p.ProposalID, ProposalVersion: int(p.ProposalVersion), SourceVersion: r.SourceVersion, SnapshotHash: p.SnapshotHash}
	var err error
	r, err = s.Store.Update(r.ID, func(run *Run) error { return s.acceptCanonical(run, command, time.Now().UTC()) })
	if err != nil {
		t.Fatal(err)
	}
	actionBody, _ := json.Marshal(map[string]any{"action_type": "scaffold", "parameters": contracts.ScaffoldParameters{PlanHash: r.Control.Audit.Plan.PlanHash}})
	action := contracts.CandidateAction(actionBody)
	ordered := r.Control.Audit.Contexts[0].Messages
	in, context, err := bindCanonicalDecision(r, "execute_chain", contracts.DecisionAdvice{Kind: "action", Action: &action, Reason: "fixed first action"}, contracts.ModelUsage{Status: "not_applicable"}, r.Control.Audit.Messages, ordered, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Hour)
	in.Facts.Workspace = &contracts.WorkspaceReference{WorkspaceID: "workspace_chain", RunID: r.ID, StorageKey: r.ID}
	in.Facts.ExpectedAction = &action
	in.Facts.OperationSequence = 1
	in.Facts.Budget = governance.BudgetFacts{Available: true, RemainingAttempts: 8, ExecutionDeadline: &deadline}
	validation, err := (governance.Validator{Definitions: s.Definitions}).Validate(in)
	if err != nil || validation.Verdict != "allowed" {
		t.Fatalf("action validation: %v %+v", err, validation.Findings)
	}
	r, err = s.Store.Update(r.ID, func(run *Run) error { return appendCanonicalDecision(run, in, context, validation) })
	if err != nil {
		t.Fatal(err)
	}
	if err = verifyControlChain(r); err != nil {
		t.Fatal("valid action chain", err)
	}
	return s, r
}

func TestControlChainExecutionCannotExpandOrDetachAuthority(t *testing.T) {
	_, original := controlledActionChain(t)
	cases := []struct {
		name   string
		change func(*contracts.ExecutionRequest)
	}{
		{"source", func(x *contracts.ExecutionRequest) { x.SourceVersion++ }},
		{"state sequence", func(x *contracts.ExecutionRequest) { x.StateSequenceAtValidation++ }},
		{"validation hash", func(x *contracts.ExecutionRequest) { x.ValidationHash = strings.Repeat("0", 64) }},
		{"command identity", func(x *contracts.ExecutionRequest) { x.CommandID = "unrelated_command" }},
		{"approval identity", func(x *contracts.ExecutionRequest) { x.Approval.ApprovalID = "unregistered" }},
		{"proposal identity", func(x *contracts.ExecutionRequest) { x.Approval.ProposalVersion++ }},
		{"approval time", func(x *contracts.ExecutionRequest) { x.Approval.ExpiresAt = "2099-01-01T00:00:00Z" }},
		{"creation time", func(x *contracts.ExecutionRequest) { x.CreatedAt = "2099-01-01T00:00:00Z" }},
		{"permission expansion", func(x *contracts.ExecutionRequest) {
			x.RequiredPermissionIDs = append(x.RequiredPermissionIDs, "gitlab_push")
		}},
		{"workspace ownership", func(x *contracts.ExecutionRequest) { x.Workspace.RunID = "other_run" }},
		{"workspace path", func(x *contracts.ExecutionRequest) { x.Workspace.StorageKey = "different" }},
		{"plan binding", func(x *contracts.ExecutionRequest) { x.PlanHash = strings.Repeat("0", 64) }},
		{"operation identity", func(x *contracts.ExecutionRequest) { x.OperationID = "unrelated_operation" }},
		{"execution identity", func(x *contracts.ExecutionRequest) { x.ExecutionID = "unrelated_execution" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := clone(original)
			x := &r.Control.Audit.Executions[0]
			tc.change(x)
			x.RequestHash = chainRehash(t, "ExecutionRequest", x)
			*r.Control.Audit.Validations[1].ExecutionRequest = *x
			assertChainRejected(t, r)
		})
	}
	t.Run("orphan execution", func(t *testing.T) {
		r := clone(original)
		x := r.Control.Audit.Executions[0]
		x.ExecutionID = "orphan_execution"
		x.RequestHash = chainRehash(t, "ExecutionRequest", x)
		r.Control.Audit.Executions = append(r.Control.Audit.Executions, x)
		assertChainRejected(t, r)
	})
	t.Run("private authority absent from archive", func(t *testing.T) {
		r := clone(original)
		a := r.Control.ValidationInputs[1].Facts.Approval
		a.ActorID = "other_actor"
		a.ApprovalHash = chainRehash(t, "Approval", a)
		assertChainRejected(t, r)
	})
	t.Run("future start at past validation", func(t *testing.T) {
		r := clone(original)
		a := r.Control.ValidationInputs[1].Facts.Approval
		stamp := time.Now().Add(time.Minute).UTC().Format(time.RFC3339Nano)
		a.Status = "started"
		a.StartedAt = &stamp
		assertChainRejected(t, r)
	})
	t.Run("coherently rehashed action outside approved plan", func(t *testing.T) {
		r := clone(original)
		d := &r.Control.Audit.Decisions[1]
		body, _ := json.Marshal(map[string]any{"action_type": "scaffold", "parameters": contracts.ScaffoldParameters{PlanHash: strings.Repeat("0", 64)}})
		action := contracts.CandidateAction(body)
		d.Advice.Action = &action
		d.DecisionHash = chainRehash(t, "DecisionProposal", d)
		in := &r.Control.ValidationInputs[1]
		in.Decision = *d
		in.Facts.ExpectedAction = &action
		v := &r.Control.Audit.Validations[1]
		v.NormalizedAdvice = &d.Advice
		identity, err := contracts.HashValue([]any{in.Facts.RunID, in.Facts.SourceVersion, in.Facts.StateSequence, in.Facts.CommandID, d.DecisionID, d.DecisionHash})
		if err != nil {
			t.Fatal(err)
		}
		v.ValidationID = "validation_" + identity[:32]
		v.ValidationHash = chainRehash(t, "ActionValidation", v)
		x := &r.Control.Audit.Executions[0]
		x.Action, x.ValidationID, x.ValidationHash = action, v.ValidationID, v.ValidationHash
		identity, err = contracts.HashValue([]any{v.ValidationID, x.OperationID})
		if err != nil {
			t.Fatal(err)
		}
		x.ExecutionID = "execution_" + identity[:32]
		x.RequestHash = chainRehash(t, "ExecutionRequest", x)
		*v.ExecutionRequest = *x
		assertChainRejected(t, r)
	})
	t.Run("private replay of grant revoked before validation", func(t *testing.T) {
		r := clone(original)
		grant := *r.Control.Audit.PermissionSnapshot
		grant.PermissionVersion++
		grant.Revoked = true
		grant.UpdatedAt = r.Control.Audit.Executions[0].CreatedAt
		grant.PermissionHash = chainRehash(t, "PermissionSnapshot", grant)
		r.Control.Audit.PermissionHistory = append(r.Control.Audit.PermissionHistory, grant)
		r.Control.Audit.PermissionSnapshot = &grant
		r.SourceVersion++
		assertChainRejected(t, r)
	})
}

func TestControlChainKeepsImmutableHistorySeparateFromCurrentLifecycle(t *testing.T) {
	_, r := controlledActionChain(t)
	stamp := time.Now().UTC().Format(time.RFC3339Nano)
	r.Control.Audit.Approval.Status = "started"
	r.Control.Audit.Approval.StartedAt = &stamp
	if err := verifyControlChain(r); err != nil {
		t.Fatal("current start cannot rewrite historical accepted approval", err)
	}
	if r.Control.Audit.ApprovalHistory[0].Status != "accepted" || r.Control.Audit.ApprovalHistory[0].StartedAt != nil {
		t.Fatal("history mutated")
	}
	grant := *r.Control.Audit.PermissionSnapshot
	grant.PermissionVersion++
	grant.Revoked, grant.UpdatedAt = true, stamp
	grant.PermissionHash = chainRehash(t, "PermissionSnapshot", grant)
	r.Control.Audit.PermissionHistory = append(r.Control.Audit.PermissionHistory, grant)
	r.Control.Audit.PermissionSnapshot = &grant
	r.Control.Audit.Approval.Status, r.Control.Audit.Approval.RevokedAt = "revoked", &stamp
	r.SourceVersion++
	if err := verifyControlChain(r); err != nil {
		t.Fatal("later trusted revocation cannot erase earlier validated history", err)
	}
	changed := clone(r)
	changed.Control.Audit.Approval.AcceptedAt = "2020-01-01T00:00:00Z"
	changed.Control.Audit.Approval.ApprovalHash = chainRehash(t, "Approval", changed.Control.Audit.Approval)
	assertChainRejected(t, changed)
	if err := verifyControlChain(Run{}); err != nil {
		t.Fatal("legacy run should remain outside S2", err)
	}
}

func TestControlChainPreservesRejectedAdviceWithoutExecutionAuthority(t *testing.T) {
	s, r := controlledActionChain(t)
	body, _ := json.Marshal(map[string]any{"action_type": "scaffold", "parameters": contracts.ScaffoldParameters{PlanHash: strings.Repeat("0", 64)}})
	action := contracts.CandidateAction(body)
	in, record, err := bindCanonicalDecision(r, "rejected_action", contracts.DecisionAdvice{Kind: "action", Action: &action, Reason: "invalid model candidate retained for audit"}, contracts.ModelUsage{Status: "not_applicable"}, r.Control.Audit.Messages, r.Control.Audit.Contexts[0].Messages, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	validation, err := (governance.Validator{Definitions: s.Definitions}).Validate(in)
	if err != nil || validation.Verdict != "rejected" || validation.ExecutionRequest != nil {
		t.Fatalf("expected real rejection: %v %+v", err, validation)
	}
	count := len(r.Control.Audit.Executions)
	if err = appendCanonicalDecision(&r, in, record, validation); err != nil {
		t.Fatal(err)
	}
	if err = verifyControlChain(r); err != nil {
		t.Fatal(err)
	}
	if len(r.Control.Audit.Executions) != count {
		t.Fatal("rejected candidate acquired execution authority")
	}
}
