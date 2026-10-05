# ADR-002 Workflow 使用确定性 SDK 原语

> 实体ID：adr.002
> 来源：[原设计](../../docs/PRODUCT-GUIDE.md)：§3、§5.2；L139、L305–391。
> 审校：[裁决记录](../../docs/analysis/REVIEW-DECISIONS.md)：R-001。

## 状态

accepted。生效日期为 2026-10-02；这是本次有效设计基线，不将来源示例、已有代码或测试结果追认为过去的批准。

## 背景

原文把 Workflow 调度说明成 Signal 通道，示例却在 Update handler 中使用原生 channel、阻塞读取与通道长度。常规 Go 同步设施不能据此成为持久重放的控制流程基线。

## 决策

Workflow 仅使用锁定 SDK 的 Channel、Selector、Await、确定性时钟和协作调度原语。模型、文件、网络、进程与外部随机结果由 Activity 返回；外部结果进入历史后作为后续控制输入，原生同步只留在服务和 Activity 内。

## 理由

控制过程必须在重放时作出相同选择。显式副作用边界同时支持测试、取消、重试与失败归因，使一次进程中的成功不成为恢复能力的唯一依据。

## 备选方案

原生 goroutine/channel 更接近普通程序，但绕过 SDK 调度；把所有逻辑都放进单个 Activity 则失去阶段可观察性和独立恢复边界。两种做法均不作为本期默认。

## 影响

审查需识别 Workflow 中所有外部调用，且以真实 Worker 重启和历史重放验证。模板渲染、模型请求或 GitLab 查询不得直接写进 Workflow；编译通过不替代确定性验收。

## 关联

- [编排域](../01-orchestrator/overview.md)。
- [持久消息](../01-orchestrator/features/durable-messaging.md)。
- [Activity 调度](../01-orchestrator/features/activity-scheduling.md)。
- [INV-002 Workflow 确定性](../00-invariants/INVARIANTS.md#inv-002-workflow-确定性)。

## 历史

2026-10-02：依据保留的原文章节和本次审校建立此决策。没有可证实的更早接受记录；未来替换须新增 ADR、说明旧新差异并更新关联，不改写本条历史。
