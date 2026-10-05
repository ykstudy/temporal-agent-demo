# 契约 1.3.0：固定 Git、公开交付证据与修复候选

2026-10-02，按 [ADR-050](../../../10-decisions/ADR-050-controlled-delivery-evidence.md) 发布。三份 Schema 是完整发布源，wire 仍为 schema_version=2；1.0、1.1、1.2 原字节不变。沿用原资源限制、JCS、精确整数、动作顺序、批准、预算和模型来源规则。本发布解决 S2 真实受控 Git 命令及远程/包/修复证据缺失；发布和 mock 检查不等于 live GitLab 或 S2 退出。

## 兼容与冻结定义

Run 按自己的 contract_version 校验完整闭包，新发布不能改写旧 Run。AgentDefinition 新增必需 GoModulePolicy；RepositoryPolicy 新增必需 GitExecutionPolicy；ProjectPlan 新增必需 nullable verification_candidate_ref，普通计划为 null。S2AuditBundle 新增必需 remote_receipts、package_receipts、verification_candidates、verification_reports 数组和 nullable verification_report。旧发布没有这些字段，旧版本重序列化不得补 null/空数组改变身份；生成器从各版 Schema 自动派生字段兼容行为，新版仍严格要求完整字段。

GoModulePolicy 冻结 proxy_url（https://proxy.golang.org 或 https://goproxy.cn）、sumdb=sum.golang.org、download_mode=proxy_only、allow_direct=false、inherit_environment=false。新 3.2 定义选择 goproxy.cn；Go 使用内置 sum.golang.org 验签身份，可通过代理的 sumdb 服务获取签名数据库。不得增加 direct fallback、关闭验签、继承服务 GOPROXY/GOSUMDB 或在失败时换代理。旧 1.2 保留其原固定 proxy.golang.org，不受新定义影响。CGO_ENABLED=0、GOTOOLCHAIN=local、GOENV/GOWORK=off 及 Run 隔离不变。

1.3受控Go进程的必要系统只读资料包括`/etc/protocols`、`/etc/services`两份公共网络协议/服务映射；macOS精确解析到`/private/etc/`下同名文件。纯Go SQLite的modernc运行库会在初始化时读取它们。仅开放这两份常量文件的只读访问，不开放`/etc`目录内容或任何系统写权限；未选数据库的Go进程仍受同一1.3系统运行边界约束。旧1.2沙箱保持原权限，运行时不得根据EPERM动态增加路径或无沙箱重试。实际冷缓存SQLite两ORM验证与禁止任意系统文件读取/写入的边界探针分别登记。

GitExecutionPolicy.policy_id=controlled_git_v1，六条命令各出现一次，固定 executable=git、cwd_scope=run_workspace、environment_policy=isolated_git，单次 timeout_seconds/max_attempts 与 BudgetPolicy 取最小上限。Agent required_tools 及 Snapshot tool_bindings 必须含 git 和 git_remote_http 的版本、平台与非空二进制 SHA256；验证真实二进制摘要后才允许启动，不能按当前 PATH 任意选择工具。Go post_commands 仍只有三条 Go 命令，仅在 VERIFY 执行。

| command_id | 固定 arguments_template |
| --- | --- |
| git.init | `[-c,init.templateDir=,init,--initial-branch,{ref}]` |
| git.add | `[-c,core.hooksPath=/dev/null,add,--all,--,.]` |
| git.commit | `[-c,core.hooksPath=/dev/null,-c,commit.gpgSign=false,commit,--no-verify,-m,{operation_marker}]` |
| git.head | `[rev-parse,--verify,HEAD]` |
| git.tree | `[ls-tree,-r,-z,--full-tree,HEAD]` |
| git.push | `[-c,core.hooksPath=/dev/null,-c,http.followRedirects=false,push,--porcelain,-o,ci.skip,{remote_url},HEAD:refs/heads/{ref}]` |

占位值只由控制平面从原请求解析：ref 固定批准 main；operation_marker=`agentstudio:<operation_id>:<request_hash>`；remote_url 为准确批准实例/namespace/project 的无凭据 `.git` HTTPS/已明示 HTTP 地址。禁止模型或用户提供 argv、shell、额外配置、force 或 refspec。git.add 前后核对独占隔离候选目录恰好含全部批准文件；git.tree 的 blob SHA1、路径、模式和文件字节必须与原 manifest 一致，不能因 `--all` 带入其他文件。仅在该 Run 的私有准备副本生成 `.git`，不污染批准源码或包。提交作者固定 Agent Studio、邮箱 agent@agentstudio.invalid，提交/作者时间取原 ExecutionRequest.created_at，message 为原 operation_marker；固定身份确保恢复复用原 SHA，不制作第二个提交。

