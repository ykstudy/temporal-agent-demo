// GENERATED from complete published JSON Schemas. Do not hand edit.
// Runtime validation is mandatory; TS cannot encode all conditional constraints.

// common.schema.json#/$defs/Id
export type Id = ((string))

// common.schema.json#/$defs/Hash
export type Hash = ((string))

// common.schema.json#/$defs/Timestamp
export type Timestamp = ((string))

// common.schema.json#/$defs/RunState
export type RunState = ("INTAKE" | "CLARIFY" | "PLAN" | "IMPLEMENT" | "VERIFY" | "DELIVER" | "DONE" | "PAUSED" | "BLOCKED" | "FAILED" | "CANCELLED")

// common.schema.json#/$defs/ExecutionMode
export type ExecutionMode = ("temporal" | "local")

// common.schema.json#/$defs/DecisionMode
export type DecisionMode = ("model" | "rules")

// common.schema.json#/$defs/DeliveryMode
export type DeliveryMode = ("gitlab" | "local")

// common.schema.json#/$defs/Modes
export type Modes = ({ "execution_mode": ExecutionMode; "decision_mode": DecisionMode; "delivery_mode": DeliveryMode })

// common.schema.json#/$defs/PositiveVersion
export type PositiveVersion = ((number))

// common.schema.json#/$defs/RelativePath
export type RelativePath = ((string))

// common.schema.json#/$defs/ObjectRef
export type ObjectRef = ({ "object_type": ("run" | "message" | "proposal" | "definition" | "plan" | "manifest" | "verification_report" | "artifact" | "summary" | "skill" | "schema" | "template" | "approval" | "decision" | "validation" | "execution" | "evidence" | "workspace" | "template_content" | "context" | "budget" | "budget_reservation" | "budget_settlement" | "verification_candidate" | "package" | "ci_artifact" | "summary_attempt"); "object_id": Id; "object_version": PositiveVersion; "source_version": PositiveVersion; "hash": Hash })

// agent.schema.json#/$defs/RelativePath
export type AgentRelativePath = ((string))

// agent.schema.json#/$defs/HttpEndpoint
export type HttpEndpoint = ((string))

// agent.schema.json#/$defs/ObjectReference
export type ObjectReference = ObjectRef

// agent.schema.json#/$defs/SchemaDependency
export type SchemaDependency = ({ "schema_uri": ("agent.schema.json" | "common.schema.json" | "message.schema.json"); "contract_version": "1.4.0"; "sha256": Hash })

// agent.schema.json#/$defs/SchemaReference
export type SchemaReference = ({ "schema_ref": ((string)); "file_sha256": Hash })

// agent.schema.json#/$defs/SystemPrompt
export type SystemPrompt = ({ "content": ((string)); "sha256": Hash })

// agent.schema.json#/$defs/ToolIdentity
export type ToolIdentity = ({ "tool_id": Id; "version": ((string)); "platform": ((((string)) | ((null)))); "binary_sha256": ((Hash | ((null)))) })

// agent.schema.json#/$defs/TemplateFile
export type TemplateFile = ({ "path": AgentRelativePath; "sha256": Hash; "content_ref": (ObjectRef) & (({ "object_type"?: "template_content"; "object_id"?: ((string)); "object_version"?: 1; "source_version"?: 1; [key: string]: unknown })) })

// agent.schema.json#/$defs/TemplateDependency
export type TemplateDependency = ({ "template_id": Id; "template_version": ((string)); "files": (Array<TemplateFile>); "manifest_hash": Hash })

// agent.schema.json#/$defs/SkillReference
export type SkillReference = ({ "skill_id": ("add_database" | "add_auth" | "add_logging"); "skill_version": ((string)); "skill_hash": Hash })

// agent.schema.json#/$defs/DatabaseParameters
export type DatabaseParameters = ({ "database": ("postgres" | "mysql" | "sqlite"); "orm": ("gorm" | "sqlx") })

// agent.schema.json#/$defs/AuthenticationParameters
export type AuthenticationParameters = ({ "auth": ("jwt" | "session"); "token_ttl": ((string)) })

// agent.schema.json#/$defs/LoggingParameters
export type LoggingParameters = ({ "log_level": ("debug" | "info" | "warn" | "error"); "log_format": ("json" | "text"); "log_output": ("console" | "file"); "log_path": (AgentRelativePath); "log_max_size": ((number)); "log_max_backups": ((number)); "log_max_age": ((string)); "log_on_write_error": ("degrade" | "fail_closed") })

// agent.schema.json#/$defs/StringParameterSchema
export type StringParameterSchema = ({ "type": "string"; "enum"?: (Array<((string))>); "minLength"?: ((number)); "maxLength"?: ((number)); "pattern"?: ((string)); "default": ((string)); "description"?: ((string)) })

// agent.schema.json#/$defs/IntegerParameterSchema
export type IntegerParameterSchema = ({ "type": "integer"; "minimum"?: ((number)); "maximum"?: ((number)); "default": ((number)); "description"?: ((string)) })

// agent.schema.json#/$defs/ParameterPropertySchema
export type ParameterPropertySchema = ((StringParameterSchema | IntegerParameterSchema))

// agent.schema.json#/$defs/SkillParameterSchema
export type SkillParameterSchema = ({ "$schema": "https://json-schema.org/draft/2020-12/schema"; "type": "object"; "additionalProperties": false; "properties": ({ "database"?: ParameterPropertySchema; "orm"?: ParameterPropertySchema; "auth"?: ParameterPropertySchema; "token_ttl"?: ParameterPropertySchema; "log_level"?: ParameterPropertySchema; "log_format"?: ParameterPropertySchema; "log_output"?: ParameterPropertySchema; "log_path"?: ParameterPropertySchema; "log_max_size"?: ParameterPropertySchema; "log_max_backups"?: ParameterPropertySchema; "log_max_age"?: ParameterPropertySchema; "log_on_write_error"?: ParameterPropertySchema }); "required": (Array<("database" | "orm" | "auth" | "token_ttl" | "log_level" | "log_format" | "log_output" | "log_path" | "log_max_size" | "log_max_backups" | "log_max_age" | "log_on_write_error")>) })

// agent.schema.json#/$defs/PostCommand
export type PostCommand = ({ "command_id": ("go.mod_tidy" | "go.test" | "go.build"); "executable": "go"; "arguments": ((["mod", "tidy"] | ["test", "./..."] | ["build", "./..."])); "cwd_scope": "run_workspace"; "environment_policy": "isolated"; "timeout_seconds": ((number)); "max_attempts": ((number)) })

