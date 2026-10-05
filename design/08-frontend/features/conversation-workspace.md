# 功能：对话工作区

> 实体ID：feature.frontend.conversation-workspace
> 来源：[原设计](../../../docs/PRODUCT-GUIDE.md)：§1.1、§3、§4.2、§5.4、§6.1、§12。
> 审校：[裁决记录](../../../docs/analysis/REVIEW-DECISIONS.md)：R-002、R-003、R-011、R-014、R-022、R-023、R-024。
> 基线日期：2026-10-02。

原文提供对话入口，不规定具体布局。本文为新增展示规范；当前工作区工作区及有限浏览器证据见[实现状态](../../../docs/analysis/IMPLEMENTATION-STATUS.md)和[浏览器验收](../../../docs/evidence/BROWSER-QA.md)，不将其推为全部契约完成。

## 目标

提供项目创建的多轮澄清、方案审阅、明确确认和结果查阅入口。范围限于 Go 脚手架、GitLab 初始化与 CI 首跑；同一会话中的不同 Run 保持身份、消息和工件隔离。

## 输入

接收会话与 canonical run_id、符合[消息协议 v2](../../contracts/MESSAGE-PROTOCOL.md)的 MessageEnvelope/five Part、命令回执、当前方案完整引用和权威快照。消息 sequence 与 source_version/state_sequence 分别保留。三轴 modes 完整显式展示，不从回复措辞推断；现有扁平 text Message 只作为只读 legacy 适配，不算结构协议完成。

## 输出

呈现五类 Part、来源角色、原对象详情、待确认方案、任务状态和工件入口；未知组件显示纯文本回退，未知 Part 标只读兼容。用户操作形成稳定命令身份、目标 Run 和完整对象引用请求；客户端草稿、发送中、已接收和任务完成分别表达。

## 算法 / 流程

1. 进入会话先选择 Run，读取带顺序/版本的历史与快照；确认每条消息归属，客户端草稿不冒权威 sequence。
2. 按本地 renderer 注册表呈现已验证 Part；Markdown 禁原始 HTML，未知类型只显示转义 fallback，不执行 payload。
3. 旧卡片解析原对象版本/hash；完整 proposal 身份缺失时禁动作，明确重新读取当前方案不改旧消息引用。
4. 发送与确认保留原请求身份，服务端回执后更新对应 Run；切换更换订阅/工件范围并挡住旧请求，重做新 Run，终止历史保留。

## 边界与异常

断线、排队拒绝及身份冲突展示可解释错误，不能把提交动作当作成功。摘要须注明派生性质并允许查阅原文；未知外部结果保持待核实。显式选择规则提供者时显示 rules；模型模式缺配置显示未就绪，Temporal 不可用不得静默切换 local。凭据不进入消息正文。

## 依赖

- [会话与 Run](../../05-conversation/features/session-run-management.md#输出)：消费公开输出契约。
- [富消息协议](../../05-conversation/features/rich-message-protocol.md#输出)：消费公开输出契约。
- [预览确认](../../05-conversation/features/preview-confirmation.md#输入)：向公开输入契约提交请求。
- [状态同步](state-stream-sync.md#输出)：消费公开输出契约。
- [工件浏览](../../05-conversation/features/artifact-browsing.md#输入)：向公开输入契约提交请求。

## 被谁依赖


- [可访问交互 · 输入](accessibility.md#输入)：消费本功能的公开输出契约。

## 验收标准

五类 Part 在连续发送、重复提交、刷新及切换两个 Run 后身份与顺序正确，重复请求不增加操作。Schema 负例拒绝、未知/legacy 内容只读；旧引用/hash 失配不执行，缺完整 proposal 身份禁动作。失败消息可审查；暂停、恢复和重做等待服务端结果。三轴能力和证据来源可见，Markdown 注入不执行，离线文档 apply 不成为创建批准。
