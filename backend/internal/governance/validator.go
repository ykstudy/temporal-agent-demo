package governance

import (
	"encoding/json"
	"fmt"
	"path"
	"sort"
	"strings"
	"time"

	"agentstudio/internal/contracts"
)

// Validate produces a complete immutable result. Structurally malformed advice
// returns an error; a well-shaped but disallowed candidate produces a rejected
// result with no normalized advice and no execution request.
func (v Validator) Validate(in Input) (contracts.ActionValidation, error) {
	d, f := in.Decision, in.Facts
	if err := contracts.Validate("DecisionProposal", d); err != nil {
		return contracts.ActionValidation{}, reject("invalid_decision_schema", "decision violates its published schema")
	}
	if f.Now.IsZero() || f.RunID == "" || f.SourceVersion < 1 || f.StateSequence < 1 || f.CommandID == "" {
		return contracts.ActionValidation{}, reject("invalid_facts", "trusted identity, version, command and time are required")
	}
	idHash, err := contracts.HashValue([]any{f.RunID, f.SourceVersion, f.StateSequence, f.CommandID, d.DecisionID, d.DecisionHash})
	if err != nil {
		return contracts.ActionValidation{}, err
	}
	out := contracts.ActionValidation{
		ContractVersion: d.ContractVersion, ValidationID: "validation_" + idHash[:32], DecisionID: d.DecisionID,
		RunID: f.RunID, SourceVersion: f.SourceVersion, StateSequence: f.StateSequence,
		DefinitionSnapshotHash: f.FrozenDefinition.SnapshotHash, DecisionKind: d.Advice.Kind,
		Verdict: "allowed", Findings: []contracts.ValidationFinding{}, ValidatedAt: f.Now.UTC().Format(time.RFC3339Nano),
	}
	problem := v.validateInput(in)
	var permissions []string
	if problem == nil {
		switch d.Advice.Kind {
		case "clarification":
			if !oneOf(f.State, "INTAKE", "CLARIFY", "PLAN", "VERIFY", "BLOCKED") {
				problem = reject("invalid_state", "clarification is not applicable in this state")
			} else {
				out.Verdict = "needs_clarification"
			}
		case "plan":
			if !oneOf(f.State, "INTAKE", "CLARIFY", "PLAN") && !(f.State == "VERIFY" && f.RevisionAllowed) {
				problem = reject("invalid_state", "plan revision requires a permitted planning boundary")
			} else {
				problem = v.validatePlanAdvice(f, *d.Advice.Plan)
			}
		case "action":
			permissions, problem = v.validateAction(f, *d.Advice.Action)
		case "diagnosis":
			if !oneOf(f.State, "VERIFY", "BLOCKED", "FAILED") {
				problem = reject("invalid_state", "diagnosis requires a failed or blocked verification context")
			}
			if problem == nil {
				for _, ref := range d.Advice.Diagnosis.EvidenceRefs {
					if !containsRef(f.AvailableEvidence, ref) {
						problem = reject("evidence_unavailable", "diagnosis references evidence outside the trusted input")
						break
					}
				}
			}
		case "completion":
			if !oneOf(f.State, "VERIFY", "DELIVER", "DONE") {
				problem = reject("invalid_state", "completion advice is not applicable in this state")
			}
		}
	}
	if problem != nil {
		out.Verdict = "rejected"
		out.Findings = append(out.Findings, contracts.ValidationFinding{RuleID: "governance.authorization", Code: problem.Code, Detail: problem.Detail})
	} else {
		out.Findings = append(out.Findings, contracts.ValidationFinding{RuleID: "governance.authorization", Passed: true, Code: out.Verdict, Detail: "Published schema, input provenance and applicable control-plane rules were checked"})
		if out.Verdict == "allowed" {
			// Keep immutable results independent from later mutations of the
			// caller's current plan and raw action buffers.
			var advice contracts.DecisionAdvice
			encoded, copyErr := json.Marshal(d.Advice)
			if copyErr != nil {
				return out, copyErr
			}
			if copyErr = json.Unmarshal(encoded, &advice); copyErr != nil {
				return out, copyErr
			}
			out.NormalizedAdvice = &advice
		}
	}
	out.ValidationHash, err = contracts.HashObject("ActionValidation", out)
	if err != nil {
		return out, err
	}
	if out.Verdict == "allowed" && d.Advice.Kind == "action" {
		request, buildErr := buildRequest(f, out, permissions)
		if buildErr != nil {
			return out, buildErr
		}
		targetHash, targetErr := RequestTargetHash(request)
		if targetErr != nil {
			return out, targetErr
		}
		if f.Operation != nil && (f.Operation.OperationID != request.OperationID || f.Operation.RequestHash != request.RequestHash || f.Operation.TargetHash != targetHash) {
			out.Verdict, out.NormalizedAdvice = "rejected", nil
			out.Findings = []contracts.ValidationFinding{{RuleID: "governance.operation", Code: "operation_conflict", Detail: "persisted operation belongs to another execution request"}}
			out.ValidationHash, err = contracts.HashObject("ActionValidation", out)
			if err != nil {
				return out, err
			}
		} else {
			out.ExecutionRequest = &request
		}
	}
	if err = contracts.Validate("ActionValidation", out); err != nil {
		return out, reject("invalid_validation_result", "validation result cannot satisfy the published schema")
	}
	return out, nil
}

