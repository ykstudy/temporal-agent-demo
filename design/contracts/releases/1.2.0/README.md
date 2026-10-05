# 契约 1.2.0 受控执行与预算账本发布

2026-10-02，按 [ADR-049](../../../10-decisions/ADR-049-controlled-execution-ledger.md) 发布。wire 仍为 schema_version=2。本目录三份 Schema 是 contract_version=1.2.0 的唯一发布源；1.0.0、1.1.0 原字节、对象身份和摘要保留。新版本增加 S2 的执行证据和预算结构，不声称完整 v2 Run、摘要、前端或 live 验收已经完成。JSON 资源限制、精确整数判断和模板原字节引用继承 [1.1.0](../1.1.0/README.md)，不放宽。

## 版本与兼容

先按对象明确的 contract_version 选择完整离线发布包，再校验结构与跨对象语义。未知版本拒绝，不能把旧对象改写版本后按新包计算 hash。DefinitionSnapshot、Definition、Skill、消息及其 action_protocol 引用必须来自同一绑定发布包；旧 Run 继续核对原始 Schema/模板/提示与同版 hash。1.1 旧批准副作用仅按原范围恢复，新范围或不能补全的来源创建关联新 Run。对没有 contract_version 的公共子对象，调用方须从所属 Run/完整对象提供版本，独立生成新对象默认使用当前版本。

1.2 保持原 DecisionAdvice/DecisionProposal/ActionValidation/ExecutionRequest/Proposal/Approval/PermissionSnapshot 字段不变。RunSnapshot.permission_snapshot 允许 null 表示尚未形成精确计划/权限；PLAN、IMPLEMENT、VERIFY、DELIVER、DONE 必须有完整 plan 与 permission_snapshot。未绑定权限不得生成获准执行请求，不把未知 GitLab 目标填成 local 或虚构 namespace_path/project_path。

## 输入来源与上下文

S2 接纳新输入时，先以真实身份、当前来源/进度和时间追加完整 MessageEnvelope/TextPart，再调用模型或规则并生成 DecisionProposal。Envelope 包括稳定 command_id，TextPart 保留精确用户内容；原始秘密在接纳前拒绝。旧 legacy 消息不补造当时不存在的 Envelope、版本和 hash。DecisionProposal.input_message_refs 保持 1–32 个真实 message 引用，不用空数组、假消息或任意临时对象满足结构。

ContextRecord 保存 context_id、run_id、decision_id、source_version、state_sequence、definition_snapshot_hash、input_message_refs、messages、input_context_hash、created_at、context_hash；messages 每项仅有 role/content，精确保存模型或规则实际收到的有序文本。input_context_hash=整个 messages 数组 JCS 的 SHA256，与对应 DecisionProposal 相同。上下文不可含凭据；服务端绑定系统规则、权限和当前需求，不接受模型自报来源。rules/model 描述真实建议来源，控制平面不得冒充模型响应；固定调度只根据已批准计划构造受控动作，不因模型失败切换语义解析模式。

上下文按实际决策目的选取必要事实：规划包含当前完整计划草稿及技能目录；已批准动作建议包含精确expectedAction、当前阶段/三轴/source/sequence、定义/计划/批准/权限hash和准确交付目标，不重复展开整份计划和全部技能默认值。冻结系统提示与最新用户原文不可截断，预算内再纳入较早消息，原消息和完整权威对象仍各自保存；不能靠增加冻结预算或把裁决必需事实移入摘要来容纳重复内容。

DecisionProposal.decision_mode 必须等于 Run.modes.decision_mode。model Run 的后续 action/diagnosis/completion 语义建议也调用冻结 provider，并预留/消费真实尝试预算；固定调度器只计算下一允许动作、参数和顺序并提供上下文，候选必须与该 expectedAction 完全相符才可能获准，模型不能自选顺序或决定 DONE。失败不转 rules、不由调度器伪造模型 Advice。rules Run 才生成同形规则候选。明确的 accept/pause 等控制命令直接接受确定性门禁，不冒充语义模型建议。

