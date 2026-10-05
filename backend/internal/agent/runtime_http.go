package agent

import (
	"context"
	"net/http"
	"sort"
	"time"

	"agentstudio/internal/contracts"
)

type runtimeWorkflowStatus struct {
	RunID             string  `json:"run_id"`
	State             string  `json:"state"`
	SourceVersion     int64   `json:"source_version"`
	StateSequence     int64   `json:"state_sequence"`
	CommandsProcessed int64   `json:"commands_processed"`
	CheckpointAt      *string `json:"checkpoint_at"`
	ExecutionRunID    string  `json:"execution_run_id"`
}

func (s *Service) runtimeCheckpoint(ctx context.Context, r Run, at string) contracts.RuntimeCheckpoint {
	q := contracts.RuntimeCheckpoint{Availability: "not_applicable", Reason: "explicit_local_execution", SampledAt: at}
	if r.ExecutionMode != "temporal" {
		return q
	}
	q.Availability = "unavailable"
	q.Reason = "temporal_unavailable"
	wf := workflowID(r.ID)
	q.WorkflowID = &wf
	if s.Temporal == nil {
		return q
	}
	bounded, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	desc, err := s.Temporal.DescribeWorkflowExecution(bounded, wf, "")
	if err != nil || desc.WorkflowExecutionInfo == nil || desc.WorkflowExecutionInfo.Execution == nil {
		q.Reason = "execution_unavailable"
		return q
	}
	execution := desc.WorkflowExecutionInfo.Execution.RunId
	q.ExecutionRunID = &execution
	value, err := s.Temporal.QueryWorkflow(bounded, wf, execution, "get_status")
	if err != nil {
		q.Reason = "query_unavailable"
		return q
	}
	var status runtimeWorkflowStatus
	if value.Get(&status) != nil || status.RunID != r.ID || status.ExecutionRunID != execution || status.CheckpointAt == nil {
		q.Reason = "checkpoint_provenance_unavailable"
		return q
	}
	q.Availability = "available"
	q.Reason = "exact_execution_query"
	q.CheckpointAt = status.CheckpointAt
	q.State = &status.State
	q.SourceVersion = &status.SourceVersion
	q.StateSequence = &status.StateSequence
	q.CommandsProcessed = &status.CommandsProcessed
	if status.StateSequence != int64(r.StateSequence) || status.SourceVersion != int64(r.SourceVersion) {
		q.Availability = "stale"
		q.Reason = "checkpoint_differs_from_durable_snapshot"
	}
	return q
}
func runtimeDiagnosis(r Run) *contracts.RuntimeFailureDiagnosis {
	c := conversationState(r)
	if c == nil {
		return nil
	}
	diagnosis := func(code, category, step, message, action string, missing []string, refs []contracts.ObjectRef) *contracts.RuntimeFailureDiagnosis {
		return &contracts.RuntimeFailureDiagnosis{Code: code, Category: category, Step: step, Message: message, SuggestedAction: action, MissingRequirements: missing, AffectedRefs: refs, RecordedAt: r.UpdatedAt}
	}
	for _, op := range r.Control.Audit.Operations {
		if op.Status == "unknown" || op.Status == "sent" && r.State == "BLOCKED" {
			d := diagnosis("operation_result_unknown", "unknown_result", string(op.ActionType), "原外部或工具尝试结果尚未核实；不表示未执行或已回滚", "query_original_result", []string{"按原operation身份核实准确副作用", "核实仍须符合冻结读取预算"}, op.EvidenceRefs)
			d.OperationID = &op.OperationID
			url := "/api/runs/" + r.ID + "/decision-audit"
			d.ReconciliationURL = &url
			d.Retryable = false
			return d
		}
	}
	if r.State == "CANCELLED" {
		return diagnosis("user_cancelled", "user_cancelled", "control", "用户取消不撤销已发生的副作用", "new_run", []string{"需要继续工作时创建关联新Run并重新批准"}, []contracts.ObjectRef{})
	}
	if report := r.Control.Audit.VerificationReport; report != nil && report.Outcome == "failed" && report.SourceVersion == int64(r.SourceVersion) {
		return diagnosis("verification_failed", "business_verification", "verify", "当前产物未通过产品内部完整验证", "revise_and_reapprove", []string{"读取失败检查，生成修复候选并取得新批准"}, runtimeObject(r, "VerificationReport", report.SourceVersion, *report))
	}
	if r.Error != "" {
		for i := len(c.ReceiptHistory) - 1; i >= 0; i-- {
			rec := c.ReceiptHistory[i]
			if rec.Error == nil {
				continue
			}
			category, action, missing := runtimeFailureClass(rec.Error.Code)
			d := diagnosis(rec.Error.Code, category, "command", rec.Error.Message, action, missing, rec.Error.EvidenceRefs)
			d.CommandID = &rec.CommandID
			d.Retryable = action == "restore_dependency" && !terminal(r.State) && runtimeBudgetAvailable(r)
			return d
		}
		if rt := runtimeState(r); rt != nil {
			for i := len(rt.Attempts) - 1; i >= 0; i-- {
				a := rt.Attempts[i]
				if a.ErrorCode == nil {
					continue
				}
				cat, action, missing := runtimeFailureClass(*a.ErrorCode)
				d := diagnosis(*a.ErrorCode, cat, a.Step, r.Error, action, missing, []contracts.ObjectRef{})
				d.CommandID = a.CommandID
				d.Retryable = action == "restore_dependency" && !terminal(r.State) && runtimeBudgetAvailable(r)
				return d
			}
		}
		return diagnosis("run_blocked", "infrastructure", r.State, r.Error, "restore_dependency", []string{"读取原操作和批准证据，恢复依赖后显式恢复"}, []contracts.ObjectRef{})
	}
	return nil
}
func runtimeBudgetAvailable(r Run) bool {
	b := r.Control.Audit.Budget
	p := r.FrozenDefinition.Definition.BudgetPolicy
	if b.Totals.Consumed.Command+b.Totals.Reserved.Command >= p.MaxCommands {
		return false
	}
	if b.ExecutionDeadlineAt != nil {
		t, e := time.Parse(time.RFC3339Nano, *b.ExecutionDeadlineAt)
		if e != nil || !time.Now().Before(t) {
			return false
		}
	}
	return true
}
func runtimeHead(r Run) *string {
	rt := runtimeState(r)
	if rt == nil || len(rt.Events) == 0 {
		return nil
	}
	h := rt.Events[len(rt.Events)-1].EventHash
	return &h
}
func (s *Service) RuntimeView(ctx context.Context, id string) (contracts.RuntimeView, error) {
	r, err := s.Store.Get(id)
	if err != nil {
		return contracts.RuntimeView{}, err
	}
	c := conversationState(r)
	if c == nil || c.LatestSnapshot == nil {
		return contracts.RuntimeView{}, fail("legacy_contract_incomplete", "原Run缺少完整业务来源，只可读取legacy")
	}
	at := time.Now().UTC()
	v := contracts.RuntimeView{RuntimeVersion: "1.0.0", RunID: id, RunRef: *c.LatestSnapshot, Modes: runModes(r), ObservedAt: at.Format(time.RFC3339Nano), ActivityAttempts: []contracts.RuntimeActivityAttempt{}, Metrics: runtimeMetrics(r, at), Diagnosis: runtimeDiagnosis(r), AuditCoverage: "unavailable", AuditHeadHash: runtimeHead(r), Budget: contracts.RuntimeBudgetStatus{Policy: r.FrozenDefinition.Definition.BudgetPolicy, ExecutionDeadline: r.Control.Audit.Budget.ExecutionDeadlineAt, ContextMeasurement: "conservative_utf8_byte_bound_plus_frozen_output_and_protocol_reserves"}}
	for _, ref := range r.Control.ObjectRefs {
		if ref.ObjectType == "budget" && ref.Hash == r.Control.Audit.Budget.LedgerHash {
			x := ref
			v.Budget.LedgerRef = &x
		}
	}
	v.Checkpoint = s.runtimeCheckpoint(ctx, r, v.ObservedAt)
	if rt := runtimeState(r); rt != nil {
		v.AuditCoverage = "complete"
		v.ActivityAttempts = append(v.ActivityAttempts, rt.Attempts...)
		s.runtime.mu.Lock()
		for i := range v.ActivityAttempts {
			a := &v.ActivityAttempts[i]
			if a.Status != "running" {
				continue
			}
			if last, ok := s.runtime.beats[a.AttemptID]; ok {
				a.LastHeartbeatAt = &last
				a.HeartbeatSource = "process"
				t, _ := time.Parse(time.RFC3339Nano, last)
				a.HeartbeatStatus = "fresh"
				if at.Sub(t) > 45*time.Second {
					a.HeartbeatStatus = "stale"
				}
			} else {
				a.HeartbeatStatus = "unavailable"
				a.HeartbeatSource = "unavailable"
			}
		}
		s.runtime.mu.Unlock()
	}
	v.ViewHash, err = contracts.HashObject("RuntimeView", v)
	if err == nil {
		err = contracts.ValidateRuntime("RuntimeView", v)
	}
	return v, err
}
func (s *Service) RuntimeAudit(id string) (contracts.RuntimeAuditTrail, error) {
	r, err := s.Store.Get(id)
	if err != nil {
		return contracts.RuntimeAuditTrail{}, err
	}
	c := conversationState(r)
	if c == nil || c.LatestSnapshot == nil {
		return contracts.RuntimeAuditTrail{}, fail("legacy_contract_incomplete", "原Run缺少完整业务来源")
	}
	a := contracts.RuntimeAuditTrail{RuntimeVersion: "1.0.0", RunID: id, RunRef: *c.LatestSnapshot, Coverage: "unavailable", Events: []contracts.RuntimeAuditEvent{}, HeadHash: runtimeHead(r), ObservedAt: now()}
	if rt := runtimeState(r); rt != nil {
		a.Coverage = "complete"
		a.Events = rt.Events
	}
	a.AuditHash, err = contracts.HashObject("RuntimeAuditTrail", a)
	if err == nil {
		err = contracts.ValidateRuntime("RuntimeAuditTrail", a)
	}
	return a, err
}
func (s *Service) RuntimeMetrics() (contracts.RuntimeServiceMetrics, error) {
	at := time.Now().UTC()
	m := contracts.RuntimeServiceMetrics{RuntimeVersion: "1.0.0", ObservedAt: at.Format(time.RFC3339Nano), IncludedRunIDs: []string{}, ExcludedRunIDs: []string{}}
	for _, r := range s.Store.List() {
		v := runtimeMetrics(r, at)
		if v.Availability != "complete" {
			m.ExcludedRunIDs = append(m.ExcludedRunIDs, r.ID)
			continue
		}
		m.IncludedRunIDs = append(m.IncludedRunIDs, r.ID)
		m.SuccessfulRuns += *v.SuccessfulRuns
		m.FailedRuns += *v.FailedRuns
		m.ActivityRetries += *v.ActivityRetries
		m.VerificationFailures += *v.VerificationFailures
		m.EffectiveBusinessRounds += *v.EffectiveBusinessRounds
		m.TotalElapsedMS += *v.TotalElapsedMS
	}
	sort.Strings(m.IncludedRunIDs)
	sort.Strings(m.ExcludedRunIDs)
	var err error
	m.MetricsHash, err = contracts.HashObject("RuntimeServiceMetrics", m)
	if err == nil {
		err = contracts.ValidateRuntime("RuntimeServiceMetrics", m)
	}
	return m, err
}
func (s *Service) registerRuntime(mux *http.ServeMux) {
	for _, probe := range []bool{false, true} {
		path := "live"
		if probe {
			path = "ready"
		}
		mux.HandleFunc("GET /api/runtime/"+path, func(w http.ResponseWriter, r *http.Request) {
			if !noV2Query(w, r) {
				return
			}
			h, err := s.RuntimeHealth(r.Context(), probe)
			if err != nil {
				writeV2Error(w, r, err)
				return
			}
			code := 200
			if probe && !h.Ready {
				code = 503
			}
			writeJSON(w, code, h)
		})
	}
	mux.HandleFunc("GET /api/runtime/metrics", func(w http.ResponseWriter, r *http.Request) {
		if !noV2Query(w, r) {
			return
		}
		v, e := s.RuntimeMetrics()
		if e != nil {
			writeV2Error(w, r, e)
			return
		}
		writeJSON(w, 200, v)
	})
	mux.HandleFunc("GET /api/v2/runs/{id}/runtime", func(w http.ResponseWriter, r *http.Request) {
		if !noV2Query(w, r) {
			return
		}
		v, e := s.RuntimeView(r.Context(), r.PathValue("id"))
		if e != nil {
			writeV2Error(w, r, e)
			return
		}
		writeJSON(w, 200, v)
	})
	mux.HandleFunc("GET /api/v2/runs/{id}/audit", func(w http.ResponseWriter, r *http.Request) {
		if !noV2Query(w, r) {
			return
		}
		v, e := s.RuntimeAudit(r.PathValue("id"))
		if e != nil {
			writeV2Error(w, r, e)
			return
		}
		writeJSON(w, 200, v)
	})
}
