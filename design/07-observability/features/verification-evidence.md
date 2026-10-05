# 功能：验收证据与证明范围

> 实体ID：feature.observability.verification-evidence
> 来源：[原设计](../../../docs/PRODUCT-GUIDE.md)：§7.5、§8、§13。
> 审校：[裁决记录](../../../docs/analysis/REVIEW-DECISIONS.md)：R-007、R-008、R-009、R-024。
> 基线日期：2026-10-02。

## 目标

为每项完成声明提供可复核证据，并区分设计约束、结构校验、编译测试、真实服务集成、第三方流水线和浏览器用户路径。让验收结论对应实际验证范围与版本。

## 输入

Run 与批准来源版本、交付模式、工件清单及哈希，验证命令和工具身份、开始结束时间、退出结果、原始日志引用，外部项目/提交/流水线标识及失败点。

## 输出

按验收项登记的结论、证据位置、验证对象、覆盖范围和未验证项。结果只使用通过、失败、未就绪或未验证等明确语义；提供工件与原始记录的双向关联。

## 算法 / 流程

从固定验收要求生成检查清单，绑定批准快照与实际产物；记录执行环境和原始结果，核对清单、源码与测试对象一致。远程结论绑定确认提交及流水线，本地结论绑定真实测试、构建与打包。汇总时保留缺口，不把前一版本、示例脚本或历史测试移植为本次证据；失败信息交给归因功能。

## 边界与异常

文档校验只证明结构与冻结摘要一致。编译成功不证明权限、重试或前端交互，模拟第三方不证明真实 GitLab/Runner 可用。日志丢失、对象哈希不符、要求缺项均不得完成。历史实现记录可供参考，但不能使规范中尚未实现的行为自动成为已验收。

## 依赖

- [约束检查要求](../../06-governance/features/constraint-validation.md#输出)：消费公开输出契约。
- [生成清单](../../03-skills/features/scaffold-generation.md#输出)：消费公开输出契约。
- [远程验证结论](../../04-integrations/features/ci-verification.md#输出)：消费公开输出契约。
- [交付清单与校验值](../../09-deploy/features/reproducible-delivery.md#输出)：消费公开输出契约。
- [失败归因](failure-attribution.md#输出)：消费公开输出契约。

## 被谁依赖


- [真实工件浏览 · 输入](../../05-conversation/features/artifact-browsing.md#输入)：消费本功能的公开输出契约。
- [富消息结构协议 · 输入](../../05-conversation/features/rich-message-protocol.md#输入)：消费本功能的公开输出契约。
- [模型与提案审阅 · 输入](../../08-frontend/features/model-review.md#输入)：消费本功能的公开输出契约。
- [进度与工件面板 · 输入](../../08-frontend/features/progress-artifact-panels.md#输入)：消费本功能的公开输出契约。
- [确定性约束校验 · 输入](../../06-governance/features/constraint-validation.md#输入)：消费本功能的公开输出契约。
- [匹配流水线验证 · 输入](../../04-integrations/features/ci-verification.md#输入)：消费本功能的公开输出契约。
- [Activity 调度、重试与心跳 · 输入](../../01-orchestrator/features/activity-scheduling.md#输入)：消费本功能的公开输出契约。
- [项目骨架生成 · 输入](../../03-skills/features/scaffold-generation.md#输入)：消费本功能的公开输出契约。

## 验收标准

给每项原设计验收场景建立输入、故障点、权威结果和证据索引。替换产物或流水线提交后旧证据失效；无真实执行记录时显示未验证，且报告明确列出浏览器、第三方和生产范围的证明边界。
