# 领域、契约与代码模块映射

日期：2026-10-02。状态：目标模块方案，当前路径仅用于定位迁移，不表示这些目标包已经创建。采用 [ADR-046](../10-decisions/ADR-046-contract-first-implementation.md)，保留 Go/Temporal 及现有有效行为；九个设计领域按职责协作，不要求机械生成九个互相依赖的包。

## 目标目录

```text
backend/
  cmd/server/                         # 配置启动和组合根
  internal/contracts/                 # v2 DTO、Schema派生物、统一编解码
  internal/definitions/               # Definition/Skill依赖登记与冻结发布
  internal/agent/                     # 上下文、rules/model候选建议；无副作用执行权
  internal/governance/                # Schema后的动作、批准、预算与范围裁决
  internal/orchestrator/              # 状态机、Temporal Workflow、local调度
  internal/skills/                    # 脚手架和登记Skill的受控执行
  internal/integrations/gitlab/       # 精确远程目标、写入账本、CI核实
  internal/conversation/              # 原消息、SummaryRecord、组件登记
  internal/observability/             # 审计、归因、产品内完成报告
  internal/storage/                  # 单写者文件实现和持久边界
  internal/httpapi/                   # /api/v2与显式legacy adapter
  assets/skills/{add_database,add_auth,add_logging}/
  assets/templates/golang/            # 版本化公开定义与模板
frontend/src/contracts/              # 由公开契约派生TS类型与边界校验
frontend/src/renderers/              # Part/组件白名单和只读回退
validation/                          # 公共Schema+HTTP/SSE/ZIP独立验收
design/_model/                       # 文档结构登记，独立于上述运行代码
```

## 2026-10-05 实施映射

下方“当前到目标”表保留迁移前定位。本轮已建立六个真实 Go 包：`contracts`、`definitions`、`governance`、`conversation`、`storage` 与组合包 `agent`。目录名不作为完成凭据；`agent` 仍组合 Workflow/local、HTTP/legacy 适配、受控执行器和产品内部验证，没有机械拆出九个包。建议经 `control_decision.go` / `control_advice.go` 绑定完整对象后进入 `governance.Validator.Validate`，实际执行经 `control_execution.go` / `control_tools.go` / `control_git_tools.go` 的 `Revalidate` 复核，不能靠语义输出直接执行。

| 当前实际归属 | 明确职责与依赖 |
| --- | --- |
| `internal/contracts` | 完整发布 Schema、Go 派生类型、严格编解码/JCS/hash；不依赖其他产品域 |
| `internal/definitions` / `assets` | 冻结定义、技能、提示、模板与版本来源；只依赖 contracts |
| `internal/governance` | 用显式 Facts/Input 裁决版本、权限、批准、预算、动作及重核；只依赖 contracts，无网络、文件或子进程写入 |
| `internal/storage` | 原对象与原文的单写者持久边界；只依赖 contracts |
| `internal/conversation` | 完整 Part、原文与摘要来源/内容校验；依赖 contracts/storage |
| `internal/agent/control_*` | 状态组合与受控调度、工作区/工具/Git/CI/打包；消费完整 ExecutionRequest 和原身份账本 |
| `internal/agent/conversation_*` / `runtime_*` | 命令事务、模型摘要接入、实际 Activity/审计/指标/健康与恢复 |
| `internal/agent/public_*` / `http.go` | 完整 v2 投影/历史/SSE与显式 legacy 入口；不使用简化 DTO 冒充 v2 |
| `frontend/src/contracts` / `v2` | 同源完整类型与 AJV 边界、历史引用/五类 Part/运行诊断；Workspace 仅组合交互 |

`scripts/check-module-boundaries.py` 用实际 `go list` 审计包依赖，并拒绝 contracts/governance 等反向依赖及 validation 运行依赖；报告见 `docs/evidence/s5-module-boundaries.json`。跨对象授权由治理/执行链负例与公开场景另证，静态依赖检查不能单独证明副作用安全。产品不加载 design/_model，也不读取 validation 报告决定 DONE。

## 当前到目标的映射

