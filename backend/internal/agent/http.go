package agent

import (
	"agentstudio/internal/contracts"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.temporal.io/sdk/temporal"
)

func (s *Service) Register(mux *http.ServeMux) {
	s.registerRuntime(mux)
	s.registerDefinitionRoutes(mux)
	s.registerControlRoutes(mux)
	s.registerV2Routes(mux)
	mux.HandleFunc("GET /api/bootstrap", func(w http.ResponseWriter, r *http.Request) {
		// Bootstrap previews the publication even when model identity is absent;
		// it is not a Run binding and never changes the configured decision mode.
		preview := CreateOptions{}
		if s.Config.DecisionMode == "model" && s.Config.AIEndpoint != "" && s.Config.AIModel != "" {
			preview.DecisionMode = "model"
		}
		frozen, e := s.freezeDefinition(preview)
		if e != nil {
			writeError(w, e)
			return
		}
		writeJSON(w, 200, map[string]any{"mode": s.Config.Mode, "execution_mode": s.Config.Mode, "decision_mode": s.Config.DecisionMode, "delivery_mode": s.Config.DeliveryMode, "provider": s.Provider(), "model_configured": s.ModelConfigured(), "gitlab_base_url": s.Config.GitLabURL, "gitlab_configured": s.Config.GitLabToken != "" && s.Config.GitLabNamespace != "", "temporal_connected": s.Temporal != nil, "approval_ttl_seconds": int(s.Config.ApprovalTTL / time.Second), "skills": Skills, "definition": legacyDefinition(frozen)})
	})
	mux.HandleFunc("GET /api/runs", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, s.Store.List()) })
	mux.HandleFunc("POST /api/runs", func(w http.ResponseWriter, r *http.Request) {
		var body CreateOptions
		if e := decode(r, &body); e != nil {
			writeError(w, e)
			return
		}
		run, e := s.CreateWithOptions(r.Context(), body)
		if e != nil {
			writeError(w, e)
			return
		}
		writeJSON(w, 201, run)
	})
	mux.HandleFunc("GET /api/runs/{id}", func(w http.ResponseWriter, r *http.Request) {
		run, e := s.Store.Get(r.PathValue("id"))
		if e != nil {
			writeError(w, e)
			return
		}
		writeJSON(w, 200, run)
	})
	mux.HandleFunc("GET /api/runs/{id}/workflow", func(w http.ResponseWriter, r *http.Request) {
		run, e := s.Store.Get(r.PathValue("id"))
		if e != nil {
			writeError(w, e)
			return
		}
		if run.ExecutionMode != "temporal" {
			writeJSON(w, 200, map[string]any{"applicable": false, "execution_mode": run.ExecutionMode})
			return
		}
		if s.Temporal == nil {
			writeError(w, fail("temporal_unavailable", "Temporal未连接"))
			return
		}
		desc, e := s.Temporal.DescribeWorkflowExecution(r.Context(), workflowID(run.ID), "")
		if e != nil {
			writeError(w, fail("workflow_unavailable", "Workflow不可查询"))
			return
		}
		query, e := s.Temporal.QueryWorkflow(r.Context(), workflowID(run.ID), "", "get_status")
		if e != nil {
			writeError(w, fail("workflow_unavailable", "Workflow Query不可用"))
			return
		}
		var status map[string]any
		if e = query.Get(&status); e != nil {
			writeError(w, e)
			return
		}
		writeJSON(w, 200, map[string]any{"applicable": true, "execution_mode": "temporal", "workflow_id": desc.WorkflowExecutionInfo.Execution.WorkflowId, "execution_run_id": desc.WorkflowExecutionInfo.Execution.RunId, "query": status, "authoritative_state": run.State, "source_version": run.SourceVersion, "state_sequence": run.StateSequence})
	})
	mux.HandleFunc("POST /api/runs/{id}/messages", func(w http.ResponseWriter, r *http.Request) {
		var cmd Command
		if e := decode(r, &cmd); e != nil {
			writeError(w, e)
			return
		}
		if cmd.Action != "" {
			writeError(w, fail("invalid_message", "use the actions endpoint for operations"))
			return
		}
		cmd.RunID = r.PathValue("id")
		run, e := s.Submit(r.Context(), cmd)
		if e != nil {
			writeError(w, e)
			return
		}
		writeJSON(w, 200, run)
	})
	mux.HandleFunc("POST /api/runs/{id}/actions", func(w http.ResponseWriter, r *http.Request) {
		var cmd Command
		if e := decode(r, &cmd); e != nil {
			writeError(w, e)
			return
		}
		if cmd.Action == "" {
			writeError(w, fail("invalid_action", "action is required"))
			return
		}
		cmd.RunID = r.PathValue("id")
		run, e := s.Submit(r.Context(), cmd)
		if e != nil {
			writeError(w, e)
			return
		}
		writeJSON(w, 200, run)
	})
	mux.HandleFunc("GET /api/runs/{id}/events", s.events)
	mux.HandleFunc("GET /api/runs/{id}/artifacts", s.artifact)
	mux.HandleFunc("GET /api/runs/{id}/download", func(w http.ResponseWriter, r *http.Request) {
		run, e := s.Store.Get(r.PathValue("id"))
		if e != nil {
			writeError(w, e)
			return
		}
		p := filepath.Join(s.Config.DataDir, "downloads", run.ID+".zip")
		if _, e = os.Stat(p); e != nil {
			writeError(w, fail("artifact_not_ready", "ZIP 尚未生成"))
			return
		}
		if e = s.verifyManifest(run); e != nil {
			writeError(w, e)
			return
		}
		if e = s.verifyPackage(run); e != nil {
			writeError(w, e)
			return
		}
		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s.zip\"", run.Plan.ProjectName))
		http.ServeFile(w, r, p)
	})
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{"ok": true, "mode": s.Config.Mode, "temporal_connected": s.Temporal != nil})
	})
}
func decode(r *http.Request, target any) error {
	body, err := io.ReadAll(io.LimitReader(r.Body, (64<<10)+1))
	if err != nil {
		return fail("invalid_json", "invalid JSON request")
	}
	if len(body) > 64<<10 {
		return fail("request_too_large", "request exceeds byte limit")
	}
	if _, err = contracts.ParseJSON(body); err != nil {
		return fail("invalid_json", "invalid JSON request")
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if e := decoder.Decode(target); e != nil {
		return fail("invalid_json", "invalid JSON request")
	}
	var extra any
	if e := decoder.Decode(&extra); e != io.EOF {
		return fail("invalid_json", "request must contain a single JSON object")
	}
	return nil
}
func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func errorStatus(code string) int {
	if code == "request_too_large" {
		return 413
	}
	if strings.Contains(code, "unavailable") || strings.Contains(code, "workflow_update") {
		return 503
	}
	if code == "budget_exceeded" {
		return 429
	}
	switch code {
	case "model_config_changed", "run_mode_mismatch", "stale_proposal", "proposal_expired", "snapshot_mismatch", "definition_mismatch", "target_changed", "invalid_state", "terminal_run", "idempotency_conflict", "proposal_closed", "run_busy", "operation_conflict", "branch_changed":
		return 409
	case "internal_error", "operation_failed":
		return 500
	}
	return 400
}
func writeError(w http.ResponseWriter, e error) {
	status, code, message := 500, "internal_error", "服务端处理失败"
	var api *APIError
	if errors.Is(e, ErrNotFound) {
		status, code, message = 404, "not_found", "Run不存在"
	} else if errors.As(e, &api) {
		code, message = api.Code, api.Message
		status = errorStatus(code)
	} else {
		var app *temporal.ApplicationError
		if errors.As(e, &app) {
			code, message = app.Type(), app.Message()
			status = errorStatus(code)
		}
	}
	writeJSON(w, status, map[string]string{"error": message, "code": code})
}
func (s *Service) events(w http.ResponseWriter, r *http.Request) {
	ch, cancel, e := s.Store.Subscribe(r.PathValue("id"))
	if e != nil {
		writeError(w, e)
		return
	}
	defer cancel()
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, fail("stream_unsupported", "streaming unsupported"))
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case run := <-ch:
			b, _ := json.Marshal(run)
			fmt.Fprintf(w, "event: snapshot\nid: %s-%d\ndata: %s\n\n", run.ID, run.StateSequence, b)
			flusher.Flush()
		case <-ticker.C:
			fmt.Fprint(w, ": keepalive\n\n")
			flusher.Flush()
		}
	}
}
func (s *Service) artifact(w http.ResponseWriter, r *http.Request) {
	run, e := s.Store.Get(r.PathValue("id"))
	if e != nil {
		writeError(w, e)
		return
	}
	p := r.URL.Query().Get("path")
	if p == "" {
		writeJSON(w, 200, run.Artifacts)
		return
	}
	if run.Workspace == "" || filepath.IsAbs(p) || strings.Contains(p, "\\") || !within(run.Workspace, filepath.Join(run.Workspace, p)) {
		writeError(w, fail("invalid_path", "artifact path outside workspace"))
		return
	}
	found := false
	expectedHash := ""
	for _, a := range run.Artifacts {
		if a.Path == p {
			found = true
			expectedHash = a.SHA256
		}
	}
	if !found {
		writeError(w, fail("artifact_not_found", "artifact is not registered"))
		return
	}
	path := filepath.Join(run.Workspace, p)
	if e = safeWorkspacePath(run.Workspace, path); e != nil {
		writeError(w, e)
		return
	}
	info, e := os.Lstat(path)
	if e != nil {
		writeError(w, e)
		return
	}
	if info.Mode()&os.ModeSymlink != 0 || info.Size() > 1<<20 {
		writeError(w, fail("invalid_artifact", "artifact must be a regular text file up to 1 MiB"))
		return
	}
	b, e := os.ReadFile(path)
	if e != nil {
		writeError(w, e)
		return
	}
	if expectedHash == "" || bytesDigest(b) != expectedHash {
		writeError(w, fail("artifact_changed", "工件内容与已登记hash不一致，拒绝展示为可信工件"))
		return
	}
	writeJSON(w, 200, map[string]any{"path": p, "content": string(b), "size": len(b), "sha256": expectedHash})
}
func CORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if strings.HasPrefix(origin, "http://localhost:") || strings.HasPrefix(origin, "http://127.0.0.1:") {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
		}
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Last-Event-ID")
		w.Header().Set("Access-Control-Expose-Headers", "X-Manifest-Hash, X-Artifact-SHA256, X-Source-Version")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		if r.Method == "OPTIONS" {
			w.WriteHeader(204)
			return
		}
		next.ServeHTTP(w, r)
	})
}
