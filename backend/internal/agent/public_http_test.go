package agent

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"agentstudio/internal/contracts"
)

func TestV2FullRequestsHistoricalSnapshotsAndReceipts(t *testing.T) {
	s := testService(t)
	mux := http.NewServeMux()
	s.Register(mux)
	call := func(method, path string, body any, status int) []byte {
		t.Helper()
		var b []byte
		switch v := body.(type) {
		case string:
			b = []byte(v)
		case nil:
		default:
			b, _ = json.Marshal(body)
		}
		req := httptest.NewRequest(method, path, bytes.NewReader(b))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != status {
			t.Fatalf("%s %s: %d %s", method, path, w.Code, w.Body.String())
		}
		return w.Body.Bytes()
	}
	d, e := s.Definitions.PublishBundled("3.3.0")
	if e != nil {
		t.Fatal(e)
	}
	input := contracts.CreateRunRequest{ContractVersion: "1.4.0", SchemaVersion: 2, CommandID: "create_public", RunID: "public_run", Title: "v2 workspace", Modes: contracts.Modes{ExecutionMode: "local", DecisionMode: "rules", DeliveryMode: "local"}, DefinitionSelection: contracts.DefinitionSelection{DefinitionID: d.DefinitionID, DefinitionVersion: d.DefinitionVersion, DefinitionHash: d.DefinitionHash}, InitialContent: "创建 public-project"}
	raw := call("POST", "/api/v2/runs", input, 201)
	var receipt contracts.CommandReceipt
	if e = contracts.Decode("CommandReceipt", raw, &receipt); e != nil {
		t.Fatal(e)
	}
	encoded, _ := json.Marshal(input)
	fingerprint, e := contracts.HashValue(json.RawMessage(encoded))
	if e != nil || receipt.CommandFingerprint != fingerprint || receipt.CommandID != input.CommandID || receipt.RunRef == nil {
		t.Fatal("wire creation receipt is not exact", e)
	}
	if again := call("POST", "/api/v2/runs", input, 201); !bytes.Equal(raw, again) {
		t.Fatal("replay changed original result")
	}
	input.Title = "conflicting title"
	call("POST", "/api/v2/runs", input, 409)
	get := func() contracts.RunSnapshot {
		var v contracts.RunSnapshot
		b := call("GET", "/api/v2/runs/public_run", nil, 200)
		if e = contracts.Decode("RunSnapshot", b, &v); e != nil {
			t.Fatal(e)
		}
		if !hashMatches("RunSnapshot", v, v.SnapshotHash) {
			t.Fatal("invalid snapshot hash")
		}
		return v
	}
	initial := get()
	ref := contracts.MessageRequestTarget{ObjectType: "run", ObjectID: initial.RunID, ObjectVersion: initial.StateSequence, SourceVersion: initial.SourceVersion, Hash: initial.SnapshotHash}
	message := contracts.MessageRequest{ContractVersion: "1.4.0", SchemaVersion: 2, CommandID: "public_progress", Target: ref, Content: "查看进度"}
	progress := call("POST", "/api/v2/runs/public_run/messages", message, 200)
	after := get()
	if after.SourceVersion != initial.SourceVersion || after.StateSequence <= initial.StateSequence {
		t.Fatal("source and progress conflated")
	}
	if replay := call("POST", "/api/v2/runs/public_run/messages", message, 200); !bytes.Equal(replay, progress) {
		t.Fatal("message replay changed result")
	}
	if current := get(); current.StateSequence != after.StateSequence {
		t.Fatal("successful replay mutated the Run")
	}
	message.Target.ObjectVersion++
	call("POST", "/api/v2/runs/public_run/messages", message, 409)
	message.CommandID = "bad_reference"
	call("POST", "/api/v2/runs/public_run/messages", message, 409)
	// Complete exact reference, including old snapshots, remains independently readable.
	path := "/api/v2/runs/public_run/objects/run/public_run?object_version=" + strconv.FormatInt(ref.ObjectVersion, 10) + "&source_version=" + strconv.FormatInt(ref.SourceVersion, 10) + "&hash=" + ref.Hash
	old := call("GET", path, nil, 200)
	var oldRun contracts.RunSnapshot
	json.Unmarshal(old, &oldRun)
	if oldRun.SnapshotHash != initial.SnapshotHash {
		t.Fatal("historical Run replaced")
	}
	call("GET", path+"&hash="+ref.Hash, nil, 400)
	call("GET", strings.Replace(path, "/objects/run/public_run", "/objects/run/foreign_run", 1), nil, 404)
	call("POST", "/api/v2/runs/public_run/messages", `{"contract_version":"1.4.0","schema_version":true,"command_id":"x","target":{},"content":"x"}`, 400)
	call("POST", "/api/v2/runs/public_run/messages", `{"content":"x","content":"y"}`, 400)
	call("GET", "/api/v2/runs/public_run?unknown=1", nil, 400)
	message.Target = ref
	message.CommandID = "revise_public"
	message.Content = "创建 revision-project"
	call("POST", "/api/v2/runs/public_run/messages", message, 200)
	revised := get()
	if revised.SourceVersion <= initial.SourceVersion {
		t.Fatal("revision not applied")
	}
	p := initial.Proposal
	action := contracts.ControlRequest{CommandID: "stale_accept", Action: "accept", Target: contracts.MessageObjectRef(ref), RequiredProposalRef: &contracts.ActionIntentRequiredProposalRef{ObjectType: "proposal", ObjectID: p.ProposalID, ObjectVersion: p.ProposalVersion, SourceVersion: p.SourceVersion, Hash: p.SnapshotHash}}
	call("POST", "/api/v2/runs/public_run/actions", action, 409)
	action = contracts.ControlRequest{CommandID: "cancel_public", Action: "cancel", Target: contracts.MessageObjectRef{ObjectType: "run", ObjectID: revised.RunID, ObjectVersion: revised.StateSequence, SourceVersion: revised.SourceVersion, Hash: revised.SnapshotHash}}
	call("POST", "/api/v2/runs/public_run/actions", action, 200)
	cancelled := get()
	if cancelled.State != "CANCELLED" {
		t.Fatal("cancel not authoritative")
	}
	action.CommandID = "retry_public"
	action.Action = "retry"
	action.Target.ObjectVersion = cancelled.StateSequence
	action.Target.Hash = cancelled.SnapshotHash
	b := call("POST", "/api/v2/runs/public_run/actions", action, 200)
	if e = contracts.Decode("CommandReceipt", b, &receipt); e != nil {
		t.Fatal(e)
	}
	if receipt.RunRef == nil || receipt.RunRef.ObjectID != "public_run" || len(receipt.ResultRefs) != 1 || receipt.ResultRefs[0].ObjectID == "public_run" {
		t.Fatal("new identity result not explicitly bound")
	}
	next, e := s.Store.Get(receipt.ResultRefs[0].ObjectID)
	if e != nil || next.RetryOf != "public_run" || next.State != "PLAN" || next.Control.Audit.Approval != nil {
		t.Fatal("redo inherited authority", e)
	}
	legacy, e := s.Create(context.Background(), "legacy_public", "legacy", "创建 legacy-project")
	if e != nil {
		t.Fatal(e)
	}
	before, e := s.Store.Get(legacy.ID)
	if e != nil {
		t.Fatal(e)
	}
	errBody := call("GET", "/api/v2/runs/legacy_public", nil, 409)
	if contracts.ValidateJSONVersion("ErrorResponse", "1.4.0", errBody) != nil || !bytes.Contains(errBody, []byte("legacy_read_url")) {
		t.Fatal("legacy error incomplete")
	}
	afterLegacy, _ := s.Store.Get(legacy.ID)
	if !equalContractValue(before, afterLegacy) {
		t.Fatal("read migrated historical facts")
	}
	call("GET", "/api/v2/runs", nil, 200)
	data := s.Config.DataDir
	s.Store.Close()
	reopened, e := NewStore(data)
	if e != nil {
		t.Fatal(e)
	}
	defer reopened.Close()
	replay, e := reopened.PublicReceipt("public_run", "create_public")
	if e != nil {
		t.Fatal(e)
	}
	b, _ = json.Marshal(replay)
	if !bytes.Equal(bytes.TrimSpace(raw), b) {
		t.Fatal("restart lost original receipt")
	}
}

