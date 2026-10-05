# 目标契约与当前实现字段差异矩阵

2026-10-05 S5收口：G01–G21在本范围的产品对象、权限/历史语义、消费端及独立oracle缺口已有逐阶段证据，G17/G18严格负例、G20实际模块依赖、G21说明与路由复核已补齐。详见[完整当前登记](IMPLEMENTATION-STATUS.md#s5-独立验收与外部阻塞2026-10-05)及[S5索引](../evidence/s5-exit.json)。这是 `contract_ready/path_verified` 的范围结论；生成项目容器和真实model/GitLab/Runner仍属 `external_blocked`，S5为 `passed_with_exclusions`，不可宣称所有模式组合或全产品/live验收完成。A01–A06/J01–J08已实现，V01保留这些外部排除项。

2026-10-05运行补齐通过，J04–J07在本项目单写者范围内完成：真实恢复/累计预算、审计/指标/活动、依赖就绪及离线恢复见[runtime退出索引](../evidence/runtime-exit.json)。S5仍须合并C01–C18证据；live与生产边界不随本阶段关闭。

2026-10-05增量裁决：G07–G16/G19在S3/S4范围内完成对象及运行工作区验收，见[状态](IMPLEMENTATION-STATUS.md)和[S4退出索引](../evidence/s4-exit.json)。下表保留初始差异基线，用于追踪原要求；不能再将其“当前代码”描述当作本轮最新实现。G17/G18/G20/G21最终整体验收仍由S5逐例收口，运行补齐与live分别登记。

2026-10-05阶段更新：S2已退出（G04/G05受控建议、授权、预算及实际交付），S3已退出（G07–G09的原文/Part、摘要来源和持久历史切片），见[实现状态](IMPLEMENTATION-STATUS.md)、[S2索引](../evidence/s2-resume-exit.json)及[S3索引](../evidence/s3-exit.json)。G09完整v2公开传输及G11–G13/G16/G19消费者仍须S4验收；Job运行收尾、S5及live未关闭。以下S0/S1定位保留为历史基线。

日期：2026-10-02。目标是[五层设计](../../design/README.md)及[阶段计划](../../design/IMPLEMENTATION-PLAN.md)。下方S0字段与行号保留为历史审计定位；后续进度以本节阶段更新和[实现状态](IMPLEMENTATION-STATUS.md)为准。历史外部20/20和浏览器15/15仅为当时选定路径证据。

## S1阶段更新（2026-10-02）

G01/G02/G03/G06的Definition/Skill冻结范围已取得contract_ready与定向path_verified证据：完整发布1.1.0对象和130个派生Go类型；不可变定义/Skill/模板/Schema注册；真实hash、参数规范化、依赖/冲突/撤销、写路径；新Run持久完整快照、读取绑定提示与模板；两版本并存及重启不漂移。产品默认发布3.0.1，原1.0.0 Schema字节保留。

独立oracle1862项自检与四轮真实只读HTTP通过；重启前后两个实际mock请求的提示与冻结来源一致。产品两版本/同版本冲突/未知检查/路径与批准负例、24组模板回归、全套/race及三技能local交付通过。准确代码指纹、命令和证明范围见实现状态S1记录；不能把oracle自检数量解释为产品路径数量。

G15/C06只完成S1兼容切片：缺完整旧来源的定义读取返回409+legacy_read_url，不能新增批准或捏造旧提示；完整v2迁移和消息历史仍在S3/S4。G04/G05及其余S2–S5差异保持未完成。新增累计预算与PostCommands实际执行、工具平台复核、消息/摘要/v2接口/前端、Job审计/readiness和live不因S1关闭。下文原始审计字段不改写成新实现证明。

## 口径和追溯方式

当前结构真值为 `design/contracts/releases/1.1.0/`的三份Schema，见ADR-048；原 `design/contracts/schemas/`保留1.0.0审计源。agent主 `$ref` 为RunSnapshot，message主 `$ref` 为MessageEnvelope；wire `schema_version=2`，当前 `contract_version="1.1.0"`。定义/Skill/模板/提案/来源/状态序列版本分别表达。完整目标入口仍为 `/api/v2`，S1只增加已发布的定义/内容读取接口；legacy id/扁平模式字段不能冒充canonical run_id/modes。

P0 表示会影响冻结能力、权威边界、对象归属、历史可信性或独立结构验收，必须在该阶段退出前闭环。P1 表示展示、读取组织或目录职责的后续完整性，不能因为主线通过就登记已完成。字段名称与 required/pattern/additionalProperties 以发布 Schema 为准；本矩阵中的业务语义也必须有独立校验，不能由字段存在或 hash 长度代替。

路径后的 `:行号` 是 2026-10-02 审计定位，会随实施调整；后续证据记录代码版本与 schema digest。现有实现可以保留行为价值，但没有明确映射的不同结构不能自动声明兼容。

下表的 `types.go/contracts.go/model.go/templates.go/service.go/store.go/http.go/verification.go` 均位于 `backend/internal/agent/`；历史`App.tsx/components.tsx`已等价迁移为`frontend/src/Workspace.vue`与`components/*.vue`；`types.ts`和`lib/api.ts`继续对应legacy公开wire；下表旧行号是S0审计定位，不能当作当前文件存在证明；验证的 `scenarios.py/fixtures.py/http_client.py` 位于 `validation/`，`browser/*.mjs` 位于 `validation/browser/`。

## 已定稿对象字段及 hash 范围

以下是字段对齐索引；完整 required、分支条件、enum/pattern 和 additionalProperties 来自三份 Schema，不能从表格省略部分推导字段可选。逻辑 `AgentDefinition.definition_hash` 与完整 `DefinitionSnapshot.snapshot_hash` 必须分开；后者包含已解析定义、技能、Schema 和模板依赖，下游 `definition_snapshot_hash` 绑定该值。旧 Run 缺依赖包时不能只把 `hash` 换名生成可信 fullSnapshot。

| 对象 | canonical 字段与分支 | legacy 差异 / 关联行 |
| --- | --- | --- |
| RunSnapshot | schema_version、contract_version、run_id、snapshot_hash、title、state、source_version、state_sequence、modes、definition_snapshot、plan/plan_hash、proposal/proposal_history、approval、permission_snapshot、messages、summary_records、steps、required_checks、artifacts、manifest、verification_report、operations、usage、workspace、workflow_checkpoint、history、active_step、pause_requested、cancel_requested、resume_to、retry_of、error、created_at/updated_at | 无协议版本；id/扁平模式；没有 summary_records/plan_hash 或完整公开快照格式。verification_report 可为null，DONE须符合passed报告与独立完成门。G12/G15 |
| Modes | execution_mode、decision_mode、delivery_mode，三项 required，各自枚举 | 不接受 execution/decision/delivery 短名，也不接受扁平模式作为 v2。G12 |
| AgentDefinition | contract_version、definition_id/definition_version/definition_hash、supported_stacks、available_skills、system_prompt={content,sha256}、action_protocol、default_ci、repository_policy、context_policy、budget_policy、completion_policy、required_tools、post_commands | 当前标签/少量hash没有系统提示和完整能力协议。G01/G02/G03/G06 |
| ActionProtocol | model_output_schema、decision_proposal_schema、action_validation_schema、execution_request_schema 各为 {schema_ref,file_sha256}；allowed_actions | 版本标签不是 Schema 内容；不接受 sha256/allowlist 作为字段别名。G01/G04/G05 |
| DefinitionSnapshot | contract_version、definition_id、definition_version、definition_hash、snapshot_version、snapshot_hash、definition、skills、schema_dependencies、template_dependencies、tool_bindings、provider_binding、bound_at | 当前快照缺完整解析依赖包；逻辑 definition_hash 不能代替 snapshot_hash。G01/G02/G03/G06 |
| SkillDefinition | contract_version、skill_id/skill_version/skill_hash、name/description、supported_stacks、parameters_schema、default_parameters、dependencies/conflicts、input_contract/output_contract、template_dependencies、write_paths、required_permissions、required_tools、post_commands、required_check_ids | 当前 id/name/description/parameters 枚举表；默认值和校验散落模板代码。G03 |
| DecisionProposal / DecisionAdvice | Proposal 为 contract_version、decision_id、run_id、source_version、state_sequence、definition_snapshot_hash、decision_mode、advice、model_usage、input_message_refs、input_context_hash、decision_hash、created_at；Advice 闭合 kind/question/plan/action/diagnosis/completion_hint/reason，分支不适用值显式 null；model_usage 的 unknown/not_applicable tokens 为 null | 当前只有 plan/explanation，不能自动充当候选决策对象。模型只产生 DecisionAdvice，可信边界绑定身份和来源后形成 Proposal。G04 |
| ActionValidation | contract_version、validation_id、decision_id、run_id、source_version、state_sequence、definition_snapshot_hash、decision_kind、verdict、findings、normalized_advice、execution_request、validation_hash、validated_at | 当前 Run/Execute bool 和分散保护，无正式中间对象；仅 allowed 且 decision_kind=action 才有请求。G05 |
| ExecutionRequest | contract_version、execution_id、operation_id、command_id、run_id、source_version、state_sequence_at_validation、definition_snapshot_hash、plan_hash、validation_id/validation_hash、approval、workspace、action、required_permission_ids、request_hash、created_at | 当前内部执行参数没有完整批准、校验与作用域绑定。G05 |
| MessageEnvelope | schema_version、contract_version、run_id、message_id、sequence、source_version、state_sequence、role、created_at、modes、parts；reply_to_message_id/command_id 按协议可选 | 当前 id/role/content/created_at/kind/proposal_id。G07/G09 |
| Part 公共字段 | part_id、part_version、type、fallback_text；canonical 五支 text/component/diff/choice/action | 当前 kind 不是 Part 结构，也没有实例版本及fallback字段。G07/G08 |
| text / component | text={text,format}；component={component_id,component_version,component_type,component_schema,payload}，payload 按已知类型闭合，未知 component 使用有界 JSON | 组件结构版本 URI 与实例 component_version 不互相替代；unknownPart 仅兼容边界。G08/G11 |
| diff / choice / action | diff={object_ref,summary,changes[path,operation,before,after]}；choice={prompt,selection_mode,choices[choice_id,label,description?]}；action={label,intent[command_id,action,target,required_proposal_ref条件]} | accept/reject/revise 必须绑具体 proposal ref；渲染按钮不产生授权。G08 |
| ObjectRef | object_type、object_id、object_version、source_version、hash；manifest 引用映射 manifest_id/manifest_version/hash，verification_report 映射 report_id/report_version/report_hash，artifact 版本来自所属 manifest_version 且hash为文件原字节sha256 | 单 proposal_id 不能替代完整引用；引用存在性、Run归属及内容不可变另做语义核验，字段hash不是文件sha256可接受别名。G08/G14 |
| SummaryRecord | contract_version、summary_id、summary_version、run_id、source_version、definition_snapshot_hash、source、generator、previous_summary_ref、content、content_sha256、summary_hash、created_at；source={first_message_id,last_message_id,message_ids,first_sequence,last_sequence,raw_jsonl_sha256}；generator={mode,generator_version,model_id,prompt_sha256} | 当前单 summary/through/count 没有独立记录历史；成功摘要追加，失败单独登记且保留上一版，不伪造成功记录。G10/G11 |
| RequiredCheck / PreflightCheck | RequiredCheck={check_id,run_id,source_version,definition_snapshot_hash,proposal_snapshot_hash,status,detail,evidence_refs,started_at,completed_at}；PreflightCheck={check_id,passed,detail} | 两者是不同契约。canonical 必需检查按 status 判定，passed 是未声明字段；legacy passed/status 不能用 OR 接受矛盾。G14/G18 |
| ArtifactManifest / ArtifactEntry | Manifest={contract_version,manifest_id,manifest_version,run_id,source_version,proposal_id,proposal_version,proposal_snapshot_hash,definition_snapshot_hash,plan_hash,files,hash,created_at}；files[]={artifact_id,path,size,sha256,media_type} | Manifest.hash 与文件 sha256 各按固定范围；不接受文件 hash 别名或 dict/list 两套结构。G14/G18 |
| VerificationReport | contract_version、report_id/report_version、run_id、source_version、definition_snapshot_hash、proposal_snapshot_hash、manifest_ref、checks、outcome、report_hash、created_at | 当前检查与证据未组成该公开版本报告；outcome=passed不代替hash、来源与完成门核验。G14 |
| CreateRunRequest / MessageRequest / ControlRequest | Create={contract_version,schema_version,command_id,run_id,title,modes,definition_selection,initial_content}；Message={contract_version,schema_version,command_id,target,content}；Control严格复用message.ActionIntent，不额外添加协议版本字段 | 当前legacy请求不是v2请求协议；路径/target Run归属、稳定命令及提案引用另验。G12/G15 |
| CommandReceipt / SnapshotEvent / ErrorResponse | Receipt={contract_version,schema_version,command_id,run_id,command_fingerprint,status,run_ref,result_refs,error,accepted_at,updated_at}；Event={contract_version,schema_version,event_id,run_id,source_version,state_sequence,run_ref,occurred_at}；Error={error,code}，仅legacy_contract_incomplete必带legacy_read_url | canonical回执、事件和错误已发布；现/api直接返回Run、snapshot SSE载荷不同，不能因名称相同声明符合。G12/G13/G15 |

字段索引只解释已定对象；具体属性名与 required清单以对应 `$defs` 为准。跨对象 hash、摘要源内容、sequence、批准范围和权限不是 JSON Schema 自动证明的事实，必须由独立语义 oracle核验。

## Definition、Skill 与决策链

| ID / 优先级 | 来源功能与要求 | 当前对象 / 代码定位 | 字段级差异 | 目标契约与阶段 | 必需负例 / 用例 |
| --- | --- | --- | --- | --- | --- |
| G01 / P0 | [definition-versioning](../../design/02-agent/features/definition-versioning.md#输入)：系统规则、动作 Schema、能力及完整依赖冻结 | `types.go:159–170` DefinitionSnapshot；`contracts.go:28–58` definitionSnapshot/digest | 当前有 definition_id、version、hash、template_version/hash、skill_versions、action_schema_version、tools、allowed_actions、budget、decision_provider；没有 SystemPrompt 内容/稳定引用/hash，完整参数与动作 Schema 不在快照；version 也未明确为 definition_version | DefinitionSnapshot 按目标 Schema 完整冻结并使用 definition_version；版本号不能替代依赖内容。S1 | 缺提示/Schema/依赖字段、同版本不同内容、已撤销依赖、hash 不匹配；C01/C02/C05 |
| G02 / P0 | [definition-versioning](../../design/02-agent/features/definition-versioning.md#算法--流程)：旧 Run 不被升级覆盖；[context-building](../../design/02-agent/features/context-building.md#输入) | `model.go:34–49` buildModelRequest 每次读函数内 prompt 常量 | 旧 Run 只冻结 provider endpoint/model；系统提示读取当前代码，而非 Run 绑定的内容。修改源码提示不会改变既有 definition hash，但改变后续请求 | SystemPrompt 纳入 DefinitionSnapshot 的不可变来源和 hash，ContextBuilder 读取绑定版本。S1 | 修改最新 prompt 后旧 Run 请求出现新提示；迁移缺失历史提示却标为 trusted；C03/C06 |
| G03 / P0 | [skill-contract](../../design/03-skills/features/skill-contract.md#输入)：skill_id/version、参数 Schema/默认值、依赖/冲突、模板/路径、post command、权限/预算 | `types.go:227–238` Skill={id,name,description,parameters:map[string][]string}；`templates.go:15–114` 默认值和 ValidatePlan | parameters 是枚举字典，无字段类型、required、范围、默认值、additionalProperties；验证/default 与注册信息分散；skill_version、依赖/冲突、生成路径、固定 command 与验收未组成可冻结 SkillDefinition | SkillDefinition + 独立参数 JSON Schema，版本/依赖内容进入 Definition。S1 | 错类型、缺必需字段、未知参数、越界数值、同 Skill 版本改默认值；C04/C05 |
| G04 / P0 | [model-decision](../../design/02-agent/features/model-decision.md#输出)：DecisionProposal、Run/版本、动作参数、澄清/理由/来源/用量 | `model.go:138–157` 仅解码 `{plan,explanation}`；`service.go:422–462` 得 Plan 后直接写当前方案 | 无显式 DecisionProposal 身份、目标 Run/definition/source 绑定、语义请求类型与 action/arguments 结构；模型 usage 未知没有独立输出状态；Plan 参数与候选动作混合 | DecisionProposal 经目标 Schema 及绑定语义验证，model/rules 来源明确，仍未批准。S2 | 合法 JSON 但错 Run/版本、未知 action、声称批准/CI 成功、缺来源或把未知 usage 填零；C07 |
| G05 / P0 | [action-validation](../../design/02-agent/features/action-validation.md#输入)：候选、显式命令、Schema、权限、批准及预算独立裁决 | `service.go:467–565` action switch；`contracts.go:76–132` 批准检查；`types.go:223–226` CommandResult={Run,Execute bool} | 已有实际确定性保护，但未形成可引用 ActionValidation 结果及受控 ExecutionRequest；Execute bool 不能携带冻结对象、参数/路径/工具范围及验证来源 | DecisionProposal → ActionValidation → ExecutionRequest 三个边界；执行器仅消费获准请求。S2 | 模型直送执行请求、未允许工具、任意 shell、扩大权限、失配批准/source/hash；C08/C10 |
| G06 / P1 | [definition-versioning](../../design/02-agent/features/definition-versioning.md#验收标准)、[template-versioning](../../design/03-skills/features/template-versioning.md#输出) | `contracts.go:29–54` 版本/模板聚合 hash；`service.go:241` 新 Run 绑定 | 已有模板内容 digest 与部分能力版本；未提供明确版本发布、同版本异内容拒绝和依赖 manifest 的完整公共契约 | 定义注册/发布与依赖内容登记，旧 Run 保留原身份。S1 | 更新全局注册后旧 Run 引用漂移；同版本不同模板未冲突；C02/C05 |

## 消息、引用与摘要历史

| ID / 优先级 | 来源功能与要求 | 当前对象 / 代码定位 | 字段级差异 | 目标契约与阶段 | 必需负例 / 用例 |
| --- | --- | --- | --- | --- | --- |
| G07 / P0 | [rich-message-protocol](../../design/05-conversation/features/rich-message-protocol.md#输入)：消息公共版本、Run、顺序、组件身份/版本、源版本与引用 | `types.go:53–60` Message={id,role,content,created_at,kind,proposal_id?}；`store.go:331` addMessage | 无 schema_version/contract_version、run_id、权威顺序、Part 数组、组件版本、source_version/state_sequence、对象引用的完整绑定；proposal_id 不是完整版本引用 | MessageEnvelope/Part 使用目标 message Schema；文本文案不能替代对象结构。S3 | 缺版本/归属、bool 序列、未知字段、Part 类型与 payload 不匹配、跨 Run ref；C11 |
| G08 / P0 | [rich-message-protocol](../../design/05-conversation/features/rich-message-protocol.md#算法--流程)：旧消息关联当时对象；组件版本不可变 | `service.go:462` 文本 message 标 kind=proposal；`S0 App.tsx:92–93 → 当前 Workspace.vue/ProposalCard.vue 等价呈现` 所有消息渲染 content，另显示最新 run.proposal | 旧 proposal_id 留在消息但未通过版本化 ref 解析；只展示一个当前 ProposalCard，历史组件无法表达原 proposal_version/source/hash/动作范围 | proposal_diff/progress/artifact_manifest/verification_report 等 Part 绑定具体对象版本，renderer 按允许类型，legacy 原文可回退。S3/S4 | 最新方案替换旧卡内容、旧 ref 指向当前提案、未知组件执行 HTML/命令、旧卡批准新版本；C11/C17 |
| G09 / P0 | [jsonl-message-store](../../design/05-conversation/features/jsonl-message-store.md#输入)与[输出]：message/command、内容 hash、权威顺序、范围及持久回执 | `store.go:221–280` 原 Message 追加 JSONL；`types.go:53–60`；`http.go:40–46` 返回全 Run | 已有 append/fsync/身份去重，但未按目标Envelope登记权威顺序与范围；当前公开返回Run不能替代目标CommandReceipt及原消息源范围核验 | Envelope表达身份/顺序，SummarySource登记连续message_ids/sequence与raw_jsonl_sha256；CommandReceipt表达命令处理。闭合Envelope不增加hash字段；首版不新增StorageRef或分页路由。S3/S4 | 同 ID 改内容、错来源 hash、跨 Run 范围、截断尾部、部分提交；C12 |
| G10 / P0 | [context-compression](../../design/02-agent/features/context-compression.md#输出)：SummaryRecord、source_range/hash、生成模式、摘要版本、校验结果 | `types.go:79–87` ContextState Summary string/Through/Count；`service.go:734–740` 覆盖当前字符串并加计数 | summary_count 不是独立摘要版本；没有每版 SummaryRecord 身份、源 hash、完整范围和失败来源；历史摘要被当前字符串替代 | 派生 SummaryRecord 追加登记、summary_version/previous_summary_ref 可读、source.raw_jsonl_sha256 可复核；失败另记且保留旧版。S3 | 原文改后旧摘要继续可信、同 summary_id/version 异内容、失败删旧摘要、摘要伪造批准；C13/C14 |
| G11 / P1 | [conversation-workspace](../../design/08-frontend/features/conversation-workspace.md#边界与异常)：摘要派生性质可见、原文可查；[rich-message-protocol](../../design/05-conversation/features/rich-message-protocol.md#验收标准) | `S0 App.tsx:92 → 当前 MarkdownMessage.vue 等价呈现` 文本渲染；`types.ts:16–25` 无 SummaryRecord/Part | 未区分结构化 Part 与普通 Markdown；没有派生摘要历史和来源入口、未知组件回退格式 | v2 renderer 消费 Part 与 SummaryRecord 引用；派生性质和原文入口不混作权威授权。S4 | 摘要显示成原话、旧引用静默更新、未知 Part 无原文回退；C11/C13/C17 |

## 公共快照、证据结构与兼容边界

| ID / 优先级 | 来源功能与要求 | 当前对象 / 代码定位 | 字段级差异 | 目标契约与阶段 | 必需负例 / 用例 |
| --- | --- | --- | --- | --- | --- |
| G12 / P0 | [state-stream-sync](../../design/08-frontend/features/state-stream-sync.md#输入)与[输出]；[configuration-modes](../../design/09-deploy/features/configuration-modes.md#输出) | `types.go:89–131` Run 的 id/扁平 execution_mode/decision_mode/delivery_mode；`http.go:26–46` 直接输出 Run | 无显式 schema_version/contract_version；legacy id 与版本镜像 version；三 modes 扁平；内部聚合类型直接用作公开快照，缺独立 DTO 和严格 Schema | RunSnapshot 主入口，canonical run_id、modes 三项嵌套、独立 source_version/state_sequence，公共对象全引用已发布 schema。S4 | v2 收到 id、扁平 modes、bool 版本、未知状态/额外字段或缺必要对象；C15 |
| G13 / P0 | [state-stream-sync](../../design/08-frontend/features/state-stream-sync.md#算法--流程)：游标、去重、缺口与重连；[durable-messaging](../../design/01-orchestrator/features/durable-messaging.md#输出) | `http.go:115` snapshot SSE；`S0 App.tsx:48–52 → 当前 Workspace.vue SSE投影` 解析 JSON 后 receive，断流轮询；`validation/http_client.py:109–127` 只读第一条 snapshot | 已有 state_sequence 防倒退，但现载荷不是目标SnapshotEvent，未独立验证完整run_ref/游标恢复；现测试不注入丢失/重复/跨Run事件，不核对快照到订阅边界 | SnapshotEvent以完整run_ref定位历史RunSnapshot；event_id、state_sequence与业务source_version分开核验；缺口重取，兼容能力明确。S4 | malformed payload、失效游标、漏事件、重连重复、跨 Run、高/低序号冲突；C15 |
| G14 / P0 | [verification-evidence](../../design/07-observability/features/verification-evidence.md#输出)、[AGENT-GUIDE](../AGENT-GUIDE.md#产物manifest与内部检查) | `types.go:25–30` Check；`types.go:203–213` ArtifactManifest；`verification.go` 已做真实内部核验 | 部分对象与行为有价值；目标仍需固定完整公共格式、canonical Run/ref、版本和独立 hash/sequence 语义；结构报告不能只复用产品 Passed 自证 | RequiredCheck/ArtifactManifest/ArtifactEntry 作为 v2 对象；固定检查集合及同源证据，canonical 仅使用 status，不能混用 PreflightCheck.passed。files[] 的 artifact_id/path/size/sha256/media_type 缺一不可。S4 | 缺 required check、额外/重复项、注入 passed=true/status=failed、错 size、文件 hash 别名、错 proposal/source/definition；C16 |
| G15 / P0 | [definition-versioning](../../design/02-agent/features/definition-versioning.md#边界与异常)、[session-run-management](../../design/05-conversation/features/session-run-management.md#输出) | 历史快照/JSONL 只保存现有结构；`store.go:72–76` 兼容补 SourceVersion/StateSequence | 历史prompt、Part版本和历次摘要未记录；不能靠当前代码或计数补造可信历史。新增结构可能破坏原批准交付/unknown恢复账本 | 仅完整可信来源投影v2，否则409 legacy_contract_incomplete+legacy_read_url供前端legacy只读；不产partial v2、不自动写fallback。保留原字节与原范围恢复，新增范围新建/复批。S1/S3/S4 | 旧批准自动授权新schema、当前prompt冒充历史、partial v2通过、写请求自动fallback、unknown写重复提交；C06/C09/C17 |
| G16 / P1 | [conversation-workspace](../../design/08-frontend/features/conversation-workspace.md#输出)、[progress-artifact-panels](../../design/08-frontend/features/progress-artifact-panels.md#算法--流程) | `types.ts:1–30` 前端类型缺 manifest/approval/完整Check；`lib/api.ts:2` data as T；`S0 components.tsx:57–65 → 当前 Inspector.vue` steps/文件/模式 | 编译类型不是运行校验；完整必需检查、文件摘要/批准对象绑定、组件版本及来源缺少严格消费与展示 | v2 严格入口校验、类型与 Schema 对照；显示组件读取 public contracts，legacy adapter 集中处理。S4 | 缺对象但 UI 显示成功、另一 SHA CI、旧 Part 当最新、artifact/hash/source不对应；C16/C17 |

## 验证 oracle 与目录职责

| ID / 优先级 | 来源与要求 | 当前定位 | 缺口及目标 | 阶段 / 必须用例 |
| --- | --- | --- | --- | --- |
| G17 / P0 | [ADR-045](../../design/10-decisions/ADR-045-agent-model-validation-separation.md#决策)：公共契约和外部验证独立；[model-decision](../../design/02-agent/features/model-decision.md#验收标准) | `validation/scenarios.py:78–89` 手工字段检查、definition 仅非空；`fixtures.py:97–104` 当前 Plan mock；`frontend/src/lib/api.ts:2` 类型断言 | 外部进程隔离已做到，但没有独立结构真值。后续 oracle 必须读取三 schemas 和语义规则，不导入内部 Service/types/模板，不用当前响应倒推期望。模型 fixture 按 ActionProtocol.model_output_schema 对应 DecisionAdvice 构造，服务绑定后的 Proposal 另验 | S0/S5；C01/C04/C07/C11/C15，逐 required/类型/enum/pattern/additionalProperties 破坏必失败 |
| G18 / P0 | [AGENT-GUIDE](../AGENT-GUIDE.md#产物manifest与内部检查)：完整 Check/manifest | `scenarios.py:210–244` 接受 passed/status OR、manifest dict/list、hash/sha256 两种字段；minimum subset | 宽松兼容误当 v2 oracle。v2 RequiredCheck 仅接受 status 和完整字段，passed 是额外字段；legacy 两字段冲突明确拒绝。独立复核固定集合、跨对象 hash、范围和 sequence。旧格式只经 legacy adapter | S5；C16，passed=true/status=failed、缺 detail/size、别名、额外 check 不能通过 |
| G19 / P1 | [rich-message-protocol](../../design/05-conversation/features/rich-message-protocol.md#验收标准)、[state-stream-sync](../../design/08-frontend/features/state-stream-sync.md#验收标准) | `browser/smoke.mjs:29–64` 页面/文本/DONE/ZIP大小；`browser/races.mjs:30–48` 迟到请求/时钟 | 15断言不是对象格式或多 renderer 兼容性。新增目标 Part、来源/旧版本、unknown类型、完整检查及事件负例；保留现主线作为回归 | S4/S5；C11/C15/C16/C17，结构负例和 path 报告分别登记 |
| G20 / P1 | [模块映射](../../design/contracts/MODULE-MAP.md)、各功能公开依赖；[ADR-045](../../design/10-decisions/ADR-045-agent-model-validation-separation.md#决策) | `backend/internal/agent` 聚合 types/service/model/contracts/store/templates/http；前端 Workspace.vue/components/*.vue；validation 独立 | 当前目录名不提供模块契约。按模块映射拆分类型/Schema、定义注册、建议、动作校验、执行、消息存储、公共传输和 renderer；目录调整不能反向扩大公共 API | S1–S4；逐模块只通过公开契约调用、类型/导入边界审查；C01/C08/C11/C15 |
| G21 / P1 | [验证入口](../../validation/README.md)与公开 fixture 实现需一致 | 原 README 错拼 `/fixture/healthz`，实际 `fixtures.py:83` 和调用器使用 `/fixture/health`；S0 文档已修为后者 | 状态为 docs_fixed，尚未 contract_verified。错误说明硬编码测试没有核对文档样例；后续样例从已发布公共契约核对 | S0文档已修；S5补文档样例/接口路由一致性负例，不能由 path pass 推导样例正确 |

## 功能 → 契约 → 接口/模块 → 测试 → 证据

| 功能 ID | 公开契约 | 接口 / 目标职责 | 计划测试 | 所需证据及当前状态 |
| --- | --- | --- | --- | --- |
| feature.agent.definition-versioning | DefinitionSnapshot/SystemPrompt/依赖 | v2 Run 创建/查询；定义注册、ContextBuilder | C01–C03/C05/C06 | 冻结内容/hash、两版本和升级后旧请求；planned，现 definition 非空不覆盖 |
| feature.skills.skill-contract | SkillDefinition/参数 Schema | v2 能力定义；Skill registry/validator | C04/C05 | 三技能逐字段正负样本、依赖版本；planned |
| feature.agent.model-decision | DecisionAdvice/DecisionProposal | model/rules 纯建议接口与服务绑定边界 | C07 | schema合法/非法候选、Run/版本/来源；planned，当前 Plan mock 不是该协议 |
| feature.agent.action-validation | ActionValidation/ExecutionRequest | 确定性放行、调度/执行边界 | C08–C10 | 拒绝与无新增副作用、准许请求及范围；planned，旧批准测试保留 path 价值 |
| feature.conversation.rich-message-protocol | MessageEnvelope/Part/对象 ref | v2 messages/events；Part renderer | C11/C17 | 持久/跨版本引用、多 renderer/未知类型；planned |
| feature.conversation.jsonl-message-store | MessageEnvelope/SummarySource/CommandReceipt | 消息追加/来源范围核验/恢复 | C12 | 原始身份/hash、顺序、截断恢复与回执；planned |
| feature.agent.context-compression | SummaryRecord | 派生摘要登记/历史查询、上下文选取 | C13/C14 | 摘要版本、source range/hash、失败与幂等；planned，Count>0不覆盖 |
| feature.frontend.state-stream-sync | RunSnapshot/事件/游标 | `/api/v2` snapshots/events；同步投影 | C15 | strict schema、缺口/乱序/跨Run/重连；planned |
| feature.frontend.progress-artifact-panels | RequiredCheck/ArtifactManifest/ArtifactEntry/ObjectRef | v2 artifacts/verification；面板 | C16 | 完整同源集合、sha256/size绑定与缺项正确展示；planned |
| feature.frontend.conversation-workspace | v2/legacy消息投影及回执 | v2客户端与legacy adapter | C17 | 旧对象不被新方案替换、明确兼容/未可信来源；planned |
| feature.deploy.reproducible-delivery | 批准到实际交付的同源链 | v2批准/下载；执行/验证/交付 | C18 | 新契约下local/Temporal主线与恢复，mock/live另标；planned |

验收用例的具体正/负例、依赖与阶段退出见[实施计划](../../design/IMPLEMENTATION-PLAN.md#具体验收用例与证据映射)。实施时将本矩阵每行改为 pending/implemented/contract_verified/path_verified 等明确状态并附证据；不能只删除差异说明或用“45功能未完成”代替逐项关闭。

## 迁移与验收边界

legacy读取、当前批准交付恢复和外部操作对账保持可用；保留原文件和版本标签。仅完整可信来源可投影v2；否则GET v2返回409、legacy_contract_incomplete和legacy_read_url，前端只读进入legacy，不产partial v2、不自动写fallback。未记录的历史提示、组件版本和摘要来源不补造成可信数据。v2新写入/批准和新增范围必须从完整目标对象建立。

本轮 docs_ready 仅表示上述目标、差异和路线可审查。contract_ready 要有独立 schema/语义负例和实际代码证据；20/20、15/15 保留为现路径事实；live 模型、GitLab/Runner、生产环境必须另有本轮真实身份和结果，任何结构检查或 fixture 都不能代替。


## Vue与静态审查阶段的关闭边界

ADR-047的框架迁移及十一静态视图已落地，新迁移浏览器15/15通过；审查/导出的最终QA随本轮最终QA归档。审查发布物与来源一致、完整字段绑定和12项负例通过，不能关闭G01–G20目标v2差异或将C01–C18从planned升级。G21的docs_fixed继续保留文档修正标签。新前端api/types保持legacy，v2目标仍依S1–S5分别形成结构、语义和路径证据。
