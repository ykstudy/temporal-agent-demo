# 功能：多 Run 生命周期

> 实体ID：feature.orchestrator.run-lifecycle
> 来源：[原设计](../../../docs/PRODUCT-GUIDE.md)：§2.2、§4.1、§5、§13；L44–55、L147–158、L280–588、L1306–1313。
> 审校：[裁决记录](../../../docs/analysis/REVIEW-DECISIONS.md)：R-003、R-006、R-022、R-023。
> 基线日期：2026-10-02。

## 目标

为每项项目创建需求维护独立、可追溯、可暂停和终止的 Run。把窗口选择、业务任务和 Temporal 执行链区分，避免切换对话混合上下文或复活已结束任务。

## 输入

新需求与客户端启动身份、会话/用户归属、Run 选择、暂停/恢复/取消/重做请求；当前版本、阶段、保存的 resume_to、外部依赖和操作核实证据，以及明确 execution_mode。

## 输出

Run 记录、独立消息/工件/历史关联、生命周期裁决与当前选择引用；重做输出 successor_run_id 和原 Run 关联。PAUSED/BLOCKED 记录恢复目标和原因，终止态保留已有副作用。

## 算法 / 流程

1. 按启动身份创建独立 INTAKE Run；将会话选择关联到它，后续命令只作用于指定 Run。
2. 暂停请求在安全边界生效并保存原阶段；正在执行的外部操作先记录已知或未知结果，不伪造撤销。
3. 恢复时核对版本、授权、预算、依赖及原操作结果，只有满足前置条件才回保存阶段。
4. 不可恢复失败为 FAILED，用户取消为 CANCELLED；DONE/FAILED/CANCELLED 的重做创建关联新 Run，从 INTAKE 重新接受继承的需求。

## 边界与异常

拒绝未应用提案回 CLARIFY，不终止整项任务。选择另一个 Run 本身不修改原 Run 或复制批准。未知结果核实前不能恢复写入；未开始应用的批准过期或取消后不能沿用；期内已启动固定动作链的已核实副作用保留，恢复仍核对来源/权限/预算。local 中断只能登记可检查的文件状态，不声称具备 Temporal replay；不同用户/会话的 Run 归属须校验。

## 依赖

- [合法生命周期阶段](state-machine.md#输出)：消费公开输出契约。
- [选择与消息归属](../../05-conversation/features/session-run-management.md#输出)：消费公开输出契约。
- [执行模式](../../09-deploy/features/configuration-modes.md#输出)：消费公开输出契约。
- [启动/重做身份](../../06-governance/features/idempotency-and-concurrency.md#输出)：消费公开输出契约。
- [恢复前核实](../../04-integrations/features/unknown-results.md#输出)：消费公开输出契约。

## 被谁依赖


- [会话与 Run 归属 · 输入](../../05-conversation/features/session-run-management.md#输入)：消费本功能的公开输出契约。
- [健康检查与恢复 · 输入](../../09-deploy/features/health-recovery.md#输入)：消费本功能的公开输出契约。
- [状态流同步 · 输入](../../08-frontend/features/state-stream-sync.md#输入)：消费本功能的公开输出契约。
- [有限预算与资源约束 · 输入](../../06-governance/features/budget-limits.md#输入)：消费本功能的公开输出契约。
- [幂等与并发控制 · 输入](../../06-governance/features/idempotency-and-concurrency.md#输入)：消费本功能的公开输出契约。
- [权限与秘密边界 · 输入](../../06-governance/features/permissions-and-secrets.md#输入)：消费本功能的公开输出契约。
- [审计与业务追踪 · 输入](../../07-observability/features/audit-traceability.md#输入)：消费本功能的公开输出契约。

## 验收标准

并列两项 Run 交错提交、暂停、切换、恢复和结束，断言消息/批准/工件不串用。暂停在下一安全边界生效；已终止任务恢复被拒绝。重复重做返回同一关联新 Run；新 Run 不能继承可执行批准，拒绝方案仍允许澄清。

