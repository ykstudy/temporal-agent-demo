# 功能：方案预览与明确确认

> 实体ID：feature.conversation.preview-confirmation
> 来源：[原设计](../../../docs/PRODUCT-GUIDE.md)：§2.1–2.3、§5.2、§5.4、§9。
> 审校：[裁决记录](../../../docs/analysis/REVIEW-DECISIONS.md)：R-010–012、R-017、R-022；原文未定义预览确认，按本次审校补全。
> 基线日期：2026-10-02。

## 目标

在项目副作用发生前让用户审查具体方案，并把接受、拒绝和修订转为绑定当前对象与版本的操作。项目方案确认与结构认知模型的文档版本 apply 分开呈现，前者授权列明运行范围，后者只改变本地设计审阅版本。

## 输入

澄清后的项目参数、冻结 definition/template/skill 版本、当前 Run/source_version、文件计划及候选 manifest、仓库目标与 CI 范围、约束结果；稳定 proposal_id/version、用户选择、用户身份、命令 ID；服务端签发 issued_at/expires_at，默认有效期30分钟，可配1分钟至24小时，客户端不能延期。

## 输出

未应用 Proposal：前后差异、能力与参数、目标、影响、检查和版本；绑定准确 proposal/source 的确认、拒绝或修订请求及处理回执。proposal_version 内容不可变更；门禁批准另有来源，不由本卡片文字自行宣告。

## 算法 / 流程

1. 在 PLAN 生成未应用候选，展示技术栈、Skill 参数、预计文件变化、仓库可见性、三项独立 execution/decision/delivery_mode 及待检查项；预览没有真实生成工件。
2. 用户接受前明确对应对象与版本；请求携带稳定命令 ID、身份、范围和服务端签发期限的引用，交门禁复核当前约束、方案 source_version 及服务端有效期；接受和实际应用入口都复核。
3. 批准返回后冻结执行快照，开始 IMPLEMENT；普通进度只改变 state_sequence，不因其变化作废批准或改动 source_version；服务端时间期限仍生效。
4. 拒绝关闭未应用提案并回 CLARIFY；再改改保存修订意图，新版本重新预览，不延用旧确认。
5. 恢复持久卡片时重查对象状态；相同确认重发返回原结果，过期或来源改变提示重新预览而不是覆盖新版本。

## 边界与异常

本节过期拒绝针对接受/首次应用或到期后的新增范围；已在期限内开始的固定批准链可继续原范围，仍逐步核验来源、权限撤销与预算，不因普通进度或到期撤销已核实事实。

blocking 检查失败不能接受；模糊自然语言没有准确对象时不能批准。终止 Run 不接收新确认。已应用结果不能用拒绝按钮撤销，回退或重做须新的具体范围与流程。仅业务方案或授权范围变化增加 source_version。期限后新增高权限动作须复批，已核实副作用保留。普通确认不包括生产或客户数据库真实迁移/写入，G3/G4 本期不适用；文档回退不撤销运行副作用。

## 依赖

- [冻结模板与参数快照](../../03-skills/features/template-versioning.md#输出)：消费公开输出契约。
- [当前模型决策建议](../../02-agent/features/model-decision.md#输出)：消费公开输出契约。
- [当前约束结果](../../06-governance/features/constraint-validation.md#输出)：消费公开输出契约。
- [运行门禁输入](../../01-orchestrator/features/human-gates.md#输入)：提交公开输入契约。
- [对象状态与阶段](../../01-orchestrator/features/state-machine.md#输出)：消费公开输出契约。

## 被谁依赖


- [富消息结构协议 · 输入](rich-message-protocol.md#输入)：消费本功能的公开输出契约。
- [可访问交互 · 输入](../../08-frontend/features/accessibility.md#输入)：消费本功能的公开输出契约。
- [对话工作区 · 输出](../../08-frontend/features/conversation-workspace.md#输出)：向本功能的公开输入提交请求。
- [模型与提案审阅 · 输出](../../08-frontend/features/model-review.md#输出)：向本功能的公开输入提交请求。
- [版本绑定确认 · 输入](../../01-orchestrator/features/human-gates.md#输入)：消费本功能的公开输出契约。

## 验收标准

预览期间无真实项目文件或远程写入；未确认、越权、检查失败和版本过期不能进入实施。接受只应用同一快照，重复请求无重复副作用；拒绝返回 CLARIFY。文档模型 apply 不能触发项目实施或批准生产动作；刷新后待确认卡片及拒绝状态可准确恢复。
