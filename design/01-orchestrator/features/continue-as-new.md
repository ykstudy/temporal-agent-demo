# 功能：Continue-As-New

> 实体ID：feature.orchestrator.continue-as-new
> 来源：[原设计](../../../docs/PRODUCT-GUIDE.md)：§4.1、§5.2–5.3、§6.1、§13.7；L147–158、L369–381、L536–554、L593、L1312。
> 审校：[裁决记录](../../../docs/analysis/REVIEW-DECISIONS.md)：R-004、R-013、R-014、R-018。
> 基线日期：2026-10-02。

## 目标

在 Temporal 历史达到策略阈值时迁移紧凑业务检查点，保持任务身份、批准与未完成工作。历史分段只改变执行 RunID，不能改变用户任务或重复外部操作。

## 输入

稳定 agent_run_id、当前阶段/子步骤、定义与源版本、待处理命令引用、批准与操作账本、resume_to、摘要/原文引用及 SDK 历史建议；所有未完成 handler 的可观察状态。

## 输出

新 Workflow 执行身份与延续关系、完整检查点和一致 Query 初值；业务 Run 身份不变。未迁移时返回等待/排队处理状态，不能把历史切段当作任务成功。

## 算法 / 流程

1. 识别历史建议或固定策略阈值，只在串行循环的安全边界计划迁移。
2. 等待 Update handler 完成，并处理或持久登记全部队列消息；新到命令使迁移重新检查。
3. 构建检查点，包含描述、阶段、版本、确认、去重、原操作结果与对话引用，校验引用可被新 Worker 读取。
4. 由主 Workflow 发起 Continue-As-New；新执行按检查点初始化 Query 和处理循环，再核对待核实/待执行步骤。

## 边界与异常

禁止从 Update handler 内直接切段；有未保存命令、未完成 handler 或丢失引用时不迁移。不能用新的 Temporal RunID 生成不同幂等业务键。未开始应用的批准过期须重审，迁移不延长期限；期内已启动固定动作链的核实事实与允许范围保留，继续复核来源/权限/预算。文件存储不可访问、检查点不完整时明确阻塞，不从 INTAKE 假装新任务。

## 依赖

- [处理完成和队列记录](durable-messaging.md#输出)：消费公开输出契约。
- [权威阶段](state-machine.md#输出)：消费公开输出契约。
- [运行定义快照](../../02-agent/features/definition-versioning.md#输出)：消费公开输出契约。
- [对话引用](../../05-conversation/features/jsonl-message-store.md#输出)：消费公开输出契约。
- [业务去重账本](../../06-governance/features/idempotency-and-concurrency.md#输出)：消费公开输出契约。

## 被谁依赖


- [健康检查与恢复 · 输入](../../09-deploy/features/health-recovery.md#输入)：消费本功能的公开输出契约。
- [工作流可观测性 · 输入](../../07-observability/features/workflow-observability.md#输入)：消费本功能的公开输出契约。

## 验收标准

在有等待输入、已批准方案、完成步骤、摘要与重复命令的任务中强制切段，断言新执行身份不同而业务身份一致。Query 不退回 INTAKE；重发迁移前命令不新增副作用；Description、批准范围、原操作 ID 和原消息引用保持完整。