进程使用过滤环境、隔离 HOME/临时目录、禁用系统/全局 git 配置、hooks、签名、交互凭据与重定向。凭据仅通过临时进程环境的准确目标 HTTP Authorization header 提供，禁止写入配置、remote URL、argv、工件、回执和日志；header 不继承给 Go 或模型。沙箱仅允许冻结 git/HTTP helper、必要系统库和 Run 目录，网络仅原 GitLab 目标；不可用不能无沙箱降级。测试使用隔离本地 HTTP/远端仓库，不向 live 第三方写入。

Git 工具预留仍为 kind=tool，scope=`tool:<command_id>`，绑定原 push_repository 请求和父 action attempt；不能在其他动作借用 git 权限。固定后置 Go 工具仍只在 verify 使用。每次真实 git 启动先持久预留，回执明确固定 tool/实际解析 arguments/environment/原 manifest/时间/exit/status/脱敏有界 output。输出摘要规则与 1.2 相同。git.head/tree 的真实 stdout 是原本地 SHA/tree 来源，不能从模型或当前目录名称推断。已有预留缺回执先 unknown，不盲重跑 push。

## 准确远程操作与完整证据

RemoteExecutionReceipt 复用准确 Run/source/definition/execution/operation/attempt/request/approval/proposal/plan/workspace 绑定，含 action_type、target、manifest_ref、nullable project/commit/ci_config_sha256/pipeline/pipeline_variables、jobs、tool_receipt_refs、observations、write_sent、起止时间、status 和 receipt_hash。它是实际执行器的不可变观察记录，不是模型建议或批准来源。失败/unknown 只填实际已知事实，缺项为 null/空数组；不得合成观察补完整。后续核实可追加新回执，原 unknown 回执和原消费结算不改写；成功归属必须唯一并复用原 operation/attempt。

RemoteProjectObservation 保存实际 project ID、namespace ID/path、project path/full path、visibility、ownership marker、empty_repository、nullable default_branch。create 不初始化 README，成功仅证明项目身份及空仓库；批准 target.ref 在这一阶段是预定分支，不代表分支存在。只有 push/CI 另行观察准确 ref/SHA 才有分支证据。

RemoteCommitObservation 保存准确 project/SHA/parents/message 和全文件 RemoteFileEvidence（path/size/SHA256/git_blob_sha1）；推送前复核空分支或批准基线，普通 push 禁止强制覆盖。推送完成与未知核实均 GET 原 ref、原 SHA/tree 及全部 manifest 文件，要求路径集合和字节一一对应，CI 配置 SHA256 也匹配原批准文件。本地 ToolReceipt 与原 manifest 证明期待 SHA；远端任意提交、仅匹配 message 或 API 返回 200 都不够。

Git push 的 `ci.skip` 防止未绑定后续 CI 操作的自动首跑被误认或重复启动；trigger_ci 在准入后通过明确变量 `AGENTSTUDIO_OPERATION_ID` 和 `AGENTSTUDIO_REQUEST_HASH` 绑定原操作。触发前查准确 project/ref/SHA 的既有流水线，并逐一读变量；只有唯一同时匹配这两个值的流水线可复用。未知触发不得再 POST；列表为零、未及时可见、任意同 SHA 首条或多条归属匹配均不证明未执行。查询须完整、有界分页；截断列表不能当作唯一性。记录已知 pipeline ID 后优先按原 ID 查询，并复核 SHA/ref/变量，不能另挑绿色流水线。

RemotePipelineObservation 保存 project/pipeline ID、ref、commit SHA、实际 status/source；RemotePipelineVariables 单独保存实际读回的原 operation_id/request_hash。CI 成功还要求完整 CIJobEvidence：project/pipeline/job ID、name/ref/commit/status/allow_failure、实际下载工件。必需 test/build 各唯一、success 且 allow_failure=false；coverage.out 须解析实际 Go coverage 内容，bin/server 须核对受支持 ELF/Mach-O 可执行文件。RemoteArtifactEvidence 带 path、原字节引用、byte_count/SHA256、format、validated。缺工件、skipped/manual/canceled、错源或未知结果不得通过。

