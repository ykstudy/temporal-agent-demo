# 功能：有限预算与资源约束

> 实体ID：feature.governance.budget-limits
> 来源：[原设计](../../../docs/PRODUCT-GUIDE.md)：§2.2、§5.2、§6.2–6.3、§8.2、§10.2。
> 审校：[裁决记录](../../../docs/analysis/REVIEW-DECISIONS.md)：R-010、R-015、R-020–021、R-024。
> 基线日期：2026-10-02。

## 目标

在调用和执行之前限制上下文、模型修正、工具资源及外部等待，防止无限循环和重试消耗。显示估计值不等于预算约束；预算必须关联 Run、操作和真实尝试，达到上限有确定停止或阻塞结果。

## 输入

冻结 BudgetPolicy/version、Run 已使用及预留额度；上下文 token、输出/协议开销、近期轮数和摘要额度；模型调用与修正次数、Activity 尝试/总时限、命令资源、文件数量/大小及 CI 查询/核实期限。各项语义和单位在定义中明确。

## 输出

BudgetDecision、允许调用的预留、剩余量、计量来源、超限或配置缺失原因；上下文裁剪约束、有限重试和等待范围。不能测得实际 token 时明确为保守估计，不伪报供应商结算数或精确成本。

## 算法 / 流程

1. 以原设计上下文默认 max_tokens=8000、recent_turns=10、summary_tokens=2000 为配置起点，定义输入限制与输出/协议预留，禁止把三者当可无限叠加额度。
2. 组装上下文前分层分配预算，固定规则、权威状态和完整当前消息不可随末尾截断丢失；必要层超限拒绝模型请求。
3. 调用、Activity 重试、固定命令及外部轮询前预留本次资源，核对模型修正、尝试和总等待上限；缺上限不按无限许可处理。
4. 完成后按真实用量或标明估计来源记账，失败和重试也计入消耗；相同尝试回执不重复扣账，未用预留按规则释放。
5. 耗尽时停止新动作并说明恢复条件，预算调整须新的可追溯政策/授权，不由模型自行抬高；未知写只允许预算内核实，不借预算重置盲重放。

## 边界与异常

UTF-8 字节切片或固定字符比例不是精确 token 计量。已有权限、原始消息和完成条件不随上下文压缩改变。单次超时与总时限分别约束；取消不证明外部未执行。资源不足不能显示成功，local 与 Temporal 模式均遵守业务额度，执行历史长度由独立 Continue-As-New 条件管理。

## 依赖

- [冻结定义与预算政策](../../02-agent/features/definition-versioning.md#输出)：消费公开输出契约。
- [当前 Run 与操作身份](../../01-orchestrator/features/run-lifecycle.md#输出)：消费公开输出契约。
- [真实调用与尝试计量](../../07-observability/features/audit-traceability.md#输出)：消费公开输出契约。
- [可信配置范围](../../09-deploy/features/configuration-modes.md#输出)：消费公开输出契约。

## 被谁依赖


- [动作确定性校验 · 输入](../../02-agent/features/action-validation.md#输入)：消费本功能的公开输出契约。
- [分层上下文构建 · 输入](../../02-agent/features/context-building.md#输入)：消费本功能的公开输出契约。
- [保留原文的摘要压缩 · 输入](../../02-agent/features/context-compression.md#输入)：消费本功能的公开输出契约。
- [AgentDefinition 版本 · 输入](../../02-agent/features/definition-versioning.md#输入)：消费本功能的公开输出契约。
- [模型决策建议 · 输入](../../02-agent/features/model-decision.md#输入)：消费本功能的公开输出契约。
- [配置与执行模式 · 输入](../../09-deploy/features/configuration-modes.md#输入)：消费本功能的公开输出契约。
- [确定性约束校验 · 输入](constraint-validation.md#输入)：消费本功能的公开输出契约。
- [CI 配置与首跑触发 · 输入](../../04-integrations/features/ci-config-trigger.md#输入)：消费本功能的公开输出契约。
- [匹配流水线验证 · 输入](../../04-integrations/features/ci-verification.md#输入)：消费本功能的公开输出契约。
- [外部结果未知核实 · 输入](../../04-integrations/features/unknown-results.md#输入)：消费本功能的公开输出契约。
- [Activity 调度、重试与心跳 · 输入](../../01-orchestrator/features/activity-scheduling.md#输入)：消费本功能的公开输出契约。
- [版本绑定确认 · 输入](../../01-orchestrator/features/human-gates.md#输入)：消费本功能的公开输出契约。
- [状态机 · 输入](../../01-orchestrator/features/state-machine.md#输入)：消费本功能的公开输出契约。
- [Skill 公共契约 · 输入](../../03-skills/features/skill-contract.md#输入)：消费本功能的公开输出契约。

## 验收标准

临界和超限上下文、必要层过大、模型反复修正、读重试、长 CI 轮询及命令超时均有限停止，超限前不调用。中文混合文本不会截坏编码或丢当前请求；失败尝试计账，重复回执不双扣。无 tokenizer 明确估计来源，模型提议增加预算不能自行生效。
