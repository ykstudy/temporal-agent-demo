package agent

import (
	"context"
	"testing"
)

func TestControlRuntimeOuterFailurePreservesRecoveryStageAndTerminalFacts(t *testing.T) {
	s := testService(t)
	r := controlFixture(t, s, "runtime_failure")
	planned, err := s.ProcessCommand(context.Background(), Command{ID: "runtime_plan", RunID: r.ID, Content: "创建 runtime-service"})
	if err != nil {
		t.Fatal(err)
	}
	r = controlAcceptCommand(t, s, planned.Run)
	// Inject lifecycle checkpoints with complete plan/approval authority. This
	// tests only the outer failure recorder, not evidence for reaching DONE.
	for _, state := range []string{"VERIFY", "BLOCKED", "PAUSED", "CANCELLED", "FAILED", "DONE"} {
		_, err := s.Store.Update(r.ID, func(run *Run) error {
			run.State = state
			run.ResumeTo = "VERIFY"
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := s.RecordExecutionFailure(context.Background(), r.ID, "controlled executor already reported failure"); err != nil {
			t.Fatal(err)
		}
		current, err := s.Store.Get(r.ID)
		if err != nil || current.ResumeTo != "VERIFY" || current.SourceVersion != r.SourceVersion {
			t.Fatalf("outer failure lost original recovery source: %v %+v", err, current)
		}
		if state == "VERIFY" || state == "BLOCKED" {
			if current.State != "BLOCKED" {
				t.Fatal("execution error did not remain blocked")
			}
		} else if current.State != state {
			t.Fatal("outer error overwrote terminal or paused fact")
		}
	}
}
