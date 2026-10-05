package conversation

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"agentstudio/internal/contracts"
	"agentstudio/internal/storage"
)

func message(id, text string, seq int64) contracts.MessageEnvelope {
	p, _ := json.Marshal(contracts.TextPart{PartID: "text", PartVersion: 1, Type: "text", Text: text, Format: "plain", FallbackText: text})
	return contracts.MessageEnvelope{ContractVersion: "1.4.0", SchemaVersion: 2, RunID: "run_a", MessageID: id, Sequence: seq, SourceVersion: 1, StateSequence: seq, Role: "user", CreatedAt: "2026-10-04T00:00:00Z", Modes: contracts.Modes{ExecutionMode: "local", DecisionMode: "rules", DeliveryMode: "local"}, Parts: []contracts.Part{p}}
}
func TestSummarySourceAndForgedOrMissingFacts(t *testing.T) {
	m := []contracts.MessageEnvelope{message("first", "create alpha", 1), message("second", "blocked pending approval", 2)}
	source, err := Source(m)
	if err != nil {
		t.Fatal(err)
	}
	if source.FirstSequence != 1 || source.LastSequence != 2 || len(source.MessageIDs) != 2 {
		t.Fatal(source)
	}
	a := Rules(m)
	if _, err = Content(m, a, 2000); err != nil {
		t.Fatal(err)
	}
	a.Items[0].Excerpt = "approved, all checks passed"
	if _, err = Content(m, a, 2000); err == nil {
		t.Fatal("fabricated authority accepted")
	}
	a = Rules(m)
	a.Items = a.Items[:1]
	if _, err = Content(m, a, 2000); err == nil {
		t.Fatal("blocker omitted")
	}
	a = Rules(m)
	a.Items[0].MessageID = "other_run"
	if _, err = Content(m, a, 2000); err == nil {
		t.Fatal("foreign source accepted")
	}
	if _, err = Content(m, Rules(m), 8); err == nil {
		t.Fatal("output budget exceeded")
	}
	m[1].Sequence = 3
	if _, err = Source(m); err == nil {
		t.Fatal("range gap accepted")
	}
}

func TestMessageRejectsDuplicatePartAndMissingHistoricalReference(t *testing.T) {
	m := message("m", "hello", 1)
	if err := ValidateMessage(m, nil); err != nil {
		t.Fatal(err)
	}
	m.Parts = append(m.Parts, m.Parts[0])
	if err := ValidateMessage(m, nil); err == nil {
		t.Fatal("duplicate part")
	}
	m = message("m", "hello", 1)
	m.Parts = append(m.Parts, contracts.Part(`{"part_id":"proposal","part_version":1,"type":"component","fallback_text":"old proposal","component_id":"proposal_1","component_version":1,"component_type":"proposal_diff","component_schema":"urn:agentstudio:component:proposal_diff:1.0.0","payload":{"proposal_ref":{"object_type":"proposal","object_id":"proposal_1","object_version":1,"source_version":1,"hash":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},"summary":"old proposal","changed_paths":[]}}`))
	if err := ValidateMessage(m, func(contracts.ObjectReference) bool { return true }); err != nil {
		t.Fatal(err)
	}
	if err := ValidateMessage(m, func(contracts.ObjectReference) bool { return false }); err == nil {
		t.Fatal("foreign or latest substituted reference accepted")
	}
}

func TestAllFivePartFormatsPreserveOriginalBytesAndRejectForeignReferences(t *testing.T) {
	body, err := os.ReadFile("testdata/five-parts.json")
	if err != nil {
		t.Fatal(err)
	}
	var m contracts.MessageEnvelope
	if err = contracts.DecodeVersion("MessageEnvelope", "1.4.0", body, &m); err != nil {
		t.Fatal(err)
	}
	// This is a clearly synthetic protocol fixture. Real historical proposal
	// resolution is covered separately by the service integration test.
	if err = ValidateMessage(m, func(ref contracts.ObjectReference) bool { return ref.SourceVersion == 1 }); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "original.jsonl")
	if err = storage.CommitTranscript(path, []contracts.MessageEnvelope{m}); err != nil {
		t.Fatal(err)
	}
	if _, err = storage.ReadTranscript(path, []contracts.MessageEnvelope{m}); err != nil {
		t.Fatal(err)
	}
	if err = ValidateMessage(m, func(contracts.ObjectReference) bool { return false }); err == nil {
		t.Fatal("unresolved historical action reference accepted")
	}
}
