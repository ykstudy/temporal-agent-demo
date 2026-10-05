package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"agentstudio/internal/contracts"
)

type controlGitLabMock struct {
	mu             sync.Mutex
	posts          int
	gets           int
	project        *controlGitLabProject
	readStatus     int
	namespaceID    int64
	lostResponse   bool
	postMutation   func(*controlGitLabProject)
	requestProblem string
	extra          func(http.ResponseWriter, *http.Request) bool
}

func (m *controlGitLabMock) serve(w http.ResponseWriter, r *http.Request) {
	if m.extra != nil && m.extra(w, r) {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	if r.Header.Get("PRIVATE-TOKEN") != "controlled-gitlab-test-secret" || strings.Contains(r.URL.String(), "controlled-gitlab-test-secret") {
		m.requestProblem = "credential not confined to trusted header"
	}
	if r.Method == http.MethodGet {
		m.gets++
		if r.URL.Path == "/api/v4/namespaces/17" {
			id := m.namespaceID
			if id == 0 {
				id = 17
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"id": id, "full_path": "schedule-group"})
			return
		}
		if r.URL.Path != "/api/v4/projects/schedule-group/create-service" && r.URL.Path != "/api/v4/projects/191" {
			m.requestProblem = "non-exact read path: " + r.URL.Path
			http.Error(w, "unexpected", 400)
			return
		}
		if m.readStatus != 0 {
			http.Error(w, "opaque remote failure controlled-gitlab-test-secret", m.readStatus)
			return
		}
		if m.project == nil {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		_ = json.NewEncoder(w).Encode(m.project)
		return
	}
	if r.Method != http.MethodPost || r.URL.Path != "/api/v4/projects" {
		m.requestProblem = "unexpected remote write"
		http.Error(w, "unexpected", 400)
		return
	}
	m.posts++
	var body struct {
		NamespaceID string `json:"namespace_id"`
		Name        string `json:"name"`
		Path        string `json:"path"`
		Visibility  string `json:"visibility"`
		Description string `json:"description"`
		Initialize  bool   `json:"initialize_with_readme"`
	}
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if decoder.Decode(&body) != nil || body.NamespaceID != "17" || body.Name != "create-service" || body.Path != "create-service" || body.Visibility != "private" || body.Initialize || !strings.HasPrefix(body.Description, "agentstudio:") {
		m.requestProblem = "create payload changed approved scope"
		http.Error(w, "unexpected body", 400)
		return
	}
	empty := true
	p := controlGitLabProject{ID: 191, Path: body.Path, PathWithNamespace: "schedule-group/" + body.Path, Description: body.Description, Visibility: body.Visibility, EmptyRepo: &empty}
	p.Namespace.ID, p.Namespace.FullPath = 17, "schedule-group"
	if m.postMutation != nil {
		m.postMutation(&p)
	}
	m.project = &p
	if m.lostResponse {
		http.Error(w, "response lost after committed effect controlled-gitlab-test-secret", 503)
		return
	}
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(p)
}

// The preceding workspace/tool receipts are explicitly synthetic scheduler
// preconditions. Only this module's HTTP calls and durable effects are real.
func controlGitLabFixture(t *testing.T, m *controlGitLabMock) (*Service, contracts.ExecutionRequest, string) {
	return controlGitLabFixtureVersion(t, m, "1.2.0")
}

func controlGitLabFixtureVersion(t *testing.T, m *controlGitLabMock, version string) (*Service, contracts.ExecutionRequest, string) {
	t.Helper()
	api := httptest.NewServer(http.HandlerFunc(m.serve))
	t.Cleanup(api.Close)
	s := testService(t)
	s.Config.GitLabURL, s.Config.GitLabNamespace, s.Config.GitLabToken = api.URL, "17", "controlled-gitlab-test-secret"
	var r Run
	if version == "1.3.0" {
		r = newControlCompletionRun(t, s, "rules")
	} else {
		r = controlFixture(t, s, "controlled_gitlab")
	}
	var err error
	r, err = s.Store.Update(r.ID, func(r *Run) error { r.DeliveryMode = "gitlab"; r.Plan.RepoProvider = "gitlab"; return nil })
	if err != nil {
		t.Fatal(err)
	}
	planned, err := s.ProcessCommand(context.Background(), Command{ID: "gitlab_plan", RunID: r.ID, Content: "创建 create-service"})
	if err != nil {
		t.Fatal(err)
	}
	r = controlAcceptCommand(t, s, planned.Run)
	for {
		next, err := s.PrepareControlAction(context.Background(), r.ID)
		if err != nil || next.Execution == nil {
			t.Fatal("preparing actual complete action", err)
		}
		x := *next.Execution
		kind, _, _ := scheduleAction(x.Action)
		if kind == "create_repository" {
			m.mu.Lock()
			m.gets = 0 // Exclude the separate planning namespace lookup.
			m.mu.Unlock()
			return s, x, r.ID
		}
		r, _ = s.Store.Get(r.ID)
		// finish() creates an op, so remove the prepared ready op in this
		// in-memory synthetic fixture before replacing it with its sent record.
		r.Control.Audit.Operations = r.Control.Audit.Operations[:len(r.Control.Audit.Operations)-1]
		f := &scheduleFixture{t: t, s: s, r: r, state: r.Control.Schedule, clock: time.Now().UTC()}
		f.finish(x)
		f.r.Control.Schedule = f.state
		if delay := time.Until(f.clock); delay > 0 {
			time.Sleep(delay)
		}
		r, err = s.Store.Update(r.ID, func(r *Run) error { *r = f.r; return nil })
		if err != nil {
			t.Fatal(err)
		}
	}
}

func assertControlCreateResult(t *testing.T, r Run, x contracts.ExecutionRequest, outcome string) {
	t.Helper()
	if len(r.Control.Schedule.Results) != 1 {
		t.Fatal("result count", len(r.Control.Schedule.Results))
	}
	result := r.Control.Schedule.Results[0]
	hash, _ := HashCanonicalActionResult(result)
	if result.ResultHash != hash || result.RequestHash != x.RequestHash || result.OperationID != x.OperationID || result.ProjectID != 191 || result.ObservedTarget == nil || result.ObservedTarget.Ref != "main" || result.CommitSHA != "" || result.PipelineID != 0 || result.CIConfigSHA256 != "" || result.ManifestRef == nil {
		t.Fatal("create facts claim unobserved state or lack binding", result)
	}
	attempt, err := controlGitLabAttempt(r, x)
	if err != nil {
		t.Fatal(err)
	}
	settlement := controlGitLabSettled(r, attempt)
	if settlement == nil || settlement.Outcome != outcome || settlement.ConsumedUnits != 1 || result.AttemptID != attempt.AttemptID {
		t.Fatal("original attempt not charged", settlement)
	}
	found := false
	for _, op := range r.Control.Audit.Operations {
		if op.OperationID == x.OperationID {
			found = op.Status == "verified" && op.ExternalObjectID != nil && *op.ExternalObjectID == "191"
		}
	}
	if !found || r.ActiveStep != "" {
		t.Fatal("operation not durably completed")
	}
	encoded, _ := json.Marshal(r)
	if strings.Contains(string(encoded), "controlled-gitlab-test-secret") {
		t.Fatal("secret reached durable run")
	}
}

func TestControlGitLabCreateObservedOnceAndSchedulerContinues(t *testing.T) {
	m := &controlGitLabMock{}
	s, x, id := controlGitLabFixture(t, m)
	r, err := s.ExecuteControlGitLab(context.Background(), id, x)
	if err != nil {
		t.Fatal(err)
	}
	assertControlCreateResult(t, r, x, "succeeded")
	if m.posts != 1 || m.gets != 3 || m.requestProblem != "" {
		t.Fatal("actual transport calls", m.posts, m.gets, m.requestProblem)
	}
	next, err := NextControlAction(r, r.Control.Schedule)
	if err != nil || next.Action == nil {
		t.Fatal("scheduler cannot consume observed create", err)
	}
	kind, _, _ := scheduleAction(*next.Action)
	if kind != "push_repository" {
		t.Fatal("wrong successor", kind)
	}
	if _, err = s.ExecuteControlGitLab(context.Background(), id, x); err != nil || m.posts != 1 || m.gets != 3 {
		t.Fatal("verified create repeated transport", err)
	}
	if _, err = s.ReconcileControlGitLab(context.Background(), id, x); err != nil || m.posts != 1 || m.gets != 3 {
		t.Fatal("verified reconciliation repeated transport", err)
	}
	if r.Control.Audit.Budget.Totals.Consumed.ReconciliationRead != 3 {
		t.Fatal("unmetered read")
	}
}

func TestControlGitLabLostCreateResponseReopensAndOnlyReadsOriginalIdentity(t *testing.T) {
	m := &controlGitLabMock{lostResponse: true}
	s, x, id := controlGitLabFixture(t, m)
	r, err := s.ExecuteControlGitLab(context.Background(), id, x)
	if err == nil || r.State != "BLOCKED" || m.posts != 1 || len(r.Control.Schedule.Results) != 0 {
		t.Fatal("lost response did not preserve unknown", err)
	}
	window := r.Control.Audit.Budget.ReconciliationWindows[0]
	if _, err = s.ExecuteControlGitLab(context.Background(), id, x); err == nil || m.posts != 1 {
		t.Fatal("unknown write replayed")
	}
	cfg := s.Config
	s.Close()
	reopened, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(reopened.Close)
	r, err = reopened.ReconcileControlGitLab(context.Background(), id, x)
	if err != nil {
		t.Fatal(err)
	}
	assertControlCreateResult(t, r, x, "unknown")
	if r.State != "BLOCKED" || r.ResumeTo != "DELIVER" || m.posts != 1 || m.gets != 3 || r.Control.Audit.Budget.ReconciliationWindows[0] != window {
		t.Fatal("reconciliation changed authority, identity, deadline or POST count")
	}
	if len(r.Control.Audit.Executions) != 3 {
		t.Fatal("recovery manufactured new action advice")
	}
}

func TestControlGitLabPreflightFailureNeverWrites(t *testing.T) {
	for _, status := range []int{401, 403, 429, 500} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			m := &controlGitLabMock{readStatus: status}
			s, x, id := controlGitLabFixture(t, m)
			r, err := s.ExecuteControlGitLab(context.Background(), id, x)
			if err == nil || m.posts != 0 || r.State != "BLOCKED" {
				t.Fatal("failed lookup led to write", err)
			}
			if _, err = controlGitLabAttempt(r, x); err == nil {
				t.Fatal("preflight issued a write attempt")
			}
		})
	}
	t.Run("namespace_identity", func(t *testing.T) {
		m := &controlGitLabMock{namespaceID: 18}
		s, x, id := controlGitLabFixture(t, m)
		if _, err := s.ExecuteControlGitLab(context.Background(), id, x); err == nil || m.posts != 0 || m.gets != 1 {
			t.Fatal("namespace mismatch created project")
		}
	})
	t.Run("existing_foreign_project", func(t *testing.T) {
		m := &controlGitLabMock{project: &controlGitLabProject{ID: 191, Description: "someone else's project"}}
		s, x, id := controlGitLabFixture(t, m)
		if _, err := s.ExecuteControlGitLab(context.Background(), id, x); err == nil || m.posts != 0 {
			t.Fatal("preexisting project adopted or mutated")
		}
	})
}

