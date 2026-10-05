package agent

import (
	"agentstudio/internal/contracts"
	"agentstudio/internal/definitions"
	"agentstudio/internal/governance"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/temporal"
)

type Config struct {
	DataDir           string
	Mode              string
	DecisionMode      string
	DeliveryMode      string
	ApprovalTTL       time.Duration
	Address           string
	TemporalAddress   string
	TemporalNamespace string
	TaskQueue         string
	GitLabURL         string
	GitLabToken       string
	GitLabNamespace   string
	AIEndpoint        string
	AIKey             string
	AIModel           string
}

func EnvConfig() Config {
	c := Config{DataDir: os.Getenv("AGENT_DATA_DIR"), Mode: os.Getenv("AGENT_MODE"), Address: os.Getenv("HTTP_ADDR"), TemporalAddress: os.Getenv("TEMPORAL_ADDRESS"), TemporalNamespace: os.Getenv("TEMPORAL_NAMESPACE"), TaskQueue: os.Getenv("TEMPORAL_TASK_QUEUE"), GitLabURL: os.Getenv("GITLAB_URL"), GitLabToken: os.Getenv("GITLAB_TOKEN"), GitLabNamespace: os.Getenv("GITLAB_NAMESPACE_ID"), AIEndpoint: os.Getenv("AI_BASE_URL"), AIKey: os.Getenv("AI_API_KEY"), AIModel: os.Getenv("AI_MODEL")}
	if explicit := os.Getenv("AGENT_EXECUTION_MODE"); explicit != "" {
		c.Mode = explicit
	}
	c.DecisionMode = os.Getenv("AGENT_DECISION_MODE")
	c.DeliveryMode = os.Getenv("AGENT_DELIVERY_MODE")
	c.ApprovalTTL = 30 * time.Minute
	if value := os.Getenv("AGENT_APPROVAL_TTL"); value != "" {
		duration, e := time.ParseDuration(value)
		if e != nil {
			c.ApprovalTTL = -1
		} else {
			c.ApprovalTTL = duration
		}
	}
	if c.DataDir == "" {
		c.DataDir = ".data"
	}
	if c.Mode == "" {
		c.Mode = "temporal"
	}
	if c.Address == "" {
		c.Address = "127.0.0.1:8080"
	}
	if c.TemporalAddress == "" {
		c.TemporalAddress = "127.0.0.1:7233"
	}
	if c.TemporalNamespace == "" {
		c.TemporalNamespace = "default"
	}
	if c.TaskQueue == "" {
		c.TaskQueue = "project-init-agent"
	}
	if c.GitLabURL == "" {
		c.GitLabURL = "https://gitlab.com"
	}
	return c
}

type Service struct {
	runtime           *runtimeMonitor
	Definitions       *definitions.Registry
	DefaultDefinition contracts.AgentDefinition
	Store             *Store
	Config            Config
	Temporal          client.Client
	mu                sync.Mutex
	active            map[string]context.CancelFunc
	commandLocks      map[string]*sync.Mutex
	createLocks       map[string]*sync.Mutex
	localWG           sync.WaitGroup
}

