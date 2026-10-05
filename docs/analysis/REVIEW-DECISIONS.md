# 原设计审校与裁决

审校日期：2026-10-02。对象为 [原设计](../PRODUCT-GUIDE.md)，原文保持原样；本文件记录对原文实质缺陷的自主裁决，不以待用户确认替代技术判断。原文 SHA-256：`51c889d53a694e1047d9a9a168ef2fe753faa50a68519c0ebbcaa59a6895a45b`。行号以该快照为准。

本文件最初在设计文档阶段形成。2026-10-02 后续用户明确授权按当前规范实现独立 Agent，并移除参考副本与实验认知运行耦合，见 [ADR-045](../../design/10-decisions/ADR-045-agent-model-validation-separation.md)。当前实现、产品内部验证与外部场景证据在 [实现状态](IMPLEMENTATION-STATUS.md) 分别登记；实现存在或测试通过，不自动覆盖有效规范。

五层关联采用：L1 不变量、L2 领域概览、L3 功能契约、L4 ADR、L5 结构模型。下表中的领域/功能是当前项目文档应登记的契约身份建议，不承诺与参考仓库领域目录同名，也不把参考项目的生产发布职责直接移植进本项目。

表内 L5 的 Message/Run/Approval 等是功能正文需要描述的业务对象线索，不是 entities.yaml 要注册的运行实体；实际文档模型只登记域、功能、ADR、不变量及其结构关系。

当前项目已确定的领域路径如下；下文“执行/Runtime、仓库、CI、验证、运维”等是职责描述，按本表归属，不另建平行领域。

| 当前领域 | 路径 / 结构模型身份 | 主要关联裁决 |
| --- | --- | --- |
| 编排 | `design/01-orchestrator` / `domain.orchestrator` | R-001–004、006、013、021–022 |
| 决策与上下文 | `design/02-agent` / `domain.agent` | R-010–011、014–015 |
| 项目模板与技能 | `design/03-skills` / `domain.skills` | R-008–009、011、017、020 |
| GitLab 与 CI | `design/04-integrations` / `domain.integrations` | R-004–009、019、021 |
| 对话与确认 | `design/05-conversation` / `domain.conversation` | R-002–003、011、014–015、018、022 |
| 治理 | `design/06-governance` / `domain.governance` | R-006、011–012、015、017、019–022 |
| 审计与证据 | `design/07-observability` / `domain.observability` | R-009、016、018–019、023–024 |
| 展示与交互 | `design/08-frontend` / `domain.frontend` | R-002–003、011、023–024 |
| 本服务配置交付 | `design/09-deploy` / `domain.deploy` | R-016、018–019、023–024 |

## R-001 — Workflow 禁止原生 channel 与非确定性副作用

- 来源：§5.2，L305、L318、L328、L347、L388–391；§3，L139。
- 缺陷：说明声称使用 Signal 通道，代码实际注册 Update，却使用原生 `chan`、`make(chan)`、`len(chan)` 和 `<-`。不能将该示例作为 Temporal Workflow 的可执行基线。
- 裁决：Workflow 只使用 `workflow.Channel`、`workflow.NewBufferedChannel`、`workflow.GetSignalChannel`、`workflow.Await` 等 SDK 原语；外部 I/O、文件、模型、时间及随机身份生成进入 Activity。原生 goroutine/channel 可用于 Activity 与 HTTP 服务，不用于 Workflow。
- 理由：Workflow replay 必须保持确定性；SDK 的协作调度不能被原生同步设施阻塞。
- 五层关联：L1 `workflow-determinism`；L2 编排；L3 `message-ingress` / `workflow-loop`；L4 “采用 SDK 调度原语”；L5 Command、Queue、WorkflowExecution、ActivityInvocation。

## R-002 — Signal、Update、Query 的语义与 SDK 版本分别固定

