package agent

import (
	"agentstudio/internal/contracts"
	"time"
)

type Plan struct {
	ProjectName     string   `json:"project_name"`
	Description     string   `json:"description"`
	TechStack       string   `json:"tech_stack"`
	Skills          []string `json:"skills"`
	Database        string   `json:"database"`
	ORM             string   `json:"orm"`
	Auth            string   `json:"auth"`
	TokenTTL        string   `json:"token_ttl"`
	LogLevel        string   `json:"log_level"`
	LogFormat       string   `json:"log_format"`
	LogOutput       string   `json:"log_output"`
	LogPath         string   `json:"log_path"`
	LogMaxSize      int64    `json:"log_max_size"`
	LogMaxBackups   int      `json:"log_max_backups"`
	LogMaxAge       string   `json:"log_max_age"`
	LogOnWriteError string   `json:"log_on_write_error"`
	RepoProvider    string   `json:"repo_provider"`
	RepoVisibility  string   `json:"repo_visibility"`
}
type Check struct {
	Name          string `json:"name"`
	Passed        bool   `json:"passed"`
	Detail        string `json:"detail"`
	Status        string `json:"status,omitempty"`
	SourceVersion int    `json:"source_version,omitempty"`
}
type Change struct {
	Path   string `json:"path"`
	Before string `json:"before"`
	After  string `json:"after"`
}
type Proposal struct {
	ID              string         `json:"id"`
	SourceVersion   int            `json:"source_version"`
	ProposalVersion int            `json:"proposal_version"`
	SnapshotHash    string         `json:"snapshot_hash"`
	DefinitionHash  string         `json:"definition_hash"`
	IssuedAt        string         `json:"issued_at"`
	ExpiresAt       string         `json:"expires_at"`
	DeliveryMode    string         `json:"delivery_mode"`
	Target          ExternalTarget `json:"target"`
	Title           string         `json:"title"`
	Status          string         `json:"status"`
	Changes         []Change       `json:"changes"`
	Impact          []string       `json:"impact"`
	Checks          []Check        `json:"checks"`
}
type Message struct {
	ID         string `json:"id"`
	Role       string `json:"role"`
	Content    string `json:"content"`
	CreatedAt  string `json:"created_at"`
	Kind       string `json:"kind"`
	ProposalID string `json:"proposal_id,omitempty"`
}
type Artifact struct {
	Path   string `json:"path"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256,omitempty"`
}
type History struct {
	ID        string `json:"id"`
	Action    string `json:"action"`
	Version   int    `json:"version"`
	CreatedAt string `json:"created_at"`
	Detail    string `json:"detail"`
}
type Step struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	State  string `json:"state"`
	Detail string `json:"detail"`
}
type ContextState struct {
	Provider        string `json:"provider"`
	RecentTurns     int    `json:"recent_turns"`
	MaxTokens       int    `json:"max_tokens"`
	EstimatedTokens int    `json:"estimated_tokens"`
	Summary         string `json:"summary"`
	Compressed      bool   `json:"compressed"`
	SummaryThrough  int    `json:"summary_through"`
	SummaryCount    int    `json:"summary_count"`
}
type Run struct {
	Control           *ControlState                 `json:"-"`
	FrozenDefinition  *contracts.DefinitionSnapshot `json:"-"`
	ID                string                        `json:"id"`
	Title             string                        `json:"title"`
	State             string                        `json:"state"`
	Version           int                           `json:"version"`
	SourceVersion     int                           `json:"source_version"`
	StateSequence     int                           `json:"state_sequence"`
	Plan              Plan                          `json:"plan"`
	Proposal          *Proposal                     `json:"proposal"`
	Messages          []Message                     `json:"messages"`
	Steps             []Step                        `json:"steps"`
	Artifacts         []Artifact                    `json:"artifacts"`
	History           []History                     `json:"history"`
	Error             string                        `json:"error"`
	ExecutionMode     string                        `json:"execution_mode"`
	DecisionMode      string                        `json:"decision_mode"`
	DeliveryMode      string                        `json:"delivery_mode"`
	Definition        DefinitionSnapshot            `json:"definition_snapshot"`
	Approval          *ApprovalRecord               `json:"approval"`
	Manifest          *ArtifactManifest             `json:"manifest"`
	RequiredChecks    []Check                       `json:"required_checks"`
	ProposalHistory   []Proposal                    `json:"proposal_history"`
	RetryOf           string                        `json:"retry_of,omitempty"`
	PauseRequested    bool                          `json:"pause_requested"`
	CancelRequested   bool                          `json:"cancel_requested"`
	ActiveStep        string                        `json:"active_step,omitempty"`
	Operations        map[string]OperationRecord    `json:"operations"`
	Usage             UsageState                    `json:"usage"`
	RepoURL           string                        `json:"repo_url"`
	PipelineURL       string                        `json:"pipeline_url"`
	Context           ContextState                  `json:"context"`
	CreatedAt         string                        `json:"created_at"`
	UpdatedAt         string                        `json:"updated_at"`
	ResumeTo          string                        `json:"resume_to,omitempty"`
	AcceptedProposal  string                        `json:"accepted_proposal,omitempty"`
	Processed         map[string]string             `json:"-"`
	Workspace         string                        `json:"-"`
	RepoProjectID     int                           `json:"-"`
	PipelineID        int                           `json:"-"`
	CommitSHA         string                        `json:"-"`
	CreateFingerprint string                        `json:"-"`
	CommandIntents    map[string]string             `json:"-"`
	UpdateAttempts    map[string]int                `json:"-"`
}
type Command struct {
	ID              string `json:"id"`
	RunID           string `json:"run_id"`
	Content         string `json:"content,omitempty"`
	Action          string `json:"action,omitempty"`
	ProposalID      string `json:"proposal_id,omitempty"`
	SourceVersion   int    `json:"source_version,omitempty"`
	ProposalVersion int    `json:"proposal_version,omitempty"`
	SnapshotHash    string `json:"snapshot_hash,omitempty"`
}

