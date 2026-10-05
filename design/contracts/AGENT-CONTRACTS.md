# 项目创建 Agent 对象与接口契约

2026-10-02 S2交付补充发布为 [contract 1.3.0](releases/1.3.0/README.md)，见 [ADR-050](../10-decisions/ADR-050-controlled-delivery-evidence.md)：固定Git/Go模块策略、完整远程与包回执、VERIFY候选及报告；1.0–1.2原字节和绑定身份保留。

2026-10-02 S2当前发布为 [contract 1.2.0](releases/1.2.0/README.md)，见 [ADR-049](../10-decisions/ADR-049-controlled-execution-ledger.md)。新增受控执行回执、预算/上下文账本和完整S2只读审计；固定旧对象hash规则不变，新增对象规则以1.2发布说明为准。1.0/1.1原字节及历史对象保留，按绑定版本独立校验；不把S2观察接口称为完整v2 Run。

实施更新：S1使用[1.1.0增量发布](releases/1.1.0/README.md)，其Schema与模板内容引用、预算、定义观察接口优先于本文同主题的1.0.0条目；其余对象语义沿用本文。原1.0.0文件不改写。S1不代表完整v2 API已实现。

> 文档标识：contract.agent-contracts（公开格式补充，经相关功能与ADR-046定位，不是认知模型的新实体类型）
> 状态：TARGET；本次有效设计基线 2026-10-02。契约包 `contract_version=1.0.0`，公开 Run/消息 wire `schema_version=2`。

本文把项目创建 Agent 的输入、输出及对象依赖固定为可实施契约。它与 [Agent Schema](schemas/agent.schema.json)、[公共 Schema](schemas/common.schema.json)、[消息 Schema](schemas/message.schema.json) 共同定义目标格式，不表示当前 `/api` 已提供这些对象。当前实现差距见 [差距矩阵](../../docs/analysis/CONTRACT-GAP-MATRIX.md)，实施顺序见 [实施计划](../IMPLEMENTATION-PLAN.md)。原始产品说明保留为来源；有效语义以本项目设计、裁决和本文为准。

## 版本、发布与产品范围

`contract_version` 标识整个契约包；`schema_version` 标识 Run/消息的 wire 代际；`definition_version`、`skill_version`、`template_version` 各标识独立发布的内容。它们互不替代。对象引用的正整数 `object_version` 标识不可变对象实例，不能装入语义版本字符串，也不能直接拿 wire 版本充当内容版本。

Schema 使用 draft 2020-12；顶层 `$id` 为 `https://agentstudio.invalid/contracts/1.0.0/agent.schema.json`，该地址只作稳定标识。运行与校验离线登记本契约包的三个文件，禁止为解决 `$ref` 下载远程资源。除 Schema 自身约束关键词外，发布对象均闭合；未知字段须拒绝，扩展经契约/Schema 发布与显式 adapter 处理。

首版只创建 Go 项目，注册 `add_database`、`add_auth`、`add_logging` 三项技能。数据库默认 PostgreSQL/GORM，MySQL/sqlx/SQLite 为明确参数候选；JWT 为认证默认，session 为候选；日志默认 console/json/info。数据库能力只生成代码与隔离环境校验，不对客户数据库执行真实迁移。交付为 Run 工件包或精确 GitLab 实例/namespace/private 项目/main 分支；不引入生产部署、任意 shell 或通用任务 DAG。

`RunSnapshot.modes` 必须完整包含 `execution_mode`、`decision_mode`、`delivery_mode`。创建时冻结为 temporal/local、model/rules、gitlab/local 三条独立轴；任一不可用均返回明确失败或阻塞，不切换另一轴。旧 Run 只能由相同 execution_mode 接收命令。模型 endpoint、GitLab URL 不含 userinfo/query/fragment；凭据由受控执行环境查找，绝不放入定义、计划、消息或摘要。

## 对象职责与依赖

完整字段、必填性和候选值以 Schema 为准；下表固定每类对象的生产者、消费者和语义边界。