func (v Validator) validateInput(in Input) *Error {
	f, d := in.Facts, in.Decision
	if v.Definitions == nil || v.Definitions.VerifySnapshot(f.FrozenDefinition) != nil {
		return reject("definition_unavailable", "the exact frozen definition closure could not be verified")
	}
	if d.RunID != f.RunID || d.SourceVersion != f.SourceVersion || d.StateSequence != f.StateSequence || d.DefinitionSnapshotHash != f.FrozenDefinition.SnapshotHash || d.DecisionMode != f.Modes.DecisionMode {
		return reject("decision_binding_mismatch", "decision is not bound to the current run, source, state and definition")
	}
	if d.ContractVersion != f.FrozenDefinition.ContractVersion {
		return reject("contract_version_mismatch", "decision and frozen definition belong to different contract publications")
	}
	if err := checkHash("DecisionProposal", d, d.DecisionHash); err != nil {
		return err
	}
	created, err := time.Parse(time.RFC3339Nano, d.CreatedAt)
	if err != nil || created.After(f.Now) {
		return reject("invalid_decision_time", "decision creation time is invalid or in the future")
	}
	if f.ActorID == "" || contracts.Validate("Modes", f.Modes) != nil || f.ObservedModelUsage == nil || !equalJSON(d.ModelUsage, *f.ObservedModelUsage) {
		return reject("decision_provenance_mismatch", "trusted actor, mode and observed model usage are required")
	}
	if f.Modes.DecisionMode == "rules" && d.ModelUsage.Status != "not_applicable" || f.Modes.DecisionMode == "model" && !oneOf(d.ModelUsage.Status, "known", "unknown") {
		return reject("model_usage_mismatch", "model usage status does not match the explicit decision mode")
	}
	if f.Modes.DecisionMode == "model" && f.FrozenDefinition.ProviderBinding == nil {
		return reject("provider_binding_missing", "model advice requires the actual frozen provider binding")
	}
	if d.ModelUsage.Status == "known" && (*d.ModelUsage.InputTokens > *d.ModelUsage.TotalTokens || *d.ModelUsage.OutputTokens != *d.ModelUsage.TotalTokens-*d.ModelUsage.InputTokens) {
		return reject("model_usage_mismatch", "provider token totals do not agree")
	}
	if len(in.InputMessages) != len(d.InputMessageRefs) || in.InputMessages == nil || in.OrderedContext == nil || len(in.OrderedContext) == 0 {
		return reject("input_provenance_mismatch", "exact persisted messages and ordered input context are required")
	}
	seen := map[string]bool{}
	var sequence int64
	for i, message := range in.InputMessages {
		ref, err := MessageRef(message)
		if err != nil || message.RunID != f.RunID || message.SourceVersion > f.SourceVersion || message.StateSequence > f.StateSequence || message.Sequence <= sequence || seen[message.MessageID] || !equalJSON(message.Modes, f.Modes) || ref != d.InputMessageRefs[i] {
			return reject("input_provenance_mismatch", "message reference, hash, run, mode or sequence does not match its persisted envelope")
		}
		sequence, seen[message.MessageID] = message.Sequence, true
	}
	for _, item := range in.OrderedContext {
		if !oneOf(item.Role, "system", "user", "assistant", "tool") {
			return reject("input_context_mismatch", "context contains an unsupported role")
		}
	}
	contextHash, err := contracts.HashValue(in.OrderedContext)
	if err != nil || contextHash != d.InputContextHash {
		return reject("input_context_mismatch", "decision does not bind the exact ordered context")
	}
	return nil
}

func (v Validator) validatePlanAdvice(f Facts, plan contracts.ProjectPlan) *Error {
	if err := v.validatePlan(f, plan); err != nil {
		return err
	}
	if plan.VerificationCandidateRef != nil {
		ref := contracts.ObjectReference(*plan.VerificationCandidateRef)
		if !contracts.DeliveryContract(plan.ContractVersion) || f.State != "VERIFY" || !f.RevisionAllowed || ref.SourceVersion != f.SourceVersion || !containsRef(f.AvailableEvidence, ref) {
			return reject("candidate_not_available", "verification revision requires exact registered evidence at the current VERIFY boundary")
		}
	}
	return nil
}

