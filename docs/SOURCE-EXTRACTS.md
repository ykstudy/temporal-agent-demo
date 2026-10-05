# 原设计逐字证据摘录

原文不改，摘录保留措辞用于审校追溯。包含错误示例；有效规则以本项目 design/ 为准。行号对应 SHA-256 `51c889d53a694e1047d9a9a168ef2fe753faa50a68519c0ebbcaa59a6895a45b`。

## 摘录 L7–L7

来源：[原设计](PRODUCT-GUIDE.md)，原始行 7–7。

````text
构建一个基于 Temporal 的持久化 Agent，用户通过多轮对话描述项目创建需求，Agent 理解意图、收集必要参数，自动完成 Golang 项目脚手架生成、GitLab 仓库初始化、GitLab CI 流水线配置与首跑验证。
````

## 摘录 L28–L28

来源：[原设计](PRODUCT-GUIDE.md)，原始行 28–28。

````text
定义项目创建 Agent 的可复用能力配置，版本化，运行中不漂移。
````

## 摘录 L72–L76

来源：[原设计](PRODUCT-GUIDE.md)，原始行 72–76。

````text
| `add_auth` | 添加认证与授权基础框架 | `auth_method`（jwt/session）、`token_ttl` |
| `add_logging` | 添加结构化日志 | `log_level`（debug/info/warn）、`format`（json/text）、`output`（console/file） |

```go
type SkillDefinition struct {
````

## 摘录 L183–L183

来源：[原设计](PRODUCT-GUIDE.md)，原始行 183–183。

````text
所有 Activity 的输入输出都是**小体积结构化数据**，大对象只传引用。Activity 参数应使用单个 struct 包裹，便于向后兼容。
````

## 摘录 L239–L248

来源：[原设计](PRODUCT-GUIDE.md)，原始行 239–248。

````text
每个写操作使用**确定性幂等键**，由 `workflow_run_id` + `step_name` 组成。Temporal Go SDK 通过 `activity.GetInfo(ctx)` 获取 `WorkflowExecution.RunID` 和 `ActivityID`。

```go
import (
    "go.temporal.io/sdk/activity"
)

func createRepoIdempotent(ctx context.Context, input RepoInput) (*RepoOutput, error) {
    info := activity.GetInfo(ctx)
    idempotencyKey := fmt.Sprintf("%s-create_repo", info.WorkflowExecution.RunID)
````

## 摘录 L305–L318

来源：[原设计](PRODUCT-GUIDE.md)，原始行 305–318。

````text
Workflow 使用 Signal 通道接收用户消息。Go SDK 中主 Workflow 方法从信号通道“接收”信号，并通过 Selector 处理。

```go
package workflows

import (
    "time"

    "go.temporal.io/sdk/workflow"
)

type ProjectInitWorkflow struct {
    state          ProjectInitState
    pendingMessage chan Message
````

## 摘录 L538–L554

来源：[原设计](PRODUCT-GUIDE.md)，原始行 538–554。

````text
当执行历史接近限制时，触发 Continue-As-New，携带当前状态开启新一轮执行。业务 `agent_run_id` 保持不变。

```go
if workflow.GetInfo(ctx).GetContinueAsNewSuggested() {
    return nil, workflow.NewContinueAsNewError(ctx,
        ProjectInitWorkflowDefinition,
        ProjectInitInput{
            ProjectName:    input.ProjectName,
            TechStack:      input.TechStack,
            SelectedSkills: input.SelectedSkills,
            SkillConfigs:   input.SkillConfigs,
            RepoVisibility: input.RepoVisibility,
            ContextBudget:  input.ContextBudget,
            State:          &w.state,
        },
    )
}
````

## 摘录 L559–L559

来源：[原设计](PRODUCT-GUIDE.md)，原始行 559–559。

````text
使用 `SetUpdateHandler` 处理用户消息，`SetQueryHandler` 提供状态查询。Update 提供同步请求-响应能力，Query 用于前端获取当前状态。
````

## 摘录 L700–L700

来源：[原设计](PRODUCT-GUIDE.md)，原始行 700–700。

````text
当对话轮次超过阈值时，对早期对话进行摘要，但**授权、预算、完成条件、安全约束必须始终保留**。
````

## 摘录 L723–L727

来源：[原设计](PRODUCT-GUIDE.md)，原始行 723–727。

````text
    // 重写对话存储：摘要 + 最近对话
    store.Rewrite(input.RunID, append([]Message{{
        Role:    "system",
        Content: "历史对话摘要：" + summary,
    }}, toKeep...))
````

## 摘录 L962–L965

来源：[原设计](PRODUCT-GUIDE.md)，原始行 962–965。

````text
        {"git", "commit", "-m", fmt.Sprintf("Initial commit [%s]", info.WorkflowExecution.RunID)},
        {"git", "remote", "add", "origin",
            fmt.Sprintf("https://oauth2:%s@gitlab.com/%s.git", gitlabToken, project.PathWithNamespace)},
        {"git", "push", "-u", "origin", "main"},
````

## 摘录 L1073–L1073

来源：[原设计](PRODUCT-GUIDE.md)，原始行 1073–1073。

````text
**幂等设计**：CI 文件推送使用 git 的幂等性（内容相同则不产生新 commit）。Pipeline 触发重复执行只产生多个 pipeline，不影响正确性，Verify 只检查最新的 pipeline。
````

## 摘录 L1179–L1179

来源：[原设计](PRODUCT-GUIDE.md)，原始行 1179–1179。

````text
`StartToCloseTimeout` 是 Activity 执行语义唯一必须设置的值。
````

## 摘录 L1207–L1210

来源：[原设计](PRODUCT-GUIDE.md)，原始行 1207–1210。

````text
- GitLab Token 从**环境变量** `GITLAB_TOKEN` 读取
- **不写入** Workflow 历史、日志、模板文件、CI 配置
- `.gitignore` 强制包含 `.env`、`*.pem`、`*.key`
- GitLab CI 中需要的密钥通过 GitLab CI/CD Variables 注入
````

## 摘录 L1306–L1313

来源：[原设计](PRODUCT-GUIDE.md)，原始行 1306–1313。

````text
1. **多轮对话正常流程**：用户说“帮我创建一个 Golang 项目” → Agent 询问项目名 → 用户回复 → Agent 询问 Skill 选择 → 用户选择 → Agent 完成创建
2. **Worker 崩溃恢复**：在 ApplySkill 执行中杀掉 Worker，重启后从断点继续，对话历史不丢失
3. **写操作幂等**：在 RepoActivity 执行中杀掉 Worker，重启后不重复创建 GitLab 项目
4. **上下文压缩**：对话超过 20 轮后，自动触发摘要压缩，早期对话被摘要替代
5. **CI 失败反馈**：Pipeline 编译失败 → Verify 捕获 → 返回失败详情，Agent 向用户报告
6. **密钥安全**：检查日志、Workflow 历史、生成的代码中无 GitLab Token 泄漏
7. **Continue-As-New**：长时间任务正确触发 Continue-As-New，对话历史和状态不丢失
8. **并发消息处理**：用户快速发送多条消息，Signal 通道不阻塞，消息按顺序处理
````

