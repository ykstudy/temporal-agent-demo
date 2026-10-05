package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"agentstudio/internal/contracts"
	"agentstudio/internal/storage"
	enumspb "go.temporal.io/api/enums/v1"
	"go.temporal.io/api/workflowservice/v1"
	"go.temporal.io/sdk/client"
)

// Probe only service-owned metadata and temporary bytes. It never invokes a
// provider or a project tool, and it does not change a Run or its sequence.
func (s *Store) runtimeStorageProbe() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.lockFile == nil || s.objects == nil {
		return fmt.Errorf("storage_closed")
	}
	for _, sub := range []string{"", "runs", "objects", "definitions", "workspaces", "downloads"} {
		path := filepath.Join(s.dir, sub)
		info, err := os.Lstat(path)
		if os.IsNotExist(err) && sub != "" && sub != "runs" && sub != "objects" {
			continue
		}
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0222 == 0 {
			return fmt.Errorf("storage_directory_unavailable")
		}
		f, err := os.CreateTemp(path, ".readiness-")
		if err != nil {
			return fmt.Errorf("storage_write_failed")
		}
		name := f.Name()
		data := []byte("agentstudio-readiness-v1\n")
		_, err = f.Write(data)
		if err == nil {
			err = f.Sync()
		}
		closeErr := f.Close()
		if err == nil {
			err = closeErr
		}
		if err == nil {
			actual, e := os.ReadFile(name)
			if e != nil || !bytes.Equal(data, actual) {
				err = fmt.Errorf("probe read mismatch")
			}
		}
		removeErr := os.Remove(name)
		if err == nil {
			err = removeErr
		}
		if err == nil {
			err = syncDirectory(path)
		}
		if err != nil {
			return fmt.Errorf("storage_probe_failed")
		}
	}
	for _, r := range s.runs {
		path := filepath.Join(s.dir, "runs", r.ID+".json")
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() {
			return fmt.Errorf("snapshot_unavailable")
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("snapshot_unreadable")
		}
		expected, err := json.MarshalIndent(disk(r), "", "  ")
		if err != nil || !bytes.Equal(raw, expected) {
			return fmt.Errorf("snapshot_integrity_failed")
		}
		if conversationState(r) != nil {
			if _, err = storage.ReadTranscript(s.transcriptPath(r.ID), r.Control.Audit.Messages); err != nil {
				return fmt.Errorf("transcript_integrity_failed")
			}
		}
	}
	return nil
}