func New(cfg Config) (*Service, error) {
	if cfg.DecisionMode == "" {
		cfg.DecisionMode = "rules"
	}
	if cfg.DeliveryMode == "" {
		cfg.DeliveryMode = "local"
	}
	if cfg.ApprovalTTL == 0 {
		cfg.ApprovalTTL = 30 * time.Minute
	}
	if cfg.DecisionMode != "rules" && cfg.DecisionMode != "model" {
		return nil, fail("invalid_mode", "AGENT_DECISION_MODE must be rules or model")
	}
	if cfg.DeliveryMode != "local" && cfg.DeliveryMode != "gitlab" {
		return nil, fail("invalid_mode", "AGENT_DELIVERY_MODE must be local or gitlab")
	}
	if cfg.ApprovalTTL < time.Minute || cfg.ApprovalTTL > 24*time.Hour {
		return nil, fail("invalid_expiry", "AGENT_APPROVAL_TTL must be 1m–24h")
	}
	if cfg.GitLabURL == "" {
		cfg.GitLabURL = "https://gitlab.com"
	}
	for _, endpoint := range []string{cfg.GitLabURL, cfg.AIEndpoint} {
		if endpoint != "" {
			u, e := url.Parse(endpoint)
			if e != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
				return nil, fail("invalid_endpoint", "服务端点只允许http(s)地址，禁止userinfo/query/fragment凭据")
			}
		}
	}
	if cfg.Mode != "local" && cfg.Mode != "temporal" {
		return nil, fmt.Errorf("AGENT_MODE must be local or temporal")
	}
	dir, err := filepath.Abs(cfg.DataDir)
	if err != nil {
		return nil, err
	}
	cfg.DataDir = dir
	store, err := NewStore(dir)
	if err != nil {
		return nil, err
	}
	s := &Service{runtime: &runtimeMonitor{instance: newID("service_"), beats: map[string]string{}}, Store: store, Config: cfg, active: map[string]context.CancelFunc{}, commandLocks: map[string]*sync.Mutex{}, createLocks: map[string]*sync.Mutex{}}
	if err = s.initDefinitions(); err != nil {
		store.Close()
		return nil, err
	}
	if err = s.recoverRuntimeRecords(); err != nil {
		store.Close()
		return nil, err
	}
	for _, r := range store.List() {
		if r.Control != nil && controlHasPendingCommands(r) {
			if _, err = store.Update(r.ID, func(run *Run) error { return recoverControlCommandBudgets(run, time.Now().UTC()) }); err != nil {
				store.Close()
				return nil, err
			}
		}
	}
	if cfg.Mode == "local" {
		for _, r := range store.List() {
			if r.ExecutionMode == "local" && (r.State == "IMPLEMENT" || r.State == "VERIFY" || r.State == "DELIVER") {
				_, err = store.Update(r.ID, func(run *Run) error {
					run.ResumeTo = run.State
					run.ActiveStep = ""
					run.State = "BLOCKED"
					run.Error = "本地执行进程已中断；检查已生成产物后点击恢复。此模式不具备 Temporal 持久工作流语义。"
					history(run, "interrupted", run.Error)
					return nil
				})
				if err != nil {
					store.Close()
					return nil, err
				}
			}
		}
	}
	return s, nil
}
func (s *Service) Provider() string {
	if s.Config.DecisionMode == "model" {
		return "openai-compatible"
	}
	return "rules"
}
func (s *Service) ModelConfigured() bool {
	return s.Config.AIEndpoint != "" && s.Config.AIKey != "" && s.Config.AIModel != ""
}
func (s *Service) Create(ctx context.Context, id, title, content string) (Run, error) {
	return s.CreateWithOptions(ctx, CreateOptions{ID: id, Title: title, Content: content})
}
func (s *Service) CreateWithOptions(ctx context.Context, opts CreateOptions) (Run, error) {
	return s.createRun(ctx, opts, nil, "")
}
func (s *Service) createRun(ctx context.Context, opts CreateOptions, plan *Plan, retryOf string) (Run, error) {
	return s.createRunWithControlRetry(ctx, opts, plan, nil, retryOf)
}