- 来源：§3，L115–116；§5.2，L305、L344–349；§5.4，L559–585。
- 缺陷：Signal 与 Update 混称；客户端示例使用过时的参数列表，并把 Update 当作仅返回 error 的调用。Query 是观察，不是推进流程的入口。
- 裁决：客户端统一使用当前文档基线中明确锁定版本的 `UpdateWorkflowOptions`、稳定 `UpdateID`、`UpdateName`、`Args`、等待阶段及 `WorkflowUpdateHandle.Get`。Signal 用于异步事件通知；Update 用于需校验并反馈处理结果的命令；Query 不执行 Activity、不改状态。Query handler 明确返回 `(可序列化结果, error)`。
- 理由：服务器接受、命令排队、命令完成、整个任务完成是四个不同事实，不能用一句“同步请求”合并。
- 五层关联：L1 `acknowledgement-semantics`；L2 对话/编排；L3 `command-protocol` / `status-query`；L4 “消息入口与响应语义”；L5 MessageID、UpdateID、CommandReceipt、RunSnapshot。

## R-003 — 消息身份、归属、排序和背压补全

- 来源：§4.2，L165–177；§5.2，L346；§6.1，L600–610；§13.8，L1313。
- 缺陷：Message 没有稳定身份、Run 归属或客户端去重键；多个 Update 在消费前可能得到相同 TurnID。“通道不阻塞”没有容量、背压或拒绝语义。
- 裁决：消息携带稳定 message_id/command_id、agent_run_id、客户端关联身份及内容摘要；同 ID 同内容返回既有结果，同 ID 不同内容明确冲突。顺序号在权威串行接收点分配；队列容量有上限，满时明确拒绝或等待，不能丢消息。多个 Run 的消息与工件不可混用。
- 理由：串行 Workflow 不自动解决 HTTP 重发、并发 handler 和错误 Run 归属。
- 五层关联：L1 `message-identity` / `run-isolation`；L2 对话；L3 `conversation-store` / `command-ordering`；L4 “稳定身份和接收顺序”；L5 Message、Run、Receipt、Sequence。

## R-004 — 幂等业务身份不能使用 Temporal 执行 RunID

- 来源：§4.4，L239、L247–248；§7.3，L933–934；§5.3，L538–554。
- 缺陷：`WorkflowExecution.RunID` 会随 Continue-As-New、重做或其他执行生命周期变化；代码计算的 `idempotencyKey` 也未实际提交给外部系统。
- 裁决：操作身份使用稳定 `agent_run_id + operation_type + approved_source_version + operation_sequence`；Temporal workflow/run/activity/attempt ID 仅用于执行追踪。每个外部操作登记 fingerprint、目标、确认结果及原调用身份。Continue-As-New 保留业务身份。
- 理由：换执行链不等于新的业务写操作；一个局部变量不是幂等实现。
- 五层关联：L1 `stable-operation-identity`；L2 编排/执行；L3 `operation-ledger`；L4 “业务身份与执行身份分离”；L5 AgentRun、Operation、Attempt、ExternalResult。

## R-005 — 查到同名仓库不能证明属于本次任务

- 来源：§4.4，L250–257；§7.3，L936–944；§8.3，L1186–1194。
- 缺陷：只按 `ProjectName` 查找，缺 namespace、目标 GitLab 实例、所有权以及源版本验证；非 404 查询失败也可能误入创建。
- 裁决：精确目标至少为 GitLab 实例 + namespace/path；仅明确不存在时创建。已存在项目必须与该业务操作的 ownership/marker/已登记 project_id 相符，否则冲突并停止。读取权限失败、限流、网络错误不得当作不存在。
- 理由：幂等恢复不能变成接受他人仓库或覆盖既有代码。
- 五层关联：L1 `external-target-binding`；L2 仓库集成；L3 `repository-provisioning`；L4 “精确仓库身份”；L5 RepositoryTarget、OwnershipEvidence、ProjectID。

## R-006 — 结果未知先核实，不能按普通失败重试

