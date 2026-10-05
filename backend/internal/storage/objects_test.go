package storage

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"agentstudio/internal/contracts"
)

func TestObjectsRejectOversizedStoredBodyBeforeDecoding(t *testing.T) {
	s, err := OpenObjects(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	m := message(t, "bounded original")
	ref, err := s.Put(m.RunID, 1, "MessageEnvelope", m)
	if err != nil {
		t.Fatal(err)
	}
	path, err := s.objectPath(m.RunID, ref, false)
	if err != nil {
		t.Fatal(err)
	}
	file, err := os.OpenFile(path, os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if err = file.Truncate(contracts.MaxJSONBytes + 1); err != nil {
		file.Close()
		t.Fatal(err)
	}
	if err = file.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Get(m.RunID, ref); err == nil || !strings.Contains(err.Error(), "size bound") {
		t.Fatal("oversized archive did not hit the bounded reader", err)
	}
}

func message(t *testing.T, content string) contracts.MessageEnvelope {
	t.Helper()
	part, err := json.Marshal(contracts.TextPart{PartID: "part_current", PartVersion: 1, Type: "text", FallbackText: content, Text: content, Format: "plain"})
	if err != nil {
		t.Fatal(err)
	}
	return contracts.MessageEnvelope{SchemaVersion: 2, ContractVersion: contracts.ContractVersion, RunID: "run_objects", MessageID: "message_current", Sequence: 1, SourceVersion: 1, StateSequence: 1, Role: "user", CreatedAt: "2026-10-02T12:00:00Z", Modes: contracts.Modes{ExecutionMode: "local", DecisionMode: "rules", DeliveryMode: "local"}, Parts: []contracts.Part{contracts.Part(part)}}
}

func TestObjectsPersistImmutableBodiesAndRejectIdentityReuse(t *testing.T) {
	root := t.TempDir()
	s, err := OpenObjects(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	m := message(t, "original request")
	ref, err := s.Put(m.RunID, 1, "MessageEnvelope", m)
	if err != nil {
		t.Fatal(err)
	}
	again, err := s.Put(m.RunID, 1, "MessageEnvelope", m)
	if err != nil || again != ref {
		t.Fatalf("idempotent registration: %v", err)
	}
	body, err := s.Get(m.RunID, ref)
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenObjects(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	restored, err := reopened.Get(m.RunID, ref)
	if err != nil || !bytes.Equal(body, restored) {
		t.Fatalf("restart changed object: %v", err)
	}
	changed := message(t, "replacement request")
	if _, err = s.Put(m.RunID, 1, "MessageEnvelope", changed); !errors.Is(err, ErrObjectConflict) {
		t.Fatalf("same ID/version accepted new hash: %v", err)
	}
	changed = m
	changed.SourceVersion = 2
	if _, err = s.Put(m.RunID, 2, "MessageEnvelope", changed); !errors.Is(err, ErrObjectConflict) {
		t.Fatalf("same ID/version accepted new source: %v", err)
	}
	if _, err = s.Get("other_run", ref); !errors.Is(err, ErrMissingObject) {
		t.Fatalf("cross-Run read: %v", err)
	}
	if _, err = s.Put("other_run", 1, "MessageEnvelope", m); err == nil {
		t.Fatal("cross-Run registration")
	}
	file, err := s.objectPath(m.RunID, ref, false)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(file, []byte(`{"broken":true}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Get(m.RunID, ref); err == nil {
		t.Fatal("corrupt object accepted")
	}
}

func TestObjectsRejectAliasesInvalidShapeAndSymlinks(t *testing.T) {
	s, err := OpenObjects(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	m := message(t, "bounded message")
	data, _ := json.Marshal(m)
	var unknown map[string]any
	_ = json.Unmarshal(data, &unknown)
	unknown["approved"] = true
	if _, err = s.Put(m.RunID, 1, "MessageEnvelope", unknown); err == nil {
		t.Fatal("incomplete Schema validation")
	}
	ref, err := s.Put(m.RunID, 1, "MessageEnvelope", m)
	if err != nil {
		t.Fatal(err)
	}
	wrong := ref
	wrong.ObjectVersion = 2
	if _, err = s.Get(m.RunID, wrong); err == nil {
		t.Fatal("alias reference resolved")
	}
	file, err := s.objectPath(m.RunID, ref, false)
	if err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "payload.json")
	if err = os.WriteFile(outside, data, 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.Remove(file); err != nil {
		t.Fatal(err)
	}
	if err = os.Symlink(outside, file); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Get(m.RunID, ref); err == nil {
		t.Fatal("object body followed symlink")
	}
	if _, err = s.Put(m.RunID, 1, "MessageEnvelope", m); err == nil {
		t.Fatal("object body replaced symlink")
	}
}
