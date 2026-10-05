package agent

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/converter"
	"go.temporal.io/sdk/testsuite"
	"go.temporal.io/sdk/workflow"
)

func TestTemporalQueryUpdateAndContinueAsNew(t *testing.T) {
	s := testService(t)
	r := planRun(t, s, "workflow", "创建 durable-service")
	var suite testsuite.WorkflowTestSuite
	env := suite.NewTestWorkflowEnvironment()
	env.RegisterActivityWithOptions(s.TemporalProcessCommand, activity.RegisterOptions{Name: "ProcessCommand"})
	env.RegisterActivityWithOptions(s.TemporalExecuteStep, activity.RegisterOptions{Name: "ExecuteStep"})
	env.RegisterActivityWithOptions(s.RecordExecutionFailure, activity.RegisterOptions{Name: "RecordExecutionFailure"})
	completed := 0
	env.RegisterDelayedCallback(func() {
		encoded, e := env.QueryWorkflow("get_status")
		if e != nil {
			t.Errorf("query registration/serialization: %v", e)
			return
		}
		var state map[string]any
		if e = encoded.Get(&state); e != nil || state["state"] != "PLAN" {
			t.Errorf("continued query did not retain state: %v %+v", e, state)
		}
	}, 500*time.Millisecond)
	for i := 0; i < 20; i++ {
		index := i
		env.RegisterDelayedCallback(func() {
			cmd := Command{ID: fmt.Sprintf("wf_message_%d", index), RunID: r.ID, Content: "查询进度"}
			env.UpdateWorkflow("submit", cmd.ID, &testsuite.TestUpdateCallback{OnAccept: func() {}, OnReject: func(e error) { t.Errorf("update rejected: %v", e) }, OnComplete: func(value any, e error) {
				if e != nil {
					t.Errorf("update failed: %v", e)
				} else {
					completed++
				}
			}}, cmd)
		}, time.Duration(i+1)*time.Second)
	}
	env.ExecuteWorkflow(ProjectWorkflow, WorkflowInput{RunID: r.ID, State: "PLAN"})
	if completed != 20 {
		t.Fatalf("completed %d updates", completed)
	}
	var continued *workflow.ContinueAsNewError
	if !errors.As(env.GetWorkflowError(), &continued) {
		t.Fatalf("did not Continue-As-New after completed updates: %v", env.GetWorkflowError())
	}
	snapshot, e := s.Store.Get(r.ID)
	if e != nil || snapshot.State != "PLAN" || len(snapshot.Messages) < 40 {
		t.Fatal("Continue-As-New lost durable business state")
	}
}

func TestTemporalRetryKeepsOldTerminalState(t *testing.T) {
	s := testService(t)
	r := planRun(t, s, "wf_retry", "创建 retry-service")
	if _, e := s.ProcessCommand(context.Background(), Command{ID: "cancel_original", RunID: r.ID, Action: "cancel"}); e != nil {
		t.Fatal(e)
	}
	var suite testsuite.WorkflowTestSuite
	env := suite.NewTestWorkflowEnvironment()
	env.RegisterActivityWithOptions(s.TemporalProcessCommand, activity.RegisterOptions{Name: "ProcessCommand"})
	env.RegisterDelayedCallback(func() {
		env.UpdateWorkflow("submit", "retry_command", &testsuite.TestUpdateCallback{OnAccept: func() {}, OnReject: func(e error) { t.Error(e) }, OnComplete: func(_ any, e error) {
			if e != nil {
				t.Error(e)
			}
		}}, Command{ID: "retry_command", RunID: r.ID, Action: "retry"})
	}, time.Second)
	env.ExecuteWorkflow(ProjectWorkflow, WorkflowInput{RunID: r.ID, State: "FAILED", CommandsProcessed: 19})
	var continued *workflow.ContinueAsNewError
	if !errors.As(env.GetWorkflowError(), &continued) {
		t.Fatalf("expected continued terminal workflow: %v", env.GetWorkflowError())
	}
	var input WorkflowInput
	if e := converter.GetDefaultDataConverter().FromPayloads(continued.Input, &input); e != nil {
		t.Fatal(e)
	}
	if input.State != "FAILED" {
		t.Fatalf("old Run adopted successor state: %s", input.State)
	}
	successor, e := s.Store.Get(retryRunID(r.ID, "retry_command"))
	if e != nil || successor.State != "PLAN" || successor.Proposal == nil {
		t.Fatalf("retry successor not initialized before workflow start: %+v %v", successor, e)
	}
}