- 来源：§7.3，L982–984；§8.3，L1183–1200；§8.1，L1159–1161。
- 缺陷：`ls-remote` 看到任意提交、`git fetch` + `git status` 或同名项目存在，不足以确认原操作已经完成；客户端断线也不证明外部写失败。
- 裁决：操作结果分为未执行、已确认成功、已确认失败、结果未知。未知结果进入 BLOCKED/待核实，按原 operation identity 核验 namespace/project_id、目标分支、提交 SHA、文件 manifest 与 pipeline ID。确认未执行才可重试；成功则登记原结果；不可核实保持阻塞。取消、超时不自动撤销已发生副作用。
- 理由：重放有副作用的请求可能重复建仓、提交或触发 CI。
- 五层关联：L1 `unknown-result-reconciliation`；L2 编排/外部集成；L3 `operation-recovery`；L4 “未知结果禁止盲重放”；L5 ResultStatus、Reconciliation、Evidence、ResumeTarget。

## R-007 — CI 必须绑定已确认提交，不能验证“最新流水线”

- 来源：§7.4，L1010–1027、L1073；§7.5，L1093–1112。
- 缺陷：原文明确允许重复触发多个 pipeline，并“只检查最新”；校验只看 branch 存在与 pipeline status，没有绑定批准产物/commit SHA。并发提交可使最新成功流水线验证其他代码。
- 裁决：登记仓库 project_id、目标 branch、批准的 manifest/hash、commit SHA、pipeline ID；CI 成功须绑定同一 commit SHA 与 ref。提交自动产生的匹配 pipeline 可复用；触发响应丢失先按该 SHA 核实，不新建第二条。不存在 ID、SHA 不匹配、pipeline 未终态均不能完成。
- 理由：CI 成功必须回答“本次批准的代码是否通过”，而非“这个项目最近是否有成功流水线”。
- 五层关联：L1 `verification-source-binding`；L2 CI/验证；L3 `pipeline-first-run`；L4 “pipeline 与产物身份绑定”；L5 ArtifactManifest、CommitSHA、PipelineID、VerificationReport。

## R-008 — CI 模板中的构建与覆盖率声明必须可执行

- 来源：§7.4，L1037–1067。
- 缺陷：`go build ./...` 没有输出到 `bin/`，模板却声明该目录为工件；`go test -coverprofile=coverage.out` 不会自动生成声明的 `coverage.xml`；安装 `govulncheck@latest` 造成版本漂移。
- 裁决：构建命令显式指定可执行包和输出路径；覆盖率报告要么真实转换成 Cobertura，要么只登记原始 coverage.out；安全工具和 Go 版本按定义快照固定。模板验收使用真实 CI runner，缺报告不视作通过。
- 理由：YAML 合法不代表构建、工件或报告存在。
- 五层关联：L1 `evidence-before-completion`；L2 CI；L3 `ci-template`；L4 “可执行报告与版本固定”；L5 BuildArtifact、CoverageArtifact、ToolVersion。

## R-009 — Verify 必需检查不能因为输入为空而省略

- 来源：§7.5，L1085–1147；§5.2，L486–530。
- 缺陷：project=nil 时跳过 code_pushed，pipeline_id 为空时跳过 CI，最后仅对已追加检查求 AND，有可能把不完整验证判为通过。文件存在与 .gitignore 包含 .env 也不证明代码可构建或密钥未泄漏。
- 裁决：先按交付模式生成固定 required_checks；缺输入是失败/未就绪检查，不从列表消失。远程模式必须有仓库、确认提交与匹配成功 CI；本地模式明确不执行远程检查，但须真实 test/build/产物 manifest。安全检查扫描日志、历史、工件与配置，不只检查忽略规则。
- 理由：完成条件必须封闭且独立，不依赖模型或之前 Activity 的成功声明。
- 五层关联：L1 `required-verification-set`；L2 验证；L3 `completion-verifier`；L4 “缺证据不完成”；L5 DeliveryMode、RequiredCheck、CheckStatus、CompletionEvidence。

## R-010 — 模型只建议，确定性控制平面裁决