命令接入先绑定不可替换的业务载荷，在同一持久事务中登记本次 command 预留和完整输入；该提交成功后才构造上下文、预留 model 并调用 provider。模型调用的成功表示收到符合协议的建议，建议被治理拒绝仍消耗该真实调用；命令是否成功另行结算。成功命令的处理标记、决定链、状态变更和预算结算共同提交，重复成功命令不再调用模型或增加账目。进程重启时未结算的 command/model 尝试以 unknown/null 实耗结算，不释放、不伪造完成，也不重写原预留。

模型计划的内容在封装前按冻结参数规则规范化、复核依赖与准确目标，并重算 plan_hash；不能用模型提供的 hash 授权。相同规范化计划的再次描述只增加 state_sequence，不增加 source_version，也不刷新仍有效提案的签发时间。reject/revise 关闭当前未执行提案并保留其历史；真正的方案或权限范围变化才增加 source。自由文本动作建议仍必须与服务端确定的阶段及下一动作相符，命令中的“已确认”或 completion 建议不会代替 P1 或产品检查。

## 动作、权限与固定命令

| action_type | 合法执行阶段 | 最小 required_permission_ids |
| --- | --- | --- |
| scaffold | IMPLEMENT | workspace_write |
| apply_skill | IMPLEMENT | 该冻结 Skill.required_permissions |
| verify | VERIFY | tool_go、workspace_read、workspace_write |
| create_repository | DELIVER | gitlab_create |
| push_repository | DELIVER | gitlab_push、workspace_read |
| trigger_ci | DELIVER | gitlab_ci |
| package | DELIVER | package_write、workspace_read |

顺序为 scaffold → selected_skills 按计划顺序逐个 apply_skill → verify → create_repository → push_repository → trigger_ci → package。delivery_mode=local 不生成三个远程动作请求。每个动作必须在冻结 ActionProtocol 和具体 Approval.allowed_actions 交集中，不能用一个 legacy 阶段权限替代多个正式动作。accept/reject/revise/pause/resume/cancel/retry 是控制意图，不是执行动作授权。

合并 Agent 与选中 Skill 的 post_commands；相同 command_id 的 executable/arguments/cwd/environment 必须一致，timeout_seconds 与 max_attempts 取各声明的最小值且不超过冻结 BudgetPolicy。Skill 后置命令在所有所选技能文件已可用后的 VERIFY 集中执行，复用相同 source、批准、工具、命令、manifest 的真实回执；不得因重复声明多执行或伪造通过。命令只有 go.mod_tidy=[mod,tidy]、go.test=[test,./...]、go.build=[build,./...]；禁止 shell、自由 argv 或继承服务凭据。

OperationRecord.target_hash 对对应 ExecutionRequest.action.parameters 的完整 JCS 计算，七类动作只有这一种主体；request_hash 复用准确 ExecutionRequest.request_hash。操作身份与业务 command 独立，重试不换原写操作身份。sent/unknown 先持久化并进入核实，在有未执行的确定证据前禁止重发。旧 legacy 操作的既有 hash/identity 保留原恢复解释，不重新套用本条后覆盖历史。

## 预算预留、结算与累计期限

BudgetPolicy 沿用 1.1 的十一个冻结上限，policy_hash 对该完整 BudgetPolicy 的 JCS 计算，无排除字段。它不是新的可变配置；账本不得提高限额。新的对象全部使用闭合必需字段，具体上限和类型见 Schema：

