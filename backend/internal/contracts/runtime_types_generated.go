// Code generated from runtime/1.0.0/runtime.schema.json; DO NOT EDIT.
package contracts

type RuntimeCorrelation struct {
	CommandID      *Id     `json:"command_id"`
	OperationID    *Id     `json:"operation_id"`
	ApprovalID     *Id     `json:"approval_id"`
	ExecutionID    *Id     `json:"execution_id"`
	WorkflowID     *string `json:"workflow_id"`
	ExecutionRunID *string `json:"execution_run_id"`
	ActivityID     *string `json:"activity_id"`
	Attempt        *int64  `json:"attempt"`
	AttemptID      *Id     `json:"attempt_id"`
}

type RuntimeParameters struct {
	Action       string   `json:"action"`
	ArgumentKeys []string `json:"argument_keys"`
	SummaryHash  Hash     `json:"summary_hash"`
}

type RuntimeAuditEvent struct {
	RuntimeVersion         string             `json:"runtime_version"`
	EventID                Id                 `json:"event_id"`
	RunID                  Id                 `json:"run_id"`
	Sequence               int64              `json:"sequence"`
	SourceVersion          int64              `json:"source_version"`
	StateSequence          int64              `json:"state_sequence"`
	DefinitionSnapshotHash Hash               `json:"definition_snapshot_hash"`
	Kind                   string             `json:"kind"`
	Step                   string             `json:"step"`
	Correlation            RuntimeCorrelation `json:"correlation"`
	Parameters             RuntimeParameters  `json:"parameters"`
	Outcome                string             `json:"outcome"`
	ElapsedMS              *int64             `json:"elapsed_ms"`
	EvidenceRefs           []ObjectRef        `json:"evidence_refs"`
	RecordedAt             Timestamp          `json:"recorded_at"`
	PreviousHash           *Hash              `json:"previous_hash"`
	EventHash              Hash               `json:"event_hash"`
}

type RuntimeActivityAttempt struct {
	AttemptID       Id            `json:"attempt_id"`
	RunID           Id            `json:"run_id"`
	Mode            ExecutionMode `json:"mode"`
	ActivityID      string        `json:"activity_id"`
	ActivityType    string        `json:"activity_type"`
	Step            string        `json:"step"`
	CommandID       *Id           `json:"command_id"`
	WorkflowID      *string       `json:"workflow_id"`
	ExecutionRunID  *string       `json:"execution_run_id"`
	Attempt         int64         `json:"attempt"`
	StartedAt       Timestamp     `json:"started_at"`
	CompletedAt     *Timestamp    `json:"completed_at"`
	Status          string        `json:"status"`
	ErrorCode       *string       `json:"error_code"`
	ElapsedMS       *int64        `json:"elapsed_ms"`
	LastHeartbeatAt *Timestamp    `json:"last_heartbeat_at"`
	HeartbeatSource string        `json:"heartbeat_source"`
	HeartbeatStatus string        `json:"heartbeat_status"`
}

type RuntimeCheckpoint struct {
	Availability      string     `json:"availability"`
	Reason            string     `json:"reason"`
	SampledAt         Timestamp  `json:"sampled_at"`
	CheckpointAt      *Timestamp `json:"checkpoint_at"`
	WorkflowID        *string    `json:"workflow_id"`
	ExecutionRunID    *string    `json:"execution_run_id"`
	State             *RunState  `json:"state"`
	SourceVersion     *int64     `json:"source_version"`
	StateSequence     *int64     `json:"state_sequence"`
	CommandsProcessed *int64     `json:"commands_processed"`
}

type RuntimeMetrics struct {
	Availability            string     `json:"availability"`
	SuccessfulRuns          *int64     `json:"successful_runs"`
	FailedRuns              *int64     `json:"failed_runs"`
	ActivityRetries         *int64     `json:"activity_retries"`
	VerificationFailures    *int64     `json:"verification_failures"`
	EffectiveBusinessRounds *int64     `json:"effective_business_rounds"`
	TotalElapsedMS          *int64     `json:"total_elapsed_ms"`
	FirstTerminalAt         *Timestamp `json:"first_terminal_at"`
	FirstTerminalState      *RunState  `json:"first_terminal_state"`
	AsOf                    Timestamp  `json:"as_of"`
	Basis                   string     `json:"basis"`
}

