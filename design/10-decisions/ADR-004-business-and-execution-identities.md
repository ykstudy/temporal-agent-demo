# ADR-004 分离业务身份与执行身份

> 实体ID：adr.004
> 来源：[原设计](../../docs/PRODUCT-GUIDE.md)：§4.2、§4.4、§5.3；L165–177、L239–257、L538–554。
> 审校：[裁决记录](../../docs/analysis/REVIEW-DECISIONS.md)：R-003、R-004。

## 状态

accepted。生效日期为 2026-10-02；这是本次有效设计基线，不将来源示例、已有代码或测试结果追认为过去的批准。

## 背景

原设计从 Temporal execution RunID 推导幂等键，而执行段在 Continue-As-New 后会变化。业务消息、工件和外部操作若沿用执行身份，会在历史切段后失去同一任务的稳定归属。

## 决策

agent_run_id 标识完整业务任务并贯穿对话、批准、工件与审计；WorkflowID、Temporal RunID、ActivityID 和 attempt 只标识执行及尝试。副作用身份绑定业务 Run、操作类型、批准来源版本和操作序列，并保存目标指纹。

## 理由

切换执行段只改变承载方式，不代表获准新建仓库或重新推送。同一个业务动作在重试前后必须可以核实同一外部事实，同时新的批准版本应拥有不同操作身份。

## 备选方案

仅用执行 RunID 方便日志关联却不能防跨段重复；仅用项目名称容易跨用户和目标冲突；随机重试键每次都新建身份，无法解决响应丢失。均不采用。

## 影响

检查点保存业务身份、去重账本和操作结果；重做产生关联原 Run 的新业务身份。验收切段前后消息和外部写只发生一次，并验证同 ID 不同内容冲突及多 Run 归属隔离。

## 关联

- [编排域](../01-orchestrator/overview.md)。
- [多 Run 生命周期](../01-orchestrator/features/run-lifecycle.md)。
- [检查点迁移](../01-orchestrator/features/continue-as-new.md)。
- [INV-005 业务 Run 身份稳定](../00-invariants/INVARIANTS.md#inv-005-业务-run-身份稳定)。
- [INV-006 副作用身份稳定](../00-invariants/INVARIANTS.md#inv-006-副作用身份稳定)。

## 历史

2026-10-02：依据保留的原文章节和本次审校建立此决策。没有可证实的更早接受记录；未来替换须新增 ADR、说明旧新差异并更新关联，不改写本条历史。