- 来源：§3，L139；§5.1，L286–300；§5.2，L418–530。
- 缺陷：直接 switch 模型 action，未登记动作 Schema、合法转移、前置条件、工具权限、预算与批准状态；未知 action 会回到循环而没有拒绝/停止语义。
- 裁决：模型生成结构化语义建议，所有动作通过白名单、参数 Schema、状态、依赖、预算与授权检查后才调度 Activity。模型输出、自然语言“接受/已批准”、自身 `propose_completion` 均不构成批准或完成事实。未知动作明确拒绝并有限修正，不无限空转。
- 理由：模型可帮助澄清和规划，但不能获得控制平面与权限裁决权。
- 五层关联：L1 `semantic-advice-vs-authority`；L2 Agent/编排；L3 `decision-contract` / `state-machine`；L4 “模型建议与执行分离”；L5 DecisionProposal、ActionSchema、Transition、Authorization。

## R-011 — 创建方案必须先预览、绑定版本再确认

- 来源：§2.1，L28；§2.2，L44–55；§5.2，L434–510；原文缺少明确预览/确认契约。
- 缺陷：用户选择 Skill 后即可直接写文件/创建仓库，无法审查范围；定义虽要求运行中不漂移，但输入未绑定定义、模板、技能与源版本。
- 裁决：新增未应用 Proposal，包含 definition/template/skill 版本、源状态版本、差异、影响、检查与目标仓库。确认绑定 proposal_id + proposal_version + source_version + 用户身份 + 操作范围/有效期。来源变更作废旧确认；重复确认仅返回同一应用结果。执行使用已确认内容快照。未确认不产生真实项目工件。
- 理由：可审查方案使授权具体化；运行中升级不能静默改变已批准内容。
- 五层关联：L1 `preview-confirm-apply` / `definition-immutability`；L2 对话/Agent；L3 `project-proposal` / `confirmation`；L4 “版本绑定确认”；L5 Proposal、DefinitionSnapshot、SourceVersion、ApprovalRecord。

## R-012 — 一般确认与生产合并/发布批准分开

- 来源：§1.1，L7；§9，L1203–1216；原文没有生产发布目标。
- 裁决：本项目范围是脚手架、仓库初始化与 CI 首跑，不直接扩展为参考 harness 的生产交付平台。普通项目方案确认只授权列明的文件/仓库/CI 操作；若后续增加合并或生产发布，必须新增独立、版本/环境/动作绑定的批准契约与受控执行器，不能复用普通“接受”。
- 理由：参考契约可提供审批方法，不能隐含扩大产品范围与凭据权限。
- 五层关联：L1 `approval-scope`；L2 治理；L3 `execution-authorization`；L4 “项目创建范围与扩展边界”；L5 ActionScope、Environment、ArtifactHash、ApprovalType。

## R-013 — Continue-As-New 迁移整个业务检查点

- 来源：§5.2，L369–381；§5.3，L536–554；§4.1，L147–158。
- 缺陷：Description 被遗漏，消息队列/handler 完成/摘要版本/去重身份/操作记录/当前等待和 Query 状态迁移未定义。只有 State 指针不足以说明全部恢复契约。
- 裁决：新执行保留业务 Run ID、当前阶段、不可变定义引用、待批准/待核实操作、摘要与原文引用、去重账本、源版本及 resume_to。迁移前所有 Update handler 完成，并处理或持久登记队列；不得从 handler 内直接 Continue-As-New。新执行 Query 从迁移检查点初始化，不退回 INTAKE。
- 理由：历史切段不能丢失业务身份、输入和已批准状态。
- 五层关联：L1 `checkpoint-completeness`；L2 编排；L3 `continue-as-new`；L4 “检查点与 handler 排空”；L5 Checkpoint、PendingCommand、SummaryVersion、BusinessRunID。

## R-014 — 原始对话不能被摘要覆盖

- 来源：§6.3，L700、L712–727；§13.4，L1309。
- 缺陷：Rewrite 删除早期原文；“超过 20 轮”与示例 `len(messages)>20` 不一致；模型摘要没有保留授权的确定性机制。
- 裁决：原消息 append-only 保存，摘要作为独立派生版本记录 source message range/hash；工作上下文可替换早期消息为摘要，原文仍可检索。阈值明确使用对话轮次或消息数，不混称。权限、批准、预算、完成条件、阻塞原因与安全约束来自结构化权威状态，不靠模型摘要继承。
- 理由：摘要是有损派生物，不能成为授权真相或销毁审计证据。
- 五层关联：L1 `raw-history-preservation` / `authority-outside-summary`；L2 对话；L3 `rolling-summary`；L4 “摘要派生而不重写历史”；L5 MessageRange、Summary、SourceHash、AuthorityState。