每个 HTTP GET 都先预留 ci_poll 或 reconciliation_read，再保留 RemoteReadObservation：准确路径/方法、HTTP状态（网络无响应为null）、预算引用、起止时间、kind、严格类型 payload/facts_hash。payload 是白名单中持久化的观察事实，摘要为其完整 JCS；不是包含潜在秘密的原始响应/日志摘要。网络失败和不可用响应 payload/facts_hash=null，不能把404转成成功事实。查询集合、下载和逐文件核实均消费原 scope 预算，原绝对窗口跨重启/source 改变不重置。Unknown 只读核实可保留历史 source，不能产生当前 source 新授权或恢复终止 Run。

公开控制 intent 新增 `reconcile`，仅绑定 Run 引用与 command_id，不带用户 operation/目标/批准参数。服务从已登记 sent/unknown 原请求选择只读核实，逐读消费原预算；终态也可记录既有副作用事实，但不能改变终态、恢复批准或发起新写。reconcile 不是 action 授权；GET 不触发远程核实。旧1.2不接受该intent，legacy适配只能对支持的新Control版本开放。

## 修复候选和可复核交付包

VerificationCandidate 保存原执行/尝试/批准全绑定、input_manifest_ref、tool_receipt_refs、完整候选 files、changed_paths、total_bytes、created_at/candidate_hash。每个文件带 UTF8 content、size、SHA256；按 path 排序且唯一，changed_paths 必须与原 manifest 的新增/修改/删除完整相等；删除体现为候选完整集合缺少原路径。不得接受逃逸路径、符号链接、秘密、超过冻结资源限额或未获准写路径。候选只证明隔离区实际差异，绝不是批准 manifest，也不能直接应用。

采用修复候选须由控制平面核对原完整请求、真实工具回执、候选字节与摘要，然后增加 source、生成新 ProjectPlan/Proposal；新 plan.verification_candidate_ref 精确引用原 source 的不可变候选，并展示完整差异供 P1。原 source 下已经生成的 source manifest 不回写，原批准和未执行请求作废。普通计划字段为 null。候选生成和采用均不得绕过模型来源、Schema、技能/路径范围、权限、预算及批准门禁。

PackageReceipt 保存全执行来源、manifest_ref、nullable package_ref/package_sha256、package_bytes/file_count、起止/status/receipt_hash。成功必须有真实原 ZIP 字节（至多4MiB）与完整文件核验：源文件集合/UTF8字节与准确 manifest 和批准一致，无额外秘密/链接/外部路径；允许规范冻结的附加 JCS manifest 元数据。包身份由原请求派生，原字节发布不可覆盖。包成功不免除产品完成门，不把打包命令返回0当作DONE。

S2AuditBundle 同时公开新回执/候选、当前 nullable verification_report 与全部 immutable verification_reports。VerificationReport/RequiredCheck 沿原完整结构，具体 required_checks 必须真实执行并绑定证据引用；模型 completion_hint 只能触发检查。数组中的synthetic/测试事实不得混作实际产品 Run 回执。GET仍只读且不冒充完整v2 Run。

## 固定摘要、对象引用和资源界

| 主体 | 顶层只排除 | ObjectRef |
| --- | --- | --- |
| RemoteExecutionReceipt / PackageReceipt | receipt_hash | evidence / receipt_id / 1 / source_version |
| VerificationCandidate | candidate_hash | verification_candidate / candidate_id / 1 / 原source_version |
| VerificationReport | report_hash | verification_report / report_id / report_version / source_version（沿原契约） |
| 原 ZIP | 不使用 JCS，SHA256 原字节 | package / package_id / 1 / 原source_version |
| CI 原工件 | 不使用 JCS，SHA256 原字节 | ci_artifact / 稳定工件ID / 1 / 原source_version |

新对象只单向引用已冻结请求、批准、manifest、工具回执和原字节；不反向包含引用自身的 hash。raw object ID 必须不含内容摘要循环；先确定原字节，再登记原体及引用。CI单文件至多16MiB、包至多4MiB，公开JSON仍16MiB/深度128/节点500000/数字literal128bytes/指数abs<=400；普通artifact预算沿原Frozen BudgetPolicy。回执和候选数组必须在当前有界Run容量内，不得悄悄增预算或截断证据。

验收分别覆盖发布源/完整Schema、旧版逐字节及hash兼容、假工具/错argv/越权路径/错source/伪回执拒绝、隔离真实git与mock网络、原身份失响应恢复、CI歧义/缺job或报告、候选重批和完整包；live第三方、浏览器及完整S2实际job另记。
