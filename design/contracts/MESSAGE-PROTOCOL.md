# 消息协议目标契约 v2

状态：2026-10-02 的目标契约，`schema_version: 2`、`contract_version: 1.0.0`。本文落实 [ADR-037](../10-decisions/ADR-037-structured-rich-message-parts.md) 的五类 Part，机器结构唯一来源是 [message.schema.json](schemas/message.schema.json)。公开目标接口拟使用 `/api/v2`；现有 `/api` 的扁平 text Message 和当前 React renderer 尚未达到本文契约，不能将已有浏览器或业务流程通过当作 v2 协议验收。

本文不扩大生产操作范围，不改变阶段、模式、批准或预算。离线文档模型的 apply 只更新本地审阅文件版本，不是运行中的批准，也不能生成本协议的权威消息或可执行按钮。

## 外层、身份与顺序

`MessageEnvelope` 必须包含下表字段；外层闭合，不能用 metadata 或未声明字段携带额外授权、脚本、秘密及状态。共同标量和三轴模式引用 [common.schema.json](schemas/common.schema.json)，不复制另一份状态集合。

| 字段 | 意义与约束 |
|---|---|
| `schema_version` / `contract_version` | 分别固定为 `2` / `1.0.0`；协议升级经兼容决策与版本登记，不由客户端猜测 |
| `run_id` / `message_id` | 业务 Run 与稳定消息身份；同 Run 同消息 ID 不允许改变内容，跨 Run 同名不能被合并 |
| `sequence` | 服务端分配的 Run 内正整数消息顺序；表示持久登记顺序，不使用客户端时间排序 |
| `source_version` | 消息形成时的业务方案/授权范围版本；普通阶段与日志进展不递增它 |
| `state_sequence` | 消息形成时的权威进度序号，正整数；独立于消息 sequence 与 source_version |
| `role` / `created_at` | 角色仅 user/assistant/system/tool；时间由服务端记录，不能单凭角色或时间产生批准 |
| `modes` | `{execution_mode, decision_mode, delivery_mode}` 三轴完整对象；各自 temporal/local、model/rules、gitlab/local，不自动互推或降级 |
| `parts` | 1–64 个有序 Part；先后为同一消息内部的阅读顺序，不是 Task DAG |
| `reply_to_message_id` / `command_id` | 可选原消息与命令关联；不得冒充未登记消息或用新命令 ID 盲重放未知结果 |

完整 envelope 通过 JSONSchema 后，还要校验 Run 归属、消息 ID 内容一致性、sequence 唯一与单调边界。历史消息保留形成时的 source_version/state_sequence；读取最新快照不能反向改写它们。新版完整快照与订阅事件必须标明同一 Run 和权威序号，终止 Run 不因旧消息恢复为活动阶段。

## 五类 Part 与组件身份

所有 Part 都有稳定 `part_id`、当前 `part_version: 1`、`type` 和 `fallback_text`（最多 4096 字符）。同消息内 part_id 唯一；同身份/版本内容不可变。`fallback_text` 是只读兼容材料，不会被解析成工具调用、批准或导航命令。机器 Schema 用互斥 oneOf 选中五个分支；每个分支通过 `unevaluatedProperties: false` 关闭多余字段。

| `type` | 数据与确定性行为 |
|---|---|
| `text` | `text` 与 `format: plain/markdown`；正文不承担隐藏动作或权威状态判断 |
| `component` | `component_id`、`component_version`、`component_type`、`component_schema`、`payload`；由组件注册表选择只读 renderer |
| `diff` | 完整 `object_ref`、摘要、路径与 add/update/delete before/after 列表；展示差异，不写文件 |
| `choice` | 问题、single/multiple 选择方式和稳定 choice_id/标签；选择形成用户输入，不自动等同批准 |
| `action` | 标签与 Command 意图；目标引用和条件提案引用完整后才可进入确认流程，消息自身不执行 |

`component_id` 是组件实例的稳定身份，`component_version` 是该实例的不可变内容版本正整数；二者不表示 payload Schema 版本。`component_schema` 是明确的 payload Schema 标识，例如 `urn:agentstudio:component:progress:1.0.0`，末尾 `1.0.0` 才是结构版本；`part_version` 则描述外层 Part 结构。升级 renderer 不改变旧实例的内容或引用。

首版已知组件包括 clarification、proposal_diff、progress、artifact_manifest、verification_report。其 payload 在消息 Schema 内闭合：澄清仅问题/字段 ID；提案仅 proposal_ref/摘要/变更路径；进度仅合法阶段/摘要及 DELIVER 专属子步骤；清单和验证报告仅完整引用/数量或结果/摘要。完整 Proposal、Definition、Manifest 和验证明细保存在各自对象契约中，不内嵌另一份可漂移正文。组件显示的 passed/DONE 不代替服务器的 required_checks、批准或实际 Query。

## 稳定对象引用与动作边界

