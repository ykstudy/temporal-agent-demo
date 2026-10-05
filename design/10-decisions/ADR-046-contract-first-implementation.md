# ADR-046 公共契约先行并按结构符合性推进实施

> 实体ID：adr.ADR-046
> 来源：[有效规范](../README.md)、2026-10-02 用户“先补文档，由文档开始推进实施”；结构差异审计。
> 审校：[逐项差异](../../docs/analysis/CONTRACT-GAP-MATRIX.md)。

## 状态

accepted（2026-10-02）。本轮先补目标契约及实施依据，不修改产品/外部验证运行代码。补充ADR-005/007/012/037/040/044/045，明确后续结构迁移顺序；不修订冻结不变量。

## 背景

现有创建主线和部分门禁已有运行证据，但Definition只含部分摘要、Skill只有枚举、模型只产出扁平Plan、消息没有版本化Part、摘要缺派生历史。文档检查只验证文档关系，路径测试未逐对象核对目标契约。当前使用手册描述实际API，不能替代有效功能规定的目标。

## 决策

发布 [目标契约](../contracts/README.md)、本地JSON Schema、[模块映射](../contracts/MODULE-MAP.md)、[差异矩阵](../../docs/analysis/CONTRACT-GAP-MATRIX.md)及[阶段路线](../IMPLEMENTATION-PLAN.md)。目标wire `schema_version=2`、契约`1.0.0`，完整定义/技能、DecisionProposal→ActionValidation→ExecutionRequest、消息组件和SummaryRecord逐项迁移。

当前 `/api` 明确为legacy；目标 `/api/v2` 与严格公共对象按阶段提供，未迁移不得标成v2。版本化定义包含系统提示、动作/参数Schema依赖及完成条件，hash覆盖规范内容；已批准文件和原远程身份不能被迁移重写。旧资料缺来源事实只读保留，需新Run/新确认才能获得新的可信定义。现有目录可逐步按职责重排，但不能用文件重命名替代对象和行为符合性。

每阶段先核对公开契约、负例及兼容方案，再实现并记录Schema符合/跨对象语义/路径回归三类证据；S0通过仅表示文档可用于实施。无需为已授权阶段重复询问许可，涉及冻结规则或用户未授权范围另按既有约束处理。

## 理由

同一业务要求需要由文档、Schema、领域对象和独立验收共同追溯。先明确核心结构与缺口，可以保留可用实现又避免将简化实现反向写成规范。定义冻结必须覆盖真正影响模型/执行的内容，renderer必须消费持久对象身份。

## 备选方案

继续补当前Plan并以路径通过宣布完成：仍缺结构证据，拒绝。修改有效规范匹配简化实现：降低已定义要求，拒绝。立即全量拆包：风险大且不能先证明结构一致，拒绝。采用契约先行、显式legacy边界和逐阶段迁移。

## 影响

新增公共Schema和发布规则，修正手册与实现状态的口径，AGENTS要求以契约和差异驱动任务。结构认知模型继续只登记文档关系；schema发布源独立，不把业务payload塞入entities/relations。20/20和15/15历史路径结果保留；v2结构符合、live第三方与生产均需各自证据。

## 关联

- 域：[agent](../02-agent/overview.md)、[skills](../03-skills/overview.md)、[conversation](../05-conversation/overview.md)、[governance](../06-governance/overview.md)、[orchestrator](../01-orchestrator/overview.md)、[frontend](../08-frontend/overview.md)、[observability](../07-observability/overview.md)、[deploy](../09-deploy/overview.md)。
- 功能：[definition-versioning](../02-agent/features/definition-versioning.md)、[model-decision](../02-agent/features/model-decision.md)、[action-validation](../02-agent/features/action-validation.md)、[context-compression](../02-agent/features/context-compression.md)、[skill-contract](../03-skills/features/skill-contract.md)、[rich-message-protocol](../05-conversation/features/rich-message-protocol.md)、[conversation-workspace](../08-frontend/features/conversation-workspace.md)、[verification-evidence](../07-observability/features/verification-evidence.md)、[reproducible-delivery](../09-deploy/features/reproducible-delivery.md)。
- 规则基线：[有效不变量](../00-invariants/INVARIANTS.md)。

## 历史

- 2026-10-02：用户指出实施格式与文档不一致，经只读核对确认核心对象缺失；随后明确要求先补文档。
- 本决定补全序列化和实施门槛，不抹去旧API或通过结果，不修改PRODUCT-GUIDE/INVARIANTS字节和摘要。
- 后续契约变更同时更新Schema、功能输入输出、模块映射、差异/阶段证据及兼容版本；相同发布身份不同内容拒绝。
