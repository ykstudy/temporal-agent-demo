package agent

import (
	"fmt"
	"sort"
	"strings"

	"agentstudio/internal/contracts"
	"agentstudio/internal/storage"
)

func optionalString(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

// projectSnapshot is available only for a Run whose provenance was recorded
// from creation. It consumes complete control objects, not the legacy display
// DTOs or reconstructed approval/check evidence.
func projectSnapshot(r Run) (contracts.RunSnapshot, error) {
	c := conversationState(r)
	if c == nil || r.FrozenDefinition == nil {
		return contracts.RunSnapshot{}, fail("legacy_contract_incomplete", "Run was not recorded with full message and history provenance")
	}
	a := r.Control.Audit
	p := contracts.RunSnapshot{ContractVersion: r.FrozenDefinition.ContractVersion, SchemaVersion: 2, RunID: r.ID, Title: r.Title, State: r.State, SourceVersion: int64(r.SourceVersion), StateSequence: int64(r.StateSequence), Modes: runModes(r), DefinitionSnapshot: *r.FrozenDefinition, Plan: a.Plan, Proposal: a.Proposal, ProposalHistory: a.ProposalHistory, Approval: a.Approval, PermissionSnapshot: a.PermissionSnapshot, Messages: a.Messages, SummaryRecords: c.Summaries, SummaryAttempts: c.Attempts, Steps: []contracts.Step{}, RequiredChecks: []contracts.RequiredCheck{}, Artifacts: []contracts.ArtifactEntry{}, Manifest: a.Manifest, Operations: a.Operations, History: c.History, ActiveStep: optionalString(r.ActiveStep), PauseRequested: r.PauseRequested, CancelRequested: r.CancelRequested, ResumeTo: optionalString(r.ResumeTo), RetryOf: optionalString(r.RetryOf), CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt, VerificationReport: a.VerificationReport, Usage: contracts.Usage{Commands: a.Budget.Totals.Consumed.Command + a.Budget.Totals.Reserved.Command, ModelAttempts: a.Budget.Totals.Consumed.Model + a.Budget.Totals.Reserved.Model, ModelFailures: int64(r.Usage.ModelFailures), StepAttempts: []contracts.StepAttempt{}}}
	if a.Plan != nil {
		p.PlanHash = &a.Plan.PlanHash
	}
	if a.Manifest != nil {
		p.Artifacts = a.Manifest.Files
	}
	if a.Approval != nil && r.Control.Schedule.Workspace.WorkspaceID != "" && r.Control.Schedule.Workspace.StorageKey == controlWorkspaceKey(r) {
		workspace := r.Control.Schedule.Workspace
		p.Workspace = &workspace
	}
	if a.VerificationReport != nil {
		if a.VerificationReport.SourceVersion != int64(r.SourceVersion) || a.VerificationReport.DefinitionSnapshotHash != r.FrozenDefinition.SnapshotHash {
			return p, fail("verification_source_mismatch", "current report is from a different source")
		}
		p.RequiredChecks = a.VerificationReport.Checks
	} else {
		for _, check := range r.FrozenDefinition.Definition.CompletionPolicy.RequiredChecks {
			if chainContains(check.DeliveryModes, r.DeliveryMode) {
				p.RequiredChecks = append(p.RequiredChecks, contracts.RequiredCheck{CheckID: check.CheckID, RunID: r.ID, SourceVersion: int64(r.SourceVersion), DefinitionSnapshotHash: r.FrozenDefinition.SnapshotHash, Status: "not_ready", Detail: "本来源尚未形成该项完整验证报告", EvidenceRefs: []contracts.ObjectReference{}})
			}
		}
	}
	operations := map[string]contracts.OperationRecord{}
	for _, op := range a.Operations {
		operations[op.OperationID] = op
	}
	for _, base := range initialSteps() {
		step := contracts.Step{StepID: base.ID, Title: base.Title, State: "pending", Detail: "等待受控执行", SourceVersion: int64(r.SourceVersion), ExecutionRefs: []contracts.ObjectReference{}}
		expected := 1
		if base.ID == "skills" && a.Plan != nil {
			expected = len(a.Plan.SelectedSkills)
		}
		if base.ID == "repo" {
			expected = 2
		}
		if r.DeliveryMode == "local" && (base.ID == "repo" || base.ID == "ci") {
			expected = 0
		}
		completed := 0
		for _, x := range a.Executions {
			if x.SourceVersion != int64(r.SourceVersion) || a.Approval == nil || x.Approval.ApprovalID != a.Approval.ApprovalID {
				continue
			}
			kind, _, e := scheduleAction(x.Action)
			if e != nil {
				return p, e
			}
			if controlLegacyStep(kind) != base.ID {
				continue
			}
			ref, e := storage.Reference(r.ID, x.SourceVersion, "ExecutionRequest", x)
			if e != nil {
				return p, e
			}
			step.ExecutionRefs = append(step.ExecutionRefs, ref)
			op := operations[x.OperationID]
			switch op.Status {
			case "verified":
				completed++
			case "sent":
				step.State = "running"
				step.Detail = "原操作执行中"
			case "unknown":
				step.State = "failed"
				step.Detail = "原操作结果未知，需按原身份核实"
			case "rejected":
				step.State = "failed"
				step.Detail = "原操作未通过，等待明确修复"
			}
		}
		if expected == 0 {
			step.State = "skipped"
			step.Detail = "当前方案不需要此步骤"
		} else if completed == expected {
			step.State = "completed"
			step.Detail = "全部所需原操作已有验证事实"
		} else if completed > 0 && step.State == "pending" {
			step.Detail = fmt.Sprintf("已完成 %d/%d 个受控动作", completed, expected)
		}
		p.Steps = append(p.Steps, step)
	}
	released := map[string]bool{}
	for _, settlement := range a.Budget.Settlements {
		if settlement.Outcome == "released" {
			released[settlement.ReservationID] = true
		}
	}
	attempts := map[string]int64{}
	for _, reservation := range a.Budget.Reservations {
		if reservation.Kind == "action" && !released[reservation.ReservationID] {
			kind := strings.Split(reservation.ScopeID, ":")[0]
			attempts[controlLegacyStep(kind)]++
		}
	}
	keys := make([]string, 0, len(attempts))
	for key := range attempts {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		p.Usage.StepAttempts = append(p.Usage.StepAttempts, contracts.StepAttempt{StepID: key, Attempts: attempts[key]})
	}
	if r.Error != "" && r.State != "DONE" && r.State != "CANCELLED" {
		code := "operation_failed"
		category := "business"
		for i := len(c.ReceiptHistory) - 1; i >= 0; i-- {
			receipt := c.ReceiptHistory[i]
			if receipt.Error != nil && (r.Error == receipt.Error.Message || strings.HasPrefix(r.Error, receipt.Error.Code+":")) {
				p.Error = receipt.Error
				break
			}
		}
		if p.Error == nil {
			for _, op := range a.Operations {
				if op.Status == "unknown" || op.Status == "sent" {
					code = "operation_result_unknown"
					category = "unknown_result"
					break
				}
			}
			p.Error = &contracts.Failure{Code: code, Category: category, Message: boundedText(r.Error), EvidenceRefs: []contracts.ObjectReference{}}
		}
	}
	var err error
	p.SnapshotHash, err = contracts.HashObject("RunSnapshot", p)
	if err != nil {
		return p, err
	}
	return p, contracts.Validate("RunSnapshot", p)
}

func bindReceiptSnapshots(r *Run, ref contracts.ObjectReference) {
	c := conversationState(*r)
	if c == nil {
		return
	}
	c.LatestSnapshot = &ref
	for id, receipt := range c.Receipts {
		if receipt.RunRef == nil {
			bound := contracts.CommandReceiptRunRef(ref)
			receipt.RunRef = &bound
			c.Receipts[id] = receipt
		}
	}
	for i := range c.ReceiptHistory {
		if c.ReceiptHistory[i].RunRef == nil {
			bound := contracts.CommandReceiptRunRef(ref)
			c.ReceiptHistory[i].RunRef = &bound
		}
	}
}