func TestControlGitLabObservationMismatchRemainsUnknown(t *testing.T) {
	m := &controlGitLabMock{postMutation: func(p *controlGitLabProject) { p.Description += "_wrong" }}
	s, x, id := controlGitLabFixture(t, m)
	if _, err := s.ExecuteControlGitLab(context.Background(), id, x); err == nil {
		t.Fatal("2xx response without accurate ownership must remain unknown")
	}
	original := *m.project
	original.Description = controlGitLabMarker(x)
	for name, mutate := range map[string]func(*controlGitLabProject){
		"namespace":          func(p *controlGitLabProject) { p.Namespace.ID = 18 },
		"path":               func(p *controlGitLabProject) { p.PathWithNamespace = "another/create-service" },
		"visibility":         func(p *controlGitLabProject) { p.Visibility = "public" },
		"marker":             func(p *controlGitLabProject) { p.Description += "different" },
		"nonempty":           func(p *controlGitLabProject) { no := false; p.EmptyRepo = &no },
		"missing_empty_repo": func(p *controlGitLabProject) { p.EmptyRepo = nil },
		"branch":             func(p *controlGitLabProject) { branch := "other"; p.DefaultBranch = &branch },
	} {
		t.Run(name, func(t *testing.T) {
			p := original
			mutate(&p)
			m.mu.Lock()
			m.project = &p
			m.mu.Unlock()
			r, err := s.ReconcileControlGitLab(context.Background(), id, x)
			if err == nil || r.State != "BLOCKED" || len(r.Control.Schedule.Results) != 0 || m.posts != 1 {
				t.Fatal("foreign or incomplete observation accepted", err)
			}
		})
	}
	m.mu.Lock()
	m.project, m.readStatus = &original, http.StatusNotFound
	m.mu.Unlock()
	if _, err := s.ReconcileControlGitLab(context.Background(), id, x); err == nil || m.posts != 1 {
		t.Fatal("absence allowed another write")
	}
}

