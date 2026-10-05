# 功能：状态流同步

> 实体ID：feature.frontend.state-stream-sync
> 来源：[原设计](../../../docs/PRODUCT-GUIDE.md)：§3、§5.3、§5.4、§10.1、§13。
> 审校：[裁决记录](../../../docs/analysis/REVIEW-DECISIONS.md)：R-002、R-003、R-013、R-022、R-023、R-024。
> 基线日期：2026-10-02。

原文给出 Update/Query，流式事件与游标为本次新增展示契约。当前工作区快照与请求竞态的有限证据见[实现状态](../../../docs/analysis/IMPLEMENTATION-STATUS.md)和[浏览器验收](../../../docs/evidence/BROWSER-QA.md)，不代替完整断线与缺口恢复验收。

## 目标

将服务端权威状态投影到工作区，正确处理刷新、断线、乱序和执行迁移。同步只负责观察与回执展示，不推进 Workflow，不把连接正常视为任务成功。

## 输入

接收 canonical run_id、带版本快照、事件身份与顺序、消息回执及重连位置；[消息协议 v2](../../contracts/MESSAGE-PROTOCOL.md)的 message_id/sequence、source_version/state_sequence 不混用。状态集合固定为 INTAKE、CLARIFY、PLAN、IMPLEMENT、VERIFY、DELIVER、DONE、PAUSED、BLOCKED、FAILED、CANCELLED；三轴 modes 独立读取。

## 输出

输出按同一 Run 归属的最新展示状态、来源版本、连接状态、数据新鲜度及待核实回执。已接收、已排队、命令完成和 Run 完成分别表示，审批与应用成功仅来自对应权威结果。

## 算法 / 流程

1. 初次读取带游标快照，从该游标订阅并补发读取与订阅间事件；无补发能力则订阅建立后重读快照核对。
2. 对 Envelope/Part 按 Schema 校验并核对 Run 归属，按 message_id/sequence 去重；历史消息内容与对象引用不覆盖为最新值。
3. 权威状态仅按 state_sequence 更新，业务方案 source_version 单独比较；序列缺口、来源不一致或游标失效则重取，普通进度不使批准过期。
4. Continue-As-New 保持业务 Run 与阶段；终止 Run 不复活，重做订阅新 Run。未知 Part/组件只读兼容，不从 fallback 推断状态或发命令。

## 边界与异常

断流保留最近结果并显示过期提示，不猜测未到达事件。Query 阶段与外部详细快照版本不一致时分别标注来源，等待核对。超时命令先查询原身份结果，不能换身份盲重发；模式缺配置显示阻塞，不静默降级或虚构 AI、仓库及 CI 成功。

## 依赖

- [持久消息](../../01-orchestrator/features/durable-messaging.md#输出)：消费公开输出契约。
- [状态机](../../01-orchestrator/features/state-machine.md#输出)：消费公开输出契约。
- [Run 生命周期](../../01-orchestrator/features/run-lifecycle.md#输出)：消费公开输出契约。
- [富消息协议](../../05-conversation/features/rich-message-protocol.md#输出)：消费公开输出契约。
- [配置模式](../../09-deploy/features/configuration-modes.md#输出)：消费公开输出契约。

## 被谁依赖


- [可访问交互 · 输入](accessibility.md#输入)：消费本功能的公开输出契约。
- [对话工作区 · 输入](conversation-workspace.md#输入)：消费本功能的公开输出契约。
- [进度与工件面板 · 输入](progress-artifact-panels.md#输入)：消费本功能的公开输出契约。

## 验收标准

重复、乱序、丢失、跨 Run 事件及断线重连不倒退或污染状态；message sequence 不替代 state_sequence，普通进度不改 source_version。执行迁移保持阶段，终止 Run 不复活；旧回执不覆盖新版本/对象 hash。未知/错误 Part 不更新权威状态或触发命令，三轴明确可见，刷新不把待处理批准显示为成功。已有请求竞态证据不代替本协议完整流恢复验收。
