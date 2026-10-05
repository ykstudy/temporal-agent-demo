# ADR-029 建立十一态封闭业务状态机

> 实体ID：adr.ADR-029
> 来源：[原设计](../../docs/PRODUCT-GUIDE.md)：§4.1、§5、§8；R-010、R-022。
> 审校：[裁决记录](../../docs/analysis/REVIEW-DECISIONS.md)。

## 状态

accepted（2026-10-02 本次审校后的有效设计基线；不是已实现状态）。

## 背景

§4.1、§5、§8；R-010、R-022。原文保留为证据。当前项目的目标与参考方法不同，本记录对原文已述选择、冲突或缺口做明确裁决；原作者未记录的理由和备选不追溯编造。

## 决策

采用 INTAKE、CLARIFY、PLAN、IMPLEMENT、VERIFY、DELIVER、DONE、PAUSED、BLOCKED、FAILED、CANCELLED；PLAN 为待确认，DELIVER 内有 remote_repo/remote_ci/package 子步骤。所有转移由不变量事件表与公开输入输出契约裁决，未列事件拒绝。

## 理由

本次评估：原设计只记录 current_step 字符串，等待、失败、取消和恢复没有闭合语义；封闭表能防止模型建议或前端状态直接越权。

## 备选方案

以下为本次评估，不表示原作者曾讨论：任意 step 文本：不可一致验证；照搬参考完整生产状态机：超出本产品；每条 Skill 一个顶层状态：扩展成本高。

## 影响

业务状态和执行步骤分开；全局版本/权限/预算核验优先；新顶层状态需要基线修订 ADR，不能只在实现里添加字符串。 领域只通过下列功能的公开输入/输出协作，运行行为由后续独立验收证明。

## 关联

- 域：[orchestrator](../01-orchestrator/overview.md)。
- 功能：[state-machine](../01-orchestrator/features/state-machine.md)。
- 规则基线：[有效不变量](../00-invariants/INVARIANTS.md)。

## 历史

- 2026-10-02：基于本项目原设计与用户授权自主审校形成明确裁决；包含本次纠错和补全。
- 原设计选择日期：原文未记录；本日不替代原始日期。
- 后续修订必须记录新旧决策和影响，不静默覆盖当前基线。