func TestV2SnapshotStreamExactReferenceAndCursor(t *testing.T) {
	s := testService(t)
	r := createConversationRun(t, s, "stream_public")
	mux := http.NewServeMux()
	s.Register(mux)
	server := httptest.NewServer(mux)
	defer server.Close()
	req, _ := http.NewRequest("GET", server.URL+"/api/v2/runs/"+r.ID+"/events", nil)
	response, e := http.DefaultClient.Do(req)
	if e != nil {
		t.Fatal(e)
	}
	scanner := bufio.NewScanner(response.Body)
	var event contracts.SnapshotEvent
	var eventID string
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "id: ") {
			eventID = strings.TrimPrefix(line, "id: ")
		}
		if strings.HasPrefix(line, "data: ") {
			if e = contracts.Decode("SnapshotEvent", []byte(strings.TrimPrefix(line, "data: ")), &event); e != nil {
				t.Fatal(e)
			}
			break
		}
	}
	response.Body.Close()
	if eventID != event.EventID || event.RunID != r.ID || event.StateSequence != int64(r.StateSequence) || event.RunRef.Hash != r.Control.Conversation.LatestSnapshot.Hash {
		t.Fatal("stream not exact committed reference")
	}
	query := url.Values{"object_version": {strconv.FormatInt(event.RunRef.ObjectVersion, 10)}, "source_version": {strconv.FormatInt(event.RunRef.SourceVersion, 10)}, "hash": {event.RunRef.Hash}}
	response, e = http.Get(server.URL + "/api/v2/runs/" + r.ID + "/objects/run/" + r.ID + "?" + query.Encode())
	if e != nil {
		t.Fatal(e)
	}
	b, _ := io.ReadAll(response.Body)
	response.Body.Close()
	if response.StatusCode != 200 || contracts.ValidateJSON("RunSnapshot", b) != nil {
		t.Fatal("event historical object unreadable")
	}
	for _, cursor := range []string{"foreign_1", snapshotEventID(r.ID, r.StateSequence+1), snapshotEventID("other_run", 1)} {
		req, _ = http.NewRequest("GET", server.URL+"/api/v2/runs/"+r.ID+"/events", nil)
		req.Header.Set("Last-Event-ID", cursor)
		response, e = http.DefaultClient.Do(req)
		if e != nil {
			t.Fatal(e)
		}
		response.Body.Close()
		if response.StatusCode != 409 {
			t.Fatal("invalid cursor accepted", cursor)
		}
	}
}
