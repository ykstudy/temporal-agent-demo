# 功能：项目骨架生成

> 实体ID：feature.skills.scaffold-generation
> 来源：[原设计](../../../docs/PRODUCT-GUIDE.md)：§4.3、§7.1、§7.5、§13；L186–197、L770–859、L1122–1128、L1306–1308。
> 审校：[裁决记录](../../../docs/analysis/REVIEW-DECISIONS.md)：R-009、R-011、R-017、R-019。
> 基线日期：2026-10-02。

## 目标

将已确认的 Golang 基础项目方案写成隔离、可构建的工件包，包含清晰入口、模块、说明和忽略规则。生成只能使用用户审查过的模板快照，不能在执行时重新挑选最新模板。

## 输入

Run/operation 身份、合法 project_name、批准 source/proposal 版本、完整模板内容/hash、模块与语言基线、目标 workspace、工具预算及技能组合引用；需要生成的文件 manifest。

## 输出

ScaffoldResult、workspace 引用、文件清单/hash、生产者和模板版本、完成/部分/失败结果与验证引用。文件数量只描述实际产物，不能替代构建或功能通过结论。

## 算法 / 流程

1. 验证名称、版本、批准和工作区边界；拒绝未经确认、越权路径和无法解析的模板。
2. 在隔离区域按确认文件快照渲染/写入模块、入口、说明、忽略规则与确定的配置示例；逐项处理 I/O 错误。
3. 只运行明确允许的格式化、依赖整理或构建命令，环境中不携带执行器秘密；失败保留部分结果供诊断。
4. 核对所有必需文件与 hash，原子登记 manifest 和完成状态，再交独立验证与集成消费工件引用。

## 边界与异常

目录/文件创建失败不能继续标成功；拒绝绝对路径、..、未授权 symlink 和同名他人工作区。marker 存在但版本/参数/文件 hash 不匹配须重新核实。Worker 重启按原操作恢复；取消不删除已有产物或声称回滚。示例 env 只含占位，不种入凭据。

## 依赖

- [固定模板内容](template-versioning.md#输出)：消费公开输出契约。
- [当前确认范围](../../01-orchestrator/features/human-gates.md#输出)：消费公开输出契约。
- [工作区授权](../../06-governance/features/workspace-isolation.md#输出)：消费公开输出契约。
- [稳定操作身份](../../06-governance/features/idempotency-and-concurrency.md#输出)：消费公开输出契约。
- [所需文件/构建证据](../../07-observability/features/verification-evidence.md#输出)：消费公开输出契约。

## 被谁依赖


- [真实工件浏览 · 输入](../../05-conversation/features/artifact-browsing.md#输入)：消费本功能的公开输出契约。
- [可复核的工件交付 · 输入](../../09-deploy/features/reproducible-delivery.md#输入)：消费本功能的公开输出契约。
- [批准工件提交与推送 · 输入](../../04-integrations/features/repository-push.md#输入)：消费本功能的公开输出契约。
- [验收证据与证明范围 · 输入](../../07-observability/features/verification-evidence.md#输入)：消费本功能的公开输出契约。
- [认证技能 · 输入](authentication-skill.md#输入)：消费本功能的公开输出契约。
- [数据库技能 · 输入](database-skill.md#输入)：消费本功能的公开输出契约。
- [日志技能 · 输入](logging-skill.md#输入)：消费本功能的公开输出契约。

## 验收标准

确认前无项目文件；确认后真实模块能执行定义要求的 go test/build，manifest 与可浏览/打包内容一致。注入中途 I/O 错误、崩溃、同名目录与路径逃逸，验证失败不报完成、重复恢复不改已批准内容或覆盖他人文件。