| 对象 | 字段与职责 |
| --- | --- |
| BudgetReservation | contract_version、reservation_id、run_id、source_version、definition_snapshot_hash、policy_hash、command_id、operation_id、execution_id、request_hash、attempt_id、kind、scope_id、units、reserved_at、deadline_at、reservation_hash。一次调用前的不可变预留，units 固定 1；kind 为 command/model/action/tool/ci_poll/reconciliation_read。command/model 的 operation/execution/request_hash 为 null，其他种类必须绑定已登记请求。纯计数 command 可没有 deadline；其他带时限的尝试必须持久化明确绝对 deadline。 |
| BudgetSettlement | contract_version、settlement_id、reservation_id、reservation_hash、attempt_id、run_id、source_version、consumed_units、outcome、elapsed_ms、measurement_source、settled_at、settlement_hash。只引用既有预留；outcome 为 succeeded/failed/interrupted/released/unknown。只有未发出尝试且有确定证据时 released，消耗为 0；其他结算消耗为 1。actual 时 elapsed_ms 为非负真实值，unknown 时为 null。 |
| BudgetWindow | window_id、kind、operation_id、scope_id、source_version、started_at、deadline_at、max_attempts。kind 为 ci_poll/reconciliation_read；同一操作窗口首次建立后不可重置。 |
| BudgetLedger | contract_version、run_id、definition_snapshot_hash、policy_hash、ledger_version、execution_started_at、execution_deadline_at、ci_windows、reconciliation_windows、reservations、settlements、totals、updated_at、ledger_hash。执行起止在首次应用前为 null；首次应用记录后跨暂停、阻塞、恢复与执行段保持原值。 |
| BudgetTotals | reserved/consumed/released/unknown 四组 BudgetCounters，加 measured_elapsed_ms、unknown_elapsed_attempts。每组闭合六种 kind 的非负计数；由完整预留和结算重算。reserved 只计尚未结算的单位，consumed 包含 unknown，unknown 是 consumed 的子集，不重复加算；elapsed 只汇总 actual，unknown 次数独立显示。 |

reservations/settlements 保持登记顺序，reservation_id、settlement_id 和业务 attempt_id 各自唯一；同 attempt 重放不能再预留或重复扣账。每份预留最多一份结算；unknown 不能变成 released 或重置消耗，后续只读核实使用独立预算尝试和操作证据。窗口与所有 ledger 版本均保留；数组容量或编码限制达到上限时停止新动作，不裁剪未核对账目。ledger_version 按账目改变递增，普通读取不改版本。

模型失败也计 model，实际固定工具启动计 tool，CI/核实每次实际读取分别计账。命令、动作和子工具是不同预算维度，不能把一个维度的计数用于重置另一个。max_step_attempts 按固定动作/技能的稳定 scope_id 聚合；跨恢复不更换 scope。max_tool_seconds 约束单条命令；max_execution_seconds 从首次批准应用起含暂停/阻塞；CI 和核实分别使用同一操作的绝对窗口，不因 Activity/HTTP 重试更新起点。原 unknown 的核实可受其独立窗口约束，执行总期限耗尽不表示外部未执行。

scope_id 固定公式：command=command_id；model=decision:model；action=action_type，apply_skill 特例为 apply_skill:<skill_id>；tool=tool:<command_id>；ci_poll=ci_poll:<原operation_id>；reconciliation_read=reconciliation_read:<原operation_id>。scope 不含 attempt/source/resume，同 Run 的 source 变更也不重置冻结政策下累计额度或期限；只有新 Run 建立独立预算。窗口绑定原操作而非运输重试身份。

请求先冻结，BudgetReservation 再单向引用 request_hash；ExecutionRequest 不包含后生成的预留 hash。结算单向引用 reservation_hash，账本再包含所有记录，避免请求/预留、预留/结算相互引用造成循环。真实输出计量和结算由可信执行器提供，模型不能签发。

执行总窗口从首次接纳已批准动作链的调度开始，包含 model 模式必需的后续动作建议时间；此时尚不表示副作用已经发生。`Approval.started_at` 则仅在真实动作尝试的原子预留/复校事务中设置，并将准确原 operation 从 ready 登记为 sent。预备 ExecutionRequest 不写项目文件、不设置 started_at；重试预备请求返回原 ExecutionRequest、validation 和 operation。sent/unknown 即使在暂停、终止或权限撤销后仍可返回原身份供有限只读核实，但不得返回可写 CandidateAction。

## 工具与工作区证据

ToolExecutionReceipt 是 object_type=evidence 的完整不可变主体：准确 receipt/run/source/definition、execution/operation/attempt/request、approval_id/approval_hash/proposal_snapshot_hash/plan_hash、workspace、tool、command_id/arguments、cwd_scope=run_workspace、environment_policy=isolated、input_manifest_ref/output_manifest_ref、started_at/completed_at、exit_code/status、output/output_sha256/output_truncated、receipt_hash。成功 exit_code=0；失败非零；unknown 为 null。工具身份必须等于冻结 RequiredTools；argv 与 command_id 精确匹配。

