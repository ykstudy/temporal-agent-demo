# 功能：Skill 公共契约

> 实体ID：feature.skills.skill-contract
> 来源：[原设计](../../../docs/PRODUCT-GUIDE.md)：§2.3、§4.3、§7.2、§9；L65–82、L199–211、L860–923、L1203–1216。
> 审校：[裁决记录](../../../docs/analysis/REVIEW-DECISIONS.md)：R-011、R-017、R-020。
> 基线日期：2026-10-02。

## 目标

把三个初始化能力声明为可版本化、可组合、可验证的 Skill，确保参数、写入范围、依赖与工具要求在执行前可审查。能力注册不意味着获得任意命令或密钥权限。

## 输入

skill_id/version、名称说明、支持栈、参数 Schema/默认值、依赖与冲突、模板引用、生成路径、固定 post command、授权/预算要求和输出验收；应用请求带 Run/operation/源版本与配置。

## 输出

SkillDefinition、Schema 验证结果、规范化配置与可执行能力请求；应用结果携带文件 manifest、定义/参数 hash、工件引用及分类失败。明确不支持、缺参数、越权和重复身份冲突。

目标SkillDefinition与parameters_schema的机器格式见 [Agent公共契约](../../contracts/AGENT-CONTRACTS.md) 和 [Schema](../../contracts/schemas/agent.schema.json)。默认值、支持栈、生成路径、固定工具许可、前置条件、依赖/冲突和验收都由版本化定义声明；现有枚举表/分散ValidatePlan仅是待迁移实现。资产及registry职责见 [模块映射](../../contracts/MODULE-MAP.md)。

## 算法 / 流程

1. 登记 add_database、add_auth、add_logging 的稳定身份与版本，检查支持栈、参数、模板及测试要求。
2. 校验必需/可选字段、枚举、类型、默认值、组合依赖与冲突；未知 Skill 或参数不默默忽略。
3. 把配置、版本和写路径纳入方案预览；确认后核对相同 source 与 operation identity，执行器只接收受控参数数组。
4. 按 manifest 验证输出，登记完成证据；重复同操作配置复用原结果，不同配置必须新版本/新操作并重新确认。

## 边界与异常

禁止用户/模型通过配置提供任意 shell、绝对路径、原始密钥或扩大权限。旧 marker 不足以复用不同参数结果。post command 仅在批准 cwd、过滤环境和时限内运行；已发生的部分文件/命令副作用随失败披露。语言或依赖不兼容明确拒绝。

## 依赖

- [工具和秘密要求](../../06-governance/features/permissions-and-secrets.md#输出)：消费公开输出契约。
- [组合约束](../../06-governance/features/constraint-validation.md#输出)：消费公开输出契约。
- [执行资源预算](../../06-governance/features/budget-limits.md#输出)：消费公开输出契约。

## 被谁依赖


- [AgentDefinition 版本 · 输入](../../02-agent/features/definition-versioning.md#输入)：消费本功能的公开输出契约。
- [可复核的工件交付 · 输入](../../09-deploy/features/reproducible-delivery.md#输入)：消费本功能的公开输出契约。
- [确定性约束校验 · 输入](../../06-governance/features/constraint-validation.md#输入)：消费本功能的公开输出契约。
- [工作区与工件隔离 · 输入](../../06-governance/features/workspace-isolation.md#输入)：消费本功能的公开输出契约。
- [认证技能 · 输入](authentication-skill.md#输入)：消费本功能的公开输出契约。
- [数据库技能 · 输入](database-skill.md#输入)：消费本功能的公开输出契约。
- [日志技能 · 输入](logging-skill.md#输入)：消费本功能的公开输出契约。
- [模板版本与快照 · 输入](template-versioning.md#输入)：消费本功能的公开输出契约。

## 验收标准

三个 Skill 定义均能独立校验与序列化；未知 ID/字段、无效枚举、冲突组合、越权 post command 在写文件前拒绝。相同操作同配置幂等；改参数不被旧完成标记跳过；输出可追踪版本、配置 hash 与实际文件。
