# 功能：会话与 Run 归属

> 实体ID：feature.conversation.session-run-management
> 来源：[原设计](../../../docs/PRODUCT-GUIDE.md)：§2.2、§4.1–4.2、§5.3–5.4、§13。
> 审校：[裁决记录](../../../docs/analysis/REVIEW-DECISIONS.md)：R-002–004、R-013、R-018、R-022–023。
> 基线日期：2026-10-02。

## 目标

为一次项目创建任务提供稳定业务身份和可恢复的会话入口。用户切换、重载和 Temporal Continue-As-New 都应回到准确 Run，消息、方案、操作与工件不串到另一个任务；终止后的重做以新 Run 明确表示。

## 输入

session_id、用户身份、创建或选择命令、agent_run_id、客户端关联键；冻结定义与三项独立模式、当前 state_sequence 与方案 source_version、消息索引、待确认或待核实引用；终止 Run 的 predecessor 关联及明确重做选择。

## 输出

会话与 Run 关联、最近会话索引、带 execution/decision/delivery_mode 的 RunSnapshot、准确 Temporal execution 链引用或本地快照引用；不存在、越权、版本冲突和终止操作拒绝。会话访问记录不等于运行推进或 Temporal replay 成功。

## 算法 / 流程

1. 创建时分配稳定会话和业务 Run 身份，登记定义快照与 execution_mode、decision_mode、delivery_mode，再交编排启动；三项互不替代，依赖不满足不得静默降级。
2. 所有消息、方案和工件访问核对当前 Run 归属；顺序和状态以权威接收点、Workflow Query 或本地运行快照为准。
3. 选择既有会话时加载该 Run 的消息、方案及两个独立序号；阶段和进度只增加 state_sequence，方案或授权范围改变才增加 source_version；执行链切换不更改业务身份。
4. 暂停或阻塞展示保存的 resume_to 与原因；恢复请求交编排复校，不按聊天列表缓存直接推进。
5. DONE、FAILED、CANCELLED 不复活。重做生成关联的新 Run，重新选择定义和方案范围；旧批准、未知结果及凭据权限不自动继承。

## 边界与异常

proposal_version 的内容不可变更，修订生成新版本。终止 Run 可继续只读查询证据，不能接受修改或确认命令。相同创建命令同内容返回同一 Run，不同内容冲突。消息列表存在不证明 Workflow 已处理；索引陈旧需重查准确身份。外部对象已存在时新 Run 不擅自接管；旧运行副作用保留其归属。单用户首版仍强制 Run 隔离。

## 依赖

- [业务 Run 生命周期](../../01-orchestrator/features/run-lifecycle.md#输出)：消费公开输出契约。
- [命令接收结果](../../01-orchestrator/features/durable-messaging.md#输出)：消费公开输出契约。
- [当前状态及恢复目标](../../01-orchestrator/features/state-machine.md#输出)：消费公开输出契约。
- [原始消息与索引](jsonl-message-store.md#输出)：消费公开输出契约。
- [定义版本快照](../../02-agent/features/definition-versioning.md#输出)：消费公开输出契约。

## 被谁依赖


- [对话工作区 · 输入](../../08-frontend/features/conversation-workspace.md#输入)：消费本功能的公开输出契约。
- [Update/Query 与串行消息 · 输入](../../01-orchestrator/features/durable-messaging.md#输入)：消费本功能的公开输出契约。
- [多 Run 生命周期 · 输入](../../01-orchestrator/features/run-lifecycle.md#输入)：消费本功能的公开输出契约。

## 验收标准

并发创建重发不多建 Run；两个会话交错发送、浏览和确认不串数据。刷新和 Continue-As-New 后业务身份及待确认对象一致。终止 Run 的发送、恢复和确认被拒绝；重做有新身份与前序关联，local 快照不标成 Temporal 执行历史。
