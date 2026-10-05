# 项目创建 Agent 后端


## 当前运行观察与离线恢复

完整1.4创建 Run 另提供只读 runtime/1.0.0：[发布协议](../design/contracts/runtime/1.0.0/README.md)。`GET /api/runtime/live` 仅表示进程存活；`GET /api/runtime/ready` 以实际存储读写与 fsync、Temporal/namespace、当前 Worker 两类 poller、Go/沙箱决定 200/503。model/GitLab 配置存在只标 `configured_unverified`，不证明真实第三方可用。旧 `/api/health` 是兼容响应，不能作为完整 readiness。

`GET /api/v2/runs/{id}/runtime` 显示准确 Workflow 执行段检查点、实际 Activity 尝试/心跳、冻结累计期限、失败归因与恢复条件。`GET /api/v2/runs/{id}/audit` 返回追加 hash 链；`GET /api/runtime/metrics` 按首次终态、真实 Temporal attempt、正式失败报告和去重业务需求计量。暂停/取消/进度查询不虚增失败或需求。未采集的旧历史明确不可用，不回填成功。完整对象/原始消息仍按原公开接口读取。

离线备份要求原服务已完全停止；命令取得同一数据目录独占锁。目标必须是新的隔离目录，拒绝符号链接和特殊文件，复制整个数据目录并写 `SHA256SUMS`（不复制运行锁、不给凭据增加来源）。在仓库根目录运行：

```sh
go -C backend run ./cmd/statebackup --source /absolute/agent-data --destination /absolute/new-offline-copy
go -C backend run ./cmd/statebackup --verify /absolute/new-offline-copy --execution-mode local
```

Temporal 副本选 `--execution-mode temporal`。verify 先检查完整文件集合/字节，再调用产品自身启动恢复与冻结来源检查；它不连接 Temporal、不执行 Run。不要对保留证据的唯一副本运行恢复：恢复可能追加 unknown/预算结算事实。实际恢复服务还须配置原执行模式及原 Temporal namespace/task queue；未完成尝试保留原身份，未知外部写只能核实。缺少冻结定义依赖即拒绝恢复，不能用当前内嵌发布物补齐后声称旧来源完整。


2026-10-05：按用户“按照顺序完成所有阶段”继续推进。S1–S4及运行补齐已完成阶段验收，S5本地验收完成并保留容器/live外部排除项；定义3.3.0/contract1.4.0显式可选，默认旧入口仍为3.0.1。各阶段及live证据分别登记于[实现状态](../docs/analysis/IMPLEMENTATION-STATUS.md)。此前停止记录为历史，不再作为当前停止条件。

Go HTTP 服务与 Temporal Worker，独立实现对话、版本绑定批准、隔离模板生成、产品内部验证和GitLab交付。公开契约见 [AGENT-GUIDE](../docs/AGENT-GUIDE.md)，规范见 [design](../design/README.md)。后端不加载参考harness、设计检查器或外部validation模块。

## 启动与模式

从本目录运行 `go run ./cmd/server`，默认监听127.0.0.1:8080，默认 temporal/rules/local。Temporal未就绪拒绝启动，不自动降级。显式本地辅助模式：

```sh
AGENT_EXECUTION_MODE=local AGENT_DECISION_MODE=rules AGENT_DELIVERY_MODE=local go run ./cmd/server
```

真实Temporal执行：

```sh
AGENT_EXECUTION_MODE=temporal TEMPORAL_ADDRESS=127.0.0.1:7233 go run ./cmd/server
```

local在服务进程执行业务阶段，文件快照提供明确恢复条件，它不是Temporal history/replay/Activity调度。rules明确不调用模型；model要求完整配置。delivery local返回验证文件包；gitlab要求准确远程与CI证据。三轴独立，新Run的execution_mode必须与当前服务相符。

## 配置与公共依赖

| 环境变量 | 默认或用途 |
| --- | --- |
| HTTP_ADDR | 127.0.0.1:8080 |
| AGENT_DATA_DIR | .data，快照/JSONL/工作区/包，需持久可写 |
| AGENT_EXECUTION_MODE | temporal，可选local；兼容显式旧AGENT_MODE |
| AGENT_DECISION_MODE | rules，可选model，不按AI配置自动选模式 |
| AGENT_DELIVERY_MODE | local，可选gitlab |
| AGENT_APPROVAL_TTL | 30m，服务端限制1m至24h |
| TEMPORAL_ADDRESS | 127.0.0.1:7233，temporal模式必需 |
| TEMPORAL_NAMESPACE | default |
| TEMPORAL_TASK_QUEUE | project-init-agent |
| AI_BASE_URL / AI_API_KEY / AI_MODEL | model模式所需OpenAI-compatible endpoint/key/model |
| GITLAB_URL | https://gitlab.com，gitlab目标实例 |
| GITLAB_TOKEN | 可信执行器凭据，需准确动作权限 |
| GITLAB_NAMESPACE_ID | gitlab交付必需的准确正整数namespace，不推断当前用户个人空间 |

