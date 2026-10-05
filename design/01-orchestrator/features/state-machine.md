# 功能：状态机

> 实体ID：feature.orchestrator.state-machine
> 来源：[原设计](../../../docs/PRODUCT-GUIDE.md)：§4.1、§5.1–5.2、§8、§13；L147–158、L284–530、L1153–1200、L1306–1313。
> 审校：[裁决记录](../../../docs/analysis/REVIEW-DECISIONS.md)：R-010、R-021、R-022。
> 基线日期：2026-10-02。

## 目标

为项目创建全过程提供明确、可验证的合法转移，保证相同状态、事件和来源版本得到相同裁决。将业务等待、未知结果和终止分别表达，防止模型或展示层越过确认与验证直接推进。

## 输入

Run 身份、当前状态/state_sequence、业务方案与授权的 source_version、事件身份、操作者和原因；提案/批准、预算和依赖裁决、Activity 结果及验证报告。恢复携带已保存 resume_to 与核实证据，调用者不能任意指定恢复阶段。

## 输出

TransitionDecision、更新状态/state_sequence、拒绝原因、审计与动作引用；普通阶段/进度只增加 state_sequence，不改变 source_version。DELIVER 输出 remote_repo、remote_ci 或 package 子步骤，远程项仅在 delivery_mode=local 时不适用。

## 算法 / 流程

1. 从 INTAKE 接受新需求；参数缺失进入 CLARIFY，参数/方案完整后进入 PLAN，未确认保持 PLAN。
2. 匹配十一态转移表并校验来源版本、权限、预算与前置证据；通过当前方案确认后才进入 IMPLEMENT。
3. 生成/技能完成进入 VERIFY；原批准快照内的有界安全重试可回 IMPLEMENT。修复改变文件、参数或动作范围时必须 VERIFY→PLAN，递增 source_version、作废旧批准并重新预览确认。
4. 必需检查满足进入 DELIVER，按交付模式执行仓库、CI 与打包，全部必需证据齐全才 DONE。
5. 拒绝未应用提案回 CLARIFY；安全暂停为 PAUSED；未知为 BLOCKED，核实后恢复保存阶段；不可恢复失败为 FAILED，主动取消为 CANCELLED。

## 边界与异常

未列事件默认拒绝；旧版本、重复事件和错误 Run 不能推进或触发新副作用。DONE、FAILED、CANCELLED 均不可原地恢复。取消不声明外部副作用被撤销；PAUSED 不能抹掉已完成结果。首次应用前批准过期或业务来源改变须重新预览；普通进度不撤销期内已启动的批准动作链或已核实副作用，核实失败保持 BLOCKED。

## 依赖

- [确认范围与来源版本](human-gates.md#输出)：消费公开输出契约。
- [阶段执行结果](activity-scheduling.md#输出)：消费公开输出契约。
- [原操作核实结果](../../04-integrations/features/unknown-results.md#输出)：消费公开输出契约。
- [预算裁决](../../06-governance/features/budget-limits.md#输出)：消费公开输出契约。

## 被谁依赖


- [动作确定性校验 · 输入](../../02-agent/features/action-validation.md#输入)：消费本功能的公开输出契约。
- [分层上下文构建 · 输入](../../02-agent/features/context-building.md#输入)：消费本功能的公开输出契约。
- [保留原文的摘要压缩 · 输入](../../02-agent/features/context-compression.md#输入)：消费本功能的公开输出契约。
- [方案预览与明确确认 · 输入](../../05-conversation/features/preview-confirmation.md#输入)：消费本功能的公开输出契约。
- [富消息结构协议 · 输入](../../05-conversation/features/rich-message-protocol.md#输入)：消费本功能的公开输出契约。
- [会话与 Run 归属 · 输入](../../05-conversation/features/session-run-management.md#输入)：消费本功能的公开输出契约。
- [状态流同步 · 输入](../../08-frontend/features/state-stream-sync.md#输入)：消费本功能的公开输出契约。
- [确定性约束校验 · 输入](../../06-governance/features/constraint-validation.md#输入)：消费本功能的公开输出契约。
- [幂等与并发控制 · 输入](../../06-governance/features/idempotency-and-concurrency.md#输入)：消费本功能的公开输出契约。
- [外部结果未知核实 · 输入](../../04-integrations/features/unknown-results.md#输入)：消费本功能的公开输出契约。
- [工作流可观测性 · 输入](../../07-observability/features/workflow-observability.md#输入)：消费本功能的公开输出契约。
- [Continue-As-New · 输入](continue-as-new.md#输入)：消费本功能的公开输出契约。
- [多 Run 生命周期 · 输入](run-lifecycle.md#输入)：消费本功能的公开输出契约。

## 验收标准

对全部十一态建立合法/非法事件表；验证 PLAN 无批准不能实施、拒绝回 CLARIFY、未知结果不能重放、暂停在安全边界生效。原快照重试回 IMPLEMENT，修改内容回 PLAN，未获新批准不能修复应用；进度更新不作废批准。所有终止态恢复拒绝，重做创建关联新 Run。

