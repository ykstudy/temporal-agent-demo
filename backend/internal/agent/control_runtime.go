package agent

import (
	"context"
	"errors"
)

func controlLegacyStep(kind string) string {
	switch kind {
	case "scaffold":
		return "scaffold"
	case "apply_skill":
		return "skills"
	case "verify":
		return "verify"
	case "create_repository", "push_repository":
		return "repo"
	case "trigger_ci":
		return "ci"
	case "package":
		return "deliver"
	}
	return ""
}

func controlStepResult(r Run, proceed bool) StepResult {
	return StepResult{State: r.State, Continue: proceed && !terminal(r.State) && r.State != "PAUSED" && r.State != "BLOCKED" && r.State != "PLAN", SourceVersion: r.SourceVersion, StateSequence: r.StateSequence}
}

// ExecuteControlStep is shared by local execution and the existing Temporal
// Activity. Step names are scheduling groups only: each effect still requires
// its own complete persisted ExecutionRequest and atomic executor admission.
func (s *Service) ExecuteControlStep(ctx context.Context, input StepInput) (StepResult, error) {
	index := -1
	for i, step := range executionSteps {
		if step == input.Step {
			index = i
		}
	}
	if index < 0 {
		return StepResult{}, fail("invalid_step", "未知调度分组")
	}
	for {
		r, err := s.Store.Get(input.RunID)
		if err != nil {
			return StepResult{}, err
		}
		if r.Control == nil || r.ExecutionMode != s.Config.Mode {
			return controlStepResult(r, false), fail("control_binding_mismatch", "受控调度需要当前模式的完整Run")
		}
		if err = ctx.Err(); err != nil {
			return controlStepResult(r, false), err
		}
		if r.State == "PAUSED" || r.State == "BLOCKED" || terminal(r.State) || r.PauseRequested || r.CancelRequested {
			return controlStepResult(r, false), nil
		}
		next, err := s.PrepareControlAction(ctx, r.ID)
		if err != nil {
			return controlStepResult(r, false), err
		}
		r, err = s.Store.Get(r.ID)
		if err != nil {
			return StepResult{}, err
		}
		if next.ActionsExhausted {
			if input.Step != "deliver" {
				return controlStepResult(r, true), nil
			}
			r, _, err = s.FinalizeControlCompletion(ctx, r.ID)
			return controlStepResult(r, false), err
		}
		if next.Execution == nil {
			return controlStepResult(r, false), fail("execution_evidence_missing", "下一动作缺少完整执行请求或前置证据")
		}
		x := *next.Execution
		kind, _, err := scheduleAction(x.Action)
		if err != nil {
			return controlStepResult(r, false), err
		}
		step := controlLegacyStep(kind)
		if step != input.Step {
			for i, expected := range executionSteps {
				if expected == step && i > index {
					return controlStepResult(r, true), nil
				}
			}
			return controlStepResult(r, false), fail("invalid_step_order", "前置受控动作尚未完成")
		}
		if next.NeedsReconciliation {
			if kind != "create_repository" && kind != "push_repository" && kind != "trigger_ci" {
				return controlStepResult(r, false), fail("reconciliation_required", "原本地尝试缺少明确结果，保留原身份等待核实")
			}
			r, err = s.ReconcileControlGitLab(ctx, r.ID, x)
			if err != nil || r.State == "BLOCKED" || terminal(r.State) {
				return controlStepResult(r, false), err
			}
			continue
		}
		if next.AwaitingEvidence {
			if kind == "verify" {
				r, err = s.RetryControlVerification(ctx, r.ID, x)
				if err == nil {
					continue
				}
				return controlStepResult(r, false), err
			}
			return controlStepResult(r, false), fail("execution_evidence_missing", "原动作尚未具有完整成功证据")
		}
		heartbeat(ctx, kind)
		switch kind {
		case "scaffold", "apply_skill":
			_, _, err = s.ExecuteControlWorkspace(ctx, r.ID, x)
		case "verify":
			result, verifyErr := s.ExecuteControlVerification(ctx, x)
			if result.ReapprovalRequired && verifyErr == nil {
				r, err = s.ProposeControlCandidate(ctx, r.ID)
				return controlStepResult(r, false), err
			}
			if verifyErr != nil {
				r, retryErr := s.RetryControlVerification(ctx, r.ID, x)
				if retryErr == nil {
					continue
				}
				return controlStepResult(r, false), errors.Join(verifyErr, retryErr)
			}
		case "create_repository", "push_repository", "trigger_ci":
			_, err = s.ExecuteControlGitLab(ctx, r.ID, x)
		case "package":
			_, _, err = s.ExecuteControlPackage(ctx, r.ID, x)
		default:
			err = fail("invalid_action", "未登记受控执行器")
		}
		if err != nil {
			current, readErr := s.Store.Get(r.ID)
			return controlStepResult(current, false), errors.Join(err, readErr)
		}
	}
}
