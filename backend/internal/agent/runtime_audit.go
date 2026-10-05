package agent

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"agentstudio/internal/contracts"
	"agentstudio/internal/storage"
)

// RuntimeState exists only for Runs instrumented from their creation. Its event
// prefix is immutable and commits in the same pending transaction as the Run.
type RuntimeState struct {
	Events   []contracts.RuntimeAuditEvent      `json:"events"`
	Attempts []contracts.RuntimeActivityAttempt `json:"attempts"`
}

func runtimeState(r Run) *RuntimeState {
	if c := conversationState(r); c != nil {
		return c.Runtime
	}
	return nil
}
func newRuntimeState() *RuntimeState {
	return &RuntimeState{Events: []contracts.RuntimeAuditEvent{}, Attempts: []contracts.RuntimeActivityAttempt{}}
}
func runtimeEvent(r *Run, e contracts.RuntimeAuditEvent) error {
	rt := runtimeState(*r)
	if rt == nil {
		return nil
	}
	if len(rt.Events) >= 32768 {
		return fail("runtime_audit_capacity", "运行审计已达发布上限，请保留历史并新建Run")
	}
	e.RuntimeVersion = "1.0.0"
	e.RunID = r.ID
	e.Sequence = int64(len(rt.Events) + 1)
	e.EventID = fmt.Sprintf("audit_%s_%d", digest(r.ID)[:20], e.Sequence)
	e.SourceVersion = int64(r.SourceVersion)
	e.DefinitionSnapshotHash = r.FrozenDefinition.SnapshotHash
	if e.StateSequence == 0 {
		e.StateSequence = int64(r.StateSequence + 1)
	}
	if e.RecordedAt == "" {
		e.RecordedAt = now()
	}
	if e.Step == "" {
		e.Step = "control"
	}
	if e.Outcome == "" {
		e.Outcome = "recorded"
	}
	if e.Parameters.Action == "" {
		e.Parameters.Action = e.Kind
	}
	if e.Parameters.ArgumentKeys == nil {
		e.Parameters.ArgumentKeys = []string{}
	}
	sort.Strings(e.Parameters.ArgumentKeys)
	var err error
	e.Parameters.SummaryHash, err = contracts.HashValue(map[string]any{"action": e.Parameters.Action, "argument_keys": e.Parameters.ArgumentKeys})
	if err != nil {
		return err
	}
	if e.EvidenceRefs == nil {
		e.EvidenceRefs = []contracts.ObjectRef{}
	}
	if n := len(rt.Events); n > 0 {
		previous := rt.Events[n-1].EventHash
		e.PreviousHash = &previous
	}
	e.EventHash, err = contracts.HashObject("RuntimeAuditEvent", e)
	if err != nil {
		return err
	}
	if err = contracts.ValidateRuntime("RuntimeAuditEvent", e); err != nil {
		return err
	}
	rt.Events = append(rt.Events, e)
	return nil
}
func runtimeActiveCorrelation(r Run, c contracts.RuntimeCorrelation) contracts.RuntimeCorrelation {
	rt := runtimeState(r)
	if rt == nil {
		return c
	}
	for i := len(rt.Attempts) - 1; i >= 0; i-- {
		a := rt.Attempts[i]
		if a.Status != "running" {
			continue
		}
		if c.CommandID != nil && a.CommandID != nil && *c.CommandID != *a.CommandID {
			continue
		}
		c.WorkflowID = a.WorkflowID
		c.ExecutionRunID = a.ExecutionRunID
		c.ActivityID = &a.ActivityID
		c.Attempt = &a.Attempt
		c.AttemptID = &a.AttemptID
		break
	}
	return c
}
func runtimeCommand(r Run, id string) Command {
	if c := conversationState(r); c != nil {
		if w, ok := c.WireCommands[id]; ok {
			return w.Command
		}
	}
	var cmd Command
	_ = json.Unmarshal([]byte(r.CommandIntents[id]), &cmd)
	return cmd
}
func runtimeObject(r Run, schema string, source int64, v any) []contracts.ObjectRef {
	ref, e := storage.Reference(r.ID, source, schema, v)
	if e != nil {
		return []contracts.ObjectRef{}
	}
	return []contracts.ObjectRef{ref}
}
func runtimeElapsed(start, end string) *int64 {
	a, e := time.Parse(time.RFC3339Nano, start)
	b, f := time.Parse(time.RFC3339Nano, end)
	if e != nil || f != nil || b.Before(a) {
		return nil
	}
	n := b.Sub(a).Milliseconds()
	return &n
}

