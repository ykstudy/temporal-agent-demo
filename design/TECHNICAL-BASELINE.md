# 文档技术基线与来源依据

日期：2026-10-02。本文件固定当前设计解释SDK接口时的版本，不声明这些版本是互联网上最新。技术选择经过本次裁决，已有运行证据提供可行性信息但不自动成为全产品规范。

| 项 | 文档基线 | 说明 |
| --- | --- | --- |
| 服务语言 | Go 1.25.0 | 与本会话可运行的保留服务技术环境一致；生成模板的Go版本由不可变定义另行锁定 |
| 编排SDK | go.temporal.io/sdk v1.38.0 | 文档中的UpdateOptions/handle、确定性Channel、handler完成与CAN基于该固定接口；升级需ADR及重放/恢复验收 |
| 项目模板 | Go template | 内嵌与版本引用，不通过模型字符串执行模板外任意脚本 |
| 仓库/CI | GitLab REST API v4 | 客户端SDK或HTTP适配器是可替换实现，契约以实例/namespace/项目/提交/流水线身份为准 |
| 初版对话存储 | JSONL原文 + 原子快照/派生摘要 | 明确单写者/存储引用/恢复边界，不等同多节点数据库事务 |
| 文档校验 | Python 3.9+ 标准库 | 未安装PyYAML时用严格Schema子集；有PyYAML时拒绝重复key的安全加载 |
| 前端框架 | Vue 3.5.43 / TypeScript 5.9.3 / Vite 7.3.6 | ADR-047确定Vue等价迁移；实际安装patch以frontend/package-lock.json为准，既有legacy API与v2目标分开 |
| 审查入口结构校验 | Ajv 8.20.0 + ajv-formats 3.0.1 | Draft2020-12公开ReviewBundle，配合浏览器来源字节摘要；不作为Agent运行Schema/权限裁决 |
| 审查离线生成验证 | Python隔离环境 jsonschema[format] 4.25.1 | 固定十一模型、来源/基线与语义边界，静态Schema同字节发布；独立于标准库认知checker |

## 官方契约依据

- [Temporal Go消息处理文档](https://github.com/temporalio/documentation/blob/main/docs/develop/go/workflows/message-passing.mdx)：Query、Signal、Update及validator分工。
- [Temporal处理消息的正确性说明](https://github.com/temporalio/documentation/blob/main/docs/encyclopedia/workflow-message-passing/handling-messages.mdx)：handler完成、幂等和更新校验的边界。
- [Go SDK v1.38.0 workflow接口](https://pkg.go.dev/go.temporal.io/sdk@v1.38.0/workflow)、[client接口](https://pkg.go.dev/go.temporal.io/sdk@v1.38.0/client)：设计接口依据；网页可用性不是文档校验器检查范围。

以上是技术依据，不把官方版本变化自动应用到当前冻结定义。原文里的旧客户端参数列表、原生channel与不完整传播示例只作为审校证据；适配器具体源码不作为功能之间的契约引用。

关联：[ADR-047](10-decisions/ADR-047-vue-readonly-review.md)、[审查发布](review/README.md)、[ADR-003](10-decisions/ADR-003-update-signal-query-contract.md)、[ADR-019](10-decisions/ADR-019-gitlab-rest-and-replaceable-adapter.md)、[消息契约](01-orchestrator/features/durable-messaging.md)、[检查点迁移](01-orchestrator/features/continue-as-new.md)。
