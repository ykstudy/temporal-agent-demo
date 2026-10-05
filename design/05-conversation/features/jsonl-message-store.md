# 功能：JSONL 原始消息存储

> 实体ID：feature.conversation.jsonl-message-store
> 来源：[原设计](../../../docs/PRODUCT-GUIDE.md)：§1.2、§4.2、§6.1、§6.3。
> 审校：[裁决记录](../../../docs/analysis/REVIEW-DECISIONS.md)：R-003、R-013–014、R-018、R-019。
> 基线日期：2026-10-02。

## 目标

首版用文件保存可追溯、不可被摘要覆盖的原始对话，为上下文读取、会话恢复和审计提供稳定引用。文件位于持久存储边界，不能把 Worker 本机暂存文件当作 Temporal 自动保护的历史。

## 输入

agent_run_id、稳定 message_id/command_id、角色、内容 hash、权威顺序号、服务端记录时间、组件和工件引用；追加、按范围/近期分页读取、派生摘要登记及恢复请求。摘要另带源消息范围、hash 和 summary_version。

## 输出

公开消息序列化由 [版本化消息协议](../../contracts/MESSAGE-PROTOCOL.md) 和 [Schema](../../contracts/schemas/message.schema.json) 规定；源JSONL原字节、顺序、稳定对象引用和SummaryRecord来源范围按 [摘要规则](../../contracts/HASHING.md) 保留。旧记录不补造缺失的身份/版本/hash，按显式legacy只读边界转换。

已持久消息回执、分页或顺序范围结果、稳定 StorageRef、派生摘要引用与原文可检索证据；身份冲突、写失败、截断或损坏结果。消息已保存与命令已执行分别记录，不提供虚假的跨文件原子承诺。

## 算法 / 流程

1. 对目录与 Run 路径做归属校验，采用单写者或明确进程锁；按稳定身份去重，同 ID 不同 hash 拒绝。
2. 编码一条完整 JSONL 记录并追加、同步，所有错误返回失败；成功回执保留内容 hash、顺序和持久位置。
3. 维护可重建索引，范围读取校验编码、顺序及行长度；“近期”接口说明是否依赖索引，不能把全扫描宣称为恒定成本。
4. 摘要作为独立派生版本追加登记，不重写早期原文；快照以原子替换保存消息边界、state_sequence 与方案 source_version，普通消息追加不自动使批准过期。
5. 崩溃恢复检查末行截断及跨文件部分提交，隔离坏尾部、重建索引并按稳定身份补账；恢复记录可审计，不静默跳过已接收消息。

## 边界与异常

存储失败不能回已处理；没有完整持久回执的响应明确未完成。原文、摘要和快照分别备份与按保留策略治理，摘要成功不触发原文删除。目录采用限制权限；日志与消息不存原始凭据。Worker 可迁移时必须有共享持久卷或独立存储服务，缺少它就明确限定部署方式并拒绝不可恢复调度。

## 依赖

- [顺序与命令身份](../../01-orchestrator/features/durable-messaging.md#输出)：消费公开输出契约。
- [锁与重复请求规则](../../06-governance/features/idempotency-and-concurrency.md#输出)：消费公开输出契约。
- [受限存储路径](../../06-governance/features/workspace-isolation.md#输出)：消费公开输出契约。
- [非敏感记录与权限](../../06-governance/features/permissions-and-secrets.md#输出)：消费公开输出契约。
- [持久存储部署约束](../../09-deploy/features/service-runtime.md#输出)：消费公开输出契约。

## 被谁依赖


- [分层上下文构建 · 输入](../../02-agent/features/context-building.md#输入)：消费本功能的公开输出契约。
- [保留原文的摘要压缩 · 输入](../../02-agent/features/context-compression.md#输入)：消费本功能的公开输出契约。
- [富消息结构协议 · 输入](rich-message-protocol.md#输入)：消费本功能的公开输出契约。
- [会话与 Run 归属 · 输入](session-run-management.md#输入)：消费本功能的公开输出契约。
- [Continue-As-New · 输入](../../01-orchestrator/features/continue-as-new.md#输入)：消费本功能的公开输出契约。
- [Update/Query 与串行消息 · 输入](../../01-orchestrator/features/durable-messaging.md#输入)：消费本功能的公开输出契约。

## 验收标准

重复消息不多追加；同 ID 改内容冲突；并发、磁盘满、编码失败、截断末行与进程崩溃能检出并恢复。生成摘要后原消息数量与 hash 不变；分页无跨 Run 记录。迁移 Worker 无法访问 StorageRef 时明确失败，不能因 Temporal 有 history 就报告对话可恢复。