## R-015 — 上下文预算分层执行，不能直接按字节截末尾

- 来源：§2.2，L58–61；§6.2，L643、L659–695。
- 缺陷：`len` 是字节长度，`1 token≈4 字符` 对中文与混合文本不可靠；`text[:maxChars]` 可能切坏 UTF-8、截掉必要约束或完整当前请求。固定层优先级与最终前缀截断的实际效果没有契约。
- 裁决：固定规则、结构化权限/预算/完成条件、当前 Run 身份与完整当前消息优先；裁剪检索、引用描述、旧消息，再在预算内保留版本化摘要。预留输出/协议开销；优先使用指定模型 tokenizer，无 tokenizer 时明确使用保守估计/上界。必要层本身超限则拒绝本次模型调用，不静默丢需求。存储原文不随请求裁剪。
- 理由：显示 token 估计不是预算 enforcement；字符切片更不是语义层级裁剪。
- 五层关联：L1 `hard-context-budget`；L2 对话/模型接入；L3 `context-builder`；L4 “分层预算与超限拒绝”；L5 ContextLayer、BudgetReservation、TokenEstimate、ContextReference。

## R-016 — ContextPropagator 示例不完整，传播只承载非敏感标识

- 来源：§6.4，L735–764。
- 缺陷：Workflow 两个接口方法为空；HeaderReader/Get 与 payload 解码示例未匹配锁定 SDK 的实际类型，未显示 client/worker 注册。不能声称示例已完成 Client→Workflow→Activity 的链路。
- 裁决：以锁定 SDK 的接口签名、converter 编解码、四向传播和双端注册作为验收；业务身份仍有显式契约字段，不依赖隐藏 context 才能正确执行。传播只允许 trace/run/tenant 等已校验非敏感标识，不传播 token 或授权结论。
- 理由：header 是上下文运输，不能代替身份校验或授权账本。
- 五层关联：L1 `non-secret-propagation`；L2 可观测性；L3 `context-propagation`；L4 “传播与身份契约分工”；L5 TraceContext、HeaderSchema、PropagationTest。

## R-017 — 文件标记必须关联定义、参数与 manifest

- 来源：§7.1，L778–784、L796–802；§7.2，L867–874、L899–915。
- 缺陷：标记文件存在即成功，没有模板/配置 hash；文件与 marker 写入多处忽略错误；相同 Skill 改参数后仍可被旧 marker 跳过。工作区路径也没有规范化、符号链接与逃逸检查。
- 裁决：写入隔离 staging/workspace，校验所有文件与命令返回值，完成后原子提交 manifest；manifest 绑定业务操作、批准版本、模板/技能参数 hash 和文件 hash。路径必须规范化并限制 Run 范围，拒绝绝对路径、`..` 与非授权符号链接。恢复按 manifest 核实，不只看 marker。
- 理由：单文件标记不能证明一组副作用完整且对应当前要求。
- 五层关联：L1 `workspace-confinement` / `artifact-integrity`；L2 Runtime；L3 `scaffold` / `skill-application`；L4 “原子工件包”；L5 Workspace、Manifest、ParameterHash、FileHash。

## R-018 — JSONL 与快照的持久化边界明确

- 来源：§1.2，L21；§6.1，L600–637；§4.3，L183。
- 缺陷：原文使用 0644、忽略 JSON/scanner 错误、无 fsync、并发锁、截断记录检测、索引及消息/状态一致性策略。文件在 Worker 本机不等于跨 Worker 可恢复；ReadRecent 实际扫描全文件。
- 裁决：首版文件存储保留，但限定单写者或明确进程锁/CAS；原始消息稳定身份去重，快照原子替换，错误 fail closed。存储目录权限、备份与 retention 明确；Worker 可迁移时使用共享持久卷或独立存储服务；外部大对象在 Workflow 内仅保留稳定引用。记录部分提交的修复策略，不承诺不存在的跨文件事务。
- 理由：Temporal 持久历史不会自动持久化 Worker 本地文件。
- 五层关联：L1 `persistent-reference-accessibility`；L2 对话/运维；L3 `storage` / `recovery`；L4 “首版文件存储部署限制”；L5 SnapshotVersion、StorageRef、WriterIdentity、RetentionPolicy。

