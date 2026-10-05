# S2 受控 GitLab 执行分片

2026-10-03收尾：当前已接入1.3固定Git推送，正常推送和失响应后只读核实有本机真实Git定向证据；CI尚未接入。用户要求结束本轮，完整S2未验收。下方初始建仓分片描述保留历史边界，新推送证据与中断检查见[实现状态](IMPLEMENTATION-STATUS.md)。

本说明落实 [项目身份与创建](../../design/04-integrations/features/gitlab-project.md#输出)、[外部结果未知核实](../../design/04-integrations/features/unknown-results.md#输出) 与发布 1.2 的完整执行请求、预算账本；不是新增批准来源，也不改变冻结规则。初始实现范围仅 `create_repository`，推送、CI 与实际完整 job 验收分别登记。

## 固定Git推送接入规则

按1.3冻结的六条Git命令在Run私有副本执行，逐条预留工具预算；批准目录和包不包含`.git`。参数只替换冻结占位符，包括`HEAD:refs/heads/{ref}`内部的ref，不接受额外argv。每条命令前后核对完整批准文件集合，实际head/tree输出给出唯一SHA、普通文件模式和blob映射；只在这些事实齐全后进入push发送边界。

Git及HTTP helper按冻结二进制摘要核验。过滤环境关闭系统/全局配置、hooks、签名、交互凭据、代理继承和重定向；认证仅在push进程的准确目标HTTP header环境中出现，原token和编码认证值都从输出中剔除。当前3.2发布绑定darwin_amd64；该平台沙箱只允许运行准确Git/helper、读取必要动态库和Run副本。Seatbelt的网络规则仅支持localhost或通配主机，故内核限制目标TCP端口，本机目标另限localhost；准确实例和项目路径由不可变argv、禁止重定向、隔离配置及URL限定认证共同约束，不宣称内核逐IP白名单。其他平台没有匹配发布和沙箱时明确失败，不无隔离降级。

工具调用的原请求、固定命令、真实核验工具身份及开始边界先持久保存。进程结果缺失时，只能据这份原记录追加unknown回执，保留null退出码、未知用量和不可用输出，不能从当前工具/模板猜造历史。恢复不得再次执行push；必须从先前真实head/tree和原远端ref/SHA/完整文件读取判定。若最后push工具返回失败或丢结果，而只读观察证明原SHA和完整批准字节已存在，可以追加远程成功事实；原工具失败/unknown回执和消费不改写。缺任一必需历史来源仍阻塞。

## 1.3完整回执接入规则

1.3建仓与原身份核实必须持久保存完整`RemoteExecutionReceipt`，私有调度结果仅作为可重算索引，不能单独证明远程成功。每个实际GET的完整`RemoteReadObservation`与预算结算同时保存，执行回执引用这些原始观察；失败后新观察追加新回执，旧unknown回执和原消费不覆盖。回执、操作证据引用、当前索引在同一次Store更新提交，恢复读取重新校验完整来源。

请求的前置namespace/项目读取发生在写准入之前，因此读时间可早于父action预留，但必须晚于原ExecutionRequest签发、绑定同一原请求，并受自己的读预算期限约束。1.3未定义namespace观察payload，前置namespace读取只保存实际HTTP状态和`kind=unavailable`、null payload/hash；不能编造project事实，成功项目GET仍需完整记录namespace归属。该unavailable表示没有可用的公开类型事实，不把HTTP 200改写成网络失败。

写调用前持久登记dispatch边界；`write_sent`表明该调用进入可能发送的边界，不承诺远端已提交。边界后的错误或进程丢失保持unknown，只有准确原项目读回才能追加成功事实。读取缺必要字段、含凭据或不符合公开Schema时不持久化响应正文，也不制造成功payload。终态只读核实不恢复执行权。本段是接入要求；实际检查结果另记实现状态。

## 接口与准入

`ExecuteControlGitLab(ctx, runID, request)` 消费已登记的完整 `ExecutionRequest`。写入之前核对请求原文与 hash、Run / source / 模式、完整批准目标和当前可信 GitLab 配置，精确查询 namespace 与项目路径。真正 POST 前调用共享 `beginControlExecution`，在同一持久事务重跑治理并将原 operation 置为 sent，预留唯一 action attempt。其他未实现动作在任何网络或预算预留之前返回明确错误。

`ReconcileControlGitLab(ctx, runID, request)` 仅处理原 sent / unknown operation；它不调用创建接口，不生成新执行请求，不重置原操作、源版本或 deadline。每次 HTTP 只读调用都先登记 `reconciliation_read:<operation_id>` 预留并按实际调用结算。账本保留该 operation 第一次核实窗口的绝对期限，source 变化不会续期。历史 source 的只读核实仍绑定原请求，终止或权限撤销的 Run 可登记已经发生的结果但不能因此恢复执行。

2026-10-03接通1.3的显式`reconcile`命令：命令只绑定Run及稳定ID，拒绝额外提案/目标参数；确定性调度选择原sent/unknown请求。尚有ActiveStep时拒绝，不能抢先结算活跃请求。成功不返回Execute授权；终态保留，成功重复命令复用回执而不再次GET。新测试的上游workspace/Go也真实执行，建仓与失响应由HTTP mock提供；后续push/CI及真实Temporal仍未验收。

## 创建与可归属结果

创建 description 固定为 `agentstudio:<operation_id>:<request_hash>`，直接由不可变请求导出。它是公开非秘密归属标识，不是凭据或独立授权。查询同时匹配实例、准确 namespace ID / full_path、项目 path / path_with_namespace、visibility 与完整标识；同名、不同 namespace、其他操作或仅相似 hash 均不能复用。任意查询失败不推断目标不存在。创建前必须成功读取准确 namespace，准确项目路径返回 404 才可尝试创建；POST 仍是受服务端唯一性保护的新项目创建，绝不更新既有项目。

POST 使用批准 namespace / project path / visibility，`initialize_with_readme=false`；不会制造未批准的 README 或初始提交。GitLab 空仓库可能尚无 default_branch。创建结果只证明项目容器身份与归属，批准 ref 是后续推送目标，不能据此声称远端分支或 commit 已存在；如观察到非空默认分支，必须与批准 ref 相同。推送阶段必须另外核对准确分支、SHA 和完整 manifest。此区别依据 [GitLab Projects API](https://docs.gitlab.com/api/projects/)：指定创建时的 default_branch 依赖 initialize_with_readme，不能通过额外文件绕过批准清单。

外部写返回后，再按确切项目身份 GET 验证对象。响应丢失、超时、返回身份不符或写后查询失败均保留原 sent 身份并转 unknown / BLOCKED；不将失败或未知费用释放为未执行。核实只有完整观察匹配时才将 operation 置 verified，记录绑定原 action attempt 的完整 `CanonicalActionResult` 和摘要。已结算 unknown 的原尝试永久保留 unknown，后续只读核实有自己的消耗。没有归属证据、404 或查询不可用均维持 BLOCKED；本分片不自动证明“未执行”并开放重试。

## 2026-10-04 CI 接入实施约束

用户重新授权继续第2阶段。本次沿用已发布1.3.0，不修改冻结Schema或扩大P1范围。`trigger_ci`消费已核实push的准确project/ref/SHA和CI配置hash；发送前完整有界分页查询原操作的流水线变量，缺页、重复身份或多条匹配均阻塞。POST仅在持久准入及dispatch后发送一次，响应丢失保留原operation，后续只读核实；登记pipeline ID后只查询该ID。

每次CI GET（含分页、变量、job和工件下载）预留`ci_poll`；显式核实使用原operation的`reconciliation_read`。窗口及累计deadline跨重启不重置。只保留白名单公开事实，不归档原变量列表或远程日志。未终态按有界间隔轮询，取消/暂停停止等待；终态核实不恢复执行权。

成功须有实际project/branch/commit/全文件观察、唯一test/build作业的准确pipeline/ref/SHA、success且allow_failure=false，以及下载的原coverage.out和bin/server。产品解析完整Go coverage行并读取ELF/Mach-O结构，工件原字节在Run目录不可变保存并按公开引用读取；完成门再次读取、核对hash/大小/格式。缺回执、缺字节或损坏工件均不能DONE。API依据：[Pipelines](https://docs.gitlab.com/api/pipelines/)、[Jobs](https://docs.gitlab.com/api/jobs/)、[Job Artifacts](https://docs.gitlab.com/api/job_artifacts/)。不跟随跨目标重定向，遇到CDN跳转保留未就绪。

本分片验证与S2总验收分开：真实本机Git及明确mock GitLab/CI不代表live Runner；公开1.3主线、C07–C10和真实Temporal仍须各自取得证据。实施结果登记IMPLEMENTATION-STATUS，不提前切换默认发布或关闭G04/G05。

## 2026-10-04 受控公开创建验证入口

服务注册不可变3.2.0发布，显式提供完整definition_id/version/hash才能创建contract1.3.0受控Run。该入口从创建时建立完整预算及S2AuditBundle，原文、建议、校验、执行均沿同一受控处理链；不能经legacy授权执行1.3。默认3.0.1/1.1及旧Run恢复保持原边界，1.2发布仍不可用于公开新建。本阶段的公开验收使用显式3.2定义，不将旧主线结果计作S2通过，也不提前进入S3或开放完整v2快照。

## 验证边界

单元/HTTP mock 验证必须区分：上游调度的 synthetic 工作区和工具 receipt 仅为该执行分片的前置测试数据；实际 create POST、权威 GET 与读写次数是本分片被验证行为。需要覆盖目标冲突、401 / 403 / 429 / 5xx、响应丢失后原身份核实、重复执行不增加 POST、核实期限 / 次数上限、源或权限变化、终止后仅登记结果、服务重启持久身份及无秘密输出。mock 不等于 live GitLab，不等于推送 / CI 已完成，不等于 S2 退出。

现有 [批准工件提交与推送](../../design/04-integrations/features/repository-push.md#输出) 要求固定 git 命令和本地提交身份。后续 push 不能直接替换为 GitLab commits API，除非先作明确规范裁决；本创建分片不扩大 shell 或工具权限。
