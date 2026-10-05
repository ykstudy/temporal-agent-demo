# 运行观察发布 1.0.0

2026-10-05，ADR-052。此发布是独立的只读运行协议，完整格式见 [Schema](runtime.schema.json)。只引用冻结1.4.0的Common/Agent对象；不改写已有RunSnapshot/Definition、不授予权限、不驱动DONE。Go/TS/外部oracle由发布Schema生成并核对原字节摘要，运行时只用产品内嵌副本。

## 入口与完整性

- `GET /api/runtime/live`：完整RuntimeHealth；只证明HTTP进程可响应，ready=false、其他组件not_applicable，不做依赖探测。
- `GET /api/runtime/ready`：完整RuntimeHealth；就绪200，否则503。实际有界存储读写/fsync探针、Temporal health/namespace、当前自有Worker状态及该identity最近90秒内的workflow/activity poller，Go与进程沙箱可用。model/GitLab只核对配置并标configured_unverified，不能声称live连通；默认模式缺必需配置则不就绪。探针绝不创建Run/模型请求/项目/CI。
- `GET /api/v2/runs/{id}/runtime`：RuntimeView，完整快照ref与观察时间；Query绑定一次Describe得到的确切执行段，不与另一段拼接。Query不可用时保留显式unavailable。Query序号落后持久业务快照为stale。检查点时间只取Workflow确定性时间，不能以HTTP时间补造。
- `GET /api/v2/runs/{id}/audit`：RuntimeAuditTrail，完整按sequence递增的追加事件链。旧Run未从创建时采集则coverage=unavailable、events=[]，不从日志补造旧活动。
- `GET /api/runtime/metrics`：RuntimeServiceMetrics，聚合具有完整采集的Run，列出排除旧Run。取消、暂停、阻塞不计失败。

查询不接受未声明参数。错误复用1.4 ErrorResponse；legacy来源不足仍409/legacy_read_url。所有运行对象是完整闭合Schema；GET不改变业务、预算或状态。live返回200只表明存活，ready必须用专用入口。

## 事件、关联与保留

新完整Run从创建事务开始保存追加事件，涵盖需求受理、命令回执、决策/授权、批准、操作转换、验证报告、首次终止、实际Activity开始/结束及恢复。业务Run、command、operation、approval、execution与Temporal执行段/activity/attempt分别关联。白名单参数仅包含动作名和已发布参数键；summary_hash只覆盖此白名单对象的action和argument_keys，不包含秘密、用户自由文本、URL或环境值。证据引用指向原完整对象，状态序号是该事件提交时的序号；event sequence独立且全Run递增。

RuntimeAuditEvent.event_hash为JCS去除顶层event_hash后的SHA256，previous_hash是上一条event_hash，首条null。RuntimeAuditTrail.audit_hash、RuntimeView.view_hash、RuntimeHealth.health_hash、RuntimeServiceMetrics.metrics_hash分别只排除自身顶层hash字段。数组顺序保留，null不省略；时间也参与响应hash。无日志轮转或隐式清理；单Run最多32768事件/4096实际尝试，超限拒绝新工作并保留已登记事实。

事件和业务对象同一单写者pending快照提交，跨重启/CAN不重编号、不补造成功；不可修改已提交事件前缀。提交失败对调用者明确返回存储错误。恢复时未完成Activity标unknown且追加recovery，保留原attempt身份；这只说明进程失去结果，外部operation须继续原身份核实。心跳是当前process实际计时上报或Temporal服务器读取，和持久活动结果分别标来源；超过45秒为stale，无值为unavailable。结束尝试不再显示实时心跳。心跳不证明效果成功。

## 指标与失败

成功/失败Run分别只计首个DONE/FAILED终止事件。CANCELLED为独立终止，冻结耗时但不计失败。elapsed从创建至首终止，未终止到observed_at，包含暂停/阻塞。Activity重试只计真实Temporal执行中attempt>1的独立attempt_id，不计HTTP重投或控制命令，local调度标mode=local/attempt=1。验证失败按不同report_hash且outcome=failed的正式报告（not_ready不计失败）计数。有效轮次按持久受理的用户需求command_id去重，排除progress查询和控制命令。旧Run无完整事件则指标null/排除，不能填0冒充历史事实。

Diagnosis优先原unknown operation、当前明确失败报告、拒绝命令或Activity错误；类别覆盖输入、模型协议、授权、工具、基础设施、业务验证、unknown、预算和取消。保留步骤/原身份/证据/待补条件；retryable仅表示发布建议操作在现有预算内可用，不能授权写重试。未知写只提供原operation账本读取和reconcile操作提示。恢复仍由原控制平面复校。

## 期限、恢复及兼容

RuntimeBudgetStatus引用原完整BudgetPolicy/当前BudgetLedger及首次执行窗口deadline。单工具、总执行、CI和核实限额仍来自原冻结定义/已消费账本，resume/CAN不重置。上下文采用保守UTF-8字节上界加冻结输出/协议预留，完整当前输入/系统/权威层不可截断；观察值不冒充供应商token结算。

单写者锁覆盖数据目录，关机先拒绝新提交、HTTP有限等待，再取消local/停止自有Worker、释放存储。数据备份仅在服务完全停止并持有目录独占锁时复制整个目录：runs快照/两种原文/pending、objects、definitions、workspace、downloads以及操作/预算/批准均在一致性范围。凭据不在备份中。恢复使用隔离副本，先离线摘要核验，再启动产品执行自身来源/引用/原文验证；损坏或缺依赖拒绝启动，不补造。未知外部写保留原身份，终止不复活；详情由恢复规程和实际故障证据单独登记。

验证尝试结束后，1.4完整采集Run还发布本次完整VerificationReport：已实际运行的固定工具根据原回执填passed/failed，待执行检查保持not_ready，unknown不写failed；内容变化待复批仍not_ready。正式报告保留原版本和工具来源，最终完成报告仍需全部检查及交付链通过。