func (s *Service) RuntimeHealth(ctx context.Context, probe bool) (contracts.RuntimeHealth, error) {
	stamp := now()
	s.runtime.mu.Lock()
	stopping, workerUp, auditFault := s.runtime.stopping, s.runtime.worker, s.runtime.auditFault
	s.runtime.mu.Unlock()
	h := contracts.RuntimeHealth{RuntimeVersion: "1.0.0", Live: true, Ready: probe && !stopping && !auditFault, Stopping: stopping, InstanceID: s.RuntimeIdentity(), ExecutionMode: s.Config.Mode, StorageOwnership: "single_writer_locked_directory", CheckedAt: stamp, Checks: []contracts.RuntimeHealthCheck{}, AffectedRunIDs: []string{}}
	add := func(name, status, reason string, required bool, capabilities ...string) {
		if capabilities == nil {
			capabilities = []string{}
		}
		if required && status == "not_ready" {
			h.Ready = false
		}
		h.Checks = append(h.Checks, contracts.RuntimeHealthCheck{Name: name, Status: status, Reason: reason, CheckedAt: now(), AffectedCapabilities: capabilities})
	}
	status, reason := "ready", "http_responsive"
	if stopping {
		status, reason = "not_ready", "service_stopping"
	}
	add("api", status, reason, true, "command_admission")
	if !probe {
		for _, name := range []string{"storage", "temporal", "worker", "toolchain", "sandbox", "model_configuration", "gitlab_configuration"} {
			add(name, "not_applicable", "liveness_does_not_probe_dependencies", false)
		}
	} else {
		status, reason = "ready", "bounded_fsync_read_write_and_snapshot_transcript_verified"
		if err := s.Store.runtimeStorageProbe(); err != nil {
			status, reason = "not_ready", err.Error()
		}
		if status == "ready" {
			for _, r := range s.Store.List() {
				if err := s.verifyFrozenDefinition(r); err != nil {
					status, reason = "not_ready", "frozen_dependency_unavailable"
					break
				}
			}
		}
		if auditFault {
			status, reason = "not_ready", "runtime_audit_write_failed_restart_after_repair"
		}
		add("storage", status, reason, true, "durable_commands", "audit", "artifact_delivery")
		if s.Config.Mode == "local" {
			add("temporal", "not_applicable", "explicit_local_execution", false)
			add("worker", "not_applicable", "in_process_local_executor", false)
		} else {
			actual, pollers := false, false
			if s.Temporal != nil {
				checkCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
				_, e := s.Temporal.CheckHealth(checkCtx, &client.CheckHealthRequest{})
				if e == nil {
					_, e = s.Temporal.WorkflowService().DescribeNamespace(checkCtx, &workflowservice.DescribeNamespaceRequest{Namespace: s.Config.TemporalNamespace})
				}
				actual = e == nil
				if actual && workerUp {
					pollers = true
					for _, kind := range []enumspb.TaskQueueType{enumspb.TASK_QUEUE_TYPE_WORKFLOW, enumspb.TASK_QUEUE_TYPE_ACTIVITY} {
						q, e := s.Temporal.DescribeTaskQueue(checkCtx, s.Config.TaskQueue, kind)
						found := false
						if e == nil {
							for _, p := range q.Pollers {
								if p.Identity == s.RuntimeIdentity() && p.LastAccessTime != nil && time.Since(p.LastAccessTime.AsTime()) < 90*time.Second {
									found = true
								}
							}
						}
						pollers = pollers && found
					}
				}
				cancel()
			}
			status, reason = "ready", "temporal_health_and_namespace_verified"
			if !actual {
				status, reason = "not_ready", "temporal_health_or_namespace_unavailable"
			}
			add("temporal", status, reason, true, "workflow_updates", "workflow_queries")
			status, reason = "ready", "own_worker_running_and_both_recent_pollers_verified"
			if !workerUp {
				status, reason = "not_ready", "own_worker_stopped"
			} else if !pollers {
				status, reason = "not_ready", "own_worker_pollers_unavailable_or_stale"
			}
			add("worker", status, reason, true, "activity_execution", "workflow_queries")
		}
		status, reason = "ready", "go_binary_available"
		if _, err := exec.LookPath("go"); err != nil {
			status, reason = "not_ready", "go_binary_missing"
		}
		add("toolchain", status, reason, true, "project_verification")
		sandbox := ""
		switch runtime.GOOS {
		case "darwin":
			sandbox = "/usr/bin/sandbox-exec"
		case "linux":
			sandbox = "bwrap"
		}
		status, reason = "ready", "sandbox_binary_available_unexecuted"
		if sandbox == "" {
			status, reason = "not_ready", "unsupported_sandbox_platform"
		} else if _, err := exec.LookPath(sandbox); err != nil {
			status, reason = "not_ready", "sandbox_binary_missing"
		}
		add("sandbox", status, reason, true, "isolated_tools")
		status, reason = "configured_unverified", "configuration_present_external_provider_not_probed"
		if !s.ModelConfigured() {
			status, reason = "not_ready", "model_configuration_missing"
		}
		add("model_configuration", status, reason, s.Config.DecisionMode == "model", "model_decisions")
		status, reason = "configured_unverified", "configuration_present_gitlab_and_runner_not_probed"
		if s.Config.GitLabToken == "" || s.Config.GitLabNamespace == "" {
			status, reason = "not_ready", "gitlab_configuration_missing"
		}
		add("gitlab_configuration", status, reason, s.Config.DeliveryMode == "gitlab", "gitlab_delivery")
	}
	if probe && !h.Ready {
		for _, r := range s.Store.List() {
			if !terminal(r.State) {
				h.AffectedRunIDs = append(h.AffectedRunIDs, r.ID)
			}
		}
	}
	var err error
	h.HealthHash, err = contracts.HashObject("RuntimeHealth", h)
	if err == nil {
		err = contracts.ValidateRuntime("RuntimeHealth", h)
	}
	return h, err
}
func (s *Service) RuntimeMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			if err := s.acceptingWork(); err != nil {
				writeV2Error(w, r, err)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}