type RuntimeFailureDiagnosis struct {
	Code                string      `json:"code"`
	Category            string      `json:"category"`
	Step                string      `json:"step"`
	CommandID           *Id         `json:"command_id"`
	OperationID         *Id         `json:"operation_id"`
	AffectedRefs        []ObjectRef `json:"affected_refs"`
	Retryable           bool        `json:"retryable"`
	MissingRequirements []string    `json:"missing_requirements"`
	SuggestedAction     string      `json:"suggested_action"`
	ReconciliationURL   *string     `json:"reconciliation_url"`
	Message             string      `json:"message"`
	RecordedAt          Timestamp   `json:"recorded_at"`
}

type RuntimeAuditTrail struct {
	RuntimeVersion string              `json:"runtime_version"`
	RunID          Id                  `json:"run_id"`
	RunRef         ObjectRef           `json:"run_ref"`
	Coverage       string              `json:"coverage"`
	Events         []RuntimeAuditEvent `json:"events"`
	HeadHash       *Hash               `json:"head_hash"`
	AuditHash      Hash                `json:"audit_hash"`
	ObservedAt     Timestamp           `json:"observed_at"`
}

type RuntimeBudgetStatus struct {
	Policy             BudgetPolicy `json:"policy"`
	LedgerRef          *ObjectRef   `json:"ledger_ref"`
	ExecutionDeadline  *Timestamp   `json:"execution_deadline"`
	ContextMeasurement string       `json:"context_measurement"`
}

type RuntimeView struct {
	RuntimeVersion   string                   `json:"runtime_version"`
	RunID            Id                       `json:"run_id"`
	RunRef           ObjectRef                `json:"run_ref"`
	Modes            Modes                    `json:"modes"`
	ObservedAt       Timestamp                `json:"observed_at"`
	Checkpoint       RuntimeCheckpoint        `json:"checkpoint"`
	ActivityAttempts []RuntimeActivityAttempt `json:"activity_attempts"`
	Metrics          RuntimeMetrics           `json:"metrics"`
	Diagnosis        *RuntimeFailureDiagnosis `json:"diagnosis"`
	Budget           RuntimeBudgetStatus      `json:"budget"`
	AuditCoverage    string                   `json:"audit_coverage"`
	AuditHeadHash    *Hash                    `json:"audit_head_hash"`
	ViewHash         Hash                     `json:"view_hash"`
}

type RuntimeHealthCheck struct {
	Name                 string    `json:"name"`
	Status               string    `json:"status"`
	Reason               string    `json:"reason"`
	CheckedAt            Timestamp `json:"checked_at"`
	AffectedCapabilities []string  `json:"affected_capabilities"`
}

type RuntimeHealth struct {
	RuntimeVersion   string               `json:"runtime_version"`
	Live             bool                 `json:"live"`
	Ready            bool                 `json:"ready"`
	Stopping         bool                 `json:"stopping"`
	InstanceID       Id                   `json:"instance_id"`
	ExecutionMode    ExecutionMode        `json:"execution_mode"`
	StorageOwnership string               `json:"storage_ownership"`
	CheckedAt        Timestamp            `json:"checked_at"`
	Checks           []RuntimeHealthCheck `json:"checks"`
	AffectedRunIDs   []Id                 `json:"affected_run_ids"`
	HealthHash       Hash                 `json:"health_hash"`
}

type RuntimeServiceMetrics struct {
	RuntimeVersion          string    `json:"runtime_version"`
	ObservedAt              Timestamp `json:"observed_at"`
	IncludedRunIDs          []Id      `json:"included_run_ids"`
	ExcludedRunIDs          []Id      `json:"excluded_run_ids"`
	SuccessfulRuns          int64     `json:"successful_runs"`
	FailedRuns              int64     `json:"failed_runs"`
	ActivityRetries         int64     `json:"activity_retries"`
	VerificationFailures    int64     `json:"verification_failures"`
	EffectiveBusinessRounds int64     `json:"effective_business_rounds"`
	TotalElapsedMS          int64     `json:"total_elapsed_ms"`
	MetricsHash             Hash      `json:"metrics_hash"`
}

const RuntimeSchemaHash = "8ce4c2641684b76dde0f073cee699f94a2f9f4122efb1b0f872c0e7cbc3a32bc"
const RuntimeSchemaID = "https://agentstudio.invalid/runtime/1.0.0/runtime.schema.json"