// agent.schema.json#/$defs/SkillDefinition
export type SkillDefinition = ({ "contract_version": "1.4.0"; "skill_id": ("add_database" | "add_auth" | "add_logging"); "skill_version": ((string)); "skill_hash": Hash; "name": ((string)); "description": ((string)); "supported_stacks": ["golang"]; "parameters_schema": SkillParameterSchema; "default_parameters": ((DatabaseParameters | AuthenticationParameters | LoggingParameters)); "dependencies": (Array<SkillReference>); "conflicts": (Array<("add_database" | "add_auth" | "add_logging")>); "input_contract": SchemaReference; "output_contract": SchemaReference; "template_dependencies": (Array<TemplateDependency>); "write_paths": (Array<AgentRelativePath>); "required_permissions": (Array<("workspace_read" | "workspace_write" | "tool_go")>); "required_tools": (Array<ToolIdentity>); "post_commands": (Array<PostCommand>); "required_check_ids": (Array<Id>) })

// agent.schema.json#/$defs/SkillSelection
export type SkillSelection = ((({ "skill_id": "add_database"; "skill_version": ((string)); "skill_hash": Hash; "parameters": DatabaseParameters }) | ({ "skill_id": "add_auth"; "skill_version": ((string)); "skill_hash": Hash; "parameters": AuthenticationParameters }) | ({ "skill_id": "add_logging"; "skill_version": ((string)); "skill_hash": Hash; "parameters": LoggingParameters })))

// agent.schema.json#/$defs/LocalTarget
export type LocalTarget = ({ "provider": "local"; "destination": "run_package" })

// agent.schema.json#/$defs/GitLabTarget
export type GitLabTarget = ({ "provider": "gitlab"; "instance_url": HttpEndpoint; "namespace_id": ((string)); "namespace_path": ((string)); "project_path": ((string)); "ref": "main"; "visibility": "private" })

// agent.schema.json#/$defs/DeliveryTarget
export type DeliveryTarget = ((LocalTarget | GitLabTarget))

// agent.schema.json#/$defs/ProjectPlan
export type ProjectPlan = ({ "contract_version": "1.4.0"; "project_name": ((string)); "description": ((string)); "tech_stack": "golang"; "selected_skills": (Array<SkillSelection>); "template_dependencies": (Array<TemplateDependency>); "delivery_target": DeliveryTarget; "plan_hash": Hash; "verification_candidate_ref": (((ObjectReference) & (({ "object_type"?: "verification_candidate"; [key: string]: unknown })) | ((null)))) })

// agent.schema.json#/$defs/BudgetPolicy
export type BudgetPolicy = ({ "max_commands": ((number)); "max_model_attempts": ((number)); "max_files": ((number)); "max_artifact_bytes": ((number)); "max_step_attempts": ((number)); "max_tool_seconds": ((number)); "max_execution_seconds": ((number)); "max_ci_wait_seconds": ((number)); "max_ci_poll_attempts": ((number)); "max_reconciliation_seconds": ((number)); "max_reconciliation_attempts": ((number)) })

// agent.schema.json#/$defs/ContextPolicy
export type ContextPolicy = ({ "max_tokens": ((number)); "recent_messages": ((number)); "summary_tokens": ((number)); "output_reserve_tokens": ((number)); "protocol_reserve_tokens": ((number)); "compression_threshold": ((number)); "compression_unit": "message"; "summary_generation": "follow_decision_mode"; "summary_generator_version": ((string)); "summary_prompt": SystemPrompt; "summary_output_schema": SchemaReference })

// agent.schema.json#/$defs/ActionType
export type ActionType = ("scaffold" | "apply_skill" | "verify" | "create_repository" | "push_repository" | "trigger_ci" | "package")

// agent.schema.json#/$defs/ActionProtocol
export type ActionProtocol = ({ "model_output_schema": SchemaReference; "decision_proposal_schema": SchemaReference; "action_validation_schema": SchemaReference; "execution_request_schema": SchemaReference; "allowed_actions": (Array<ActionType>) })

// agent.schema.json#/$defs/CheckDefinition
export type CheckDefinition = ({ "check_id": Id; "title": ((string)); "kind": ("source" | "manifest" | "tool" | "file" | "secret_boundary" | "package" | "remote_repository" | "remote_commit" | "ci"); "delivery_modes": (Array<DeliveryMode>); "required_evidence_types": (Array<("source_hash" | "manifest_hash" | "tool_exit" | "file_hash" | "secret_scan" | "zip_integrity" | "remote_identity" | "commit_tree" | "pipeline_jobs" | "coverage_report" | "build_artifact")>) })

// agent.schema.json#/$defs/CompletionPolicy
export type CompletionPolicy = ({ "required_checks": (Array<CheckDefinition>); "completion_authority": "deterministic_validator"; "external_validation_authority": false; "allow_unverified_done": false })

// agent.schema.json#/$defs/RepositoryPolicy
export type RepositoryPolicy = ({ "allowed_providers": (Array<DeliveryMode>); "visibility": "private"; "required_ref": "main"; "require_explicit_namespace": true; "overwrite_existing_unowned_project": false; "unknown_write_policy": "block_and_reconcile_original_operation"; "git_execution": GitExecutionPolicy })

// agent.schema.json#/$defs/AgentDefinition
export type AgentDefinition = ({ "contract_version": "1.4.0"; "definition_id": Id; "definition_version": ((string)); "definition_hash": Hash; "supported_stacks": ["golang"]; "available_skills": (Array<SkillReference>); "system_prompt": SystemPrompt; "action_protocol": ActionProtocol; "default_ci": "gitlab_ci"; "repository_policy": RepositoryPolicy; "context_policy": ContextPolicy; "budget_policy": BudgetPolicy; "completion_policy": CompletionPolicy; "required_tools": (Array<ToolIdentity>); "post_commands": (Array<PostCommand>); "go_module_policy": GoModulePolicy }) & (({ "required_tools"?: unknown; [key: string]: unknown }))

// agent.schema.json#/$defs/ProviderBinding
export type ProviderBinding = ({ "provider_kind": "openai_compatible"; "endpoint": HttpEndpoint; "model_id": ((string)) })

// agent.schema.json#/$defs/DefinitionSnapshot
export type DefinitionSnapshot = ({ "contract_version": "1.4.0"; "definition_id": Id; "definition_version": ((string)); "definition_hash": Hash; "snapshot_version": ((number)); "snapshot_hash": Hash; "definition": AgentDefinition; "skills": (Array<SkillDefinition>); "schema_dependencies": (Array<SchemaDependency>); "template_dependencies": (Array<TemplateDependency>); "tool_bindings": (Array<ToolIdentity>); "provider_binding": ((ProviderBinding | ((null)))); "bound_at": Timestamp })

// agent.schema.json#/$defs/ScaffoldParameters
export type ScaffoldParameters = ({ "plan_hash": Hash })

