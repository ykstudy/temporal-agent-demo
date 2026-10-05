# ADR-003 固定 Update / Signal / Query 语义和 SDK 版本

> 实体ID：adr.003
> 来源：[原设计](../../docs/PRODUCT-GUIDE.md)：§3、§5.2、§5.4；L115–116、L305、L344–349、L559–585。
> 审校：[裁决记录](../../docs/analysis/REVIEW-DECISIONS.md)：R-002、R-003、R-013。

## 状态

accepted。生效日期为 2026-10-02；这是本次有效设计基线，不将来源示例、已有代码或测试结果追认为过去的批准。

## 背景

原文混用异步通知和可返回结果的命令，并使用不能直接照抄的客户端签名。调用返回可能仅说明命令被接受，不能自然证明项目已生成，查询也不能承担推进状态的职责。

## 决策

本次基线明确选择 Go Temporal SDK v1.38.0 并固定依赖。业务命令使用 Update 与稳定 UpdateID，校验、接收和结果等待阶段分别声明；Signal 仅用于无处理结果的通知，Query 为无副作用观察并返回可序列化结果及 error。

## 理由

明确锁定版本使接口验收有可重复目标。区分接收、排队、命令完成和 Run 完成，可让超时和客户端重连处理保持准确，并避免观察请求变成新的业务写入。

## 备选方案

全部使用 Signal 简化入口但缺少命令结果协议；以 Query 驱动执行破坏只读语义；浮动 SDK 依赖会让示例和已验证接口漂移。其他版本需另行兼容审查。

## 影响

当前文档技术基线固定 Go 1.25.0 与 Temporal Go SDK v1.38.0，见[技术基线](../TECHNICAL-BASELINE.md)；不宣称最新版，升级需接口/重放/恢复验收。

升级同时检查客户端选项、handler、服务器兼容和未完成 Update 的历史迁移。真实验收覆盖重复 Update、校验拒绝、客户端等待超时与重连；Query 状态范围和进度同步边界写入契约。

## 关联

- [编排域](../01-orchestrator/overview.md)。
- [持久消息](../01-orchestrator/features/durable-messaging.md)。
- [历史切段](../01-orchestrator/features/continue-as-new.md)。
- [INV-007 消息可去重可追溯](../00-invariants/INVARIANTS.md#inv-007-消息可去重可追溯)。
- [INV-008 命令串行与有界背压](../00-invariants/INVARIANTS.md#inv-008-命令串行与有界背压)。

## 历史

2026-10-02：依据保留的原文章节和本次审校建立此决策。没有可证实的更早接受记录；未来替换须新增 ADR、说明旧新差异并更新关联，不改写本条历史。
