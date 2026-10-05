# 项目创建 Agent — 五层设计入口

2026-10-02 有效基线。当前有效文档包含：36 条不变量、9 个领域、45 份功能契约、51 条 ADR，以及 142 个结构实体、576 条关系和 15 类检查目录。ADR-048发布契约1.1.0并推进S1冻结定义；ADR-049发布契约1.2.0的受控执行、预算和审计结构；ADR-050发布1.3.0固定交付工具与完整回执/候选，S2实际运行已取得独立阶段退出证据；ADR-051发布1.4.0完整原文、摘要尝试及持久回执，S3验收单独登记。ADR-047的Vue等价工作区与十一类静态只读审查保留此前137实体/529关系的发布时点；v2 Agent结构按S1–S5推进。45 份功能是设计覆盖，不代表全部运行完成。产品内验证、外部场景验证与离线文档检查分别登记。

原始输入是 [docs/PRODUCT-GUIDE.md](../docs/PRODUCT-GUIDE.md)。参考 `ai-agent-harness` 的文档组织与审校方法，当前九域和业务规范围绕本项目重新划分。参考项目仅为历史组织方法来源，参考副本及实验认知运行模块已按最新授权移除；原项目未被修改或合并。当前 Agent 不依赖外部项目路径，有效规范入口是根目录 `design/`，解耦决定见 [ADR-045](10-decisions/ADR-045-agent-model-validation-separation.md)。

## 建议阅读顺序

后续实施先完成 [公共对象契约](contracts/README.md)、[职责映射](contracts/MODULE-MAP.md)、[结构差异](../docs/analysis/CONTRACT-GAP-MATRIX.md) 与 [阶段路线](IMPLEMENTATION-PLAN.md)。本阶段按 [ADR-047](10-decisions/ADR-047-vue-readonly-review.md) 推进Vue等价迁移及[只读可视化审查](review/README.md)；当前API仍是legacy，目标schema_version=2按S1–S5独立实施验收。新Vue迁移浏览器15/15通过，审查/导出的最终QA另行登记；完整创建路径或图可见不表示已符合Definition/Skill/决策/消息/摘要结构。

1. [业务主线](BUSINESS-FLOW.md)：用户提出需求后怎样推进、每步产出、确认点和失败处理。
2. [有效不变量](00-invariants/INVARIANTS.md)：不可静默修改的规则、十一态表、门禁、权限矩阵与反模式。
3. 下方领域概览及各功能：公开输入/输出、异常和验收契约。
4. [ADR 索引](10-decisions/INDEX.md)：选择理由、备选、影响和本次修正。
5. [模型 Schema](_model/SCHEMA.md)：结构字段、契约关系、冻结与校验范围。

## 五层职责

| 层 | 产物 | 维护边界 |
| --- | --- | --- |
| L1 不变量 | [INVARIANTS.md](00-invariants/INVARIANTS.md) | 本次审校后冻结；后续规则变更有ADR、明确批准和新基线 |
| L2 领域 | 九个 overview | 职责/不做什么/关系/改动节奏理由/功能清单 |
| L3 功能 | 45份 features | 固定八节，只依赖其他功能公开输入或输出 |
| L4 决策 | [50份ADR](10-decisions/INDEX.md) | 状态/背景/决策/理由/备选/影响/关联/历史；每项关联域和功能 |
| L5 结构模型 | [entities](_model/entities.yaml)、[relations](_model/relations.yaml)、[constraints](_model/constraints.yaml) | 只存身份、路径和结构关系，不复制正文或运行状态 |

## 领域划分与理由

| 领域 | 职责 | 按改动节奏划分的理由 | 功能数 |
| --- | --- | --- | --- |
| [编排](01-orchestrator/overview.md) | 状态、命令排序、调度重试、检查点和Run生命周期 | 控制流程与恢复策略共同演进 | 6 |
| [决策与上下文](02-agent/overview.md) | Definition快照、模型建议、动作校验、上下文与摘要 | 模型协议与记忆预算共同演进 | 5 |
| [模板与技能](03-skills/overview.md) | 脚手架、数据库/认证/日志能力、模板版本和工件 | 能力参数、模板与生成验收共同演进 | 6 |
| [GitLab与CI](04-integrations/overview.md) | 仓库身份、推送、首跑、验证和未知结果核实 | 外部平台API、凭据能力与副作用恢复共同演进 | 5 |
| [对话与确认](05-conversation/overview.md) | 会话Run、原文JSONL、富消息、预览确认和工件浏览 | 消息协议、用户确认和展示数据共同演进 | 5 |
| [治理](06-governance/overview.md) | 权限密钥、工作区、幂等并发、规则与预算 | 安全策略和确定性放行条件共同演进 | 5 |
| [审计与证据](07-observability/overview.md) | 追踪指标、验证报告、失败归因与证据边界 | 证据口径和业务追溯共同演进 | 4 |
| [展示与交互](08-frontend/overview.md) | 对话工作区、进度产物、模型审阅、状态同步和可访问性 | 用户体验、渲染与响应式行为共同演进 | 5 |
| [本服务配置交付](09-deploy/overview.md) | 三轴模式、服务运行、可复核交付、健康备份恢复 | Agent服务配置和运维条件共同演进，不是生成项目生产发布 | 4 |

编排消费治理裁决与验证报告；模型提供建议而无控制权；技能产出工件，集成处理精确外部目标；对话保存原文与确认；前端消费权威快照。功能关系存在反馈，不等同运行 Task DAG；不把文档依赖环判成业务调度环。

## 本次关键裁决

- Workflow 用 SDK 确定性调度原语；Update、Signal、Query职责和响应语义分开。
- 业务Run/操作身份跨 Continue-As-New 不变，外部写结果未知先核实。
- 未确认提案不落项目工件；source_version 与普通进度 state_sequence 分开；批准有服务端期限和明确范围。
- 原消息追加保留，摘要是派生物；权限和完成条件来自结构化权威记录。
- 远程验证绑定批准的manifest、commit SHA和pipeline，不验证“最新成功”；缺必需证据不完成。
- 执行 temporal/local、决策 model/rules、交付 gitlab/local 三轴独立，缺配置不静默切换。
- 首版数据库能力仅生成代码与隔离测试；客户数据库真实迁移、生产发布不在普通创建批准范围。

Agent 的公开输入、输出、公共能力与依赖见 [使用契约](../docs/AGENT-GUIDE.md)；外部场景调用器见 [validation](../validation/README.md)，它不参与产品内部完成判定。

全部来源、缺陷与理由见 [24条审校裁决](../docs/analysis/REVIEW-DECISIONS.md) 和 [全章覆盖](../docs/analysis/SOURCE-MAP.md)。原文逐字证据见 [摘录](../docs/SOURCE-EXTRACTS.md)，设计与保留实现的差异见 [实现状态](../docs/analysis/IMPLEMENTATION-STATUS.md)。

## 维护与验证

[文档维护约束](DOCUMENT-GOVERNANCE.md) 规定变更入口、冻结修订、依赖同步与证据分类；[技术基线](TECHNICAL-BASELINE.md) 固定当前文档使用的SDK版本与接口依据。

```sh
python3 design/_model/check.py
python3 design/_model/check.py --self-test
```

无需安装Python第三方依赖。结果见 [检查报告](../docs/DESIGN-CHECK-REPORT.md)，按五步说明并带完整路径代码块的全文见 [完整交付](../docs/DESIGN-REFACTOR-OUTPUT.md)。通过只证明文档结构、来源摘要和模型一致，不能证明产品状态、权限、重试、外部副作用或浏览器交互已实现。
