package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"agentstudio/internal/contracts"
)

func TestModelAdviceFiveKindsPreserveObservedUsageWithoutAuthority(t *testing.T) {
	s := testService(t)
	var wire atomic.Value
	wire.Store(`{}`)
	var calls atomic.Int64
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		calls.Add(1)
		if request.Header.Get("Authorization") != "Bearer provider-secret-value" {
			t.Error("credential transport mismatch")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(wire.Load().(string)))
	}))
	defer provider.Close()
	s.Config.AIEndpoint = provider.URL
	s.Config.AIKey = "provider-secret-value"
	s.Config.AIModel = "frozen-model"
	r, err := s.CreateWithOptions(context.Background(), CreateOptions{ID: "five_advice", DecisionMode: "model"})
	if err != nil {
		t.Fatal(err)
	}
	p := r.Plan
	p.ProjectName = "advice-service"
	plan, err := projectPlanFor(r, p)
	if err != nil {
		t.Fatal(err)
	}
	question, completion := "What project name?", "The candidate work may be complete; verify all required checks."
	action := contracts.CandidateAction([]byte(`{"action_type":"scaffold","parameters":{"plan_hash":"` + plan.PlanHash + `"}}`))
	advice := []contracts.DecisionAdvice{
		{Kind: "clarification", Question: &question, Reason: "missing input"},
		{Kind: "plan", Plan: &plan, Reason: "parsed requirements"},
		{Kind: "action", Action: &action, Reason: "candidate only"},
		{Kind: "diagnosis", Diagnosis: &contracts.Diagnosis{Category: "infrastructure", Code: "temporary_failure", Message: "Tool temporarily unavailable", EvidenceRefs: []contracts.ObjectReference{}, SuggestedRemedy: "Retry within the frozen bound"}, Reason: "diagnostic suggestion"},
		{Kind: "completion", CompletionHint: &completion, Reason: "completion suggestion"},
	}
	request, err := buildModelRequest(r, "actual input", "frozen-model")
	if err != nil {
		t.Fatal(err)
	}
	for _, candidate := range advice {
		t.Run(candidate.Kind, func(t *testing.T) {
			content, e := json.Marshal(candidate)
			if e != nil {
				t.Fatal(e)
			}
			response := map[string]any{"id": "provider-request", "choices": []map[string]any{{"message": map[string]string{"content": string(content)}}}}
			if candidate.Kind == "plan" {
				response["usage"] = map[string]int64{"prompt_tokens": 0, "completion_tokens": 0, "total_tokens": 0}
			}
			if candidate.Kind == "diagnosis" {
				response["usage"] = map[string]int64{"prompt_tokens": 5, "completion_tokens": 7, "total_tokens": 1}
			}
			data, e := json.Marshal(response)
			if e != nil {
				t.Fatal(e)
			}
			wire.Store(string(data))
			got, e := s.callModelAdvice(context.Background(), r, request)
			if e != nil || got.Advice.Kind != candidate.Kind {
				t.Fatal(got.Advice.Kind, e)
			}
			if e = contracts.ValidateVersion("DecisionAdvice", r.FrozenDefinition.ContractVersion, got.Advice); e != nil {
				t.Fatal(e)
			}
			if e = contracts.ValidateVersion("ModelUsage", r.FrozenDefinition.ContractVersion, got.Usage); e != nil {
				t.Fatal(e)
			}
			if candidate.Kind == "plan" {
				if got.Usage.Status != "known" || got.Usage.TotalTokens == nil || *got.Usage.TotalTokens != 0 {
					t.Fatal("actual known zero lost")
				}
			} else if got.Usage.Status != "unknown" || got.Usage.TotalTokens != nil {
				t.Fatal("missing/inconsistent usage fabricated a number")
			}
			if len(got.Messages) == 0 || got.Messages[len(got.Messages)-1].Content != "actual input" {
				t.Fatal("actual submitted context lost")
			}
			after, _ := s.Store.Get(r.ID)
			if after.State != "INTAKE" || after.Approval != nil || after.Proposal != nil {
				t.Fatal("model response granted authority or advanced Run")
			}
		})
	}
	before := calls.Load()
	rules := r
	rules.DecisionMode = "rules"
	if _, err = s.callModelAdvice(context.Background(), rules, request); err == nil || calls.Load() != before {
		t.Fatal("rules mode invoked model transport")
	}
	bad := `{"kind":"completion","question":null,"plan":null,"action":null,"diagnosis":null,"completion_hint":"DONE","reason":"approved","approved":true}`
	body, _ := json.Marshal(map[string]any{"choices": []map[string]any{{"message": map[string]string{"content": bad}}}})
	wire.Store(string(body))
	if _, err = s.callModelAdvice(context.Background(), r, request); err == nil {
		t.Fatal("model invented approval field accepted")
	}
	if strings.Contains(string(request), s.Config.AIKey) {
		t.Fatal("credential entered model input")
	}
}
