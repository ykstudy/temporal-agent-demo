package agent

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"agentstudio/internal/contracts"
	"agentstudio/internal/governance"
	"agentstudio/internal/storage"
)

// ControlState persists complete published objects. The reference index and
// original validator inputs are private persistence metadata, never substitute
// public DTOs or authority reconstructed from the legacy display projection.
type ControlState struct {
	Conversation     *ConversationState          `json:"conversation,omitempty"`
	Audit            contracts.S2AuditBundle     `json:"audit"`
	ObjectRefs       []contracts.ObjectReference `json:"object_refs"`
	ValidationInputs []governance.Input          `json:"validation_inputs"`
	Schedule         CanonicalScheduleState      `json:"schedule"`
	// Read observations survive independently of a write attempt, including
	// preflight reads. Their complete published bodies are copied into receipts.
	RemoteObservations map[string][]contracts.RemoteReadObservation `json:"remote_observations,omitempty"`
	RemoteWriteStarted map[string]bool                              `json:"remote_write_started,omitempty"`
	GitInvocations     map[string]controlGitInvocation              `json:"git_invocations,omitempty"`
	RetryOrigin        *controlRetryOrigin                          `json:"retry_origin,omitempty"`
}

func runModes(r Run) contracts.Modes {
	return contracts.Modes{ExecutionMode: r.ExecutionMode, DecisionMode: r.DecisionMode, DeliveryMode: r.DeliveryMode}
}

func newControlState(r Run, budget contracts.BudgetLedger) (*ControlState, error) {
	if r.FrozenDefinition == nil || !chainContains([]string{"1.2.0", "1.3.0", "1.4.0"}, r.FrozenDefinition.ContractVersion) {
		return nil, fail("legacy_contract_incomplete", "受控执行需要新发布的完整来源，不能补造历史")
	}
	a := contracts.S2AuditBundle{
		ContractVersion: r.FrozenDefinition.ContractVersion, SchemaVersion: 2, RunID: r.ID, Modes: runModes(r), State: r.State,
		SourceVersion: int64(r.SourceVersion), StateSequence: int64(r.StateSequence), DefinitionSnapshot: *r.FrozenDefinition,
		ProposalHistory: []contracts.Proposal{}, ApprovalHistory: []contracts.Approval{}, PermissionHistory: []contracts.PermissionSnapshot{},
		Decisions: []contracts.DecisionProposal{}, Validations: []contracts.ActionValidation{}, Executions: []contracts.ExecutionRequest{},
		Messages: []contracts.MessageEnvelope{}, Contexts: []contracts.ContextRecord{}, Operations: []contracts.OperationRecord{},
		ManifestHistory: []contracts.ArtifactManifest{}, Budget: budget, ToolReceipts: []contracts.ToolExecutionReceipt{}, WorkspaceReceipts: []contracts.WorkspaceReceipt{},
		CreatedAt: r.CreatedAt,
	}
	if contracts.DeliveryContract(r.FrozenDefinition.ContractVersion) {
		a.RemoteReceipts = []contracts.RemoteExecutionReceipt{}
		a.PackageReceipts = []contracts.PackageReceipt{}
		a.VerificationCandidates = []contracts.VerificationCandidate{}
		a.VerificationReports = []contracts.VerificationReport{}
	}
	if budget.ContractVersion != r.FrozenDefinition.ContractVersion {
		return nil, fail("budget_binding_mismatch", "预算契约版本必须明确等于Run冻结版本")
	}
	var err error
	a.AuditHash, err = contracts.HashObject("S2AuditBundle", a)
	if err == nil {
		err = contracts.Validate("S2AuditBundle", a)
	}
	c := &ControlState{Audit: a, ObjectRefs: []contracts.ObjectReference{}, ValidationInputs: []governance.Input{}}
	if r.FrozenDefinition.ContractVersion == "1.4.0" {
		c.Conversation = newConversation()
	}
	return c, err
}