type CreateOptions struct {
	wire              *publicWireCommand
	DefinitionID      string `json:"definition_id,omitempty"`
	DefinitionVersion string `json:"definition_version,omitempty"`
	DefinitionHash    string `json:"definition_hash,omitempty"`
	ID                string `json:"id"`
	Title             string `json:"title"`
	Content           string `json:"content"`
	Message           string `json:"message,omitempty"`
	ExecutionMode     string `json:"execution_mode,omitempty"`
	DecisionMode      string `json:"decision_mode,omitempty"`
	DeliveryMode      string `json:"delivery_mode,omitempty"`
}
type ExternalTarget struct {
	GitLabURL   string `json:"gitlab_url,omitempty"`
	NamespaceID string `json:"namespace_id,omitempty"`
	ProjectPath string `json:"project_path,omitempty"`
	Ref         string `json:"ref,omitempty"`
}
type DefinitionSnapshot struct {
	FrozenSnapshotHash  string                    `json:"frozen_snapshot_hash,omitempty"`
	ID                  string                    `json:"definition_id"`
	Version             string                    `json:"version"`
	Hash                string                    `json:"hash"`
	TemplateVersion     string                    `json:"template_version"`
	TemplateHash        string                    `json:"template_hash"`
	SkillVersions       map[string]string         `json:"skill_versions"`
	ActionSchemaVersion string                    `json:"action_schema_version"`
	Tools               map[string]string         `json:"tools"`
	AllowedActions      []string                  `json:"allowed_actions"`
	Budget              BudgetPolicy              `json:"budget"`
	DecisionProvider    *DecisionProviderSnapshot `json:"decision_provider,omitempty"`
}
type DecisionProviderSnapshot struct {
	Endpoint string `json:"endpoint"`
	Model    string `json:"model"`
}
type BudgetPolicy struct {
	MaxCommands      int   `json:"max_commands"`
	MaxModelAttempts int   `json:"max_model_attempts"`
	MaxFiles         int   `json:"max_files"`
	MaxArtifactBytes int64 `json:"max_artifact_bytes"`
	MaxStepAttempts  int   `json:"max_step_attempts"`
}
type UsageState struct {
	Commands       int            `json:"commands"`
	ModelAttempts  int            `json:"model_attempts"`
	ModelFailures  int            `json:"model_failures"`
	StepAttempts   map[string]int `json:"step_attempts"`
	LastModelError string         `json:"last_model_error,omitempty"`
}
type ApprovalRecord struct {
	UserID          string   `json:"user_id"`
	ProposalID      string   `json:"proposal_id"`
	ProposalVersion int      `json:"proposal_version"`
	SourceVersion   int      `json:"source_version"`
	SnapshotHash    string   `json:"snapshot_hash"`
	DefinitionHash  string   `json:"definition_hash"`
	IssuedAt        string   `json:"issued_at"`
	ExpiresAt       string   `json:"expires_at"`
	AcceptedAt      string   `json:"accepted_at"`
	StartedAt       string   `json:"started_at,omitempty"`
	Actions         []string `json:"actions"`
}
type ArtifactManifest struct {
	RunID           string     `json:"run_id"`
	SourceVersion   int        `json:"source_version"`
	ProposalID      string     `json:"proposal_id"`
	ProposalVersion int        `json:"proposal_version"`
	SnapshotHash    string     `json:"snapshot_hash"`
	DefinitionHash  string     `json:"definition_hash"`
	PlanHash        string     `json:"plan_hash"`
	Files           []Artifact `json:"files"`
	Hash            string     `json:"hash"`
	CreatedAt       string     `json:"created_at"`
}
type OperationRecord struct {
	ID            string `json:"id"`
	Kind          string `json:"kind"`
	SourceVersion int    `json:"source_version"`
	Fingerprint   string `json:"fingerprint"`
	Status        string `json:"status"`
	ExternalID    string `json:"external_id,omitempty"`
}
type CommandResult struct {
	Run     Run  `json:"run"`
	Execute bool `json:"execute"`
}
type Skill struct {
	ID          string              `json:"id"`
	Name        string              `json:"name"`
	Description string              `json:"description"`
	Parameters  map[string][]string `json:"parameters"`
}

