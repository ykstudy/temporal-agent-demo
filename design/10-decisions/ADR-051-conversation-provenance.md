# ADR-051 原始消息与摘要来源的完整持久化

> 实体ID：adr.ADR-051
> 来源：2026-10-04 用户要求按顺序完成全部阶段。

## 状态

accepted（2026-10-04）。先发布contract1.4.0，S3实现和验收通过后进入S4。

## 背景

S2保留了建议的用户输入，但没有完整输出Part、原始字节日志、历史摘要与持久回执。仅有SummaryRecord字段不能冻结模型摘要协议；旧文本记录不能推断当时的对象引用和进度序号。

## 决策

发布[1.4.0契约](../contracts/releases/1.4.0/README.md)。新定义冻结摘要提示和完整输出Schema。模型选择可核实的原文片段，控制面验证来源，按显式模式及预算生成派生记录；失败保留原文及前版摘要。新Run从第一条消息记录完整Envelope和历史事件，JCS加LF作为真正的原始JSONL。单写者待提交快照提供精确部分追加恢复；冲突拒绝启动。

持久命令回执绑定请求指纹和原结果，历史对象按精确引用读取。独立S3观察接口公开原文范围及摘要失败，不能冒充完整v2快照。旧Run保持旧来源，无法完整投影时返回legacy_contract_incomplete。

## 理由

完整冻结输入与可验证原始字节使摘要来源、历史卡片和重试结果可独立核验；摘要不能生成授权。重启使用已持久化的同一事务，避免静默截断原文或制造历史。

## 备选方案

拒绝读取时补造旧Part、覆盖单个summary字符串、失败切换rules或用最新提案替换旧引用。

## 影响

新增冻结摘要协议和尝试对象、对话日志/恢复、上下文摘要引用。S4继续消费完整对象；本阶段不变更生产发布范围及冻结不变量。

## 关联

- 领域：[agent](../02-agent/overview.md)、[conversation](../05-conversation/overview.md)。
- 功能：[context-compression](../02-agent/features/context-compression.md)、[context-building](../02-agent/features/context-building.md)、[definition-versioning](../02-agent/features/definition-versioning.md)、[jsonl-message-store](../05-conversation/features/jsonl-message-store.md)、[rich-message-protocol](../05-conversation/features/rich-message-protocol.md)。
- 基线：[有效不变量](../00-invariants/INVARIANTS.md)。

## 历史

- 2026-10-04：发布1.4及新定义边界；不改写既有发布。