## R-019 — 密钥不得出现在 git remote 与进程参数

- 来源：§7.3，L962–965；§9.1，L1207–1210。
- 缺陷：把 token 放进 remote URL 会写入 `.git/config`，也进入进程参数；与“不落盘”及“不写日志”明确冲突。`.gitignore` 不阻止凭据已写入受版本控制文件。
- 裁决：执行器通过可信 HTTP header、受控 credential helper/短生命周期 askpass 等机制使用凭据，禁止带 token 的 remote URL/命令行。生成工作区、日志、Workflow payload、JSONL、子进程环境和 ZIP 都扫描凭据/秘密边界；日志错误须脱敏。权限 scope 按实际 API/传输方式取最小集合，不硬要求不必要 scope。
- 理由：凭据由环境读取，只说明入口；后续传播仍可能泄漏。
- 五层关联：L1 `secret-boundary`；L2 治理/仓库；L3 `credential-use` / `secret-verification`；L4 “可信执行器持钥”；L5 SecretReference、AllowedEnvironment、RedactionPolicy、SecretCheck。

## R-020 — Skill post hook 不是任意 Shell 授权

- 来源：§2.3，L71–81；§7.2，L907–910；§9.2，L1212–1216。
- 裁决：Skill 版本、支持栈、参数 Schema、写路径与固定命令清单同时校验。`post hook` 使用确定参数数组，限定 cwd、环境、工具白名单、超时与资源预算，不接受模型/用户任意 shell；生成代码测试也不能继承执行器原始密钥。数据库接入、JWT/session 与 logging 的支持参数和默认值在功能契约中完整定义。
- 理由：可复用 Skill 不自动具备扩大工具权限的资格。
- 五层关联：L1 `least-tool-authority`；L2 Runtime/技能；L3 `skill-contract` / `subprocess-execution`；L4 “模板能力与工具白名单”；L5 ToolGrant、SkillVersion、CommandSpec、EnvironmentFilter。

## R-021 — Activity timeout 与业务失败处理纠正

- 来源：§8.1，L1159–1161；§8.2，L1166–1179；§5.2，L440–444 等错误直接 return。
- 缺陷：声称 StartToClose 是唯一必须值，忽略 ScheduleToClose 可选替代；业务失败说明“重新决策”但代码直接终止 Workflow。网络超时也不区分安全读取与结果未知写入。
- 裁决：至少设置 StartToClose 或 ScheduleToClose，分别约束单次尝试与总等待；长任务有 heartbeat/cancellation；读失败按有限重试，参数/权限永久失败不重试，业务失败进入明确诊断/修订或终止，未知写按 R-006 核实。无限重试与无限模型修正均禁止。
- 理由：错误类型决定重试权限；超时不是回滚证明。
- 五层关联：L1 `finite-retries`；L2 编排/Runtime；L3 `failure-classification`；L4 “尝试/总时限与恢复类别”；L5 ErrorClass、RetryPolicy、TimeoutBudget、PartialSideEffects。

## R-022 — 状态机补齐暂停、阻塞、终止与重做

- 来源：§4.1，L148–158；§5.2，L429–432、L486–530；§13，L1306–1313。
- 缺陷：仅 CurrentStep 字符串与 PendingUserInput，没有合法转移、resume_to、取消、审批超时或终止不可恢复语义。
- 裁决：定义项目创建专用状态机与默认拒绝转移；WAITING_HUMAN/BLOCKED 保存原阶段，恢复复校当前版本、权限、依赖与结果；取消终止不声称副作用已撤销。DONE/FAILED 等终止 Run 不复活，重做创建显式关联的新 Run。不要为“像参考”而强制复制当前业务不需要的生产部署状态。
- 理由：恢复语义与完成事实必须能确定性验证。
- 五层关联：L1 `terminal-run-integrity`；L2 编排；L3 `run-lifecycle`；L4 “项目创建状态机”；L5 State、Event、ResumeTo、SuccessorRun。

