# 由契约推进实现的阶段计划

2026-10-02实施更新：用户已授权按提示词1–8推进，S1实施从[contract 1.1.0](contracts/releases/1.1.0/README.md)开始，修复原发布的内容引用歧义并补齐预算声明。下文阶段顺序及验收条件不变，原1.0.0文件与S0证据保留；新状态与实际结果逐项写入实现状态，不能从本说明推导S1已通过。

日期：2026-10-02。本文是后续实施路线，本轮只完成文档和独立 Schema，不修改产品代码、运行测试或重跑昂贵集成。差异基线见[逐对象矩阵](../docs/analysis/CONTRACT-GAP-MATRIX.md)，职责和目录目标见[模块映射](contracts/MODULE-MAP.md)。现有[实现证据](../docs/analysis/IMPLEMENTATION-STATUS.md)继续作为当时选定路径的事实，不提升为目标结构符合性。

## 交付状态与边界

| 状态 | 必须具备的证据 | 不由该状态推导的结论 |
| --- | --- | --- |
| docs_ready | 目标对象、字段、版本、Schema、跨对象语义、职责、迁移和验收案例可引用且相互一致 | 代码符合结构、现有 Run 可直接迁移、live provider 成功 |
| contract_ready | 产品和独立调用器使用相同的已发布契约；正例和逐字段负例通过；语义绑定及历史迁移有证据 | 真实模型、GitLab/Runner、生产部署成功 |
| path_verified | 某代码版本和配置下的具体用户链路、恢复或产物运行通过 | 全部功能、全部结构对象或全部兼容路径通过 |
| live_verified | 独立记录准确模型或 GitLab/Runner 实例、版本、目标、请求和结果的真实证据 | 其他模式、其他目标、生产运维已验收 |

当前外部 20/20 是 local 9、Temporal 9、模型重试定向 2 个所选 case，包含重复的前置和主线；浏览器 15/15 是主线 10 个与竞态/到期 5 个断言。这两组没有独立验证 MessageEnvelope、SkillDefinition、DecisionProposal 和完整 DefinitionSnapshot，不能作为 contract_ready。离线文档模型也只校验 Markdown/YAML 结构。

## 版本、契约入口和依赖

目标版本固定为 `schema_version: 2`、`contract_version: "1.0.0"`；`definition_version`、`skill_version`、模板版本、proposal_version、source_version 和 state_sequence 各有独立语义，不互相替代。目标公共接口使用 `/api/v2`；现有 `/api` 是 legacy，现有对象的 `id` 只在显式适配器中映射为 canonical `run_id`。RunSnapshot 使用 `modes.execution_mode/decision_mode/delivery_mode` 三个独立字段。

`AgentDefinition.definition_hash` 表示逻辑定义；`DefinitionSnapshot.snapshot_hash` 表示完整解析依赖包，包括冻结定义、Skill、Schema、模板等内容。下游 DecisionProposal、ActionValidation、ExecutionRequest 及批准/工件来源绑定字段 `definition_snapshot_hash` 必须等于该 snapshot_hash，不能把现有单个 `Definition.hash` 字符串改名后视为已经取得这份依赖包。

独立结构真值位于三个发布文件：

- [agent.schema.json](contracts/schemas/agent.schema.json)：主 `$ref` 指向 RunSnapshot，相关业务对象由其定义和引用组合。
- [message.schema.json](contracts/schemas/message.schema.json)：主 `$ref` 指向 MessageEnvelope，Part 及引用按类型约束。
- [common.schema.json](contracts/schemas/common.schema.json)：共享身份、版本、SHA-256、时间、模式、状态及其他公共基础类型。

运行 Schema 与 `design/_model/SCHEMA.md` 的文档登记 Schema 分开。实现目录服从模块映射；下面的现有文件是迁移切入点，文件名不构成跨域内部依赖授权。

