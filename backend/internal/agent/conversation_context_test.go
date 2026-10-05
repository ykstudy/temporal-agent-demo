package agent

import (
	"context"
	"strings"
	"testing"
	"time"

	"agentstudio/internal/conversation"
)

func TestConversationContextTrimsOlderTurnsButPreservesCurrentInput(t *testing.T) {
	s := testService(t)
	d, e := s.Definitions.PublishBundled("3.3.0")
	if e != nil {
		t.Fatal(e)
	}
	r, e := s.CreateWithOptions(context.Background(), CreateOptions{ID: "context_revision", Content: "创建 context-project，SQLite + GORM、JWT 认证和 JSON 日志", DecisionMode: "rules", DeliveryMode: "local", DefinitionID: d.DefinitionID, DefinitionVersion: d.DefinitionVersion, DefinitionHash: d.DefinitionHash})
	if e != nil {
		t.Fatal(e)
	}
	first := r.Control.Audit.Proposal.SnapshotHash
	r, e = s.Submit(context.Background(), Command{ID: "actual_revision", RunID: r.ID, Content: "改用 session 认证和 text 日志"})
	if e != nil {
		t.Fatal(e)
	}
	if r.SourceVersion != 3 || r.Control.Audit.Proposal.SnapshotHash == first {
		t.Fatal("three-skill second turn did not revise actual plan")
	}
	record := r.Control.Audit.Contexts[len(r.Control.Audit.Contexts)-1]
	var kept bool
	for _, m := range record.Messages {
		if m.Role == "user" && m.Content == "改用 session 认证和 text 日志" {
			kept = true
		}
	}
	if !kept {
		t.Fatal("current instruction was trimmed")
	}
	if _, e = canonicalModelRequest(r, record.Messages); e != nil {
		t.Fatal(e)
	}
	if len(record.InputMessageRefs) >= len(r.Control.Audit.Messages)-1 {
		t.Fatal("older history was not bounded")
	}
	raw, e := s.Store.ConversationTranscript(r.ID, "", "")
	if e != nil || !strings.Contains(string(raw), "SQLite") {
		t.Fatal("context trim removed original source", e)
	}
	// The current input alone cannot fit: refusal must retain it and must never
	// call or silently reduce the frozen provider/output budget.
	large := strings.Repeat("完整中文输入", 500)
	r, e = s.Store.Update(r.ID, func(run *Run) error {
		_, err := appendCanonicalInput(run, Command{ID: "large_current", RunID: r.ID, Content: large}, time.Now().UTC())
		return err
	})
	if e != nil {
		t.Fatal(e)
	}
	_, _, e = canonicalContextWithAuthority(r, map[string]any{"run_id": r.ID})
	if e == nil {
		t.Fatal("oversized current input silently truncated")
	}
	if conversation.Text(r.Control.Audit.Messages[len(r.Control.Audit.Messages)-1]) != large {
		t.Fatal("original message mutated")
	}
}