内部生成验证需要Go工具链；首次下载模板依赖需要网络。文件存储限定单服务写者，Temporal不自动持久化本机文件。详见 [输入输出依赖与公共能力](../docs/AGENT-GUIDE.md#输入输出依赖与公共能力)。

每Run冻结BudgetPolicy：最多256条命令、32次模型尝试（含失败）、128个工件文件、2MiB工件总字节和每阶段3次尝试；Usage持久记录实际消耗。定义包含模板、Skill、动作、工具与预算；model模式另冻结endpoint/model，不把key写入定义。provider地址或模型变更要求新Run，服务端凭据可轮换；已有Run不能跨当前execution模式处理。

## HTTP与批准

当前完整入口为 `/api/v2`，显式定义3.3.0/contract1.4.0，已接通完整建议/校验/执行/批准/消息/摘要/历史快照与交付。请求、回执、SSE、精确对象和错误均做完整Schema校验；Vue运行工作区默认消费此入口。详见[当前API手册](../docs/AGENT-GUIDE.md#完整-v2-运行接口)。以下S1/S2段落是接入过程的历史说明，阶段未完成描述由最新状态覆盖；原legacy接口和发布版本保留。

S1默认产品定义3.0.1使用发布1.1.0完整Schema。系统提示、三Skill参数/默认值/依赖/写路径、模板原字节、Schema/工具/provider及策略进入不可变DefinitionSnapshot；旧Run读取绑定版本。legacy投影以frozen_snapshot_hash绑定完整来源。模型规划输出改为严格DecisionAdvice；完整建议/校验/执行请求持久链仍待S2。发布中的新增工具/累计执行/CI等待/核实期限和预留结算仍待S2及运行补齐阶段接入。

创建请求另支持同时指定definition_id、definition_version、definition_hash。`GET /api/definitions`返回完整定义列表；`GET /api/runs/{id}/definition`返回完整绑定快照；`GET /api/runs/{id}/definition/content?object_type=template_content|schema&object_id=准确身份&hash=准确摘要`返回绑定原字节及X-Content-SHA256。缺少旧来源返回409 legacy_contract_incomplete及legacy_read_url，不补造历史或新增批准。详细边界见[Agent手册](../docs/AGENT-GUIDE.md#完整冻结定义的只读接口s1)。

S2的1.2/1.3已提供完整类型、多版本注册、确定性治理/预算、不可变对象存储及下述受控执行接入。三个只读入口 `/api/runs/{id}/decision-audit`、`/objects`、`/budget` 只读取已持久登记的完整来源；现有S1 Run返回409 `legacy_contract_incomplete`及legacy读取地址。budget成功响应以`Link`头给出首次登记source的精确objects引用。默认仍为3.0.1/contract1.1.0；公开创建尚未切换到新受控主线，显式选择未就绪发布返回`definition_not_ready`。模块与synthetic检查不能算实际job通过S2。

S2接入进度：已接入完整Control Run的命令处理、真实model/rules计划建议、P1控制命令、失败/重放预算，以及后续动作的真实模型调用、确定性调度和原子执行前复校。`PrepareControlAction`登记完整请求与ready operation，执行器通过`beginControlExecution`复校后才预留实际尝试并登记sent；原Execute布尔值不授予受控执行权。`ExecuteStep`对完整Control Run使用共用受控调度；1.3的候选修订、新P1、独立目录、真实Go验证、打包和完成报告已有定向本地测试。默认仍为3.0.1/1.1，1.2/1.3公开创建保持`definition_not_ready`，受控push/CI和完整S2公开/Temporal验收尚未完成。

1.3候选修订的`revise`绑定原提案ID、proposal_version、source_version及snapshot_hash；成功新提案与命令回执一起提交，原工具证据、目录和累计预算保留。1.3新增`reconcile`只接受Run及命令ID，从持久账本选择原sent/unknown操作；当前已接通建仓与原推送核实，不接受调用者提供目标/批准，不触发后续写入。终态核实不会复活Run，GET不触发核实。该扩展仅对具有完整1.3来源的Run生效，不开放旧发布的新intent。

1.3建仓及只读核实保存完整`RemoteExecutionReceipt`和逐GET观察；unknown后核实追加新回执，原回执和消费不覆盖。调度器及恢复读取核对完整回执、批准/请求/manifest、读预算和操作引用，私有成功索引不能单独推进。创建成功、重启重放、失响应后取消核实与凭据响应拒绝已有mock路径证据；push分支已接入并有定向验证，CI分支尚未接通。

3.2冻结模板的SQLite使用纯Go驱动、CGO=0；GORM/sqlx两ORM已有macOS实际冷Run的tidy/test/build证据。1.3沙箱额外只读固定`/etc/protocols`和`/etc/services`公共运行库数据，旧1.2权限不变；不开放任意系统目录或shell。Linux、真实Temporal及远程CI证据另行验收。

历史发布3.1.0锁定7种Go依赖图，3.1.1仅更新Agent各阶段建议提示并复用3.1.0技能/模板。63种组合已做真实tidy/test/build和原字节不变验证；这是当时本机默认CGO的发布验证。受控Go工具另在Run候选目录、固定环境/命令和进程沙箱中执行；新的3.2纯Go SQLite证据见上文，不将旧63组合结果转成CGO=0证明。Linux bubblewrap仍未在本机验收。详情见[实现状态](../docs/analysis/IMPLEMENTATION-STATUS.md)。

| 方法 | 地址 | 用途 |
| --- | --- | --- |
| GET | /api/health、/api/bootstrap | 进程状态、三项模式、定义与能力就绪 |
| GET / POST | /api/runs | 列表 / 创建Run |
| GET | /api/runs/{id} | 权威快照 |
| GET | /api/runs/{id}/workflow | 真实Workflow/执行段与Query，只读；local返回不适用 |
| POST | /api/runs/{id}/messages | 稳定id与自然语言content |
| POST | /api/runs/{id}/actions | 接受/拒绝/修订，暂停/恢复/取消/重做 |
| GET | /api/runs/{id}/events | SSE snapshot |
| GET | /api/runs/{id}/artifacts | 清单，?path=读取批准范围内容 |
| GET | /api/runs/{id}/download | 真实生成ZIP |

source_version仅方案/授权范围变化增加，version为兼容镜像，普通进度用state_sequence。Proposal包含不可变proposal_version、snapshot_hash、definition_hash和服务端issued_at/expires_at。accept/reject/revise携带稳定命令id，以及当前proposal_id、proposal_version、source_version、snapshot_hash。接受/应用复核版本、hash和期限，客户端不能延期；重复成功请求复用原结果。

创建请求为 `{id,title,content,decision_mode,delivery_mode,execution_mode}`；后续消息为稳定`id/content`。pause/resume/cancel/retry提交`id/action`。bootstrap公开三轴、model_configured、gitlab_base_url、approval_ttl_seconds、能力及定义。运输层命令意图和Temporal Update尝试身份持久化，失败重试仍绑定原payload；不靠更换SDK身份重放结果未知的Update。

终止Run重做产生新身份；未知外部写先BLOCKED核实原对象；暂停/取消不撤销已有副作用。期限后新增高权限动作重新批准，已核实副作用保留。不存在设计认知/api/model服务，当前结构由独立离线工具维护。

## 内部验证与外部场景

产品内验证位于`verification.go`，执行器位于`execution.go`；保留真实go mod tidy、go test、go build、必需文件、manifest及秘密检查。local固定11项required_checks；gitlab另核对精确仓库、提交文件/tree与manifest，以及精确SHA/ref流水线、必需test/build job和coverage/executable工件。缺必需项不得DONE，外部validation不是完成依赖。数据库能力仅模板与隔离测试，不执行客户数据库实际迁移。

GitLab发送前登记sent账本，丢响应或崩溃按原操作只读核实；未知且没有可归属结果仍BLOCKED，不新增POST。分支基线冲突拒绝覆盖。生成日志file首版限定logs/独立目录，排除源码/manifest/备份冲突；故障健康策略及能力限制见Agent手册。

```sh
go test ./...
go test -race ./...
go vet ./...
go build ./cmd/server
```

最新结果和真实Temporal、mock/live第三方、外部场景与未覆盖项分别见 [实现状态](../docs/analysis/IMPLEMENTATION-STATUS.md)。[validation](../validation/README.md) 在另一进程调用公开Agent接口；服务启动及项目完成不依赖它。

S1之前的2026-10-02历史记录：全套与race分别38.031s、51.659s通过；最后HTTP/CI错误分类与读取重试修复另通过定向测试，最终vet/build通过。外部local 9/9、真实Temporal 9/9及model重试2/2有独立公开报告；模型/GitLab为mock，容器构建和live第三方未验收。当前代码的分阶段结果见实现状态，不能复用这些历史计数当作新验收。