// agent.schema.json#/$defs/ApplySkillParameters
export type ApplySkillParameters = ({ "plan_hash": Hash; "selection": SkillSelection })

// agent.schema.json#/$defs/VerifyParameters
export type VerifyParameters = ({ "manifest_ref": ObjectReference; "required_check_ids": (Array<Id>) })

// agent.schema.json#/$defs/CreateRepositoryParameters
export type CreateRepositoryParameters = ({ "target": GitLabTarget })

// agent.schema.json#/$defs/PushRepositoryParameters
export type PushRepositoryParameters = ({ "target": GitLabTarget; "project_id": ((number)); "manifest_ref": ObjectReference; "expected_branch_sha": ((((string)) | ((null)))) })

// agent.schema.json#/$defs/TriggerCIParameters
export type TriggerCIParameters = ({ "target": GitLabTarget; "project_id": ((number)); "commit_sha": ((string)); "ci_config_sha256": Hash })

// agent.schema.json#/$defs/PackageParameters
export type PackageParameters = ({ "manifest_ref": ObjectReference })

// agent.schema.json#/$defs/CandidateAction
export type CandidateAction = ((({ "action_type": "scaffold"; "parameters": ScaffoldParameters }) | ({ "action_type": "apply_skill"; "parameters": ApplySkillParameters }) | ({ "action_type": "verify"; "parameters": VerifyParameters }) | ({ "action_type": "create_repository"; "parameters": CreateRepositoryParameters }) | ({ "action_type": "push_repository"; "parameters": PushRepositoryParameters }) | ({ "action_type": "trigger_ci"; "parameters": TriggerCIParameters }) | ({ "action_type": "package"; "parameters": PackageParameters })))

// agent.schema.json#/$defs/Diagnosis
export type Diagnosis = ({ "category": ("input" | "constraint" | "permission" | "budget" | "infrastructure" | "business" | "unknown_result"); "code": Id; "message": ((string)); "evidence_refs": (Array<ObjectReference>); "suggested_remedy": ((string)) })

// agent.schema.json#/$defs/DecisionAdvice
export type DecisionAdvice = ({ "kind": ("clarification" | "plan" | "action" | "diagnosis" | "completion"); "question": ((((string)) | ((null)))); "plan": ((ProjectPlan | ((null)))); "action": ((CandidateAction | ((null)))); "diagnosis": ((Diagnosis | ((null)))); "completion_hint": ((((string)) | ((null)))); "reason": ((string)) })

// agent.schema.json#/$defs/ModelUsage
export type ModelUsage = ({ "status": ("known" | "unknown" | "not_applicable"); "input_tokens": ((((number)) | ((null)))); "output_tokens": ((((number)) | ((null)))); "total_tokens": ((((number)) | ((null)))); "provider_request_id": ((Id | ((null)))) })

// agent.schema.json#/$defs/DecisionProposal
export type DecisionProposal = ({ "contract_version": "1.4.0"; "decision_id": Id; "run_id": Id; "source_version": ((number)); "state_sequence": ((number)); "definition_snapshot_hash": Hash; "decision_mode": DecisionMode; "advice": DecisionAdvice; "model_usage": ModelUsage; "input_message_refs": (Array<ObjectReference>); "input_context_hash": Hash; "decision_hash": Hash; "created_at": Timestamp })

// agent.schema.json#/$defs/ValidationFinding
export type ValidationFinding = ({ "rule_id": Id; "passed": ((boolean)); "code": Id; "detail": ((string)) })

// agent.schema.json#/$defs/ApprovalBinding
export type ApprovalBinding = ({ "approval_id": Id; "proposal_id": Id; "proposal_version": ((number)); "source_version": ((number)); "proposal_snapshot_hash": Hash; "definition_snapshot_hash": Hash; "permission_hash": Hash; "issued_at": Timestamp; "expires_at": Timestamp })

// agent.schema.json#/$defs/WorkspaceReference
export type WorkspaceReference = ({ "workspace_id": Id; "run_id": Id; "storage_key": AgentRelativePath })

// agent.schema.json#/$defs/ExecutionRequest
export type ExecutionRequest = ({ "contract_version": "1.4.0"; "execution_id": Id; "operation_id": Id; "command_id": Id; "run_id": Id; "source_version": ((number)); "state_sequence_at_validation": ((number)); "definition_snapshot_hash": Hash; "plan_hash": Hash; "validation_id": Id; "validation_hash": Hash; "approval": ApprovalBinding; "workspace": WorkspaceReference; "action": CandidateAction; "required_permission_ids": (Array<("workspace_read" | "workspace_write" | "tool_go" | "gitlab_create" | "gitlab_push" | "gitlab_ci" | "package_write")>); "request_hash": Hash; "created_at": Timestamp })

// agent.schema.json#/$defs/ActionValidation
export type ActionValidation = ({ "contract_version": "1.4.0"; "validation_id": Id; "decision_id": Id; "run_id": Id; "source_version": ((number)); "state_sequence": ((number)); "definition_snapshot_hash": Hash; "decision_kind": ("clarification" | "plan" | "action" | "diagnosis" | "completion"); "verdict": ("allowed" | "rejected" | "needs_clarification"); "findings": (Array<ValidationFinding>); "normalized_advice": ((DecisionAdvice | ((null)))); "execution_request": ((ExecutionRequest | ((null)))); "validation_hash": Hash; "validated_at": Timestamp })

// agent.schema.json#/$defs/SummarySource
export type SummarySource = ({ "first_message_id": Id; "last_message_id": Id; "message_ids": (Array<Id>); "first_sequence": ((number)); "last_sequence": ((number)); "raw_jsonl_sha256": Hash })

// agent.schema.json#/$defs/SummaryGenerator
export type SummaryGenerator = ({ "mode": DecisionMode; "generator_version": ((string)); "model_id": ((((string)) | ((null)))); "prompt_sha256": ((Hash | ((null)))) })

// agent.schema.json#/$defs/SummaryRecord
export type SummaryRecord = ({ "contract_version": "1.4.0"; "summary_id": Id; "summary_version": ((number)); "run_id": Id; "source_version": ((number)); "definition_snapshot_hash": Hash; "source": SummarySource; "generator": SummaryGenerator; "previous_summary_ref": ((ObjectReference | ((null)))); "content": ((string)); "content_sha256": Hash; "summary_hash": Hash; "created_at": Timestamp })