var Skills = []Skill{
	{ID: "add_database", Name: "数据库", Description: "数据库连接、迁移和 ORM 配置", Parameters: map[string][]string{"database": {"postgres", "mysql", "sqlite"}, "orm": {"gorm", "sqlx"}}},
	{ID: "add_auth", Name: "身份认证", Description: "JWT 或 session 认证与凭据校验", Parameters: map[string][]string{"auth": {"jwt", "session"}}},
	{ID: "add_logging", Name: "结构化日志", Description: "slog 控制台/文件日志、受控轮转与写入失败策略", Parameters: map[string][]string{"log_level": {"debug", "info", "warn", "error"}, "log_format": {"json", "text"}, "log_output": {"console", "file"}, "log_on_write_error": {"degrade", "fail_closed"}}},
}

func now() string { return time.Now().UTC().Format(time.RFC3339Nano) }
func defaultPlan() Plan {
	return Plan{TechStack: "golang", Skills: []string{}, Database: "postgres", ORM: "gorm", Auth: "jwt", TokenTTL: "24h", LogLevel: "info", LogFormat: "json", LogOutput: "console", LogPath: "logs/app.log", LogMaxSize: 10 << 20, LogMaxBackups: 3, LogMaxAge: "168h", LogOnWriteError: "degrade", RepoProvider: "local", RepoVisibility: "private"}
}
func initialSteps() []Step {
	return []Step{{ID: "scaffold", Title: "创建 Go 项目骨架", State: "pending"}, {ID: "skills", Title: "应用能力模板", State: "pending"}, {ID: "verify", Title: "执行测试与构建", State: "pending"}, {ID: "repo", Title: "创建并推送 GitLab 仓库", State: "pending"}, {ID: "ci", Title: "触发并验证 CI", State: "pending"}, {ID: "deliver", Title: "打包交付产物", State: "pending"}}
}