func captureRuntimeDelta(r *Run, old Run) error {
	rt := runtimeState(*r)
	if rt == nil {
		if runtimeState(old) != nil {
			return fail("runtime_history_truncated", "不能删除已采集运行历史")
		}
		return nil
	}
	previous := runtimeState(old)
	if previous != nil {
		if len(rt.Events) < len(previous.Events) || len(rt.Attempts) < len(previous.Attempts) {
			return fail("runtime_history_truncated", "运行历史不可截断")
		}
		for i, a := range previous.Attempts {
			b := rt.Attempts[i]
			if a.AttemptID != b.AttemptID || a.ActivityID != b.ActivityID || a.Attempt != b.Attempt || a.Mode != b.Mode || a.StartedAt != b.StartedAt || a.Step != b.Step || !equalContractValue(a.CommandID, b.CommandID) || !equalContractValue(a.ExecutionRunID, b.ExecutionRunID) || a.CompletedAt != nil && !equalContractValue(a, b) {
				return fail("runtime_attempt_conflict", "实际尝试身份及已登记结果不可修改")
			}
		}
		for i, e := range previous.Events {
			if !equalContractValue(e, rt.Events[i]) {
				return fail("runtime_history_conflict", "已登记审计不可修改")
			}
		}
	}
	emit := func(kind, outcome string, c contracts.RuntimeCorrelation, refs []contracts.ObjectRef, elapsed *int64) error {
		step := r.ActiveStep
		if step == "" {
			step = "control"
		}
		return runtimeEvent(r, contracts.RuntimeAuditEvent{Kind: kind, Outcome: outcome, Step: step, StateSequence: int64(r.StateSequence), RecordedAt: r.UpdatedAt, Correlation: runtimeActiveCorrelation(*r, c), EvidenceRefs: refs, ElapsedMS: elapsed})
	}
	if previous == nil && len(rt.Events) == 0 {
		if e := emit("run_created", r.State, contracts.RuntimeCorrelation{}, nil, nil); e != nil {
			return e
		}
	}
	c := conversationState(*r)
	var before ConversationState
	if b := conversationState(old); b != nil {
		before = *b
	}
	if len(c.ReceiptHistory) < len(before.ReceiptHistory) {
		return fail("runtime_history_truncated", "命令回执历史不可截断")
	}
	for _, receipt := range c.ReceiptHistory[len(before.ReceiptHistory):] {
		cmd := runtimeCommand(*r, receipt.CommandID)
		action := cmd.Action
		if action == "" {
			action = "message"
		}
		correlation := runtimeActiveCorrelation(*r, contracts.RuntimeCorrelation{CommandID: &receipt.CommandID})
		if e := runtimeEvent(r, contracts.RuntimeAuditEvent{Kind: "command_" + receipt.Status, Outcome: receipt.Status, Step: "command", StateSequence: int64(r.StateSequence), Correlation: correlation, Parameters: contracts.RuntimeParameters{Action: action}, EvidenceRefs: receipt.ResultRefs}); e != nil {
			return e
		}
	}
	var prior contracts.S2AuditBundle
	if old.Control != nil {
		prior = old.Control.Audit
	}
	a := r.Control.Audit
	known := map[string]bool{}
	for _, m := range prior.Messages {
		known[m.MessageID] = true
	}
	for _, m := range a.Messages {
		if known[m.MessageID] || m.Role != "user" || m.CommandID == nil {
			continue
		}
		cmd := runtimeCommand(*r, *m.CommandID)
		if cmd.Action != "" || progressQuery(cmd.Content) {
			continue
		}
		if e := emit("business_input", "accepted", contracts.RuntimeCorrelation{CommandID: m.CommandID}, runtimeObject(*r, "MessageEnvelope", m.SourceVersion, m), nil); e != nil {
			return e
		}
	}
	decisions := map[string]bool{}
	for _, v := range prior.Decisions {
		decisions[v.DecisionID] = true
	}
	for _, v := range a.Decisions {
		if decisions[v.DecisionID] {
			continue
		}
		co := contracts.RuntimeCorrelation{}
		for _, in := range r.Control.ValidationInputs {
			if in.Decision.DecisionID == v.DecisionID {
				co.CommandID = optionalString(in.Facts.CommandID)
			}
		}
		if e := emit("decision_proposed", "suggested", co, runtimeObject(*r, "DecisionProposal", v.SourceVersion, v), nil); e != nil {
			return e
		}
	}
	validations := map[string]bool{}
	for _, v := range prior.Validations {
		validations[v.ValidationID] = true
	}
	for _, v := range a.Validations {
		if validations[v.ValidationID] {
			continue
		}
		co := contracts.RuntimeCorrelation{}
		for _, in := range r.Control.ValidationInputs {
			if in.Decision.DecisionID == v.DecisionID {
				co.CommandID = optionalString(in.Facts.CommandID)
			}
		}
		if v.ExecutionRequest != nil {
			x := v.ExecutionRequest
			co.ExecutionID = &x.ExecutionID
			co.OperationID = &x.OperationID
			co.ApprovalID = &x.Approval.ApprovalID
		}
		if e := emit("authorization", v.Verdict, co, runtimeObject(*r, "ActionValidation", v.SourceVersion, v), nil); e != nil {
			return e
		}
	}
	approvals := map[string]string{}
	for _, v := range prior.ApprovalHistory {
		approvals[v.ApprovalID] = v.Status
	}
	for _, v := range a.ApprovalHistory {
		if approvals[v.ApprovalID] == v.Status {
			continue
		}
		if e := emit("approval", v.Status, contracts.RuntimeCorrelation{ApprovalID: &v.ApprovalID}, nil, nil); e != nil {
			return e
		}
	}
	ops := map[string]contracts.OperationRecord{}
	for _, v := range prior.Operations {
		ops[v.OperationID] = v
	}
	for _, v := range a.Operations {
		if p, ok := ops[v.OperationID]; ok && equalContractValue(p, v) {
			continue
		}
		co := contracts.RuntimeCorrelation{OperationID: &v.OperationID}
		for _, x := range a.Executions {
			if x.OperationID == v.OperationID {
				co.ExecutionID = &x.ExecutionID
				co.CommandID = &x.CommandID
				co.ApprovalID = &x.Approval.ApprovalID
			}
		}
		if e := emit("operation", v.Status, co, v.EvidenceRefs, runtimeElapsed(v.CreatedAt, v.UpdatedAt)); e != nil {
			return e
		}
	}
	reports := map[string]bool{}
	for _, v := range prior.VerificationReports {
		reports[v.ReportHash] = true
	}
	for _, v := range a.VerificationReports {
		if reports[v.ReportHash] {
			continue
		}
		if e := emit("verification", v.Outcome, contracts.RuntimeCorrelation{}, runtimeObject(*r, "VerificationReport", v.SourceVersion, v), nil); e != nil {
			return e
		}
	}
	if terminal(r.State) {
		found := false
		for _, e := range rt.Events {
			if e.Kind == "run_terminal" {
				found = true
				break
			}
		}
		if !found {
			if e := emit("run_terminal", r.State, contracts.RuntimeCorrelation{}, nil, runtimeElapsed(r.CreatedAt, r.UpdatedAt)); e != nil {
				return e
			}
		}
	}
	return validateRuntimeState(*r)
}

