# Temporal 项目创建 Agent

通过多轮对话澄清需求、预览并确认方案，生成 Go 项目及数据库/认证/日志能力，真实执行测试、构建和工件检查，再交付本地包或准确 GitLab 仓库与 CI 首跑。

当前规范是 [五层设计](design/README.md) 与 [业务主线](design/BUSINESS-FLOW.md)。Agent 的公开输入、输出、强依赖和使用流程集中在 [AGENT-GUIDE](docs/AGENT-GUIDE.md)；运行配置见 [backend README](backend/README.md)。45份功能是规范覆盖，实际完成和未验证范围见 [实现状态](docs/analysis/IMPLEMENTATION-STATUS.md)。

本阶段已按文档推进 Vue 等价迁移和十一类只读审查。后续 v2 实施从 [目标公共契约](design/contracts/README.md)、[模块映射](design/contracts/MODULE-MAP.md)、[结构差异矩阵](docs/analysis/CONTRACT-GAP-MATRIX.md) 和 [分阶段路线](design/IMPLEMENTATION-PLAN.md) 开始。目标Schema v2尚未在服务中提供；现有 `/api` 与使用手册为legacy。已有路径测试不证明核心对象结构已符合规范。

三项模式独立选择：执行 temporal/local、决策 model/rules、交付 gitlab/local。默认 temporal/rules/local；缺配置不静默转换或虚构成功。需要本地辅助执行时明确选择：

```sh
cd backend
AGENT_EXECUTION_MODE=local AGENT_DECISION_MODE=rules AGENT_DELIVERY_MODE=local go run ./cmd/server
```

启动完整本地对话工作区，在项目根运行：

```sh
npm --prefix frontend ci
npm run dev
```

开发脚本明确默认 local/rules/local，编译并启动后端及 Vite；后端本身的默认执行模式仍为 temporal。浏览器打开 http://127.0.0.1:5173，后端默认监听127.0.0.1:8080。相同数据根只允许一个写者，不同时启动两个服务共用 `.data`。

持久 Temporal 执行要求提供可访问的 Temporal 服务：

```sh
cd backend
AGENT_EXECUTION_MODE=temporal TEMPORAL_ADDRESS=127.0.0.1:7233 go run ./cmd/server
```

| 入口 | 独立用途 |
| --- | --- |
| [backend](backend/README.md) | Agent HTTP/Workflow/Activity 和产品内部真实完成验证 |
| [design/_model](design/_model/SCHEMA.md) | 当前设计结构、来源和冻结规则的离线检查 |
| [validation](validation/README.md) | 通过公开接口、事件和ZIP调用独立场景，记录外部验收证据 |
| [docs/design](docs/design/README.md) | Vue 工作区、审查页视觉规范和概念图 |

Agent 不读取其他项目的文档或运行模块，不导入外部场景调用器。参考副本与实验认知服务已按 [ADR-045](design/10-decisions/ADR-045-agent-model-validation-separation.md) 移除，当前设计模型保留；普通项目批准不包含客户数据库真实迁移或生产发布。

```sh
python3 design/_model/check.py --self-test --json
```

该检查只证明文档结构与冻结摘要一致，不能代替产品验证或外部场景。原始 [PRODUCT-GUIDE](docs/PRODUCT-GUIDE.md) 和冻结不变量保持原字节。完整设计导出见 [DESIGN-REFACTOR-OUTPUT](docs/DESIGN-REFACTOR-OUTPUT.md)，维护约定见 [AGENTS.md](AGENTS.md)。

此前公开场景曾分别取得local完整9/9、真实Temporal核心9/9、模型传输重试定向2/2及浏览器主线10/10、定向5/5证据；第三方模型和GitLab为明确mock，不代表live服务或Runner。具体模式、报告、生成项目运行和未覆盖项见[实现状态](docs/analysis/IMPLEMENTATION-STATUS.md)与[浏览器验收](docs/evidence/BROWSER-QA.md)。容器构建及生产发布未验收。


当前前端为 Vue 3 + TypeScript + Vite，原工作区的公开 API、交互和外部场景入口保留；后端没有因迁移引入参考项目依赖。审查页面打开 `http://127.0.0.1:5173/?review`，11 种视图独立链接，支持节点/关系来源、筛选、主题、缩放/平移、完整 SVG/PNG 导出。默认契约图显示八个核心对象及七条字段绑定链，全部对象开关显示完整 108 个 Schema 对象及 397 条结构/语义关系；文档关系图保留 137 个实体与 529 条关系，以领域筛选阅读。

审查数据是当前规范的离线发布物，来源路径、正文和摘要包含在包内。原生 Vue SVG 组件的本地副本来源见 [PROVENANCE](frontend/src/review/diagrams/PROVENANCE.json)，构建和运行不需要访问 process-diagram。更新规范后先按 [审查发布说明](design/review/README.md) 重新生成和检查，再重新构建前端；浏览器只接受当前构建固定的 bundle/Schema 摘要。

```sh
npm run review:data
npm run review:check
npm run check
npm run build
```

前两个命令要求当前 Python 环境装有发布说明指定的 `jsonschema[format]`；构建本身不运行文档 checker，也不启动 Agent。静态审查页面可直接部署 `frontend/dist`，需要工作区时提供 `/api` 同源代理。详见 [本阶段验收记录](docs/evidence/VUE-REVIEW-QA.md)。v2 S1–S5 与真实第三方验收继续按实施路线单独推进。