// agent.schema.json#/$defs/FileChange
export type FileChange = ((({ "operation"?: "create"; "before"?: ((null)); "before_sha256"?: ((null)); "after"?: ((string)); "after_sha256"?: Hash; [key: string]: unknown }) | ({ "operation"?: "update"; "before"?: ((string)); "before_sha256"?: Hash; "after"?: ((string)); "after_sha256"?: Hash; [key: string]: unknown }) | ({ "operation"?: "delete"; "before"?: ((string)); "before_sha256"?: Hash; "after"?: ((null)); "after_sha256"?: ((null)); [key: string]: unknown }))) & ({ "path": AgentRelativePath; "operation": ("create" | "update" | "delete"); "before": ((((string)) | ((null)))); "before_sha256": ((Hash | ((null)))); "after": ((((string)) | ((null)))); "after_sha256": ((Hash | ((null)))) })

// agent.schema.json#/$defs/PreflightCheck
export type PreflightCheck = ({ "check_id": Id; "passed": ((boolean)); "detail": ((string)) })

// agent.schema.json#/$defs/Proposal
export type Proposal = ({ "contract_version": "1.4.0"; "proposal_id": Id; "run_id": Id; "proposal_version": ((number)); "source_version": ((number)); "definition_snapshot_hash": Hash; "plan_hash": Hash; "snapshot_hash": Hash; "title": ((string)); "status": ("pending" | "blocked" | "accepted" | "rejected" | "revision_requested" | "expired" | "superseded"); "plan": ProjectPlan; "changes": (Array<FileChange>); "impact": (Array<((string))>); "allowed_actions": (Array<ActionType>); "required_permission_ids": (Array<("workspace_read" | "workspace_write" | "tool_go" | "gitlab_create" | "gitlab_push" | "gitlab_ci" | "package_write")>); "permission_hash": Hash; "checks": (Array<PreflightCheck>); "issued_at": Timestamp; "expires_at": Timestamp })

// agent.schema.json#/$defs/Approval
export type Approval = ({ "contract_version": "1.4.0"; "approval_id": Id; "run_id": Id; "actor_id": Id; "proposal_id": Id; "proposal_version": ((number)); "source_version": ((number)); "proposal_snapshot_hash": Hash; "definition_snapshot_hash": Hash; "plan_hash": Hash; "permission_hash": Hash; "allowed_actions": (Array<ActionType>); "delivery_target": DeliveryTarget; "issued_at": Timestamp; "expires_at": Timestamp; "accepted_at": Timestamp; "started_at": ((Timestamp | ((null)))); "revoked_at": ((Timestamp | ((null)))); "status": ("accepted" | "started" | "revoked" | "expired"); "approval_hash": Hash })

// agent.schema.json#/$defs/ArtifactEntry
export type ArtifactEntry = ({ "artifact_id": Id; "path": AgentRelativePath; "size": ((number)); "sha256": Hash; "media_type": ((string)) })

// agent.schema.json#/$defs/ArtifactManifest
export type ArtifactManifest = ({ "contract_version": "1.4.0"; "manifest_id": Id; "run_id": Id; "source_version": ((number)); "proposal_id": Id; "proposal_version": ((number)); "proposal_snapshot_hash": Hash; "definition_snapshot_hash": Hash; "plan_hash": Hash; "files": (Array<ArtifactEntry>); "hash": Hash; "created_at": Timestamp; "manifest_version": ((number)) })

// agent.schema.json#/$defs/RequiredCheck
export type RequiredCheck = ({ "check_id": Id; "run_id": Id; "source_version": ((number)); "definition_snapshot_hash": Hash; "proposal_snapshot_hash": ((Hash | ((null)))); "status": ("not_ready" | "running" | "passed" | "failed" | "not_verified"); "detail": ((string)); "evidence_refs": (Array<ObjectReference>); "started_at": ((Timestamp | ((null)))); "completed_at": ((Timestamp | ((null)))) })

// agent.schema.json#/$defs/Step
export type Step = ({ "step_id": Id; "title": ((string)); "state": ("pending" | "running" | "completed" | "failed" | "skipped"); "detail": ((string)); "source_version": ((number)); "execution_refs": (Array<ObjectReference>) })

// agent.schema.json#/$defs/OperationRecord
export type OperationRecord = ({ "operation_id": Id; "run_id": Id; "source_version": ((number)); "action_type": ActionType; "operation_sequence": ((number)); "request_hash": Hash; "target_hash": Hash; "status": ("ready" | "sent" | "unknown" | "verified" | "rejected"); "external_object_id": ((((string)) | ((null)))); "evidence_refs": (Array<ObjectReference>); "created_at": Timestamp; "updated_at": Timestamp })

// agent.schema.json#/$defs/StepAttempt
export type StepAttempt = ({ "step_id": Id; "attempts": ((number)) })

// agent.schema.json#/$defs/Usage
export type Usage = ({ "commands": ((number)); "model_attempts": ((number)); "model_failures": ((number)); "step_attempts": (Array<StepAttempt>); "last_model_error_code": ((Id | ((null)))) })

// agent.schema.json#/$defs/PermissionSnapshot
export type PermissionSnapshot = ({ "permission_version": ((number)); "actor_id": Id; "allowed_permissions": (Array<("workspace_read" | "workspace_write" | "tool_go" | "gitlab_create" | "gitlab_push" | "gitlab_ci" | "package_write")>); "delivery_target": DeliveryTarget; "revoked": ((boolean)); "permission_hash": Hash; "updated_at": Timestamp })

// agent.schema.json#/$defs/Failure
export type Failure = ({ "code": Id; "category": ("input" | "constraint" | "permission" | "budget" | "infrastructure" | "business" | "unknown_result"); "message": ((string)); "operation_id": ((Id | ((null)))); "evidence_refs": (Array<ObjectReference>) })

// agent.schema.json#/$defs/HistoryEvent
export type HistoryEvent = ({ "event_id": Id; "action": Id; "source_version": ((number)); "state_sequence": ((number)); "object_refs": (Array<ObjectReference>); "detail": ((string)); "created_at": Timestamp })

// agent.schema.json#/$defs/WorkflowCheckpoint
export type WorkflowCheckpoint = ({ "workflow_id": ((string)); "execution_run_id": Id; "commands_processed": ((number)); "query_state": RunState; "checkpoint_state_sequence": ((number)) })

