package agent

import (
	"context"
	"errors"
	"time"

	"agentstudio/internal/contracts"
	"agentstudio/internal/governance"
)

// ExecuteControlVerification runs only an already persisted VERIFY request. A
// candidate diff is returned for the explicit revision workflow, never applied
// to the approved workspace. A successful action still does not decide DONE.
func (s *Service) ExecuteControlVerification(ctx context.Context, request contracts.ExecutionRequest) (ControlToolsResult, error) {
	kind, _, err := scheduleAction(request.Action)
	if err != nil || kind != "verify" {
		return ControlToolsResult{}, fail("invalid_action", "验证执行器仅接受完整verify请求")
	}
	r, reservation, err := s.beginControlExecution(request.RunID, request)
	if err != nil {
		return ControlToolsResult{}, err
	}
	commands, err := governance.MergePostCommands(*r.FrozenDefinition, *r.Control.Audit.Plan)
	if err != nil {
		return ControlToolsResult{}, s.finishControlVerification(r.ID, request, reservation, ControlToolsResult{}, err)
	}
	deadline, err := time.Parse(time.RFC3339Nano, *reservation.DeadlineAt)
	if err != nil {
		return ControlToolsResult{}, s.finishControlVerification(r.ID, request, reservation, ControlToolsResult{}, err)
	}
	bounded, cancel := context.WithDeadline(ctx, deadline)
	result, executionErr := s.ExecuteControlTools(bounded, r, request, commands, reservation.AttemptID)
	cancel()
	return result, s.finishControlVerification(r.ID, request, reservation, result, executionErr)
}

func (s *Service) finishControlVerification(id string, request contracts.ExecutionRequest, reservation contracts.BudgetReservation, result ControlToolsResult, executionErr error) error {
	var receiptErr error
	_, saveErr := s.Store.Update(id, func(run *Run) error {
		if run.Control == nil || run.ActiveStep != request.ExecutionID || run.SourceVersion != int(request.SourceVersion) {
			return fail("execution_binding_mismatch", "验证结束时原请求身份已改变，不能写入成功事实")
		}
		outcome, status := "succeeded", "verified"
		if executionErr != nil {
			outcome, status = "failed", "rejected"
		}
		if result.ReapprovalRequired {
			outcome, status = "failed", "rejected"
		}
		for _, receipt := range result.Receipts {
			if receipt.Status == "unknown" || receipt.Status == "interrupted" {
				outcome, status = "unknown", "unknown"
			}
		}
		var api *APIError
		if errors.As(executionErr, &api) && api.Code == "tool_attempt_unknown" {
			outcome, status = "unknown", "unknown"
		}
		refs := []contracts.ObjectReference{}
		seen := map[string]bool{}
		for _, receipt := range result.Receipts {
			registered := false
			for _, stored := range run.Control.Audit.ToolReceipts {
				if stored.ReceiptID == receipt.ReceiptID && equalContractValue(stored, receipt) && stored.ExecutionID == request.ExecutionID && stored.RequestHash == request.RequestHash {
					registered = true
					break
				}
			}
			if !registered || seen[receipt.ReceiptID] {
				receiptErr = fail("tool_receipt_mismatch", "返回工具回执未准确登记，不能证明已完成事实")
				continue
			}
			seen[receipt.ReceiptID] = true
			refs = append(refs, contracts.ObjectReference{ObjectType: "evidence", ObjectID: receipt.ReceiptID, ObjectVersion: 1, SourceVersion: receipt.SourceVersion, Hash: receipt.ReceiptHash})
		}
		if outcome == "succeeded" {
			commands, e := governance.MergePostCommands(*run.FrozenDefinition, *run.Control.Audit.Plan)
			if e != nil {
				receiptErr = e
			} else if len(result.Receipts) != len(commands) {
				receiptErr = fail("tool_receipt_mismatch", "成功验证缺少完整固定工具回执")
			} else {
				for i, receipt := range result.Receipts {
					if receipt.CommandID != commands[i].CommandID || receipt.Status != "succeeded" || receipt.ExecutionID != request.ExecutionID || receipt.RequestHash != request.RequestHash || receipt.OutputManifestRef == nil || !equalContractValue(receipt.InputManifestRef, receipt.OutputManifestRef) {
						receiptErr = fail("tool_receipt_mismatch", "成功验证的命令或批准manifest不一致")
					}
				}
			}
		}
		if receiptErr != nil {
			outcome, status = "unknown", "unknown"
		}
		completed := time.Now().UTC()
		var candidate *contracts.VerificationCandidate
		if result.ReapprovalRequired && contracts.DeliveryContract(request.ContractVersion) && receiptErr == nil && outcome == "failed" {
			value, e := s.buildControlCandidate(*run, request, reservation, result, completed)
			if e == nil {
				trial := clone(*run)
				e = settleControlInRun(&trial, reservation, "failed", completed)
				if e == nil {
					_, _, _, e = controlCandidateEvidence(trial, value)
				}
			}
			if e != nil {
				receiptErr, outcome, status = e, "unknown", "unknown"
			} else {
				candidate = &value
			}
		}
		if e := settleControlInRun(run, reservation, outcome, completed); e != nil {
			return e
		}
		found := false
		for i := range run.Control.Audit.Operations {
			op := &run.Control.Audit.Operations[i]
			if op.OperationID != request.OperationID {
				continue
			}
			if op.RequestHash != request.RequestHash || op.Status != "sent" {
				return fail("operation_conflict", "验证原操作身份或状态改变")
			}
			op.Status = status
			op.UpdatedAt = completed.Format(time.RFC3339Nano)
			op.EvidenceRefs = append(op.EvidenceRefs, refs...)
			found = true
		}
		if !found {
			return fail("operation_conflict", "原验证操作缺失")
		}
		if candidate != nil {
			run.Control.Audit.VerificationCandidates = append(run.Control.Audit.VerificationCandidates, *candidate)
		}
		run.ActiveStep = ""
		if executionErr != nil || receiptErr != nil || result.ReapprovalRequired {
			run.ResumeTo = "VERIFY"
			run.State = "BLOCKED"
			run.Error = "验证未通过；保留原工具回执与候选，需按原请求核实或修订"
		}
		if run.CancelRequested {
			run.State = "CANCELLED"
			run.Error = "用户取消；保留验证事实和未批准候选"
			run.CancelRequested = false
			run.PauseRequested = false
		} else if run.PauseRequested {
			if executionErr == nil && receiptErr == nil && !result.ReapprovalRequired {
				run.ResumeTo = run.State
				run.State = "PAUSED"
			}
			run.PauseRequested = false
		}
		if receiptErr == nil {
			return recordRuntimeVerification(run, request, reservation, result)
		}
		return nil
	})
	return errors.Join(executionErr, receiptErr, saveErr)
}