| 对象 | 输入与生产者 | 输出与消费者 | 对象边界 |
| --- | --- | --- | --- |
| `AgentDefinition` | 已发布的支持栈、三 Skill 引用、完整 SystemPrompt、动作协议 Schema、工具/后置命令、仓库/上下文/预算/完成策略 | 定义注册表供 Run 创建解析 | 完整版本化定义，不是名称加版本标签；`system_prompt.content` 必须保留原文及内容哈希 |
| `SkillDefinition` | 注册者发布参数 JSON Schema、默认参数、依赖/冲突、读写路径、权限、工具、模板及必需检查 | 计划规范化与技能执行 | 参数 Schema 是实际可校验的 JSON Schema；首版限于声明的 string/integer 对象词汇，不接受随意扩展的动态代码 |
| `DefinitionSnapshot` | 精确 AgentDefinition、三个完整 SkillDefinition、模板内容引用、Schema 文件哈希、工具及非敏感 provider 绑定 | 模型上下文、校验器、执行器、恢复逻辑 | 创建 Run 时一次绑定完整闭包；定义 hash 与全闭包 snapshot hash 是两种不同身份 |
| `ProjectPlan` | 用户目标经显式 model/rules 建议、确定性参数规范化 | 文件预览和提案签发 | 保存规范化技能选择及各自版本/hash、模板和精确交付目标；无批准或执行权 |
| `DecisionProposal` | 权威输入消息、分层上下文、已绑定定义；服务端封装原始 `DecisionAdvice` | `ActionValidation` | 服务端签发 run/source/state/definition/context 绑定、usage 与 hash；模型不能自填授权或权威来源版本 |
| `ActionValidation` | 建议及当前状态、权限、预算、参数 Schema、工作区与批准对象 | 规范化建议，或 `ExecutionRequest` | 确定性规则裁决；结构正确不等于获准执行。拒绝/待澄清不得含执行请求 |
| `ExecutionRequest` | 已允许的动作校验及有效批准 | 受控 Activity/local 执行器 | 固定 operation/command/run 身份、workspace、参数、批准链及 required permissions；不接收自然语言 shell |
| `SummaryRecord` | 不可变原始消息范围、绑定摘要生成策略 | 后续上下文构建 | 追加派生记录，保留来源序列/ID/JSONL 原字节 hash；摘要不能成为授权、阶段或完成的权威 |
| `RunSnapshot` | 单写者归并消息、提案、批准、执行、检查及审计结果 | GET、SSE 客户端与恢复 | 完整业务快照，包含各对象及独立 source/state 计数；Workflow execution_run_id 仅是运行身份 |