// agent.schema.json#/$defs/RunSnapshot
export type RunSnapshot = ({ "contract_version": "1.4.0"; "schema_version": 2; "run_id": Id; "snapshot_hash": Hash; "title": ((string)); "state": RunState; "source_version": ((number)); "state_sequence": ((number)); "modes": Modes; "definition_snapshot": DefinitionSnapshot; "plan": ((ProjectPlan | ((null)))); "plan_hash": ((Hash | ((null)))); "proposal": ((Proposal | ((null)))); "proposal_history": (Array<Proposal>); "approval": ((Approval | ((null)))); "permission_snapshot": ((PermissionSnapshot | ((null)))); "messages": (Array<MessageEnvelope>); "summary_records": (Array<SummaryRecord>); "steps": (Array<Step>); "required_checks": (Array<RequiredCheck>); "artifacts": (Array<ArtifactEntry>); "manifest": ((ArtifactManifest | ((null)))); "operations": (Array<OperationRecord>); "usage": Usage; "workspace": ((WorkspaceReference | ((null)))); "workflow_checkpoint": ((WorkflowCheckpoint | ((null)))); "history": (Array<HistoryEvent>); "active_step": ((Id | ((null)))); "pause_requested": ((boolean)); "cancel_requested": ((boolean)); "resume_to": ((RunState | ((null)))); "retry_of": ((Id | ((null)))); "error": ((Failure | ((null)))); "created_at": Timestamp; "updated_at": Timestamp; "verification_report": ((VerificationReport | ((null)))); "summary_attempts": (Array<SummaryAttempt>) })

// agent.schema.json#/$defs/VerificationReport
export type VerificationReport = ({ "contract_version": "1.4.0"; "report_id": Id; "report_version": ((number)); "run_id": Id; "source_version": ((number)); "definition_snapshot_hash": Hash; "proposal_snapshot_hash": Hash; "manifest_ref": ObjectReference; "checks": (Array<RequiredCheck>); "outcome": ("not_ready" | "passed" | "failed"); "report_hash": Hash; "created_at": Timestamp }) & (({ "manifest_ref"?: ({ "object_type"?: "manifest"; [key: string]: unknown }); [key: string]: unknown }))

// agent.schema.json#/$defs/DefinitionSelection
export type DefinitionSelection = ({ "definition_id": Id; "definition_version": ((string)); "definition_hash": Hash })

// agent.schema.json#/$defs/CreateRunRequest
export type CreateRunRequest = ({ "contract_version": "1.4.0"; "schema_version": 2; "command_id": Id; "run_id": Id; "title": ((string)); "modes": Modes; "definition_selection": DefinitionSelection; "initial_content": ((string)) })

// agent.schema.json#/$defs/MessageRequest
export type MessageRequest = ({ "contract_version": "1.4.0"; "schema_version": 2; "command_id": Id; "target": (MessageObjectRef) & (({ "object_type"?: "run"; [key: string]: unknown })); "content": ((string)) })

// agent.schema.json#/$defs/ControlRequest
export type ControlRequest = ActionIntent

// agent.schema.json#/$defs/CommandReceipt
export type CommandReceipt = ({ "contract_version": "1.4.0"; "schema_version": 2; "command_id": Id; "run_id": Id; "command_fingerprint": Hash; "status": ("accepted" | "queued" | "processed" | "rejected" | "result_unknown"); "run_ref": (((MessageObjectRef) & (({ "object_type"?: "run"; [key: string]: unknown })) | ((null)))); "result_refs": (Array<ObjectReference>); "error": ((Failure | ((null)))); "accepted_at": Timestamp; "updated_at": Timestamp })

// agent.schema.json#/$defs/SnapshotEvent
export type SnapshotEvent = ({ "contract_version": "1.4.0"; "schema_version": 2; "event_id": Id; "run_id": Id; "source_version": ((number)); "state_sequence": ((number)); "run_ref": (MessageObjectRef) & (({ "object_type"?: "run"; [key: string]: unknown })); "occurred_at": Timestamp })

// agent.schema.json#/$defs/ErrorResponse
export type ErrorResponse = ({ "error": ((string)); "code": Id; "legacy_read_url"?: ((string)) })

// agent.schema.json#/$defs/BudgetReservation
export type BudgetReservation = ({ "contract_version": "1.4.0"; "reservation_id": Id; "run_id": Id; "source_version": ((number)); "definition_snapshot_hash": Hash; "policy_hash": Hash; "command_id": Id; "operation_id": ((Id | ((null)))); "execution_id": ((Id | ((null)))); "request_hash": ((Hash | ((null)))); "attempt_id": Id; "kind": ("command" | "model" | "action" | "tool" | "ci_poll" | "reconciliation_read"); "scope_id": Id; "units": 1; "reserved_at": Timestamp; "deadline_at": ((Timestamp | ((null)))); "reservation_hash": Hash })

// agent.schema.json#/$defs/BudgetSettlement
export type BudgetSettlement = ({ "contract_version": "1.4.0"; "settlement_id": Id; "reservation_id": Id; "reservation_hash": Hash; "attempt_id": Id; "run_id": Id; "source_version": ((number)); "consumed_units": ((number)); "outcome": ("succeeded" | "failed" | "interrupted" | "released" | "unknown"); "elapsed_ms": ((((number)) | ((null)))); "measurement_source": ("actual" | "unknown"); "settled_at": Timestamp; "settlement_hash": Hash })

// agent.schema.json#/$defs/BudgetWindow
export type BudgetWindow = ({ "window_id": Id; "kind": ("ci_poll" | "reconciliation_read"); "operation_id": Id; "scope_id": Id; "source_version": ((number)); "started_at": Timestamp; "deadline_at": Timestamp; "max_attempts": ((number)) })

// agent.schema.json#/$defs/BudgetCounters
export type BudgetCounters = ({ "command": ((number)); "model": ((number)); "action": ((number)); "tool": ((number)); "ci_poll": ((number)); "reconciliation_read": ((number)) })

// agent.schema.json#/$defs/BudgetTotals
export type BudgetTotals = ({ "reserved": BudgetCounters; "consumed": BudgetCounters; "released": BudgetCounters; "unknown": BudgetCounters; "measured_elapsed_ms": ((number)); "unknown_elapsed_attempts": ((number)) })

// agent.schema.json#/$defs/BudgetLedger
export type BudgetLedger = ({ "contract_version": "1.4.0"; "run_id": Id; "definition_snapshot_hash": Hash; "policy_hash": Hash; "ledger_version": ((number)); "execution_started_at": ((Timestamp | ((null)))); "execution_deadline_at": ((Timestamp | ((null)))); "ci_windows": (Array<BudgetWindow>); "reconciliation_windows": (Array<BudgetWindow>); "reservations": (Array<BudgetReservation>); "settlements": (Array<BudgetSettlement>); "totals": BudgetTotals; "updated_at": Timestamp; "ledger_hash": Hash }) & (({ "ci_windows"?: unknown; "reconciliation_windows"?: unknown; [key: string]: unknown }))

// agent.schema.json#/$defs/ContextMessage
export type ContextMessage = ({ "role": ("system" | "user" | "assistant" | "tool"); "content": ((string)) })

