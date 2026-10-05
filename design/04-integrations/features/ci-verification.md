# 功能：匹配流水线验证

> 实体ID：feature.integrations.ci-verification
> 来源：[原设计](../../../docs/PRODUCT-GUIDE.md)：§7.4–7.5、§13.5–13.7。
> 审校：[裁决记录](../../../docs/analysis/REVIEW-DECISIONS.md)：R-007–009、R-021、R-023–024。
> 基线日期：2026-10-02。

## 目标

独立证明本次批准且已推送的代码通过要求的 CI 首跑，并为最终交付提供完整可审计证据。未终态、身份不符、必需作业或报告缺失均不能完成，前置 Activity 的“成功”声明不能免除核验。

## 输入

交付模式、批准版本与 manifest、确认 project_id/ref/commit SHA、登记 pipeline_id；定义固定 required_checks、Runner 工具版本、预期构建/测试/安全报告、读取权限和总验证期限。证据来自特定流水线及作业，不能从最新列表自由挑选。

## 输出

VerificationReport，逐项记录 required check 的成功、失败、未就绪或不适用状态、来源身份、检查时间及工件引用；流水线终态和失败说明。远程 CI 必需项不可缺省，本地交付的不适用项明确展示，而不是伪造通过。

## 算法 / 流程

1. 在 DELIVER/remote_ci 根据批准交付模式固定检查集合，要求仓库身份、精确提交、匹配首跑、必需 job 及报告完整性。
2. 按 project_id/pipeline_id 查询，复核 pipeline 的 SHA/ref 与批准提交相同；身份不一致立即拒绝本轮验证。
3. 未终态按有限轮询和总期限等待；成功终态继续核验必需作业、构建工件及报告内容，不只读取整体 status。
4. 每个缺输入、不可访问、未生成或失败项保留在报告中；只在所有必需项通过时输出 remote_ci 已验证结果。
5. 把报告交最终证据汇总，状态机独立决定是否进入 DONE；超时与权限故障交诊断或阻塞，不由模型宣布完成。

## 边界与异常

取消、skipped、manual 待操作、允许失败的必需作业以及成功状态但报告缺失均不满足完成。另一个提交的绿色流水线不能代替当前失败。查询错误有限重试，不重新触发 pipeline；无法确认状态保持未就绪或 BLOCKED。测试脚本存在、模拟通过与真实 Runner 通过分别标明证据等级。

## 依赖

- [已登记首跑身份](ci-config-trigger.md#输出)：消费公开输出契约。
- [批准提交及 manifest](repository-push.md#输出)：消费公开输出契约。
- [固定验收检查与证据约定](../../07-observability/features/verification-evidence.md#输出)：消费公开输出契约。
- [查询与等待预算](../../06-governance/features/budget-limits.md#输出)：消费公开输出契约。

## 被谁依赖


- [可复核的工件交付 · 输入](../../09-deploy/features/reproducible-delivery.md#输入)：消费本功能的公开输出契约。
- [失败归因与恢复提示 · 输入](../../07-observability/features/failure-attribution.md#输入)：消费本功能的公开输出契约。
- [验收证据与证明范围 · 输入](../../07-observability/features/verification-evidence.md#输入)：消费本功能的公开输出契约。

## 验收标准

覆盖 pipeline ID 缺失、SHA/ref 错误、running、失败、取消、必需 job 跳过及报告缺失；每项必需检查仍出现在报告中且不得通过。只有匹配提交的真实成功首跑与完整工件允许 remote_ci 完成；delivery_mode=local 的 package 报告不宣称远程 CI 验证。
