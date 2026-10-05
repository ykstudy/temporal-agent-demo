# ADR-031 Continue-As-New 迁移完整检查点

> 实体ID：adr.ADR-031
> 来源：[原设计](../../docs/PRODUCT-GUIDE.md)：§5.2–5.3、§6.3；R-013。
> 审校：[裁决记录](../../docs/analysis/REVIEW-DECISIONS.md)。

## 状态

accepted（2026-10-02 本次审校后的有效设计基线；不是已实现状态）。

## 背景

§5.2–5.3、§6.3；R-013。原文保留为证据。当前项目的目标与参考方法不同，本记录对原文已述选择、冲突或缺口做明确裁决；原作者未记录的理由和备选不追溯编造。

## 决策

迁移整个业务检查点：Run身份、阶段/版本/参数、定义快照、原文及摘要引用、去重/操作账本、等待批准和 resume_to。主循环在 handler 完成、队列消费或可靠登记后切段；不在 Update handler 中直接切段。

## 理由

本次评估：只携带 State 指针不能说明遗漏 Description、排队命令和去重记录的恢复；执行历史切段不应改变业务授权或身份。

## 备选方案

以下为本次评估，不表示原作者曾讨论：只传最小参数重新猜状态：重复副作用；每条消息切段：维护负担高；永不切段：历史超限。

## 影响

阈值为明确可配置策略并遵从 SDK 建议；新执行 Query 首次就呈现迁移检查点；跨段幂等与待批准场景纳入验收。 领域只通过下列功能的公开输入/输出协作，运行行为由后续独立验收证明。

## 关联

- 域：[orchestrator](../01-orchestrator/overview.md)。
- 功能：[continue-as-new](../01-orchestrator/features/continue-as-new.md)。
- 功能：[durable-messaging](../01-orchestrator/features/durable-messaging.md)。
- 规则基线：[有效不变量](../00-invariants/INVARIANTS.md)。

## 历史

- 2026-10-02：基于本项目原设计与用户授权自主审校形成明确裁决；包含本次纠错和补全。
- 原设计选择日期：原文未记录；本日不替代原始日期。
- 后续修订必须记录新旧决策和影响，不静默覆盖当前基线。