哈希输入与规范化遵循 [HASHING](contracts/HASHING.md)，逻辑对象使用 JCS 后 SHA-256，Schema/模板/文件和摘要的原 JSONL 来源范围使用确切字节 SHA-256。[Agent契约](contracts/AGENT-CONTRACTS.md#目标-v2-http-与命令回执)已规定最小 v2 路径，以及 CreateRunRequest、MessageRequest、ControlRequest、CommandReceipt、SnapshotEvent 和 ErrorResponse；ControlRequest 直接引用 message.ActionIntent。snapshot SSE 的 payload 为 SnapshotEvent，通过完整 run_ref 读取对应历史 RunSnapshot。原消息范围由 SummarySource 登记；首版不新增独立 StorageRef 或分页路由。

实施依赖为 `S0 → S1 → S2 → S3 → S4 → S5`。S3 的存储设计可在 S2 后并行准备，S4 的类型生成和只读适配可在 S1/S3 对象冻结后准备；写接口和最终验收不能越过前置阶段的退出条件。每阶段保留工作区和历史证据，提交以契约对象为边界，禁止为快速得到绿色报告放宽 Schema。

## S0：文档与独立 Schema

**输入：** 有效不变量、功能输入/输出、ADR、字段差异矩阵，以及现有运行对象的只读清单。

**输出：** 三份目标 Schema、目标公开输入/回执/事件/错误、对象之间的引用与 hash/sequence 语义、功能到契约到模块到案例的追溯表、迁移规则和已标明范围的阶段路线。仅这些产物一致才标记 docs_ready。

**模块目标：** `design/contracts/` 保存公共契约正文、三个 schemas、版本策略和 MODULE-MAP；矩阵保存当前缺口。后续独立 oracle 位于 validation 的契约验证边界，不依赖 backend 内部类型、常量、模板或生成器。

**验收 oracle：** 从 schemas 文件加载 `$ref`；验证 required、类型、enum/const、pattern、边界和 additionalProperties；对合法样本及每类负例得到预期结果。JSON Schema 之外的 Run 归属、版本绑定、hash 内容范围、sequence 单调和不可变历史另有语义校验表。不能把 `action_schema_version` 字符串当作动作 Schema 已存在。

**兼容与回归：** 同时登记 v2 正例和 legacy 输入的显式适配结果。只通过文档和 schema 自检，现有 20/20、15/15 不需因此重跑，旧报告保留原范围。

**退出条件：** 矩阵每项都有来源、目标、阶段和负例；schema 主入口及内部引用可解析；本阶段发布的核心对象及最小公开接口没有待猜测的格式、字段别名或 hash 算法范围。扩展接口先完成对应文档和 Schema，不能在代码中猜格式。

## S1：Definition 与 Skill 冻结

**输入：** S0 发布的 DefinitionSnapshot、SkillDefinition 和基础类型；现有模板、参数、动作白名单、预算和模型配置。

**输出：** Run 创建时绑定的完整能力快照，包含可定位且可复核的系统提示、动作/参数 Schema、Skill 定义和依赖内容；运行构建上下文从该快照读取。定义版本、Skill 版本和内容 hash 能追溯，旧 Run 不读当前源码提示。

**模块切入：** 现 `backend/internal/agent/types.go`、`contracts.go`、`model.go`、`templates.go`、`service.go`；拆分职责按 MODULE-MAP，先形成契约类型/验证与定义注册边界，再替换调用链。现 `Skills.Parameters` 枚举迁移为有类型、required、默认值、范围和 additionalProperties 的参数 Schema。

S1实际落位为 `backend/internal/contracts/types_generated.go`及校验/hash模块、`internal/definitions/`注册/发布/快照/读取模块、`backend/assets/definitions/`与三Skill/四组模板原字节资产。`agent/model.go`读取Run绑定提示，`frozen_templates.go`读取绑定模板，源码提示和模板仅用于新版本发布。2026-10-02本阶段contract_ready和定向path_verified证据见[实现状态S1记录](../docs/analysis/IMPLEMENTATION-STATUS.md#第1阶段--s1完整定义与技能冻结验收)；其余阶段仍按下述退出条件推进。

**验收 oracle：** C01–C06。独立校验完整快照及依赖 hash；两个定义版本创建 Run，修改最新提示/Skill/模板后旧 Run 的内容、hash 和模型请求提示不变。同一 definition_version 或 skill_version 不得绑定不同内容。必需依赖缺失不能创建可信快照。

**兼容迁移：** legacy 未保存的历史 SystemPrompt 无法从当前 `model.go` 字符串还原。保留原文件和原批准/交付恢复账本，标记 legacy 来源不完整；只读展示不得宣称完整冻结。迁移工具不自动授予 v2 批准，需要新 Run 或按目标范围明确重新确认。

**回归及退出：** 原模板 test/build、三技能参数与批准 hash 回归继续通过；旧恢复链保持原范围。冻结提示和 Skill Schema 的差异项全部关闭，且没有把当前全局变量重新读入已冻结 Run 的路径，才进入 S2。

## S2：建议、校验与执行请求分层

**输入：** S1 的冻结 Definition/Skill；当前 Run、版本、预算和批准状态；model/rules 的纯语义输入。

**输出：** 具有明确 Run/版本/来源的 DecisionProposal；确定性 ActionValidation 结果；只有合法且授权的候选才形成受控 ExecutionRequest。执行器不接受模型输出直接构成命令。

**模块切入：** 现 `model.go` 的 `{plan, explanation}` 解码、`service.go` 的 Plan 赋值与动作 switch、`workflow.go` 的调度、`execution.go` 的执行参数；目标职责分别为建议协议、动作校验和执行请求，避免共用一个可随意填充的结构体。rules 也输出同一候选协议，不冒充模型来源。

计划文件目标为 `backend/internal/contracts/decision.go`、`validation.go`、`execution.go`，`internal/agent/decider.go`、`internal/governance/action_validator.go` 和 `internal/orchestrator/execution_request.go`；由组合根连接这些公开边界。固定流程继续保留，不新增运行 Task DAG。

**验收 oracle：** C07–C10。模型 fixture 从 ActionProtocol.model_output_schema 对应的 DecisionAdvice 样本派生，不照 `ModelPlan` 当前结构拼响应；服务绑定身份、版本和来源后的 DecisionProposal 再独立校验。合法建议只进入校验，只有 verdict=allowed 且 decision_kind=action 才产生 ExecutionRequest。未知 action、错误 Run/版本、未允许工具、任意 shell、声称批准或伪造 ExecutionRequest 全部拒绝且无新增副作用。model_usage.status=unknown/not_applicable 时 tokens 为 null，与 known 且零用量分开。

**兼容迁移：** legacy Plan 协议由明确 adapter 转成候选，保留其来源和缺失信息；不填造批准、可执行范围或模型用量。未知结果和失败 Update 的稳定业务身份继续保持，不能借新 v2 command_id 重发未知外部写。

**回归及退出：** stale/expiry/hash、幂等、预算、pause/cancel、终止重做、Temporal failed Update 重试及远程 sent/unknown 对账定向通过。候选、批准和执行三个对象的 Schema 与关联负例全部通过，才进入 S3。

## S3：MessageEnvelope、Part 与 SummaryRecord

**输入：** 冻结对象引用、消息/命令身份、Run 顺序、提案/工件/验证结果和摘要源范围。

**输出：** 可持久检索的版本化 MessageEnvelope 与按类型约束的 Part；旧消息绑定当时对象；成功摘要独立追加为 SummaryRecord，保留 source 范围/raw_jsonl_sha256、generator、summary_version 和 previous_summary_ref。失败单独登记并保留上一摘要，不伪造成功记录。原文与摘要历史均可追溯。

**模块切入：** 现 `types.go` 的 Message/ContextState、`store.go` 的 JSONL/快照、`service.go` 的 addMessage/compactContext、模型上下文读取。持久层按显式新版本记录和转换边界处理，不在读取时静默重写旧记录。

计划文件目标为 `backend/internal/contracts/message.go`、`summary.go`，`internal/conversation/messages.go`、`summaries.go`，`internal/storage/transcript.go`、`summary_store.go`；Envelope 的五类 canonical Part 是 text、component、diff、choice、action。unknown Part 只用于显式 legacy 适配回退，不是 canonical 第六支；未知 component 按协议保留有界 JSON 和 fallback_text。

**验收 oracle：** C11–C14。校验 MessageEnvelope/Part required、类型和引用；换 Run 的引用、旧组件改内容、缺版本、未知可执行组件均拒绝或按协议安全回退。压缩前后原文身份/hash 不变；摘要 v1/v2 可分别读到其源范围；失败保留上一摘要，不丢原文或扩大授权。

**兼容迁移：** legacy `id/role/content/kind/proposal_id` 可只读映射为明确 legacy text Part；没有的对象版本、原文 hash 或 summary history 标为不可恢复，不伪造。保留已登记 ProposalHistory 的原对象；摘要当前字符串不能补造历次 SummaryRecord。

**回归及退出：** 原消息去重、并发、截断恢复、跨 CAN 原文保留及摘要预算回归通过；持久回执、原消息和派生记录符合目标 Schema，旧 proposal 引用不指向当前提案，才进入 S4。

## S4：v2 API 与前端严格消费

**输入：** S1–S3 已通过的业务对象与持久引用；确定的 `/api/v2` 路由、模式、快照和事件契约。

**输出：** 以 canonical run_id、嵌套 modes 和显式契约版本输出的 RunSnapshot/消息/回执/事件；前端严格类型和入口运行校验；legacy `/api` adapter 保留现有读与受限恢复链。旧字段仅存在于兼容边界，v2 不接受别名。

**模块切入：** 现 `http.go` 的直接 Run 序列化、SSE 端点、`frontend/src/types.ts`、`lib/api.ts`、`App.tsx`、`components.tsx`。目标按照公共快照和 Part renderer 组织，领域内部类型不直接充当 HTTP DTO；类型生成或手工维护都需对 Schema 验证。

计划文件目标为 `backend/internal/httpapi/v2.go`、`legacy.go`、`projector.go`，`frontend/src/contracts/v2.ts`、`runtime.ts`，`frontend/src/renderers/MessageRenderer.vue`、`LegacyMessageAdapter.ts`；App/components只组合公开对象与交互，不成为另一个字段真值。

**验收 oracle：** C15–C17。HTTP/SSE 响应和前端入口都加载目标契约；未知字段、bool 充当版本、扁平 modes、`id` 代替 run_id、manifest 文件用 `hash` 代替 `sha256`、缺组件引用均不默默接受。ObjectRef 的规范字段仍是 `hash`，不能反向统一成文件摘要字段。事件去重、缺口、跨 Run、断线重取和旧版本组件展示有定向结果。

**兼容迁移：** 只有完整可信来源才可投影 canonical v2；无法完整投影的 legacy Run 返回 409、code=legacy_contract_incomplete 和 legacy_read_url，前端进入 legacy 只读展示。不产出 partial v2，不自动把 v2 写请求重发到 legacy。v2 写入只接受 v2 命令；已批准 legacy 交付可在既有范围恢复，不通过 adapter 自动获得新增范围。保留回滚时可读的原始文件和版本标识。

**回归及退出：** 现有浏览器主线/窄屏/竞态作为回归保留，另验 Part、旧对象、required_checks/manifest 身份和来源显示。所有 v2 接口、持久投影与前端消费者通过结构及语义验证，才进入 S5。

## S5：独立契约场景与主线回归

**输入：** 发布的三份 Schema、语义校验表、合法与逐类负例样本、S1–S4 产品、legacy 数据样本和对应版本标识。

**输出：** 分别记录 contract、legacy migration、path、mock/live 的独立报告；每例关联功能 ID、契约 `$ref`、代码版本、Schema hash 和实际 evidence，缺前置保持 not_run。

**模块切入：** 后续 `validation/` 契约 oracle、HTTP 场景、model/GitLab fixtures、浏览器脚本；不导入产品类型或使用当前输出倒推预期格式。Schema self-test 与产品运行测试分开登记。

计划文件目标为 `validation/contracts/oracle.py`、`semantic_checks.py`、`fixtures/`、`cases/` 和对应报告清单；现 `scenarios.py`/`fixtures.py`/`browser`迁移为消费者。oracle读取发布的本地 Schema registry，拒绝网络 `$ref`、重复 JSON key、非 JSON 数值和超限输入，显式开启 format 断言；所用 Draft 2020-12 验证器及支持范围在实现前登记，不能把自制部分校验器称为完整标准验证。

**验收 oracle：** C01–C18 全部执行，并验证“合法对象能通过、每个契约破坏能失败”。枚举、required、pattern、additionalProperties 以及跨对象 hash/sequence 必须独立判断；required_checks 要求固定集合、完整字段及同源，canonical RequiredCheck 仅以 status 判定且拒绝注入 passed，legacy 两字段矛盾也不能用 OR 判通过；manifest.files 只接受 ArtifactEntry 数组及 sha256，不接受结构别名。

**兼容及回归：** 先跑无副作用契约负例与迁移读取，再 local 主线、fixture 协议、浏览器、真实 Temporal CAN/并发/崩溃恢复；实际耗时场景统一协调一次。旧 20/20 和 15/15 保持历史标签，新报告不得自动继承旧 passed。live 模型、GitLab/Runner 单独安排并保留身份，缺配置不补造成功。

**退出条件：** 目标范围的 P0 差异闭环、全部必须结构/语义负例通过、legacy 读/恢复链无破坏、原用户链路回归有新证据。仍未实现的 P1 逐项保留范围、原因和下一阶段；本计划不承诺本轮新实现全部 45 份功能或生产运维。

## 具体验收用例与证据映射

下表是后续执行规格，状态均为 `planned`，本轮没有执行这些目标契约用例。完整对象字段与当前缺口在矩阵中登记。

| 用例 | 功能 ID | 公开契约 / 接口与模块边界 | 正例及必须负例 | 阶段 / 证据 |
| --- | --- | --- | --- | --- |
| C01 | feature.agent.definition-versioning | DefinitionSnapshot；定义注册与 v2 RunSnapshot | 完整快照；缺 SystemPrompt/依赖/hash、额外字段拒绝 | S0/S1；schema-negative 与 definition-binding 报告 |
| C02 | feature.agent.definition-versioning | 定义发布/Run 创建 | vA/vB 并存；同版本异内容拒绝 | S1；版本冲突响应和内容 digest |
| C03 | feature.agent.definition-versioning | 冻结 SystemPrompt → ContextBuilder | 修改最新提示后旧 Run 请求仍含原提示；旧定义不可被当前代码覆盖 | S1；独立 mock 请求文本 digest 与快照对应 |
| C04 | feature.skills.skill-contract | SkillDefinition/参数校验 | 三 Skill 合法配置；错类型、缺 required、未知字段、范围越界拒绝 | S0/S1；逐 Skill Schema 正负样本 |
| C05 | feature.skills.template-versioning | Definition/Skill/template 依赖 | 原模板复用；依赖内容改 hash、缺版本或依赖撤销拒绝 | S1；冻结与依赖校验报告 |
| C06 | feature.conversation.session-run-management | legacy → v2 只读边界 / ErrorResponse | 完整可信来源可投影；未知历史 prompt 返回409和legacy_read_url，不产partial v2、不自动写fallback或继承批准 | S1/S4；migration 前后摘要与原文件保护记录 |
| C07 | feature.agent.model-decision | DecisionAdvice → DecisionProposal；model/rules 入口 | Advice 正例；Proposal 缺 Run/版本/来源、未知 action、假批准拒绝 | S2；目标 fixture 响应与 oracle 分类 |
| C08 | feature.agent.action-validation | ActionValidation → ExecutionRequest | 合法获准请求；模型直接发送 ExecutionRequest、任意 shell/秘密读取拒绝 | S2；拒绝结果与无新增副作用证据 |
| C09 | feature.governance.idempotency-and-concurrency | 命令/operation/transport 身份 | 同 payload 重放；异 payload 冲突，unknown 写不重复 POST | S2；HTTP/账本公开对账与 fixture 请求次数 |
| C10 | feature.conversation.preview-confirmation | proposal/approval/source/hash/期限 | 具体批准；旧版本、错 hash、过期及扩大范围拒绝 | S2；版本绑定与不应用证据 |
| C11 | feature.conversation.rich-message-protocol | MessageEnvelope/Part；v2 messages/events | 合法 Part；缺版本、未知字段、跨 Run ref、未知执行类型拒绝/安全回退 | S3/S4；message-contract 与 renderer 报告 |
| C12 | feature.conversation.jsonl-message-store | 消息持久回执和读取 | 原文顺序/hash 保留；重复、改内容、截断或坏尾部可检出 | S3；存储恢复及范围读取报告 |
| C13 | feature.agent.context-compression | SummaryRecord；摘要登记/读取 | v1/v2 各自源范围可读；错 raw_jsonl_sha256、同摘要身份/版本异内容、摘要伪造批准拒绝 | S3；summary-history 与 authority 不变证据 |
| C14 | feature.agent.context-compression | 上下文窗口与旧摘要关系 | 摘要失败仍保留旧版/原文；并发新增不混进冻结范围 | S3；失败/并发恢复报告 |
| C15 | feature.frontend.state-stream-sync | RunSnapshot 与 v2 snapshot event | 正常收敛；bool 版本、扁平模式、未知状态、缺口/乱序/跨 Run 拒绝或重取 | S4；独立 schema 与事件语义记录 |
| C16 | feature.frontend.progress-artifact-panels | RequiredCheck/ArtifactManifest/ArtifactEntry/ObjectRef | 完整同源集合；缺项、注入 passed、legacy status/passed 矛盾、size/sha256/来源错误拒绝并正确显示 | S4；API/面板对应 evidence |
| C17 | feature.frontend.conversation-workspace | v2/legacy renderer 适配 | 旧 proposal 卡绑定旧对象；最新方案不替换历史 Part、切 Run 不污染 | S4；浏览器组件/引用负例 |
| C18 | feature.deploy.reproducible-delivery | 从批准到交付的公共链 | 新契约下 local ZIP 真启动；Temporal CAN/并发/VERIFY crash；fixture 显式 mock | S5；新 schema hash 绑定的 path 报告 |

## 阶段推进和变更记录

每阶段首先更新矩阵状态，再提交该对象的实现、独立 oracle 与对应证据；评审核对完整链条后进入下一阶段。失败不得靠放宽 required/pattern、允许未知字段、接受别名或把 unknown 当 passed 消除。新增字段、对象或模式先发布新契约版本及兼容说明，再改实现；冻结不变量变更按治理流程和新基线处理。

本轮退出只表示 docs_ready 文档阶段可审查。实现阶段、契约符合、昂贵恢复和 live 第三方必须分别有后续实际证据。


## 已授权前端与审查并行阶段

用户“开始推进所有项”授权按[ADR-047](10-decisions/ADR-047-vue-readonly-review.md)先发布[审查模型规范](review/README.md)，再等价迁移Vue并交付十一类只读审查及SVG/PNG导出。它与S1–S5分开验收，不将框架迁移、图可见或数据检查通过升级为目标v2结构符合。先完成现有浏览器路径回归，审查只读边界、来源完整性与导出另有证据。