## R-023 — Temporal 与 local 的成功事实分别表达

- 来源：§1.1，L7；§1.2，L13–14；§12，L1296–1301；§13.2、13.7，L1307、L1312。
- 裁决：本产品持久执行基线明确为 Temporal。local 如保留，只是同阶段逻辑的本地运行能力，必须显式 execution_mode；文件快照/进程重启后的人工恢复不等于 Temporal history/replay/Activity 调度。选择 Temporal 却未连接时拒绝启动，不静默降级。依赖缺配置展示 BLOCKED 或未配置，禁止虚构 repo/pipeline/模型调用成功。
- 理由：逻辑复用与持久执行能力不是同一验收事实。
- 五层关联：L1 `execution-mode-truthfulness`；L2 运维/前端；L3 `startup-readiness` / `mode-reporting`；L4 “持久基线与本地辅助模式”；L5 ExecutionMode、DependencyReadiness、EvidenceType。

## R-024 — 可观测性、验收与实施计划必须区分目标和现状

- 来源：§10，L1239–1252；§12，L1294–1301；§13，L1306–1313。
- 裁决：每个验收关联输入、故障注入点、权威输出及证据文件；指标有事件来源、单位、成功/失败判定与重试计数规则。Query 的 Workflow 阶段状态与外部快照/Activity 详细进度明确定义同步边界；前端不把未完成请求显示为成功。周次是计划而非实现证明；测试脚本存在、测试通过、真实第三方通过、浏览器端到端通过分别登记。
- 理由：文档重构不能借已有代码或示例把计划改写成已交付产品。
- 五层关联：L1 `evidence-qualified-status`；L2 可观测性/前端/运维；L3 `status-stream` / `acceptance-evidence`；L4 “计划与运行证据分离”；L5 AuditEvent、MetricDefinition、AcceptanceCase、EvidenceRecord。

## 规范与证据优先关系

本次已裁决的当前项目不变量/功能契约/ADR为新设计的规范入口；原设计作为原始证据保留，问题不得因原文示例仍在而重新获得优先权。参考 harness 仅用于借鉴结构、契约分层和表达方法；其 52 条不变量、63 份功能、82 条 ADR 与生产权限设定不能作为当前项目自动冻结的规则。实现与测试状态仅证明当时的具体行为，差异须在实现状态中登记。

SDK 技术事实依据本地锁定 `go.temporal.io/sdk@v1.38.0` 的源码及官方接口文档：[Workflow API](https://pkg.go.dev/go.temporal.io/sdk@v1.38.0/workflow)、[Client API](https://pkg.go.dev/go.temporal.io/sdk@v1.38.0/client)、[ActivityOptions](https://pkg.go.dev/go.temporal.io/sdk@v1.38.0/internal#ActivityOptions)。产品选型版本应由当前项目 ADR 明确，不因该历史实现版本而自动固定。

## 跨文档复核后的补充裁决

- VERIFY 的原批准快照安全重试回 IMPLEMENT，内容/范围修订显式回 PLAN；不把需要新批准的修复藏在自动重试里。
- execution temporal/local、decision model/rules、delivery gitlab/local 三轴独立；仅delivery local可把远程验证标不适用，缺模型不自动降rules。
- source_version 仅随方案/授权范围变化，state_sequence记录普通进度；proposal_version内容不可变。服务端签发30分钟期限（1分钟至24小时可配置），接受及首次应用核验，客户端不能延期。期内开始的固定链按原范围继续，新的范围/撤销/来源变化仍需复校，已核实事实保留。
- 首版数据库Skill只生成入口及隔离测试；客户DB真实迁移不在普通创建授权范围。file日志定义具体路径、大小/保留和失败策略，尚未实现的部分单独登记。

这些补充进入当前五层功能及ADR，不改原文，也不以保留代码当前行为覆盖新规范。
