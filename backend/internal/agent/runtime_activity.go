package agent

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"agentstudio/internal/contracts"
	"go.temporal.io/sdk/activity"
)

type runtimeMonitor struct {
	mu         sync.Mutex
	instance   string
	worker     bool
	stopping   bool
	auditFault bool
	beats      map[string]string
}

func (s *Service) RuntimeIdentity() string { return s.runtime.instance }
func (s *Service) SetWorkerRunning(running bool) {
	s.runtime.mu.Lock()
	s.runtime.worker = running
	s.runtime.mu.Unlock()
}
func (s *Service) BeginShutdown() {
	s.runtime.mu.Lock()
	s.runtime.stopping = true
	s.runtime.worker = false
	s.runtime.mu.Unlock()
}
func (s *Service) acceptingWork() error {
	s.runtime.mu.Lock()
	defer s.runtime.mu.Unlock()
	if s.runtime.stopping {
		return fail("service_stopping", "服务正在停止，请查询原命令结果")
	}
	if s.runtime.auditFault {
		return fail("runtime_audit_unavailable", "审计写入失败，修复存储并重启后继续")
	}
	return nil
}
func (s *Service) runtimeFault() {
	s.runtime.mu.Lock()
	s.runtime.auditFault = true
	s.runtime.mu.Unlock()
}