| 领域 / 公开契约 | 当前入口 | 目标归属及职责 | 消费者 |
| --- | --- | --- | --- |
| [定义版本 · 输出](../02-agent/features/definition-versioning.md#输出)：AgentDefinition/DefinitionSnapshot | internal/agent/contracts.go、types.go | definitions登记完整prompt/Schema/Skill/工具/完成条件；contracts声明wire对象 | agent、governance、orchestrator |
| [技能契约 · 输出](../03-skills/features/skill-contract.md#输出)：SkillDefinition | types.go小型Skills表、templates.go | definitions发布可组合技能定义；skills只执行已裁决配置，assets存版本化模板 | definitions、proposal预览、执行器 |
| [模型建议 · 输出](../02-agent/features/model-decision.md#输出)：DecisionProposal | model.go、service.go ParsePlan | agent生成同形rules/model候选建议，返回来源及unknown用量 | governance、conversation |
| [动作校验 · 输出](../02-agent/features/action-validation.md#输出)：ActionValidation/ExecutionRequest | service.go、contracts.go、execution.go分散检查 | governance纯裁决且绑定完整范围；orchestrator提交结构化请求，skills/集成执行前重检 | Workflow/local及各受控执行器 |
| [调度 · 输出](../01-orchestrator/features/activity-scheduling.md#输出) | workflow.go、execution.go固定步骤 | orchestrator保持SDK确定性和同一业务身份；local调用同一契约执行边界 | 执行器、Query、HTTP |
| [富消息 · 输出](../05-conversation/features/rich-message-protocol.md#输出)：MessageEnvelope/Part | legacy Message文本、Workspace.vue及components/ProposalCard.vue | conversation登记可持久组件及原对象引用；renderers按版本白名单呈现 | HTTP/SSE、多个renderer |
| [摘要 · 输出](../02-agent/features/context-compression.md#输出)：SummaryRecord | ContextState单个summary字符串 | agent生成派生记录；conversation/storage追加保存源范围/hash及生成身份 | 上下文构建、历史审查 |
| [仓库/CI · 输出](../04-integrations/features/ci-verification.md#输出) | gitlab.go、verification.go | integrations保留原副作用核实；observability汇总同source/commit/CI证据 | 内部完成门、消息组件 |
| [持久消息 · 输出](../05-conversation/features/jsonl-message-store.md#输出) | store.go | storage实现写入接口、JSONL与原子快照；conversation拥有消息语义 | 所有产品域经明确接口访问 |
| [工作区 · 输入](../08-frontend/features/conversation-workspace.md#输入) | frontend/src/types.ts、Workspace.vue、components/*.vue | contracts派生完整类型；API边界校验；renderers与交互状态分开 | 浏览器，不直接访问executor |
| [服务运行 · 输出](../09-deploy/features/service-runtime.md#输出) | cmd/server、service.go Config | cmd组合定义、Store、Worker和HTTP；读取秘密的executor adapter | 各模式需要的显式依赖 |

## 依赖方向和公共能力

`contracts` 只声明公开对象及纯结构校验，不引用其他产品域。`definitions` 提供发布定义；`agent` 只生产建议；`governance` 根据显式输入裁决；`orchestrator` 依赖这些公开接口及执行器接口。skills/integrations处理副作用，依赖存储、批准事实和受控工具接口，不回调HTTP或语义模型授予权限。cmd组合具体实现；存储接口由需求方定义，避免用集中Service私有Run指针跨域修改。

Agent强依赖的公共能力是：结构编解码、规范化/hash、冻结定义注册、持久身份与写入边界、批准/预算裁决、受控工具执行和证据核验。每项由目标模块明确拥有，传对象或稳定引用，权限值来自可信控制面。模型、Temporal、GitLab、Go工具链仅按模式声明依赖。外部validation与design checker均不是Agent完成依赖，详见 [Agent契约](AGENT-CONTRACTS.md)。

内部依赖是接口/对象，跨域不靠自由字符串命令、私有Run字段或文档关系图来调度。不新增运行Task DAG；现有固定流程可继续作为orchestrator实现，前提是它消费正式ActionValidation/ExecutionRequest并有完整版本绑定。

## 迁移约束

按 [阶段路线](../IMPLEMENTATION-PLAN.md) 逐步迁移。先建立contracts并保持现有适配器，再按职责拆分代码；不把一次移动文件当作契约完成。每个原入口必须登记新对象转换、身份/副作用恢复、数据兼容和对应负例。API/Schema迁移先使两端符合，再替换renderer；现有运行、ZIP和GitLab身份账本持续保留。本轮Vue迁移和静态审查不创建上述后端目标运行包。


## 本阶段已落地的Vue与静态审查映射

| 当前路径 | 已实现职责 | 保持的边界 |
| --- | --- | --- |
| frontend/src/main.ts、App.vue | Vue启动、工作区/独立审查入口选择 | ?review是静态只读审查，不作为Run状态或批准来源 |
| frontend/src/Workspace.vue | legacy任务/消息/当前提案/控制/SSE/下载组合、请求隔离 | 对应迁移前App.tsx行为；不捏造v2对象或历史冻结事实 |
| frontend/src/components/*.vue | Sidebar、ProposalCard、Inspector、MarkdownMessage | 批准意图仍绑定服务端具体提案，UI不自行推进状态 |
| frontend/src/lib/api.ts、types.ts | 现有公开legacy API客户端与类型 | endpoint和产品wire结构未因Vue迁移改变；目标contracts/renderers仍按S4验收 |
| frontend/src/review/* | 十一图、来源与状态、筛选、缩放、同源SVG/PNG导出 | 仅消费本项目发布物；无运行命令、设计apply或checker调用 |
| frontend/public/review/bundle.json、schema.json | 固定本阶段审查数据和同字节Schema发布 | source摘要与冻结基线可复核；图对象是规范投影，不是实际Run批准 |
| scripts/build-review-data.py | 离线规范派生、元Schema/语义检查、负例自检 | 不导入backend/validation；产品运行不依赖Python生成器 |

参照[ADR-047](../10-decisions/ADR-047-vue-readonly-review.md)独立验收。当前迁移浏览器15/15通过；审查浏览器/导出最终证据另行归档。S1–S5保持planned，frontend当前路径可定位不等于公开v2对象符合。
