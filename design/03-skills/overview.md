# 项目模板与技能域

> 实体ID：domain.skills
> 基线日期：2026-10-02。
> 来源：[原设计](../../docs/PRODUCT-GUIDE.md)；审校：[裁决记录](../../docs/analysis/REVIEW-DECISIONS.md)。

## 域职责

把已确认的 Go 项目方案转换为版本固定、参数明确、可核验的工件包。维护 scaffold、数据库、认证、日志与模板集合的公开能力契约；所有输出带生产者、模板/技能版本及文件 manifest，便于恢复和独立验证。

## 域边界

不决定 Run 转移或批准，不提供生产发布权限，不保存原始数据库/认证/GitLab 密钥，也不接受任意 shell post hook。生成的基础框架不等于完整业务系统；文件存在、标记存在或编译通过，不能替代数据库连接与安全行为的专项验证。

## 与其他域的关系

消费 Agent 的定义与参数建议、编排的版本绑定确认、治理的工具/路径/秘密约束。生成物交审计域登记验证证据，GitLab 与 CI 域按确认 manifest 提交和验证；前端仅消费公开工件引用，不能回写模板或批准事实。

## 按改动节奏划域的理由

基础项目与三个可组合能力围绕模板、语言版本、依赖、生成文件和测试一起调整，变化节奏与消息协议、GitLab API 集成明显不同，因此形成独立能力域。这里依据输出工件与版本生命周期分类，不推断不存在的开发历史。

## 功能清单

- [Skill 公共契约](features/skill-contract.md)
- [项目骨架生成](features/scaffold-generation.md)
- [数据库技能](features/database-skill.md)
- [认证技能](features/authentication-skill.md)
- [日志技能](features/logging-skill.md)
- [模板版本与快照](features/template-versioning.md)

## 规范与证据

功能契约规定目标行为；原文代码只保留为来源证据。实现与历史测试的边界见[实现状态](../../docs/analysis/IMPLEMENTATION-STATUS.md)，不据文件存在宣称本域已经全面交付。

