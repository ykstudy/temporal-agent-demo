# 功能：审计与业务追踪

> 实体ID：feature.observability.audit-traceability
> 来源：[原设计](../../../docs/PRODUCT-GUIDE.md)：§4.2、§4.4、§9.3、§10。
> 审校：[裁决记录](../../../docs/analysis/REVIEW-DECISIONS.md)：R-003、R-004、R-018、R-019、R-024。
> 基线日期：2026-10-02。

## 目标

把用户命令、批准、执行和外部结果连成可核对的业务轨迹。即使 Activity 重试或 Continue-As-New 更换 Temporal 执行身份，也能识别同一业务操作及其结果来源。

## 输入

Run、稳定命令与操作身份，来源版本、批准引用、阶段、尝试次数、目标对象、结果类别及耗时；外部项目、提交和流水线等可公开引用，不接收凭证或原始秘密参数。

## 输出

追加式审计记录与追踪关联，包含时间、业务 Run、步骤、动作、脱敏参数摘要、结果、耗时和证据引用。记录区别受理、排队、执行、已确认结果及待核实状态。

## 算法 / 流程

在命令受理、授权裁决、Activity 开始与结束、外部结果核实处生成事件；用稳定业务身份串联事件，以 Temporal 执行链和尝试身份补充诊断。先按白名单脱敏，再计算参数摘要；保存结果引用并向展示层提供只读查询。重复事件可保留尝试信息，业务统计按操作身份去重。

## 边界与异常

审计不替代工作流权威状态或操作台账。参数哈希不能证明权限正确，也不能使秘密安全；凭证、带令牌地址和敏感环境均不得写入。记录失败须显式暴露，不能用一条“成功”日志补齐未知外部结果。时间排序仅辅助阅读，命令顺序依赖权威序号。

## 依赖

- [业务身份与执行链](../../01-orchestrator/features/run-lifecycle.md#输出)：消费公开输出契约。
- [命令受理顺序](../../01-orchestrator/features/durable-messaging.md#输出)：消费公开输出契约。
- [操作身份与结果台账](../../06-governance/features/idempotency-and-concurrency.md#输出)：消费公开输出契约。
- [脱敏与权限边界](../../06-governance/features/permissions-and-secrets.md#输出)：消费公开输出契约。

## 被谁依赖


- [有限预算与资源约束 · 输入](../../06-governance/features/budget-limits.md#输入)：消费本功能的公开输出契约。
- [幂等与并发控制 · 输出](../../06-governance/features/idempotency-and-concurrency.md#输出)：向本功能的公开输入提交请求。
- [权限与秘密边界 · 输出](../../06-governance/features/permissions-and-secrets.md#输出)：向本功能的公开输入提交请求。
- [外部结果未知核实 · 输入](../../04-integrations/features/unknown-results.md#输入)：消费本功能的公开输出契约。
- [失败归因与恢复提示 · 输入](failure-attribution.md#输入)：消费本功能的公开输出契约。
- [工作流可观测性 · 输入](workflow-observability.md#输入)：消费本功能的公开输出契约。
- [日志技能 · 输入](../../03-skills/features/logging-skill.md#输入)：消费本功能的公开输出契约。

## 验收标准

同一操作经过重试和执行链切换仍可完整关联；并发命令可追溯权威顺序。核对日志、历史、消息与工件扫描结果，确保无秘密。外部结果未知时审计显示待核实，并能定位原操作和最后一项证据。