func validateRuntimeState(r Run) error {
	rt := runtimeState(r)
	if rt == nil {
		return nil
	}
	if len(rt.Events) == 0 || rt.Events[0].Kind != "run_created" || len(rt.Events) > 32768 || len(rt.Attempts) > 4096 {
		return fail("runtime_history_invalid", "缺少完整运行历史")
	}
	var previous *string
	seq := int64(0)
	for i, e := range rt.Events {
		hash, err := contracts.HashObject("RuntimeAuditEvent", e)
		if err != nil || contracts.ValidateRuntime("RuntimeAuditEvent", e) != nil || e.RunID != r.ID || e.SourceVersion > int64(r.SourceVersion) || e.DefinitionSnapshotHash != r.FrozenDefinition.SnapshotHash || e.Sequence != int64(i+1) || e.EventHash != hash || !equalContractValue(e.PreviousHash, previous) || e.StateSequence < seq || e.StateSequence > int64(r.StateSequence) {
			return fail("runtime_history_invalid", "审计来源、顺序或hash链损坏")
		}
		want, _ := contracts.HashValue(map[string]any{"action": e.Parameters.Action, "argument_keys": e.Parameters.ArgumentKeys})
		if e.Parameters.SummaryHash != want {
			return fail("runtime_history_invalid", "脱敏参数摘要损坏")
		}
		h := e.EventHash
		previous = &h
		seq = e.StateSequence
	}
	ids := map[string]bool{}
	for _, a := range rt.Attempts {
		if ids[a.AttemptID] || contracts.ValidateRuntime("RuntimeActivityAttempt", a) != nil || a.RunID != r.ID || a.Mode != r.ExecutionMode {
			return fail("runtime_activity_invalid", "实际尝试身份或Schema损坏")
		}
		ids[a.AttemptID] = true
		starts, ends := 0, 0
		for _, e := range rt.Events {
			if e.Correlation.AttemptID == nil || *e.Correlation.AttemptID != a.AttemptID {
				continue
			}
			if !equalContractValue(e.Correlation.ActivityID, &a.ActivityID) || !equalContractValue(e.Correlation.Attempt, &a.Attempt) || !equalContractValue(e.Correlation.ExecutionRunID, a.ExecutionRunID) {
				return fail("runtime_activity_invalid", "活动与审计关联不一致")
			}
			if e.Kind == "activity_started" {
				starts++
			}
			if e.Kind == "activity_finished" || e.Kind == "recovery" {
				ends++
				if e.Outcome != a.Status || !equalContractValue(e.ElapsedMS, a.ElapsedMS) {
					return fail("runtime_activity_invalid", "活动结果与审计结论不同")
				}
			}
		}
		if starts != 1 || (a.Status == "running" && ends != 0) || (a.Status != "running" && ends != 1) {
			return fail("runtime_activity_invalid", "活动缺少唯一真实调度或完成事件")
		}
		if a.Mode == "temporal" && (a.WorkflowID == nil || a.ExecutionRunID == nil) || a.Mode == "local" && (a.WorkflowID != nil || a.ExecutionRunID != nil || a.Attempt != 1) {
			return fail("runtime_activity_invalid", "执行模式与实际追踪身份不同")
		}
		if (a.Status == "running") != (a.CompletedAt == nil) || a.CompletedAt != nil && a.ElapsedMS == nil {
			return fail("runtime_activity_invalid", "尝试结果时间不完整")
		}
	}
	return nil
}