// agent.schema.json#/$defs/ContextRecord
export type ContextRecord = ({ "contract_version": "1.4.0"; "context_id": Id; "run_id": Id; "decision_id": Id; "source_version": ((number)); "state_sequence": ((number)); "definition_snapshot_hash": Hash; "input_message_refs": (Array<(ObjectReference) & (({ "object_type"?: "message"; [key: string]: unknown }))>); "messages": (Array<ContextMessage>); "input_context_hash": Hash; "created_at": Timestamp; "context_hash": Hash; "summary_refs": (Array<ObjectReference>) })

// agent.schema.json#/$defs/ToolExecutionReceipt
export type ToolExecutionReceipt = ({ "contract_version": "1.4.0"; "receipt_id": Id; "run_id": Id; "source_version": ((number)); "definition_snapshot_hash": Hash; "execution_id": Id; "operation_id": Id; "attempt_id": Id; "request_hash": Hash; "approval_id": Id; "approval_hash": Hash; "proposal_snapshot_hash": Hash; "plan_hash": Hash; "workspace": WorkspaceReference; "tool": ToolIdentity; "command_id": ("go.mod_tidy" | "go.test" | "go.build" | "git.init" | "git.add" | "git.commit" | "git.head" | "git.tree" | "git.push"); "arguments": ((["mod", "tidy"] | ["test", "./..."] | ["build", "./..."] | (Array<never>) | (Array<never>) | (Array<never>) | (Array<never>) | (Array<never>) | (Array<never>))); "cwd_scope": "run_workspace"; "environment_policy": ("isolated" | "isolated_git"); "input_manifest_ref": (((ObjectReference) & (({ "object_type"?: "manifest"; [key: string]: unknown })) | ((null)))); "output_manifest_ref": (((ObjectReference) & (({ "object_type"?: "manifest"; [key: string]: unknown })) | ((null)))); "started_at": Timestamp; "completed_at": Timestamp; "exit_code": ((((number)) | ((null)))); "status": ("succeeded" | "failed" | "interrupted" | "unknown"); "output": ((string)); "output_sha256": Hash; "output_truncated": ((boolean)); "receipt_hash": Hash })

// agent.schema.json#/$defs/WorkspaceReceipt
export type WorkspaceReceipt = ({ "contract_version": "1.4.0"; "receipt_id": Id; "run_id": Id; "source_version": ((number)); "definition_snapshot_hash": Hash; "execution_id": Id; "operation_id": Id; "attempt_id": Id; "request_hash": Hash; "approval_id": Id; "approval_hash": Hash; "proposal_snapshot_hash": Hash; "plan_hash": Hash; "workspace": WorkspaceReference; "action_type": ("scaffold" | "apply_skill"); "written_paths": (Array<AgentRelativePath>); "input_manifest_ref": (((ObjectReference) & (({ "object_type"?: "manifest"; [key: string]: unknown })) | ((null)))); "output_manifest_ref": (((ObjectReference) & (({ "object_type"?: "manifest"; [key: string]: unknown })) | ((null)))); "started_at": Timestamp; "completed_at": Timestamp; "status": ("succeeded" | "failed" | "interrupted" | "unknown"); "receipt_hash": Hash })

// agent.schema.json#/$defs/S2AuditBundle
export type S2AuditBundle = ({ "contract_version": "1.4.0"; "schema_version": 2; "run_id": Id; "modes": Modes; "state": RunState; "source_version": ((number)); "state_sequence": ((number)); "definition_snapshot": DefinitionSnapshot; "plan": ((ProjectPlan | ((null)))); "proposal": ((Proposal | ((null)))); "approval": ((Approval | ((null)))); "permission_snapshot": ((PermissionSnapshot | ((null)))); "proposal_history": (Array<Proposal>); "approval_history": (Array<Approval>); "permission_history": (Array<PermissionSnapshot>); "decisions": (Array<DecisionProposal>); "validations": (Array<ActionValidation>); "executions": (Array<ExecutionRequest>); "messages": (Array<MessageEnvelope>); "contexts": (Array<ContextRecord>); "operations": (Array<OperationRecord>); "manifest": ((ArtifactManifest | ((null)))); "manifest_history": (Array<ArtifactManifest>); "budget": BudgetLedger; "tool_receipts": (Array<ToolExecutionReceipt>); "workspace_receipts": (Array<WorkspaceReceipt>); "created_at": Timestamp; "audit_hash": Hash; "remote_receipts": (Array<RemoteExecutionReceipt>); "package_receipts": (Array<PackageReceipt>); "verification_candidates": (Array<VerificationCandidate>); "verification_reports": (Array<VerificationReport>); "verification_report": ((VerificationReport | ((null)))) })

// agent.schema.json#/$defs/GitCommandPolicy
export type GitCommandPolicy = ({ "command_id": ("git.init" | "git.add" | "git.commit" | "git.head" | "git.tree" | "git.push"); "executable": "git"; "arguments_template": (Array<((string))>); "cwd_scope": "run_workspace"; "environment_policy": "isolated_git"; "timeout_seconds": ((number)); "max_attempts": ((number)) })

// agent.schema.json#/$defs/GitExecutionPolicy
export type GitExecutionPolicy = ({ "policy_id": "controlled_git_v1"; "commands": (Array<GitCommandPolicy>); "http_auth": "ephemeral_header_environment"; "inherit_environment": false; "hooks_enabled": false; "automatic_ci": "skip_push_then_explicit_bound_trigger"; "allowed_network": "approved_gitlab_instance_only" })

// agent.schema.json#/$defs/GoModulePolicy
export type GoModulePolicy = ({ "proxy_url": ("https://proxy.golang.org" | "https://goproxy.cn"); "sumdb": "sum.golang.org"; "download_mode": "proxy_only"; "allow_direct": false; "inherit_environment": false })

// agent.schema.json#/$defs/VerificationCandidateFile
export type VerificationCandidateFile = ({ "path": RelativePath; "content": ((string)); "sha256": Hash; "size": ((number)) })

// agent.schema.json#/$defs/VerificationCandidate
export type VerificationCandidate = ({ "contract_version": "1.4.0"; "run_id": Id; "source_version": ((number)); "definition_snapshot_hash": Hash; "execution_id": Id; "operation_id": Id; "attempt_id": Id; "request_hash": Hash; "approval_id": Id; "approval_hash": Hash; "proposal_snapshot_hash": Hash; "plan_hash": Hash; "workspace": WorkspaceReference; "candidate_id": Id; "input_manifest_ref": (ObjectReference) & (({ "object_type"?: "manifest"; [key: string]: unknown })); "tool_receipt_refs": (Array<(ObjectReference) & (({ "object_type"?: "evidence"; [key: string]: unknown }))>); "files": (Array<VerificationCandidateFile>); "changed_paths": (Array<RelativePath>); "total_bytes": ((number)); "created_at": Timestamp; "candidate_hash": Hash })

