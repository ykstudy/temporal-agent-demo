# ADR-021 绑定提交 SHA 并核实推送

> 实体ID：adr.021
> 来源：[原设计](../../docs/PRODUCT-GUIDE.md)：§7.3、§8.3、§7.5；L954–984、L1183–1200、L1093–1112。
> 审校：[裁决记录](../../docs/analysis/REVIEW-DECISIONS.md)：R-004、R-006、R-007、R-019。

## 状态

accepted。生效日期为 2026-10-02；这是本次有效设计基线，不将来源示例、已有代码或测试结果追认为过去的批准。

## 背景

原文在推送响应丢失后用 ls-remote 看任意提交或 fetch/status 判断成功，不能证明远端收到本次批准文件。带 token 的 remote URL 还会让凭据落入 git 配置与进程参数。

## 决策

提交绑定批准 manifest、来源版本、目标 project_id/ref 和确定提交 SHA。推送超时或取消列为未知结果，按原操作身份读取目标 ref 并核验同一 SHA 及必要内容；成功登记原结果，确认未执行才可安全重试，否则保持 BLOCKED。

## 理由

明确提交身份使仓库交付、CI 和产物验证回答同一个问题。超时只说明调用者未得到确认，无法证明外部没有执行；核实优先可以防止重复提交和错误完成。

## 备选方案

检查分支存在不足以证明版本；盲重推可能覆盖并发内容；每次重试生成新提交失去原操作身份。凭据由受控 header/helper 使用，禁止放入 remote 或命令参数。

## 影响

推送证据记录目标、SHA、核实时间和结果类别，冲突不自动强推。故障注入包含响应丢失、目标其他 SHA、并发提交、权限失败与秘密 canary，恢复仍关联原操作且后续 CI 使用确认 SHA。

## 关联

- [GitLab 与 CI 域](../04-integrations/overview.md)。
- [仓库推送](../04-integrations/features/repository-push.md)。
- [未知结果核实](../04-integrations/features/unknown-results.md)。
- [INV-006 副作用身份稳定](../00-invariants/INVARIANTS.md#inv-006-副作用身份稳定)。
- [INV-015 结果未知先核实](../00-invariants/INVARIANTS.md#inv-015-结果未知先核实)。
- [INV-017 验证绑定来源版本](../00-invariants/INVARIANTS.md#inv-017-验证绑定来源版本)。

## 历史

2026-10-02：依据保留的原文章节和本次审校建立此决策。没有可证实的更早接受记录；未来替换须新增 ADR、说明旧新差异并更新关联，不改写本条历史。
