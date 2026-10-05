# 当前项目工作约定

## 规范和产品边界

本项目是独立的 Temporal 项目创建 Agent。先读 `design/README.md`、`design/BUSINESS-FLOW.md`、有关功能契约和 ADR；`docs/AGENT-GUIDE.md` 说明现有公开输入、输出与依赖。按当前 `design/` 规范实现，不能以旧原文示例、其他项目或历史实验覆盖已裁决契约。

当前用户要求先补文档，再由文档推进实施。2026-10-02用户“开始推进所有项”已授权Vue等价迁移、十一类只读审查视图与同源导出；按ADR-047先发布本阶段规范，再改对应产品与验证代码。目标v2完整对象仍按S1–S5逐阶段验收，前端迁移不能代替其完成。开始实施前读 `design/contracts/README.md`、`design/contracts/MODULE-MAP.md`、`docs/analysis/CONTRACT-GAP-MATRIX.md` 与 `design/IMPLEMENTATION-PLAN.md`；沿既有授权和阶段顺序推进，不用重复询问代替落实必要工作。

公开目标Schema v2与现有legacy API明确区分。先冻结本阶段对象/字段/Schema、hash输入及兼容规则，再实现对应模块；不能以扁平Plan、文本Message或字段存在检查替代完整AgentDefinition/SkillDefinition/DecisionProposal/ActionValidation/ExecutionRequest/MessageEnvelope/SummaryRecord。手写精简DTO或修改测试断言以迁就现有响应，均不能作为结构完成。

`docs/PRODUCT-GUIDE.md` 与 `design/00-invariants/INVARIANTS.md` 已冻结。未经用户明确授权修订规则，不改其字节，不更新 constraints 中的摘要来让校验通过。新 ADR 登记实体、关系和 INDEX；功能依赖只指公开 `#输入` / `#输出`，正反向引用保持一致。

## 三条独立边界

- `backend/` 是产品。必要 Schema、模板、批准、工件及验证能力在本项目内实现，不依赖外部项目目录或参考文档副本。
- `design/_model/` 是设计结构检查工具，不是运行状态、批准来源或服务启动条件；离线通过仅证明文档一致性。
- `validation/` 是独立黑盒场景调用器，只消费公开服务接口与产物；产品不导入它，不读取其内部结果来决定 DONE。产品内部 required_checks 仍须真实执行。

## 执行约束

execution_mode 为 temporal/local，decision_mode 为 model/rules，delivery_mode 为 gitlab/local，三轴独立且不可静默降级。模型只建议；Schema、阶段、权限、版本、预算和完成由确定性控制平面判定。当前范围不含客户数据库真实迁移或生产合并/发布。

source_version 只在业务方案/授权范围变化时增加，普通进度增加 state_sequence。proposal_version 内容不可变；确认绑定身份、版本和快照 hash，issued_at/expires_at 由服务端签发，客户端不能延期。未知外部写进入 BLOCKED 核实，禁止盲重放；终止 Run 重做须新身份。

文件与子进程限制在 Run 工作区；凭据不进入命令参数、remote URL、模型上下文、日志、历史或包。修改时保护并行工作，只改任务文件。代码检查、产品内部验证、独立场景、live 第三方和浏览器证据分别登记，不把 45 份功能文档全部宣称已实现。

审查规范见 `design/review/README.md`、`design/review/MODELS.md`、`design/review/review-bundle.schema.json`；离线生成 `frontend/public/review/bundle.json`。审查运行只消费本地发布物，不访问参考项目、不触发checker、不提交批准。

## 验证入口

文档修改运行 `python3 design/_model/check.py --self-test --json`。后端按 `backend/README.md` 执行相关 Go 检查；黑盒场景按 `validation/README.md` 调用公开服务。各项新结果写入 `docs/analysis/IMPLEMENTATION-STATUS.md`，注明当前代码时点、配置、未覆盖项及证据路径。

认知模型通过仅是docs_ready。另验证目标Schema自身、离线引用与正负例；后续实现需要公开响应完整符合Schema、跨对象版本/hash/权限语义以及已有路径回归三类证据。阶段通过后才更新对应差异项，禁止把20/20或15/15路径计数写成全部结构符合。派生Go/TS/外部验收Schema与发布源摘要核对，Agent运行不读取设计认知模型。旧记录缺失的提示/Schema/引用不得由adapter捏造为已冻结事实。
