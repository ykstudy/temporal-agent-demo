package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// Workspace and Go prerequisites are explicitly synthetic here. Actual HTTP,
// complete remote receipts, Store restart and next-action checks are exercised.
func TestControlGitLab13CreateReceiptAndRestart(t *testing.T) {
	mock := &controlGitLabMock{}
	s, request, id := controlGitLabFixtureVersion(t, mock, "1.3.0")
	r, err := s.ExecuteControlGitLab(context.Background(), id, request)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Control.Audit.RemoteReceipts) != 1 || r.Control.Audit.RemoteReceipts[0].Status != "succeeded" || !r.Control.Audit.RemoteReceipts[0].WriteSent || len(r.Control.Audit.RemoteReceipts[0].Observations) != 3 {
		t.Fatal("complete create receipt missing")
	}
	assertControlCreateResult(t, r, request, "succeeded")
	assertControlCreateReceiptNegatives(t, r, request)
	next, err := NextControlAction(r, r.Control.Schedule)
	if err != nil || next.Action == nil {
		t.Fatal("complete create cannot schedule approved push", err)
	}
	kind, _, _ := scheduleAction(*next.Action)
	if kind != "push_repository" {
		t.Fatal("create bypassed push", kind)
	}
	beforeGets, beforePosts, beforeBudget := mock.gets, mock.posts, r.Control.Audit.Budget.LedgerVersion
	s.Store.Close()
	s.Store, err = NewStore(s.Config.DataDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Store.Close)
	r, err = s.ExecuteControlGitLab(context.Background(), id, request)
	if err != nil || mock.gets != beforeGets || mock.posts != beforePosts || r.Control.Audit.Budget.LedgerVersion != beforeBudget {
		t.Fatal("restart replay changed original remote effect or charge", err)
	}
	if _, err = s.Store.CanonicalObject(id, controlRemoteReceiptRef(r.Control.Audit.RemoteReceipts[0])); err != nil {
		t.Fatal("full receipt unavailable after restart", err)
	}
}

func TestControlGitLab13ReadSecretNeverBecomesEvidence(t *testing.T) {
	const secret = "controlled-gitlab-test-secret"
	mock := &controlGitLabMock{postMutation: func(project *controlGitLabProject) { project.Description = secret }}
	s, request, id := controlGitLabFixtureVersion(t, mock, "1.3.0")
	r, err := s.ExecuteControlGitLab(context.Background(), id, request)
	if err == nil || r.State != "BLOCKED" || mock.posts != 1 || len(r.Control.Audit.RemoteReceipts) != 1 {
		t.Fatal("untrusted secret-bearing response did not preserve unknown boundary", err)
	}
	q := r.Control.Audit.RemoteReceipts[0]
	if q.Status != "unknown" || q.Project != nil || q.Observations[len(q.Observations)-1].Kind != "unavailable" {
		t.Fatal("secret-bearing response manufactured public facts")
	}
	body, marshalErr := json.Marshal(r)
	if marshalErr != nil || strings.Contains(string(body), secret) || strings.Contains(err.Error(), secret) {
		t.Fatal("secret leaked into durable Run or returned error")
	}
}
