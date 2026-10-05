# ADR-045 项目 Agent、设计模型与外部场景验证独立

> 实体ID：adr.ADR-045
> 来源：[原设计](../../docs/PRODUCT-GUIDE.md)：§1.1、§3、§7.5、§12–13；2026-10-02 用户最新范围授权。
> 审校：[裁决记录](../../docs/analysis/REVIEW-DECISIONS.md)：R-009、R-012、R-023–024。

## 状态

accepted（2026-10-02）；本次明确授权修改实现并删除不属于当前产品的运行耦合。取代 ADR-043/044 中继续保留来源副本及实验认知运行模块的阶段性决定，其历史记录仍保留。

## 背景

本项目已有独立的五层设计与结构认知模型，产品职责是通过对话创建 Go 项目并按交付模式验证产物。早期实验把参考 harness 文档副本与认知 HTTP 服务加入 Agent 启动及执行流程；此前在文档阶段按保留指令暂未移除。用户本次明确要求当前项目独立、按现有规范实现，并把外部场景调用放在独立 validation 中。

## 决策

项目创建 Agent 仅依赖本项目能力模块及显式配置的公共服务，不加载其他项目的文档、模型、源目录或运行模块。删除参考副本、实验认知服务及其配置和调用；必要的通用能力以本项目自己的参数 Schema、批准校验、manifest、身份账本和模板契约实现。

`design/_model` 留作当前设计结构登记和离线检查；其通过不能批准运行动作，也不是服务启动条件。独立 `validation/` 通过公开 HTTP、事件、下载及可选 Temporal 公共接口验证场景，不被产品导入，不成为产品的完成依赖。Agent 自身仍执行真实 test/build、manifest/秘密检查及匹配远程 CI，完整 required_checks 不能交给外部调用器替代。

## 理由

产品的输入、输出和强依赖应能从当前规范直接解释。移除方法参考的运行耦合后，用户只部署 Agent 所需服务；设计检查和外部验收可分别演进。区分三种证据，既避免把文档一致性声称为业务完成，也避免因场景调用器未运行就让真实产品无法自行验证交付。

## 备选方案

把 harness 继续作为公共运行依赖：引入与项目创建无关的源文件和权限范围，拒绝。把全部验证搬入 validation：产品无法独立判定 DONE，拒绝。删除现有设计模型或降低功能验收：失去当前规范和约束，拒绝。保留离线模型、当前 Agent 能力与外部黑盒调用三条独立边界，采用。

## 影响

更新运行说明、Agent 输入/输出/依赖手册、实现状态和维护指令，移除失效运行链接。当前 45 份功能仍是规范；本次修正不代表前端、生产运维或 live 第三方全部完成。公共依赖按 execution/decision/delivery 三轴声明，未配置和外部验收未执行保持可见；既有核实副作用和冻结原文/不变量不改变。

## 关联

- 域：[orchestrator](../01-orchestrator/overview.md)、[agent](../02-agent/overview.md)、[observability](../07-observability/overview.md)、[deploy](../09-deploy/overview.md)、[frontend](../08-frontend/overview.md)。
- 功能：[run-lifecycle](../01-orchestrator/features/run-lifecycle.md)、[action-validation](../02-agent/features/action-validation.md)、[verification-evidence](../07-observability/features/verification-evidence.md)、[reproducible-delivery](../09-deploy/features/reproducible-delivery.md)、[model-review](../08-frontend/features/model-review.md)。
- 规则基线：[有效不变量](../00-invariants/INVARIANTS.md)。

## 历史

- 2026-10-02：在先前文档阶段保留实现的决定之后，用户明确授权删除耦合并继续实现当前 Agent；本 ADR 记录新范围及旧决定替代关系。
- ADR-043/044 的历史保留，来源方法参考不重新成为运行依赖；原始 PRODUCT-GUIDE 和冻结 INVARIANTS 的字节与摘要保持不变。
- 后续增加公共能力须说明产品输入/输出、权限、强依赖与独立验收；不可通过场景脚本或参考副本静默扩展产品范围。