func TestControlGitLabReservationDeadlineIsAbsolute(t *testing.T) {
	end := time.Now().UTC().Add(50 * time.Millisecond).Format(time.RFC3339Nano)
	reservation := contracts.BudgetReservation{DeadlineAt: &end}
	ctx, cancel, err := controlGitLabDeadline(context.Background(), reservation)
	if err != nil {
		t.Fatal(err)
	}
	defer cancel()
	deadline, ok := ctx.Deadline()
	if !ok || deadline.Format(time.RFC3339Nano) != end {
		t.Fatal("helper extended persisted deadline")
	}
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("attempt did not honor original deadline")
	}
	if _, _, err = controlGitLabDeadline(context.Background(), reservation); err == nil {
		t.Fatal("retry reset expired deadline")
	}
	if _, _, err = controlGitLabDeadline(context.Background(), contracts.BudgetReservation{}); err == nil {
		t.Fatal("missing deadline accepted")
	}
}

func TestControlGitLabOriginalSourceReconciliationDoesNotReviveTerminalRun(t *testing.T) {
	m := &controlGitLabMock{lostResponse: true}
	s, x, id := controlGitLabFixture(t, m)
	if _, err := s.ExecuteControlGitLab(context.Background(), id, x); err == nil {
		t.Fatal("expected lost response")
	}
	_, err := s.Store.Update(id, func(r *Run) error { sourceChanged(r); r.State = "CANCELLED"; r.CancelRequested = true; return nil })
	if err != nil {
		t.Fatal(err)
	}
	r, err := s.ReconcileControlGitLab(context.Background(), id, x)
	if err != nil {
		t.Fatal(err)
	}
	assertControlCreateResult(t, r, x, "unknown")
	if r.State != "CANCELLED" || !r.CancelRequested || int64(r.SourceVersion) != x.SourceVersion+1 || m.posts != 1 {
		t.Fatal("old result revived terminated authority")
	}
	last := r.Control.Audit.Budget.Reservations[len(r.Control.Audit.Budget.Reservations)-1]
	if last.Kind != "reconciliation_read" || last.SourceVersion != x.SourceVersion || last.OperationID == nil || *last.OperationID != x.OperationID {
		t.Fatal("historical read rebound to current source")
	}
}

