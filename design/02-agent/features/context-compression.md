# 功能：保留原文的摘要压缩

> 实体ID：feature.agent.context-compression
> 来源：[原设计](../../../docs/PRODUCT-GUIDE.md)：§6.1–6.3、§13.4；L593、L643、L700–727、L1309。
> 审校：[裁决记录](../../../docs/analysis/REVIEW-DECISIONS.md)：R-014、R-015、R-018。
> 基线日期：2026-10-02。

## 目标

把长期对话形成可引用的工作摘要，减小模型输入而保留原始证据。摘要只描述已知语义，权限、批准、预算和完成条件继续由结构化状态提供。

## 输入

稳定 run_id、明确以轮次或消息数定义的阈值、待摘要消息范围/身份/hash、最近窗口、旧摘要版本、定义及摘要预算；来源可读性、显式 decision_mode 与 provider 就绪状态。[消息协议 v2](../../contracts/MESSAGE-PROTOCOL.md)的 message_id/sequence 和五类 Part 为目标结构，legacy 文本只读适配，不能补造缺失对象版本/hash。

## 输出

SummaryRecord、source_range/hash、生成模式、摘要版本、覆盖范围及校验结果；未压缩/失败/过期分别可辨认。原始 JSONL 不重写，摘要引用可定位到具体原消息。

## 算法 / 流程

1. 以确定阈值识别待摘要范围，排除当前窗口；冻结 message_id/sequence 范围和原记录字节 hash，Part/ObjectRef 的逻辑对象 hash 独立保留，避免并发消息混入。
2. 按预算与显式 decision_mode 生成模型或规则摘要；model 失败不降级规则，保留旧版。摘要说明参数、已知事实、澄清与未解决问题，不把推测写成完成证据。
3. 校验身份、来源与受保护字段；原 Part/ObjectRef 的类型、版本、source/hash 不被摘要替换。未知 Part/组件仅引用只读 fallback，授权/批准/预算从独立状态注入，不能从模型摘要恢复或扩大。
4. 持久化新的派生版本与范围，再让 ContextBuilder 使用；生成失败保留旧摘要和全部原文，新消息进入后续范围。

## 边界与异常

不得执行原文 Rewrite 删除早期消息；“20轮”与“20条”按统一配置定义。来源或定义变化后旧摘要标记版本关系，不授权新目标；普通 state_sequence 不使同 source 批准过期。摘要不能把旧缺身份 proposal 文本改成可执行 action。模型截断、编造批准或丢失阻塞原因时拒绝；并发与重试按范围身份幂等，不产生重复替代版本。

## 依赖

- [不可变原消息](../../05-conversation/features/jsonl-message-store.md#输出)：消费公开输出契约。
- [摘要协议与定义版本](definition-versioning.md#输出)：消费公开输出契约。
- [独立权威阶段](../../01-orchestrator/features/state-machine.md#输出)：消费公开输出契约。
- [授权边界](../../06-governance/features/permissions-and-secrets.md#输出)：消费公开输出契约。
- [摘要预算](../../06-governance/features/budget-limits.md#输出)：消费公开输出契约。

## 被谁依赖


- [分层上下文构建 · 输入](context-building.md#输入)：消费本功能的公开输出契约。

## 验收标准

超过阈值后摘要版本增加且可追溯 source range，全部原消息/Part 数量、身份及原始字节 hash 不变，引用仍解析原对象而非最新对象。注入失败、过期来源、并发新增、未知 Part 及“已批准”文本，断言授权不变、失败不删历史、重复生成幂等、当前窗口完整；legacy 缺版本/hash 的旧 proposal 不因摘要恢复动作。