output 仅保存持久化前已脱敏的有界输出，UTF-8 最大 65,536 字节，Schema 的字符上限不能代替字节上限；超过按完整字符截断并标记 output_truncated。output_sha256 计算实际保存的脱敏文本原字节。原始秘密不得先进入日志再补脱敏，不保存原始敏感 stdout/stderr 或其可逆编码。缺确切退出状态不可填 0。

固定 Go 后置命令在该 Run 下的隔离候选副本执行，原批准目录只由父执行器读取并核对完整 manifest；候选区、HOME、TMPDIR、GOCACHE、GOMODCACHE 均锚定该 Run，拒绝符号链接和特殊文件。子进程不继承服务环境，GOENV=off、GOWORK=off、GOTOOLCHAIN=local、CGO_ENABLED=0；tidy 仅使用固定公开 Go proxy/sumdb，test/build 禁止依赖网络下载，测试只允许回环网络。实际 Go 二进制构建版本/平台及已冻结的可选二进制 hash 必须匹配。执行需要可用的系统沙箱（macOS sandbox-exec 或 Linux bubblewrap），缺失或失败不能降级为普通子进程。

工具预留绑定父 VERIFY 的已登记 action attempt；逐命令 attempt_id 由该父 attempt 与固定 command_id 派生。同 attempt 有回执时复用原回执，已预留但缺回执不能重新启动而须记录 unknown 消耗。回执先持久化、再结算，恢复时可凭原完整回执补齐原预留结算。任何候选持久文件差异都返回需重新批准的结果；此时即使进程 exit_code=0 也只证明命令正常退出，output_manifest_ref 为 null，不能作为原批准内容的验证通过证据。候选差异保留供新 source/Proposal 使用；内容完全不变且实际核对后，回执才可引用原登记 manifest 作为输出。失败与超时保留真实回执及预算消费，工具执行器不自行推进状态或修改批准。

WorkspaceReceipt 是 object_type=workspace 的完整不可变主体，复用上述准确来源绑定与 workspace，另含 action_type=scaffold/apply_skill、written_paths、input_manifest_ref/output_manifest_ref、started_at/completed_at、status、receipt_hash。成功必须有真实 output_manifest_ref，written_paths 排序且无重复并受批准 changes/Skill.write_paths 限制。receipt_id 标识回执，不与 WorkspaceReference.workspace_id 混同。

manifest 引用必须解析为该 Run/source 下已登记的完整 ArtifactManifest。第一次写入之前 input_manifest_ref 可 null，失败且无可验证输出时 output_manifest_ref 可 null；不能伪造不存在的产物。结构合法的 receipt 仍须核对原请求、来源、批准、工具与实际输出，不能从模型建议生成成功回执。

## VERIFY 修复与权限生命周期

同批准快照的有限安全重试保留 source，记录新 attempt 并消费原预算。修改文件、依赖元数据、参数或动作范围的修复必须递增 source、回 PLAN、作废未执行旧授权并重新预览确认；不可恢复或预算耗尽才终止 FAILED，终态重做使用新 Run。

1.2 不以 impact 自由文本授予隐式 go.mod/go.sum 派生写权限。首次 tidy 也是受控命令，其持久写结果须与已批准内容一致；若产生未批准差异，在隔离候选区保留差异和证据，形成新 source/Proposal，重批前不得把差异应用为原批准工作区事实。可在新模板发布中预先冻结解析后的 go.mod/go.sum 原字节纳入初始预览。旧 1.1 已批准路径只保留原恢复范围，不改写其历史授权。

权限撤销由可信管理操作签发新 permission_version/source，当前 Approval 标记撤销并停止新副作用，保存已发生事实；历史 Approval/Permission 保留登记时原字节。Proposal/Approval 的生命周期投影与不可变历史档案分开，历史 accepted/started 不能替代当前权限。普通 state_sequence 与读取不重新签发时间或批准。

## 固定摘要及引用

