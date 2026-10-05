# 从文档推进实现的公共契约

2026-10-05 S3发布为 [contract 1.4.0](releases/1.4.0/README.md)，见 [ADR-051](../10-decisions/ADR-051-conversation-provenance.md)。新定义3.3.0完整冻结摘要协议、原始Envelope/来源范围、摘要尝试及命令回执；旧发布原字节保留。最新阶段证据以实现状态为准，下方带日期段落保留当时基线。

2026-10-02 S2交付补充发布为 [contract 1.3.0](releases/1.3.0/README.md)，见 [ADR-050](../10-decisions/ADR-050-controlled-delivery-evidence.md)：固定Git/Go模块策略、完整远程与包回执、VERIFY候选及报告；1.0–1.2原字节和绑定身份保留。

2026-10-02 S2当前发布为 [contract 1.2.0](releases/1.2.0/README.md)，见 [ADR-049](../10-decisions/ADR-049-controlled-execution-ledger.md)。新增受控执行回执、预算/上下文账本和完整S2只读审计；固定旧对象hash规则不变，新增对象规则以1.2发布说明为准。1.0/1.1原字节及历史对象保留，按绑定版本独立校验；不把S2观察接口称为完整v2 Run。

2026-10-02 S1实施更新：当前新实现使用[contract 1.1.0发布源](releases/1.1.0/README.md)，修正模板原字节引用并补齐冻结预算，见[ADR-048](../10-decisions/ADR-048-definition-runtime-release.md)。下文1.0.0及schemas目录保留为原发布基线，不能覆盖其字节或以新包冒充旧身份。wire继续为schema_version=2；完整v2 API仍按阶段验收。

日期：2026-10-02。状态：目标契约 `contract_version=1.0.0`，公开 wire `schema_version=2`。这是下一阶段实施依据；当前代码仍是未声明 Schema 版本的 legacy API，不能据此声称 v2 已提供。

## 规范来源和阅读顺序

冻结不变量及有效功能/ADR定义业务要求；本目录把其公开对象落实为字段、类型和序列化约束。若 Schema 与有效功能冲突，先记录差异、修订本目录和相关 ADR，保持冻结原文，不让现有代码成为第三套规范。组织决定见 [ADR-046](../10-decisions/ADR-046-contract-first-implementation.md)。

1. [Agent 契约](AGENT-CONTRACTS.md)：输入、输出、强依赖、定义/技能/决策/执行/摘要/Run 结构与语义检查。
2. [消息协议](MESSAGE-PROTOCOL.md)：版本化消息 Part、对象引用、renderer 和旧消息适配。
3. [模块映射](MODULE-MAP.md)：领域、公开对象、当前代码及目标模块职责。
4. [逐项差异](../../docs/analysis/CONTRACT-GAP-MATRIX.md)：尚未符合规范的结构及证据。
5. [实施路线](../IMPLEMENTATION-PLAN.md)：依赖顺序、每阶段输入/产出/验收/退出条件。

对象摘要规则见 [HASHING](HASHING.md)；对象字段和参与hash的内容在其归属契约中唯一规定。

Schema正负样例及离线检查复现见 [SCHEMA-CHECK](SCHEMA-CHECK.md)。

## Schema 目录和发布

| 文件 | 职责 | 主入口 |
| --- | --- | --- |
| [common.schema.json](schemas/common.schema.json) | 身份、摘要、UTC时间、十一态与独立模式 | `$defs` 公共类型 |
| [agent.schema.json](schemas/agent.schema.json) | Agent/Skill 定义、建议、裁决、执行及权威 Run | `$defs/RunSnapshot`；其他对象使用其明确 `$defs` |
| [message.schema.json](schemas/message.schema.json) | 消息与五类 Part、组件和对象引用 | `$defs/MessageEnvelope` |

采用 [JSON Schema Draft 2020-12](https://json-schema.org/draft/2020-12/json-schema-core) 和[验证词汇](https://json-schema.org/draft/2020-12/json-schema-validation)。`$id` 的 `.invalid` 域名仅作发布身份；验证器从完整本地注册表解析 `$ref`，禁止网络获取。`format` 必须显式启用断言，尤其是 `date-time`；只把它当注释的配置不能算通过。

公共对象拒绝未知字段；合法参数映射和未知组件 payload 按各自明确的值 Schema校验。公开输入先拒绝重复 JSON key、非JSON数值和超限数据，再验证 Schema，不能依赖 decoder 默默保留最后一项。`default` 是文档注释，不负责自动补值；规范化结果重新校验并绑定 hash。跨对象身份、引用是否存在、时间有效性、权限、依赖、预算、状态转移和摘要内容真实性属于确定性语义检查，Schema通过不能代替它们。

`schema_version` 表示公开封装的破坏性变化，`contract_version` 标识本套契约，`definition_version`/`skill_version` 表示业务能力发布，`source_version` 是批准范围，`state_sequence` 是进度；不得用一个版本字段替代全部概念。当前 `version` legacy 字段只映射 `source_version`。

本目录是发布源。后续 Go嵌入 Schema、TS 类型和外部验收副本均是带来源摘要的派生发布物，不能手改形成第二真相。Agent运行不读文档认知模型、不执行文档checker；它使用编译/发布时固定的公共契约。外部验证消费同一发布版本，但校验断言与产品实现独立，不能根据接口实际形状降低 required/枚举/字段白名单。

## 兼容与实施门槛

目标公开入口为 `/api/v2`；本轮只是定义，当前 `/api` 不改。v2使用 `run_id` 与 `modes`，旧 `id` 与平铺模式由明确的 legacy adapter转换，不混在目标 Schema里。Schema不接受 `hash/sha256` 自动别名或真假状态的 OR 判定。接口细目由Agent与消息契约指定，执行顺序以实施路线为准。

旧数据缺完整 prompt、Skill Schema或对象版本时，不能由 adapter补出“曾经冻结”的事实。旧已批准文件快照及核实副作用按原身份恢复；未执行的旧方案若无法满足新契约，形成带 `retry_of` 的新Run和新确认，保留原历史。旧消息只读适配；动作仅在获得可核实完整对象绑定后开放。

目标v2 GET遇到无法完整投影的legacy Run时返回HTTP409、`code=legacy_contract_incomplete` 和只读 `legacy_read_url=/api/runs/{id}`，不返回缺字段的v2 RunSnapshot。客户端明确进入legacy只读显示，不自动重发v2写请求到legacy路由。原有已批准交付可在legacy既有授权范围按原身份恢复；新增范围、新确认或未冻结定义的模型调用转入关联新Run。完整目标错误外壳由Agent契约规定。

后续每个阶段先交付契约映射及所需负例，再实现对应模块和兼容边界。阶段退出同时要求结构符合与行为回归。原来的20/20场景和15/15浏览器结果继续作为当时代码的路径证据，不能折算为 v2结构已符合。S0文档检查的具体结果见 [设计检查报告](../../docs/DESIGN-CHECK-REPORT.md)。

## 独立运行观察协议

[运行1.0.0](runtime/1.0.0/README.md)于2026-10-05先行发布，完整Schema冻结审计、活动、指标、诊断及readiness。只读引用1.4业务对象，不改变原Definition或RunSnapshot。按提示词6实施和独立验收。