func (v Validator) validatePlan(f Facts, plan contracts.ProjectPlan) *Error {
	if err := checkHash("ProjectPlan", plan, plan.PlanHash); err != nil {
		return err
	}
	if plan.ContractVersion != f.FrozenDefinition.ContractVersion {
		return reject("contract_version_mismatch", "plan and frozen definition belong to different contract publications")
	}
	if !contains(f.FrozenDefinition.Definition.SupportedStacks, plan.TechStack) || !equalJSON(plan.TemplateDependencies, f.FrozenDefinition.TemplateDependencies) {
		return reject("plan_dependency_mismatch", "plan stack or template closure differs from the frozen definition")
	}
	var target struct {
		Provider string `json:"provider"`
	}
	if json.Unmarshal(plan.DeliveryTarget, &target) != nil || target.Provider != f.Modes.DeliveryMode {
		return reject("target_mismatch", "plan delivery target does not match the Run delivery mode")
	}
	if target.Provider == "gitlab" {
		var remote contracts.GitLabTarget
		if contracts.Decode("GitLabTarget", plan.DeliveryTarget, &remote) != nil || remote.ProjectPath != plan.ProjectName || remote.Ref != f.FrozenDefinition.Definition.RepositoryPolicy.RequiredRef || remote.Visibility != f.FrozenDefinition.Definition.RepositoryPolicy.Visibility {
			return reject("target_mismatch", "plan repository target violates the frozen repository policy")
		}
	}
	ids := []string{}
	previous := ""
	for _, raw := range plan.SelectedSkills {
		var selection selectionFields
		if json.Unmarshal(raw, &selection) != nil || selection.SkillID <= previous {
			return reject("invalid_skill_selection", "skills must be unique and sorted by registered skill identity")
		}
		previous = selection.SkillID
		skill := findSkill(f.FrozenDefinition, selection.SkillID)
		if skill == nil || skill.SkillVersion != selection.SkillVersion || skill.SkillHash != selection.SkillHash {
			return reject("invalid_skill_selection", "plan skill does not match its frozen version and hash")
		}
		normalized, err := v.Definitions.NormalizeParameters(f.FrozenDefinition, selection.SkillID, selection.Parameters)
		if err != nil || !equalJSON(normalized, selection.Parameters) {
			return reject("invalid_skill_parameters", "skill parameters are invalid or not normalized")
		}
		ids = append(ids, selection.SkillID)
	}
	if v.Definitions.ValidateSelection(f.FrozenDefinition, ids) != nil {
		return reject("invalid_skill_dependencies", "skill dependencies or conflicts are not satisfied")
	}
	return nil
}

type selectionFields struct {
	SkillID, SkillVersion, SkillHash string
	Parameters                       map[string]any `json:"parameters"`
}

func (s *selectionFields) UnmarshalJSON(data []byte) error {
	var fields struct {
		SkillID      string         `json:"skill_id"`
		SkillVersion string         `json:"skill_version"`
		SkillHash    string         `json:"skill_hash"`
		Parameters   map[string]any `json:"parameters"`
	}
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	s.SkillID, s.SkillVersion, s.SkillHash, s.Parameters = fields.SkillID, fields.SkillVersion, fields.SkillHash, fields.Parameters
	return nil
}

func checkHash(name string, value any, expected string) *Error {
	if contracts.Validate(name, value) != nil {
		return reject("invalid_"+strings.ToLower(name), "object violates its published schema")
	}
	actual, err := contracts.HashObject(name, value)
	if err != nil || actual != expected {
		return reject("hash_mismatch", "object content does not match its immutable hash")
	}
	return nil
}

func equalJSON(a, b any) bool {
	ah, e := contracts.HashValue(a)
	bh, e2 := contracts.HashValue(b)
	return e == nil && e2 == nil && ah == bh
}
func contains(values []string, value string) bool {
	for _, item := range values {
		if item == value {
			return true
		}
	}
	return false
}
func oneOf(value string, values ...string) bool { return contains(values, value) }
func containsRef(values []contracts.ObjectReference, value contracts.ObjectReference) bool {
	for _, item := range values {
		if item == value {
			return true
		}
	}
	return false
}
func findSkill(snapshot contracts.DefinitionSnapshot, id string) *contracts.SkillDefinition {
	for i := range snapshot.Skills {
		if snapshot.Skills[i].SkillID == id {
			return &snapshot.Skills[i]
		}
	}
	return nil
}
func sortedStrings(values []string) bool {
	return sort.StringsAreSorted(values) && len(unique(values)) == len(values)
}
func unique(values []string) map[string]bool {
	out := map[string]bool{}
	for _, value := range values {
		out[value] = true
	}
	return out
}
func safePath(value string) bool {
	return value != "" && value != "." && !strings.Contains(value, "\\") && !strings.HasPrefix(value, "/") && !strings.HasPrefix(value, "../") && path.Clean(value) == value
}
func hashID(prefix string, values ...any) string {
	hash, err := contracts.HashValue(values)
	if err != nil {
		panic(fmt.Sprintf("internal identity hashing: %v", err))
	}
	return prefix + hash[:32]
}
