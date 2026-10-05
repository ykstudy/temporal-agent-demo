# 原文来源与五层归宿

审阅日期：2026-10-02。来源为 [docs/PRODUCT-GUIDE.md](../PRODUCT-GUIDE.md)，共 1313 行；本次不改原文。SHA-256 为 `51c889d53a694e1047d9a9a168ef2fe753faa50a68519c0ebbcaa59a6895a45b`。

引用使用“章节 + 固定快照行号”；下表是一份来源追溯表，不能把示例代码误读成新规范。实质纠错见 [审校裁决](REVIEW-DECISIONS.md)。表中 L1–L5 分别指不变量、领域概览、功能契约、ADR、结构模型；具体规则、字段、状态与错误语义以本项目五层文档的最终登记为准。

## 当前项目领域登记

九域为当前项目自己的职责边界，不复制参考 harness 的九域。L2/L3 路径和 L5 domain 身份对应如下：

| 领域路径 | 结构模型身份 | 来源重点 |
| --- | --- | --- |
| `design/01-orchestrator` | `domain.orchestrator` | §3、4、5、8、13：串行控制、状态、恢复、操作身份 |
| `design/02-agent` | `domain.agent` | §2、3、5、6：语义建议、动作协议、Definition 与上下文 |
| `design/03-skills` | `domain.skills` | §2.3、7.1–7.2：模板、参数、工具和工件验证 |
| `design/04-integrations` | `domain.integrations` | §4.4、7.3–7.5、8.3：GitLab 精确目标、提交和 pipeline |
| `design/05-conversation` | `domain.conversation` | §4.2、5.4、6、13：消息、原文/摘要、预览和确认 |
| `design/06-governance` | `domain.governance` | §2.2、7、8、9：权限、预算、密钥、路径和批准 |
| `design/07-observability` | `domain.observability` | §6.4、9.3、10、13：追踪、审计、指标和验收证据 |
| `design/08-frontend` | `domain.frontend` | §3 用户侧、5.4、10.1、12–13：对话/确认/进度展示与用户端验收 |
| `design/09-deploy` | `domain.deploy` | §1.2、6.1、9.2、11–13：当前服务配置、依赖 readiness、持久卷和交付 |

全章表中的 Runtime/执行、仓库/CI、验证、运维等描述按上述九域定位公开契约；不是额外 domain。

## 全章覆盖

| 原文章节与行号 | 保留的来源含义 | 裁决/补全 | 五层归宿建议 |
| --- | --- | --- | --- |
| §1 概述，L3–23 | Temporal 持久项目创建 Agent；Go、GitLab、JSONL 的初始技术方向 | R-002、019、023：SDK 固定；密钥边界；持久与本地模式区分；选型示例不证明已安装或配置 | L1 模式/密钥；L2 产品与运行边界；L3 readiness；L4 技术基线；L5 Definition/Mode |
| §2 核心概念，L24–85 | Definition、Run、ContextBudget、三个 Skill | R-003、011、015、020：稳定身份、不可变定义/确认版本、预算、技能权限 | L1 不漂移/预算；L2 Agent/技能；L3 Definition/Skill/Proposal；L4 快照和确认；L5 Run/Version/Budget |
| §3 系统架构，L86–141 | 客户端、Temporal、Worker、外部系统职责 | R-001、002、010：确定性编排与语义/执行分离；消息入口分工 | L1 确定性；L2 编排/对话/执行/外部集成；L3 API/执行边界；L4 分层；L5 依赖图 |
| §4 数据模型，L142–279 | Run 状态、消息、Activity 小对象与工件引用 | R-003–007、011、017–018：消息/操作身份、批准与产物链、精确外部目标、存储引用 | L1 稳定身份/证据；L2 数据归属；L3 消息/操作/工件契约；L4 身份分离；L5 核心实体关系 |
| §5 Workflow 设计，L280–588 | 串行协调、用户输入、Activity 调度、Continue-As-New | R-001–002、010–013、021–022：SDK 原语、合法转移、批准、检查点与失败处理 | L1 确定性/状态/授权；L2 编排；L3 lifecycle/message/CAN；L4 调度/迁移；L5 State/Event/Checkpoint |
| §6 对话与压缩，L589–767 | 外部原消息、层级上下文、摘要、传播 | R-003、014–016、018：保留原文、权威状态不靠摘要、预算硬裁剪、传播和持久语义 | L1 原文/预算/标识；L2 对话与可观测性；L3 store/context/summary/propagation；L4 存储与预算；L5 Message/Summary/ContextLayer |
| §7 Activity，L768–1152 | scaffold/skill/repository/CI/verify 五类业务副作用 | R-005–009、017、019–020：工作区与 manifest、秘密边界、精确提交/CI、固定必需检查 | L1 工件/工具/证据；L2 Runtime/仓库/CI/验证；L3 各 Activity 契约；L4 外部身份/原子写入；L5 Artifact/Invocation/Verification |
| §8 错误，L1153–1202 | 基础设施、业务、永久失败与结果未知 | R-006、021–022：安全重试和未知结果分开；attempt/total timeout；等待/终止 | L1 有限重试/结果核实；L2 编排；L3 error/reconciliation；L4 恢复分类；L5 ErrorClass/ResultState |
| §9 安全，L1203–1234 | env 密钥、权限最小化、工作区和审计 | R-011–012、017、019–020：版本绑定授权；禁止凭据 remote URL；工具/路径限制 | L1 密钥/授权/隔离；L2 治理；L3 approval/secret/audit；L4 受控执行器；L5 Grant/Approval/AuditEvent |
| §10 可观测性，L1235–1254 | Temporal history/Query/heartbeat 与业务指标 | R-002、016、023–024：观察语义、同步边界、模式区分与证据分类 | L1 真实状态；L2 运维/前端；L3 query/stream/metrics；L4 证据记录；L5 Snapshot/Trace/Metric |
| §11 目录结构，L1255–1291 | 工程模块候选布局 | R-024：代码目录是实现参考，不是跨域私有类型依赖契约，不证明对应文件已实现 | L2 领域边界；L3 公开输入输出；L4 工程组织；L5 模块/实体关联 |
| §12 实施计划，L1292–1303 | 六阶段建设顺序和预期验收 | R-023–024：保留来源计划；当前状态另册，不将周次或脚本存在视作完成 | L2 交付范围；L3 acceptance；L4 路线图；L5 Milestone/Evidence |
| §13 验收，L1304–1313 | 多轮、崩溃、幂等、压缩、CI、安全、CAN、并发八种场景 | R-003–024：加入反例、精确副作用身份、审批旧版本、存储/模式和浏览器证据边界 | L1 所有关键规则；L3 各功能验收；L4 验收策略；L5 AcceptanceCase/EvidenceRecord |

