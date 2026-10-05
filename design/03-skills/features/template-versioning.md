# 功能：模板版本与快照

> 实体ID：feature.skills.template-versioning
> 来源：[原设计](../../../docs/PRODUCT-GUIDE.md)：§2.1、§2.3、§7.1–7.4；L28、L71–82、L770–1073。
> 审校：[裁决记录](../../../docs/analysis/REVIEW-DECISIONS.md)：R-008、R-011、R-017、R-020。
> 基线日期：2026-10-02。

## 目标

把基础与技能模板作为不可变发布单元，固定语言、依赖、工具和文件内容。保证预览、确认、执行、恢复与 CI 对同一模板快照进行验证，不因工作进程升级生成另一份代码。

## 输入

template_id/version、语言/工具/驱动基线、内容文件、支持参数 Schema、依赖/冲突、生成路径、测试要求和发布校验；应用请求引用批准的定义、参数和 source/proposal 版本。

## 输出

TemplateSnapshot、规范 hash/manifest、兼容性与发布状态、可追溯旧版本内容及生成映射。缺依赖、同版本内容冲突、未经验证或撤销版本明确不可用于新方案。

## 算法 / 流程

1. 规范化模板文件与元数据，校验路径、参数插值和固定 post command，不允许任意可执行片段或秘密。
2. 按支持栈与技能组合真实渲染并测试，锁定依赖/工具版本，验证构建工件及 CI 报告声明与输出匹配。
3. 登记内容 hash 与版本关系，发布后不可变；修正发布新版本，旧版本保留用于已绑定 Run 的恢复与证据。
4. 方案预览绑定完整内容/参数/manifest，确认后执行读取该快照；升级或撤销通过新方案和确认，不替换既有批准。

## 边界与异常

同版本不同内容、无法解析引用、路径逃逸与失效依赖拒绝发布。@latest、动态远程模板和升级 Worker 不能改变批准内容。撤销阻止新的应用，正在执行任务按安全和已知副作用裁决，不删除历史。SQLite 等平台/CGO 约束和 CI 工具兼容性必须显式登记。

## 依赖

- [能力与参数声明](skill-contract.md#输出)：消费公开输出契约。
- [模板/命令秘密边界](../../06-governance/features/permissions-and-secrets.md#输出)：消费公开输出契约。
- [模板生成路径](../../06-governance/features/workspace-isolation.md#输出)：消费公开输出契约。
- [可复现工具与交付基线](../../09-deploy/features/reproducible-delivery.md#输出)：消费公开输出契约。

## 被谁依赖


- [AgentDefinition 版本 · 输入](../../02-agent/features/definition-versioning.md#输入)：消费本功能的公开输出契约。
- [方案预览与明确确认 · 输入](../../05-conversation/features/preview-confirmation.md#输入)：消费本功能的公开输出契约。
- [可复核的工件交付 · 输入](../../09-deploy/features/reproducible-delivery.md#输入)：消费本功能的公开输出契约。
- [工作区与工件隔离 · 输入](../../06-governance/features/workspace-isolation.md#输入)：消费本功能的公开输出契约。
- [CI 配置与首跑触发 · 输入](../../04-integrations/features/ci-config-trigger.md#输入)：消费本功能的公开输出契约。
- [版本绑定确认 · 输入](../../01-orchestrator/features/human-gates.md#输入)：消费本功能的公开输出契约。
- [认证技能 · 输入](authentication-skill.md#输入)：消费本功能的公开输出契约。
- [数据库技能 · 输入](database-skill.md#输入)：消费本功能的公开输出契约。
- [日志技能 · 输入](logging-skill.md#输入)：消费本功能的公开输出契约。
- [项目骨架生成 · 输入](scaffold-generation.md#输入)：消费本功能的公开输出契约。

## 验收标准

修改最新模板或升级 Worker 后，旧批准任务生成字节仍与确认快照相同。非法路径、秘密内容、同版本变体及不兼容依赖发布失败；支持组合真实 test/build，声明的 bin/覆盖率文件确实存在。恢复、浏览与 ZIP 使用同一 manifest。

