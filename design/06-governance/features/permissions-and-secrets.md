# 功能：权限与秘密边界

> 实体ID：feature.governance.permissions-and-secrets
> 来源：[原设计](../../../docs/PRODUCT-GUIDE.md)：§1.2、§2.3、§7.3、§9.1–9.2。
> 审校：[裁决记录](../../../docs/analysis/REVIEW-DECISIONS.md)：R-010–012、R-019–020、R-023。
> 基线日期：2026-10-02。

## 目标

确保文件、仓库和 CI 动作只使用当前批准所需的权限，秘密仅由可信执行器在限定范围内使用。单用户部署仍执行动作和目标校验；模型、生成代码与测试子进程不因为同一服务持有 token 就获得它。

## 输入

用户/执行器身份、业务 Run、当前定义和批准版本、动作类型、目标实例/namespace/project/ref；必要 API 与工具权限、SecretReference、可信环境及凭据渠道、服务端 issued_at/expires_at 和撤销状态，默认30分钟、可配1分钟至24小时，客户端不能延期。权限以准确动作所需 scope 判断，不笼统要求多余权限。

## 输出

AuthorizationDecision、允许动作/目标/期限、受控秘密引用、过滤后的子进程环境和脱敏策略；越权、秘密泄漏或凭据失效拒绝及安全检查证据。批准的是项目创建范围，不是生产合并、发布或任意 Shell。

## 算法 / 流程

1. 将批准方案分解为文件生成、准确仓库创建/推送与首跑动作，按定义白名单核对用户、工具、目标及 source_version。
2. 可信执行器从部署环境解析秘密引用，通过 HTTP header 或受控短生命周期凭据渠道使用；不得写含 token 的 remote URL 或命令行。
3. Skill 固定参数数组限定 cwd、工具和过滤环境；生成项目的测试/构建不继承 GitLab 或模型服务原始凭据。
4. 对待登记消息、配置、日志、工件及包执行秘密边界检查；已识别敏感输入在接纳前拒绝，不将原秘密作为普通 JSONL 原文持久化。
5. 接受、应用及恢复时复核批准、权限和服务端有效期；过期后新增高权限动作须复批，已核实副作用保留，在途结果先核实。记录身份和结果，不记录 token。

## 边界与异常

本节过期拒绝针对接受/首次应用或到期后的新增范围；已在期限内开始的固定批准链可继续原范围，仍逐步核验来源、权限撤销与预算，不因普通进度或到期撤销已核实事实。

source_version 仅业务方案/授权范围变化增加，阶段进度仅改 state_sequence；proposal_version 内容不可变更。模型和文档 apply 不提供执行授权。普通接受不包含生产或客户数据库真实迁移/写入；数据库能力只生成模板及隔离测试，本期不设置 G3/G4。`.gitignore` 不证明历史或日志未泄漏；错误信息也要脱敏。权限失败属于不可盲重试类别，缺配置明确 BLOCKED，不伪造 GitLab/模型调用成功。

## 依赖

- [定义的允许能力](../../02-agent/features/definition-versioning.md#输出)：消费公开输出契约。
- [当前运行与执行模式](../../01-orchestrator/features/run-lifecycle.md#输出)：消费公开输出契约。
- [部署的凭据引用与环境范围](../../09-deploy/features/configuration-modes.md#输出)：消费公开输出契约。
- [授权审计登记](../../07-observability/features/audit-traceability.md#输入)：提交公开输入契约。

## 被谁依赖


- [动作确定性校验 · 输入](../../02-agent/features/action-validation.md#输入)：消费本功能的公开输出契约。
- [保留原文的摘要压缩 · 输入](../../02-agent/features/context-compression.md#输入)：消费本功能的公开输出契约。
- [真实工件浏览 · 输入](../../05-conversation/features/artifact-browsing.md#输入)：消费本功能的公开输出契约。
- [JSONL 原始消息存储 · 输入](../../05-conversation/features/jsonl-message-store.md#输入)：消费本功能的公开输出契约。
- [配置与执行模式 · 输入](../../09-deploy/features/configuration-modes.md#输入)：消费本功能的公开输出契约。
- [可访问交互 · 输入](../../08-frontend/features/accessibility.md#输入)：消费本功能的公开输出契约。
- [模型与提案审阅 · 输入](../../08-frontend/features/model-review.md#输入)：消费本功能的公开输出契约。
- [进度与工件面板 · 输入](../../08-frontend/features/progress-artifact-panels.md#输入)：消费本功能的公开输出契约。
- [确定性约束校验 · 输入](constraint-validation.md#输入)：消费本功能的公开输出契约。
- [工作区与工件隔离 · 输入](workspace-isolation.md#输入)：消费本功能的公开输出契约。
- [CI 配置与首跑触发 · 输入](../../04-integrations/features/ci-config-trigger.md#输入)：消费本功能的公开输出契约。
- [GitLab 项目身份与创建 · 输入](../../04-integrations/features/gitlab-project.md#输入)：消费本功能的公开输出契约。
- [批准工件提交与推送 · 输入](../../04-integrations/features/repository-push.md#输入)：消费本功能的公开输出契约。
- [外部结果未知核实 · 输入](../../04-integrations/features/unknown-results.md#输入)：消费本功能的公开输出契约。
- [审计与业务追踪 · 输入](../../07-observability/features/audit-traceability.md#输入)：消费本功能的公开输出契约。
- [Activity 调度、重试与心跳 · 输入](../../01-orchestrator/features/activity-scheduling.md#输入)：消费本功能的公开输出契约。
- [版本绑定确认 · 输入](../../01-orchestrator/features/human-gates.md#输入)：消费本功能的公开输出契约。
- [认证技能 · 输入](../../03-skills/features/authentication-skill.md#输入)：消费本功能的公开输出契约。
- [数据库技能 · 输入](../../03-skills/features/database-skill.md#输入)：消费本功能的公开输出契约。
- [日志技能 · 输入](../../03-skills/features/logging-skill.md#输入)：消费本功能的公开输出契约。
- [Skill 公共契约 · 输入](../../03-skills/features/skill-contract.md#输入)：消费本功能的公开输出契约。
- [模板版本与快照 · 输入](../../03-skills/features/template-versioning.md#输入)：消费本功能的公开输出契约。

## 验收标准

未确认、目标越界、过期/撤销权限、任意 post hook 和生产动作均在副作用前拒绝。检查进程参数、remote 配置、JSONL、Workflow payload、日志和 ZIP 不出现原 token；测试子进程环境无执行器凭据。重复或恢复不能绕过撤销，delivery_mode=local 没有远程写权限；execution_mode=local 配 GitLab 交付仍须准确授权和真实证据。