## 关键证据定位

| 主题 | 原文精确定位 | 对应裁决 | 不应从原文推导的错误结论 |
| --- | --- | --- | --- |
| Definition 不漂移 | §2.1 L28–39 | R-011 | 只有一个 version 字符串就自动锁住模板/技能 |
| 小对象、引用 | §4.3 L181–204 | R-018 | 对话存本机就自动跨 Worker 恢复 |
| 错误幂等身份 | §4.4 L239–248；§7.3 L933–934 | R-004 | Temporal RunID 跨 Continue-As-New 不变 |
| 原生 channel | §5.2 L318、328、347、388–391 | R-001 | 普通 Go channel 能直接承担 Workflow 调度 |
| 过时 Update 调用 | §5.4 L580–585 | R-002 | SDK 当前仍接受该参数列表，且只返回 error |
| CAN 字段/队列 | §5.2 L369–381；§5.3 L538–554 | R-013 | 迁移 State 就保证 pending handler/消息和 Description 不丢失 |
| JSONL 错误与权限 | §6.1 L600–637 | R-018 | 0644 + Append 即构成可靠审计存储 |
| UTF-8 与 token | §6.2 L681–695 | R-015 | len/4 能约束所有语言 token，截前缀能保护必要约束 |
| 摘要覆盖原文 | §6.3 L723–727 | R-014 | 为压缩工作上下文可以删除原始历史 |
| 传播空实现 | §6.4 L740–764 | R-016 | 四个方法存在就是传播链已完成 |
| marker 幂等 | §7.1 L778–784；§7.2 L867–874、914–915 | R-017 | 存在标记即可证明当前版本全部文件生成成功 |
| 带密钥的 remote | §7.3 L962–965 vs §9.1 L1207–1208 | R-019 | 凭据从 env 读取就不会写进 `.git/config` 或进程参数 |
| 任意 post hook | §7.2 L907–910 | R-020 | Skill 能执行字符串就有任意 shell 权限 |
| CI 报告缺失 | §7.4 L1044–1060 | R-008 | go build 自动产出 bin，coverprofile 自动产出 coverage.xml |
| 允许重复 CI / 最新 | §7.4 L1073 | R-007 | 最新 pipeline 成功就是该批准提交成功 |
| 检查项消失 | §7.5 L1093–1119、1138–1147 | R-009 | pipeline_id 为空时可以省略 CI 然后 AND 其余项完成 |
| 错误重新决策 | §8.1 L1160 vs §5.2 L440–444 等 | R-021 | NonRetryableApplicationError 会自行让 Workflow 重新规划 |
| timeout 绝对化 | §8.2 L1179 | R-021 | StartToClose 是唯一合法必需 timeout 选项 |
| 权限/临时目录 | §9.2 L1214–1216 | R-017、019 | 固定 `/tmp` 路径与两个 scope 足以保证最小权限 |
| 计划与产品状态 | §12 L1296–1301；§13 L1306–1313 | R-023–024 | 列出验收、存在代码或参考副本就证明前端/外部系统已完成 |

## 来源层级与使用约束

1. 本项目用户授权的范围优先；早期文档保留阶段之后，最新授权允许删除参考运行耦合并实现当前 Agent，见 [ADR-045](../../design/10-decisions/ADR-045-agent-model-validation-separation.md)。
2. 新五层文档中的裁决是当前项目的有效规范；本表将原文线索送往该规范，不反向以原文错误示例覆盖裁决。
3. ai-agent-harness 的五层组织是历史方法来源；本项目不再保存或加载参考副本，不要求该项目路径可访问。当前 `design/_model` 是独立结构登记，参考产品的实体数量、规则和权限不成为当前运行依赖。
4. `docs/design/DESIGN-SPEC.md` 和 concept PNG 是此前 UI 方向草案；无可运行 UI 时只能作为设计材料。
5. 代码、测试脚本与历史运行证据的效力仅限具体实现行为；见 [IMPLEMENTATION-STATUS.md](IMPLEMENTATION-STATUS.md)。不把“尚未实现”改写为设计缺陷，也不把“已经实现”改写成既定规范。

## 本次独立运行归宿

- 产品输入、输出、公共能力及强依赖统一见 [Agent 使用契约](../AGENT-GUIDE.md)，实现受当前45份功能规范约束。
- [设计模型](../../design/_model/SCHEMA.md) 保留离线结构检查；删除参考运行模块没有降低冻结规则或验收。
- [外部 validation](../../validation/README.md) 使用公开接口调用场景，不依赖内部包；它与产品自身 test/build、manifest 及远程 CI 完成验证分开。
- 原始 PRODUCT-GUIDE 和当前 INVARIANTS 的原始字节、来源行号与冻结摘要不因该调整变化。
