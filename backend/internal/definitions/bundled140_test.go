package definitions

import (
	"testing"
	"time"

	"agentstudio/internal/contracts"
)

func TestBundled330FreezesSummaryProtocolAndPreservesPriorRelease(t *testing.T) {
	r, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	old, err := r.PublishBundled("3.2.0")
	if err != nil {
		t.Fatal(err)
	}
	oldSnap, err := r.Freeze(old.DefinitionID, old.DefinitionVersion, old.DefinitionHash, nil, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	d, err := r.PublishBundled("3.3.0")
	if err != nil {
		t.Fatal(err)
	}
	snap, err := r.Freeze(d.DefinitionID, d.DefinitionVersion, d.DefinitionHash, nil, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if snap.ContractVersion != "1.4.0" || snap.Definition.ContextPolicy.SummaryPrompt.Content == "" || contracts.BytesHash([]byte(snap.Definition.ContextPolicy.SummaryPrompt.Content)) != snap.Definition.ContextPolicy.SummaryPrompt.SHA256 {
		t.Fatal("summary protocol not frozen")
	}
	if err = r.VerifySnapshot(oldSnap); err != nil {
		t.Fatal("prior release changed", err)
	}
	snap.Definition.ContextPolicy.SummaryPrompt.Content = "new instruction"
	if err = r.VerifySnapshot(snap); err == nil {
		t.Fatal("changed frozen summary prompt accepted")
	}
}
