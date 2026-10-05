package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"agentstudio/internal/contracts"
	"agentstudio/internal/definitions"
	"agentstudio/internal/governance"
	"agentstudio/internal/storage"
)

// This fixture uses a real independently published 1.2 definition and a fresh
// unstarted Run. It exercises the S2 modules, not the still-legacy scheduler.
func controlFixture(t *testing.T, s *Service, id string, modes ...string) Run {
	t.Helper()
	bundles, err := releaseTemplatePrograms()
	if err != nil {
		t.Fatal(err)
	}
	for i := range bundles {
		bundles[i].Version = "9.2.0"
	}
	platform := runtime.GOOS + "_" + runtime.GOARCH
	definition, err := s.Definitions.PublishDefault(definitions.PublishOptions{ContractVersion: "1.2.0", DefinitionVersion: "9.2.0", SkillVersion: "9.2.0", TemplateVersion: "9.2.0", SystemPrompt: defaultSystemPrompt, Tools: []contracts.ToolIdentity{{ToolID: "go", Version: runtime.Version(), Platform: &platform}}, Templates: bundles})
	if err != nil {
		t.Fatal(err)
	}
	mode := "rules"
	var provider *contracts.ProviderBinding
	if len(modes) > 0 {
		mode = modes[0]
	}
	if mode == "model" {
		provider = &contracts.ProviderBinding{ProviderKind: "openai_compatible", Endpoint: s.Config.AIEndpoint, ModelID: s.Config.AIModel}
	}
	frozen, err := s.Definitions.Freeze(definition.DefinitionID, definition.DefinitionVersion, definition.DefinitionHash, provider, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	r := Run{ID: id, Title: id, State: "INTAKE", SourceVersion: 1, Version: 1, StateSequence: 1, FrozenDefinition: &frozen, Definition: legacyDefinition(frozen), Plan: defaultPlan(), ExecutionMode: "local", DecisionMode: mode, DeliveryMode: "local", Messages: []Message{}, Steps: initialSteps(), Artifacts: []Artifact{}, History: []History{}, Operations: map[string]OperationRecord{}, Processed: map[string]string{}, CreatedAt: now(), UpdatedAt: now()}
	budget, err := governance.NewBudgetLedger(id, frozen.SnapshotHash, frozen.Definition.BudgetPolicy, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	r.Control, err = newControlState(r, budget)
	if err != nil {
		t.Fatal(err)
	}
	r, err = s.Store.Create(r)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func controlledPlan(t *testing.T, s *Service, r Run) Run {
	t.Helper()
	command := Command{ID: "input_" + r.ID, RunID: r.ID, Content: "创建 controlled-service，Postgres GORM JWT"}
	var message contracts.MessageEnvelope
	r, err := s.Store.Update(r.ID, func(run *Run) error {
		var e error
		message, e = appendCanonicalInput(run, command, time.Now().UTC())
		return e
	})
	if err != nil {
		t.Fatal(err)
	}
	p := ParsePlan(r.Plan, command.Content)
	p, err = s.normalizeFrozenPlan(r, p)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := projectPlanFor(r, p)
	if err != nil {
		t.Fatal(err)
	}
	advice := contracts.DecisionAdvice{Kind: "plan", Plan: &plan, Reason: "explicit rules parsed persisted user input"}
	ordered := []contracts.ContextMessage{{Role: "system", Content: r.FrozenDefinition.Definition.SystemPrompt.Content}, {Role: "user", Content: command.Content}}
	in, record, err := bindCanonicalDecision(r, command.ID, advice, contracts.ModelUsage{Status: "not_applicable"}, []contracts.MessageEnvelope{message}, ordered, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	validation, err := (governance.Validator{Definitions: s.Definitions}).Validate(in)
	if err != nil || validation.Verdict != "allowed" {
		t.Fatalf("plan validation: %s %v %+v", validation.Verdict, err, validation.Findings)
	}
	r, err = s.Store.Update(r.ID, func(run *Run) error {
		if e := s.recordCanonicalDecision(run, in, record, validation); e != nil {
			return e
		}
		run.Plan = p
		sourceChanged(run)
		files, e := s.renderFrozenFiles(*run)
		if e != nil {
			return e
		}
		if e = s.makeCanonicalProposal(run, plan, files, time.Now().UTC()); e != nil {
			return e
		}
		run.State = "PLAN"
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestCanonicalObjectsPersistExactDecisionAndApprovalHistory(t *testing.T) {
	s := testService(t)
	r := controlledPlan(t, s, controlFixture(t, s, "canonical_history"))
	a := r.Control.Audit
	if len(a.Decisions) != 1 || len(a.Validations) != 1 || len(a.Contexts) != 1 || len(a.Messages) != 1 || len(a.Executions) != 0 {
		t.Fatal("planning produced missing objects or execution authority")
	}
	if a.Decisions[0].SourceVersion != 1 || a.SourceVersion != 2 || a.Messages[0].SourceVersion != 1 {
		t.Fatal("progress or source identities were rewritten")
	}
	if a.Proposal == nil || a.PermissionSnapshot == nil || len(a.ProposalHistory) != 1 {
		t.Fatal("full proposal/permission not registered")
	}
	proposalRef, err := storage.Reference(r.ID, a.Proposal.SourceVersion, "Proposal", *a.Proposal)
	if err != nil {
		t.Fatal(err)
	}
	initial, err := s.Store.CanonicalObject(r.ID, proposalRef)
	if err != nil {
		t.Fatal(err)
	}
	cmd := Command{ID: "accept_canonical", RunID: r.ID, Action: "accept", ProposalID: a.Proposal.ProposalID, ProposalVersion: int(a.Proposal.ProposalVersion), SourceVersion: r.SourceVersion, SnapshotHash: a.Proposal.SnapshotHash}
	r, err = s.Store.Update(r.ID, func(run *Run) error { return s.acceptCanonical(run, cmd, time.Now().UTC()) })
	if err != nil {
		t.Fatal(err)
	}
	if r.Control.Audit.Approval == nil || r.Control.Audit.Approval.StartedAt != nil || len(r.Control.Audit.Executions) != 0 {
		t.Fatal("accept did not preserve first-application boundary")
	}
	after, err := s.Store.CanonicalObject(r.ID, proposalRef)
	if err != nil || string(after) != string(initial) {
		t.Fatal("current lifecycle replaced immutable proposal", err)
	}
	if r.Control.Audit.ProposalHistory[0].Status != "pending" || r.Control.Audit.Proposal.Status != "accepted" {
		t.Fatal("lifecycle leaked into immutable history")
	}
	before, _ := contracts.HashObject("S2AuditBundle", r.Control.Audit)
	cfg := s.Config
	s.Close()
	reopened, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	restored, err := reopened.Store.CanonicalAudit(r.ID)
	if err != nil {
		t.Fatal(err)
	}
	// Local restart explicitly records the interruption, while immutable chain
	// bodies and approval time stay unchanged.
	if restored.State != "BLOCKED" || restored.Approval.ApprovalHash != r.Control.Audit.Approval.ApprovalHash || restored.Approval.AcceptedAt != r.Control.Audit.Approval.AcceptedAt {
		t.Fatal("restart rewrote authority")
	}
	if restored.AuditHash == before {
		t.Fatal("restart state change missing from audit hash")
	}
	body, err := reopened.Store.CanonicalObject(r.ID, proposalRef)
	if err != nil || string(body) != string(initial) {
		t.Fatal("archived proposal lost on restart", err)
	}
}

func TestCanonicalReadOnlyRoutesExactReferencesAndLegacyBoundary(t *testing.T) {
	s := testService(t)
	r := controlledPlan(t, s, controlFixture(t, s, "canonical_http"))
	_, notReady := s.CreateWithOptions(context.Background(), CreateOptions{ID: "unready_release", DefinitionID: r.FrozenDefinition.DefinitionID, DefinitionVersion: r.FrozenDefinition.DefinitionVersion, DefinitionHash: r.FrozenDefinition.DefinitionHash})
	if api, ok := notReady.(*APIError); !ok || api.Code != "definition_not_ready" {
		t.Fatal("unwired contract ran through legacy creation", notReady)
	}
	if _, readErr := s.Store.Get("unready_release"); readErr != ErrNotFound {
		t.Fatal("unready release persisted a legacy Run", readErr)
	}
	old, err := s.Create(context.Background(), "old_http", "", "")
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	s.Register(mux)
	read := func(path string) *httptest.ResponseRecorder {
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, httptest.NewRequest("GET", path, nil))
		return rr
	}
	before := r.StateSequence
	for _, entry := range []struct{ suffix, schema string }{{"decision-audit", "S2AuditBundle"}, {"budget", "BudgetLedger"}} {
		response := read("/api/runs/" + r.ID + "/" + entry.suffix)
		if response.Code != 200 {
			t.Fatal(response.Code, response.Body.String())
		}
		if err = contracts.ValidateJSON(entry.schema, response.Body.Bytes()); err != nil {
			t.Fatal(err)
		}
		if entry.suffix == "budget" {
			link := response.Header().Get("Link")
			if !strings.HasPrefix(link, "<") || !strings.HasSuffix(link, ">; rel=\"canonical\"") {
				t.Fatal("budget original source is not discoverable", link)
			}
			path := strings.TrimSuffix(strings.TrimPrefix(link, "<"), ">; rel=\"canonical\"")
			target, parseErr := url.Parse(path)
			if parseErr != nil || target.Query().Get("source_version") != "1" {
				t.Fatal("budget reference guessed current source", link)
			}
			original := read(path)
			if original.Code != 200 || contracts.ValidateJSON("BudgetLedger", original.Body.Bytes()) != nil {
				t.Fatal("budget exact object link unreadable", original.Code, original.Body.String())
			}
		}
		response = read("/api/runs/" + old.ID + "/" + entry.suffix)
		if response.Code != 409 || !strings.Contains(response.Body.String(), "legacy_read_url") {
			t.Fatal("legacy source fabricated", response.Code, response.Body.String())
		}
	}
	ref, err := governance.MessageRef(r.Control.Audit.Messages[0])
	if err != nil {
		t.Fatal(err)
	}
	q := url.Values{"object_type": {ref.ObjectType}, "object_id": {ref.ObjectID}, "object_version": {fmt.Sprint(ref.ObjectVersion)}, "source_version": {fmt.Sprint(ref.SourceVersion)}, "hash": {ref.Hash}}
	base := "/api/runs/" + r.ID + "/objects?"
	response := read(base + q.Encode())
	if response.Code != 200 || contracts.ValidateJSON("MessageEnvelope", response.Body.Bytes()) != nil {
		t.Fatal(response.Code, response.Body.String())
	}
	q.Set("source_version", "99")
	if got := read(base + q.Encode()); got.Code != 404 {
		t.Fatal("source alias accepted", got.Code, got.Body.String())
	}
	q.Set("source_version", fmt.Sprint(ref.SourceVersion))
	q.Add("hash", ref.Hash)
	if got := read(base + q.Encode()); got.Code != 400 {
		t.Fatal("duplicate query accepted", got.Code)
	}
	q.Del("hash")
	if got := read(base + q.Encode()); got.Code != 400 {
		t.Fatal("incomplete reference accepted", got.Code)
	}
	q.Set("hash", ref.Hash)
	q.Set("object_id", "../escape")
	if got := read(base + q.Encode()); got.Code != 400 {
		t.Fatal("path accepted", got.Code)
	}
	after, _ := s.Store.Get(r.ID)
	if after.StateSequence != before || len(after.Control.Audit.Executions) != 0 {
		t.Fatal("GET caused a mutation")
	}
}

func TestCanonicalRejectsStaleApprovalAndArchiveMutation(t *testing.T) {
	s := testService(t)
	r := controlledPlan(t, s, controlFixture(t, s, "canonical_reject"))
	p := r.Control.Audit.Proposal
	cmd := Command{ID: "accept", RunID: r.ID, Action: "accept", ProposalID: p.ProposalID, ProposalVersion: int(p.ProposalVersion), SourceVersion: r.SourceVersion, SnapshotHash: p.SnapshotHash}
	for _, mode := range []string{"hash", "source", "expired", "revoked", "actor"} {
		t.Run(mode, func(t *testing.T) {
			candidate := clone(r)
			input := cmd
			at := time.Now().UTC()
			switch mode {
			case "hash":
				input.SnapshotHash = strings.Repeat("0", 64)
			case "source":
				input.SourceVersion++
			case "expired":
				at, _ = time.Parse(time.RFC3339Nano, p.ExpiresAt)
			case "revoked":
				candidate.Control.Audit.PermissionSnapshot.Revoked = true
			case "actor":
				candidate.Control.Audit.PermissionSnapshot.ActorID = "another-user"
			}
			if err := s.acceptCanonical(&candidate, input, at); err == nil {
				t.Fatal("invalid approval accepted")
			}
			if candidate.Control.Audit.Approval != nil || len(candidate.Control.Audit.Executions) != 0 {
				t.Fatal("failed confirmation acquired authority")
			}
		})
	}
	if _, err := s.Store.Update(r.ID, func(run *Run) error { run.Control.Audit.ProposalHistory[0].Status = "accepted"; return nil }); err == nil {
		t.Fatal("mutated archive accepted because status excluded from hash")
	}
	missingIndex := clone(r)
	missingIndex.Control.ObjectRefs = nil
	if err := s.Store.verifyCanonical(missingIndex); err == nil {
		t.Fatal("missing complete index was accepted")
	}
	proposalRef, err := storage.Reference(r.ID, p.SourceVersion, "Proposal", *p)
	if err != nil {
		t.Fatal(err)
	}
	proposalPath := filepath.Join(s.Config.DataDir, "objects", r.ID, "proposal", p.ProposalID, fmt.Sprint(p.ProposalVersion), fmt.Sprint(p.SourceVersion), proposalRef.Hash+".json")
	body, err := os.ReadFile(proposalPath)
	if err != nil {
		t.Fatal(err)
	}
	var tampered contracts.Proposal
	if err = json.Unmarshal(body, &tampered); err != nil {
		t.Fatal(err)
	}
	tampered.Status = "accepted"
	encoded, err := json.Marshal(tampered)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err = contracts.Canonicalize(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(proposalPath, encoded, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Store.CanonicalObject(r.ID, proposalRef); err == nil {
		t.Fatal("excluded lifecycle field changed archived raw body")
	}
	if err = os.WriteFile(proposalPath, body, 0600); err != nil {
		t.Fatal(err)
	}
	ref, err := governance.MessageRef(r.Control.Audit.Messages[0])
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(s.Config.DataDir, "objects", r.ID, ref.ObjectType, ref.ObjectID, "1", fmt.Sprint(ref.SourceVersion), ref.Hash+".json")
	if err = os.WriteFile(file, []byte(`{"tampered":true}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Store.CanonicalAudit(r.ID); err == nil {
		t.Fatal("corrupt registered object read as valid")
	}
	if _, err = s.Store.CanonicalObject(r.ID, ref); err == nil {
		t.Fatal("corrupt body returned")
	}
}

func TestCanonicalInputPreservesLongTextAndRevocationCannotRegrant(t *testing.T) {
	s := testService(t)
	r := controlFixture(t, s, "canonical_long")
	text := strings.Repeat("原", 4097)
	cmd := Command{ID: "long_input", RunID: r.ID, Content: text}
	message, err := appendCanonicalInput(&r, cmd, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	var part contracts.TextPart
	if err = json.Unmarshal(message.Parts[0], &part); err != nil {
		t.Fatal(err)
	}
	if part.Text != text || len([]rune(part.FallbackText)) != 4096 {
		t.Fatal("original long input was truncated or fallback exceeded bound")
	}
	duplicate, err := appendCanonicalInput(&r, cmd, time.Now().UTC())
	if err != nil || duplicate.MessageID != message.MessageID || duplicate.CreatedAt != message.CreatedAt || len(r.Control.Audit.Messages) != 1 {
		t.Fatal("retry rewrote input identity", err)
	}
	r = controlledPlan(t, s, controlFixture(t, s, "canonical_revoked"))
	grant := r.Control.Audit.PermissionSnapshot
	grant.Revoked = true
	grant.PermissionVersion++
	grant.UpdatedAt = now()
	grant.PermissionHash, err = contracts.HashObject("PermissionSnapshot", *grant)
	if err != nil {
		t.Fatal(err)
	}
	r.Control.Audit.PermissionHistory = append(r.Control.Audit.PermissionHistory, *grant)
	sourceChanged(&r)
	files, err := s.renderFrozenFiles(r)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.makeCanonicalProposal(&r, *r.Control.Audit.Plan, files, time.Now().UTC()); err == nil {
		t.Fatal("ordinary replan regranted revoked permissions")
	}
	if !r.Control.Audit.PermissionSnapshot.Revoked {
		t.Fatal("revocation was cleared")
	}
}

func TestCanonicalCurrentPermissionCannotReplayOlderGrant(t *testing.T) {
	s := testService(t)
	r := controlledPlan(t, s, controlFixture(t, s, "canonical_grant_replay"))
	newer := *r.Control.Audit.PermissionSnapshot
	newer.PermissionVersion++
	newer.Revoked = true
	newer.UpdatedAt = now()
	var err error
	newer.PermissionHash, err = contracts.HashObject("PermissionSnapshot", newer)
	if err != nil {
		t.Fatal(err)
	}
	r.Control.Audit.PermissionHistory = append(r.Control.Audit.PermissionHistory, newer)
	sourceChanged(&r)
	if err = verifyControlChain(r); err == nil {
		t.Fatal("current pointer replayed older unrevoked permission")
	}
	r.Control.Audit.PermissionSnapshot = &newer
	if err = verifyControlChain(r); err != nil {
		t.Fatal("valid latest revoked permission rejected", err)
	}
}
