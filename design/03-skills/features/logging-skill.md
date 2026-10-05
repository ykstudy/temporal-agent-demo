# 功能：日志技能

> 实体ID：feature.skills.logging-skill
> 来源：[原设计](../../../docs/PRODUCT-GUIDE.md)：§2.3、§7.2、§9.3、§10；L73、L877–920、L1218–1252。
> 审校：[裁决记录](../../../docs/analysis/REVIEW-DECISIONS.md)：R-009、R-017、R-019、R-020、R-024。
> 基线日期：2026-10-02。

## 目标

为生成项目提供固定配置、可测试、可脱敏的结构化日志能力。区分生成项目的运行日志与 Agent 的审计账本，避免把普通日志行当作批准、完成或外部操作证据。

## 输入

已确认 Skill/模板、工作区及日志 Schema：默认 info/json/console；file 的 path 默认相对 Run 工作区 logs/app.log，首版仅允许 logs/ 独立目录下且不得与源文件、manifest 或其备份路径冲突。max_size=10 MiB、max_backups=3、max_age=7 天。大小/保留参数为有界正值，调整须批准；on_write_error 默认 degrade，可批准为 fail_closed，秘密字段有固定脱敏规则。

## 输出

日志模块/配置、示例测试 manifest；debug/info/warn/error、json/text、console/file 的能力与结果，及初始化/写入/轮转错误、失败计数和健康状态。上述是规范目标；console/file生成实现与当前运行验收另在实现状态登记，本文不把源码存在或文档校验视为故障策略已通过。

## 算法 / 流程

1. 校验参数，file 只允许受准的 logs/ 目录内相对路径，拒绝逃逸/symlink及源文件、manifest、备份或目录冲突；限制 max_size 为 1 MiB–1 GiB、备份 1–100 份、max_age 为 1 小时–365 天。初始化目录/文件失败阻止生成服务启动。
2. 用 slog 生成时间、级别、事件和关联字段，脱敏后 append 写入；即将超 max_size 时轮转，备份按数量与时限清理，只操作登记日志路径。
3. 运行写入/轮转失败返回明确错误、增加失败计数、health=degraded，并向 stderr 输出脱敏诊断；磁盘满不得静默丢失或暗换 console。fail_closed 另将 readiness 置失败、停止接纳新业务写请求。
4. 验证级别、格式、append/轮转、保留和故障策略，登记 manifest 与报告；日志不能补全批准或作为外部操作完成事实。

## 边界与异常

单条超 max_size 明确拒绝，不截断伪装完整；stderr 失败仍保留错误计数与健康告警。DSN、认证及执行器秘密不得输出，file 故障不宣称日志持久化成功。生产采集/保留独立验收，普通日志成功不等于审计提交。

## 依赖

- [日志参数及默认值](skill-contract.md#输出)：消费公开输出契约。
- [日志模板快照](template-versioning.md#输出)：消费公开输出契约。
- [入口与文件 manifest](scaffold-generation.md#输出)：消费公开输出契约。
- [敏感字段过滤](../../06-governance/features/permissions-and-secrets.md#输出)：消费公开输出契约。
- [可写路径](../../06-governance/features/workspace-isolation.md#输出)：消费公开输出契约。
- [日志/审计关联规则](../../07-observability/features/audit-traceability.md#输出)：消费公开输出契约。

## 被谁依赖


当前基线无其他功能消费本契约；入口/外部使用者在该功能输入与验收中说明。

## 验收标准

验证 info/json/console 默认、枚举过滤、受限路径、append 和大小轮转，默认最多 3 份备份且超过 7 天清理。注入初始化拒绝、磁盘满和 stderr 失败，错误/健康可见；fail_closed 拒绝新写。canary 秘密不进入日志/ZIP，未实现能力不报已验收。