func (s *Service) startRuntimeActivity(ctx context.Context, id, kind, step, command string) (func(error) error, error) {
	noop := func(error) error { return nil }
	r, err := s.Store.Get(id)
	if err != nil {
		return noop, err
	}
	if runtimeState(r) == nil {
		return noop, nil
	}
	// Direct unit calls with a temporal-configured Service are not actual SDK
	// dispatch. Never fabricate a Temporal execution/attempt from configuration.
	if r.ExecutionMode == "temporal" && !activity.IsActivity(ctx) {
		return noop, nil
	}
	a := contracts.RuntimeActivityAttempt{RunID: id, Mode: r.ExecutionMode, ActivityID: newID("local_activity_"), ActivityType: kind, Step: step, CommandID: optionalString(command), Attempt: 1, StartedAt: now(), Status: "running", HeartbeatSource: "process", HeartbeatStatus: "fresh"}
	if activity.IsActivity(ctx) {
		info := activity.GetInfo(ctx)
		a.ActivityID = info.ActivityID
		a.Attempt = int64(info.Attempt)
		a.WorkflowID = optionalString(info.WorkflowExecution.ID)
		a.ExecutionRunID = optionalString(info.WorkflowExecution.RunID)
	}
	a.AttemptID = "activity_" + digest(id + "/" + stringPointer(a.ExecutionRunID) + "/" + a.ActivityID + fmt.Sprint("/", a.Attempt))[:40]
	a.LastHeartbeatAt = &a.StartedAt
	_, err = s.Store.Update(id, func(run *Run) error {
		rt := runtimeState(*run)
		if len(rt.Attempts) >= 4096 {
			return fail("runtime_attempt_capacity", "活动尝试已达发布上限")
		}
		for _, old := range rt.Attempts {
			if old.AttemptID == a.AttemptID {
				return fail("runtime_attempt_conflict", "实际调度身份已登记")
			}
		}
		rt.Attempts = append(rt.Attempts, a)
		return runtimeEvent(run, contracts.RuntimeAuditEvent{Kind: "activity_started", Step: step, Outcome: "running", Correlation: runtimeActiveCorrelation(*run, contracts.RuntimeCorrelation{CommandID: a.CommandID})})
	})
	if err != nil {
		s.runtimeFault()
		return noop, err
	}
	done := make(chan struct{})
	stopped := make(chan struct{})
	beat := func() {
		stamp := now()
		s.runtime.mu.Lock()
		s.runtime.beats[a.AttemptID] = stamp
		s.runtime.mu.Unlock()
		heartbeat(ctx, step)
	}
	beat()
	go func() {
		defer close(stopped)
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ctx.Done():
				return
			case <-ticker.C:
				beat()
			}
		}
	}()
	return func(workErr error) error {
		close(done)
		<-stopped
		s.runtime.mu.Lock()
		last := s.runtime.beats[a.AttemptID]
		delete(s.runtime.beats, a.AttemptID)
		s.runtime.mu.Unlock()
		_, err := s.Store.Update(id, func(run *Run) error {
			rt := runtimeState(*run)
			for i := range rt.Attempts {
				item := &rt.Attempts[i]
				if item.AttemptID != a.AttemptID {
					continue
				}
				if item.Status != "running" {
					return fail("runtime_attempt_conflict", "实际尝试结果已登记")
				}
				end := now()
				item.CompletedAt = &end
				item.ElapsedMS = runtimeElapsed(item.StartedAt, end)
				item.LastHeartbeatAt = optionalString(last)
				item.HeartbeatStatus = "not_applicable"
				item.Status = "succeeded"
				if workErr != nil {
					item.Status = "failed"
					code := "activity_failed"
					var api *APIError
					if errors.As(workErr, &api) {
						code = api.Code
					} else if errors.Is(workErr, context.DeadlineExceeded) {
						code = "activity_deadline"
					} else if errors.Is(workErr, context.Canceled) {
						code = "activity_interrupted"
					}
					item.ErrorCode = &code
				}
				co := contracts.RuntimeCorrelation{CommandID: item.CommandID, WorkflowID: item.WorkflowID, ExecutionRunID: item.ExecutionRunID, ActivityID: &item.ActivityID, Attempt: &item.Attempt, AttemptID: &item.AttemptID}
				return runtimeEvent(run, contracts.RuntimeAuditEvent{Kind: "activity_finished", Step: step, Outcome: item.Status, ElapsedMS: item.ElapsedMS, Correlation: co})
			}
			return fail("runtime_attempt_missing", "实际尝试来源缺失")
		})
		if err != nil {
			s.runtimeFault()
		}
		return err
	}, nil
}
func stringPointer(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

func (s *Service) ProcessCommand(ctx context.Context, cmd Command) (result CommandResult, err error) {
	if !safeID.MatchString(cmd.ID) || !safeID.MatchString(cmd.RunID) {
		return result, fail("invalid_identity", "操作与Run需要稳定合法身份")
	}
	if s.containsSecret(cmd.Content) {
		return result, fail("secret_input", "消息包含服务端凭据，拒绝持久化")
	}
	if !activity.IsActivity(ctx) {
		if r, e := s.Store.Get(cmd.RunID); e == nil {
			if _, ok := r.Processed[cmd.ID]; ok {
				return s.processCommand(ctx, cmd)
			}
		}
	}
	finish, e := s.startRuntimeActivity(ctx, cmd.RunID, "ProcessCommand", "command", cmd.ID)
	if e != nil {
		return result, e
	}
	defer func() {
		endErr := finish(err)
		if endErr != nil {
			err = errors.Join(err, endErr)
		}
		if result.Run.ID == cmd.RunID {
			if current, e := s.Store.Get(cmd.RunID); e == nil {
				result.Run = current
			}
		}
	}()
	return s.processCommand(ctx, cmd)
}
func (s *Service) ExecuteStep(ctx context.Context, input StepInput) (result StepResult, err error) {
	finish, e := s.startRuntimeActivity(ctx, input.RunID, "ExecuteStep", input.Step, "")
	if e != nil {
		return result, e
	}
	defer func() {
		endErr := finish(err)
		if endErr != nil {
			err = errors.Join(err, endErr)
		}
		if current, e := s.Store.Get(input.RunID); e == nil {
			result.State = current.State
			result.SourceVersion = current.SourceVersion
			result.StateSequence = current.StateSequence
		}
	}()
	return s.executeStep(ctx, input)
}
func (s *Service) recoverRuntimeRecords() error {
	for _, r := range s.Store.List() {
		rt := runtimeState(r)
		if rt == nil {
			continue
		}
		pending := false
		for _, a := range rt.Attempts {
			if a.Status == "running" {
				pending = true
			}
		}
		if !pending {
			continue
		}
		_, err := s.Store.Update(r.ID, func(run *Run) error {
			for i := range runtimeState(*run).Attempts {
				a := &runtimeState(*run).Attempts[i]
				if a.Status != "running" {
					continue
				}
				end, code := now(), "activity_interrupted"
				a.Status = "unknown"
				a.CompletedAt = &end
				a.ElapsedMS = runtimeElapsed(a.StartedAt, end)
				a.ErrorCode = &code
				a.HeartbeatStatus = "unavailable"
				a.HeartbeatSource = "unavailable"
				if e := runtimeEvent(run, contracts.RuntimeAuditEvent{Kind: "recovery", Step: a.Step, Outcome: "unknown", Correlation: contracts.RuntimeCorrelation{CommandID: a.CommandID, WorkflowID: a.WorkflowID, ExecutionRunID: a.ExecutionRunID, ActivityID: &a.ActivityID, Attempt: &a.Attempt, AttemptID: &a.AttemptID}, ElapsedMS: a.ElapsedMS}); e != nil {
					return e
				}
			}
			return nil
		})
		if err != nil {
			return err
		}
	}
	return nil
}