定义的输入/输出边界对应 [定义版本输入](../02-agent/features/definition-versioning.md#输入)、[定义快照输出](../02-agent/features/definition-versioning.md#输出)；技能对应 [Skill 输入](../03-skills/features/skill-contract.md#输入)、[Skill 输出](../03-skills/features/skill-contract.md#输出)。建议和规则分别对应 [模型建议输出](../02-agent/features/model-decision.md#输出)、[动作校验输入](../02-agent/features/action-validation.md#输入)及[动作校验输出](../02-agent/features/action-validation.md#输出)。消息对象采用 [富消息协议输出](../05-conversation/features/rich-message-protocol.md#输出)，不另发一套缩减文本协议。

## 定义冻结与可重放读取

创建请求选择 `definition_id + definition_version + definition_hash`；注册表同版本不同内容必须拒绝。服务端解析并复核完整依赖闭包后保存 `DefinitionSnapshot`，分别保留模板/Schema 原字节和可检索内容引用。Snapshot 中的 `definition_id/version/hash` 必须与内嵌 definition 一致，available_skills 必须与三个内嵌 Skill 的 ID/version/hash 一一对应。依赖不能缺失、重名、形成未解析循环或存在冲突。

每次模型调用读取该 Run Snapshot 的 `definition.system_prompt.content`、动作 Schema、完成策略、预算和技能，而非当前 Worker 二进制的最新常量。恢复、Continue-As-New、重试 Activity 不重新选择最新定义。模板按冻结模板依赖渲染；工具按绑定 identity 检查实际版本。依赖未找到或 hash 不符时阻塞，不能跳过复核再使用同名资源。

新定义、新提示词、新技能参数/模板均发布新版本和内容 hash，只影响之后创建的 Run。终态重做创建关联的新 Run，由创建请求明确选择定义；不得把旧 Run 原地升级。工具运行、CI 配置和模板修改都属于有证据的内容变更，不可借“只改实现细节”规避定义闭包更新。

## 建议、校验与执行顺序

1. 接收稳定业务 command_id，校验目标历史引用、当前 source、阶段与预算；输入含秘密时拒绝且不写入公共消息/历史。持久命令指纹后再调用模型或进行外部传输。
2. 按冻结定义构建上下文，model 模式只调用已绑定 provider，rules 模式明确执行规则解析。原始输出必须符合 `DecisionAdvice`：clarification、plan、action、diagnosis、completion 五种建议之一，对应 payload 唯一，其他 payload 为 null。
3. 服务端封装 `DecisionProposal`，复核/重算候选计划 hash 和本地已知依赖绑定，不信任 provider 自报的 hash、使用量或完成提示。provider 未报告 token 时标 unknown；失败调用也消耗 model_attempts。
4. 确定性校验器依次检查 Schema/动作白名单、定义绑定、参数与冲突、允许阶段、权限、预算及工作区。计划类建议可产生未应用预览；只有 action 类且全部检查允许，才产生执行请求。
5. 首次执行前重新核对 source、提案不可变版本/hash、完整定义闭包、权限、批准有效期、精确交付目标与预算。Executor 只执行七种类型：scaffold、apply_skill、verify、create_repository、push_repository、trigger_ci、package。
6. 执行器将工具输出、文件哈希、操作账本与外部核验证据写入权威快照。必需检查聚合生成 `VerificationReport`；确定性完成门决定 DONE，模型 completion_hint 或外部 validation 场景调用器不能决定 DONE。

`ActionValidation.decision_kind` 必须等于来源建议的 kind，normalized_advice 与 execution_request.action/参数完全一致；execution_request.validation_id/hash 指向同一冻结校验主体。后置命令 command_id 与固定 arguments 必须一一匹配，禁止只因 executable=go 就开放任意 Go 参数。数据库/auth/logging 参数还要接受 duration、组合、路径、权限等语义校验；JSON Schema 只负责结构。

执行与完成依赖 [Activity 调度输出](../01-orchestrator/features/activity-scheduling.md#输出)、[预算限制输入](../06-governance/features/budget-limits.md#输入)、[权限输出](../06-governance/features/permissions-and-secrets.md#输出)和[验证证据输出](../07-observability/features/verification-evidence.md#输出)。GitLab 必需检查使用 [CI 核验输出](../04-integrations/features/ci-verification.md#输出)，必须同时绑定 project、ref、commit SHA、必需 job、报告和构建产物，不能以 pipeline success 独立代替。

## 提案、批准、状态与有效期

`source_version` 只在方案/授权范围变化时递增，例如用户改需求、规范化计划/模板内容改变、交付目标改变或权限撤销；`state_sequence` 在每次权威快照推进时递增，包括普通阶段和检查进度。普通进度不改变 source，也不使已批准范围过期。新 Proposal 的 `proposal_version` 不可变；同一 proposal_id/version 不得替换计划、文件、权限或目标。

`Proposal` 保存 plan、文件 changes、影响、检查、allowed_actions、required_permission_ids、permission_hash，以及服务端签发 issued_at/expires_at。签发 TTL 默认 30min，可配置 1min–24h；客户端不能延期。`Approval` 单向绑定既有 proposal_id/version/source/hash、definition snapshot、plan、permissions、允许动作与精确 delivery_target，接受时间和 actor 由服务端身份产生。status 只是生命周期显示，不能替代批准链。

accept/reject/revise 都提交 `required_proposal_ref`。服务器核对引用存在及 hash、当前 source 和当前待确认提案，随后才改变状态。拒绝未应用提案回 CLARIFY；VERIFY 中的修复若改变批准文件/依赖元数据或计划，必须生成新提案回 PLAN 重批准。暂停在安全边界生效；恢复不得撤销已有核实副作用再重做。

批准过期在首次实际执行前复核。`Approval.started_at` 由首次复核成功后服务端记录；长执行链按已批准动作范围推进，不能仅因普通阶段进度或最初 TTL 到期取消已核实副作用。每个动作仍检查当前撤销/权限、输入 source、冻结参数、预算及操作账本；超出批准范围需要新 source/提案。未知外部写入结果进入 BLOCKED，按原 operation_id 只读核实，在没有未执行的确定证据前不得重新发送写请求。

上述绑定依赖 [确认输入](../01-orchestrator/features/human-gates.md#输入)、[确认输出](../01-orchestrator/features/human-gates.md#输出)、[状态机输出](../01-orchestrator/features/state-machine.md#输出)及[未知结果输出](../04-integrations/features/unknown-results.md#输出)。DONE/FAILED/CANCELLED 不原地重开，retry 只创建关联新 Run。

## 固定哈希输入与无循环关系

算法、编码、重复键/数字限制以 [HASHING](HASHING.md) 为准。下表的“排除字段”是规范固定清单，不能由实现者按方便增删。JCS hash 覆盖对象其余所有字段；只移除表内该对象顶层字段，保留嵌套子对象 hash 并单独验证。null 与缺字段不得互换。

| 对象 / hash 字段 | 输入与排除清单 |
| --- | --- |
| AgentDefinition.definition_hash | 完整定义 JCS，排除 `definition_hash` |
| SkillDefinition.skill_hash | 完整技能 JCS，排除 `skill_hash`；实际参数 Schema 参与 |
| DefinitionSnapshot.snapshot_hash | 完整解析闭包 JCS，排除 `snapshot_hash`；保留内嵌 definition_hash、skill_hash、Schema/模板/tool/provider 绑定 |
| ProjectPlan.plan_hash | 规范化完整计划 JCS，排除 `plan_hash` |
| DecisionProposal.decision_hash | 服务端封装的完整建议 JCS，排除 `decision_hash`；含原上下文与来源引用 |
| ActionValidation.validation_hash | 冻结校验主体 JCS，排除 `validation_hash`、`execution_request`；先冻结输入、findings、verdict、normalized_advice，再产生执行请求 |
| ExecutionRequest.request_hash | 完整受控请求 JCS，排除 `request_hash`；保留单向 validation_id/hash 及批准绑定 |
| Proposal.snapshot_hash | 完整未应用提案 JCS，排除 `snapshot_hash`、`status`；含确认要求/允许动作/权限/有效期，不包含之后签发的 Approval 实体 |
| Approval.approval_hash | 固定授权内容 JCS，排除 `approval_hash`、`status`、`started_at`、`revoked_at`；后两项为服务端执行/撤销生命周期，不扩展授权范围 |
| PermissionSnapshot.permission_hash | 完整权限对象 JCS，排除 `permission_hash`；撤销或授权目标改变产生新权限身份和 source，不是普通进度 |
| ArtifactManifest.hash | 完整 manifest JCS，排除 `hash`；files 不包含 manifest 自身，避免自引用 |
| VerificationReport.report_hash | 完整报告 JCS，排除 `report_hash`；检查集合与来源绑定均参与 |
| SummaryRecord.summary_hash | 完整摘要记录 JCS，排除 `summary_hash`；保留来源和内容原字节 hash |
| RunSnapshot.snapshot_hash | 完整历史快照 JCS，排除 `snapshot_hash`；其余嵌套 hash 保留，不删除消息、usage 或生命周期状态 |
| TemplateDependency.manifest_hash | 完整模板依赖 JCS，排除 `manifest_hash`；模板各文件原字节 SHA256 单独复核 |
| MessageEnvelope 引用 hash | 完整不可变 Envelope JCS，没有需排除的自身 hash 字段；不是 JSONL 行字节 hash |

SystemPrompt.sha256 和 SummaryRecord.content_sha256 对相应 UTF-8 内容原字节计算。SchemaDependency.sha256、SchemaReference.file_sha256、TemplateFile.sha256、ArtifactEntry.sha256、FileChange.before/after_sha256 对对应文件原字节计算。SummarySource.raw_jsonl_sha256 对指定连续原始 JSONL 行的精确字节串计算，包含每行实际行结束符；禁止解析再序列化后冒充原字节证据。

集合先规范化再冻结：技能引用/定义/选择按 skill_id；Schema 依赖按 schema_uri；模板依赖按 template_id 后 template_version；模板文件、工件 files 与提案 changes 按 path；工具按 tool_id 后 platform；权限和 check_id 字符串集合按字符串排序。ObjectRef 集合按 object_type/object_id/object_version/source_version/hash 排序。required_checks 的定义集合按 check_id；报告 checks 同序。消息、摘要来源 message_ids、input_message_refs、history、findings、执行步骤和 post_commands 保留业务发生/执行顺序，不按字典重排；重复身份即拒绝，不能靠集合去重掩盖冲突。

DecisionProposal.input_context_hash 对模型/规则实际收到的有序上下文消息数组 JCS 计算，每项固定 `{role,content}`；role 取 system/user/assistant/tool，content 为精确字符串，外层不加入 provider 凭据或不参与输入的日志字段。Permission/target/request hash 的用途不同；OperationRecord.target_hash 对该动作规范化 target JCS 计算，request_hash 复用对应 ExecutionRequest.request_hash。

无循环顺序为：定义/技能及依赖 → 定义 Snapshot → 计划 → 提案 → 批准 → 校验主体 → 执行请求 → 工件/报告 → Run 快照。Proposal 的固定确认要求不是未来 Approval；ActionValidation 中展示的 execution_request 不进入自身主体 hash，但必须另验 request_hash 和单向 validation_hash，并语义核对 normalized action。Run 内按钮只引用已经保存的历史 Run 快照，不能反过来引用包含该按钮的当前快照。

## 对象引用与读取索引

公共 `ObjectRef` 的闭合五字段是 object_type、object_id、object_version、source_version、hash；存储索引键为 `(run_id, object_type, object_id, object_version, source_version, hash)`。不同类型按下表解析，不允许实现者把任意字段凑成引用。

| object_type | object_id / object_version / hash |
| --- | --- |
| run | run_id / 历史 RunSnapshot.state_sequence / 该历史 snapshot_hash |
| definition | definition_id / DefinitionSnapshot.snapshot_version / snapshot_hash；引用限定当前 Run 已绑定快照 |
| skill | skill_id / Run 绑定快照的 snapshot_version / skill_hash；Skill.semver 仍由完整定义独立核对 |
| proposal | proposal_id / proposal_version / snapshot_hash |
| approval、decision、validation、execution | 对应 *_id / 1（每次新实例用新 ID）/ approval_hash、decision_hash、validation_hash、request_hash |
| plan | `plan_` + 完整 64 位 plan_hash / source_version / plan_hash；ID 长 69 字符，不拼长 run_id、不截断摘要 |
| manifest | manifest_id / manifest_version / hash |
| verification_report | report_id / report_version / report_hash |
| summary | summary_id / summary_version / summary_hash |
| message | message_id / 1（不可变消息）/ 完整 Envelope 的 JCS hash |
| artifact | artifact_id / 来源 manifest_version / 文件原字节 sha256；source 与来源 manifest 相同 |
| template、schema | 发布对象 ID / Run 绑定 snapshot_version / 模板 manifest_hash 或 Schema 文件原字节 sha256 |
| evidence、workspace | 不可变实例 ID / 1 / 已登记回执主体 JCS hash；登记主体格式须在使用前发布，不能任意包装临时对象 |

目标 Run 引用首先核对该历史快照存在，再独立检查当前 source、阶段及权限；不能要求其 hash 等于最新普通进度快照的 hash。source 相同且历史引用真实存在才有可能接受命令，仍不等于批准有效。修改显示 status 或普通进度会产生新的 Run state_sequence，但不会改变已冻结 Proposal 的批准范围 hash。

Proposal 与 Approval 的 hash 标识上述固定排除清单定义的不可变方案/授权范围。status、started_at、revoked_at 等被明确排除的生命周期字段，其当前值由最新 RunSnapshot 投影提供。历史对象读取返回首次登记时保存的对象，不用最新状态改写归档；归档中的 pending、active 等状态只表示当时事实，不能作为当前执行许可。执行仍须重新检查当前 Run 的权限、source、阶段和批准有效性。

## 摘要与完成证据

上下文按已绑定系统定义、权威状态/授权/预算、当前指令、近期原文、历史摘要分层。先为输出和协议保留预算，再计算实际输入，不能用显示估计替代上限。当前指令与授权链不得被摘要替代；必要层本身超预算时阻塞并要求缩小输入，不能裁掉权限或静默调低模型能力。见 [上下文输入](../02-agent/features/context-building.md#输入)及[上下文输出](../02-agent/features/context-building.md#输出)。

`SummaryRecord` 用连续消息范围的 ID/sequence/原字节 hash、generator version/mode/model/prompt hash 和 previous_summary_ref 建立来源链。model 模式摘要失败时保留原文并记录失败，不切 rules；rules 模式生成显式规则摘要，不声称模型理解。摘要不能修改原 JSONL，不能推断“曾批准”而生成 Approval，也不能覆盖权威 phase。来源记录丢失或 hash 不符即不可用。见 [摘要输入](../02-agent/features/context-compression.md#输入)及[摘要输出](../02-agent/features/context-compression.md#输出)。

`VerificationReport` 只聚合定义 completion_policy 按 delivery_mode 要求的已有 RequiredCheck，不引入另一套执行系统。报告 passed 必须包含适用的完整 check_id 集合、所有状态 passed，且 run/source/definition/proposal/manifest 绑定一致；缺检查、只有工具启动日志、只有 mock pipeline 成功、或证据过期都不能 passed。外部 validation runner 的场景结果可作为测试证据，但不直接写 Agent RequiredCheck 或绕过内部完成门。Package 下载复核文件 SHA256、manifest hash 与 zip 完整性；manifest 之外的包文件不能当作已批准源文件。

## 目标 v2 HTTP 与命令回执

`/api/v2` 是本契约的目标公开边界；当前 `/api` 保持 legacy，不因发送 `schema_version` 字段就升级。本表规定最小路径和对象，不承诺本轮已实现端点。请求 Content-Type 为 application/json，身份由服务端可信会话/首版受限单用户边界确定，不能接受正文自报 actor_id。

| 方法与路径 | 请求 | 成功响应 / 作用 |
| --- | --- | --- |
| POST /api/v2/runs | CreateRunRequest | 201 CommandReceipt；命令成功后 run_ref 指向保存的初始处理快照，未完成传输可为 202 queued |
| GET /api/v2/runs | 无；分页参数须发布后使用 | RunSnapshot 数组；首版受单写者部署与有界存储限制 |
| GET /api/v2/runs/{run_id} | 无 | 最新 RunSnapshot |
| POST /api/v2/runs/{run_id}/messages | MessageRequest | 200 processed 或 202 queued CommandReceipt；服务端追加 user Envelope |
| POST /api/v2/runs/{run_id}/actions | ControlRequest（直接复用 message.ActionIntent） | 200/202 CommandReceipt；控制输入与可执行 ExecutionRequest 是不同对象 |
| GET /api/v2/runs/{run_id}/commands/{command_id} | 无 | 原业务命令的最新 CommandReceipt；用于 result_unknown 和 queued 的结果核实，不产生新执行 |
| GET /api/v2/runs/{run_id}/objects/{object_type}/{object_id}?object_version=N&source_version=N&hash=H | 完整引用定位参数 | 对应已发布 $defs 的历史对象；仅返回该 Run 可读取范围，不能按 ID 跨 Run 猜取 |
| GET /api/v2/runs/{run_id}/events | 可带 Last-Event-ID | SSE，event: snapshot；data: SnapshotEvent，随后以 run_ref 读取对应历史快照 |
| GET /api/v2/runs/{run_id}/artifacts?path=P | manifest 内相对路径 | 已核实文件内容及 SHA256/manifest 身份响应头；路径不存在或越界拒绝 |
| GET /api/v2/runs/{run_id}/download | 无 | 200 application/zip；响应头声明 X-Manifest-Hash、X-Artifact-SHA256、X-Source-Version |

CreateRunRequest 绑定精确定义选择和三轴模式，initial_content 是原用户输入；客户端不能供应完整服务端 Snapshot、批准或过期时间。MessageRequest.target 必须为历史 run ObjectRef；路径 run_id 与 target.object_id 必须相等。ControlRequest 不另外增加 schema_version/contract_version 字段，其结构严格等于消息 ActionIntent，契约由 v2 路径与发布包确定；accept/reject/revise 必须含 proposal 引用。

command_id 在同一 Run 业务范围稳定；同 ID 同规范请求指纹返回已有回执，同 ID 不同内容返回 409 idempotency_conflict。command_fingerprint 对固定包装 `{method:"POST",path:规范路径,body:完整客户端请求对象}` 的 JCS SHA256 计算；路径为本表对应已校验 ASCII ID 的完整 v2 路径，不含 host/query。执行传输重试 ID 与业务 ID 分开。失败 provider 调用允许同业务内容的新传输 attempt，不重写已成功消息；超时结果未知保存 result_unknown，不冒称业务失败或触发新写副作用。

CommandReceipt.status 的 accepted/queued 仅证明命令已持久接收；processed 才证明本次业务处理完成；rejected 附 Failure，result_unknown 要继续读取原 command_id 的结果。SSE 是通知通道，丢事件以 GET 快照收敛；客户端按 state_sequence 丢弃旧事件，检测跳号读取权威快照，不能从流断开推断 Run 失败。

统一错误响应由闭合 `ErrorResponse` 规定：通常为 `{error:string,code:string}`；仅 code=legacy_contract_incomplete 时必须另带只读 legacy_read_url，其他错误不得带该字段。400 为结构/参数/不支持契约版本，409 为 source/stale/hash/state/idempotency/run_mode 冲突，503 为 model/workflow/provider 不可用，404 为不存在或不可读的 Run/对象，423 为单写者存储锁，413 为请求字节超限。Schema 验证错误不泄漏秘密原文。远程命令 BLOCKED 是可查询 Run 的业务结果，不等同 API 传输超时。

## Legacy 适配与验收门

legacy.id 映射 run_id；平铺模式映射 modes；平铺 Plan 的 skills/config 只能经明确默认规范化生成 SkillSelection。迁移必须保存原始记录及来源 adapter 版本，不能靠补空字段/零 hash 就升级可执行对象。旧记录没有冻结提示词、实际 SkillDefinition/参数 Schema、完整依赖字节或摘要来源时，只能保留 legacy 只读展示或明确创建新 v2 Run；不能从当前 Worker 拼一个定义假称旧 Run 当时就已绑定。

GET v2 无法完整投影的 legacy Run 时返回 HTTP 409、code=legacy_contract_incomplete、legacy_read_url=`/api/runs/{legacy.id}`；客户端只读进入该地址，不自动把 v2 写请求重发到 legacy。旧记录已批准文件及已核实副作用可在 legacy 既有授权范围按原身份恢复；新增范围或未冻结定义的模型调用转入带 retry_of 的新 v2 Run 与新批准。不得返回残缺 RunSnapshot 或使适配后按钮获得新授权。

结构正例是 [AgentDefinition](examples/agent-definition.valid.json)、[SkillDefinition](examples/skill-definition.valid.json)、[DecisionProposal](examples/decision-proposal.valid.json)、[初始 RunSnapshot](examples/run-snapshot-intake.valid.json)和[SnapshotEvent](examples/snapshot-event.valid.json)。初始 Run 包含完整三技能定义闭包，消息/摘要/步骤/产物为空，批准与执行事实为 null，检查尚未运行。它们字段完整，但教学 hash 全零，仅证明 Schema 格式；不具有内容一致、可执行、批准或 DONE 语义。实现前还须发布真实内容 hash 的语义验收 fixtures。

S0 契约验收至少覆盖：离线解析全部 $ref、draft metaschema、自包含正例，以及缺字段/额外字段/非法动作/模式/错误类型的负例。后续实施必须证明旧 Run 经 Worker 更新仍使用原提示词与 Skill/Schema；改变依赖字节即拒绝；普通 state_sequence 推进仍能确认原 source 提案，source/权限改变则拒绝；过期未开始执行不能应用；修改 validation 中请求必须因 request_hash 或主体一致性被拒绝；摘要原文范围变更不可用；缺必需检查不能生成 passed 报告或 DONE。

本轮只发布文档与 JSON Schema。Schema 通过不能代替 hash、业务身份、权限、预算、目录隔离、秘密扫描、未知结果核实和真实工具/CI证据的语义校验，也不能说明 target v2 已有运行实现。


## S4 传输与历史快照实施规则（2026-10-05）

本节落实已有v2路由；完整对象来自1.4.0发布源，不新增精简DTO。每次持久提交以同一state_sequence归档完整RunSnapshot，snapshot_hash和五字段run_ref均可独立复核。GET最新快照与SSE事件均指向已提交对象；读取不得增加source_version或state_sequence。首版GET v2 runs仅列有完整原始来源的Run；旧Run仍由legacy列表发现，精确GET v2旧Run返回409及legacy_read_url，消费者只读。

CreateRunRequest的command_id直接用作原始创建命令身份，指纹为整个实际wire请求JCS的SHA256。MessageRequest/ControlRequest同样冻结完整wire输入，再映射内部Command；持久绑定原wire及其规范化命令，禁止重放时丢弃target、模式或定义字段。路径与target必须同Run，准确历史对象必须已归档；首次接收时target.source_version须为当前来源，进度state_sequence可以已推进。accept/reject/revise的提案引用须为该来源当前准确Proposal。重复已processed命令返回原回执/原run_ref，不以最新快照替换原结果；同ID异输入冲突。失败的显式同身份重试保存原尝试历史；result_unknown只核实原身份。结构或引用验证拒绝不产生业务执行。

retry仍在终止的原Run登记回执，run_ref属于原Run；result_refs可包含服务端创建的关联新Run准确快照，仅在新Run.retry_of等于原Run、创建来源已核实后登记。此处是显式新Run结果，不允许其他跨Run对象进入旧Run索引或假装继承批准。消费者读取关联新Run自身路由。

SSE订阅先建立原子快照/订阅边界再发送当前已提交引用；event_id为v2_、Run ID UTF-8的SHA256前20位、下划线及state_sequence。Last-Event-ID仅接受同Run且不大于当前序列的该格式；错误/未来游标返回409，客户端以GET收敛。重连发送当前引用；相同序列可以忽略，旧序列丢弃，跳号必须GET，期间收到的新事件不能被迟到GET覆盖。传输断开不改变业务状态。事件保存的是对应提交的精确引用，不能在发送时替换为另一版本。

工件与ZIP端点先核验完整当前manifest及准确包回执，再校验实际字节size/SHA256；返回原始内容，声明X-Manifest-Hash、X-Artifact-SHA256和X-Source-Version。不存在、符号链接、路径越界、来源或内容变更均拒绝。完成依旧由产品内部required_checks/VerificationReport判定，下载存在不代表DONE。
