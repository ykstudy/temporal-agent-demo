# 功能：AgentDefinition 版本

> 实体ID：feature.agent.definition-versioning
> 来源：[原设计](../../../docs/PRODUCT-GUIDE.md)：§2.1–2.3、§5.3；L28–82、L538–554。
> 审校：[裁决记录](../../../docs/analysis/REVIEW-DECISIONS.md)：R-004、R-011、R-013、R-020。
> 基线日期：2026-10-02。

## 目标

给每个项目创建 Run 一份不可变、可追溯的能力定义，保证提示、动作协议、Skill 和模板不会因运行中升级而漂移。定义版本是产品能力身份，不是模型 provider 或 Temporal 执行 RunID。

## 输入

definition_id/version、支持栈、动作 Schema、可用技能及参数/模板版本、默认 CI 与仓库策略、系统规则和完成条件；定义发布请求、校验结果和已有 Run 的绑定引用。

## 输出

DefinitionSnapshot、稳定版本/hash、完整依赖 manifest 与可引用状态；非法、缺依赖或版本冲突结果。已有 Run 查询始终返回创建时或明确修订批准的快照，不能自动替换为最新版本。

目标字段与完整冻结包见 [Agent公共契约](../../contracts/AGENT-CONTRACTS.md) 和 [agent Schema](../../contracts/schemas/agent.schema.json)。逻辑definition_hash与依赖包snapshot_hash分开；系统提示正文/规则、支持栈、Skill参数Schema、动作Schema及完成条件进入冻结内容，不能只登记版本标签。当前legacy缺口按 [逐项矩阵](../../../docs/analysis/CONTRACT-GAP-MATRIX.md) 登记。

## 算法 / 流程

1. 组装定义及 Skill/模板依赖，核对支持栈、参数 Schema、动作白名单、工具范围和完成条件。
2. 计算规范内容 hash，登记版本与依赖引用；同版本不同内容拒绝，修订必须发布新版本。
3. 新 Run 按可用定义绑定快照；提示、方案与执行都引用同一版本，原始密钥只保留秘密引用的类型边界。
4. 运行中提出定义升级时创建新的变更/确认流程，重新验证源版本与影响，不能改变已批准内容；历史查询保留旧定义。

## 边界与异常

缺模板、已撤销依赖、非法动作或秘密内容不允许发布。SDK/Go/工具升级不自动更新既有定义。重做可选择明确的新定义，但不能继承旧批准；Continue-As-New 保持原定义引用。规则解析与模型解析都服从同一支持能力，模型不能自增 Skill 或扩大权限。

## 依赖

- [可组合能力定义](../../03-skills/features/skill-contract.md#输出)：消费公开输出契约。
- [模板版本及内容 hash](../../03-skills/features/template-versioning.md#输出)：消费公开输出契约。
- [能力预算边界](../../06-governance/features/budget-limits.md#输出)：消费公开输出契约。
- [可用模式与依赖状态](../../09-deploy/features/configuration-modes.md#输出)：消费公开输出契约。

## 被谁依赖


- [动作确定性校验 · 输入](action-validation.md#输入)：消费本功能的公开输出契约。
- [分层上下文构建 · 输入](context-building.md#输入)：消费本功能的公开输出契约。
- [保留原文的摘要压缩 · 输入](context-compression.md#输入)：消费本功能的公开输出契约。
- [模型决策建议 · 输入](model-decision.md#输入)：消费本功能的公开输出契约。
- [会话与 Run 归属 · 输入](../../05-conversation/features/session-run-management.md#输入)：消费本功能的公开输出契约。
- [配置与执行模式 · 输入](../../09-deploy/features/configuration-modes.md#输入)：消费本功能的公开输出契约。
- [有限预算与资源约束 · 输入](../../06-governance/features/budget-limits.md#输入)：消费本功能的公开输出契约。
- [确定性约束校验 · 输入](../../06-governance/features/constraint-validation.md#输入)：消费本功能的公开输出契约。
- [幂等与并发控制 · 输入](../../06-governance/features/idempotency-and-concurrency.md#输入)：消费本功能的公开输出契约。
- [权限与秘密边界 · 输入](../../06-governance/features/permissions-and-secrets.md#输入)：消费本功能的公开输出契约。
- [CI 配置与首跑触发 · 输入](../../04-integrations/features/ci-config-trigger.md#输入)：消费本功能的公开输出契约。
- [Continue-As-New · 输入](../../01-orchestrator/features/continue-as-new.md#输入)：消费本功能的公开输出契约。

## 验收标准

用两个定义版本创建任务，修改最新提示/模板后验证旧 Run 内容与 hash 不变。同版本内容变化被拒绝；缺依赖和密钥字段不发布。CAN 与重做均能追踪定义关系，已有批准不会自动授权新版本。
