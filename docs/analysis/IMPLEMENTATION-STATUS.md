# 当前实现、内部验证与外部证据

2026-10-05最终登记：提示词1–6通过；提示词7为 `passed_with_exclusions`，提示词8为 `external_blocked`。见[S5退出与阻塞索引](../evidence/s5-exit.json)。197项当前顶层Go测试跨记录批次通过，5项显式可选探针/发布生成工具未在普通套件执行；原整包超时和两处过时测试输入均保留原报告，详见[回归汇总](../evidence/s5-go-regression-summary.json)。容器及live未通过，因此没有宣称全部阶段完成。

2026-10-05运行阶段（提示词6）通过：[退出索引](../evidence/runtime-exit.json)。已先发布ADR-052/runtime1.0完整只读Schema，再接入真实Activity/心跳、追加审计链、准确累计指标、Workflow执行段检查点、失败归因与冻结期限、实际readiness及离线整目录备份/恢复。真实Temporal CAN、VERIFY崩溃、唯一原身份恢复、164文件备份校验及缺依赖拒绝恢复分别有证据；最终浏览器主线12项和诊断10项通过。进入S5整体验收，live未通过。

2026-10-05 S4阶段门通过：[退出索引](../evidence/s4-exit.json)。完整 v2 API、Schema 派生 TS/AJV 入口、五类 Part、历史提案、原命令回执、SSE恢复、批准/文件/ZIP与legacy只读工作区已接通。最终 API race 62.1s、vet/build、Vue check/build、Go/TS来源校验通过；浏览器真实创建/修订/11项内部检查/DONE/下载/重做主线12项，异常响应/迟到响应/事件及Part安全13项，legacy只读5项分别通过。真实local和Temporal公开主线使用三技能并实际解包构建/启动；浏览器发现的多轮上下文超限已按发布规则修复并经race及真实主线复核。报告的代码时点各自保留，当前不将其换算为运行收尾、S5或live通过。

2026-10-05 S4后端切片通过：完整wire请求与精确指纹、历史RunSnapshot、原命令回执、SSE及游标、legacy只读、终止重做身份通过[定向race](../evidence/s4-api-race.log)（61.4s）。独立[local](../../validation/reports/s4-local-public-2/summary.json)和[真实Temporal](../../validation/reports/s4-temporal-public/summary.json)均完成3技能、11项产品内部检查、全部文件size/SHA256及manifest/包来源复核，ZIP实际构建并启动health通过；没有live提供方或浏览器验收含义。此为后端切片历史记录，整体S4退出结论见本页最新记录。

2026-10-05阶段门：S3已通过，见[退出索引](../evidence/s3-exit.json)。五类Part、历史提案引用、原始JCS JSONL、摘要来源/版本、模型失败保旧、并发来源冻结及持久回执已验证；最终回执准确RunSnapshot和部分提交/损坏恢复race通过218.8s，存储链接/字节边界通过2.2s。公开local与真实Temporal各45消息/3摘要/22回执，后者实际CAN保持原文；该公开记录早于最终回执快照加强，最终加强以定向race/重启核对留证，S4再做公开传输验收。文档141实体/568关系及self-tests通过；S4、运行收尾、S5与live继续独立推进，当前不宣称全部完成。

2026-10-04阶段门更新：S2必需定向检查已补齐，详见[退出索引](../evidence/s2-resume-exit.json)。广泛Go回归593项通过、两个夹具身份错误保留原失败报告；修复为在签发请求前冻结真实工作区身份，原断言不变，两项race复验81.5s通过。原launch恢复及缺launch两分支race通过；同进程后续推送超时，另起完整正常/失响应推送及20项篡改负例race通过（471.3s），不将超时报告整体改称通过。结构缓存只复用完整相同字节/版本/Schema的成功验证，不复用治理结果；缓存、旧发布及源字节验证通过。S2退出，S3进入实际接入，后续阶段仍未验收。

2026-10-04恢复：用户“按照顺序完成所有阶段”覆盖下方历史停止条件。S2受控CI、公开local/真实Temporal、三技能包实际启动、CAN/并发和崩溃后原身份只读核实已有新增证据，见[本轮证据索引](../evidence/s2-resume-exit.json)。原失败及中断日志保留，定向推送race和广泛Go套件仍单独核对；未完成项目不计通过。后续S3–S5及运行/live验收仍须逐阶段完成。

日期：2026-10-02。当前用户授权已从“只生成文档、保留实验实现”进入“按现有规范实现独立项目创建Agent、移除参考耦合”。本文件分别记录产品代码、内部完成验证、外部场景与未覆盖范围，不能把45份功能文档视为全部实现。

## S5 独立验收与外部阻塞（2026-10-05）

本轮按提示词1→8推进，S1–S4与运行补齐已通过。S5的结构、语义、公开路径和浏览器证据已汇总；Go广泛回归按超时前记录与未完成用例续跑合并，原失败/超时仍保留。生成项目 Docker 构建与 live 第三方分别保留外部阻塞，因此本阶段是 `passed_with_exclusions`，并非 full_acceptance 或所有阶段完成。

| 验收项 | 当前证据与范围 |
| --- | --- |
| C01–C05 完整定义/技能/冻结来源 | 当前1.4公开 DefinitionSnapshot、技能/模板/提示及内容 hash 经完整 Schema 和独立审计；原1.1定义负例与版本/撤销/旧提示 Go 回归保留其真实发布版本 |
| C06 迁移边界 | v2缺历史来源409、legacy只读、原批准不继承；当前浏览器legacy5项与严格legacy实际交付/ZIP启动通过 |
| C07–C10 建议、授权、幂等、批准 | 当前3.3/1.4公开模型503稳定身份重试、伪批准/shell/假执行、合法Schema错误计划、错hash/旧source、真实TTL过期共5类通过；真实Temporal并发与CAN幂等另证 |
| C11–C14 Part、原文、摘要 | 五类Part/跨Run负例、原JCS JSONL与回执恢复；公开model路径45消息/3摘要/22回执及8个负例通过，另注入一次503保旧、读查询无自动重试、显式新消息生成第4摘要 |
| C15–C17 v2、同源交付和工作区 | 当前local三技能11内部检查/完整历史请求回执/SSE/文件与ZIP复核、真实启动；浏览器主线12项、异常/安全13项、诊断10项、legacy5项分别记录 |
| C18 运行交付 | 当前local与真实Temporal交付/CAN/并发/VERIFY中自有Worker崩溃恢复通过；1.4远程链实际Go/Git/打包、14内部检查通过，GitLab HTTP与Runner仍是mock |
| 本范围运行补齐 | Activity真实attempt/心跳/执行段、完整审计/预算/诊断、真实依赖就绪及离线恢复见runtime退出索引；不等于多节点/生产运维 |
| G17/G18 独立oracle | 当前业务1.4+runtime1.0共149个命名定义都有正例与结构负例，其中127个来自公开对象、22个仅为独立synthetic；命名定义覆盖不能替代所有运行分支验收 |
| G20/G21 模块与说明 | 6个真实Go包的导入方向与4个反向依赖负例通过；12条验证CLI示例参数有效，实际 `/fixture/health` 成功、旧错拼路由404。当前职责见MODULE-MAP新增实施映射 |
| 生成项目容器 | 原ZIP/Dockerfile已实际尝试构建；Docker Hub认证端点连接超时，未构建/启动成功。原日志保留，状态 external_blocked；未替换Dockerfile或镜像版本 |
| 真实模型/GitLab/Runner | 所需6项配置未出现在当前进程环境，且未收到授权测试namespace/project；状态 external_blocked/not_run，没有真实远程写入或live成功证据 |

