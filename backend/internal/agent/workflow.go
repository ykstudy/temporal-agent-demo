package agent

import (
	"context"
	"errors"
	"time"

	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/worker"
	"go.temporal.io/sdk/workflow"
)

func workflowID(runID string) string { return "agentstudio/" + runID }

type WorkflowInput struct {
	CheckpointAt      *string `json:"checkpoint_at,omitempty"`
	RunID             string  `json:"run_id"`
	CommandsProcessed int     `json:"commands_processed"`
	State             string  `json:"state,omitempty"`
	SourceVersion     int     `json:"source_version,omitempty"`
	StateSequence     int     `json:"state_sequence,omitempty"`
}
type workflowRequest struct {
	Command Command
	Reply   workflow.Channel
}

// The workflow contains no filesystem, network, native goroutine or wall-clock
// operations. Every side effect is an Activity shared with local execution.
func ProjectWorkflow(ctx workflow.Context, input WorkflowInput) error {
	runtimeVersion := workflow.GetVersion(ctx, "runtime-checkpoint-v1", workflow.DefaultVersion, 1)
	checkpointAt := input.CheckpointAt
	queue := workflow.NewBufferedChannel(ctx, 128)
	status := "INTAKE"
	if input.State != "" {
		status = input.State
	}
	processed := input.CommandsProcessed
	sourceVersion, stateSequence := input.SourceVersion, input.StateSequence
	if e := workflow.SetQueryHandler(ctx, "get_status", func() (map[string]any, error) {
		return map[string]any{"run_id": input.RunID, "state": status, "source_version": sourceVersion, "state_sequence": stateSequence, "commands_processed": processed, "checkpoint_at": checkpointAt, "execution_run_id": workflow.GetInfo(ctx).WorkflowExecution.RunID}, nil
	}); e != nil {
		return e
	}
	if e := workflow.SetUpdateHandler(ctx, "submit", func(updateCtx workflow.Context, cmd Command) (CommandResult, error) {
		if cmd.RunID != input.RunID || cmd.ID == "" {
			return CommandResult{}, temporal.NewNonRetryableApplicationError("command identity mismatch", "invalid_identity", nil)
		}
		if queue.Len() >= 128 {
			return CommandResult{}, temporal.NewNonRetryableApplicationError("命令队列已满，请稍后用原ID重试", "queue_full", nil)
		}
		reply := workflow.NewChannel(updateCtx)
		queue.Send(updateCtx, workflowRequest{Command: cmd, Reply: reply})
		var response struct {
			Result CommandResult
			Error  string
			Code   string
		}
		reply.Receive(updateCtx, &response)
		if response.Error != "" {
			return CommandResult{}, temporal.NewNonRetryableApplicationError(response.Error, response.Code, nil)
		}
		return response.Result, nil
	}); e != nil {
		return e
	}
	options := workflow.ActivityOptions{StartToCloseTimeout: 12 * time.Minute, HeartbeatTimeout: 90 * time.Second, RetryPolicy: &temporal.RetryPolicy{InitialInterval: time.Second, BackoffCoefficient: 2, MaximumInterval: 15 * time.Second, MaximumAttempts: 3, NonRetryableErrorTypes: []string{"invalid_input", "stale_proposal", "terminal_run", "invalid_state", "idempotency_conflict", "proposal_closed", "invalid_action", "invalid_message", "invalid_project_name", "unapproved_execution"}}}
	activityCtx := workflow.WithActivityOptions(ctx, options)
	process := func(request workflowRequest) (bool, error) {
		var result CommandResult
		err := workflow.ExecuteActivity(activityCtx, "ProcessCommand", request.Command).Get(activityCtx, &result)
		response := struct {
			Result CommandResult
			Error  string
			Code   string
		}{Result: result}
		if err != nil {
			response.Error = err.Error()
			response.Code = "operation_failed"
			var app *temporal.ApplicationError
			_ = errors.As(err, &app)
			if app != nil {
				response.Code = app.Type()
				response.Error = app.Message()
			}
		}
		request.Reply.Send(ctx, response)
		processed++
		if err != nil {
			return false, nil
		}
		if result.Run.ID == input.RunID {
			status = result.Run.State
			sourceVersion = result.Run.SourceVersion
			stateSequence = result.Run.StateSequence
			if runtimeVersion != workflow.DefaultVersion {
				stamp := workflow.Now(ctx).UTC().Format(time.RFC3339Nano)
				checkpointAt = &stamp
			}
		}
		return result.Execute, nil
	}
	for {
		var request workflowRequest
		queue.Receive(ctx, &request)
		execute, err := process(request)
		if err != nil {
			return err
		}
		if execute {
			for _, step := range executionSteps {
				// Updates are serialized at safe boundaries. A pause never claims
				// to undo an Activity that has already produced a side effect.
				var pending workflowRequest
				for queue.ReceiveAsync(&pending) {
					if _, e := process(pending); e != nil {
						return e
					}
				}
				var result StepResult
				if status == "PAUSED" || status == "PLAN" || status == "BLOCKED" || terminal(status) {
					break
				}
				if runtimeVersion == workflow.DefaultVersion {
					if step == "verify" {
						status = "VERIFY"
					} else if step == "repo" || step == "ci" || step == "deliver" {
						status = "DELIVER"
					} else {
						status = "IMPLEMENT"
					}
				}
				if err = workflow.ExecuteActivity(activityCtx, "ExecuteStep", StepInput{RunID: input.RunID, Step: step}).Get(activityCtx, &result); err != nil {
					_ = workflow.ExecuteActivity(activityCtx, "RecordExecutionFailure", input.RunID, err.Error()).Get(activityCtx, nil)
					status = "BLOCKED"
					break
				}
				status = result.State
				sourceVersion = result.SourceVersion
				stateSequence = result.StateSequence
				if runtimeVersion != workflow.DefaultVersion {
					stamp := workflow.Now(ctx).UTC().Format(time.RFC3339Nano)
					checkpointAt = &stamp
				}
				if !result.Continue {
					break
				}
			}
		}
		if (workflow.GetInfo(ctx).GetContinueAsNewSuggested() || processed >= 20) && queue.Len() == 0 {
			if err := workflow.Await(ctx, func() bool { return workflow.AllHandlersFinished(ctx) }); err != nil {
				return err
			}
			if queue.Len() == 0 {
				return workflow.NewContinueAsNewError(ctx, ProjectWorkflow, WorkflowInput{RunID: input.RunID, CommandsProcessed: 0, CheckpointAt: checkpointAt, State: status, SourceVersion: sourceVersion, StateSequence: stateSequence})
			}
		}
		// Keep the workflow available for a terminal Run's retry command.
	}
}
func (s *Service) TemporalProcessCommand(ctx context.Context, cmd Command) (CommandResult, error) {
	result, e := s.ProcessCommand(ctx, cmd)
	if e != nil {
		if api, ok := e.(*APIError); ok {
			return result, temporal.NewNonRetryableApplicationError(api.Message, api.Code, nil)
		}
		return result, e
	}
	// The durable history receives a small receipt, never proposal files or raw
	// transcripts. HTTP retrieves the authoritative persisted Run by this ID.
	result.Run = Run{ID: result.Run.ID, State: result.Run.State, SourceVersion: result.Run.SourceVersion, Version: result.Run.SourceVersion, StateSequence: result.Run.StateSequence}
	return result, nil
}
func (s *Service) RecordExecutionFailure(ctx context.Context, id, reason string) error {
	_, e := s.Store.Update(id, func(r *Run) error {
		if terminal(r.State) || r.State == "PAUSED" {
			return nil
		}
		// A controlled executor may already have recorded a precise recovery
		// stage. An outer Activity error must not replace it with BLOCKED.
		if r.State != "BLOCKED" {
			r.ResumeTo = r.State
		}
		r.ActiveStep = ""
		r.State = "BLOCKED"
		label := "本地阶段"
		if s.Config.Mode == "temporal" {
			label = "Temporal Activity"
		}
		r.Error = label + "重试耗尽或中断，副作用须核实后恢复：" + s.redact(reason)
		history(r, "activity_interrupted", r.Error)
		return nil
	})
	return e
}

func (s *Service) TemporalExecuteStep(ctx context.Context, input StepInput) (StepResult, error) {
	result, err := s.ExecuteStep(ctx, input)
	var api *APIError
	if errors.As(err, &api) {
		// A known control-plane refusal is a completed business result. SDK
		// retry must not obtain another model suggestion and erase that refusal.
		return result, temporal.NewNonRetryableApplicationError(api.Message, api.Code, err)
	}
	return result, err
}

func (s *Service) RegisterWorker(w worker.Worker) {
	w.RegisterWorkflow(ProjectWorkflow)
	w.RegisterActivityWithOptions(s.TemporalProcessCommand, activity.RegisterOptions{Name: "ProcessCommand"})
	w.RegisterActivityWithOptions(s.TemporalExecuteStep, activity.RegisterOptions{Name: "ExecuteStep"})
	w.RegisterActivityWithOptions(s.RecordExecutionFailure, activity.RegisterOptions{Name: "RecordExecutionFailure"})
}