`ObjectRef` 固定为 `{object_type, object_id, object_version, source_version, hash}`，五项缺一即不满足 canonical v2；结构唯一来源是[公共 ObjectRef](schemas/common.schema.json)。消息 Schema 以 allOf 复用该闭合对象，再将 object_type 限定为本 renderer 支持的九类，不接受公共能力的其他类型。object_version 是下表规定的不可变版本正整数，不是 HTTP 协议版本或语义版本字符串。

| `object_type` | `object_id` | `object_version` | `hash` 的权威来源 |
|---|---|---|---|
| `run` | `run_id` | 被引用 RunSnapshot 的 `state_sequence` | 该历史 RunSnapshot 的 `snapshot_hash`，包含该快照自身的阶段/进度，不能用最新快照替换 |
| `definition` | `definition_id` | 完整 DefinitionSnapshot 的 `snapshot_version` | 该依赖包的 `snapshot_hash`；不是逻辑 AgentDefinition.definition_hash，definition_version 的 semver 不填入 object_version |
| `proposal` | `proposal_id` | `proposal_version` | 未应用提案的 `snapshot_hash`，不含后签发的 Approval 实体 |
| `plan` | `plan_` + 完整 64 位 `plan_hash` | `source_version` | 该来源版本已登记的 `plan_hash`；ID 长 69 字符，满足公共 Id 上限，不拼长 run_id、不截断摘要 |
| `message` | `message_id` | 固定 `1` | 完整 MessageEnvelope 经 JCS 后 SHA-256，Envelope 无自身 hash 字段；不是原 JSONL 行的字节 hash |
| `summary` | `summary_id` | `summary_version` | 独立 SummaryRecord 的 `summary_hash` |
| `manifest` | `manifest_id` | `manifest_version` | 完整工件清单的 `hash` |
| `artifact` | `artifact_id` | 来源清单的 `manifest_version` | 文件实际原始字节的 SHA-256，与清单中该文件 `sha256` 相同；source_version 也与来源清单相同，不是 ArtifactRef 结构的 JCS hash |
| `verification_report` | `report_id` | `report_version` | 独立 VerificationReport 的 `report_hash`；重验追加版本，不覆盖旧报告 |

全部引用都在 envelope.run_id 的命名空间内解析，不按全局 ID 搜索。source_version 记录该 Run 中对象的来源绑定版本，须与索引登记的来源相同；不能将当前 source 自动补到旧对象上。索引完整键为 `(run_id, object_type, object_id, object_version, source_version, hash)`；不同 schema_version 保留独立 namespace，legacy 字段不混入 v2 索引。同一 schema/run/type/id/version/source 已登记不同内容 hash 属于身份冲突，不能借索引含 hash 而允许改写同一版本。

上述 hash 必须来自权威不可变记录，不能用摘要文案 hash、最新对象 hash 或 renderer 临时计算替换。清单与文件字节的对应关系、报告的来源绑定仍独立核对；一条格式正确引用不证明目标存在。

引用解析在消息的 run_id 范围内完成；对象类型、ID、版本、source_version、hash 与权威记录逐项相同后才算解析成功。按[公共摘要规则](HASHING.md)，逻辑对象采用 RFC 8785 JCS 后 SHA-256，固定排除清单由对应对象契约规定；JSONL 原记录、文件、模板和已发布 Schema 用原始字节 SHA-256，两者不得混用。Refs 读取权威已登记 hash，不由 renderer 重算。当前业务 source_version 更新后，旧引用仍指向原对象，显示“历史版本”；不得自动重定向最新提案。旧卡片可以读取原对象，但旧批准不跨新 source 生效。普通 state_sequence 增加不使同 source 的批准失效。

