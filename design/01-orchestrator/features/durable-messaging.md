# 功能：Update/Query 与串行消息

> 实体ID：feature.orchestrator.durable-messaging
> 来源：[原设计](../../../docs/PRODUCT-GUIDE.md)：§3、§4.2、§5.2、§5.4、§13.8；L115–116、L165–177、L305–391、L557–585、L1313。
> 审校：[裁决记录](../../../docs/analysis/REVIEW-DECISIONS.md)：R-001、R-002、R-003。
> 基线日期：2026-10-02。

## 目标

将客户端命令可靠归属到独立 Run，并提供接收、排队与处理完成的明确反馈。把异步 Signal、需响应的 Update 和只读 Query 分开，保持 Workflow 的确定性串行处理。

## 输入

稳定 command_id/message_id、Run/会话归属、命令内容与摘要、客户端选择的对象/版本及操作者；SDK 基线和队列容量。状态查询携带 Run 身份，不能包含改变阶段的隐式操作。

## 输出

CommandReceipt、处理结果/明确拒绝与稳定关联；Query 输出当前 Workflow 阶段检查点和版本。原消息持久引用保留，响应区分 accepted、queued、processed；任务整体完成另有验证结果。

## 算法 / 流程

1. 验证身份、内容摘要、Run 归属与命令 Schema；同 ID 同内容复用记录，同 ID 不同内容拒绝。
2. 使用锁定 SDK 的 Update options 和等待阶段接入命令，以 SDK workflow channel 排队；Signal 仅运输约定的异步事件。
3. 在权威串行消费点分配顺序并确认原消息持久化，再把请求交动作/状态校验；只有处理结果可返回 processed。
4. Query 只读取可序列化检查点，不调用 Activity、不改状态；队列满时按明确等待或拒绝规则实施背压。

## 边界与异常

原生 channel/goroutine 不能进入 Workflow。保存失败不能声称已接收后启动副作用。乱序或跨 Run 请求拒绝；断线和 Update 超时只表示客户端未取得结果，不证明业务失败，重试须复用身份。Query 详细外部进度需通过公开快照同步，不能声称检查点就是实时外部 I/O。

## 依赖

- [会话和 Run 归属](../../05-conversation/features/session-run-management.md#输出)：消费公开输出契约。
- [消息持久记录](../../05-conversation/features/jsonl-message-store.md#输出)：消费公开输出契约。
- [去重和冲突裁决](../../06-governance/features/idempotency-and-concurrency.md#输出)：消费公开输出契约。
- [合法命令结论](../../02-agent/features/action-validation.md#输出)：消费公开输出契约。

## 被谁依赖


- [JSONL 原始消息存储 · 输入](../../05-conversation/features/jsonl-message-store.md#输入)：消费本功能的公开输出契约。
- [会话与 Run 归属 · 输入](../../05-conversation/features/session-run-management.md#输入)：消费本功能的公开输出契约。
- [Harness 服务运行 · 输出](../../09-deploy/features/service-runtime.md#输出)：向本功能的公开输入提交请求。
- [状态流同步 · 输入](../../08-frontend/features/state-stream-sync.md#输入)：消费本功能的公开输出契约。
- [审计与业务追踪 · 输入](../../07-observability/features/audit-traceability.md#输入)：消费本功能的公开输出契约。
- [Continue-As-New · 输入](continue-as-new.md#输入)：消费本功能的公开输出契约。

## 验收标准

使用并发命令、断线重发、重复 ID/不同内容、满队列和错误 Run 验证不丢失、不重复副作用且顺序稳定。分别断言 Update 处理反馈与任务完成，Query 无写入；Workflow replay 和 SDK runtime 验证通过，不能仅靠代码编译。

