package agent

import (
	"agentstudio/internal/contracts"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"
)

type modelMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

func clippedBytes(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	value = value[:limit]
	for !utf8.ValidString(value) {
		value = value[:len(value)-1]
	}
	return value
}

// Byte length is a deliberately conservative token upper bound for the
// OpenAI-compatible byte/BPE interface. It avoids claiming a provider-specific
// tokenizer estimate is an enforced limit. Reserve output and protocol tokens.
func buildModelRequest(r Run, text, model string, targets ...contracts.DeliveryTarget) ([]byte, error) {
	if r.FrozenDefinition == nil {
		return nil, fail("legacy_contract_incomplete", "旧Run没有冻结提示，不能调用当前提示冒充历史")
	}
	prompt := r.FrozenDefinition.Definition.SystemPrompt.Content
	if contracts.BytesHash([]byte(prompt)) != r.FrozenDefinition.Definition.SystemPrompt.SHA256 {
		return nil, fail("definition_mismatch", "冻结提示内容损坏")
	}

	maxTokens := int(r.FrozenDefinition.Definition.ContextPolicy.MaxTokens)
	if maxTokens <= 0 {
		maxTokens = 8000
	}
	outputTokens := int(r.FrozenDefinition.Definition.ContextPolicy.OutputReserveTokens)
	if maxTokens/4 < outputTokens {
		outputTokens = maxTokens / 4
	}
	inputBudget := maxTokens - outputTokens - int(r.FrozenDefinition.Definition.ContextPolicy.ProtocolReserveTokens)
	plan := r.Plan
	plan.Description = clippedBytes(plan.Description, 400)
	canonical, err := projectPlanFor(r, plan)
	if err != nil {
		return nil, err
	}
	if len(targets) > 0 {
		canonical.DeliveryTarget = targets[0]
	}
	current, _ := json.Marshal(canonical)
	checkStates := map[string]string{}
	for _, c := range r.RequiredChecks {
		checkStates[c.Name] = c.Status
	}
	authority, _ := json.Marshal(map[string]any{"run_id": r.ID, "state": r.State, "source_version": r.SourceVersion, "state_sequence": r.StateSequence, "definition_snapshot_hash": r.FrozenDefinition.SnapshotHash, "approved": r.Approval != nil, "checks": checkStates, "modes": []string{r.ExecutionMode, r.DecisionMode, r.DeliveryMode}, "blocking_reason": r.Error})
	catalog, _ := json.Marshal(frozenSkillCatalog(r.FrozenDefinition))
	core := []modelMessage{{Role: "system", Content: prompt}, {Role: "system", Content: fmt.Sprintf("Run %s; state %s; source_version %d. Authority: %s. Current ProjectPlan draft: %s. Frozen skill catalog: %s", r.ID, r.State, r.SourceVersion, authority, current, catalog)}}

	user := modelMessage{Role: "user", Content: text}
	encode := func(contextMessages []modelMessage) []byte {
		all := append(append([]modelMessage{}, contextMessages...), user)
		body, _ := json.Marshal(map[string]any{"model": model, "messages": all, "temperature": 0.1, "response_format": map[string]string{"type": "json_object"}, "max_tokens": outputTokens})
		return body
	}
	if len(encode(core)) > inputBudget {
		return nil, fail("context_budget_exceeded", "当前消息与必要任务信息超过上下文预算，请缩短这条消息后重试；尚未调用模型")
	}
	messages := append([]modelMessage{}, core...)
	if r.Context.Summary != "" {
		candidate := append(append([]modelMessage{}, messages...), modelMessage{Role: "system", Content: clippedBytes(r.Context.Summary, 2000)})
		if len(encode(candidate)) <= inputBudget {
			messages = candidate
		}
	}
	start := len(r.Messages) - int(r.FrozenDefinition.Definition.ContextPolicy.RecentMessages)
	if start < 0 {
		start = 0
	}
	selected := []modelMessage{}
	for i := len(r.Messages) - 1; i >= start; i-- {
		m := r.Messages[i]
		if m.Kind == "text" && (m.Role == "user" || m.Role == "assistant") {
			candidate := append([]modelMessage{{Role: m.Role, Content: m.Content}}, selected...)
			contextCandidate := append(append([]modelMessage{}, messages...), candidate...)
			if len(encode(contextCandidate)) <= inputBudget {
				selected = candidate
			}
		}
	}
	return encode(append(messages, selected...)), nil
}

