package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"agentstudio/internal/contracts"
	"agentstudio/internal/storage"
)

func createConversationRun(t *testing.T, s *Service, id string) Run {
	t.Helper()
	d, err := s.Definitions.PublishBundled("3.3.0")
	if err != nil {
		t.Fatal(err)
	}
	r, err := s.CreateWithOptions(context.Background(), CreateOptions{ID: id, Title: "conversation provenance", Content: "创建 conversation-project", DecisionMode: "rules", DeliveryMode: "local", DefinitionID: d.DefinitionID, DefinitionVersion: d.DefinitionVersion, DefinitionHash: d.DefinitionHash})
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestConversationRealCommandsSummaryHistoryAndOriginalBytes(t *testing.T) {
	s := testService(t)
	r := createConversationRun(t, s, "conversation_rules")
	initial, err := s.Store.ConversationTranscript(r.ID, "", "")
	if err != nil {
		t.Fatal(err)
	}
	firstProposal := *r.Control.Audit.Proposal
	for i := 0; i < 20; i++ {
		r, err = s.Submit(context.Background(), Command{ID: "progress_" + strconv.Itoa(i), RunID: r.ID, Content: "查看进度"})
		if err != nil {
			t.Fatal(i, err)
		}
	}
	final, err := s.Store.ConversationTranscript(r.ID, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(final, initial) || r.SourceVersion != int(firstProposal.SourceVersion) || len(r.Control.Conversation.Summaries) < 2 {
		t.Fatalf("original source or summary history missing: %d summaries", len(r.Control.Conversation.Summaries))
	}
	before := len(r.Control.Audit.Messages)
	r, err = s.Submit(context.Background(), Command{ID: "progress_0", RunID: r.ID, Content: "查看进度"})
	if err != nil || len(r.Control.Audit.Messages) != before {
		t.Fatal("replay appended original input", err)
	}
	if _, err = s.Submit(context.Background(), Command{ID: "progress_0", RunID: r.ID, Content: "different"}); err == nil {
		t.Fatal("command fingerprint conflict accepted")
	}
	audit, err := s.Store.ConversationAudit(r.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(audit.CommandReceipts) != 21 {
		t.Fatal("durable receipts missing", len(audit.CommandReceipts))
	}
	for _, receipt := range audit.CommandReceipts {
		if receipt.RunRef == nil || receipt.Status != "processed" {
			t.Fatal("receipt lacks actual result", receipt.CommandID)
		}
		b, e := s.Store.CanonicalObject(r.ID, contracts.ObjectReference(*receipt.RunRef))
		if e != nil {
			t.Fatal(e)
		}
		var snapshot contracts.RunSnapshot
		if e = contracts.Decode("RunSnapshot", b, &snapshot); e != nil {
			t.Fatal(e)
		}
		if snapshot.RunID != r.ID || snapshot.StateSequence != receipt.RunRef.ObjectVersion || snapshot.SnapshotHash != receipt.RunRef.Hash {
			t.Fatal("receipt points to another snapshot")
		}
	}
	ref, err := storage.Reference(r.ID, firstProposal.SourceVersion, "Proposal", firstProposal)
	if err != nil {
		t.Fatal(err)
	}
	body, err := s.Store.CanonicalObject(r.ID, ref)
	if err != nil {
		t.Fatal(err)
	}
	var stored contracts.Proposal
	if err = json.Unmarshal(body, &stored); err != nil || stored.SnapshotHash != firstProposal.SnapshotHash {
		t.Fatal("historical proposal replaced", err)
	}
	data := s.Config.DataDir
	s.Store.Close()
	reopened, err := NewStore(data)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	after, err := reopened.ConversationTranscript(r.ID, "", "")
	if err != nil || !bytes.Equal(after, final) {
		t.Fatal("restart changed original transcript", err)
	}
	if got, e := reopened.ConversationAudit(r.ID); e != nil || !equalContractValue(got.CommandReceipts, audit.CommandReceipts) {
		t.Fatal("restart changed receipt result", e)
	}
}

func TestConversationPartialTransactionRecoveryAndCorruptTail(t *testing.T) {
	s := testService(t)
	r := createConversationRun(t, s, "conversation_recovery")
	r.StateSequence++
	addMessage(&r, "assistant", "durable pending message", "text", "")
	history(&r, "pending_test", "test interrupted after staging original envelope")
	if err := s.Store.prepareCanonical(&r); err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(disk(r))
	if err != nil {
		t.Fatal(err)
	}
	pending := filepath.Join(s.Config.DataDir, "runs", ".pending-"+r.ID+".json")
	if err = os.WriteFile(pending, body, 0600); err != nil {
		t.Fatal(err)
	}
	all, err := storage.TranscriptBytes(r.Control.Audit.Messages)
	if err != nil {
		t.Fatal(err)
	}
	path := s.Store.transcriptPath(r.ID)
	if err = os.WriteFile(path, all[:len(all)-7], 0600); err != nil {
		t.Fatal(err)
	}
	data := s.Config.DataDir
	s.Store.Close()
	reopened, err := NewStore(data)
	if err != nil {
		t.Fatal(err)
	}
	actual, err := reopened.ConversationTranscript(r.ID, "", "")
	if err != nil || !bytes.Equal(actual, all) {
		t.Fatal("pending transaction not completed exactly", err)
	}
	reopened.Close()
	f, _ := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0600)
	f.WriteString("unexplained")
	f.Close()
	if unexpected, err := NewStore(data); err == nil {
		unexpected.Close()
		t.Fatal("unexplained tail silently recovered")
	}
}

func TestConversationOldProposalReferenceRemainsOriginalAfterRevision(t *testing.T) {
	s := testService(t)
	r := createConversationRun(t, s, "conversation_revision")
	old := *r.Control.Audit.Proposal
	before, err := s.Store.ConversationTranscript(r.ID, "", "")
	if err != nil {
		t.Fatal(err)
	}
	r, err = s.Submit(context.Background(), Command{ID: "revision", RunID: r.ID, Content: "创建 revised-project"})
	if err != nil {
		t.Fatal(err)
	}
	if r.Control.Audit.Proposal.ProposalID == old.ProposalID || r.SourceVersion <= int(old.SourceVersion) {
		t.Fatal("revision did not create a new source and proposal")
	}
	after, err := s.Store.ConversationTranscript(r.ID, "", "")
	if err != nil || !bytes.HasPrefix(after, before) {
		t.Fatal("revision rewrote old card", err)
	}
	ref, err := storage.Reference(r.ID, old.SourceVersion, "Proposal", old)
	if err != nil {
		t.Fatal(err)
	}
	body, err := s.Store.CanonicalObject(r.ID, ref)
	if err != nil {
		t.Fatal(err)
	}
	var actual contracts.Proposal
	if err = json.Unmarshal(body, &actual); err != nil || actual.ProposalID != old.ProposalID || actual.SnapshotHash != old.SnapshotHash {
		t.Fatal("old reference returned current proposal", err)
	}
}