Proposal/Approval 的不可变范围 hash 与当前生命周期状态分开，具体排除字段见 [Agent 契约](AGENT-CONTRACTS.md#固定哈希输入与无循环关系)。历史读取返回首次登记的归档对象，当前 status/started_at/revoked_at 来自最新 RunSnapshot；不改写旧归档，也不把旧对象显示的 pending/active 状态当作当前权限。

ActionIntent 固定 `command_id`、`action`、`target`，target 必须为原 Run 完整引用且 object_id 等于 envelope.run_id；它指向不可变历史 Run 快照。先核对该历史快照确实存在并 hash 相符，再独立复核当前业务 source 和阶段权限，不要求历史 hash 等于普通进度更新后的最新快照 hash。accept/reject/revise 额外要求 `required_proposal_ref` 为完整 proposal 引用。服务器重新查询完整提案与批准，核对 proposal ID/version/source/hash、定义和授权范围、状态及服务端签发的有效期；传入一个 Schema 合法的按钮不能绕过这些检查。命令 ID 绑定完整载荷，未知回执先查询原身份；同 ID 不同载荷冲突，终止任务重做须新 Run。

revision 输入在对应命令表单收集并按 Agent 命令契约验证，ActionPart 不夹带未声明的请求参数。按钮能够打开确认/修订表单，不表示表单已提交。普通创建确认不包含生产部署或客户数据库真实迁移，显示文本和 choice 均不能扩大其范围。

## 未知类型、旧消息与兼容层

**未知组件与未知 Part 采取不同规则。** 未登记的 component_type 可以通过 component 分支，但其 payload 只允许 JSON 值，字符串、容器数量均受 Schema 上限约束；另执行整体 UTF-8 序列化 64 KiB 和最大深度 8 的语义限制，component_schema 必须与 type 对应。客户端只显示 fallback_text 与“不支持的组件”标记，不递归解释其中的 action、HTML、URL、脚本或子组件，不加载远程代码。已经登记的组件若 Schema 标识/字段不正确，不能冒充未知组件绕过严格校验。

未知 `type` 或不支持的 `part_version` **不能通过 canonical v2 MessageEnvelope Schema**。读取兼容层在有界解析后保存原始记录与其字节摘要，作为非 canonical、只读展示对象：显示已有 fallback_text 或保留的纯文本，注明兼容错误；不把未知 Part 改成已验证 action，不持久化为“校验成功”的新 envelope。新写入和动作入口仍拒绝它。损坏或超限记录隔离并显示错误，不先执行再回退。

现 `/api` legacy Message `{id,role,content,created_at,kind,proposal_id}` 的只读适配，将 id 保持为 legacy message_id、content 作为一个 text 展示片段；run_id 仅从确知归属的读取接口取得。缺失 sequence、source_version、state_sequence、component_version 和 hash 时明确标记未知，不能把当前 Run 版本补到历史消息上，也不将这种显示对象输出为 canonical v2。保留原字节/原字段用于追溯，不重写已有 JSONL。

旧 Run 缺完整 DefinitionSnapshot/提示/Schema 等必要事实时，v2 GET 返回 `409 legacy_contract_incomplete` 并给出只读 legacy 入口；不返回填 null、空对象、当前提示或伪造 hash 的 v2 成功快照。legacy 入口的存在不向 v2 补授批准，已批准交付仅在旧身份和既有范围恢复；新增可信定义/范围须新 Run 和新确认。

legacy proposal_id 单独不足以组成 proposal 引用。没有 proposal_version/source_version/snapshot_hash 的旧卡片一律禁用批准、拒绝和修订动作，提供只读详情或显式重新读取当前待确认方案；重新读取的方案是新的权威对象，不改变旧消息引用。权限错误、对象缺失及 hash 不匹配分别展示，不用文字“已确认”修复。

## 安全渲染与持久化

renderer 由本地注册表选择，不执行模型提供的模板、动态 HTML、JS、表达式或组件路径。Markdown 禁原始 HTML；URL 使用协议白名单及明确外链策略，拒绝 javascript/data/file 和带凭据 URL，链接文本不能自动变工具命令。diff 路径仅作为相对标签，任何读取/下载仍由服务端 Run 工作区、规范化路径及权限校验处理。fallback 按纯文本转义，秘密、隐藏指令和不可控 metadata 同样不能进入记录。

进入持久 JSONL 前严格解析 JSON、拒绝重复键/非法数字，完成 Schema 与业务语义校验；单 envelope 的 UTF-8 字节上限为 256 KiB。来源消息、Part 版本与对象引用追加保存；压缩生成独立 SummaryRecord，其 source_range/hash 指向原始 message_id/sequence 和记录内容，不删除、改写或为旧按钮恢复权限。流式片段只作为临时生成内容，最终完整 envelope 登记成功后才获得稳定 sequence；未终结片段不能显示应用完成。

## 校验层、样例与实施验收

JSONSchema 证明字段类型、必填、闭合性、版本、已知组件形状和局部上限；`format` 断言必须显式开启。Schema 不能证明 ID 唯一、顺序连续、hash 与对象内容相同、跨对象/Run 归属、深度和总字节上限、阶段权限、批准有效或完成条件。这些是确定性语义校验器及独立运行验收的职责。Schema `$id` 是本地 catalog 的稳定资源名，不是联网下载指令；所有 `$ref` 只从本仓库对应文件解析，测试不得通过远程资源补齐。

可解析的正例见 [五类 Part](examples/message-valid.json)、[其余已知组件](examples/message-known-components-valid.json) 和 [未知组件只读回退](examples/message-unknown-component-valid.json)；Schema 必须拒绝 [外层额外字段](examples/message-invalid-extra-field.json)、[缺完整 proposal 引用](examples/message-invalid-proposal-ref.json)、[已知组件错结构](examples/message-invalid-known-component.json) 和 [未知 Part](examples/message-invalid-unknown-part.json)。样例中的 hash 是格式占位，结构通过不代表真实对象解析或批准通过。

后续实施必须独立证明：Go/TS 对同一样例接受/拒绝一致；所有五种 Part 经存储/分页/刷新/两个 renderer 保持身份；已知错误组件不能走未知分支；未知组件和 Part 只读兼容且不执行；旧对象 hash/版本冲突不能确认；乱序/跨 Run/缺口事件不覆盖新状态；恶意 Markdown 与超限 JSON 被安全处理；摘要后全部原始记录 hash 不变。文档结构 checker 只检查文档关系与来源，不是上述 runtime 符合性证明。