func (s *Service) ModelPlan(ctx context.Context, r Run, text string) (Plan, string, error) {
	if !s.ModelConfigured() {
		return Plan{}, "", fail("model_unavailable", "所选模型provider未配置，不会降级规则")
	}
	if r.DecisionMode == "model" {
		provider := r.Definition.DecisionProvider
		if provider == nil || definitionDigest(r.Definition) != r.Definition.Hash || provider.Endpoint != s.Config.AIEndpoint || provider.Model != s.Config.AIModel {
			return Plan{}, "", fail("model_config_changed", "本Run模型端点或模型定义已改变，请创建新Run；不会切换provider")
		}
	}
	if r.FrozenDefinition == nil {
		return Plan{}, "", fail("legacy_contract_incomplete", "旧Run缺少冻结模型来源，请创建新Run")
	}
	if err := s.verifyFrozenDefinition(r); err != nil {
		return Plan{}, "", err
	}
	draft, e := projectPlanFor(r, r.Plan)
	if e != nil {
		return Plan{}, "", e
	}
	if r.DeliveryMode == "gitlab" {
		if s.Config.GitLabToken == "" || s.Config.GitLabNamespace == "" {
			return Plan{}, "", fail("gitlab_unavailable", "GitLab模型方案需要可核实的准确namespace")
		}
		var ns struct {
			FullPath string `json:"full_path"`
		}
		if _, e = s.gitlab(ctx, "GET", "/namespaces/"+url.PathEscape(s.Config.GitLabNamespace), nil, &ns); e != nil || ns.FullPath == "" {
			return Plan{}, "", fail("gitlab_unavailable", "GitLab namespace不可核实")
		}
		target, _ := json.Marshal(contracts.GitLabTarget{Provider: "gitlab", InstanceURL: s.Config.GitLabURL, NamespaceID: s.Config.GitLabNamespace, NamespacePath: ns.FullPath, ProjectPath: r.Plan.ProjectName, Ref: "main", Visibility: "private"})
		draft.DeliveryTarget = contracts.DeliveryTarget(target)
	}
	request, e := buildModelRequest(r, text, s.Config.AIModel, draft.DeliveryTarget)
	if e != nil {
		return Plan{}, "", e
	}
	response, e := s.callModelAdvice(ctx, r, request)
	if e != nil {
		return Plan{}, "", e
	}
	advice := response.Advice
	switch advice.Kind {
	case "clarification":
		p := r.Plan
		p.ProjectName = ""
		return p, *advice.Question, nil
	case "plan":
		expectedTarget := draft.DeliveryTarget
		if r.DeliveryMode == "gitlab" {
			var target contracts.GitLabTarget
			if e = json.Unmarshal(expectedTarget, &target); e != nil {
				return Plan{}, "", e
			}
			target.ProjectPath = advice.Plan.ProjectName
			body, _ := json.Marshal(target)
			expectedTarget = contracts.DeliveryTarget(body)
		}
		p, err := s.compatibilityPlan(r, *advice.Plan, expectedTarget)
		if err != nil {
			return Plan{}, "", err
		}
		return p, "已调用配置模型并通过冻结Schema和参数校验。" + advice.Reason, nil
	default:
		return Plan{}, "", fail("model_schema_invalid", "当前规划入口不接受执行或完成建议，不能因此触发副作用")
	}
}

// modelAdviceResult contains only provider-observed facts. No identity or
// authority supplied by a response is promoted into the control plane.
type modelAdviceResult struct {
	Payload  json.RawMessage
	Advice   contracts.DecisionAdvice
	Usage    contracts.ModelUsage
	Messages []contracts.ContextMessage
	// Started records transport admission, not HTTP success. Preflight failures
	// cannot be counted as provider calls by a surrounding budget ledger.
	Started bool
}

func (s *Service) callModelAdvice(ctx context.Context, r Run, request []byte) (modelAdviceResult, error) {
	return s.callModelPayload(ctx, r, request, "DecisionAdvice")
}

