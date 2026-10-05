// GENERATED complete runtime publication. Validate every boundary.
import type * as C from './types.generated'

export type RuntimeCorrelation = { "command_id": (C.Id | null); "operation_id": (C.Id | null); "approval_id": (C.Id | null); "execution_id": (C.Id | null); "workflow_id": (string | null); "execution_run_id": (string | null); "activity_id": (string | null); "attempt": (number | null); "attempt_id": (C.Id | null) }

export type RuntimeParameters = { "action": string; "argument_keys": Array<string>; "summary_hash": C.Hash }

export type RuntimeAuditEvent = { "runtime_version": "1.0.0"; "event_id": C.Id; "run_id": C.Id; "sequence": number; "source_version": number; "state_sequence": number; "definition_snapshot_hash": C.Hash; "kind": ("run_created" | "business_input" | "command_accepted" | "command_processed" | "command_rejected" | "command_result_unknown" | "decision_proposed" | "authorization" | "approval" | "operation" | "verification" | "run_terminal" | "activity_started" | "activity_finished" | "recovery"); "step": string; "correlation": RuntimeCorrelation; "parameters": RuntimeParameters; "outcome": string; "elapsed_ms": (number | null); "evidence_refs": Array<C.ObjectRef>; "recorded_at": C.Timestamp; "previous_hash": (C.Hash | null); "event_hash": C.Hash }

export type RuntimeActivityAttempt = { "attempt_id": C.Id; "run_id": C.Id; "mode": C.ExecutionMode; "activity_id": string; "activity_type": ("ProcessCommand" | "ExecuteStep"); "step": string; "command_id": (C.Id | null); "workflow_id": (string | null); "execution_run_id": (string | null); "attempt": number; "started_at": C.Timestamp; "completed_at": (C.Timestamp | null); "status": ("running" | "succeeded" | "failed" | "unknown"); "error_code": (string | null); "elapsed_ms": (number | null); "last_heartbeat_at": (C.Timestamp | null); "heartbeat_source": ("process" | "temporal" | "unavailable"); "heartbeat_status": ("fresh" | "stale" | "unavailable" | "not_applicable") }

export type RuntimeCheckpoint = { "availability": ("available" | "stale" | "unavailable" | "not_applicable"); "reason": string; "sampled_at": C.Timestamp; "checkpoint_at": (C.Timestamp | null); "workflow_id": (string | null); "execution_run_id": (string | null); "state": (C.RunState | null); "source_version": (number | null); "state_sequence": (number | null); "commands_processed": (number | null) }

export type RuntimeMetrics = { "availability": ("complete" | "unavailable"); "successful_runs": (number | null); "failed_runs": (number | null); "activity_retries": (number | null); "verification_failures": (number | null); "effective_business_rounds": (number | null); "total_elapsed_ms": (number | null); "first_terminal_at": (C.Timestamp | null); "first_terminal_state": (C.RunState | null); "as_of": C.Timestamp; "basis": "first_terminal_actual_attempt_distinct_report_accepted_requirement_v1" }

export type RuntimeFailureDiagnosis = { "code": string; "category": ("input" | "model_protocol" | "authorization" | "tool" | "infrastructure" | "business_verification" | "unknown_result" | "budget" | "user_cancelled"); "step": string; "command_id": (C.Id | null); "operation_id": (C.Id | null); "affected_refs": Array<C.ObjectRef>; "retryable": boolean; "missing_requirements": Array<string>; "suggested_action": ("correct_input" | "revise_and_reapprove" | "restore_dependency" | "query_original_result" | "new_run" | "none"); "reconciliation_url": (string | null); "message": string; "recorded_at": C.Timestamp }

export type RuntimeAuditTrail = { "runtime_version": "1.0.0"; "run_id": C.Id; "run_ref": C.ObjectRef; "coverage": ("complete" | "unavailable"); "events": Array<RuntimeAuditEvent>; "head_hash": (C.Hash | null); "audit_hash": C.Hash; "observed_at": C.Timestamp }

export type RuntimeBudgetStatus = { "policy": C.BudgetPolicy; "ledger_ref": (C.ObjectRef | null); "execution_deadline": (C.Timestamp | null); "context_measurement": "conservative_utf8_byte_bound_plus_frozen_output_and_protocol_reserves" }

export type RuntimeView = { "runtime_version": "1.0.0"; "run_id": C.Id; "run_ref": C.ObjectRef; "modes": C.Modes; "observed_at": C.Timestamp; "checkpoint": RuntimeCheckpoint; "activity_attempts": Array<RuntimeActivityAttempt>; "metrics": RuntimeMetrics; "diagnosis": (RuntimeFailureDiagnosis | null); "budget": RuntimeBudgetStatus; "audit_coverage": ("complete" | "unavailable"); "audit_head_hash": (C.Hash | null); "view_hash": C.Hash }

export type RuntimeHealthCheck = { "name": ("api" | "storage" | "temporal" | "worker" | "toolchain" | "sandbox" | "model_configuration" | "gitlab_configuration"); "status": ("ready" | "not_ready" | "not_applicable" | "configured_unverified"); "reason": string; "checked_at": C.Timestamp; "affected_capabilities": Array<string> }

export type RuntimeHealth = { "runtime_version": "1.0.0"; "live": boolean; "ready": boolean; "stopping": boolean; "instance_id": C.Id; "execution_mode": C.ExecutionMode; "storage_ownership": "single_writer_locked_directory"; "checked_at": C.Timestamp; "checks": Array<RuntimeHealthCheck>; "affected_run_ids": Array<C.Id>; "health_hash": C.Hash }

export type RuntimeServiceMetrics = { "runtime_version": "1.0.0"; "observed_at": C.Timestamp; "included_run_ids": Array<C.Id>; "excluded_run_ids": Array<C.Id>; "successful_runs": number; "failed_runs": number; "activity_retries": number; "verification_failures": number; "effective_business_rounds": number; "total_elapsed_ms": number; "metrics_hash": C.Hash }
