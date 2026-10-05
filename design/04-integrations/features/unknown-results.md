# 功能：外部结果未知核实

> 实体ID：feature.integrations.unknown-results
> 来源：[原设计](../../../docs/PRODUCT-GUIDE.md)：§4.4、§7.3–7.4、§8.1、§8.3。
> 审校：[裁决记录](../../../docs/analysis/REVIEW-DECISIONS.md)：R-004–007、R-021–022。
> 基线日期：2026-10-02。

## 目标

在外部请求可能已生效但结果无法确认时，阻止盲目重放，按原身份核实真实副作用。核实的目标是确定原操作是否完成或安全未执行，而不是再发一次请求来制造新的可见结果。

## 输入

稳定 operation_id、原批准源版本、请求指纹、操作类型、原尝试身份与发送记录；GitLab 实例/namespace/project_id、目标 ref、预期 commit/manifest、pipeline 关联；超时、断连或崩溃信息，以及读取权限和核实预算。

## 输出

ReconciliationResult：确认成功、确认未执行、确认失败或仍未知，附目标、原操作、证据引用和可恢复阶段。成功复用原结果；未执行才允许原操作重试；失败按分类处置；未知保留 BLOCKED 和下一次可用核实条件。

## 算法 / 流程

1. 把不确定外部写标记为未知，保存原 DELIVER 子步骤与 resume_to，通知编排进入 BLOCKED，关闭相同副作用重试入口。
2. 读取原操作账本，验证业务身份和目标；创建按精确路径及所有权，推送按目标 ref 与 SHA/manifest，CI 按原 pipeline 或可归属的 SHA/ref 查询。
3. 比较权威外部证据与原请求指纹，区别相似对象、其他尝试和真正原结果；查询不可用不推断“没有执行”。
4. 原副作用完整成功时登记既有结果；确证没有执行才开放有限重试；存在部分成功或目标冲突时保留细分结果并要求诊断。
5. 恢复前重新校验状态、批准范围、预算和版本；核实不会复活终止 Run，终止后仅登记已发生副作用供新 Run 参考。

## 边界与异常

任意 commit、同名项目、最新 pipeline、HTTP 超时或取消都不是原操作成功/未执行证明。无法唯一归属流水线时不能猜测复用或重触发。核实自身是有限只读查询，不执行补偿删除、强制覆盖或新触发；清理与重做必须有新的明确范围及业务身份。

## 依赖

- [操作账本与请求指纹](../../06-governance/features/idempotency-and-concurrency.md#输出)：消费公开输出契约。
- [阻塞及恢复阶段](../../01-orchestrator/features/state-machine.md#输出)：消费公开输出契约。
- [读取权限与凭据](../../06-governance/features/permissions-and-secrets.md#输出)：消费公开输出契约。
- [核实查询预算](../../06-governance/features/budget-limits.md#输出)：消费公开输出契约。
- [原尝试审计证据](../../07-observability/features/audit-traceability.md#输出)：消费公开输出契约。

## 被谁依赖


- [健康检查与恢复 · 输入](../../09-deploy/features/health-recovery.md#输入)：消费本功能的公开输出契约。
- [CI 配置与首跑触发 · 输入](ci-config-trigger.md#输入)：消费本功能的公开输出契约。
- [GitLab 项目身份与创建 · 输入](gitlab-project.md#输入)：消费本功能的公开输出契约。
- [批准工件提交与推送 · 输入](repository-push.md#输入)：消费本功能的公开输出契约。
- [失败归因与恢复提示 · 输入](../../07-observability/features/failure-attribution.md#输入)：消费本功能的公开输出契约。
- [Activity 调度、重试与心跳 · 输入](../../01-orchestrator/features/activity-scheduling.md#输入)：消费本功能的公开输出契约。
- [多 Run 生命周期 · 输入](../../01-orchestrator/features/run-lifecycle.md#输入)：消费本功能的公开输出契约。
- [状态机 · 输入](../../01-orchestrator/features/state-machine.md#输入)：消费本功能的公开输出契约。

## 验收标准

分别注入建仓、推送和触发成功后响应丢失；核实复用原结果且外部副作用计数不增加。查询失败、对象归属不明和部分完成维持 BLOCKED。证明确未执行才可重试，取消不伪报已撤销；终止后核实不得推动原 Run 重回执行。