func TestControlGitLabRejectsUnregisteredAlteredAndUnavailableActionBeforeIO(t *testing.T) {
	m := &controlGitLabMock{}
	s, x, id := controlGitLabFixture(t, m)
	for _, mutate := range []func(*contracts.ExecutionRequest){
		func(x *contracts.ExecutionRequest) { x.ExecutionID += "_forged" },
		func(x *contracts.ExecutionRequest) { x.RunID = "another_run" },
		func(x *contracts.ExecutionRequest) {
			x.Action = contracts.CandidateAction(`{"action_type":"package","parameters":{"manifest_ref":{"object_type":"manifest","object_id":"manifest_fake","object_version":1,"source_version":2,"hash":"` + strings.Repeat("0", 64) + `"}}}`)
		},
	} {
		copy := x
		mutate(&copy)
		copy.RequestHash, _ = contracts.HashObject("ExecutionRequest", copy)
		if _, err := s.ExecuteControlGitLab(context.Background(), id, copy); err == nil || m.gets != 0 || m.posts != 0 {
			t.Fatal("forged request reached transport")
		}
	}
	originalURL := s.Config.GitLabURL
	s.Config.GitLabURL = "https://changed.invalid"
	if _, err := s.ExecuteControlGitLab(context.Background(), id, x); err == nil || m.gets != 0 || m.posts != 0 {
		t.Fatal("changed transport target authorized")
	}
	s.Config.GitLabURL = originalURL
	_, err := s.Store.Update(id, func(r *Run) error {
		p := *r.Control.Audit.PermissionSnapshot
		p.PermissionVersion++
		p.Revoked = true
		p.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
		p.PermissionHash = chainRehash(t, "PermissionSnapshot", p)
		r.Control.Audit.PermissionSnapshot = &p
		r.Control.Audit.PermissionHistory = append(r.Control.Audit.PermissionHistory, p)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.ExecuteControlGitLab(context.Background(), id, x); err == nil || m.gets != 0 || m.posts != 0 {
		t.Fatal("revoked permission reached network")
	}
}

func TestControlGitLabCrashAfterSendSettlesUnknownBeforeRead(t *testing.T) {
	m := &controlGitLabMock{}
	s, x, id := controlGitLabFixture(t, m)
	if _, _, err := s.beginControlExecution(id, x); err != nil {
		t.Fatal(err)
	}
	// Simulate the external send and process loss between persisted admission
	// and any product settlement. No synthetic success is inserted in the Run.
	_, err := s.gitlab(context.Background(), http.MethodPost, "/projects", map[string]any{"namespace_id": "17", "name": "create-service", "path": "create-service", "visibility": "private", "description": controlGitLabMarker(x), "initialize_with_readme": false}, nil)
	if err != nil {
		t.Fatal(err)
	}
	cfg := s.Config
	s.Close()
	reopened, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(reopened.Close)
	r, err := reopened.ReconcileControlGitLab(context.Background(), id, x)
	if err != nil {
		t.Fatal(err)
	}
	assertControlCreateResult(t, r, x, "unknown")
	if m.posts != 1 || m.gets != 1 || r.State != "BLOCKED" || r.Control.Audit.Budget.Totals.Reserved.Action != 0 {
		t.Fatal("crash recovery replayed write or lost unsettled charge")
	}
}