// agent.schema.json#/$defs/PackageReceipt
export type PackageReceipt = ({ "contract_version": "1.4.0"; "run_id": Id; "source_version": ((number)); "definition_snapshot_hash": Hash; "execution_id": Id; "operation_id": Id; "attempt_id": Id; "request_hash": Hash; "approval_id": Id; "approval_hash": Hash; "proposal_snapshot_hash": Hash; "plan_hash": Hash; "workspace": WorkspaceReference; "receipt_id": Id; "manifest_ref": (ObjectReference) & (({ "object_type"?: "manifest"; [key: string]: unknown })); "package_ref": (((ObjectReference) & (({ "object_type"?: "package"; [key: string]: unknown })) | ((null)))); "package_sha256": ((Hash | ((null)))); "package_bytes": ((number)); "file_count": ((number)); "started_at": Timestamp; "completed_at": Timestamp; "status": ("succeeded" | "failed" | "interrupted" | "unknown"); "receipt_hash": Hash })

// agent.schema.json#/$defs/RemoteProjectObservation
export type RemoteProjectObservation = ({ "project_id": ((number)); "namespace_id": ((string)); "namespace_path": ((string)); "project_path": ((string)); "path_with_namespace": ((string)); "visibility": ("private" | "internal" | "public"); "ownership_marker": ((string)); "empty_repository": ((boolean)); "default_branch": ((((string)) | ((null)))) })

// agent.schema.json#/$defs/RemoteBranchObservation
export type RemoteBranchObservation = ({ "project_id": ((number)); "ref": ((string)); "commit_sha": ((string)) })

// agent.schema.json#/$defs/RemoteFileEvidence
export type RemoteFileEvidence = ({ "path": RelativePath; "size": ((number)); "sha256": Hash; "git_blob_sha1": ((string)) })

// agent.schema.json#/$defs/RemoteCommitObservation
export type RemoteCommitObservation = ({ "project_id": ((number)); "commit_sha": ((string)); "parent_shas": (Array<((string))>); "message": ((string)); "files": (Array<RemoteFileEvidence>) })

// agent.schema.json#/$defs/RemotePipelineObservation
export type RemotePipelineObservation = ({ "project_id": ((number)); "pipeline_id": ((number)); "ref": ((string)); "commit_sha": ((string)); "status": ("created" | "waiting_for_resource" | "preparing" | "pending" | "running" | "success" | "failed" | "canceled" | "skipped" | "manual" | "scheduled"); "source": ((string)) })

// agent.schema.json#/$defs/RemotePipelineVariables
export type RemotePipelineVariables = ({ "project_id": ((number)); "pipeline_id": ((number)); "operation_id": Id; "request_hash": Hash })

// agent.schema.json#/$defs/RemoteArtifactEvidence
export type RemoteArtifactEvidence = ({ "path": RelativePath; "artifact_ref": (ObjectReference) & (({ "object_type"?: "ci_artifact"; [key: string]: unknown })); "byte_count": ((number)); "sha256": Hash; "format": ("go_coverage" | "elf_executable" | "mach_o_executable"); "validated": ((boolean)) })

// agent.schema.json#/$defs/CIJobEvidence
export type CIJobEvidence = ({ "project_id": ((number)); "pipeline_id": ((number)); "job_id": ((number)); "name": ((string)); "ref": ((string)); "commit_sha": ((string)); "status": ((string)); "allow_failure": ((boolean)); "artifacts": (Array<RemoteArtifactEvidence>) })

// agent.schema.json#/$defs/RemoteJobListObservation
export type RemoteJobListObservation = ({ "project_id": ((number)); "pipeline_id": ((number)); "jobs": (Array<CIJobEvidence>) })

// agent.schema.json#/$defs/RemoteReadObservation
export type RemoteReadObservation = ({ "reservation_ref": (ObjectReference) & (({ "object_type"?: "budget_reservation"; [key: string]: unknown })); "method": "GET"; "path": ((string)); "http_status": ((((number)) | ((null)))); "started_at": Timestamp; "completed_at": Timestamp; "kind": ("project" | "branch" | "file" | "commit" | "pipeline" | "pipeline_variables" | "jobs" | "artifact" | "unavailable"); "payload": ((RemoteProjectObservation | RemoteBranchObservation | RemoteFileEvidence | RemoteCommitObservation | RemotePipelineObservation | RemotePipelineVariables | RemoteJobListObservation | RemoteArtifactEvidence | ((null)))); "facts_hash": ((Hash | ((null)))) })

// agent.schema.json#/$defs/RemoteExecutionReceipt
export type RemoteExecutionReceipt = ({ "contract_version": "1.4.0"; "run_id": Id; "source_version": ((number)); "definition_snapshot_hash": Hash; "execution_id": Id; "operation_id": Id; "attempt_id": Id; "request_hash": Hash; "approval_id": Id; "approval_hash": Hash; "proposal_snapshot_hash": Hash; "plan_hash": Hash; "workspace": WorkspaceReference; "receipt_id": Id; "action_type": ("create_repository" | "push_repository" | "trigger_ci"); "target": GitLabTarget; "manifest_ref": (ObjectReference) & (({ "object_type"?: "manifest"; [key: string]: unknown })); "project": ((RemoteProjectObservation | ((null)))); "commit": ((RemoteCommitObservation | ((null)))); "ci_config_sha256": ((Hash | ((null)))); "pipeline": ((RemotePipelineObservation | ((null)))); "pipeline_variables": ((RemotePipelineVariables | ((null)))); "jobs": (Array<CIJobEvidence>); "tool_receipt_refs": (Array<(ObjectReference) & (({ "object_type"?: "evidence"; [key: string]: unknown }))>); "observations": (Array<RemoteReadObservation>); "write_sent": ((boolean)); "started_at": Timestamp; "completed_at": Timestamp; "status": ("succeeded" | "failed" | "interrupted" | "unknown"); "receipt_hash": Hash })

// agent.schema.json#/$defs/SummaryAdvice
export type SummaryAdvice = ({ "items": (Array<({ "message_id": Id; "excerpt": ((string)) })>) })

// agent.schema.json#/$defs/SummaryAttempt
export type SummaryAttempt = ({ "contract_version": "1.4.0"; "attempt_id": Id; "run_id": Id; "source_version": ((number)); "definition_snapshot_hash": Hash; "source": SummarySource; "generator": SummaryGenerator; "status": ("succeeded" | "failed" | "unknown"); "model_usage": ModelUsage; "summary_ref": ((ObjectReference | ((null)))); "failure_code": ((Id | ((null)))); "created_at": Timestamp; "completed_at": Timestamp; "attempt_hash": Hash })

