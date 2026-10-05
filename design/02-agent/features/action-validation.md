# 功能：动作确定性校验

> 实体ID：feature.agent.action-validation
> 来源：[原设计](../../../docs/PRODUCT-GUIDE.md)：§2.3、§5.2、§8、§9；L71–81、L428–530、L1153–1216。
> 审校：[裁决记录](../../../docs/analysis/REVIEW-DECISIONS.md)：R-010、R-011、R-012、R-020、R-021。
> 基线日期：2026-10-02。

## 目标

在模型或用户命令进入执行器前，对动作和参数作确定性裁决。禁止语义建议越过状态、批准、约束、工具和预算边界，提供可重放的允许/拒绝结果。

## 输入

DecisionProposal/显式命令、稳定 command/operation 身份、definition 与源版本、当前 Run 状态/state_sequence、参数 Schema、工具权限、不可变 proposal_version/批准记录及签发期限、约束与预算；待执行工作区与外部目标引用。

## 输出

ActionValidation、allowed/rejected/needs_clarification 结论、违规原因、绑定的状态/来源/批准版本和经过规范化的执行请求。允许结果仅覆盖列明动作，不等于 Activity 已执行或 Run 已推进。

字段与严格条件见 [Agent公共契约](../../contracts/AGENT-CONTRACTS.md) 和 [Schema](../../contracts/schemas/agent.schema.json)：拒绝/澄清不携带可执行请求，允许结论绑定候选与规范化ExecutionRequest；执行器再次核对当前权威版本和批准范围。Schema外形、hash/身份语义与动作是否允许是三项独立检查，`Execute=true`不能替代完整结果对象。

## 算法 / 流程

1. 核对身份、Run 归属、动作枚举、参数类型/默认值与支持栈，未知字段和危险命令按协议拒绝。
2. 检查当前合法阶段和前置依赖；读取、澄清、查询与写入分别按授权规则处理。
3. 对写入核验当前提案批准、业务 source_version、目标范围、约束与预算；首次应用未确认或过期拒绝。期内已启动的固定动作链不因普通 state_sequence 更新失效，仍禁止扩大范围。
4. 生成版本绑定的最小执行请求；执行前业务方案/授权范围变化需新来源版本与重新批准；阶段进度只更新 state_sequence。失败交诊断或澄清，不扩大允许集合。

## 边界与异常

模型不能降低 blocking 约束、提高预算或授予新工具。非法 workspace、秘密环境、错误 namespace、终止 Run 与失效确认必须在副作用前拦截。重复请求消费既有操作账本；结果未知交核实，不将同一请求自动视为无副作用重试。查询合法不意味着可写入。

## 依赖

- [无授权效力的候选建议](model-decision.md#输出)：消费公开输出契约。
- [动作与参数协议](definition-versioning.md#输出)：消费公开输出契约。
- [当前合法阶段](../../01-orchestrator/features/state-machine.md#输出)：消费公开输出契约。
- [版本绑定批准](../../01-orchestrator/features/human-gates.md#输出)：消费公开输出契约。
- [工具与秘密边界](../../06-governance/features/permissions-and-secrets.md#输出)：消费公开输出契约。
- [约束裁决](../../06-governance/features/constraint-validation.md#输出)：消费公开输出契约。
- [预算结论](../../06-governance/features/budget-limits.md#输出)：消费公开输出契约。

## 被谁依赖


- [失败归因与恢复提示 · 输入](../../07-observability/features/failure-attribution.md#输入)：消费本功能的公开输出契约。
- [Activity 调度、重试与心跳 · 输入](../../01-orchestrator/features/activity-scheduling.md#输入)：消费本功能的公开输出契约。
- [Update/Query 与串行消息 · 输入](../../01-orchestrator/features/durable-messaging.md#输入)：消费本功能的公开输出契约。

## 验收标准

用相同输入重放得到相同允许性；逐项改变源版本、批准对象、阶段、权限、预算和目标，确认在写入前拒绝。模型自称批准或要求关闭约束无效；合法 query 不改变业务状态，合法写入带完整执行范围。