当前结构报告：[完整覆盖](../evidence/s5-schema-coverage.json)、[公开local与remote导出](../../validation/reports/s5-schema-sweep-final-local/summary.json)、[真实Temporal](../../validation/reports/s5-schema-sweep-final-temporal/summary.json)、[model公开对象](../../validation/reports/s5-schema-sweep-final-model/summary.json)、[仅synthetic分支](../../validation/reports/s5-oracles/supplemental-shapes.json)。公开路径：[权限](../../validation/reports/s5-public-authorization-final/summary.json)、[Temporal模型交付](../../validation/reports/s5-temporal-model-completion/summary.json)、[模型摘要](../../validation/reports/s5-model-summary-final/summary.json)、[摘要故障保旧](../../validation/reports/s5-model-summary-failure/summary.json)、[三技能本地交付](../../validation/reports/runtime-final-local-delivery/summary.json)、[当前remote集成](../evidence/s5-runtime-remote-2.log)。

本轮广泛回归达到30分钟包级上限后，只续跑剩余39项（另外1项新增remote集成已单独通过）。摘要测试在续跑中通过，未将时间中断改写为成功。

本轮保留失败记录：两处旧未知版本测试使用的1.4已成为真实发布，补充当前版本正例并改用不存在的9.9.9后原拒绝断言通过；模型摘要fixture误向message Schema查找SummaryAdvice，修正为发布agent Schema后重跑通过；独立遍历器补齐布尔Schema及同形父对象下的子分支收集。均未放松产品或Schema断言，也未把历史失败文件改写成成功。Docker失败不能由这些本地通过覆盖。

三轴组合按当前证据登记，表内 model/GitLab 均为明确 mock；未跑组合不能由其他主线填成成功。原默认legacy3.0.1另有实际包回归，不改名为1.4。

| execution / decision / delivery | 当前1.4覆盖 |
| --- | --- |
| local / rules / local | 实际三技能、11检查、ZIP与生成服务启动、浏览器 |
| temporal / rules / local | 实际交付、CAN、并发、VERIFY崩溃/取消/新Run重批 |
| local / model / local | 公开计划/权限故障/摘要与失败保旧；此组合未重复完整DONE交付 |
| temporal / model / local | 实际完整项目到DONE；provider为mock |
| local / rules / gitlab | 实际Go/Git/ZIP及14检查；GitLab协议/Runner为mock |
| local / model / gitlab | not_run：当前完整组合未验收 |
| temporal / rules / gitlab | not_run：当前完整组合未验收 |
| temporal / model / gitlab | not_run：当前完整组合未验收 |

外部阻塞的可复现入口：`validation/contracts/container_delivery.py` 使用公开包目录再构建；`validation/live_preflight.py` 只检查环境存在及目标绑定。收到真实模型配置位置、GitLab配置位置及授权测试namespace/project后，依提示词8先形成具体方案并取得该Run产品批准，再运行live；Runner权限/可用性仍需实际核验。生产合并/发布和客户数据库真实迁移不在范围。

## 结构审计后的历史结论

当前用户在S0文档发布后授权“开始推进所有项”。按[ADR-047](../../design/10-decisions/ADR-047-vue-readonly-review.md)已完成Vue等价迁移，并落地独立只读设计审查入口、十一类静态模型与同源SVG/PNG导出。工作区仍消费legacy公开API，后端业务协议未因框架迁移改变。新迁移浏览器主线10项与竞态5项合计15/15通过；最终生产审查187/187、发布来源15/15及1个Schema探针通过；[本阶段验收](../evidence/VUE-REVIEW-QA.md)归档准确时点、图像、导出和证明范围。

当前按用户要求“拆阶段执行，按1–8顺序推进”。S1完整Definition/Skill冻结已完成本阶段实现与验收，具体本轮证据见文末；S2契约与基础治理、预算、对象归档正在实施；S2–S5及运行补齐/live仍未完成。[逐项差异矩阵](CONTRACT-GAP-MATRIX.md)保留原审计基线并追加阶段结果。Vue组件路径、图可见和Schema发布检查不能代替运行v2结构符合。新Run的提示与模板已经读取完整冻结快照；旧记录缺少的来源不补造。

审查静态数据采用[发布规范](../../design/review/README.md)：11模型、128公开来源、137文档实体/529关系、44冻结状态事件、21差异/18计划用例，契约图含108类与397条关系（379条$ref+18字段绑定）。离线元Schema/结构/格式/来源摘要、绑定与语义检查通过，12项数据错误注入与2项发布摘要负例全部拒绝；证据见[审查数据检查](../evidence/vue-review-data-check.json)。前端运行不依赖参考项目目录、design checker或外部validation结果，批准/完成仍在产品控制平面。

ADR-047静态审查发布时点为137实体/529关系/47 ADR，见[当时认知检查](../evidence/vue-review-design-check.json)。S1的ADR-048后为138实体/536关系/48 ADR，见[当前文档检查](../evidence/s1-design-check.json)。冻结PRODUCT-GUIDE/INVARIANTS字节不变。离线检查只表示docs_ready，运行对象的证据分别登记。

历史S0记录：三份Draft2020-12 Schema、381处静态引用、37个正负例（12正/25反）和四份技能参数默认声明通过，136实体/519关系及7+4检查通过；[原始契约文档证据](../evidence/contract-document-check.json)与[原始模型证据](../evidence/contract-design-check.json)保留原时点。当时32运行源文件hash未改属于S0事实，不用于说明之后Vue源码仍未修改。旧20/20场景、旧15/15浏览器仅支持当时代码的选定legacy路径。

## 当前边界

- `backend/` 是独立产品，不加载参考harness、离线设计检查器或外部validation。参考副本与实验认知运行模块按 [ADR-045](../../design/10-decisions/ADR-045-agent-model-validation-separation.md) 移除，早期相关测试仅留历史事实。
- `design/_model` 保留当前项目的离线结构模型，138个实体/536条关系/45份功能。通过只证明结构、来源与冻结摘要一致，不证明业务权限或状态执行正确。
- `validation/` 是公开HTTP/SSE/ZIP场景调用器，可选公共Temporal接口，不导入产品内部包。mock、live、rules以及passed、failed、not_run分别记录；外部未运行不阻止产品自己的完整required_checks。
- 原始PRODUCT-GUIDE与冻结INVARIANTS保持原字节和hash。普通项目创建不包括客户数据库真实迁移、生产合并或发布。

## 产品代码与本轮迁移范围

