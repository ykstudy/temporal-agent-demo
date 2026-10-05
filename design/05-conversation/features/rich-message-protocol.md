# 功能：富消息结构协议

> 实体ID：feature.conversation.rich-message-protocol
> 来源：[原设计](../../../docs/PRODUCT-GUIDE.md)：§4.2–4.3、§5.4、§6.1、§10。
> 审校：[裁决记录](../../../docs/analysis/REVIEW-DECISIONS.md)：R-002–003、R-011、R-014、R-019、R-024；组件协议为本次补全。
> 基线日期：2026-10-02。

## 目标

把澄清文本、方案差异、运行进度、工件和验证结果统一为可持久引用的结构化消息，使不同 renderer 表达同一事实。显示组件不反向裁决运行状态，也不因会话离开或升级而失去原对象来源。具体目标字段由[消息协议 v2](../../contracts/MESSAGE-PROTOCOL.md)与[JSONSchema](../../contracts/schemas/message.schema.json)共同约束；现有 `/api` 扁平 Message 不等于本功能已完成。

## 输入

接收 canonical run_id/message_id、服务端 sequence、role/created_at、schema_version=2/contract_version=1.0.0、三轴 modes、方案 source_version、独立 state_sequence 与 parts。Part 具有 part_id/part_version/type/fallback_text；组件另带 component_id、不可变实例 component_version 和明确 component_schema。ObjectRef 固定对象类型、ID、版本、source_version/hash；按钮动作只含稳定命令意图和完整引用，不携带秘密。

## 输出

校验后的闭合 MessageEnvelope 与 text/component/diff/choice/action 五类版本化 Part；已知组件 payload 按各自 Schema 校验，未知组件仅有界 JSON 与纯文本回退。兼容读取结果另标非 canonical/只读，不伪装校验成功。状态文字携带三项独立模式和权威来源，区分已接收、处理完成、应用完成及 Run 完成。

## 算法 / 流程

1. 对闭合外层、五类 Part、已知组件 Schema 校验，再核对 Run 归属、ID/顺序、引用 hash 和字节/深度限额；Schema 合法不代替业务验证。
2. 将工具大结果登记为引用，消息仅保存摘要和结构字段；差异、关系及图数据使用节点/边结构，不把可执行脚本当协议。
3. 原消息与组件版本持久保存，proposal_version 内容不可变更；旧消息指向当时对象，最新详情明确差异，阶段进度只增加 state_sequence。
4. renderer 仅按本地注册类型呈现，安全处理 Markdown；未知组件纯文本回退。未知 Part/版本不能通过 canonical Schema，仅兼容层保存原文并只读显示，不加载动态 HTML、远程 renderer 或指令。
5. 用户动作发出明确命令，等待服务器回执后更新展示；流式片段在最终协议校验及登记前标为生成中。

## 边界与异常

模型生成的“通过”“已批准”和按钮外观都不能构成权威结论。缺失对象、版本过期或数据部分到达显示错误/待同步，可重试读取；不能拿旧卡片确认新方案。legacy 仅 text 只读适配，缺提案完整身份/hash 时禁动作，不补当前版本冒充旧来源。敏感字段不得藏在 payload；离线文档 apply 不授予运行批准。协议兼容失败不丢原文，也不静默把未知状态映射为 DONE。

## 依赖

- [持久消息与来源引用](jsonl-message-store.md#输出)：消费公开输出契约。
- [未应用方案及确认命令](preview-confirmation.md#输出)：消费公开输出契约。
- [权威阶段与版本](../../01-orchestrator/features/state-machine.md#输出)：消费公开输出契约。
- [可访问工件清单](artifact-browsing.md#输出)：消费公开输出契约。
- [验证证据](../../07-observability/features/verification-evidence.md#输出)：消费公开输出契约。

## 被谁依赖


- [对话工作区 · 输入](../../08-frontend/features/conversation-workspace.md#输入)：消费本功能的公开输出契约。
- [状态流同步 · 输入](../../08-frontend/features/state-stream-sync.md#输入)：消费本功能的公开输出契约。

## 验收标准

正反[契约样例](../../contracts/MESSAGE-PROTOCOL.md#校验层样例与实施验收)在 Go/TS 接受和拒绝一致；刷新及旧版本读取后五类 Part 身份、引用/hash 不变。未知组件和 Part 只读回退，已知错误组件不得绕过校验；恶意 Markdown 不执行。旧 proposal 卡片冲突或缺身份禁动作，流式未终结不显示应用成功。两个 renderer 状态与动作范围一致，本地构建不冒远程 CI，rules 不冒模型调用。
