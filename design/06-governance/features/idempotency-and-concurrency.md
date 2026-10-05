# 功能：幂等与并发控制

> 实体ID：feature.governance.idempotency-and-concurrency
> 来源：[原设计](../../../docs/PRODUCT-GUIDE.md)：§4.4、§5.2–5.4、§7.1–7.4。
> 审校：[裁决记录](../../../docs/analysis/REVIEW-DECISIONS.md)：R-003–006、R-011、R-013、R-017–018。
> 基线日期：2026-10-02。

## 目标

让同一业务命令与副作用在 HTTP 重发、Activity 重试、Worker 崩溃和执行链切换后仍有同一身份与结果，防止并发覆盖批准版本。幂等必须登记、检查和复用真实操作，不能只计算一个未提交的字符串。

## 输入

agent_run_id、operation_type、approved_source_version、operation_sequence、稳定 command/update ID；请求 fingerprint、目标、预期 source_version/state_sequence、现存账本与部分结果。Temporal workflow/run/activity/attempt 身份另作追踪，不作为业务幂等身份。

## 输出

OperationIdentity、允许执行/既有结果/内容冲突/版本冲突/待核实决定、写者占用与 CAS 回执；每次尝试、原结果和外部目标的关联账本。重复已应用操作返回原结果，不重复历史或副作用。

## 算法 / 流程

1. 在权威接收点登记稳定命令身份和内容 hash；同 ID 同内容返回既有回执，不同内容冲突；顺序只分配一次。
2. 使用业务 Run、类型、批准源版本及操作序号确定 operation_id，持久登记 fingerprint、目标和准备状态，再允许执行。
3. 单用户文件采用明确锁/单写者；运行状态 CAS 使用 state_sequence，方案批准 CAS 使用 source_version；阶段进度不使批准过期，同 Run 并发写串行化。
4. 已完成操作复用已核验结果；未完成记录按 manifest/外部身份核实，未知不能被重试标志改写为未执行。
5. 原子保存可同提交的数据；跨消息、快照和远程系统的部分提交用账本恢复，Continue-As-New 迁移未完成操作及去重范围。

## 边界与异常

source_version 仅方案/授权范围变化增加，proposal_version 内容不可变更。参数、定义或目标改变形成新指纹，不能沿用原成功记录。进程锁不等于跨主机锁；部署扩展需共享存储及一致写者约束。确认后响应丢失仍返回同一批准/应用结果；服务端期限不被重发或 state_sequence 增加延期。终止 Run 不以新序号复活；重做新 Run 明确关联历史，外部未知副作用仍按旧身份核实。

## 依赖

- [稳定业务 Run 与生命周期](../../01-orchestrator/features/run-lifecycle.md#输出)：消费公开输出契约。
- [当前状态及预期版本](../../01-orchestrator/features/state-machine.md#输出)：消费公开输出契约。
- [冻结定义身份](../../02-agent/features/definition-versioning.md#输出)：消费公开输出契约。
- [持久部署边界](../../09-deploy/features/service-runtime.md#输出)：消费公开输出契约。
- [操作尝试审计登记](../../07-observability/features/audit-traceability.md#输入)：提交公开输入契约。

## 被谁依赖


- [JSONL 原始消息存储 · 输入](../../05-conversation/features/jsonl-message-store.md#输入)：消费本功能的公开输出契约。
- [工作区与工件隔离 · 输入](workspace-isolation.md#输入)：消费本功能的公开输出契约。
- [GitLab 项目身份与创建 · 输入](../../04-integrations/features/gitlab-project.md#输入)：消费本功能的公开输出契约。
- [外部结果未知核实 · 输入](../../04-integrations/features/unknown-results.md#输入)：消费本功能的公开输出契约。
- [审计与业务追踪 · 输入](../../07-observability/features/audit-traceability.md#输入)：消费本功能的公开输出契约。
- [Continue-As-New · 输入](../../01-orchestrator/features/continue-as-new.md#输入)：消费本功能的公开输出契约。
- [Update/Query 与串行消息 · 输入](../../01-orchestrator/features/durable-messaging.md#输入)：消费本功能的公开输出契约。
- [多 Run 生命周期 · 输入](../../01-orchestrator/features/run-lifecycle.md#输入)：消费本功能的公开输出契约。
- [项目骨架生成 · 输入](../../03-skills/features/scaffold-generation.md#输入)：消费本功能的公开输出契约。

## 验收标准

重复消息、确认、生成、建仓和触发不增加对应业务副作用；同 ID 改内容明确冲突。两个写者同时用旧版本申请，至多一个 CAS 成功；跨执行链仍沿用原操作。注入结果保存前崩溃可由原证据恢复，不能依靠重发远程写来填账；历史不重复追加。
