# 编排域

> 实体ID：domain.orchestrator
> 基线日期：2026-10-02。
> 来源：[原设计](../../docs/PRODUCT-GUIDE.md)；审校：[裁决记录](../../docs/analysis/REVIEW-DECISIONS.md)。

## 域职责

维护项目创建 Run 的权威状态、串行命令、Activity 调度、检查点和人工确认门禁。当前主线为 INTAKE → CLARIFY → PLAN → IMPLEMENT → VERIFY → DELIVER → DONE；挂起/终止态为 PAUSED、BLOCKED、FAILED、CANCELLED，共十一态。DELIVER 内部依次区分 remote_repo、remote_ci、package，子步骤不是额外 Run 状态。

## 域边界

不生成代码、不自行持有模型或 GitLab 原始密钥，不用模型语言改状态，不把一次 Activity 成功当作整项任务完成。未知写入结果必须核实；取消不证明已有副作用已回滚；终止任务不可原地复活。local 模式的进程逻辑不能冒称 Temporal 持久执行。

## 与其他域的关系

消费 Agent 域的结构化建议与动作校验；调度技能和集成域的受控副作用；消费对话的命令与确认，治理的权限、预算及幂等裁决，以及审计域的验证证据。向展示域提供可观察阶段，向部署域提出运行依赖与恢复要求，依赖只能通过功能公开输入/输出表达。

## 按改动节奏划域的理由

状态转移、消息调度、恢复、确认和历史迁移随项目创建流程一起调整，错误处理与等待语义也必须同步修订，因此聚为一域。这个划分来自控制责任与生命周期的耦合，不声称由实际提交频率统计得出。

## 功能清单

- [状态机](features/state-machine.md)
- [Update/Query 与串行消息](features/durable-messaging.md)
- [Activity 调度、重试与心跳](features/activity-scheduling.md)
- [Continue-As-New](features/continue-as-new.md)
- [多 Run 生命周期](features/run-lifecycle.md)
- [版本绑定确认](features/human-gates.md)

## 规范与证据

功能契约规定目标行为；原文代码只保留为来源证据。实现与历史测试的边界见[实现状态](../../docs/analysis/IMPLEMENTATION-STATUS.md)，不据文件存在宣称本域已经全面交付。

