# 功能：确定性约束校验

> 实体ID：feature.governance.constraint-validation
> 来源：[原设计](../../../docs/PRODUCT-GUIDE.md)：§2.1–2.3、§5.2、§7.5、§9。
> 审校：[裁决记录](../../../docs/analysis/REVIEW-DECISIONS.md)：R-009–012、R-015、R-017、R-020–024。
> 基线日期：2026-10-02。

## 目标

在预览、批准、执行和恢复前用可执行规则校验项目创建范围，阻断不完整输入、越权动作和缺少证据的完成声明。规则引用当前项目规范，不因参考 harness 的来源副本存在而自动把其全部规则用于本业务。

## 输入

候选方案/动作、Run 当前状态/state_sequence 与方案 source_version、固定定义与模板/Skill 参数、目标、批准记录的服务端 issued_at/expires_at、预算和工件证据；constraint_id/version、适用范围、severity、可执行 rule 及所需事实。文档审阅请求另带模型 scope 和 base_version。

## 输出

ConstraintResult：每条规则的通过、违反或无法评估状态、证据及来源版本；blocking 放行结论、warning 列表与拒绝原因。运行检查与单用户文档模型 apply 检查分属明确 scope，不能互相替代。

## 算法 / 流程

1. 按当前定义、动作和交付模式收集适用规则，检查 Schema、Skill 组合、允许状态、批准版本、目标、工具范围及预算。
2. 为每条规则取得必要结构事实；事实缺失、未知规则或不可执行检查按不允许继续处理，不让模型评分替代。
3. 验证 manifest、固定 required_checks 和 source 绑定；blocking 拒绝，warning 记录，结果绑定候选 hash 与模型/运行版本。
4. 接受、应用和恢复重算当前结果及服务端期限；仅业务方案/授权范围变化增加 source_version，普通进度只改 state_sequence；proposal_version 内容不可变更。
5. 文档审阅只校验当前设计实体、关系、冻结规则、版本 CAS 及依赖完整性；回退先形成新提案，不改变运行授权、仓库或 CI 事实。

## 边界与异常

本节过期拒绝针对接受/首次应用或到期后的新增范围；已在期限内开始的固定批准链可继续原范围，仍逐步核验来源、权限撤销与预算，不因普通进度或到期撤销已核实事实。

模型可建议新约束，但不能删除、降低或自行更新有效基线；规则变化须可追溯 ADR 和授权。删除仍被依赖实体、悬空关系与规则修改在文档 apply 前阻断。空输入不能让必需检查从集合消失。默认确认30分钟、可配1分钟至24小时，客户端不能延期；过期后新增高权限动作须复批，已核实副作用保留。真实客户数据库迁移/写入不在普通创建确认范围；规则只允许模板及隔离测试。校验失败交状态机处置。

## 依赖

- [定义及能力 Schema](../../02-agent/features/definition-versioning.md#输出)：消费公开输出契约。
- [Skill 参数与组合声明](../../03-skills/features/skill-contract.md#输出)：消费公开输出契约。
- [当前状态与版本](../../01-orchestrator/features/state-machine.md#输出)：消费公开输出契约。
- [动作权限](permissions-and-secrets.md#输出)：消费公开输出契约。
- [剩余预算](budget-limits.md#输出)：消费公开输出契约。
- [固定验收条件与证据](../../07-observability/features/verification-evidence.md#输出)：消费公开输出契约。

## 被谁依赖


- [动作确定性校验 · 输入](../../02-agent/features/action-validation.md#输入)：消费本功能的公开输出契约。
- [方案预览与明确确认 · 输入](../../05-conversation/features/preview-confirmation.md#输入)：消费本功能的公开输出契约。
- [可复核的工件交付 · 输入](../../09-deploy/features/reproducible-delivery.md#输入)：消费本功能的公开输出契约。
- [模型与提案审阅 · 输入](../../08-frontend/features/model-review.md#输入)：消费本功能的公开输出契约。
- [验收证据与证明范围 · 输入](../../07-observability/features/verification-evidence.md#输入)：消费本功能的公开输出契约。
- [版本绑定确认 · 输入](../../01-orchestrator/features/human-gates.md#输入)：消费本功能的公开输出契约。
- [认证技能 · 输入](../../03-skills/features/authentication-skill.md#输入)：消费本功能的公开输出契约。
- [Skill 公共契约 · 输入](../../03-skills/features/skill-contract.md#输入)：消费本功能的公开输出契约。

## 验收标准

未批准、过期、非法参数、未知动作、工具越权、预算超限及缺完成证据均拒绝；旧通过不能放行新版本。文档依赖删除和规则修改被阻断，回退保留新历史；文档模型 apply 不批准运行操作。每项拒绝可关联规则、输入、版本和真实证据，模型自然语言不能改结果。
