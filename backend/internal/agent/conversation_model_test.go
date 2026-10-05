package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"agentstudio/internal/contracts"
)

func TestConversationModelSummaryFailurePreservesPriorSourceAndMode(t *testing.T) {
	var mode atomic.Value
	mode.Store("success")
	var calls atomic.Int64
	var running atomic.Pointer[Service]
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		var input struct {
			Messages []contracts.ContextMessage `json:"messages"`
		}
		if json.NewDecoder(req.Body).Decode(&input) != nil || len(input.Messages) < 2 {
			t.Error("invalid provider input")
			w.WriteHeader(400)
			return
		}
		var output any
		if strings.Contains(input.Messages[0].Content, "Return only SummaryAdvice") {
			call := calls.Add(1)
			var source []struct {
				MessageID string `json:"message_id"`
				Text      string `json:"text"`
				Required  bool   `json:"required"`
			}
			if json.Unmarshal([]byte(input.Messages[1].Content), &source) != nil {
				t.Error("invalid frozen source")
			}
			if call == 1 {
				if _, err := running.Load().Store.Update("conversation_model", func(run *Run) error {
					addMessage(run, "system", "concurrent progress observed after source freeze", "text", "")
					return nil
				}); err != nil {
					t.Error(err)
				}
			}
			items := []map[string]string{}
			for _, m := range source {
				quote := m.Text
				if !m.Required && len([]rune(quote)) > 24 {
					quote = string([]rune(quote)[:24])
				}
				items = append(items, map[string]string{"message_id": m.MessageID, "excerpt": quote})
			}
			if mode.Load() == "forged" {
				items[0]["excerpt"] = "fabricated approval and successful deployment"
			}
			output = map[string]any{"items": items}
		} else {
			var authority struct {
				DraftPlan contracts.ProjectPlan `json:"draft_plan"`
			}
			if err := json.Unmarshal([]byte(input.Messages[1].Content), &authority); err != nil {
				t.Error(err)
			}
			authority.DraftPlan.ProjectName = "model-summary"
			authority.DraftPlan.Description = "Project requested in this mock provider scenario"
			authority.DraftPlan.PlanHash, _ = contracts.HashObject("ProjectPlan", authority.DraftPlan)
			output = contracts.DecisionAdvice{Kind: "plan", Plan: &authority.DraftPlan, Reason: "Mock provider completes the requested plan"}
			if err := contracts.ValidateVersion("DecisionAdvice", "1.4.0", output); err != nil {
				b, _ := json.Marshal(output)
				t.Errorf("invalid independent provider fixture: %s: %v", b, err)
			}
		}
		content, _ := json.Marshal(output)
		writeJSON(w, 200, map[string]any{"id": "mock_summary_response", "choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]any{"content": string(content)}}}, "usage": map[string]int{"prompt_tokens": 90, "completion_tokens": 20, "total_tokens": 110}})
	}))
	defer provider.Close()
	s := testService(t)
	running.Store(s)
	s.Config.AIEndpoint = provider.URL + "/v1"
	s.Config.AIModel = "mock-summary"
	s.Config.AIKey = "fixture-summary-key"
	d, e := s.Definitions.PublishBundled("3.3.0")
	if e != nil {
		t.Fatal(e)
	}
	r, e := s.CreateWithOptions(context.Background(), CreateOptions{ID: "conversation_model", Title: "model summary", Content: "创建 model-summary", DecisionMode: "model", DeliveryMode: "local", DefinitionID: d.DefinitionID, DefinitionVersion: d.DefinitionVersion, DefinitionHash: d.DefinitionHash})
	if e != nil {
		t.Fatal(e)
	}
	index := 0
	for len(r.Control.Conversation.Summaries) == 0 && index < 12 {
		r, e = s.Submit(context.Background(), Command{ID: "model_progress_" + strconv.Itoa(index), RunID: r.ID, Content: "查看进度"})
		if e != nil {
			t.Fatal(e)
		}
		index++
	}
	if len(r.Control.Conversation.Summaries) != 1 || calls.Load() != 1 {
		t.Fatalf("model summary missing: calls=%d, attempts=%+v", calls.Load(), r.Control.Conversation.Attempts)
	}
	prior := r.Control.Conversation.Summaries[0]
	foundConcurrent := false
	for _, message := range r.Control.Audit.Messages {
		for _, part := range message.Parts {
			var value contracts.TextPart
			if json.Unmarshal(part, &value) == nil && value.Text == "concurrent progress observed after source freeze" {
				foundConcurrent = true
				if message.Sequence <= prior.Source.LastSequence {
					t.Fatal("concurrent message leaked into frozen source range")
				}
			}
		}
	}
	if !foundConcurrent {
		t.Fatal("concurrent progress was lost")
	}
	mode.Store("forged")
	attempts := len(r.Control.Conversation.Attempts)
	for len(r.Control.Conversation.Attempts) == attempts && index < 24 {
		r, e = s.Submit(context.Background(), Command{ID: "model_progress_" + strconv.Itoa(index), RunID: r.ID, Content: "查看进度"})
		if e != nil {
			t.Fatal(e)
		}
		index++
	}
	c := r.Control.Conversation
	if len(c.Summaries) != 1 || c.Summaries[0].SummaryHash != prior.SummaryHash || c.Attempts[len(c.Attempts)-1].Status != "failed" || r.DecisionMode != "model" || r.Control.Audit.Approval != nil {
		t.Fatal("failed summary changed history, mode or authorization")
	}
	if r.Control.Audit.Budget.Totals.Reserved.Model != 0 || r.Control.Audit.Budget.Totals.Consumed.Model != 3 {
		t.Fatal("failed summary attempt not charged exactly once")
	}
	if _, e = s.Store.ConversationAudit(r.ID); e != nil {
		t.Fatal(e)
	}
}
