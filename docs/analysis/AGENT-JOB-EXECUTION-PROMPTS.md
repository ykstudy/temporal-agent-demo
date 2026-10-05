# 项目创建 Agent 和运行 Job 的补齐执行提示词

2026-10-05执行状态：提示词1–6（S1–S4及运行补齐）已通过。提示词7的本地契约/公开路径/浏览器验收完成，原生成项目容器构建被Docker Hub网络超时阻塞，结论为 `passed_with_exclusions`。提示词8已核对前置条件，缺真实模型/GitLab配置与授权测试目标，保持 `external_blocked`，没有宣称全部阶段完成。当前证据见[实现状态](IMPLEMENTATION-STATUS.md)及[S5索引](../evidence/s5-exit.json)。下方原缺口表只保留追踪基线。

| 初始范围 | 当前结论 |
| --- | --- |
| A01–A06 创建Agent定义、技能、建议、授权、上下文 | 本范围已实现并取得契约/路径证据；模型实际外部提供方仍待live |
| J01–J08 消息、工作区、交付、恢复、预算、审计、健康、VERIFY修订 | 本项目单写者范围已实现并验证；不扩展通用Job平台或生产部署 |
| V01 独立与外部验收 | 严格oracle、当前local/Temporal、mock故障和浏览器已完成；原容器与live仍有外部排除项 |

外部环境具备后的续执行提示词：

```text
继续本项目提示词7–8的外部阻塞项。先读 docs/evidence/s5-exit.json、docs/evidence/live-prerequisites.json 与 IMPLEMENTATION-STATUS.md，只复验本次受环境影响的项，保留全部原始失败报告。对公开三技能交付包运行 validation.contracts.container_delivery，使用原Dockerfile与准确镜像版本，实际构建/启动成功后再关闭容器排除项。真实模型和GitLab配置通过可信环境或配置路径提供，凭据不进入提示、命令参数或报告。先只读核对指定测试namespace/project及Runner，形成准确Run方案并按产品门禁确认；不得用总实施授权替代具体远程目标批准。随后执行原提示词8的模型/摘要/建仓/提交/准确SHA CI/工件全链，记录live证据及仍未跑的模式组合。缺条件继续保留blocked，不能借mock或缓存的其他镜像宣布完成。最后更新阶段索引、差异项和状态文档，并运行文档检查。
```

**当前执行已恢复（2026-10-04）。** 用户本轮明确要求“按照顺序完成所有阶段”，覆盖2026-10-03的停止条件。保留S1既有验收和S2新增实现，先核对并完成S2退出证据，再依次推进后续阶段；本声明不把未通过项目标为完成。以下2026-10-03停止记录仅保留为历史。

核对日期：2026-10-02。本文根据当前规范和工作区源码整理后续实施任务。本轮只做缺口核对和提示词交付，没有实施产品功能或重跑产品验收。现有 [S1–S5 计划](../../design/IMPLEMENTATION-PLAN.md) 及 [G01–G21 差异矩阵](CONTRACT-GAP-MATRIX.md) 继续有效；本文不另立一套公共契约，也不把待实施项目改为完成。