// prepareCanonical publishes complete immutable bodies before their owner
// snapshot can reference them. A crash may leave an unreachable body, never a
// snapshot that advertises a body which was not durably registered.
func (s *Store) prepareCanonical(r *Run) error {
	if r.Control == nil {
		return nil
	}
	if err := prepareConversation(r); err != nil {
		return err
	}
	if err := captureRuntimeDelta(r, s.runs[r.ID]); err != nil {
		return err
	}
	a := &r.Control.Audit
	if r.FrozenDefinition == nil || a.DefinitionSnapshot.SnapshotHash != r.FrozenDefinition.SnapshotHash || a.RunID != r.ID {
		return fail("control_binding_mismatch", "受控对象与Run冻结来源不匹配")
	}
	if err := verifyControlChain(*r); err != nil {
		return err
	}
	if err := governance.ValidateBudgetLedger(r.FrozenDefinition.Definition.BudgetPolicy, a.Budget); err != nil {
		return err
	}
	if a.Budget.RunID != r.ID || a.Budget.DefinitionSnapshotHash != r.FrozenDefinition.SnapshotHash {
		return fail("budget_binding_mismatch", "预算账本未绑定当前Run和冻结定义")
	}
	a.State, a.SourceVersion, a.StateSequence, a.Modes = r.State, int64(r.SourceVersion), int64(r.StateSequence), runModes(*r)
	var err error
	a.AuditHash, err = contracts.HashObject("S2AuditBundle", *a)
	if err != nil {
		return err
	}
	if err = contracts.Validate("S2AuditBundle", *a); err != nil {
		return fmt.Errorf("canonical control schema: %w", err)
	}
	archive := func(schema string, source int64, body any) error {
		ref, e := storage.Reference(r.ID, source, schema, body)
		if e != nil {
			return e
		}
		for _, old := range r.Control.ObjectRefs {
			if old.ObjectType == ref.ObjectType && old.ObjectID == ref.ObjectID && old.ObjectVersion == ref.ObjectVersion {
				// A ledger has no source field: retain the source of its first
				// publication when its unchanged version is observed again.
				if schema == "BudgetLedger" && old.Hash == ref.Hash {
					ref.SourceVersion = old.SourceVersion
				}
				if old != ref {
					return storage.ErrObjectConflict
				}
				stored, readErr := s.objects.Get(r.ID, old)
				if readErr != nil {
					return readErr
				}
				encoded, encodeErr := json.Marshal(body)
				if encodeErr != nil {
					return encodeErr
				}
				encoded, encodeErr = contracts.Canonicalize(encoded)
				if encodeErr != nil {
					return encodeErr
				}
				if !bytes.Equal(stored, encoded) {
					return storage.ErrObjectConflict
				}
				return nil
			}
		}
		if _, e = s.objects.Put(r.ID, source, schema, body); e != nil {
			return e
		}
		r.Control.ObjectRefs = append(r.Control.ObjectRefs, ref)
		return nil
	}
	if err = visitCanonicalBodies(*r, archive); err != nil {
		return err
	}
	if conversationState(*r) != nil {
		snapshot, e := projectSnapshot(*r)
		if e != nil {
			return e
		}
		if e = archive("RunSnapshot", int64(r.SourceVersion), snapshot); e != nil {
			return e
		}
		ref, e := storage.Reference(r.ID, int64(r.SourceVersion), "RunSnapshot", snapshot)
		if e != nil {
			return e
		}
		bindReceiptSnapshots(r, ref)
	}
	return nil
}

