package storage

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"agentstudio/internal/contracts"
)

func TestTranscriptExactPrefixRecoveryAndTamperRejection(t *testing.T) {
	m := contracts.MessageEnvelope{ContractVersion: "1.4.0", SchemaVersion: 2, RunID: "run_a", MessageID: "msg_a", Sequence: 1, SourceVersion: 1, StateSequence: 1, Role: "user", CreatedAt: "2026-10-04T00:00:00Z", Modes: contracts.Modes{ExecutionMode: "local", DecisionMode: "rules", DeliveryMode: "local"}, Parts: []contracts.Part{contracts.Part(`{"part_id":"text","part_version":1,"type":"text","fallback_text":"原文","text":"原文\nsecond line","format":"plain"}`)}}
	messages := []contracts.MessageEnvelope{m}
	expected, err := TranscriptBytes(messages)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "messages.jsonl")
	// Simulate a torn write, then finish using the same durable transaction.
	if err = os.WriteFile(path, expected[:len(expected)-5], 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = ReadTranscript(path, messages); err == nil {
		t.Fatal("torn committed log accepted")
	}
	if err = CommitTranscript(path, messages); err != nil {
		t.Fatal(err)
	}
	if err = CommitTranscript(path, messages); err != nil {
		t.Fatal(err)
	}
	actual, err := ReadTranscript(path, messages)
	if err != nil || !bytes.Equal(actual, expected) {
		t.Fatal("exact source not preserved", err)
	}
	changed := bytes.Replace(expected, []byte("second"), []byte("forged"), 1)
	os.WriteFile(path, changed, 0600)
	if err = CommitTranscript(path, messages); err == nil {
		t.Fatal("conflicting transcript rewritten")
	}
	os.WriteFile(path, append(expected, []byte("extra")...), 0600)
	if _, err = ReadTranscript(path, messages); err == nil {
		t.Fatal("unexplained tail accepted")
	}
	other := filepath.Join(t.TempDir(), "outside.jsonl")
	os.WriteFile(other, expected, 0600)
	os.Remove(path)
	if err = os.Symlink(other, path); err != nil {
		t.Fatal(err)
	}
	if _, err = ReadTranscript(path, messages); err == nil {
		t.Fatal("symlink transcript read")
	}
	if err = CommitTranscript(path, messages); err == nil {
		t.Fatal("symlink transcript append")
	}
}