func (s *Service) callModelPayload(ctx context.Context, r Run, request []byte, schema string) (modelAdviceResult, error) {
	result := modelAdviceResult{Usage: contracts.ModelUsage{Status: "unknown"}}
	if !s.ModelConfigured() || r.FrozenDefinition == nil || r.FrozenDefinition.ProviderBinding == nil {
		return result, fail("model_unavailable", "所选model模式缺少准确provider；不会转为rules")
	}
	binding := r.FrozenDefinition.ProviderBinding
	if r.DecisionMode != "model" || binding.Endpoint != s.Config.AIEndpoint || binding.ModelID != s.Config.AIModel {
		return result, fail("model_config_changed", "当前模型配置与冻结身份不符")
	}
	if err := s.verifyFrozenDefinition(r); err != nil {
		return result, err
	}
	var submitted struct {
		Model    string                     `json:"model"`
		Messages []contracts.ContextMessage `json:"messages"`
	}
	if _, err := contracts.ParseJSON(request); err != nil {
		return result, fail("invalid_model_request", "服务端模型请求编码无效")
	}
	if err := json.Unmarshal(request, &submitted); err != nil || submitted.Model != binding.ModelID || len(submitted.Messages) == 0 || s.containsSecret(string(request)) {
		return result, fail("invalid_model_request", "模型请求身份、上下文或秘密边界不符")
	}
	result.Messages = submitted.Messages
	endpoint := strings.TrimRight(binding.Endpoint, "/")
	if !strings.HasSuffix(endpoint, "/chat/completions") {
		endpoint += "/chat/completions"
	}
	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(request))
	if err != nil {
		return result, fail("model_unavailable", "模型请求无法构造")
	}
	req.Header.Set("Authorization", "Bearer "+s.Config.AIKey)
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 60 * time.Second, CheckRedirect: func(req *http.Request, via []*http.Request) error { return http.ErrUseLastResponse }}
	result.Started = true
	resp, err := client.Do(req)
	if err != nil {
		return result, fail("model_unavailable", "模型请求失败，可按原输入重试；不会静默切换provider或模式")
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		code := "model_failed"
		if resp.StatusCode >= 500 || resp.StatusCode == 429 {
			code = "model_unavailable"
		}
		return result, fail(code, fmt.Sprintf("模型返回HTTP %d，未采纳建议", resp.StatusCode))
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, (1<<20)+1))
	if err != nil || len(data) > 1<<20 {
		return result, fail("model_invalid", "模型响应读取失败或超过大小上限")
	}
	if _, err = contracts.ParseJSON(data); err != nil {
		return result, fail("model_invalid", "模型响应JSON无效")
	}
	var payload struct {
		ID      *string `json:"id"`
		Choices []struct {
			FinishReason string `json:"finish_reason"`
			Message      struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage *struct {
			Input  *int64 `json:"prompt_tokens"`
			Output *int64 `json:"completion_tokens"`
			Total  *int64 `json:"total_tokens"`
		} `json:"usage"`
	}
	if json.Unmarshal(data, &payload) != nil || len(payload.Choices) != 1 {
		return result, fail("model_invalid", "模型响应需要一份准确建议")
	}
	if payload.ID != nil && contracts.Validate("Id", *payload.ID) == nil && !s.containsSecret(*payload.ID) {
		result.Usage.ProviderRequestID = payload.ID
	}
	if u := payload.Usage; u != nil && u.Input != nil && u.Output != nil && u.Total != nil && *u.Input >= 0 && *u.Output >= 0 && *u.Total >= *u.Input && *u.Output == *u.Total-*u.Input {
		result.Usage.Status = "known"
		result.Usage.InputTokens = u.Input
		result.Usage.OutputTokens = u.Output
		result.Usage.TotalTokens = u.Total
	}
	content := strictModelContent(payload.Choices[0].Message.Content)
	if s.containsSecret(content) {
		return result, fail("secret_output", "模型输出包含服务端凭据，拒绝持久化")
	}
	if schema == "SummaryAdvice" && payload.Choices[0].FinishReason != "stop" {
		return result, fail("summary_truncated", "摘要没有完整正常结束")
	}
	if err = contracts.ValidateJSONVersion(schema, r.FrozenDefinition.ContractVersion, []byte(content)); err != nil {
		return result, fail("model_schema_invalid", "模型输出不符合冻结DecisionAdvice Schema")
	}
	result.Payload = json.RawMessage(content)
	if schema == "DecisionAdvice" {
		if err = json.Unmarshal(result.Payload, &result.Advice); err != nil {
			return result, err
		}
	}
	return result, nil
}