// The required index is derived from the complete public bodies. The private
// ObjectRefs list cannot hide a missing message, context or authorization.
func visitCanonicalBodies(r Run, archive func(string, int64, any) error) error {
	a := &r.Control.Audit
	var err error
	if c := conversationState(r); c != nil {
		for _, value := range c.Summaries {
			if err = archive("SummaryRecord", value.SourceVersion, value); err != nil {
				return err
			}
		}
		for _, value := range c.Attempts {
			if err = archive("SummaryAttempt", value.SourceVersion, value); err != nil {
				return err
			}
		}
	}
	if a.Plan != nil {
		if err = archive("ProjectPlan", a.SourceVersion, *a.Plan); err != nil {
			return err
		}
	}
	for _, value := range a.ProposalHistory {
		if err = archive("Proposal", value.SourceVersion, value); err != nil {
			return err
		}
	}
	for _, value := range a.ApprovalHistory {
		if err = archive("Approval", value.SourceVersion, value); err != nil {
			return err
		}
	}
	for _, value := range a.Messages {
		if err = archive("MessageEnvelope", value.SourceVersion, value); err != nil {
			return err
		}
	}
	for _, value := range a.Decisions {
		if err = archive("DecisionProposal", value.SourceVersion, value); err != nil {
			return err
		}
	}
	for _, value := range a.Contexts {
		if err = archive("ContextRecord", value.SourceVersion, value); err != nil {
			return err
		}
	}
	for _, value := range a.Validations {
		if err = archive("ActionValidation", value.SourceVersion, value); err != nil {
			return err
		}
	}
	for _, value := range a.Executions {
		if err = archive("ExecutionRequest", value.SourceVersion, value); err != nil {
			return err
		}
	}
	for _, value := range a.ManifestHistory {
		if err = archive("ArtifactManifest", value.SourceVersion, value); err != nil {
			return err
		}
	}
	for _, value := range a.Budget.Reservations {
		if err = archive("BudgetReservation", value.SourceVersion, value); err != nil {
			return err
		}
	}
	for _, value := range a.Budget.Settlements {
		if err = archive("BudgetSettlement", value.SourceVersion, value); err != nil {
			return err
		}
	}
	for _, value := range a.ToolReceipts {
		if err = archive("ToolExecutionReceipt", value.SourceVersion, value); err != nil {
			return err
		}
	}
	for _, value := range a.WorkspaceReceipts {
		if err = archive("WorkspaceReceipt", value.SourceVersion, value); err != nil {
			return err
		}
	}
	for _, value := range a.RemoteReceipts {
		if err = archive("RemoteExecutionReceipt", value.SourceVersion, value); err != nil {
			return err
		}
	}
	for _, value := range a.PackageReceipts {
		if err = archive("PackageReceipt", value.SourceVersion, value); err != nil {
			return err
		}
	}
	for _, value := range a.VerificationCandidates {
		if err = archive("VerificationCandidate", value.SourceVersion, value); err != nil {
			return err
		}
	}
	for _, value := range a.VerificationReports {
		if err = archive("VerificationReport", value.SourceVersion, value); err != nil {
			return err
		}
	}
	return archive("BudgetLedger", a.SourceVersion, a.Budget)
}

func (s *Store) verifyCanonical(r Run) error {
	if err := validateRuntimeState(r); err != nil {
		return err
	}
	if r.Control == nil {
		return nil
	}
	if err := verifyControlChain(r); err != nil {
		return err
	}
	a := r.Control.Audit
	if err := contracts.Validate("S2AuditBundle", a); err != nil {
		return err
	}
	if r.FrozenDefinition == nil {
		return fail("control_binding_mismatch", "完整冻结来源缺失")
	}
	if err := governance.ValidateBudgetLedger(r.FrozenDefinition.Definition.BudgetPolicy, a.Budget); err != nil {
		return err
	}
	if a.Budget.RunID != r.ID || a.Budget.DefinitionSnapshotHash != r.FrozenDefinition.SnapshotHash {
		return fail("budget_binding_mismatch", "预算账本绑定错误")
	}
	hash, err := contracts.HashObject("S2AuditBundle", a)
	if err != nil || hash != a.AuditHash || a.RunID != r.ID || a.SourceVersion != int64(r.SourceVersion) || a.StateSequence != int64(r.StateSequence) || a.State != r.State || a.Modes != runModes(r) || r.FrozenDefinition == nil || a.DefinitionSnapshot.SnapshotHash != r.FrozenDefinition.SnapshotHash {
		return fail("control_binding_mismatch", "受控快照身份、版本或摘要损坏")
	}
	indexed := map[string]contracts.ObjectReference{}
	for _, ref := range r.Control.ObjectRefs {
		key := ref.ObjectType + "/" + ref.ObjectID + "/" + strconv.FormatInt(ref.ObjectVersion, 10)
		if _, duplicate := indexed[key]; duplicate || ref.SourceVersion > int64(r.SourceVersion) {
			return fail("control_index_mismatch", "不可变对象索引重复或来源越界")
		}
		indexed[key] = ref
		if _, err := s.objects.Get(r.ID, ref); err != nil {
			return err
		}
	}
	if err := visitCanonicalBodies(r, func(schema string, source int64, body any) error {
		ref, err := storage.Reference(r.ID, source, schema, body)
		if err != nil {
			return err
		}
		key := ref.ObjectType + "/" + ref.ObjectID + "/" + strconv.FormatInt(ref.ObjectVersion, 10)
		bound, ok := indexed[key]
		if !ok {
			return fail("control_index_mismatch", "完整历史主体缺少不可变登记引用")
		}
		if schema == "BudgetLedger" {
			ref.SourceVersion = bound.SourceVersion
		}
		if ref != bound {
			return fail("control_index_mismatch", "完整历史主体与索引身份不符")
		}
		stored, err := s.objects.Get(r.ID, bound)
		if err != nil {
			return err
		}
		encoded, err := json.Marshal(body)
		if err != nil {
			return err
		}
		encoded, err = contracts.Canonicalize(encoded)
		if err != nil {
			return err
		}
		if !bytes.Equal(stored, encoded) {
			return fail("control_index_mismatch", "历史主体与不可变原体不符")
		}
		return nil
	}); err != nil {
		return err
	}
	if c := conversationState(r); c != nil {
		snapshot, e := projectSnapshot(r)
		if e != nil {
			return e
		}
		ref, e := storage.Reference(r.ID, int64(r.SourceVersion), "RunSnapshot", snapshot)
		if e != nil || c.LatestSnapshot == nil || *c.LatestSnapshot != ref {
			return fail("snapshot_binding_mismatch", "current Run lacks its exact immutable snapshot")
		}
		if _, e = s.objects.Get(r.ID, ref); e != nil {
			return e
		}
		for _, receipt := range c.ReceiptHistory {
			if receipt.RunRef == nil {
				return fail("receipt_binding_mismatch", "committed receipt lacks its result snapshot")
			}
		}
		if e = validateConversation(r); e != nil {
			return e
		}
	}
	return nil
}

