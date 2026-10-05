# 功能：GitLab 项目身份与创建

> 实体ID：feature.integrations.gitlab-project
> 来源：[原设计](../../../docs/PRODUCT-GUIDE.md)：§1.1、§4.4、§7.3、§8.3。
> 审校：[裁决记录](../../../docs/analysis/REVIEW-DECISIONS.md)：R-004、R-005、R-006、R-011、R-019。
> 基线日期：2026-10-02。

## 目标

为本次批准方案建立准确且可恢复的 GitLab 项目身份，避免把同名仓库误认成本次任务产物。创建与查询结果必须回答项目属于哪个实例、namespace、操作及源版本，随后推送只消费该身份。

## 输入

agent_run_id、稳定创建 operation_id、批准 proposal/source 版本、GitLab 实例与 namespace/path、描述和可见性；执行权限及凭据引用；已登记 project_id、所有权证据与历史尝试。默认 private，其他可见性必须在方案中明确展示并确认。

## 输出

RepositoryTarget、project_id、web/clone URL、默认分支、所有权证据、操作指纹及结果状态；明确未执行、确认成功、确认失败或未知。URL 不包含凭据。已存在但不属于原操作的项目返回目标冲突，不成为可复用结果。

## 算法 / 流程

1. 在 DELIVER/remote_repo 核对批准范围、阶段及 namespace 权限，冻结实例、完整路径和创建参数的操作指纹。
2. 优先按已登记 project_id 查询，并复核完整目标；未登记时查准确 namespace/path，不用展示名称进行模糊匹配。
3. 已存在项目只在原操作登记或可核验所有权标识一致时复用；仅明确不存在且有创建权限时提交创建请求。
4. 保存响应身份及证据，再返回成功。响应丢失或 Worker 崩溃留下原调用身份，转入未知核实，不提交第二次创建。

## 边界与异常

403、429、网络故障和不可解释的 404 不证明项目不存在；查询失败保持失败或阻塞，不进入创建。目标被转移、参数改变或 namespace 不符使旧结果失效。取消不删除已创建项目，清理必须是独立明确操作；本功能不操作既有项目的生产分支。delivery_mode=local 的 package 交付不创建虚构 URL。

## 依赖

- [版本绑定批准](../../01-orchestrator/features/human-gates.md#输出)：消费公开输出契约。
- [权限与凭据引用](../../06-governance/features/permissions-and-secrets.md#输出)：消费公开输出契约。
- [稳定操作身份](../../06-governance/features/idempotency-and-concurrency.md#输出)：消费公开输出契约。
- [创建结果核实](unknown-results.md#输出)：消费公开输出契约。

## 被谁依赖


- [批准工件提交与推送 · 输入](repository-push.md#输入)：消费本功能的公开输出契约。

## 验收标准

同一操作重试只得到一个 project_id；同名但不同 namespace、所有权不符、查询限流与权限失败均不得创建或接管。注入创建成功后响应丢失，恢复必须找到原项目并关联原操作；无法证明归属保持 BLOCKED。证据中实例、路径、批准版本和 project_id 完整一致。