| 能力 | 代码/接口入口 | 证据边界 |
| --- | --- | --- |
| HTTP与三轴配置 | backend/cmd/server；internal/agent/http.go、service.go | execution temporal/local、decision model/rules、delivery gitlab/local独立；公开使用见Agent手册 |
| 方案、批准与版本 | types.go、service.go、contracts.go、model.go | source_version与state_sequence分开；proposal_version/hash不可变；服务端期限；定义/provider/预算冻结 |
| Temporal Workflow/Activity | workflow.go | Update/Query、顺序、CAN、心跳与业务身份；SDK测试不等于真实服务故障注入 |
| JSONL/快照和Run隔离 | store.go、service.go | 每个数据根单写者；消息身份与运输尝试持久化；不是多节点事务或生产租户鉴权 |
| 项目模板与三项技能 | templates.go、execution.go | 数据库仅生成代码/隔离验证；认证/日志为生成项目能力，不是Agent生产权限系统 |
| 产品内部完成验证 | verification.go，execution.go调用 | 实际test/build、固定required_checks、manifest和秘密检查；GitLab交付额外准确远程/CI证据，不导入validation |
| GitLab与模型接口 | gitlab.go、model.go | 契约mock可证明接口行为，不能宣称live第三方成功；未知外部写须核实原身份 |
| Vue对话工作区 | frontend/src/Workspace.vue、main.ts、App.vue、components/*.vue、style.css | 等价保留任务/对话方案/产物历史；新迁移浏览器15/15通过，稳定报告见本阶段验收，不作为v2符合性 |
| 静态设计审查 | frontend/src/review、frontend/public/review | 十一类规范/现状/历史证据视图，来源与筛选、同源SVG/PNG；最终187项交互/下载与来源边界15+1通过，完整图像与摘要见本阶段验收 |
| 日志console/file | 生成项目logging模板与测试 | 有界append/轮转/保留、独立logs路径、redact、失败health/ready；brokenWriter与关闭stderr为明确故障注入，不证明真实磁盘满 |

公开输入、输出、依赖及字段见 [AGENT-GUIDE](../AGENT-GUIDE.md)，配置见 [backend README](../../backend/README.md)。当前不存在运行认知API；文档模型审阅已提供独立静态只读页，不提供在线设计编辑/apply服务。

## 迁移前历史运行验证登记（2026-10-02）

| 分类 | 命令/入口 | 当前登记 |
| --- | --- | --- |
| 前端构建 | frontend的npm run build | 本轮已通过；不等于浏览器交互通过 |
| 后端全套与race | go test ./... -count=1；go test -race ./... -count=1 | 38.031s与51.659s passed；最终小修后的定向覆盖与vet/build另列，不把全部45功能当作运行验收 |
| 最后修复定向 | TestGitLab\|Transient\|UnknownPipeline\|ErrorCodes；TestHTTP\|ErrorCodes\|Model\|Proposal\|ConcurrentCreate | 7.746s与0.730s passed；覆盖HTTP错误分类、CI工件读取瞬断重试及内容hash复核 |
| 最终vet/build | go vet ./...；go build -o /tmp/agentstudio-core-final ./cmd/server | 2026-10-02T02:06:45Z passed；该时间位于最后小修与定向验证后 |
| 产品内生成验证 | Run的required_checks与manifest | 本轮local与temporal交付Run均有同source的11项通过；下载ZIP复核文件hash并实际启动生成项目health |
| 生成日志故障注入 | logging_test.go与模板运行测试 | 纳入本轮Go测试；append/轮转/保留、canary、brokenWriter与关闭stderr故障断言通过，不称真实磁盘满 |
| 外部local完整场景 | validation/scenarios.py --suite full | 9/9 passed，exit0；真实生成与运行，GitLab及损坏模型为明确mock |
| 外部真实Temporal | validation/scenarios.py --suite core --expect-execution-mode temporal | 9/9 passed，exit0；含真实CAN、并发和VERIFY中SIGKILL恢复 |
| SDK失败Update重试定向 | validation的model-retry套件 | 修复Temporal ApplicationError分类后2/2 passed，exit0；model provider为mock，真实Temporal及稳定业务ID重试 |
| 浏览器主线/定向 | 保存版validation/browser脚本，Playwright既有Chrome | 最终新binary 10/10与5/5 passed；桌面1440×1000、手机视口390×844，无报告console/横向溢出，详见浏览器册 |
| 开发启动入口 | npm run dev | 实际启动后端API18097及UI5173就绪；脚本默认local/rules/local，与独立后端默认temporal区分 |
| 当时设计结构 | design/_model/check.py --self-test --json | 135实体/502关系，七self-test与四guard通过；后续S0为136/519，本阶段ADR-047为137/529 |

本段保留迁移前历史事实；新Vue/审查结果见上方当前结论，未登记新结果的项目保持未验证；每项证据只证明其报告所用代码与配置，不把之后的修改自动包含在旧报告中。最后运输层错误分类另以定向报告验证，不将它当作重跑完整core/crash。

## 迁移前历史可审查证据（2026-10-02）

| 报告 | 实际结果与模式 | 来源 |
| --- | --- | --- |
| [local完整summary](../evidence/validation-local-summary.json) | local/rules/local核心及缺配置，另连local/gitlab与损坏model fixture；9/9、100次公开HTTP交换；交付ZIP含13个manifest文件及manifest本身，真实health logging=ok | `validation/reports/20261002T015956Z-4394ba5e6e/summary.json`，UTC01:59:56–02:00:59 |
| [真实Temporal summary](../evidence/validation-temporal-summary.json) | temporal/rules/local；9/9、722次公开HTTP交换；21条长消息触发CAN、11并发请求含9唯一命令；VERIFY观测后SIGKILL，约95秒恢复，同Workflow和execution | `validation/reports/20261002T015822Z-4ee5ba3e1f/summary.json`，UTC01:58:22–02:00:09 |
| [model重试定向summary](../evidence/validation-model-retry-summary.json) | temporal/model/local、provider为mock；2/2、13次公开HTTP交换；初次503/model_unavailable，同ID及payload重试到PLAN，成功重放不增provider调用，异payload409，真实Query仍PLAN | `validation/reports/20261002T020530Z-a33ae84a0b/summary.json`，UTC02:05:30；为新HTTP分类定向验证 |
| [浏览器主线](../evidence/browser-summary.json)与[竞态](../evidence/browser-races.json) | 最终local/rules/local新binary；10与5项通过；真实方案、修改、DONE、登记文件、ZIP、历史和窄屏 | 02:07:13Z保存版脚本；[浏览器命令、截图与证明范围](../evidence/BROWSER-QA.md) |

迁移前三个独立套件合计20/20所选case通过；不是全部45功能的验收计数。summary已复制为稳定文档证据，不含启动controller凭据；完整HTTP/ZIP/runtime日志保留在各自独立报告目录。私有launcher.json及其token不进入文档bundle。外部确认前断言针对公开产物/下载接口，不声称已经检查隐藏私有文件系统。最后HTTP/CI瞬断小修后的证据是对应定向Go测试、vet/build与保存版浏览器；未重复昂贵的完整Temporal故障注入。

BudgetPolicy在每Run定义中冻结：commands256、model尝试32含失败、files128、artifact bytes2MiB、每阶段尝试3。定义v2.0.0含TemplateHash、SkillVersions、Tools、Budget，model provider另冻结endpoint/model；所有通过required_checks需同source。GitLab账本sent/unknown恢复只读核实，远程文件/tree和准确SHA流水线、必需job/coverage/executable artifact为内部完成门；本轮远程协议仍为mock。

## 历史证据

早期实现曾通过go test -race ./...、go vet和go build，含当时的agent与已移除的cognitive实验模块。该事实仅说明当时代码和测试，不证明本轮三轴、独立版本、批准期限/hash、manifest或新日志全部通过。

早期真实Temporal场景报告在主机临时目录：

| 历史证据 | 曾证明的场景 | 限制 |
| --- | --- | --- |
| /var/folders/h3/6lpkk9815hvbrb6_54rdmj9c0000gp/T/agentstudio-integration-ec3krjca/summary.json | 真实Temporal、多轮/版本/幂等/SSE、worker SIGKILL、test/build/ZIP、CAN、并发与秘密canary | 当时代码；路径未复制归档，不能证明本轮或live第三方 |
| /var/folders/h3/6lpkk9815hvbrb6_54rdmj9c0000gp/T/agentstudio-integration-widck82t/summary.json | Postgres/GORM+JWT+logging生成、终止重做Query、暂停/拒绝、CAN及并发 | 未做执行中SIGKILL，不能替代上一轮或本轮故障注入 |

历史参考认知测试及来源校验不作为当前产品测试入口，也不为保留已删除耦合提供理由。

## 未配置与未覆盖

| 项目 | 当前边界 |
| --- | --- |
| live模型 | 未取得真实endpoint/key/model的本轮成功证据；mock或rules不得标成live模型 |
| liveGitLab/Runner | 未取得本项目真实namespace/token/首跑验证；mock结果不证明实际建仓或CI |
| 客户数据库迁移 | 本期范围明确排除，不是以连接或迁移冒充创建Agent验证 |
| 全部45功能 | 尚无逐项运行证据；完整可访问性、指标、传播、长期备份、多节点与生产鉴权分别验收 |
| 浏览器端到端 | 已有主线与定向实测；不涵盖完整断线恢复、键盘/读屏或物理手机认证 |
| 容器构建 | Go模板镜像固定1.25.0-bookworm；远端manifest查询网络超时，Docker build未运行，标签不证明容器验收 |
| 日志真实存储故障 | brokenWriter/关闭stderr是故障注入；未做真实磁盘满、长期保留或生产采集验证 |
| 生产部署/运维 | 无生产发布、外域名、租户鉴权、长期持久卷/备份恢复和监控验收证据 |

设计结构检查、产品内完成验证、mock/live第三方、外部场景和浏览器证据始终分别使用。缺配置/未运行必须明确保持BLOCKED或not_run，不能为完成任务降低冻结规范。

## 项目创建 Agent 与运行 Job 提示词核对

2026-10-02 18:54（Asia/Shanghai）完成本范围的只读源码核对，交付 [缺口清单与执行提示词](AGENT-JOB-EXECUTION-PROMPTS.md)：1份总提示词和8份分阶段提示词，覆盖完整定义/技能、建议与执行、消息/摘要、v2 API与运行工作区、运行预算/审计/readiness、独立场景和live第三方验证。Job按业务Run及Temporal Workflow/Activity理解，不新增通用任务DAG，不扩充静态审查图。

本轮另外定位到VERIFY失败修复/重批分支、local工具期限、unknown核实累计预算及真实就绪检查的差距；细节和源码入口在上述清单中。预算和部分运行观测的发布Schema也需先补契约，不能只向既有闭合对象增加字段。

本次没有修改产品/前端/validation运行代码，没有启动服务、运行产品测试或执行外部写。S1–S5及C01–C18继续保留原实施状态。当前目录没有Git元数据，代码时点用 [77份源码与Schema摘要清单](../evidence/agent-job-prompts-source-audit.json) 标识，聚合SHA-256为 `32256be2dd814c64869b30de6a86ce9cd8c2b5adb5dfb12f78c905b29b761959`；未登记commit，也不继承旧测试结果。

文档检查命令 `python3 design/_model/check.py --self-test --json` 通过：137实体、529关系、7项self-test和4项guard，报告见 [本轮文档检查](../evidence/agent-job-prompts-design-check.json)。该结果仅证明文档结构和冻结摘要一致，不证明提示词中的功能已经实现。live模型、liveGitLab/Runner、容器构建及新契约下运行验收均未在本次执行。

## 第1阶段 / S1：完整定义与技能冻结验收

2026-10-02，按用户“拆阶段执行，按1–8顺序推进”落实第1阶段。状态为本阶段contract_ready与定向path_verified；第2–8阶段仍未完成。工作区无Git元数据，代码时点以[83份产品/契约/oracle源摘要](../evidence/s1-source-audit.json)标识，聚合SHA-256为`bfa30a1eefe9cad5a859f5b5376499c6a100fbf7ba80222c629ac991db33f8a4`。

先按ADR-048发布[契约1.1.0](../../design/contracts/releases/1.1.0/README.md)：区分template_content原字节与模板清单引用、冻结六项新增期限/核实预算，并声明严格JSON资源界。原1.0.0三份Schema及PRODUCT-GUIDE/INVARIANTS字节保持。产品内contracts保存130个从完整Schema派生的Go类型、原字节Schema、离线Draft2020-12校验、严格JSON与JCS；definitions独立保存完整定义、三Skill、模板/Schema原字节、不可变发布和撤销记录。

默认产品定义3.0.1的hash为`45cfaa983628b2ca23c9e2eb9a537d7428a4ce1d6078ab68eae0f8e36de01608`。新Run创建时持久绑定完整Snapshot，模型请求读取冻结prompt，模板渲染读取冻结内容并检查技能写路径；参数默认值、Schema、依赖和冲突来自绑定Skill。兼容投影以frozen_snapshot_hash绑定来源，七类动作映射受控旧步骤，完成检查消费冻结check_id/kind/evidence集合；未知、重复、缺基线或证据不支持均阻止DONE。完整建议/授权执行对象仍待S2。

| 证据层 | 当前结果与入口 | 证明范围 |
| --- | --- | --- |
| 文档 | [S1检查](../evidence/s1-design-check.json)：138实体/536关系，7 self-test及4 guard通过 | 仅docs_ready与冻结摘要 |
| 派生类型 | `python3 scripts/generate-contracts.py --check`通过，130类型/3原字节Schema | Go发布副本一致；不代替运行语义 |
| 产品测试 | [全套](../evidence/s1-go-test.log)agent包123.164s；[race](../evidence/s1-go-race.log)agent263.942s/contracts1.737s/definitions33.155s；[vet](../evidence/s1-go-vet.log)与[build](../evidence/s1-go-build.log)通过 | 新冻结集成、24模板组合、两版本共存/重启、冲突/撤销、真实未知检查阻止DONE及既有后端回归；无本轮真实Temporal服务验收 |
| 独立oracle | [自检](../../validation/reports/contracts-s1/oracle-self-test.json)1862/1862，其中1748结构破坏负例；381引用；[JS JCS](../../validation/reports/contracts-s1/jcs-javascript.json)7向量通过 | 合成样本与oracle正确性，不是1862条产品路径 |
| 真实公开读取 | [rules定义](../../validation/reports/contracts-s1/http-s1-public-oracle/summary.json)、[model定义](../../validation/reports/contracts-s1/http-s1-model-oracle/summary.json)及各自重启后报告：四轮共52次GET交换通过 | 完整Snapshot/三Skill/三Schema/四模板原bytes、错误身份拒绝；两个真实发布版本升级由产品集成测试覆盖，未声称此HTTP用例覆盖 |
| 原来源恢复 | [rules原字节](../../validation/reports/contracts-s1/restart-frozen-bytes.json)、[model请求恢复](../../validation/reports/contracts-s1/model-restart-frozen-request.json) | 两Run快照及7依赖原文件重启不变；两次实际mock模型请求的首system原文/hash、model_id等于冻结来源；不是live模型 |
| 实际local交付 | [最终三技能路径](../evidence/s1-local-final-all-skills-path.json)：local/rules/local，Postgres/GORM/JWT/JSON日志，DONE，11项同源内部检查passed、18个ZIP条目 | 真实产品tidy/test/build/manifest/秘密/ZIP检查；本轮未额外解包启动、浏览器或容器验收 |

公开新增GET /api/definitions、/api/runs/{id}/definition和绑定content读取；创建允许精确definition_id/version/hash全量选择。缺历史完整来源返回409 legacy_contract_incomplete及legacy_read_url，旧pending不能新增批准；已有legacy批准与unknown账本继续限定原范围，不迁移成伪完整v2。JSON重复键、未知字段、非法数字/Unicode、越界数据拒绝。公开手册与后端说明同步。

配置与未覆盖：本轮Go1.25.5/darwin_amd64，隔离服务127.0.0.1:18127、mock provider18128；未使用真实模型/GitLab凭据或生产目标，测试进程完成后停止。definition/Skill/模板的工具平台随具体发布冻结，换平台/工具需要新发布身份，不能覆盖同版本。PostCommands完整声明及新增单工具/累计执行/CI等待/unknown核实预算已冻结但尚未全部接入执行；工具平台执行前复核、预留/结算及VERIFY修订重批在第2阶段落实。Message/Summary/v2 API/前端、Job审计/指标/readiness、真实Temporal恢复与容器/live均继续按第3–8阶段执行。

差异项仅关闭G01/G02/G03/G06的S1范围；G15/C06只通过旧来源拒绝和既有恢复回归切片，完整迁移仍未完成。没有把45份功能、历史20/20、15/15或本次52次HTTP交换声明为整个Agent完成。


## 第2阶段 / S2基础模块进度（尚未达到退出条件）

2026-10-02。先发布 [contract 1.2.0](../../design/contracts/releases/1.2.0/README.md) 与 [ADR-049](../../design/10-decisions/ADR-049-controlled-execution-ledger.md)，再实现对应模块。本记录仅覆盖基础模块及回归，**不关闭G04/G05/C07–C10，不代表实际job已采用新执行链**。默认产品定义仍为3.0.1/contract1.1.0；旧1.1发布原体与hash保持不变，新增1.2没有回写历史来源。显式选择未接通执行的发布创建新Run会被拒绝，不交给旧执行器。

### 本轮实际实现

- contracts完整派生147个Go类型、两个版本的6份嵌入Schema。codec和definitions按对象绑定版本验证；同一注册表保留1.1与1.2发布并可重开，不把旧Run切到最新Schema。
- governance纯校验器支持clarification/plan/action/diagnosis/completion及七类候选动作，绑定真实输入、上下文、观察用量、批准/权限、准确目标、固定调度expectedAction和预算。只有获准action生成完整ExecutionRequest；执行前复核入口保留原请求及当前权限，sent/unknown只返回原身份核实要求。
- 完整预算预留/结算与累计窗口，按稳定scope跨source/恢复计数；失败与unknown不释放，重复attempt不双扣，账本重算计数与hash。固定post_commands去重，版本/argv/cwd/environment一致，超时和次数采用各冻结声明最小值。Service已提供原子持久化预算接口，但实际调度器尚未调用。
- 完整S2AuditBundle和私有ControlState持久化；MessageEnvelope保存新输入原文，ContextRecord绑定实际有序上下文，Proposal/Approval/Permission保持完整字段。历史对象先登记、后写Run快照；私有索引同时封存逻辑引用与完整原体SHA256，防止被逻辑hash排除的生命周期字段遭改写。重启及读取校验完整索引和跨对象关联。
- 三个只读观察路由已实现：decision-audit、objects、budget。缺完整历史来源返回409及legacy读取地址；预算Link指向首次登记source的精确对象。模块测试使用新1.2发布、实际Store/HTTP handler和重启路径，当前默认创建主线不会补造这些对象。
- 模型传输入口解析五类严格DecisionAdvice并保留实际提交上下文；known零用量与unknown/null区分，缺失或不一致用量不填零。rules不能调用model transport。当前公开legacy规划入口仍只采用plan/clarification，后续动作/诊断/完成建议还未接入实际job。
- 交叉审查发现并修复：长原文误用较短fallback上限、被hash排除的归档状态可变、缺归档索引未拒绝、普通重提案自动恢复撤销权限、跨source决定/校验拼接。均有针对性负例，未通过放宽Schema回避。

### 证据范围与未完成工作

本轮Go全量、定向race、vet/build与派生一致性日志分别保存为 `docs/evidence/s2-foundation-*.log`，最终退出码和代码摘要以本节后续检查记录为准。该组检查与S1历史日志分开保留。

独立oracle 1.2自检1058项（918结构负例、140语义与正例）和502离线引用通过；1.1的1862项Schema自检与2750项决定链自检回归通过。只读采集器已完成自有synthetic HTTP的175次GET，含预算原source Link。报告位于 [contracts-s2](../../validation/reports/contracts-s2/)，全部是oracle/synthetic/mock证据，**没有连接产品S2执行服务**。缺历史状态观察时明确未覆盖，不从上下文自由文本反推动作签发阶段。

后续仍按第2阶段推进：将新消息/建议/批准/预算接入真实命令路径；local/Temporal共用七类动作请求执行器；固定工具身份、隔离cwd/env及deadline落到真实进程；完成按原operation核实及恢复计账；完成VERIFY同快照重试、未批准内容隔离候选、新source修订重批和耗尽终止；再运行C07–C10实际HTTP/副作用次数与完整对象oracle验收。此前不进入S3，不将legacy Execute布尔值、旧StepInput或上述模块通过当成S2完成。


S2基础本轮最终检查：全量 `go test ./...` 通过（agent 187.092s）；定向agent/storage race通过（agent 205.934s），最后追加的决定链权限回放修复再跑相关race通过（105.366s），存储原体大小上限再跑storage race通过（1.634s）。未就绪发布创建拒绝测试通过（3.253s）；vet/build及147类型/6份版本化Schema派生一致性通过。后两项为新改动的定向复核，不把较早全量日志说成包含后来新增断言。命令、退出码与日志见[检查记录](../evidence/s2-foundation-verification.json)。

当前代码以[181份源文件摘要](../evidence/s2-foundation-source-audit.json)标识，聚合SHA-256为`5aeeb07268dee3203036bce651c2c80ddda1f7b0ab2bc99b94f237c10bb291fc`。文档检查139实体/549关系通过，原PRODUCT-GUIDE和INVARIANTS摘要未变。没有新live或浏览器证据；第2阶段仍未退出。

## 第2阶段 / S2命令、动作接纳与实际工具接入进度

2026-10-02，继续第2阶段；S1状态不变，S2仍未达到退出条件，S3–S8不据此提前验收。下列结果对应新增`control_commands.go`、`control_execution.go`、`control_schedule.go`及`control_tools*.go`；旧基础模块日志不作为这些新代码的验证证据。当前默认仍是3.0.1/1.1，`definition_not_ready`保留，完整Control Run明确拒绝走旧ExecuteStep授权。

- 命令处理已接完整决定链：真实输入先随command预算持久化，模型实际请求/usage与原文引用绑定，明确P1/暂停/取消/恢复不冒充模型建议；成功命令的处理标记、状态及预算结算原子提交。失败调用计账，成功重放不再调用provider，同计划不升source、不刷新尚有效提案期限。重启把尚未结算的command/model尝试记为unknown/null，保留原预留。
- `PrepareControlAction`根据已批准计划及持久回执顺序生成七类准确候选，model动作建议真实调用冻结provider。先持久完整DecisionProposal/ContextRecord/ActionValidation/ExecutionRequest和ready operation；`beginControlExecution`再次核对原链、当前source/权限/工具/预算，原子预留action、设置首次应用时间并登记sent。伪造请求不能通过重算hash获得授权；sent/unknown保留原请求且不返回可写候选。
- 纯调度器只接受准确workspace/tool回执、完整manifest和原attempt结算；local无远程动作，缺证据不推进，动作耗尽不等于DONE。该组测试是完整合成对象/治理链的调度测试，不能当成实际GitLab或整个job。
- 3.1.0冻结无DB及六DB/ORM依赖图的go.mod/go.sum；3.1.1只更新Agent各阶段建议提示，仍引用3.1.0技能/模板。旧3.0.1与3.1.0资产原字节保留。63组合实际tidy/test/build及负例报告见[发布验证](../evidence/31-publication/publication-verification.json)和[矩阵](../evidence/31-publication/template-matrix.json)。这些命令使用本机默认CGO，不能推导受控CGO=0下SQLite已可用。
- 受控Go工具使用固定合并命令、实际Go二进制身份、Run内隔离候选/缓存/临时目录、进程沙箱和逐命令预算/完整回执。候选变化不写回批准工作区；原attempt缺回执按unknown消费而不盲重跑。macOS真实std项目工具与边界探针通过；受控SQLite两ORM、Linux沙箱及公开依赖下载完整路径仍需补齐。

当前已有根入口测试证据：[实际命令与动作接纳普通测试](../evidence/s2-control-admission-normal.log)及[定向race](../evidence/s2-control-admission-race.log)。普通动作接纳27.169s、命令/接纳race179.878s通过；race运行后又补充失败命令的持久错误说明及VERIFY包装，后续须以新定向结果验证，不能将此日志当成后来代码已全部覆盖。[本次文档检查](../evidence/s2-control-integration-doc-check.json)139实体/549关系通过；最终文档变更仍须重跑检查。

仍需完成：实际scaffold/逐Skill/VERIFY/package与远程三动作执行器和local/Temporal共用调度、VERIFY有限重试与内容差异新source重批、完整unknown读核实和原预算恢复、SQLite C工具支持，以及实际S2公开创建/批准/七动作的独立oracle与C07–C10验收。当前workspace/GitLab执行接入及VERIFY包装在开发中，未登记为已通过。没有新增live模型、liveGitLab、真实Temporal服务或浏览器验收；G04/G05及第2阶段退出门槛保持未关闭。

## 第2阶段 / S2修订重批、本地交付与只读核实进度

2026-10-03。按八份执行提示词计算，**已验收1个阶段，S2进行中，第3–8阶段未进入**。本节更新上方历史进度：scaffold、三Skill、VERIFY、package和本地完成报告已接入共享调度；受控push/CI及S2完整公开验收仍未完成，不能将模块数或测试断言数计成已完成阶段。默认公开定义继续为3.0.1/contract1.1.0；3.2.0/contract1.3.0创建门禁保留。

当前Go1.25.5/darwin_amd64代码以[208份源码、发布与oracle摘要](../evidence/s2-recovery-delivery-source-audit.json)标识，聚合SHA-256为`c80e0c835d9fe1dbffa70cd05d8e25f656a3fdf49823ce6573f81cd062f96830`。测试使用local/rules或明确mock模型/GitLab，未使用live凭据；PRODUCT-GUIDE与INVARIANTS原字节未变。

本轮已落实的行为：

- 1.3修复候选逐一绑定原执行、父子尝试、完整工具回执、实际预算结算和批准manifest；错source/operation/工具/环境/输出/差异、缺结算或unknown结算均拒绝。显式revise检查旧提案版本/hash，在同一事务完成新source提案和命令回执；重复命令不刷新批准。模型失败或改变候选以外业务内容时保留原BLOCKED/VERIFY与失败消费。
- 实际隔离tidy产生候选后，旧批准不能执行候选。重批、重启后通过共享执行入口完成新目录的真实tidy/test/build、ZIP和11项内部检查到DONE；原目录和旧批准历史保留。初始未使用依赖由测试在P1之前明确提供，完整Run也由测试入口建立；这是实际产品执行模块证据，尚非公开创建的完整S2场景。
- 1.3纯Go SQLite的GORM、sqlx各自冷缓存Run通过实际tidy/test/build；仅增加两份系统网络映射文件的只读权限，1.2边界保留。系统写、其他系统文件读取和shell探针仍拒绝。上述证据限本机macOS，Linux沙箱未验收。
- 1.3显式reconcile只接Run与命令身份，按原sent/unknown请求只读核实。模拟建仓响应丢失后实际HTTP POST只发生一次；取消后的GET观察接口不触发核实，显式核实只增加一次原项目读取，重复命令不重读、不重写，Run保持CANCELLED。该GitLab服务是mock。
- Workflow外层错误登记保留原BLOCKED恢复阶段，不将ResumeTo覆盖为BLOCKED；PAUSED和终态保持。该定向测试的状态检查点是明确构造的生命周期样本，不作为真实Temporal服务恢复证据。

| 检查 | 本轮实际结果 | 证据及范围 |
| --- | --- | --- |
| 修订、核实、沙箱与期限race | agent 581.333s通过 | [日志](../evidence/s2-recovery-delivery-race.log)；覆盖真实本地交付及mock外部失响应；编译早于后续Workflow外层错误修复 |
| Workflow外层错误race | 17.671s通过 | [定向日志](../evidence/s2-recovery-runtime-race.log)，验证后续修复 |
| 治理race、既有命令/恢复回归 | governance 149.065s；agent普通回归150.601s通过 | [治理race](../evidence/s2-recovery-governance-race.log)、[回归](../evidence/s2-recovery-command-regression.log)；不称全后端重跑 |
| SQLite两ORM冷Run | 160.529s通过 | [运行日志](../evidence/s2-sqlite-cold-runs.log)、[GORM公开对象导出](../evidence/s2-sqlite-cold-runs/sqlite_gorm.json)、[sqlx导出](../evidence/s2-sqlite-cold-runs/sqlite_sqlx.json)；截至VERIFY，未声称这两Run完成打包 |
| 完整对象导出独立检查 | 4份导出中284次逐对象校验；629离线引用；1712结构负例及3个hash负例通过 | [独立报告](../../validation/reports/contracts-s2/release13-exports.json)；含跨导出重复依赖，未覆盖公开HTTP创建、完整跨对象语义或RemoteExecutionReceipt实例 |
| 旧1.2 oracle回归 | 1058项通过，含918结构负例/140语义与正例，502引用 | [独立报告](../../validation/reports/contracts-s2/recovery-regression-1.2.json)；synthetic自检，不是产品路径数 |
| 派生与静态Go检查 | 174派生类型/9份原字节Schema一致；vet/build通过 | [派生日志](../evidence/s2-recovery-delivery-generated.log)、[vet](../evidence/s2-recovery-delivery-vet.log)、[build](../evidence/s2-recovery-delivery-build.log)；最后Workflow变更后已复核 |

规则重批交付的[完整公开对象](../evidence/s2-candidate-revision/rules-reapproval-done.json)与[原ZIP](../evidence/s2-candidate-revision/rules-reapproval-done.zip)、mock模型修订的[公开对象](../evidence/s2-candidate-revision/model-revision-plan.json)分别保留。测试最初因相对输出目录将文件写入包目录下，后按原字节迁入这些稳定路径，未改动对象、hash或ZIP。独立检查的首次缺文件失败已通过修正输入路径解除，未重跑真实工具来制造新结果。

S2接下来须依次闭环：固定Git真实执行与完整create/push/CI回执、CI原工件读取及完成检查集成；无候选的VERIFY参数修订、过期批准重提与终态新Run的完整控制链；当前1.3公开创建/批准/七动作与C07–C10独立验收；同一代码的真实Temporal Update/CAN/故障恢复。`ValidateControlRemoteReceipt`目前只有代码及编译证据，尚未集成验证，不算已完成。上述完成前不解除3.2发布门禁、不关闭G04/G05、不进入S3。没有新增浏览器、容器、live模型或liveGitLab/Runner证据。

本轮[文档检查](../evidence/s2-recovery-delivery-design-check.json)通过，140实体/561关系、7项self-test和4项guard；只证明文档及冻结摘要一致。详细日志身份与检查范围见[检查登记](../evidence/s2-recovery-delivery-verification.json)。

当前目标停止条件：用户于2026-10-03要求“完成第2阶段后停止目标”。S2尚未通过，因此继续本阶段；其完整退出条件通过后立即停止目标，不进入S3，也不将原八阶段目标宣称全部完成。

## 第2阶段 / 完整建仓回执接入

2026-10-03，继续同一项目创建Agent范围。本次只将`create_repository`与原操作核实接入已发布1.3完整回执，不新增通用Agent、Job/DAG或部署功能。当前[210份源摘要](../evidence/s2-remote-create-source-audit.json)的聚合SHA-256为`0443a34da38d4d531bcadabf4a2136c3662a59b8ac46c4d234ba82875a00eb85`；与前一节208份时点分开记录。

- 实际GET的完整`RemoteReadObservation`与预算结算原子保存，发送前记录dispatch边界。建仓成功或unknown生成完整`RemoteExecutionReceipt`，后续核实追加成功回执，原unknown字节及消费保留。完整回执、操作证据引用与私有调度索引同次提交。
- Store写入/恢复校验远程观察和完整回执；1.3调度与幂等重放必须具备准确的公开成功回执，不能只消费私有成功标记。无论成功或unknown，非null项目等事实都必须来自实际记录的观察。假source、请求、批准、manifest、项目/marker、读路径/hash/预算、重复读、发送标志、attempt和时间在重算hash后仍拒绝。
- namespace预读没有1.3公开payload分支，记录其实际HTTP状态及unavailable/null，未伪装成project。缺字段或包含执行器凭据的响应不保存正文，不授予成功。此限制及前置读取时间语义见[接入说明](S2-CONTROLLED-GITLAB.md#13完整回执接入规则)。

| 检查 | 当前证据 | 限制 |
| --- | --- | --- |
| 实际本地前置工具＋mock建仓失响应、取消后核实race | [146.145s通过](../evidence/s2-remote-create-receipt-race.log)；一次POST，成功重复命令不增加GET，CANCELLED保持，15类重算hash负例拒绝 | 编译早于随后追加的unknown非null事实校验；不是live或公开创建 |
| 建仓模块与既有1.2路径回归 | [209.754s通过](../evidence/s2-remote-create-regression.log)；包含3.2成功、重启、凭据响应拒绝和新增unknown事实负例 | 此组建仓测试的上游workspace/Go证据为明确synthetic；前一行才含真实上游工具 |
| 独立1.3导出检查 | [5份导出报告](../../validation/reports/contracts-s2/release13-remote-create-exports.json)：346次完整对象校验、629引用、2324结构负例及5个hash负例通过；新增远程unknown/succeeded两个分支 | 对象次数含重复依赖；导出中的远程系统为mock；不代替完整跨对象语义或公开HTTP场景 |
| 静态检查 | [vet](../evidence/s2-remote-create-vet.log)、[build](../evidence/s2-remote-create-build.log)通过 | 包含最后的unknown事实校验，未重跑无修改的SQLite冷Run |

真实工具＋mock远程的[完整公开对象导出](../evidence/s2-remote-create/cancelled-create-reconciled.json)保留CANCELLED、原unknown和新成功回执。检查命令和代码时点限制见[登记](../evidence/s2-remote-create-verification.json)。`ValidateControlRemoteReceipt`现在已在create分支、Store与调度接入并有正负例；push/CI分支仍只有待接入代码，不算验收完成。

S2剩余退出项与停止条件保持：完成受控Git push、CI及同源工件/完成检查，补齐剩余修订/批准/终态新Run路径，再开放并验证当前公开主线、C07–C10及真实Temporal恢复；通过后停止目标，不进入S3。


## 2026-10-03 用户要求立即收尾并停止目标

最新指令为“耗时太久了，不对劲，感觉收个尾结束”。它覆盖此前“第2阶段完成后停止”的等待条件；本轮停止继续实施，不进入S3–S8，不把尚未完成的S2标为通过。当前仍为**S1已验收、S2未验收**，公开默认保持3.0.1/contract1.1.0，3.2/1.3创建门禁保留。

本轮范围仍只有项目创建Agent及其Run/Temporal Job。新增部分是生成项目的固定Git交付：六条冻结命令在Run私有副本执行，准确head/tree和完整远端文件读取绑定批准、manifest、请求、工具和预算；失响应后只能读取原提交，取消不会撤销已发生的写，也不会因核实成功恢复执行。没有新增通用Agent平台、Job/DAG、客户数据库迁移或生产发布。

[本轮源摘要](../evidence/s2-controlled-push-source-audit.json)包含215份文件，聚合SHA-256为`b3cf46eb7f169c2503ee38e81c955eea0e93297535eaea0ddeb37c59cefc8fa4`；[收尾检查登记](../evidence/s2-controlled-push-closeout.json)记录配置、日志和未覆盖项。

- 普通测试中的真实Git正常推送、失响应后取消核实、重启幂等及两分支各10项重算hash负例通过（137.21s）。上游workspace/Go回执为明确synthetic；实际Git命令及本机bare远端真实，GitLab API为mock，不是live Runner验收。
- [包含这些子测试的原整组日志](../evidence/s2-controlled-push-tests.log)结果为失败：第一次启动检查点恢复fixture未复制不可变对象。测试夹具已补复制实际检查点对象，不改产品断言；修正后的race按用户要求中断，**不能声称恢复测试已通过**。
- [沙箱检查](../evidence/s2-controlled-push-sandbox.log)2.778s通过，覆盖六命令、远端SHA、凭据不落仓、无关文件读写及其他端口拒绝。macOS内核仅能限制端口/localhost，精确远端主机由冻结参数、禁止重定向及隔离配置约束，未宣称逐IP内核白名单。
- 最后产品源码的[vet](../evidence/s2-controlled-push-vet.log)和[build](../evidence/s2-controlled-push-build.log)退出0。后续只改测试、独立检查的分支采样和说明文档。
- 三个仍在运行的检查均只终止本任务进程组，退出143：[推送race](../evidence/s2-controlled-push-race.log)、[检查点恢复race](../evidence/s2-controlled-push-recovery-race.log)、[建仓及旧路径回归](../evidence/s2-controlled-push-create-regression.log)。它们是用户要求中断，不能记成通过；原失败日志保留。独立导出检查增加按action_type区分样本，但未重新运行，不新增oracle通过声明。

未完成项明确保留：CI与远程完成检查；完整检查点恢复/最终race回归；剩余参数修订、过期批准和终态新Run路径；公开1.3主线、C07–C10及真实Temporal验收。冻结文件保持原字节，差异项不以本轮模块测试关闭。后续需用户重新授权才继续。

## Git 忽略规则整理（2026-10-05）

按本次用户要求整理根目录与 `validation/.gitignore`：补充 Go 编译产物、Python 缓存与虚拟环境、本地环境配置及编辑器临时文件；环境配置示例、发布契约、审查 bundle 继续保留在可提交范围。已有 `node_modules`、前端构建目录、Run 数据和独立场景报告继续忽略。

首次整理错误地为 `docs/evidence/**/*.log` 添加例外，导致256份日志重新进入可提交范围；另新增1份维护检查报告、忽略1个后端二进制，候选文件从760增至1016。按用户后续反馈撤销日志例外并移除本次新增的维护检查报告，候选降至759个。256份原日志仍在本地，默认由 `*.log` 忽略；结果直接登记在本节。

继续核对提交范围后，将仅位于 `docs/evidence/` 的34张PNG截图、3张SVG导出图、2个生成项目ZIP和7份原始JSONL，以及可重新导出的 `docs/DESIGN-REFACTOR-OUTPUT.md` 纳入忽略。47份附件均留在本地，候选由759降为712；这些附件不随源码提交时，历史文档中的附件链接需使用本地归档。验收结论、结构化摘要、前端实际消费的审查发布数据、公开契约、测试夹具及设计源图仍保留在可提交范围。

本次忽略规则检查时（首次提交前），`main` 尚无提交。证据为本节的检查记录及实际规则文件：[根规则](../../.gitignore) SHA-256 `35c52bb5362062e6657732f954cb789388ec603ef0e720ba4c691e26a9b17971`；[场景规则](../../validation/.gitignore) SHA-256 `b363616edc89acf6df35f0b516eb5f4350215a928c7f322f4966038dc72292fd`。完整候选清单对比确认本次恰好排除上述47个文件、无其他变化；`git check-ignore --no-index -q -- <path>` 对这47个实际附件及16个应保留路径全部符合预期。冻结原文和不变量摘要未变。

文档结构复核：`python3 design/_model/check.py --self-test --json` 退出0，142实体、576关系、7项自检及4项守卫检查通过。验证范围限于忽略规则和离线文档结构，不改变产品阶段、集成或 live 验收结论。

## 首次 Git 提交整理（2026-10-05）

按本次用户“整理提交顺序及内容，然后执行提交”授权，对现有712个候选文件建立初始版本历史。按依赖顺序分为四组：规范与仓库约定210个文件；Go/Temporal后端及生成工具220个文件；Vue工作区、发布数据与当前审查生成直接使用的7份摘要共79个文件；独立验证、验收追溯和本状态记录203个文件。已有功能作为当前工作区基线纳入版本管理，不将初始导入表述为本轮新实现，也不改变先前阶段验收范围。

本轮代码时点、逐文件SHA-256、明确路径清单、命令输出及实际暂存字节核对保存在本地忽略目录 `.data/git-commit-audits/20261005-f2dedjkc/`；`initial.json` 记录执行前文件，分组 `.paths` 绑定提交范围。除本节登记及对前述“尚无提交”检查时点的澄清外，产品、契约、模板与既有证据字节保持原样。凭据、依赖目录、运行数据、编译产物及已忽略的历史附件不纳入提交。

- Go/TS/runtime派生契约 `--check`、独立Oracle对1.1–1.4及runtime1.0的元Schema、原字节、摘要与离线引用核对通过；前端审查发布 `--check --self-test` 通过。
- `go -C backend test -vet=off -run '^$' ./...` 仅编译测试包；随后 `go vet ./...`、`go build ./...` 通过。`npm --prefix frontend run build` 包含Vue/TypeScript检查并通过；Shell语法及48份Python源码语法检查通过，265份候选JSON可解析。
- 旧 `validation.contracts.publish --check` 对1.1–1.3通过，对1.4因旧序列化格式不包含新增 `contract_version` 而拒绝清单。保留原失败记录 `external-schemas.log`；1.4的版本、完整文件集合、Schema身份、路径、大小、SHA-256及原始字节另行严格核对通过，现有Oracle也正常验证该发布。没有删减1.4清单字段来迁就旧工具。
- 本轮未执行完整Go运行测试、race、公开API场景、浏览器、真实Temporal或live第三方；历史通过与外部阻塞保持原时点，不将编译和离线检查替代这些验收。
- 逐组暂存核对文件集合、模式及Git blob原字节与已审阅清单一致；前端暂存的bundle/Schema摘要和依赖锁声明也单独核对。`git diff --cached --check` 对旧规范和静态图库给出文件末尾空行提示，详见各组 `*-whitespace.log`；本轮保留原发布内容和图库来源字节，不将这些样式提示记成无警告通过。

提交前文档结构检查：`python3 design/_model/check.py --self-test --json` 退出0，142实体、576关系、7项自检及4项守卫检查通过，输出见本地审计目录的 `document-model.log`。