func (s *Store) CanonicalAudit(id string) (contracts.S2AuditBundle, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.runs[id]
	if !ok {
		return contracts.S2AuditBundle{}, ErrNotFound
	}
	if r.Control == nil {
		return contracts.S2AuditBundle{}, fail("legacy_contract_incomplete", "旧Run缺少完整受控对象来源")
	}
	if err := s.verifyCanonical(r); err != nil {
		return contracts.S2AuditBundle{}, fail("object_corrupt", "受控对象或不可变来源损坏")
	}
	return clone(r).Control.Audit, nil
}

func (s *Store) CanonicalObject(id string, ref contracts.ObjectReference) (json.RawMessage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.runs[id]
	if !ok {
		return nil, ErrNotFound
	}
	if r.Control == nil {
		return nil, fail("legacy_contract_incomplete", "旧Run没有完整对象索引")
	}
	if err := contracts.Validate("ObjectReference", ref); err != nil {
		return nil, fail("invalid_reference", "对象引用需要完整准确的五个字段")
	}
	for _, bound := range r.Control.ObjectRefs {
		if bound == ref {
			body, err := s.objects.Get(id, ref)
			if err != nil {
				return nil, fail("object_corrupt", "不可变对象身份或内容损坏")
			}
			return body, nil
		}
	}
	return nil, ErrNotFound
}

func (s *Store) CanonicalBudget(id string) (contracts.BudgetLedger, contracts.ObjectReference, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.runs[id]
	if !ok {
		return contracts.BudgetLedger{}, contracts.ObjectReference{}, ErrNotFound
	}
	if r.Control == nil {
		return contracts.BudgetLedger{}, contracts.ObjectReference{}, fail("legacy_contract_incomplete", "旧Run缺少完整预算来源")
	}
	if err := s.verifyCanonical(r); err != nil {
		return contracts.BudgetLedger{}, contracts.ObjectReference{}, fail("object_corrupt", "预算对象或登记索引损坏")
	}
	budget := r.Control.Audit.Budget
	for _, ref := range r.Control.ObjectRefs {
		if ref.ObjectType == "budget" && ref.ObjectID == id && ref.ObjectVersion == budget.LedgerVersion && ref.Hash == budget.LedgerHash {
			return clone(r).Control.Audit.Budget, ref, nil
		}
	}
	return contracts.BudgetLedger{}, contracts.ObjectReference{}, fail("object_corrupt", "预算对象缺少首次登记的精确来源引用")
}