// agent.schema.json#/$defs/ConversationAuditBundle
export type ConversationAuditBundle = ({ "contract_version": "1.4.0"; "schema_version": 2; "run_id": Id; "source_version": ((number)); "state_sequence": ((number)); "definition_snapshot_hash": Hash; "messages": (Array<MessageEnvelope>); "summaries": (Array<SummaryRecord>); "summary_attempts": (Array<SummaryAttempt>); "command_receipts": (Array<CommandReceipt>); "history": (Array<HistoryEvent>); "transcript_sha256": Hash; "audit_hash": Hash })

// message.schema.json#/$defs/MessageEnvelope
export type MessageEnvelope = ({ "schema_version": 2; "contract_version": "1.4.0"; "run_id": Id; "message_id": Id; "sequence": PositiveVersion; "source_version": PositiveVersion; "state_sequence": PositiveVersion; "role": ("user" | "assistant" | "system" | "tool"); "created_at": Timestamp; "modes": Modes; "reply_to_message_id"?: Id; "command_id"?: Id; "parts": (Array<Part>) })

// message.schema.json#/$defs/ObjectRef
export type MessageObjectRef = ({ "object_type"?: ("run" | "message" | "proposal" | "definition" | "plan" | "manifest" | "verification_report" | "artifact" | "summary"); [key: string]: unknown }) & (ObjectRef)

// message.schema.json#/$defs/Part
export type Part = ((TextPart | ComponentPart | DiffPart | ChoicePart | ActionPart))

// message.schema.json#/$defs/PartBase
export type PartBase = ({ "part_id": Id; "part_version": 1; "type": ((string)); "fallback_text": ((string)); [key: string]: unknown })

// message.schema.json#/$defs/TextPart
export type TextPart = ({ "type"?: "text"; "text": ((string)); "format": ("plain" | "markdown"); [key: string]: unknown }) & (PartBase)

// message.schema.json#/$defs/ComponentPart
export type ComponentPart = ((({ "component_type"?: "clarification"; "component_schema"?: "urn:agentstudio:component:clarification:1.0.0"; "payload"?: ClarificationPayload; [key: string]: unknown }) | ({ "component_type"?: "proposal_diff"; "component_schema"?: "urn:agentstudio:component:proposal_diff:1.0.0"; "payload"?: ProposalDiffPayload; [key: string]: unknown }) | ({ "component_type"?: "progress"; "component_schema"?: "urn:agentstudio:component:progress:1.0.0"; "payload"?: ProgressPayload; [key: string]: unknown }) | ({ "component_type"?: "artifact_manifest"; "component_schema"?: "urn:agentstudio:component:artifact_manifest:1.0.0"; "payload"?: ArtifactManifestPayload; [key: string]: unknown }) | ({ "component_type"?: "verification_report"; "component_schema"?: "urn:agentstudio:component:verification_report:1.0.0"; "payload"?: VerificationReportPayload; [key: string]: unknown }) | ({ "component_type"?: unknown; "component_schema"?: ((string)); "payload"?: JsonValue; [key: string]: unknown }))) & ({ "type"?: "component"; "component_id": Id; "component_version": PositiveVersion; "component_type": ((string)); "component_schema": ((string)); "payload": unknown; [key: string]: unknown }) & (PartBase)

// message.schema.json#/$defs/ClarificationPayload
export type ClarificationPayload = ({ "question": ((string)); "field_ids": (Array<Id>) })

// message.schema.json#/$defs/ProposalDiffPayload
export type ProposalDiffPayload = ({ "proposal_ref": (MessageObjectRef) & (({ "object_type"?: "proposal"; [key: string]: unknown })); "summary": ((string)); "changed_paths": (Array<MessageRelativePath>) })

// message.schema.json#/$defs/ProgressPayload
export type ProgressPayload = ({ "state": RunState; "summary": ((string)); "delivery_step"?: ("remote_repo" | "remote_ci" | "package") })

// message.schema.json#/$defs/ArtifactManifestPayload
export type ArtifactManifestPayload = ({ "manifest_ref": (MessageObjectRef) & (({ "object_type"?: "manifest"; [key: string]: unknown })); "artifact_count": ((number)); "summary": ((string)) })

// message.schema.json#/$defs/VerificationReportPayload
export type VerificationReportPayload = ({ "report_ref": (MessageObjectRef) & (({ "object_type"?: "verification_report"; [key: string]: unknown })); "result": ("not_ready" | "passed" | "failed"); "summary": ((string)) })

// message.schema.json#/$defs/DiffPart
export type DiffPart = ({ "type"?: "diff"; "object_ref": MessageObjectRef; "summary": ((string)); "changes": (Array<DiffChange>); [key: string]: unknown }) & (PartBase)

// message.schema.json#/$defs/DiffChange
export type DiffChange = ((({ "operation"?: "add"; "before"?: ((null)); "after"?: ((string)); [key: string]: unknown }) | ({ "operation"?: "update"; "before"?: ((string)); "after"?: ((string)); [key: string]: unknown }) | ({ "operation"?: "delete"; "before"?: ((string)); "after"?: ((null)); [key: string]: unknown }))) & ({ "path": MessageRelativePath; "operation": ("add" | "update" | "delete"); "before": ((string | null)); "after": ((string | null)) })

// message.schema.json#/$defs/ChoicePart
export type ChoicePart = ({ "type"?: "choice"; "prompt": ((string)); "selection_mode": ("single" | "multiple"); "choices": (Array<Choice>); [key: string]: unknown }) & (PartBase)

// message.schema.json#/$defs/Choice
export type Choice = ({ "choice_id": Id; "label": ((string)); "description"?: ((string)) })

// message.schema.json#/$defs/ActionPart
export type ActionPart = ({ "type"?: "action"; "label": ((string)); "intent": ActionIntent; [key: string]: unknown }) & (PartBase)

// message.schema.json#/$defs/ActionIntent
export type ActionIntent = ({ "command_id": Id; "action": ("accept" | "reject" | "revise" | "pause" | "resume" | "cancel" | "retry" | "reconcile"); "target": MessageObjectRef; "required_proposal_ref"?: (MessageObjectRef) & (({ "object_type"?: "proposal"; [key: string]: unknown })) }) & (({ "target"?: ({ "object_type"?: "run"; [key: string]: unknown }); [key: string]: unknown }))

// message.schema.json#/$defs/RelativePath
export type MessageRelativePath = ((string))

// message.schema.json#/$defs/JsonValue
export type JsonValue = ((((null)) | ((boolean)) | ((number)) | ((string)) | (Array<JsonValue>) | ({ [key: string]: JsonValue })))