func (s *Service) createRunWithControlRetry(ctx context.Context, opts CreateOptions, plan *Plan, retry *controlRetryOrigin, retryOf string) (Run, error) {
	if err := s.acceptingWork(); err != nil {
		return Run{}, err
	}
	if opts.ExecutionMode == "" {
		opts.ExecutionMode = s.Config.Mode
	}
	if opts.ExecutionMode != s.Config.Mode {
		return Run{}, fail("invalid_mode", "execution_mode must match the server mode")
	}
	if opts.DecisionMode == "" {
		opts.DecisionMode = s.Config.DecisionMode
	}
	if opts.DecisionMode != "rules" && opts.DecisionMode != "model" {
		return Run{}, fail("invalid_mode", "decision_mode must be rules or model")
	}
	if opts.DeliveryMode == "" {
		opts.DeliveryMode = s.Config.DeliveryMode
	}
	if opts.DeliveryMode != "local" && opts.DeliveryMode != "gitlab" {
		return Run{}, fail("invalid_mode", "delivery_mode must be local or gitlab")
	}
	id, title, content := opts.ID, opts.Title, opts.Content
	if content == "" {
		content = opts.Message
	}
	if s.containsSecret(content) || s.containsSecret(title) {
		return Run{}, fail("secret_input", "消息包含服务端凭据，拒绝持久化")
	}
	fingerprint := digest(struct {
		Options CreateOptions
		Plan    *Plan
		RetryOf string
	}{opts, plan, retryOf})
	if retry != nil {
		fingerprint = digest(struct {
			Options      CreateOptions
			ControlRetry controlRetryOrigin
			RetryOf      string
		}{opts, *retry, retryOf})
	}
	if opts.wire != nil {
		fingerprint = opts.wire.Fingerprint
	}
	if id == "" {
		id = newID("run_")
	}
	if !safeID.MatchString(id) {
		return Run{}, fail("invalid_id", "invalid run ID")
	}
	s.mu.Lock()
	createLock := s.createLocks[id]
	if createLock == nil {
		createLock = &sync.Mutex{}
		s.createLocks[id] = createLock
	}
	s.mu.Unlock()
	createLock.Lock()
	defer createLock.Unlock()
	if title == "" {
		title = "新的项目创建任务"
	}
	if old, e := s.Store.Get(id); e == nil {
		if old.CreateFingerprint != fingerprint {
			return Run{}, fail("idempotency_conflict", "同一Run ID用于不同创建请求")
		}
		if content != "" && !terminal(old.State) {
			initial := initialCreateCommand(opts, id, content)
			if _, done := old.Processed[initial.ID]; !done {
				return s.Submit(ctx, initial)
			}
		}
		return old, nil
	}
	if s.Config.Mode == "temporal" && s.Temporal == nil {
		return Run{}, fail("temporal_unavailable", "所选Temporal模式未就绪，不会切换为local")
	}
	frozen, definitionErr := s.freezeDefinition(opts)
	if definitionErr != nil {
		return Run{}, definitionErr
	}
	if frozen.ContractVersion != "1.1.0" && !contracts.DeliveryContract(frozen.ContractVersion) {
		return Run{}, fail("definition_not_ready", "所选定义暂不支持创建新Run，请使用当前可用发布")
	}
	if plan != nil && frozen.ContractVersion != "1.1.0" || retry != nil && !contracts.DeliveryContract(frozen.ContractVersion) {
		return Run{}, fail("definition_not_ready", "重做必须沿原完整契约初始化，不能通过legacy计划降级")
	}
	definition := legacyDefinition(frozen)
	r := Run{ID: id, Title: title, State: "INTAKE", Version: 1, SourceVersion: 1, StateSequence: 1, Plan: defaultPlan(), Messages: []Message{}, Steps: initialSteps(), Artifacts: []Artifact{}, History: []History{}, ExecutionMode: s.Config.Mode, DecisionMode: opts.DecisionMode, DeliveryMode: opts.DeliveryMode, Definition: definition, Operations: map[string]OperationRecord{}, ProposalHistory: []Proposal{}, RetryOf: retryOf, CreateFingerprint: fingerprint, CreatedAt: now(), UpdatedAt: now(), Processed: map[string]string{}, Context: ContextState{Provider: opts.DecisionMode, RecentTurns: 10, MaxTokens: 8000}}
	r.Plan.RepoProvider = r.DeliveryMode
	r.FrozenDefinition = &frozen
	r.RequiredChecks = requiredChecksFor(r)
	r.Plan, definitionErr = s.defaultFrozenPlan(frozen)
	if definitionErr != nil {
		return Run{}, definitionErr
	}
	r.Plan.RepoProvider = r.DeliveryMode
	r.Context.RecentTurns = int(frozen.Definition.ContextPolicy.RecentMessages)
	r.Context.MaxTokens = int(frozen.Definition.ContextPolicy.MaxTokens)
	if contracts.DeliveryContract(frozen.ContractVersion) {
		budget, e := governance.NewBudgetLedgerVersion(frozen.ContractVersion, r.ID, frozen.SnapshotHash, frozen.Definition.BudgetPolicy, time.Now().UTC())
		if e != nil {
			return Run{}, e
		}
		r.Control, e = newControlState(r, budget)
		if e != nil {
			return Run{}, e
		}
		if opts.wire != nil {
			if r.Control.Conversation == nil {
				return Run{}, fail("definition_not_ready", "v2 creation requires a complete conversation contract")
			}
			r.Control.Conversation.WireCommands = map[string]publicWireCommand{opts.wire.Command.ID: *opts.wire}
			if e = acceptConversationCommand(&r, opts.wire.Command); e != nil {
				return Run{}, e
			}
		}
		if retry != nil {
			if e = s.initializeControlRetry(&r, *retry); e != nil {
				return Run{}, e
			}
		}
		syncBudgetProjection(&r)
	}
	addMessage(&r, "assistant", "告诉我项目名称和需要的能力。例如：创建 order-service，使用 PostgreSQL + GORM、JWT 认证和 JSON 日志。我会先展示文件预览，确认后才生成项目。", "text", "")
	history(&r, "created", "独立 Run 已创建，从 INTAKE 开始")
	if plan != nil {
		r.Plan = *plan
		r.State = "PLAN"
		if err := s.makeProposal(&r); err != nil {
			return Run{}, err
		}
		history(&r, "retry_of", retryOf)
		addMessage(&r, "assistant", "已创建独立重做任务，请审查新的方案并确认后执行。", "proposal", r.Proposal.ID)
	}
	var err error
	r, err = s.Store.Create(r)
	if err != nil {
		return r, err
	}
	if s.Config.Mode == "temporal" {
		if s.Temporal == nil {
			return r, fail("temporal_unavailable", "Temporal client is unavailable")
		}
		_, err = s.Temporal.ExecuteWorkflow(ctx, client.StartWorkflowOptions{ID: workflowID(id), TaskQueue: s.Config.TaskQueue}, ProjectWorkflow, WorkflowInput{RunID: id, State: r.State, SourceVersion: r.SourceVersion, StateSequence: r.StateSequence})
		if err != nil {
			_, _ = s.Store.Update(id, func(r *Run) error { r.State = "BLOCKED"; r.Error = "Temporal 无法启动 Workflow"; return nil })
			return r, fail("temporal_unavailable", "无法启动 Temporal Workflow")
		}
	}
	if content != "" {
		return s.Submit(ctx, initialCreateCommand(opts, id, content))
	}
	return r, nil
}
func (s *Service) Submit(ctx context.Context, cmd Command) (Run, error) {
	if err := s.acceptingWork(); err != nil {
		return Run{}, err
	}
	if !safeID.MatchString(cmd.ID) {
		return Run{}, fail("invalid_message_id", "操作需要稳定的 id（字母、数字、连字符或下划线）")
	}
	// Temporal's Update ID deduplication also needs an application-level
	// payload check: a reused ID must never disguise different content.
	if current, err := s.Store.Get(cmd.RunID); err == nil {
		if current.ExecutionMode != s.Config.Mode {
			return Run{}, fail("run_mode_mismatch", "历史Run执行模式与当前服务不一致；保持只读，不能切换执行器")
		}
		if prior, ok := current.Processed[cmd.ID]; ok {
			payload, _ := json.Marshal(cmd)
			if prior != string(payload) {
				return Run{}, fail("idempotency_conflict", "同一个操作 ID 已用于不同内容")
			}
			if cmd.Action == "retry" {
				return s.Store.Get(retryRunID(cmd.RunID, cmd.ID))
			}
			return current, nil
		}
	}
	if s.Config.Mode == "temporal" {
		if s.Temporal == nil {
			return Run{}, fail("temporal_unavailable", "Temporal client is unavailable")
		}
		bound, e := s.bindCommand(cmd)
		if e != nil {
			return Run{}, e
		}
		attempt := bound.UpdateAttempts[cmd.ID]
		updateID := cmd.ID + "_attempt_" + strconv.Itoa(attempt)
		h, e := s.Temporal.UpdateWorkflow(ctx, client.UpdateWorkflowOptions{WorkflowID: workflowID(cmd.RunID), UpdateID: updateID, UpdateName: "submit", Args: []interface{}{cmd}, WaitForStage: client.WorkflowUpdateStageCompleted})
		if e != nil {
			s.recordUpdateFailure(cmd, attempt, e)
			return Run{}, fail("workflow_update_failed", "Workflow 无法接收该操作，请查询任务状态后重试")
		}
		var result CommandResult
		if e = h.Get(ctx, &result); e != nil {
			s.recordUpdateFailure(cmd, attempt, e)
			return Run{}, e
		}
		return s.Store.Get(result.Run.ID)
	}
	result, err := s.ProcessCommand(ctx, cmd)
	if err != nil {
		return result.Run, err
	}
	if result.Execute {
		s.StartLocal(cmd.RunID)
	}
	return result.Run, nil
}
func (s *Service) processCommand(ctx context.Context, cmd Command) (CommandResult, error) {
	if !safeID.MatchString(cmd.ID) || !safeID.MatchString(cmd.RunID) {
		return CommandResult{}, fail("invalid_identity", "操作与Run需要稳定合法身份")
	}
	if s.containsSecret(cmd.Content) {
		return CommandResult{}, fail("secret_input", "消息包含服务端凭据，拒绝持久化")
	}
	s.mu.Lock()
	lock := s.commandLocks[cmd.RunID]
	if lock == nil {
		lock = &sync.Mutex{}
		s.commandLocks[cmd.RunID] = lock
	}
	s.mu.Unlock()
	lock.Lock()
	defer lock.Unlock()
	current, err := s.Store.Get(cmd.RunID)
	if err != nil {
		return CommandResult{}, err
	}
	if current.ExecutionMode != s.Config.Mode {
		return CommandResult{}, fail("run_mode_mismatch", "历史Run执行模式与当前服务不一致；保持只读，不能切换执行器")
	}
	fingerprint, _ := json.Marshal(cmd)
	fp := string(fingerprint)
	if previous, ok := current.Processed[cmd.ID]; ok {
		if previous != fp {
			return CommandResult{}, fail("idempotency_conflict", "同一操作ID用于不同内容")
		}
		if cmd.Action == "retry" {
			next, e := s.Store.Get(retryRunID(cmd.RunID, cmd.ID))
			return CommandResult{Run: next}, e
		}
		return CommandResult{Run: current}, nil
	}
	if current.Control != nil {
		return s.processControlCommand(ctx, cmd)
	}
	if _, err = s.bindCommand(cmd); err != nil {
		return CommandResult{}, err
	}
	current, err = s.Store.Update(current.ID, func(r *Run) error {
		if r.Usage.Commands >= r.Definition.Budget.MaxCommands {
			return fail("budget_exceeded", "Run命令预算已耗尽")
		}
		r.Usage.Commands++
		return nil
	})
	if err != nil {
		return CommandResult{}, err
	}
	if cmd.Action == "" && progressQuery(cmd.Content) {
		r, e := s.Store.Update(cmd.RunID, func(r *Run) error {
			r.Processed[cmd.ID] = fp
			r.Messages = append(r.Messages, Message{ID: cmd.ID, Role: "user", Content: cmd.Content, Kind: "text", CreatedAt: now()})
			addMessage(r, "assistant", fmt.Sprintf("当前阶段：%s；%d个产物；执行/决策/交付：%s/%s/%s。", r.State, len(r.Artifacts), r.ExecutionMode, r.DecisionMode, r.DeliveryMode), "text", "")
			compactContext(r)
			return nil
		})
		return CommandResult{Run: r}, e
	}
	if cmd.Action == "retry" {
		if !terminal(current.State) {
			return CommandResult{}, fail("invalid_state", "重做仅适用于终止Run")
		}
		next, e := s.createRun(ctx, CreateOptions{ID: retryRunID(current.ID, cmd.ID), Title: current.Title + " · 重做", DecisionMode: current.DecisionMode, DeliveryMode: current.DeliveryMode}, &current.Plan, current.ID)
		if e != nil {
			return CommandResult{}, e
		}
		_, e = s.Store.Update(current.ID, func(r *Run) error { r.Processed[cmd.ID] = fp; history(r, "retried", next.ID); return nil })
		return CommandResult{Run: next}, e
	}
	if terminal(current.State) {
		return CommandResult{}, fail("terminal_run", "Run已终止，请重做创建关联新Run")
	}
	var plan Plan
	var explanation string
	if cmd.Action == "" {
		if strings.TrimSpace(cmd.Content) == "" || len(cmd.Content) > 16000 {
			return CommandResult{}, fail("invalid_message", "消息须为1–16000字节")
		}
		if current.AcceptedProposal != "" || current.State == "IMPLEMENT" || current.State == "VERIFY" || current.State == "DELIVER" || current.State == "PAUSED" || current.State == "BLOCKED" {
			return CommandResult{}, fail("run_busy", "执行/暂停中的Run不可直接改方案；请恢复、取消或重做")
		}
		if current.DecisionMode == "model" {
			if !s.ModelConfigured() {
				return CommandResult{}, fail("model_unavailable", "所选model模式缺完整provider，不会降级规则")
			}
			current, err = s.Store.Update(current.ID, func(r *Run) error {
				if r.Usage.ModelAttempts >= r.Definition.Budget.MaxModelAttempts {
					return fail("budget_exceeded", "模型尝试预算耗尽，包含失败尝试")
				}
				r.Usage.ModelAttempts++
				history(r, "model_attempt", cmd.ID)
				return nil
			})
			if err != nil {
				return CommandResult{}, err
			}
			plan, explanation, err = s.ModelPlan(ctx, current, cmd.Content)
			if err != nil {
				_, _ = s.Store.Update(current.ID, func(r *Run) error {
					r.Usage.ModelFailures++
					r.Usage.LastModelError = s.redact(err.Error())
					history(r, "model_failed", r.Usage.LastModelError)
					return nil
				})
				return CommandResult{}, err
			}
		} else if current.DecisionMode == "rules" {
			plan = ParsePlan(current.Plan, cmd.Content)
			plan, err = s.normalizeFrozenPlan(current, plan)
			if err != nil {
				return CommandResult{}, err
			}
			explanation = "已按显式rules模式解析需求；未调用AI。"
		} else {
			return CommandResult{}, fail("invalid_mode", "Run决策模式无效")
		}
	}
	execute := false
	r, err := s.Store.Update(cmd.RunID, func(r *Run) error {
		if cmd.Action == "" {
			r.Messages = append(r.Messages, Message{ID: cmd.ID, Role: "user", Content: cmd.Content, Kind: "text", CreatedAt: now()})
			r.Plan = plan
			r.Title = plan.ProjectName
			if r.Title == "" {
				r.Title = "项目参数澄清"
			}
			sourceChanged(r)
			r.Error = ""
			r.DeliveryMode = plan.RepoProvider
			r.Context.Provider = r.DecisionMode
			r.RequiredChecks = requiredChecksFor(*r)
			if plan.ProjectName == "" {
				r.State = "CLARIFY"
				r.Proposal = nil
				addMessage(r, "assistant", explanation+"\n请提供小写英文项目名，例如 order-service。", "text", "")
			} else {
				if e := s.makeProposal(r); e != nil {
					return e
				}
				r.State = "PLAN"
				addMessage(r, "assistant", explanation+"\n先审查文件、参数与目标；确认后生成真实项目并测试构建。", "proposal", r.Proposal.ID)
			}
			compactContext(r)
			history(r, "message", explanation)
		} else {
			switch cmd.Action {
			case "accept", "reject", "revise":
				if e := commandBinding(*r, cmd); e != nil {
					return e
				}
				if r.Proposal.Status != "pending" {
					if cmd.Action == "accept" && r.Proposal.Status == "accepted" {
						r.Processed[cmd.ID] = fp
						return nil
					}
					return fail("proposal_closed", "提案已经关闭")
				}
				if r.State != "PLAN" {
					return fail("invalid_state", "确认仅适用于PLAN")
				}
				if cmd.Action == "accept" {
					if r.FrozenDefinition == nil {
						return fail("legacy_contract_incomplete", "旧Run未冻结完整来源，不能新增批准；请创建关联新Run")
					}
					if e := s.verifyFrozenDefinition(*r); e != nil {
						return e
					}
					expires, e := time.Parse(time.RFC3339Nano, r.Proposal.ExpiresAt)
					if e != nil || !time.Now().UTC().Before(expires) {
						return fail("proposal_expired", "方案已过期，请修订重新预览")
					}
					for _, check := range r.Proposal.Checks {
						if !check.Passed {
							return fail("proposal_blocked", "方案规则未通过")
						}
					}
					r.Proposal.Status = "accepted"
					r.AcceptedProposal = r.Proposal.ID
					r.Approval = &ApprovalRecord{UserID: "local-user", ProposalID: r.Proposal.ID, ProposalVersion: r.Proposal.ProposalVersion, SourceVersion: r.SourceVersion, SnapshotHash: r.Proposal.SnapshotHash, DefinitionHash: r.Definition.Hash, IssuedAt: r.Proposal.IssuedAt, ExpiresAt: r.Proposal.ExpiresAt, AcceptedAt: now(), Actions: append([]string{}, r.Definition.AllowedActions...)}
					r.State = "IMPLEMENT"
					r.Error = ""
					execute = true
					addMessage(r, "assistant", "方案已确认。首次执行前会再次校验快照、来源与服务端有效期。", "text", r.Proposal.ID)
				} else {
					r.Proposal.Status = "rejected"
					if cmd.Action == "revise" {
						r.Proposal.Status = "revision_requested"
					}
					r.State = "CLARIFY"
					sourceChanged(r)
					addMessage(r, "assistant", "已关闭未应用提案。补充要求后将生成新的版本与预览。", "text", r.Proposal.ID)
				}
			case "pause":
				if r.State == "PAUSED" || r.State == "BLOCKED" {
					return fail("invalid_state", "Run已暂停或阻塞")
				}
				if r.ActiveStep != "" {
					r.PauseRequested = true
					addMessage(r, "assistant", "暂停请求已登记，将在当前操作安全边界生效。", "text", "")
				} else {
					r.ResumeTo = r.State
					r.State = "PAUSED"
					addMessage(r, "assistant", "已暂停，保留已完成事实与原批准范围。", "text", "")
				}
			case "resume":
				if r.State != "PAUSED" && r.State != "BLOCKED" {
					return fail("invalid_state", "只有暂停或阻塞Run可恢复")
				}
				if r.ActiveStep != "" {
					return fail("run_busy", "原操作仍在执行，请等待安全边界")
				}
				if r.AcceptedProposal != "" {
					if e := s.verifyApproval(*r, false); e != nil {
						return e
					}
					r.State = r.ResumeTo
					if r.State == "" {
						r.State = "IMPLEMENT"
					}
					execute = true
				} else {
					r.State = r.ResumeTo
					if r.State == "" {
						r.State = "CLARIFY"
					}
				}
				r.Error = ""
				r.PauseRequested = false
				r.CancelRequested = false
				r.ResumeTo = ""
				addMessage(r, "assistant", "恢复原阶段；未知外部操作仍按原身份核实后继续。", "text", "")
			case "cancel":
				if r.ActiveStep != "" {
					r.CancelRequested = true
					addMessage(r, "assistant", "取消请求已登记；当前操作核实后终止，不声明回滚。", "text", "")
				} else {
					r.State = "CANCELLED"
					r.Error = "用户取消"
					r.PauseRequested = false
					addMessage(r, "assistant", "任务已取消，保留所有已发生副作用与未知结果记录。", "text", "")
				}
			default:
				return fail("invalid_action", "unknown action")
			}
			history(r, cmd.Action, "动作已按当前身份、状态、版本及批准范围校验")
		}
		r.Processed[cmd.ID] = fp
		return nil
	})
	return CommandResult{Run: r, Execute: execute}, err
}
func retryRunID(runID, commandID string) string {
	hash := sha256.Sum256([]byte(runID + "/retry/" + commandID))
	return fmt.Sprintf("run_%x", hash[:12])
}
func (s *Service) makeProposal(r *Run) error {
	files, e := s.renderFrozenFiles(*r)
	if e != nil {
		return e
	}
	version := 1
	if r.Proposal != nil {
		version = r.Proposal.ProposalVersion + 1
		r.ProposalHistory = append(r.ProposalHistory, *r.Proposal)
	}
	issued := time.Now().UTC()
	r.Proposal = &Proposal{ID: newID("proposal_"), SourceVersion: r.SourceVersion, ProposalVersion: version, DefinitionHash: r.Definition.Hash, IssuedAt: issued.Format(time.RFC3339Nano), ExpiresAt: issued.Add(s.Config.ApprovalTTL).Format(time.RFC3339Nano), DeliveryMode: r.DeliveryMode, Target: s.target(*r), Title: r.Plan.ProjectName + " 项目创建方案", Status: "pending", Changes: changesFor(files), Impact: []string{"仅写入该 Run 的隔离工作区", "应用选定技能，不注入默认密钥", "允许 go mod tidy 产生锁定依赖元数据，源代码使用批准快照", "运行 go test ./... 与 go build ./...", "生成可下载 ZIP 与 GitLab CI 配置"}, Checks: []Check{{Name: "plan_schema", Passed: true, Detail: "项目名、技能与参数通过确定性校验"}, {Name: "secret_boundary", Passed: true, Detail: "密钥只从执行器环境读取，不进入模板或 Workflow 参数"}}}
	r.Proposal.SnapshotHash = proposalDigest(*r, *r.Proposal)
	if e := checkProposalBudget(*r); e != nil {
		return e
	}
	if r.Plan.RepoProvider == "gitlab" {
		r.Proposal.Impact = append(r.Proposal.Impact, "将创建 private GitLab 仓库并提交文件、触发真实流水线")
	}
	return nil
}

