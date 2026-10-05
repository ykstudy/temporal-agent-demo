package agent

import (
	"agentstudio/internal/contracts"
	"fmt"
)

// A failed real tool produces a complete report rather than a success-shaped
// counter. Unperformed checks remain not_ready; no external validator is read.
func recordRuntimeVerification(r *Run, request contracts.ExecutionRequest, reservation contracts.BudgetReservation, result ControlToolsResult) error {
	if runtimeState(*r) == nil || r.Control.Audit.Manifest == nil {
		return nil
	}
	id := "verification_" + contracts.BytesHash([]byte(reservation.AttemptID))[:40]
	for _, old := range r.Control.Audit.VerificationReports {
		if old.ReportID == id {
			return fail("verification_report_conflict", "原尝试已具有正式验证报告")
		}
	}
	a := r.Control.Audit
	stamp := now()
	report := contracts.VerificationReport{ContractVersion: r.FrozenDefinition.ContractVersion, ReportID: id, ReportVersion: 1, RunID: r.ID, SourceVersion: int64(r.SourceVersion), DefinitionSnapshotHash: r.FrozenDefinition.SnapshotHash, ProposalSnapshotHash: request.Approval.ProposalSnapshotHash, ManifestRef: controlManifestRef(*a.Manifest), Checks: []contracts.RequiredCheck{}, Outcome: "not_ready", CreatedAt: stamp}
	commands := map[string]string{"check.go_mod_tidy": "go.mod_tidy", "check.go_test": "go.test", "check.go_build": "go.build"}
	failed := false
	for _, definition := range r.FrozenDefinition.Definition.CompletionPolicy.RequiredChecks {
		if !chainContains(definition.DeliveryModes, r.DeliveryMode) {
			continue
		}
		check := contracts.RequiredCheck{CheckID: definition.CheckID, RunID: r.ID, SourceVersion: int64(r.SourceVersion), DefinitionSnapshotHash: r.FrozenDefinition.SnapshotHash, ProposalSnapshotHash: &request.Approval.ProposalSnapshotHash, Status: "not_ready", Detail: "本次工具验证不代替后续完整交付检查", EvidenceRefs: []contracts.ObjectReference{}}
		if command, ok := commands[definition.CheckID]; ok {
			for _, receipt := range result.Receipts {
				if receipt.CommandID != command {
					continue
				}
				check.StartedAt = &receipt.StartedAt
				check.CompletedAt = &receipt.CompletedAt
				check.EvidenceRefs = runtimeObject(*r, "ToolExecutionReceipt", receipt.SourceVersion, receipt)
				switch receipt.Status {
				case "failed", "timed_out":
					check.Status = "failed"
					check.Detail = "原受限工具尝试明确失败或超时；保留原回执"
					failed = true
				case "succeeded":
					if !result.ReapprovalRequired && receipt.OutputManifestRef != nil && equalContractValue(receipt.InputManifestRef, receipt.OutputManifestRef) {
						check.Status = "passed"
						check.Detail = "原工具在准确批准内容上执行通过"
					} else {
						check.Detail = "工具返回了候选内容；需新方案批准后重新验证"
					}
				default:
					check.Detail = "原工具结果未知或中断；须核实，不能推断为失败或成功"
				}
			}
		}
		report.Checks = append(report.Checks, check)
	}
	if failed {
		report.Outcome = "failed"
	}
	var err error
	report.ReportHash, err = contracts.HashObject("VerificationReport", report)
	if err == nil {
		err = contracts.Validate("VerificationReport", report)
	}
	if err != nil {
		return fmt.Errorf("verification attempt report: %w", err)
	}
	r.Control.Audit.VerificationReport = &report
	r.Control.Audit.VerificationReports = append(r.Control.Audit.VerificationReports, report)
	return nil
}
