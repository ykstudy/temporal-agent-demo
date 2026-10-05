# ADR-039 执行决策交付三轴模式显式区分

> 实体ID：adr.ADR-039
> 来源：[原设计](../../docs/PRODUCT-GUIDE.md)：§1、§5、§12–13；R-023–024；新增部署边界。
> 审校：[裁决记录](../../docs/analysis/REVIEW-DECISIONS.md)。

## 状态

accepted（2026-10-02 本次审校后的有效设计基线；不是已实现状态）。

## 背景

§1、§5、§12–13；R-023–024；新增部署边界。原文保留为证据。当前项目的目标与参考方法不同，本记录对原文已述选择、冲突或缺口做明确裁决；原作者未记录的理由和备选不追溯编造。

## 决策

执行轴 temporal/local，决策轴 model/rules，交付轴 gitlab/local 独立选择并显示。temporal 缺服务、model 缺完整 provider、gitlab 缺权限/runner 均保持未就绪或阻塞，禁止静默切换；改模式必须生成可审查范围。

## 理由

本次评估：Temporal执行也可本地下载；规则解析并非模型调用；本地文件成功不证明GitLab首跑成功。三轴独立避免误把环境缺配置当同一件事。

## 备选方案

以下为本次评估，不表示原作者曾讨论：一个demo开关代替全部：误报能力；模型失败自动rules：用户看不出建议来源；GitLab失败自动本地DONE：改变授权范围。

## 影响

配置readiness按所选轴检查；local不提供Temporal持久保证；模式变化留下审计/提案，UI不显示不存在的远程URL。 领域只通过下列功能的公开输入/输出协作，运行行为由后续独立验收证明。

## 关联

- 域：[deploy](../09-deploy/overview.md)。
- 功能：[configuration-modes](../09-deploy/features/configuration-modes.md)。
- 规则基线：[有效不变量](../00-invariants/INVARIANTS.md)。

## 历史

- 2026-10-02：基于本项目原设计与用户授权自主审校形成明确裁决；包含本次纠错和补全。
- 原设计选择日期：原文未记录；本日不替代原始日期。
- 后续修订必须记录新旧决策和影响，不静默覆盖当前基线。
