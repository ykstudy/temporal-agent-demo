# 功能：模型决策建议

> 实体ID：feature.agent.model-decision
> 来源：[原设计](../../../docs/PRODUCT-GUIDE.md)：§3、§5.1–5.2、§6.2–6.3；L139、L286–300、L418–530、L643–727。
> 审校：[裁决记录](../../../docs/analysis/REVIEW-DECISIONS.md)：R-010、R-014、R-015、R-021、R-023。
> 基线日期：2026-10-02。

## 目标

把授权上下文中的项目意图转换为结构化澄清、参数、方案、诊断或下一步建议。模型帮助理解语义，不能产生批准、状态推进、文件或仓库成功事实。

## 输入

显式 decision_mode（model/rules）、语义请求类型、definition/source 版本、合法阶段、允许动作与输出 Schema、预算化上下文和 provider 就绪状态。只有非敏感授权内容进入模型，执行凭据和工具副作用不进入语义请求。

## 输出

DecisionProposal、解析来源、目标 Run/版本、动作及参数、澄清问题、理由、分类错误和可获得的用量。provider 未返回用量时标记未知，不伪造零用量；建议明确尚未批准/应用。

目标序列化按 [Agent公共契约](../../contracts/AGENT-CONTRACTS.md) 的DecisionProposal与 [Schema](../../contracts/schemas/agent.schema.json)。rules/model两分支返回同形候选对象，绑定run_id、source/state、完整定义包hash及来源；Plan只是其中的业务参数，不替代决策协议。模型返回候选经独立确定性校验才生成执行请求。

## 算法 / 流程

1. 校验 decision_mode、请求协议和预算：仅显式 rules 分支执行规则解析；model 分支要求完整 provider，缺配置返回 not_ready/BLOCKED，不自动切换模式。
2. model 分支发送纯语义请求并限制超时/输出；rules 分支不调用模型、不计模型用量，两者都不授予 Shell、文件或生产权限。
3. 解析响应并校验类型、必需字段和受支持枚举，保留原调用身份与分类失败，不采纳自由文本作为动作。
4. 输出候选建议交动作校验；必要信息缺失提出具体问题，完成建议仍交独立验证，失败诊断不自动改变来源。

## 边界与异常

模型声称已批准、已创建仓库、已测试或已修改权限时不能据此执行。格式错误、截断、不支持能力、超时、预算耗尽明确失败或有限修正；禁止无限空转。多个 Run 或提案归属不清须澄清，不能以置信度替代绑定。model 请求失败不降为 rules，保留原错误；改变 decision_mode 须有明确新选择和来源记录，不假装 AI 成功。

## 依赖

- [授权且预算内上下文](context-building.md#输出)：消费公开输出契约。
- [能力及输出协议](definition-versioning.md#输出)：消费公开输出契约。
- [模型调用预算](../../06-governance/features/budget-limits.md#输出)：消费公开输出契约。
- [结构化失败分类](../../07-observability/features/failure-attribution.md#输出)：消费公开输出契约。

## 被谁依赖


- [动作确定性校验 · 输入](action-validation.md#输入)：消费本功能的公开输出契约。
- [方案预览与明确确认 · 输入](../../05-conversation/features/preview-confirmation.md#输入)：消费本功能的公开输出契约。

## 验收标准

对相同 Schema 分别测试合法参数、缺字段、任意 shell、秘密读取、声称批准、未知 action、超时和无 provider。只有合法候选进入校验；调用自身不写工件、不改状态。显式 rules 不请求模型；model 缺配置或超时不调用规则分支。来源、错误与用量未知状态可辨认。