func runtimeMetrics(r Run, at time.Time) contracts.RuntimeMetrics {
	m := contracts.RuntimeMetrics{Availability: "unavailable", AsOf: at.UTC().Format(time.RFC3339Nano), Basis: "first_terminal_actual_attempt_distinct_report_accepted_requirement_v1"}
	rt := runtimeState(r)
	if rt == nil {
		return m
	}
	m.Availability = "complete"
	success, failed, retries, verify, rounds := int64(0), int64(0), int64(0), int64(0), int64(0)
	inputs := map[string]bool{}
	reports := map[string]bool{}
	end := m.AsOf
	for _, e := range rt.Events {
		if e.Kind == "run_terminal" && m.FirstTerminalAt == nil {
			t, state := e.RecordedAt, e.Outcome
			m.FirstTerminalAt = &t
			m.FirstTerminalState = &state
			end = t
			if state == "DONE" {
				success = 1
			}
			if state == "FAILED" {
				failed = 1
			}
		}
		if e.Kind == "business_input" && e.Correlation.CommandID != nil {
			inputs[*e.Correlation.CommandID] = true
		}
		if e.Kind == "verification" && e.Outcome == "failed" {
			for _, r := range e.EvidenceRefs {
				reports[r.Hash] = true
			}
		}
	}
	for _, a := range rt.Attempts {
		if a.Mode == "temporal" && a.Attempt > 1 {
			retries++
		}
	}
	rounds = int64(len(inputs))
	verify = int64(len(reports))
	m.SuccessfulRuns = &success
	m.FailedRuns = &failed
	m.ActivityRetries = &retries
	m.VerificationFailures = &verify
	m.EffectiveBusinessRounds = &rounds
	m.TotalElapsedMS = runtimeElapsed(r.CreatedAt, end)
	return m
}
func runtimeFailureClass(code string) (string, string, []string) {
	switch {
	case strings.Contains(code, "unknown") || strings.Contains(code, "reconcil"):
		return "unknown_result", "query_original_result", []string{"核实原operation的准确结果；不要重新发送外部写入"}
	case strings.Contains(code, "budget") || strings.Contains(code, "deadline"):
		return "budget", "new_run", []string{"当前冻结额度或累计期限已耗尽；新Run仍须重新批准"}
	case strings.Contains(code, "model_") && (strings.Contains(code, "schema") || strings.Contains(code, "protocol") || strings.Contains(code, "invalid")):
		return "model_protocol", "correct_input", []string{"模型输出必须符合冻结Schema；修正provider配置后显式重试原命令"}
	case strings.Contains(code, "approval") || strings.Contains(code, "permission") || strings.Contains(code, "unauthorized"):
		return "authorization", "revise_and_reapprove", []string{"取得当前来源及准确作用域的有效批准"}
	case strings.Contains(code, "tool") || strings.Contains(code, "build") || strings.Contains(code, "command_timeout"):
		return "tool", "restore_dependency", []string{"恢复受限工具依赖并核对当前产物"}
	case strings.Contains(code, "invalid") || strings.Contains(code, "stale") || strings.Contains(code, "conflict") || strings.Contains(code, "schema"):
		return "input", "correct_input", []string{"修正输入或读取当前版本；保留原命令身份"}
	default:
		return "infrastructure", "restore_dependency", []string{"恢复依赖并核对原执行与预算账本"}
	}
}