var namePattern = regexp.MustCompile(`(?i)(?:项目名(?:称)?|名字|命名为|叫|named|name|project_name|project)\s*(?:[:：=]|为|是)?\s*[\x{201c}\x{201d}"'\x60]?([a-z][a-z0-9-]{1,62})`)
var slugPattern = regexp.MustCompile(`\b[a-z][a-z0-9]*(?:-[a-z0-9]+)+\b`)

func ParsePlan(p Plan, text string) Plan {
	t := strings.ToLower(text)
	if match := namePattern.FindStringSubmatch(t); len(match) > 1 {
		candidate := match[1]
		if candidate != "golang" && candidate != "go" {
			p.ProjectName = candidate
		}
	}
	if p.ProjectName == "" {
		if x := slugPattern.FindString(t); x != "" {
			p.ProjectName = x
		} else if projectName.MatchString(strings.TrimSpace(t)) {
			p.ProjectName = strings.TrimSpace(t)
		}
	}
	p.TechStack = "golang"
	p.Description = text
	add := func(id string) {
		if !hasSkill(p, id) {
			p.Skills = append(p.Skills, id)
		}
	}
	remove := func(id string) {
		out := []string{}
		for _, s := range p.Skills {
			if s != id {
				out = append(out, s)
			}
		}
		p.Skills = out
	}
	if strings.Contains(t, "数据库") || strings.Contains(t, "database") || strings.Contains(t, "postgres") || strings.Contains(t, "mysql") || strings.Contains(t, "sqlite") || strings.Contains(t, "gorm") || strings.Contains(t, "sqlx") {
		add("add_database")
	}
	if strings.Contains(t, "认证") || strings.Contains(t, "鉴权") || strings.Contains(t, "jwt") || strings.Contains(t, "session") || strings.Contains(t, "auth") {
		add("add_auth")
	}
	if strings.Contains(t, "日志") || strings.Contains(t, "logging") || strings.Contains(t, "slog") {
		add("add_logging")
	}
	if strings.Contains(t, "全部能力") || strings.Contains(t, "全部技能") || strings.Contains(t, "all skills") {
		add("add_database")
		add("add_auth")
		add("add_logging")
	}
	if strings.Contains(t, "不要数据库") || strings.Contains(t, "取消数据库") || strings.Contains(t, "no database") {
		remove("add_database")
	}
	if strings.Contains(t, "不要认证") || strings.Contains(t, "取消认证") || strings.Contains(t, "no auth") {
		remove("add_auth")
	}
	if strings.Contains(t, "不要日志") || strings.Contains(t, "取消日志") || strings.Contains(t, "no logging") {
		remove("add_logging")
	}
	for _, v := range []string{"postgres", "mysql", "sqlite"} {
		if strings.Contains(t, v) {
			p.Database = v
		}
	}
	if strings.Contains(t, "postgresql") {
		p.Database = "postgres"
	}
	for _, v := range []string{"gorm", "sqlx"} {
		if strings.Contains(t, v) {
			p.ORM = v
		}
	}
	for _, v := range []string{"jwt", "session"} {
		if strings.Contains(t, v) {
			p.Auth = v
		}
	}
	for _, v := range []string{"debug", "info", "warn", "error"} {
		if strings.Contains(t, v) {
			p.LogLevel = v
		}
	}
	if strings.Contains(t, "json") {
		p.LogFormat = "json"
	}
	if strings.Contains(t, "text 日志") || strings.Contains(t, "text logging") {
		p.LogFormat = "text"
	}
	if strings.Contains(t, "file 日志") || strings.Contains(t, "日志文件") || strings.Contains(t, "file logging") || strings.Contains(t, "文件日志") {
		p.LogOutput = "file"
	}
	if strings.Contains(t, "console logging") || strings.Contains(t, "控制台日志") {
		p.LogOutput = "console"
	}
	if strings.Contains(t, "fail_closed") {
		p.LogOnWriteError = "fail_closed"
	}
	if m := regexp.MustCompile(`log_path\s*[:：=]\s*([a-z0-9_./-]+)`).FindStringSubmatch(t); len(m) > 1 {
		p.LogPath = m[1]
	}
	if m := regexp.MustCompile(`log_max_size\s*[:：=]\s*(\d+)`).FindStringSubmatch(t); len(m) > 1 {
		p.LogMaxSize, _ = strconv.ParseInt(m[1], 10, 64)
	}
	if m := regexp.MustCompile(`log_max_backups\s*[:：=]\s*(\d+)`).FindStringSubmatch(t); len(m) > 1 {
		p.LogMaxBackups, _ = strconv.Atoi(m[1])
	}
	if m := regexp.MustCompile(`log_max_age\s*[:：=]\s*([0-9]+[hms]+)`).FindStringSubmatch(t); len(m) > 1 {
		p.LogMaxAge = m[1]
	}
	if strings.Contains(t, "gitlab") {
		p.RepoProvider = "gitlab"
	}
	if strings.Contains(t, "public") || strings.Contains(t, "公开仓库") {
		p.RepoVisibility = "public"
	}
	if strings.Contains(t, "仅本地") || strings.Contains(t, "本地交付") || strings.Contains(t, "local only") {
		p.RepoProvider = "local"
	}
	if m := regexp.MustCompile(`(?:ttl|有效期|过期时间)\s*[:：=]?\s*(\d+)(h|m|s|d|小时|分钟|天)`).FindStringSubmatch(t); len(m) > 2 {
		n, _ := strconv.Atoi(m[1])
		u := m[2]
		switch u {
		case "d", "天":
			n *= 24
			u = "h"
		case "小时":
			u = "h"
		case "分钟":
			u = "m"
		}
		p.TokenTTL = strconv.Itoa(n) + u
	}
	return p
}
func progressQuery(s string) bool {
	t := strings.ToLower(strings.TrimSpace(s))
	return t == "进度" || t == "状态" || t == "查询进度" || t == "查看进度" || t == "status" || t == "progress" || strings.Contains(t, "现在进度") || strings.Contains(t, "当前进度")
}
func compactContext(r *Run) {
	r.Context.EstimatedTokens = 0
	for _, m := range r.Messages {
		r.Context.EstimatedTokens += len([]rune(m.Content))/2 + 12
	}
	if len(r.Messages) > 20 {
		keep := 10
		end := len(r.Messages) - keep
		if end > r.Context.SummaryThrough {
			r.Context.Summary = fmt.Sprintf("项目 %s；技术栈 %s；能力 %s；数据库 %s/%s；认证 %s；日志 %s/%s；仓库 %s。已记录 %d 条原始消息，原文仍保留于 JSONL。", r.Plan.ProjectName, r.Plan.TechStack, strings.Join(r.Plan.Skills, ","), r.Plan.Database, r.Plan.ORM, r.Plan.Auth, r.Plan.LogLevel, r.Plan.LogFormat, r.Plan.RepoProvider, end)
			r.Context.SummaryThrough = end
			r.Context.SummaryCount++
			r.Context.Compressed = true
		}
		r.Context.EstimatedTokens = len([]rune(r.Context.Summary)) / 2
		for _, m := range r.Messages[end:] {
			r.Context.EstimatedTokens += len([]rune(m.Content))/2 + 12
		}
	}
}
func (s *Service) Close() {
	s.BeginShutdown()
	s.mu.Lock()
	for _, cancel := range s.active {
		cancel()
	}
	s.mu.Unlock()
	s.localWG.Wait()
	s.Store.Close()
	if s.Temporal != nil {
		s.Temporal.Close()
	}
}

