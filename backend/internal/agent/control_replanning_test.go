package agent

import (
	"context"
	"os"
	"testing"
	"time"

	"agentstudio/internal/contracts"
)

func controlReviseCommand(r Run, id string) Command {
	p := r.Control.Audit.Proposal
	return Command{ID: id, RunID: r.ID, Action: "revise", ProposalID: p.ProposalID, ProposalVersion: int(p.ProposalVersion), SourceVersion: int(p.SourceVersion), SnapshotHash: p.SnapshotHash}
}

func TestControlKnownFailureParameterReplanPreservesHistory(t *testing.T) {
	s, r, x := recoveryFailedVerificationVersion(t, "1.3.0")
	workspace, source, deadline := r.Workspace, r.SourceVersion, *r.Control.Audit.Budget.ExecutionDeadlineAt
	manifest, oldApproval := *r.Control.Audit.Manifest, r.Control.Audit.ApprovalHistory[len(r.Control.Audit.ApprovalHistory)-1]
	cmd := controlReviseCommand(r, "known_failure_revise")
	result, err := s.ProcessCommand(context.Background(), cmd)
	if err != nil || result.Execute || result.Run.State != "CLARIFY" || result.Run.Control.Audit.Approval != nil || result.Run.AcceptedProposal != "" {
		t.Fatal("known failure could not close approval", err)
	}
	if _, err = s.PrepareControlAction(context.Background(), r.ID); err == nil {
		t.Fatal("closed old approval authorized another action")
	}
	again, err := s.ProcessCommand(context.Background(), cmd)
	if err != nil || again.Run.State != "CLARIFY" {
		t.Fatal("revise replay", err)
	}
	result, err = s.ProcessCommand(context.Background(), Command{ID: "changed_parameters", RunID: r.ID, Content: "改用 JSON 日志"})
	if err != nil {
		t.Fatal("replan parameters", err)
	}
	updated := result.Run
	if updated.State != "PLAN" || updated.SourceVersion != source+1 || updated.Control.Audit.Approval != nil || updated.Control.Audit.Proposal.SnapshotHash == r.Control.Audit.Proposal.SnapshotHash || updated.Workspace != "" || updated.Control.Audit.Manifest != nil || *updated.Control.Audit.Budget.ExecutionDeadlineAt != deadline {
		t.Fatal("parameter revision changed incorrect boundary")
	}
	if _, err = os.Stat(workspace); err != nil {
		t.Fatal("old workspace lost", err)
	}
	if !chainEqual(updated.Control.Audit.ManifestHistory[len(updated.Control.Audit.ManifestHistory)-1], manifest) || !chainEqual(updated.Control.Audit.ApprovalHistory[len(updated.Control.Audit.ApprovalHistory)-1], oldApproval) {
		t.Fatal("replan rewrote immutable old facts")
	}
	accepted, err := s.ProcessCommand(context.Background(), approvalCommand(updated, "accept_replanned"))
	if err != nil {
		t.Fatal(err)
	}
	updated = accepted.Run
	next, err := s.PrepareControlAction(context.Background(), updated.ID)
	if err != nil || next.Execution == nil || next.Execution.SourceVersion != int64(source+1) || next.Execution.OperationID == x.OperationID || next.Execution.Workspace.StorageKey == x.Workspace.StorageKey {
		t.Fatal("new approval inherited old execution", err)
	}
}

func TestControlExpiryReproposalKeepsSourceAndRejectsOldApproval(t *testing.T) {
	s := testService(t)
	r := newControlCompletionRun(t, s, "rules")
	// Shorten only this in-process clock fixture; Config.New still enforces
	// the public 1m lower bound. No archived timestamp/hash is rewritten.
	s.Config.ApprovalTTL = 10 * time.Second
	planned, err := s.ProcessCommand(context.Background(), Command{ID: "expiry_plan", RunID: r.ID, Content: "创建 expiry-service"})
	if err != nil {
		t.Fatal(err)
	}
	r = controlAcceptCommand(t, s, planned.Run)
	expiry, err := time.Parse(time.RFC3339Nano, r.Control.Audit.Approval.ExpiresAt)
	if err != nil {
		t.Fatal(err)
	}
	if delay := time.Until(expiry); delay > 0 {
		time.Sleep(delay + 10*time.Millisecond)
	}
	if _, err = s.PrepareControlAction(context.Background(), r.ID); err == nil {
		t.Fatal("expired first execution authorized")
	}
	cmd := controlReviseCommand(r, "expired_reproposal")
	result, err := s.ProcessCommand(context.Background(), cmd)
	if err != nil {
		t.Fatal("explicit expiry reissue", err)
	}
	newRun := result.Run
	if result.Execute || newRun.State != "PLAN" || newRun.SourceVersion != r.SourceVersion || newRun.Control.Audit.Proposal.ProposalVersion != r.Control.Audit.Proposal.ProposalVersion+1 || newRun.Control.Audit.Proposal.IssuedAt == r.Control.Audit.Proposal.IssuedAt || newRun.Control.Audit.Approval != nil || len(newRun.Control.Audit.ApprovalHistory) != 1 {
		t.Fatal("expiry changed source or inherited approval")
	}
	if _, err = s.ProcessCommand(context.Background(), approvalCommand(r, "old_expired_accept")); err == nil {
		t.Fatal("old approval accepted against new version")
	}
	before := len(newRun.Control.Audit.ProposalHistory)
	again, err := s.ProcessCommand(context.Background(), cmd)
	if err != nil || len(again.Run.Control.Audit.ProposalHistory) != before {
		t.Fatal("expiry reissue replay created another proposal", err)
	}
	if err = contracts.Validate("S2AuditBundle", again.Run.Control.Audit); err != nil {
		t.Fatal(err)
	}
}
