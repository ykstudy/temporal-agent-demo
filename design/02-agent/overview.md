# 决策与上下文域

> 实体ID：domain.agent
> 基线日期：2026-10-02。
> 来源：[原设计](../../docs/PRODUCT-GUIDE.md)；审校：[裁决记录](../../docs/analysis/REVIEW-DECISIONS.md)。

## 域职责

维护不可变 AgentDefinition，按预算构建上下文、保存摘要派生版本、请求语义模型并输出结构化建议。把用户的项目名称、技能、配置与变更意图组织成可验证的项目创建方案，保留当前阶段、批准范围与完成条件。

## 域边界

不直接写文件、创建仓库、触发 CI 或改 Run 状态；不将模型置信度、摘要中的“已同意”或自由文本当作批准。无 provider 配置时可按显式规则解析，但输出必须标明来源；定义或上下文改变不能静默迁移运行中的任务。

## 与其他域的关系

消费对话域的原消息与 Run 归属、编排域的当前状态、技能域的定义和模板版本、治理域的预算与约束、审计域的失败分类。向编排提供动作建议与校验结果，向对话和前端提供可审查的方案与解释，不暴露模型 provider 私有响应格式。

## 按改动节奏划域的理由

定义、语义协议、上下文与摘要随提示和项目参数策略一起演进；它们都影响模型见到的事实及建议质量，适合共同维护。工具权限和持久流程的改动节奏独立，因此仍归治理与编排。这是职责推断，不是历史变更次数统计。

## 功能清单

- [AgentDefinition 版本](features/definition-versioning.md)
- [模型决策建议](features/model-decision.md)
- [动作确定性校验](features/action-validation.md)
- [分层上下文构建](features/context-building.md)
- [保留原文的摘要压缩](features/context-compression.md)

## 规范与证据

功能契约规定目标行为；原文代码只保留为来源证据。实现与历史测试的边界见[实现状态](../../docs/analysis/IMPLEMENTATION-STATUS.md)，不据文件存在宣称本域已经全面交付。

