# ADR-016 slog / JSON / console 默认与受控 file 扩展

> 实体ID：adr.016
> 来源：[原设计](../../docs/PRODUCT-GUIDE.md)：§2.3、§7.2、§9.3、§10；L73、L877–920、L1218–1252。
> 审校：[裁决记录](../../docs/analysis/REVIEW-DECISIONS.md)：R-009、R-019、R-020、R-024。

## 状态

accepted。生效日期为 2026-10-02；这是本次有效设计基线，不将来源示例、已有代码或测试结果追认为过去的批准。

## 背景

原文登记日志级别、格式和 console/file 目标，但没有区分生成项目日志与 Agent 审计，也未规定 file 路径、轮转、磁盘故障和秘密字段的处理责任。

## 决策

默认采用 slog/info/json/console；text/file 显式批准。file 默认受准相对路径 logs/app.log、max_size=10 MiB、max_backups=3、max_age=7 天，append 写入、超大小前轮转并清理过期备份。初始化失败阻止启动；运行写失败返回错误、health=degraded、stderr 脱敏诊断，禁止静默丢失。

## 理由

标准日志接口降低生成项目依赖，默认 JSON 适合结构化检索。可控输出边界防止用户路径变成任意文件写入，同时避免日志行被误用为批准或外部副作用的权威证据。

## 备选方案

立即引入第三方日志框架可满足扩展但增加版本面；隐式回退 stdout 掩盖 file 故障；无条件打印原始错误可能泄漏 DSN 或 token。采用明确状态与统一脱敏。

## 影响

on_write_error 默认 degrade，可在方案中批准 fail_closed（readiness 失败并停止新业务写）。文件路径、参数范围和轮转行为详见功能契约，磁盘满/诊断失败纳入验收。该 file 策略当前实现未完成；服务审计账本独立，不随项目日志关闭。

## 关联

- [项目模板与技能域](../03-skills/overview.md)。
- [日志技能](../03-skills/features/logging-skill.md)。
- [审计追溯](../07-observability/features/audit-traceability.md)。
- [INV-020 工作区严格隔离](../00-invariants/INVARIANTS.md#inv-020-工作区严格隔离)。
- [INV-021 密钥不进入数据平面](../00-invariants/INVARIANTS.md#inv-021-密钥不进入数据平面)。
- [INV-033 审计与传播边界](../00-invariants/INVARIANTS.md#inv-033-审计与传播边界)。

## 历史

2026-10-02：依据保留的原文章节和本次审校建立此决策。没有可证实的更早接受记录；未来替换须新增 ADR、说明旧新差异并更新关联，不改写本条历史。
