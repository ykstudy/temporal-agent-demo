# ADR 索引

49 条记录均为 2026-10-02 当前有效文档基线，不表示已经实现。原作者未记录的理由和备选标为本次评估，原始日期不追溯编造。每条通过模型关系关联至少一个域和一个功能。

| 编号 | 决策 |
| --- | --- |
| ADR-001 | [采用 Go / Temporal 的项目创建范围](ADR-001-project-creation-scope.md) |
| ADR-002 | [Workflow 使用确定性 SDK 原语](ADR-002-deterministic-workflow-primitives.md) |
| ADR-003 | [固定 Update / Signal / Query 语义和 SDK 版本](ADR-003-update-signal-query-contract.md) |
| ADR-004 | [分离业务身份与执行身份](ADR-004-business-and-execution-identities.md) |
| ADR-005 | [固定运行 Definition / Template / Skill 快照](ADR-005-immutable-run-definition-snapshots.md) |
| ADR-006 | [模型提出结构化建议，规则保留控制权](ADR-006-semantic-model-and-rule-authority.md) |
| ADR-007 | [动作 Schema 校验与工具白名单](ADR-007-action-schema-and-allowlist.md) |
| ADR-008 | [首版 JSONL / 快照与单写者部署边界](ADR-008-jsonl-single-writer-storage.md) |
| ADR-009 | [分层注入模型上下文](ADR-009-layered-context-injection.md) |
| ADR-010 | [摘要派生，原始对话追加保存](ADR-010-append-only-raw-and-derived-summary.md) |
| ADR-011 | [执行 token 预算与输出预留](ADR-011-token-budget-and-output-reservation.md) |
| ADR-012 | [三项 Skill 注册与参数 Schema](ADR-012-three-skills-and-parameter-schemas.md) |
| ADR-013 | [Go template 渲染与模板版本](ADR-013-go-template-versioned-rendering.md) |
| ADR-014 | [PostgreSQL / GORM 默认与显式候选配置](ADR-014-postgres-gorm-default-and-explicit-alternatives.md) |
| ADR-015 | [JWT 默认、session 候选与外置密钥](ADR-015-jwt-session-ttl-and-secret-policy.md) |
| ADR-016 | [slog / JSON / console 默认与受控 file 扩展](ADR-016-slog-json-console-and-controlled-file-output.md) |
| ADR-017 | [工件 manifest 与原子登记](ADR-017-artifact-manifest-and-atomic-publication.md) |
| ADR-018 | [post hook 命令白名单](ADR-018-allowlisted-post-hooks.md) |
| ADR-019 | [GitLab REST 契约与可替换适配器](ADR-019-gitlab-rest-and-replaceable-adapter.md) |
| ADR-020 | [GitLab 精确 namespace 与目标所有权](ADR-020-exact-gitlab-target-and-ownership.md) |
| ADR-021 | [绑定提交 SHA 并核实推送](ADR-021-commit-sha-and-push-reconciliation.md) |
| ADR-022 | [CI 配置与触发身份去重](ADR-022-ci-configuration-and-trigger-identity.md) |
| ADR-023 | [CI 验证绑定批准提交](ADR-023-ci-source-verification.md) |
| ADR-024 | [CI 工具和报告可复现](ADR-024-reproducible-ci-reports.md) |
| ADR-025 | [凭据只在受控执行器使用](ADR-025-executor-secret-boundary.md) |
| ADR-026 | [权限按动作和精确目标约束](ADR-026-least-privilege-target-grants.md) |
| ADR-027 | [采用版本绑定的预览确认应用](ADR-027-version-bound-preview-apply.md) |
| ADR-028 | [项目确认与质量门禁分别定义](ADR-028-creation-quality-gates.md) |
| ADR-029 | [建立十一态封闭业务状态机](ADR-029-closed-business-state-machine.md) |
| ADR-030 | [暂停安全边界生效且终止重做新 Run](ADR-030-pause-cancel-new-run.md) |
| ADR-031 | [Continue-As-New 迁移完整检查点](ADR-031-complete-continue-as-new-checkpoint.md) |
| ADR-032 | [分类错误并限制重试期限](ADR-032-bounded-retry-timeouts.md) |
| ADR-033 | [按交付模式执行独立必需验证](ADR-033-independent-required-verification.md) |
| ADR-034 | [大对象采用可验证工件引用](ADR-034-versioned-artifact-references.md) |
| ADR-035 | [业务审计与非敏感上下文传播](ADR-035-audit-traces-no-secrets.md) |
| ADR-036 | [对话为入口并列展示进度和产物](ADR-036-conversation-first-workspace.md) |
| ADR-037 | [富消息采用可验证结构 Part](ADR-037-structured-rich-message-parts.md) |
| ADR-038 | [认知模型仅登记文档结构关系](ADR-038-document-cognitive-structure.md) |
| ADR-039 | [执行决策交付三轴模式显式区分](ADR-039-explicit-execution-decision-delivery-modes.md) |
| ADR-040 | [路线图以验收证据为退出条件](ADR-040-phased-evidence-based-roadmap.md) |
| ADR-041 | [服务运行以 readiness 和可恢复引用约束](ADR-041-readiness-backup-recovery.md) |
| ADR-042 | [文档冻结摘要与结构约束可执行](ADR-042-frozen-document-constraints.md) |
| ADR-043 | [参考来源和当前有效规范独立](ADR-043-reference-method-not-product-merge.md) |
| ADR-044 | [保留实现并单独登记证据边界](ADR-044-implementation-evidence-separate.md) |
| ADR-045 | [项目 Agent、设计模型与外部场景验证独立](ADR-045-agent-model-validation-separation.md) |
| ADR-046 | [公共契约先行并按结构符合性推进实施](ADR-046-contract-first-implementation.md) |
| ADR-047 | [Vue 等价迁移与独立只读可视化审查](ADR-047-vue-readonly-review.md) |
| ADR-048 | [完整冻结定义的内容引用与运行预算发布](ADR-048-definition-runtime-release.md) |
| ADR-049 | [受控执行、完整来源与预算证据账本](ADR-049-controlled-execution-ledger.md) |
| ADR-050 | [固定交付工具、远程证据与修复候选](ADR-050-controlled-delivery-evidence.md) |

## 分类

- ADR-001–011：产品边界、确定性、消息、业务身份、定义、模型、文件存储与上下文。
- ADR-012–024：技能、模板、隔离工件、GitLab目标、提交、CI与可执行报告。
- ADR-025–035：密钥权限、预览、门禁、状态、暂停终止、检查点、重试、验证与审计。
- ADR-036–047：对话展示、结构消息、文档模型、三轴模式、阶段交付、服务恢复、冻结、参考来源、证据边界、产品/模型/场景解耦、契约先行实施及Vue只读审查。

- ADR-048–049：完整冻结定义内容引用、运行预算发布、受控执行与来源/证据账本。

## 修订原则

修订记录保留旧决定和生效版本；涉及冻结规则遵循[维护约束](../DOCUMENT-GOVERNANCE.md)。决策关联和反向契约是结构检查，决策是否在运行中得到执行由独立证据验证。

| ADR-051 | [Original conversation and summary provenance](ADR-051-conversation-provenance.md) |

| ADR-052 | [运行观察、实际就绪与持久审计](ADR-052-runtime-observation.md) |