func (s *Service) registerControlRoutes(mux *http.ServeMux) {
	writeReadError := func(w http.ResponseWriter, req *http.Request, err error) {
		var api *APIError
		if errors.As(err, &api) && api.Code == "legacy_contract_incomplete" {
			writeJSON(w, 409, map[string]string{"error": api.Message, "code": api.Code, "legacy_read_url": "/api/runs/" + req.PathValue("id")})
		} else if errors.As(err, &api) && api.Code == "object_corrupt" {
			writeJSON(w, 409, map[string]string{"error": api.Message, "code": api.Code})
		} else {
			writeError(w, err)
		}
	}
	mux.HandleFunc("GET /api/runs/{id}/conversation-audit", func(w http.ResponseWriter, req *http.Request) {
		audit, err := s.Store.ConversationAudit(req.PathValue("id"))
		if err != nil {
			writeReadError(w, req, err)
			return
		}
		writeJSON(w, 200, audit)
	})
	mux.HandleFunc("GET /api/runs/{id}/transcript", func(w http.ResponseWriter, req *http.Request) {
		q := req.URL.Query()
		for key, values := range q {
			if (key != "first_sequence" && key != "last_sequence") || len(values) != 1 {
				writeError(w, fail("invalid_range", "unknown or duplicate range parameter"))
				return
			}
		}
		body, err := s.Store.ConversationTranscript(req.PathValue("id"), q.Get("first_sequence"), q.Get("last_sequence"))
		if err != nil {
			writeReadError(w, req, err)
			return
		}
		w.Header().Set("Content-Type", "application/x-ndjson")
		w.Header().Set("Cache-Control", "no-store")
		w.Write(body)
	})
	mux.HandleFunc("GET /api/runs/{id}/decision-audit", func(w http.ResponseWriter, req *http.Request) {
		audit, err := s.Store.CanonicalAudit(req.PathValue("id"))
		if err != nil {
			writeReadError(w, req, err)
			return
		}
		writeJSON(w, 200, audit)
	})
	mux.HandleFunc("GET /api/runs/{id}/budget", func(w http.ResponseWriter, req *http.Request) {
		budget, ref, err := s.Store.CanonicalBudget(req.PathValue("id"))
		if err != nil {
			writeReadError(w, req, err)
			return
		}
		query := url.Values{"object_type": {ref.ObjectType}, "object_id": {ref.ObjectID}, "object_version": {strconv.FormatInt(ref.ObjectVersion, 10)}, "source_version": {strconv.FormatInt(ref.SourceVersion, 10)}, "hash": {ref.Hash}}
		w.Header().Set("Link", "</api/runs/"+req.PathValue("id")+"/objects?"+query.Encode()+">; rel=\"canonical\"")
		writeJSON(w, 200, budget)
	})
	mux.HandleFunc("GET /api/runs/{id}/objects", func(w http.ResponseWriter, req *http.Request) {
		q := req.URL.Query()
		for _, key := range []string{"object_type", "object_id", "object_version", "source_version", "hash"} {
			if len(q[key]) != 1 || q.Get(key) == "" {
				writeError(w, fail("invalid_reference", "对象引用需要完整准确的五个字段"))
				return
			}
		}
		if len(q) != 5 {
			writeError(w, fail("invalid_reference", "对象引用包含未知参数"))
			return
		}
		version, e1 := strconv.ParseInt(q.Get("object_version"), 10, 64)
		source, e2 := strconv.ParseInt(q.Get("source_version"), 10, 64)
		if e1 != nil || e2 != nil || strconv.FormatInt(version, 10) != q.Get("object_version") || strconv.FormatInt(source, 10) != q.Get("source_version") {
			writeError(w, fail("invalid_reference", "版本须为规范正整数"))
			return
		}
		ref := contracts.ObjectReference{ObjectType: q.Get("object_type"), ObjectID: q.Get("object_id"), ObjectVersion: version, SourceVersion: source, Hash: q.Get("hash")}
		if ref.ObjectType == "package" {
			body, err := s.ReadControlPackage(req.PathValue("id"), ref)
			if err != nil {
				writeReadError(w, req, err)
				return
			}
			w.Header().Set("Content-Type", "application/zip")
			w.Header().Set("X-Content-SHA256", contracts.BytesHash(body))
			_, _ = w.Write(body)
			return
		}
		if ref.ObjectType == "ci_artifact" {
			body, err := s.ReadControlCIArtifact(req.PathValue("id"), ref)
			if err != nil {
				writeReadError(w, req, err)
				return
			}
			w.Header().Set("Content-Type", "application/octet-stream")
			w.Header().Set("X-Content-SHA256", contracts.BytesHash(body))
			_, _ = w.Write(body)
			return
		}
		body, err := s.Store.CanonicalObject(req.PathValue("id"), ref)
		if err != nil {
			writeReadError(w, req, err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Content-SHA256", contracts.BytesHash(body))
		_, _ = w.Write(body)
	})
}