// Command IDs identify a business intent, including failed attempts. Temporal
// Update IDs identify transport executions. Only a definitive rejected/failed
// application result permits a new transport execution; unknown results retain
// the original Update ID and are queried/retried through that identity.
func (s *Service) bindCommand(cmd Command) (Run, error) {
	if s.containsSecret(cmd.Content) {
		return Run{}, fail("secret_input", "消息包含服务端凭据，拒绝持久化")
	}
	b, _ := json.Marshal(cmd)
	fingerprint := string(b)
	return s.Store.Update(cmd.RunID, func(r *Run) error {
		if r.ExecutionMode != s.Config.Mode {
			return fail("run_mode_mismatch", "历史Run不可切换执行器")
		}
		if previous, ok := r.CommandIntents[cmd.ID]; ok {
			if previous != fingerprint {
				return fail("idempotency_conflict", "命令ID已绑定其他业务内容，包括已失败尝试")
			}
			return nil
		}
		if err := validateWireCommand(*r, cmd, true); err != nil {
			return err
		}
		if len(r.CommandIntents) >= r.Definition.Budget.MaxCommands {
			return fail("budget_exceeded", "Run命令身份预算已耗尽")
		}
		r.CommandIntents[cmd.ID] = fingerprint
		return acceptConversationCommand(r, cmd)
	})
}
func (s *Service) recordUpdateFailure(cmd Command, attempt int, err error) {
	var application *temporal.ApplicationError
	if !errors.As(err, &application) {
		return
	}
	_, _ = s.Store.Update(cmd.RunID, func(r *Run) error {
		if r.UpdateAttempts[cmd.ID] == attempt {
			r.UpdateAttempts[cmd.ID] = attempt + 1
		}
		return nil
	})
}