后续执行更新（2026-10-03）：用户已要求按1–8顺序执行。第1阶段S1的实现与验收已记录在[实现状态](IMPLEMENTATION-STATUS.md#第1阶段--s1完整定义与技能冻结验收)；下方缺口描述保留提示词交付时点，实际关闭范围以阶段证据为准。第2阶段已发布1.2/1.3，接入受控命令、工作区、Go、打包、候选修订重批和本地完成报告；SQLite两ORM冷Run已通过，建仓未知结果可显式只读核实。受控push/CI、完整公开/Temporal/独立oracle仍未完成，默认新Run仍受未就绪门禁保护；S2退出前不进入S3，第2–8阶段继续按序推进。

当前停止边界（2026-10-03）：用户最新要求“完成第2阶段后停止目标”。本轮持续完成S2的实现和退出验收，达到退出条件后停止目标，不进入提示词3。下方3–8保留为未执行的后续提示词，不再作为本次目标的继续授权。

## 范围和当前结论

这里的 **Job 指一次项目创建业务 Run 及其执行过程**。现有产品实体是 Run；Temporal 模式通过 ProjectWorkflow 调度 Activity，local 模式执行同一组业务步骤。GitLab CI job 是交付验证中的远程任务，不能与业务 Run 混同。当前规范没有要求新建通用 Job 平台或运行 Task DAG。

当前已有 legacy 创建主线：需求与澄清、方案差异、版本绑定确认、Go 脚手架和数据库/认证/日志三技能、真实 test/build、manifest、ZIP，以及 GitLab 集成和未知结果核实代码。已有暂停/恢复/取消/关联新 Run 重做、幂等、部分预算、Temporal Continue-As-New 与故障恢复路径证据。这些能力应保留并迁移。

**主要未完成的是完整冻结定义、结构化建议与执行授权、可追溯消息和摘要、严格 v2 接口、运行记录与就绪检查，以及新契约下的独立验收。** live 模型和 live GitLab/Runner 仍缺真实成功证据。历史通过数量只证明当时选定的 legacy 路径，不能作为这些缺口已经关闭的依据。

本次补齐覆盖创建 Agent、实际执行器、生成项目能力与交付验证、运行工作区和必要的单写者恢复能力。静态十一类设计审查图已单独交付，不再扩充。多语言、通用插件市场、任意 shell、多租户生产平台、多节点事务、客户数据库真实迁移和生产合并/发布不纳入本文完成目标。

## 缺口清单

“缺失”表示目标对象或能力尚未落地；“部分”表示已有有效实现但未覆盖目标契约；“待验证”表示不能仅凭已有代码或旧报告判断通过。

| 编号 | 缺口及实际影响 | 当前判断和源码依据 | 归属与执行提示词 |
| --- | --- | --- | --- |
| A01 | 完整 AgentDefinition/DefinitionSnapshot。提示、动作 Schema、完成条件、全部依赖需要冻结；旧 Run 后续调用必须使用原定义 | 缺失。`types.go` 的 DefinitionSnapshot 只有部分元数据；`model.go` 每次使用当前代码中的 prompt；`contracts.go` 的版本标签不能证明完整冻结 | G01/G02/G06，S1，提示词 1 |
| A02 | 三项 SkillDefinition。参数类型、默认值、范围、依赖/冲突、模板、路径、固定命令、权限与验收必须来自同一版本 | 部分。三技能生成代码已有；`types.go` 的 Skills 是枚举表，默认和校验分散在 `templates.go` | G03，S1，提示词 1 |
| A03 | 严格 Schema 和真实 hash 内容校验。重复键、非法数值、日期、未知字段、跨语言规范化均需一致 | 部分。`http.go` 使用 Go decoder，`contracts.go` 的 digest 使用普通 json.Marshal；没有完整发布 Schema/JCS 的运行边界 | G01–G06/G17，S1 起逐阶段落实，提示词 1、7 |
| A04 | clarification/plan/action/diagnosis/completion 五类建议，以及服务端绑定身份、上下文、来源和真实用量状态 | 缺失。`model.go` 只解析 plan+explanation；`service.go` 直接写 Plan；rules 也没有同形 DecisionProposal | G04，S2，提示词 2 |
| A05 | ActionValidation、PermissionSnapshot、完整 Approval 与 ExecutionRequest。把动作、参数、目标、权限、预算和批准链形成可核实对象，并在执行前重检 | 部分。现有版本/hash/TTL/目标保护有效，但 `CommandResult.Execute` 布尔值和 `StepInput` 不能代替执行请求；当前批准的动作集合和身份记录较简化 | G05/G15/G20，S2，提示词 2、6 |
| A06 | 可追溯上下文和 SummaryRecord。model/rules 按显式模式生成摘要，冻结原文范围，追加历史，失败保留旧版 | 缺失。`service.go:compactContext` 根据当前 Plan 覆盖单个字符串；没有模型摘要、来源字节 hash 和历版记录。`model.go` 已有保守输入预算，应保留 | G10，S3，提示词 3 |
| J01 | MessageEnvelope、五类 Part、权威顺序、历史 ObjectRef、持久命令回执和历史对象读取 | 部分。`store.go` 已追加 JSONL 并检查损坏；消息仍是文本 DTO，不能表达完整版本引用和摘要来源 | G07–G09，S3/S4，提示词 3、4 |
| J02 | 完整 RunSnapshot、v2 HTTP/SSE 与严格运行工作区。历史方案卡、来源、完整检查、断线/乱序恢复需真实可用 | 缺失。`http.go` 只提供 legacy `/api`；前端 `lib/api.ts` 使用 `data as T`，`Workspace.vue` 展示文本和单一当前 ProposalCard | G11–G13/G16/G19，S4，提示词 4、5 |
| J03 | v2 同源交付证据：批准、定义、manifest、文件、RequiredCheck、VerificationReport、精确 commit/pipeline/job 必须绑定 | 部分。`verification.go` 已做真实内部验证；完整 v2 对象及独立复核尚缺。不能将已有 test/build 说成不存在 | G14/G18，S4/S5，提示词 4、7 |
| J04 | 新契约下的执行恢复和持久一致性：命令/operation/运输尝试身份、CAN、暂停/取消、安全恢复、unknown 原身份核实和损坏存储边界 | 部分且待新验收。`workflow.go`、`execution.go`、`store.go` 已有基线；旧路径证据不覆盖新增对象、引用和权限链 | G05/G09/G15，S2–S5，提示词 2、3、6、7 |
| J05 | 完整冻结预算、预留/结算和总等待边界。失败计账、重复回执不双扣，工具和 unknown 核实不能无限等待 | 部分。`BudgetPolicy` 只含五类计数上限；local 使用 Background，Go 子命令无独立期限；只读恢复豁免阶段尝试计数，CI 每次重新开计时器，缺持久总核实额度/期限 | budget-limits 功能及目标 BudgetPolicy，提示词 1、2、6 |
| J06 | 结构化审计、活动尝试/最后心跳、准确累计指标、稳定失败归因及恢复入口 | 部分。`History`/`Step`/`OperationRecord` 提供基本记录，但缺完整关联事件和指标；Workflow Query 与执行进度来源尚需明确表达 | workflow-observability、audit-traceability、failure-attribution，提示词 6 |
| J07 | 真实存活/就绪检查及单写者恢复。Temporal、Worker、存储故障应可见，恢复不丢批准和外部操作事实 | 部分。`http.go` 的 health 固定 ok，temporal_connected 仅检查客户端非空；已有文件锁/启动读取，不等于完整恢复验收 | health-recovery、service-runtime，提示词 6 |
| J08 | VERIFY 失败的修复闭环。可安全重试时沿原批准有限重试；改变文件/依赖/参数时回 PLAN 新提案重批 | 缺失关键分支。`execution.go:failStep` 将普通验证错误直接设为 FAILED；`service.go` 拒绝已批准 Run 的新需求，尚无上述两类修复路径。不可恢复失败和终态新 Run 重做仍须保留 | state-machine、AGENT-CONTRACTS，S2，提示词 2、7 |
| V01 | 独立严格 oracle、逐字段/跨对象负例、当前代码的端到端回归、真实外部系统验证 | 缺失或待验证。`validation/scenarios.py` 存在 passed/status OR 和 manifest 别名兼容；model/GitLab fixture 是 mock；live 与生成项目 Docker build 证据缺失 | G17–G19/G21，S5，提示词 7、8 |

源码定位均相对 `backend/internal/agent/`，前端和 validation 路径除外。精确规范及既有证据见 [Agent 使用手册](../AGENT-GUIDE.md)、[实现状态](IMPLEMENTATION-STATUS.md)、[对象契约](../../design/contracts/AGENT-CONTRACTS.md)、[消息协议](../../design/contracts/MESSAGE-PROTOCOL.md)、[hash 规则](../../design/contracts/HASHING.md)。J05–J08 还需结合对应功能文档；现有目标 BudgetPolicy 本身也只声明五类计数上限，尚未覆盖功能契约规定的工具/累计时限与核实额度。先补发布契约，不能直接向闭合 v2 对象塞字段。

## 使用顺序

希望连续推进时复制“总执行提示词”。希望拆成可验收任务时，按 1 → 2 → 3 → 4 → 5 → 6 → 7 → 8 执行；每条都先读取本文件的共同约束。提示词 1–5 对应正式 S1–S4，提示词 6 关闭本范围内运行缺口，提示词 7 对应 S5，提示词 8 单列 live 验证。

独立 oracle 的准备应从 S1 开始，不能到 S5 才发现结构错误。提示词 6 的契约盘点也应提前完成，其中授权/预算/执行边界分别纳入 S1/S2；它的末轮用于检查所有运行缺口是否真正关闭。允许分工准备测试、存储和消费者，依赖阶段的写入及验收仍须按正式阶段门槛推进。

## 所有提示词的共同约束

1. 先读根目录及工作目录 AGENTS.md、`design/README.md`、`design/BUSINESS-FLOW.md`、`design/contracts/README.md`、`MODULE-MAP.md`、`AGENT-CONTRACTS.md`、`MESSAGE-PROTOCOL.md`、`HASHING.md`、`docs/AGENT-GUIDE.md`、差异矩阵和 S1–S5 计划，再读本阶段功能/ADR。源码与规范有差异时明确记录，不以源码降低契约。
2. `docs/PRODUCT-GUIDE.md` 和 `design/00-invariants/INVARIANTS.md` 的字节与冻结摘要不变。已发布身份不能绑定不同内容；需要补充对象/路由/字段时先发布契约、Schema、hash 输入、兼容规则和必要 ADR。新 ADR 按登记规则同步 INDEX、实体和关系。
3. 三轴模式独立；model 不静默变 rules，gitlab 不静默变 local。仅 Go 项目与三项已登记技能；不创建通用 Job/DAG 系统，不追加生产部署和客户数据库写入。
4. 模型只提建议。可信控制面决定身份、版本、权限、预算、批准、状态与 DONE；原文和摘要中的指令均不能授予执行权。工具调用使用批准工作区、固定参数数组和过滤环境，秘密不进入模型、参数、remote URL、历史或包。
5. 保留 legacy 原始资料和已有批准交付/unknown 核实链。无法完整可信投影时 v2 GET 返回 409 legacy_contract_incomplete 与 legacy_read_url；前端只读。禁止补造历史 prompt/Part/摘要，禁止 v2 写请求自动回落 legacy。
6. 产品自身运行 required_checks；`validation/` 只消费公开接口/产物，`design/_model` 只做离线文档检查。Go/TS/外部验收 Schema 必须与发布源摘要一致；不得缩减 DTO、放松 required、接受字段别名或改测试迁就响应。
7. 保护并行修改，仅写本任务文件。当前目录未发现 `.git`；若实施时仍无 Git，用时间、源文件 SHA-256 清单与配置标识记录代码时点，不能伪造 commit。不要初始化仓库或提交无关文件来凑验收。
8. 每阶段交付完整实现、对应独立结构/语义负例和必要回归，再更新差异矩阵、实现状态及使用说明。文档改动跑 `python3 design/_model/check.py --self-test --json`；后端遵循 backend README，涉及并发/批准/恢复跑 race，完成后 vet/build。前端构建、浏览器、真实 Temporal、mock/live 分开登记。
9. 每个结果写明代码时点、契约摘要、模式/配置、命令、报告路径、通过范围与未覆盖项。未执行记 not_run，缺必需外部条件保留 blocked；不能把 docs_ready、contract_ready、path_verified、live_verified 混成“全部完成”。

## 总执行提示词

```text
请完成当前项目“项目创建 Agent 以及实际运行 Job”的已定义范围。
先读取 docs/analysis/AGENT-JOB-EXECUTION-PROMPTS.md，遵守其中所有共同约束，并核对实施时最新源码与证据。本任务正式授权在该范围内推进产品、运行前端、契约和独立验证实现；不要只继续输出计划。

Job 以业务 Run 为核心，Temporal Workflow/Activity 为执行实现，GitLab CI job 为交付验证对象。保留现有有效创建主线，按 design/IMPLEMENTATION-PLAN.md 的 S1→S2→S3→S4→S5 顺序推进，执行本提示词文件中的 1–8。不要把 Vue 迁移或静态设计审查图计入本任务完成度。

先将 A01–A06、J01–J08、V01 映射到现有 G01–G21、功能契约和测试；已有实现先验证后复用。S1 冻结完整定义/技能并建立严格Schema/JCS；S2 打通建议→确定性校验→批准绑定执行及VERIFY修复；S3 打通原文、Part、历史引用、上下文和摘要；S4 提供完整 v2 API 与可用运行工作区；补齐范围内的预算、审计、失败归因、readiness和恢复；S5 用独立 oracle 验收完整对象及当前代码路径；live模型/GitLab另行留证。

每个阶段开始先核对契约、负例和迁移规则，阶段通过后再推进依赖写入。可以委派独立审查和测试准备，明确文件归属；不能并行修改同一核心状态机。对缺少公开格式的运行输出先完成契约发布，不修改已冻结规则来消除实现困难。

对验证发现的缺陷修复根因并复测，不用别名、缩减对象、mock success或历史报告填补缺口。保留用户确认门，实施授权不能自动批准产品中每个新Run的真实仓库操作。live写入只使用已明确授权的测试目标及该Run的有效批准；缺配置或目标授权时列出准确缺项并继续其他可完成工作，不得猜测目标或读取无关秘密。

持续交付，直到本范围内所有功能缺口和必需验证逐项闭环，或只剩有证据的外部阻塞。S5原计划允许遗留P1不等于本任务允许把范围内P1省略；权限、预算、审计、就绪和实际Job可用性都须逐项处理。最终报告列已完成、未完成/blocked、证据位置和可复现入口；任何必需项仍blocked时不得宣称全部完成。
```

## 提示词 1 冻结定义和技能

```text
执行本项目 S1。先读 docs/analysis/AGENT-JOB-EXECUTION-PROMPTS.md 的共同约束，以及 definition-versioning、skill-contract、template-versioning、context-building、budget-limits 和 ADR-005/012/013/046。实现完整 AgentDefinition、SkillDefinition、DefinitionSnapshot，关闭 A01–A03 及 G01/G02/G03/G06 对应缺口。

以发布 Schema 为唯一结构来源建立 contracts 与 definitions 边界。冻结系统提示原文/hash、DecisionAdvice动作协议及参数Schema、三技能定义、模板原字节和依赖、固定工具/post_commands、上下文/预算/完成策略及非秘密provider绑定。同定义/技能版本不同内容必须拒绝，依赖闭包缺失/冲突/hash不符不得创建可信快照。

实现严格JSON入口、离线Schema registry和HASHING规定的JCS；按固定hash排除清单计算，不把普通json.Marshal当作JCS。发布真实hash向量，核对Go/TS/Python一致性。Schema的default不能直接当参数规范化实现；三技能参数须统一补默认并复校，包括duration、路径和组合语义。

实际模型上下文、模板渲染和执行器必须消费Run绑定的旧内容，而非只保存hash、执行时继续读最新源码。现有模板和三技能生成测试保留。旧Run缺失历史内容只读保留，不拿当前prompt补成可信冻结。

验收C01–C06中本阶段可观测部分：两个版本并存，同版本异内容拒绝，更新最新提示/Skill/模板后旧Run请求及产物不漂移，缺依赖拒绝，参数负例拒绝，三技能生成test/build仍通过。没有公开v2入口时不声称C06的HTTP分支已验收，留待S4完成。更新差异状态与证据，通过退出条件后进入S2。
```

## 提示词 2 打通建议校验与受控执行

```text
执行本项目 S2。先读提示词文件共同约束，确认S1退出证据，再读 model-decision、action-validation、human-gates、permissions-and-secrets、idempotency-and-concurrency、activity-scheduling、unknown-results 及相关ADR。

把plan+explanation路径改造成完整 DecisionAdvice→DecisionProposal→ActionValidation→ExecutionRequest。model/rules都支持契约规定的clarification、plan、action、diagnosis、completion建议，来源明确；服务绑定Run/source/state/definition/input_context，不信任模型自报身份、hash、批准、用量或完成。unknown/not_applicable用量按契约表达，不填零冒充known。

落地PermissionSnapshot和完整Proposal/Approval。动作、精确GitLab目标、权限hash、定义闭包、计划、期限和可信actor互相绑定；接受与首次执行重检，执行链逐步检查当前撤销、来源、预算和范围。普通进度只增加state_sequence；业务范围变化增加source并重新提案。只有allowed的action生成执行请求，其他建议不获得副作用权。

让Temporal和local共用受控执行入口，支持七种已登记执行类型及固定工具参数；不再依赖Execute布尔值授权。保留实际文件/Go命令/GitLab副作用和操作账本。写前登记原operation；sent/unknown先BLOCKED并按原身份核实，禁止换command或运输ID盲重放。终止重做为关联新Run；暂停/取消在安全边界生效且披露已发生事实。

将冻结预算与实际尝试相接，失败计账、重放不双扣、预留/结算可核对，总等待与核实预算不可因resume重置。精确工具版本、cwd、路径、环境和秘密边界在执行前复核。

补齐VERIFY失败处理：按确定性错误分类区分同批准快照的有限安全重试、需要改内容的修订和不可恢复终止。修订若改变文件/依赖/参数，先新source与新提案回PLAN，重批前不得写修复工件；保留失败证据和原副作用。不把全部构建失败直接FAILED后用新Run冒充已实现原Run修订，也不复活已终止Run。

验收C07–C10及原有批准/重试路径：假批准、跨Run、过期、撤销、错hash、越权目标、任意shell和伪造ExecutionRequest均拒绝且无新增副作用；包含实际允许请求、失败Update同身份重试、unknown写次数断言、pause/cancel/终止重做，以及VERIFY同快照重试、修订重批和耗尽终止三种路径。涉及状态/并发跑race，记录结构、语义与路径三类证据。
```

## 提示词 3 实现原文消息上下文与摘要历史

```text
执行本项目 S3。先读提示词文件共同约束，确认S2退出证据，再读 MESSAGE-PROTOCOL、jsonl-message-store、rich-message-protocol、context-building、context-compression、durable-messaging、continue-as-new。

实现完整 MessageEnvelope 与text/component/diff/choice/action五类Part。服务端分配权威sequence，历史提案/工件/报告绑定完整ObjectRef并解析当时对象；新增方案不能改旧消息卡。未知component保留有界JSON/fallback，canonical未知Part不能伪装第六分支。消息与命令去重、顺序、回执和不可变对象登记要有明确持久提交顺序。

实现分层ContextBuilder及真实SummaryRecord链。冻结原消息ID/sequence范围和包含换行的原JSONL字节hash；按显式decision_mode与冻结策略生成规则或模型摘要。model失败不转rules，不覆盖上一摘要；摘要版本、previous_summary_ref、生成来源、来源核验和失败记录可追溯。按范围幂等，生成中新增消息进入后续范围。权限、预算、状态和完成条件只读权威对象，摘要“已批准”文本不能赋权。

预算包含必要规则、完整当前消息、状态、输出和协议预留，超限在调用前拒绝；保留现有保守计量并明确估计来源。追加成功后再切换当前摘要引用，原文和历史对象不重写。处理重复、坏尾部、部分提交、重启和跨CAN；禁止静默丢行或凭当前Plan补历史摘要。

按C11–C14独立核对完整对象与来源语义，并复测并发/消息去重/长期上下文/失败保旧。检查model摘要确实调用冻结provider、rules没有模型调用。v2公开读取留待S4时补齐验收，不能用summary_count>0代替历史和来源证明。
```

## 提示词 4 提供完整 v2 API 和交付对象

```text
执行S4后端部分。先读提示词文件共同约束，确认S1–S3退出证据。严格实现AGENT-CONTRACTS中的最小v2路由和全部请求/回执/事件/错误，不猜测扩展接口。RunSnapshot使用run_id、modes和完整关联对象；领域内部结构与HTTP投影分开。

实现对象历史索引、来源范围读取、CommandReceipt、SnapshotEvent及精确run_ref。SSE引用对应历史快照，核对event_id/state_sequence与source的不同语义；定义断线、游标失效、缺口和快照订阅边界行为。不用最新快照替换不可变历史引用。

让实际内部验证输出完整ArtifactEntry/ArtifactManifest、RequiredCheck和VerificationReport。完成策略从冻结定义读取；保留真实test/build、来源/manifest/秘密/ZIP检查，gitlab额外核对准确project/ref/SHA/pipeline、必需job、coverage与可执行构建产物。缺项、重复项、来源错配、not_run不能DONE。RequiredCheck只用canonical status，文件摘要字段是sha256，ObjectRef仍使用hash。

legacy原读/受限恢复链保留。v2无法完整可信投影时返回409 legacy_contract_incomplete和legacy_read_url，不产partial v2、不补历史事实、不自动授予新范围。输入与输出都经发布Schema和语义校验；严格拒绝别名、重复key、未知字段、bool版本、非法日期、超限数据和错Run引用。

完成C06、C15–C17的API/迁移部分和C16交付绑定；独立调用所有已发布路由，核对Schema发布摘要，验证旧文件字节保护、unknown恢复不重复写、旧批准不扩大。同步使用手册及配置说明，再交给前端消费。
```

## 提示词 5 完成实际运行工作区

```text
执行S4前端部分。先读提示词文件共同约束，确认v2公开接口已通过后端结构/语义验收。只改项目创建Agent的运行工作区，不扩充静态设计审查，不重新做Vue框架迁移。

从已发布Schema派生完整TS契约并记录来源摘要，在HTTP/SSE入口做真实运行校验，移除以data as T充当验证的路径。实现Part白名单renderer、未知组件安全回退、历史提案/版本/hash/来源呈现；每个动作绑定当时对象并由服务判定当前是否仍可用，旧卡不能确认新方案。

打通创建、澄清、预览、确认、拒绝/修订、VERIFY修复重批、暂停/恢复/取消、终止后新Run、进度、错误恢复条件、文件/manifest、验证报告、ZIP以及摘要历史/原文入口。完整展示三轴模式、RequiredCheck规定的not_ready/running/passed/failed/not_verified状态、批准绑定、operation核实状态、预算使用、Workflow/执行段身份和远程证据，不以消息文本或ZIP存在显示DONE。外部验收报告的not_run不混进RequiredCheck枚举。摘要明确为派生记录。

实现Run切换隔离、晚响应保护、重复/乱序/跨Run事件拒绝、缺口重取和断线恢复；只有符合契约的权威快照更新界面。legacy_contract_incomplete进入明确legacy只读视图，禁止写请求回落。按现有可访问性契约补齐运行操作的键盘、焦点、状态通知、错误说明和安全Markdown展示。

验收C11/C15/C16/C17对应浏览器行为，保留当前主线/窄屏/竞态回归，注入错误payload、旧卡、跨Run事件、断线和缺检查。构建通过与真实浏览器通过分别登记；截图只证明外观，点击后的服务事实也要核对。
```

## 提示词 6 关闭实际 Job 的运行缺口

```text
执行本范围运行能力的完整收尾。先读提示词文件共同约束和J04–J07，逐条对照state-machine、activity-scheduling、continue-as-new、run-lifecycle、budget-limits、audit-traceability、workflow-observability、failure-attribution、service-runtime、health-recovery。已有通过能力保留，只补未满足的行为。

先核对这些功能的公开输出是否已有完整发布格式。缺少的审计/活动进度/指标/readiness字段、路由、hash、保留和兼容规则先做契约与必要ADR发布，不向1.0.0闭合对象临时塞字段，不把内部debug结构变成另一套API。授权/预算/执行绑定应已在S1/S2落实，本阶段检查遗漏并补齐。

实现追加式关联审计：command/operation/批准/定义/source/Activity尝试/Temporal执行段/结果/耗时/脱敏证据可串联。公开只读运行进度区分权威检查点与活动心跳，显示各自时间、数据过期和不可用；实现规范规定的成功/失败Run、实际重试、验证失败、总耗时和有效业务轮次，跨CAN/恢复不重复计数，暂停/取消不冒充失败。

补全稳定失败类别、步骤、影响对象、可重试性、待补条件和核实入口。local与Temporal均有冻结的工具单次期限和业务累计期限，不能只依赖Temporal的StartToClose。检查总预算/预留结算/总等待以及unknown核实上限，重复resume不能无限延长。健康检查分开liveness/readiness，检查实际Temporal连接、Worker可用和存储状态，返回检查时间/原因/受影响能力；探针不创建项目、不调模型、不触发CI。

在单写者部署边界内落实可复核的持久恢复规程：快照、原文、对象、批准、manifest和operation账本一致，坏尾部/缺文件/缺冻结依赖不能静默绕过。若恢复规程涉及备份，先定义一致性范围和停止写入边界，并在隔离数据副本演练恢复；不因此扩展多节点生产运维。

验收覆盖正常、失败、暂停、取消、实际重试和CAN累计；仅操作本任务自有进程，注入Temporal断连、Worker停止、存储不可写/损坏、命令超时和未知外部结果。恢复后保持原身份、原批准、原副作用，终止Run不复活。运行UI也要展示新增诊断的真实含义。逐项关闭J04–J07后再进入最终S5，不能仅以查询接口200或有日志宣布完成。
```

## 提示词 7 完成独立契约和运行验收

```text
执行S5。先读提示词文件共同约束、validation/AGENTS.md与README、C01–C18验收映射，确认S1–S4及范围内运行补齐已经可测试。保留原历史报告，新报告绑定当前代码与Schema摘要。

在validation建立独立Draft2020-12 oracle与语义校验，使用已发布本地Schema registry、format断言及严格JSON解析；不导入backend私有类型、常量或模板，不从当前响应反推期望。先验证合法对象能过、每类required/type/enum/pattern/additionalProperties/日期/数字/ref破坏必失败。Go/TS/外部副本与发布源摘要一致，内容hash/版本/权限/引用/sequence另行核验。

改造model fixture为冻结ActionProtocol规定的DecisionAdvice；服务绑定后的Proposal单独验证。legacy兼容与v2 oracle分开，去掉v2中的passed/status OR、manifest dict/list和hash/sha256别名容忍；legacy字段矛盾也须失败。固定required_checks完整集合、文件size/hash、准确远程来源由外部独立复核。

执行C01–C18及运行补齐新增用例：严格HTTP/SSE/历史对象与迁移读取，local真实ZIP解包/test/build/启动health，三技能参数与必要隔离验证，VERIFY失败修复重批，mock模型/GitLab故障，浏览器完整创建路径，真实Temporal CAN/并发/VERIFY中Worker故障恢复。对生成Dockerfile补实际容器构建/启动验收，不部署生产；容器运行时缺失则明确not_run，不能说标签存在即通过。新增对象必须跨重启/CAN保留，模型completion或外部validation结果不能直接决定产品DONE。

为三轴组合建立覆盖表，标明实际运行、明确mock或not_run；不能把两条主线说成全部组合已验收。先跑无副作用结构负例，再跑实际路径，昂贵恢复统一协调，避免每个子任务重复整套。需要的工具/隔离依赖缺失时记录准确阻塞，不借客户数据库或生产目标补齐。

报告逐例列功能ID、Schema ref、代码时点/配置、正负例、HTTP/事件/ZIP/运行证据和结论。更新G01–G21及A/J/V范围清单，只关闭已有证据的项。live第三方留给提示词8；阶段结论分别使用contract_ready与path_verified。
```

## 提示词 8 验证真实模型和 GitLab 交付

```text
在前述契约和路径验收通过后执行live验证。先读提示词文件共同约束及模型/GitLab/CI/权限规范。先只读核对可信环境是否具备完整model endpoint/key/model、GitLab实例/准确namespace/凭据权限/Runner和专用测试目标，不输出秘密。把缺配置、缺目标授权、缺Runner与实现失败分别记录。

仅在明确授权的测试目标内写入，先生成具体Run方案并按产品门禁取得该Run有效批准。总实施授权不能替代具体远程目标确认；不猜namespace、不接管已有项目、不强推、不合并或发布生产。若尚缺外部授权，先完成可审查方案和所有独立工作，再报告准确待办。

真实模型验证包括多轮澄清/计划、冻结提示和实际输入来源、五类协议中适用行为、结构拒绝、失败不静默转rules、实际/unknown用量口径，以及model模式摘要的来源与失败保旧。记录非秘密provider/模型身份、时点和脱敏请求结果摘要。

真实GitLab验证包括批准目标下建仓、批准manifest对应提交、准确SHA/ref流水线、必需test/build job、报告与可执行工件、来源一致性和最终下载。重试/取消/未知结果只在已授权隔离范围做故障试验，按原operation核实且无重复写入；破坏性模拟继续使用fixture并明确范围。记录真实project/commit/pipeline/job和产物摘要。

缺前置时相关用例保持blocked/not_run，不用mock补成live通过；必要live项未完成时不能宣布全部完成。成功后登记live_verified的具体模式和目标，保留mock和live边界，最终给出所有范围项的状态、证据、可复现入口和剩余外部阻塞。
```

## 最终完成判定

| 完成层级 | 本范围必须满足 |
| --- | --- |
| 契约完成 | S1–S4完整对象、来源hash、引用、权限及版本语义通过独立正负例，legacy迁移不补造历史 |
| 运行完成 | 真实内部验证与交付、实际Job控制/恢复/预算/审计/就绪行为符合范围，运行工作区路径可用 |
| 独立验收完成 | S5 C01–C18及本范围补充用例具有当前代码证据；无遗漏的必需P0/P1，外部调用器不决定产品DONE |
| 外部验证完成 | 已配置并授权范围内的真实模型、GitLab/Runner有独立live证据；未执行项明确保留阻塞 |

本提示词文件的交付不改变以上任何产品状态。实施者应逐项关闭本范围清单，而不是把“45份功能文档”“20/20路径”或“15/15浏览器”换算为完整产品完成。
