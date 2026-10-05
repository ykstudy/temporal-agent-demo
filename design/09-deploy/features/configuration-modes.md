# 功能：配置与执行模式

> 实体ID：feature.deploy.configuration-modes
> 来源：[原设计](../../../docs/PRODUCT-GUIDE.md)：§1.2、§2.1、§2.2、§6.2、§9、§11。
> 审校：[裁决记录](../../../docs/analysis/REVIEW-DECISIONS.md)：R-009、R-015、R-016、R-019、R-023。
> 基线日期：2026-10-02。

## 目标

在启动与新 Run 创建时形成可复核配置，明确 Temporal 或 local 执行模式、规则或模型决策方式及交付能力。缺少依赖时暴露未配置状态，保持产品承诺与实际能力一致。

## 输入

服务配置、环境中的秘密引用、版本化 Agent 定义，Temporal 地址/命名空间/任务队列，存储与工作目录、模型端点和身份、上下文预算，以及 local/gitlab 交付选择。

## 输出

验证后的非敏感配置快照、execution_mode=temporal/local、decision_mode=model/rules、delivery_mode=gitlab/local、工具版本和能力就绪状态；三项模式分别记录，秘密仅保留执行时引用。每项 Run 绑定配置与定义版本，不因运行环境变化静默改写。

## 算法 / 流程

解析明确配置并校验合法值、必要字段、权限与预算。Temporal 是持久编排基线，local 是辅助运行能力，二者都需显式选定。规则提供者声明规则裁决，不宣称调用模型；模型模式验证端点和必要层预算。对 GitLab、Runner 与模型能力分别检查依赖，保存非敏感结果，再决定能否接受对应操作。

## 边界与异常

Temporal 不可用不能自动降为 local；模型不可用不能自动制造“AI 成功”。缺少远程依赖时 gitlab 交付保持未就绪或 BLOCKED，切换为本地需明确新选择和重新验收。令牌与密码不进入快照、日志、历史、浏览器或生成项目。配置加载不授权新的外部副作用。

## 依赖

- [定义版本](../../02-agent/features/definition-versioning.md#输出)：消费公开输出契约。
- [凭证与能力授权](../../06-governance/features/permissions-and-secrets.md#输出)：消费公开输出契约。
- [上下文和执行预算](../../06-governance/features/budget-limits.md#输出)：消费公开输出契约。

## 被谁依赖


- [AgentDefinition 版本 · 输入](../../02-agent/features/definition-versioning.md#输入)：消费本功能的公开输出契约。
- [Harness 服务运行 · 输入](service-runtime.md#输入)：消费本功能的公开输出契约。
- [进度与工件面板 · 输入](../../08-frontend/features/progress-artifact-panels.md#输入)：消费本功能的公开输出契约。
- [状态流同步 · 输入](../../08-frontend/features/state-stream-sync.md#输入)：消费本功能的公开输出契约。
- [有限预算与资源约束 · 输入](../../06-governance/features/budget-limits.md#输入)：消费本功能的公开输出契约。
- [权限与秘密边界 · 输入](../../06-governance/features/permissions-and-secrets.md#输入)：消费本功能的公开输出契约。
- [多 Run 生命周期 · 输入](../../01-orchestrator/features/run-lifecycle.md#输入)：消费本功能的公开输出契约。

## 验收标准

缺失 Temporal、模型凭证、GitLab 或 Runner 时分别显示真实原因，无静默降级与虚构结果。Run、界面和交付报告可识别执行模式与提供者；扫描所有公开配置输出确保无秘密，已有 Run 的版本引用不随配置编辑变化。