原有对象的排除字段继承 [公共契约](../../AGENT-CONTRACTS.md#固定哈希输入与无循环关系)，保持嵌套 hash、null、业务数组顺序。新增固定规则如下：

| 对象 | 只排除的自身顶层字段 | ObjectRef 身份 |
| --- | --- | --- |
| BudgetReservation | reservation_hash | budget_reservation / reservation_id / 1 / reservation.source_version |
| BudgetSettlement | settlement_hash | budget_settlement / settlement_id / 1 / settlement.source_version |
| BudgetLedger | ledger_hash | budget / run_id / ledger_version / 该版本首次登记时 Run 的 source_version |
| ContextRecord | context_hash | context / context_id / 1 / context.source_version |
| ToolExecutionReceipt | receipt_hash | evidence / receipt_id / 1 / receipt.source_version |
| WorkspaceReceipt | receipt_hash | workspace / receipt_id / 1 / receipt.source_version |
| S2AuditBundle | audit_hash | 只读观察对象，不冒充 RunSnapshot 引用 |

BudgetPolicy 完整对象 hash、ordered ContextMessage 数组 hash 无排除字段。BudgetWindow/BudgetCounters/BudgetTotals 为账本内嵌结构，无独立对象身份。引用必须精确匹配 object_type/id/version/source/hash，先登记主体再引用；跨 Run、未知主体、缺内容或摘要不匹配拒绝。

BudgetLedger 没有 source_version 字段，其引用来源由首次登记该 ledger_version 的索引持久保存。后续 Run source 改变而账目未变时继续返回原引用，不改写 Ledger 原字节或给相同 ledger_version 另绑新 source；下一次真实账目变更才产生新 ledger_version，并登记当时 source。普通读取不能创建预算版本。当前阶段的历史核验依真实公开快照轨迹与拒绝测试；ContextRecord 自由文本不能当成权威历史阶段或完成证据。

## S2 公开观察接口

| 方法与地址 | 完整输出 |
| --- | --- |
| GET /api/runs/{id}/decision-audit | S2AuditBundle：Run/modes/当前 state/source/sequence，完整冻结定义，nullable 当前 Plan/Proposal/Approval/Permission，原始 proposal/approval/permission 历史，全部 decisions/validations/executions/messages/contexts/operations，当前及历史 manifest，完整 budget、tool_receipts/workspace_receipts、created_at/audit_hash。数组同时保留允许与拒绝事实；不是 v2 RunSnapshot。 |
| GET /api/runs/{id}/objects?object_type&object_id&object_version&source_version&hash | 精确不可变登记对象或对应原字节，核对该 Run 的归属与引用许可；不把最新生命周期投影覆盖归档，不接受任意文件路径。 |
| GET /api/runs/{id}/budget | 完整 BudgetLedger，含未结算预留、全部结算、窗口及可重算计量，不把显示计数当作账本。 |

三个 GET 无副作用、不触发模型/工具/检查器。公开错误沿 legacy error/code，缺历史来源 409 legacy_contract_incomplete 并保留 legacy_read_url；参数缺项 400、未绑定或不存在 404、内容损坏 409。S2AuditBundle 中 nullable 当前对象如缺合法来源保持 null，不能伪装为已冻结；执行阶段的必需绑定依 Schema/语义门禁要求拒绝缺失。历史来源缺失无法形成完整审计链时整个接口返回缺来源错误，不拼凑字段。

`GET /budget` 同时返回 `Link: </api/runs/{id}/objects?object_type=budget&object_id=...&object_version=...&source_version=...&hash=...>; rel="canonical"`，指向该 ledger_version 首次登记时的精确不可变对象。查询参数由已持久化的五字段引用生成；调用方不得用当前 Run.source_version 猜测历史预算引用。完整 BudgetLedger 响应体保持不变。对象 GET 的 `X-Content-SHA256` 为返回原体字节摘要，不冒充排除字段后的逻辑对象 hash。

## 验收范围

发布先验证 Draft2020-12、离线引用、生成类型和来源摘要；随后分别验收结构、跨对象 hash/权限/预算语义以及真实副作用路径。跨重启/CAN保留完整来源、窗口、未结算预留和原操作；错误来源、未知 action、未批准/过期/撤销、错 argv/工具/路径、超预算和假完成均在副作用前拒绝。结构验收不替代 VERIFY 修复、受控执行与 live 第三方证据。
