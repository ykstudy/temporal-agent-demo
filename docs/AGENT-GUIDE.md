# 项目创建 Agent API 使用手册


运行详情新增真实就绪检查、Workflow 检查点、活动尝试与心跳、追加审计、累计指标及失败恢复条件。使用 [runtime/1.0.0](../design/contracts/runtime/1.0.0/README.md) 独立只读协议，完整业务1.4对象保持不变。观察滞后或连接失败明确显示，心跳不代表业务成功。预算沿原冻结定义预留/结算，恢复不延长总执行/CI/核实期限；未实际执行的检查保持 not_ready。终止 Run 重做仍创建新身份并重新批准。

运维使用 `/api/runtime/ready`；兼容 `/api/health` 不证明依赖就绪。离线备份/恢复入口及单写者边界见 [backend 使用说明](../backend/README.md#当前运行观察与离线恢复)。历史状态说明以 [最新验收登记](analysis/IMPLEMENTATION-STATUS.md) 为准。


2026-10-05：S1–S4已通过阶段验收，新运行工作区默认使用完整 v2 API，显式选择定义3.3.0/contract1.4.0。legacy `/api/runs` 的默认定义仍是3.0.1；两个入口不能混用。当前状态及证据见[实现状态](analysis/IMPLEMENTATION-STATUS.md)。

## 完整 v2 运行接口

`POST /api/v2/runs` 接收发布的完整CreateRunRequest；`POST /api/v2/runs/{id}/messages` 接收MessageRequest；`POST /api/v2/runs/{id}/actions`接收ControlRequest。所有输入先严格JSON/Schema校验。命令ID固定绑定完整wire JCS指纹，重复处理返回原CommandReceipt；失响应后读取 `/commands/{command_id}`，不能换ID盲重投。显式失败重试仍用同一请求；终止重做返回关联新Run且重新批准。

`GET /api/v2/runs`、`/{id}` 返回完整RunSnapshot。`/{id}/events` 的SnapshotEvent只指向准确历史快照；event_id含Run身份和state_sequence，Last-Event-ID必须属于该Run且不领先。重复/乱序丢弃、缺口和断线重取公开权威快照。旧记录缺历史来源时返回409 `legacy_contract_incomplete`和legacy_read_url；只读显示原文，不生成partial v2或自动回落写请求。

`/{id}/objects/{object_type}/{object_id}` 的查询必须同时携带object_version、source_version、hash，返回当时对象；过时提案只读。`/{id}/artifacts` 返回已批准manifest，`?path=`返回清单内当前文件；`/{id}/download`读取已验证ZIP。文件/包均带X-Manifest-Hash、X-Artifact-SHA256、X-Source-Version，界面复核响应字节及来源。批准、定义、预算、验证报告、摘要历史及原文入口均保留精确引用。

完整Schema与请求格式见[1.4发布](../design/contracts/releases/1.4.0/README.md)和[传输契约](../design/contracts/AGENT-CONTRACTS.md)。400表示输入/Schema拒绝，409表示身份/版本/来源冲突，429表示预算额度拒绝，503表示依赖不可用；以返回的稳定code作进一步判断。来源版本仅业务变化增加，普通进度使用state_sequence。

运行补齐与S5本地契约/路径验收已登记；原生成项目容器被镜像仓库网络阻塞，真实model/GitLab/Runner仍缺配置及授权目标。当前结论为有外部排除项的验收，不是全部阶段或live完成。

以下保留2026-10-02至10-03的legacy手册及实施过程记录，其阶段待办由上面的当前状态覆盖；legacy路径/格式仍适用于原记录。

## 当前结构差异

S1新增完整冻结定义读取：新Run绑定发布1.1.0 Schema、完整AgentDefinition/三类SkillDefinition、系统提示原文、参数规则、模板原字节、工具/provider及策略；当前产品定义版本3.0.1。模型规划入口接收严格DecisionAdvice，提示和生成模板读取绑定内容。既有Run响应、Proposal、消息、摘要和执行授权仍为legacy；结构化建议/校验/执行请求持久链、Message Part、Summary历史与完整 `/api/v2` 留待S2–S4。定义读取通过不代表整个v2完成。

S2的1.2.0契约、治理/预算、完整对象归档及只读观察基础已进入模块验证。`GET /api/runs/{id}/decision-audit`、`/objects`、`/budget` 不执行模型或工具；缺完整来源时返回409 `legacy_contract_incomplete`，不会适配旧记录为完整链。budget的`Link`头是该账本版本首次登记的精确引用，不能用当前source猜测。当前创建主线仍使用3.0.1；选择尚未接通实际执行的发布会返回`definition_not_ready`，S2实际job验收未完成。

后续S2接入已覆盖完整Control Run的原文先存、model/rules计划、准确P1、命令/模型失败计账、原请求复用与执行前复校。model Run的后续action确实调用冻结provider；sent/unknown只给原身份只读核实。1.3/定义3.2已发布，候选差异→新source提案→重批→新目录→真实验证/ZIP/完整报告的本地定向路径已通过；模型修订失败保留原批准且计账。纯Go SQLite两ORM已通过macOS受控冷缓存tidy/test/build。默认未切换，受控Git push/CI、公开新Run完整路径、真实Temporal与独立1.3 oracle仍待验收。

1.3建仓与只读核实已提供完整远程回执：每次GET的观察与预算结算绑定，核实成功追加记录而不覆盖unknown。错误项目/请求/批准/manifest、读来源和伪造成功索引不能推动后续调度。现有证据包含mock建仓、重启重放、取消后核实和公开对象导出的独立结构/hash检查；完整公开1.3链路oracle及真实GitLab/Runner仍未完成。

## 输入、输出、依赖与公共能力

| 类别 | 契约 |
| --- | --- |
| 用户输入 | 自然语言需求、项目名/描述、Go技术栈、数据库/认证/日志参数；三项模式与仓库目标 |
| 控制输入 | 稳定Run/命令身份；接受/拒绝/修订带proposal/source版本和snapshot hash；暂停/恢复/取消/重做明确动作 |
| 配置输入 | 可信数据根、当前部分定义/模板/Skill版本元数据、服务端批准期限、公共服务地址与秘密引用；不接受客户端token或任意shell |
| 状态输出 | 精确Run、十一态、source_version、state_sequence、三项模式、错误/阻塞原因和历史 |
| 预览输出 | 未应用差异、影响、checks、proposal_version、snapshot_hash、definition_hash、issued_at/expires_at |
| 交付输出 | 文件/manifest、内容、真实ZIP和required_checks；gitlab交付另有精确仓库/提交/pipeline证据 |
| 产品内公共能力 | 参数Schema、确定性动作/批准校验、Run隔离、消息去重、模板渲染、test/build、manifest/秘密检查和原操作核实 |

强依赖按模式启用，不因缺一项就静默选择另一模式：

| 依赖 | 必需范围 | 边界 |
| --- | --- | --- |
| Go工具链与模块获取 | 编译源码服务和运行生成项目test/build | 当前服务Go1.25；模板版本由definition_snapshot锁定；首次模块下载需网络 |
| 可写持久目录 | 全部Run | 快照、原消息、工作区、包；每个数据根只允许一个写者，Temporal不持久化本机文件，不宣称跨节点事务 |
| Temporal公共服务 | execution_mode=temporal | 准确地址/namespace/task queue；无需其他项目源文件 |
| 模型公共接口 | decision_mode=model | 完整endpoint/key/model；结构建议仍过规则，失败不改成rules |
| GitLab API和Runner | delivery_mode=gitlab | 准确实例和必需namespace ID、最小动作权限、真实首跑；缺证据BLOCKED，不推断个人空间 |
| 客户业务数据库 | 本期不需要 | 数据库技能只模板与隔离测试，不对真实客户库迁移/写入 |
| 设计模型或validation | 不属于运行依赖 | 离线结构检查与外部黑盒场景分别执行 |

## 模式与启动

execution_mode取temporal/local，decision_mode取model/rules，delivery_mode取gitlab/local，默认temporal/rules/local。local执行可以选择真实gitlab交付，Temporal执行也可交付本地包；组合仍满足各自依赖。rules没有模型调用，本地进程不声明Temporal replay。

按 [backend README](../backend/README.md) 配置并启动。`AGENT_APPROVAL_TTL` 默认30m，可配1m至24h；期限由服务端签发，不接受客户端延长。HTTP默认127.0.0.1:8080，秘密在可信环境提供。

新Run持久化完整DefinitionSnapshot，legacy definition_snapshot仅为兼容投影，以frozen_snapshot_hash绑定完整来源。model模式冻结endpoint/model，不含key；缺少准确身份时拒绝创建，改变身份要求新Run。旧记录缺少来源时不补造，只保留legacy读取和原账本限定恢复，不能新增批准。已有Run不可跨服务execution模式处理。commands256、model attempts32（含失败）、files128、artifact bytes2MiB、step attempts3继续真实计账；发布中的单工具/总执行/CI等待/未知结果核实期限及预留结算尚待S2/运行阶段接入。

## 完整冻结定义的只读接口（S1）

- `GET /api/definitions` 返回完整AgentDefinition列表。
- `GET /api/runs/{id}/definition` 返回该Run绑定的完整DefinitionSnapshot，符合发布1.1.0。
- `GET /api/runs/{id}/definition/content?object_type=template_content|schema&object_id=准确身份&hash=准确摘要` 返回绑定范围内的原字节与`X-Content-SHA256`。schema身份使用schema_uri；模板身份使用文件的content_ref.object_id。

创建请求可以同时指定definition_id、definition_version、definition_hash三个精确字段；全部省略时绑定默认发布，缺任一字段拒绝。发布/撤销为可信进程内管理操作，模型和用户HTTP不能发布模板、代码或定义。同版本异内容拒绝；缺失、篡改或撤销依赖拒绝新绑定及后续执行。数据根的definitions目录必须与Run一起持久保存。

旧Run没有完整来源时，定义读取返回409 `legacy_contract_incomplete`及`legacy_read_url`，不把当前提示/Schema当成旧来源。HTTP JSON拒绝重复键和未知字段；创建/命令体最多64KiB，完整对象codec资源限额见[1.1.0发布说明](../design/contracts/releases/1.1.0/README.md)。

## 创建与补充需求

读 `/api/bootstrap` 获取模式、能力和定义，然后创建：

```http
POST /api/runs
Content-Type: application/json

{"id":"run-example","title":"工单服务","content":"创建Go工单服务，Postgres和JWT，交付本地包","decision_mode":"rules","delivery_mode":"local"}
```

`id`用于重发关联；可选execution_mode必须等当前服务。后续 `POST /api/runs/{id}/messages` 发送 `{"id":"message-1","content":"增加JSON结构化日志"}`。同ID同内容复用既有结果，同ID改内容冲突。权威响应决定CLARIFY或PLAN，模型文字本身不推进状态。

日志默认为 info/json/console。file 输出只接受项目相对路径，首版限定在独立 logs/ 目录，默认 logs/app.log；路径及轮转备份不得与源文件、manifest 或目录冲突，也拒绝逃逸及符号链接。大小、数量、保留期和 degrade/fail_closed 故障策略属于待确认参数。日志写入故障会影响生成服务的健康状态；fail_closed 还拒绝新的业务写请求。brokenWriter 与关闭 stderr 的故障注入只能证明对应错误处理，不能代替真实磁盘满或生产日志采集验收。

## 审查与确认

PLAN返回未应用提案，此时没有真实项目生成或远程副作用。审查参数、预计文件、准确目标、checks和版本，再从当前提案复制确认字段：

```http
POST /api/runs/run-example/actions
Content-Type: application/json

{"id":"accept-1","action":"accept","proposal_id":"从响应复制","proposal_version":1,"source_version":1,"snapshot_hash":"从响应复制"}
```

示例版本和hash不是可直接提交的真实确认，必须取当前响应。reject/revise使用同样对象绑定字段；拒绝回CLARIFY，补充需求后生成不可变新提案。source_version仅业务方案/授权范围变化，普通进度只增加state_sequence。接受与首次应用检查版本/hash/服务端期限；期限内已开始的固定批准链可继续原范围，过期后新增范围或高权限动作须复批，已有核实副作用保留。

GitLab提案target明确gitlab_url、namespace_id、project_path及ref；预览的Plan、Definition、Target和Changes共同绑定snapshot_hash。用户应核对准确外部目标及日志路径/轮转/故障参数后确认，不能以“GitLab交付”四字代替准确范围。

普通创建确认不包括客户数据库真实迁移/写入、生产合并或发布。文档认知模型离线检查及未来文档审阅apply都不提供运行授权。

## 产物manifest与内部检查

manifest包含run_id、source_version、proposal_id/proposal_version、snapshot_hash、definition_hash、plan_hash、files、hash和created_at；files逐项path/size/sha256。required_checks逐项name、passed、status（not_ready/passed/failed）、detail、source_version。本地固定检查集合当前11项，gitlab交付另加remote_repository、remote_commit和gitlab_ci；缺输入保留失败/未就绪，不能省略后对剩余项判通过。

## 跟进与产物

`GET /api/runs/{id}` 返回权威快照，`GET /api/runs/{id}/events` 返回SSE snapshot。阶段为INTAKE、CLARIFY、PLAN、IMPLEMENT、VERIFY、DELIVER、DONE，以及PAUSED、BLOCKED、FAILED、CANCELLED；DELIVER的remote_repo、remote_ci、package是子步骤。

`GET /api/runs/{id}/workflow`为只读公共观察接口，temporal模式返回真实Workflow及execution run ID和Query；local明确不适用。该接口不推动业务阶段，Continue-As-New改变执行段而保持业务Run身份。

应用后读取 `/artifacts` 清单和 `/artifacts?path=相对路径`，从 `/download` 下载ZIP。required_checks与manifest来自产品真实内部验证；成功命令、收到消息或存在包文件都不自行证明DONE。gitlab还要证明本次批准提交的匹配CI，不接受“最新绿色流水线”。

暂停/恢复/取消通过actions明确请求。终止Run不能复活，retry创建关联新Run，不是无条件重发原远程写。未知结果先BLOCKED核实原namespace/project/ref/SHA/pipeline；取消不谎称已撤销副作用。

完整1.3 Control Run支持版本绑定的VERIFY候选`revise`：引用原批准提案的ID、版本、source和hash，生成新提案后等待新P1，不把差异写回原目录。新增`reconcile`的请求为`{"id":"稳定命令身份","action":"reconcile"}`，目标由原账本决定，不接受额外提案/目标字段；当前建仓与原推送核实已接通。仅只读记录原副作用，可用于已取消Run，成功重放不增加远程读写，GET保持无副作用。1.2及缺完整来源的legacy Run不接受新intent；这些分阶段入口不代表1.3公开创建已开放。

## 错误与恢复

错误响应包含error/code。HTTP409表示身份、版本或非法状态等冲突；先重读当前快照与提案，不能只换命令ID绕过检查。配置、路径/秘密、provider不可用和验证失败保留准确原因；文档检查和mock场景不能代替真实事实。详细错误码以当前API响应和实现状态为准。

## 独立验证与证据

离线检查 `python3 design/_model/check.py --self-test --json` 只证明结构与冻结摘要。产品无需外部场景运行也必须自己完成真实test/build、manifest和必要远程检查。

[validation](../validation/README.md) 是另一进程的黑盒调用器，只用公开HTTP、SSE、ZIP及可选Temporal公共接口，不导入Agent内部服务或模板。其summary把passed/failed/not_run及mock/live/rules分别登记；可以针对已运行Agent独立调用。入口、flags与证据目录以该README为准，最终实测回填实现状态。

参考harness只是历史组织方法来源，副本及运行认知模块已移除。迁移前React工作区的历史桌面/手机证据见[原浏览器验收](evidence/BROWSER-QA.md)。当前Vue工作区及十一类只读审查、来源详情、同源SVG/PNG导出见[本阶段验收](evidence/VUE-REVIEW-QA.md)；独立local与真实Temporal报告、mock第三方及未覆盖范围见实现状态。本手册公开HTTP契约可供独立客户端使用，不声称全部45功能或生产运维完成。
