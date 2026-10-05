# 功能：批准工件提交与推送

> 实体ID：feature.integrations.repository-push
> 来源：[原设计](../../../docs/PRODUCT-GUIDE.md)：§4.3–4.4、§7.3、§8.3、§9.1。
> 审校：[裁决记录](../../../docs/analysis/REVIEW-DECISIONS.md)：R-004–007、R-017、R-019、R-021。
> 基线日期：2026-10-02。

## 目标

把确认方案产生的完整工件提交到绑定仓库与分支，并证明远程存在的提交正是批准产物。推送成功由提交 SHA、目标和文件 manifest 共同说明，不能只靠命令返回或远程有任意 commit。

## 输入

Run/operation 身份、批准源版本、工作区及 manifest、目标 project_id/namespace/branch、预期远程基线和本地验证证据；固定 git 命令授权、过滤环境、凭据引用及超时预算。工件身份包含 CI 配置；可复用的既有提交须关联同一操作。

## 输出

CommitEvidence：本地 commit SHA、批准 manifest/hash、目标分支与核实后的远程 SHA、提交操作记录和无凭据 URL；或本地准备失败、远程冲突、权限失败、结果未知。完成结果只说明该推送，不替代 CI 首跑或 Run 完成。

## 算法 / 流程

1. 在 DELIVER/remote_repo 校验隔离工作区、批准 manifest 与实际文件，确认固定文件集合和本地验证证据未变化。
2. 在 Run 独占工作区初始化或复用 git；按冻结文件集合暂存和提交，记录 SHA 与 tree/manifest 映射，不把未批准文件带入提交。
3. 复核远程身份和预期分支基线；相同批准提交已存在则复用，存在其他改动则报告冲突，禁止强制覆盖。
4. 使用受控凭据通道推送，并核实远程目标 ref 对应精确 SHA；提交和证据登记完成后返回成功。
5. 网络中断、超时或推送后崩溃留下原操作与本地 SHA，提交未知核实，确认未执行才能安全重试。

## 边界与异常

manifest 不匹配、脏工作区包含额外文件或秘密扫描失败均在网络写入前拒绝。token 不进入 remote URL、命令参数、提交或工件包。分支被并发更新不自动合并；用户修改方案须重新确认。取消可能留下远程提交，应披露实际结果，不宣称自动回滚。

## 依赖

- [精确仓库身份](gitlab-project.md#输出)：消费公开输出契约。
- [批准版本与动作范围](../../01-orchestrator/features/human-gates.md#输出)：消费公开输出契约。
- [完整生成工件](../../03-skills/features/scaffold-generation.md#输出)：消费公开输出契约。
- [工作区与固定文件范围](../../06-governance/features/workspace-isolation.md#输出)：消费公开输出契约。
- [秘密与工具授权](../../06-governance/features/permissions-and-secrets.md#输出)：消费公开输出契约。
- [推送结果核实](unknown-results.md#输出)：消费公开输出契约。

## 被谁依赖


- [真实工件浏览 · 输入](../../05-conversation/features/artifact-browsing.md#输入)：消费本功能的公开输出契约。
- [可复核的工件交付 · 输入](../../09-deploy/features/reproducible-delivery.md#输入)：消费本功能的公开输出契约。
- [CI 配置与首跑触发 · 输入](ci-config-trigger.md#输入)：消费本功能的公开输出契约。
- [匹配流水线验证 · 输入](ci-verification.md#输入)：消费本功能的公开输出契约。

## 验收标准

远程 SHA 与批准工件 manifest 可双向追溯；额外文件、凭据 URL、并发分支更新和版本过期均拒绝。成功推送后断连恢复复用原 SHA，不额外提交。远程存在无关提交不得判成功；delivery_mode=local 的 package 交付明确没有远程推送证据。
