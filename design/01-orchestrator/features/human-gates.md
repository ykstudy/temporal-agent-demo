# 功能：版本绑定确认

> 实体ID：feature.orchestrator.human-gates
> 来源：[原设计](../../../docs/PRODUCT-GUIDE.md)：§2.1、§5.2、§9；L28、L434–510、L1203–1216；原文缺少明确预览确认契约。
> 审校：[裁决记录](../../../docs/analysis/REVIEW-DECISIONS.md)：R-010、R-011、R-012、R-022。
> 基线日期：2026-10-02。

## 目标

使项目文件、远程仓库与 CI 的写操作依据具体可审查方案获准，保证用户确认绑定当前版本与范围。普通项目确认不隐含合并或生产发布权限。

## 输入

未应用 proposal_id/proposal_version、source_version、state_sequence、定义/模板/技能快照、差异/manifest、仓库/动作范围、检查；用户身份、确认响应与服务端 issued_at/expires_at。source_version 只在业务方案或授权范围改变时递增；普通阶段/进度仅变 state_sequence。每个提案版本绑定不可变内容，修订产生新版本并关闭旧版。

## 输出

GateDecision、对象/来源/动作绑定的批准记录、签发与到期时间、拒绝/过期/待确认和允许范围。有效接受不直接宣称已应用或完成；普通进度不改变业务来源和批准事实。

## 算法 / 流程

1. 展示方案并由服务端签发 UTC 时间，默认有效期 30 分钟，可配置 1 分钟至 24 小时；客户端只能回传身份，不能设置或延长期限。
2. 接受及首次应用前核验用户、当前 proposal/source、有效期和约束；源或提案内容改变使旧批准失效，state_sequence 变化不影响。
3. 登记批准后执行同一快照与目标；在有效期内启动的固定动作链可于原范围内完成长任务，仍复核权限、来源与预算，不因普通进度或到期撤销已核实副作用。
4. 拒绝关闭未应用提案并回 CLARIFY；修订重新预览，重复接受复用原批准/结果，不重新签发到期时间。

## 边界与异常

模糊接受不能批准；未应用撤销与已应用回退分开。首次应用超期、授权撤销或来源改变停止新副作用并重新审批；原已核实事实保留。暂停/未知不能扩大范围，客户 DB 迁移及合并/生产发布不属于普通批准，扩展须独立门禁。

## 依赖

- [绑定对象的确认请求](../../05-conversation/features/preview-confirmation.md#输出)：消费公开输出契约。
- [确认权限](../../06-governance/features/permissions-and-secrets.md#输出)：消费公开输出契约。
- [当前约束](../../06-governance/features/constraint-validation.md#输出)：消费公开输出契约。
- [确认内容快照](../../03-skills/features/template-versioning.md#输出)：消费公开输出契约。
- [当前动作预算](../../06-governance/features/budget-limits.md#输出)：消费公开输出契约。

## 被谁依赖


- [动作确定性校验 · 输入](../../02-agent/features/action-validation.md#输入)：消费本功能的公开输出契约。
- [方案预览与明确确认 · 输出](../../05-conversation/features/preview-confirmation.md#输出)：向本功能的公开输入提交请求。
- [模型与提案审阅 · 输出](../../08-frontend/features/model-review.md#输出)：向本功能的公开输入提交请求。
- [GitLab 项目身份与创建 · 输入](../../04-integrations/features/gitlab-project.md#输入)：消费本功能的公开输出契约。
- [批准工件提交与推送 · 输入](../../04-integrations/features/repository-push.md#输入)：消费本功能的公开输出契约。
- [状态机 · 输入](state-machine.md#输入)：消费本功能的公开输出契约。
- [项目骨架生成 · 输入](../../03-skills/features/scaffold-generation.md#输入)：消费本功能的公开输出契约。

## 验收标准

测试未确认、越权、旧 source/proposal、边界到期、客户端延期、重复接受与拒绝回 CLARIFY；首次应用前无项目副作用。进度序号变化不作废批准，期内已启动长任务不回滚事实；范围改变必须重新批准，普通接受不能授权客户 DB 迁移或生产动作。

