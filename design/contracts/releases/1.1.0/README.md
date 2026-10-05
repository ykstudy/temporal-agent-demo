# 契约 1.1.0 发布说明

2026-10-02，按 [ADR-048](../../../10-decisions/ADR-048-definition-runtime-release.md) 发布。wire 仍为 schema_version=2；本目录三份 Schema 是 contract_version=1.1.0 的唯一发布源。原 [1.0.0 Schema](../../schemas/agent.schema.json) 字节保留；不能按同一发布身份覆盖旧内容。1.1.0 与旧包严格分别校验，未知版本拒绝，不能混用跨包 `$ref`。S1 实施不表示完整 v2 Run API 已提供。

## 模板内容与清单引用

原1.0.0将TemplateFile.content_ref和模板清单都称作template引用，清单hash又包含文件引用，造成引用主体不明确。1.1.0为TemplateFile.content_ref使用 `object_type=template_content`，其不可变主体是该文件UTF-8原字节，不作JSON重排。`object_id=template_content_`加完整64位原字节sha256，object_version=1、source_version=1、hash=原字节sha256。后两项是发布内容版本，不冒充Run业务来源；只允许通过绑定快照授权读取。

TemplateDependency.manifest_hash仍对完整TemplateDependency按JCS计算，仅排除自身manifest_hash。`object_type=template` 仍引用清单。先存内容，再建文件引用/清单，再建Skill/Definition/快照，消除循环。SchemaDependency的内容引用仍为schema，hash为发布文件原字节sha256，解析仅允许快照内已登记依赖。

S1内容读取接口中schema的object_id固定为SchemaDependency.schema_uri（agent.schema.json、common.schema.json或message.schema.json）；hash为该条sha256。不得以文件系统路径或任意URL解析内容。

## JSON 编解码资源边界与整数语义

以下限制在Schema校验和逻辑对象hash之前执行，产品与独立oracle使用相同边界；它们是本发布的输入资源限制，不由模型、客户端或Schema的default注释调整。

| 项目 | 最大允许值与计数方式 |
| --- | --- |
| 单份JSON原始输入 | 16 MiB，即16,777,216字节；按UTF-8原字节计数，空输入拒绝 |
| 值嵌套深度 | 128；根值深度为0，每进入对象属性值或数组元素增加1，空容器本身也占一个值 |
| 值节点总数 | 500,000；对象、数组、字符串、数值、boolean和null各计一个，属性名不另计值节点 |
| 单个数值字面量 | 128字节，包含符号、小数点和指数部分 |
| 十进制指数 | 绝对值不超过400；省略指数相当于0；满足该上限仍须能转换为有限binary64值 |

先拒绝重复JSON属性名（包括转义后相同的名称）、无效UTF-8、未配对Unicode代理项、NaN/Infinity、数值溢出、超限输入及一个JSON值之后的额外内容。不得先由通用decoder保留最后一个重复键、修复非法字符或转换成浮点数后再校验。

无小数点和指数的整数字面量限制为−(2^53−1)至2^53−1；身份、版本和预算字段还按各自Schema限制为非负或正整数。带小数点或指数的数值保留原十进制精度进行Schema判断：`1`、`1.0`、`1e0`都满足integer，`1.00000000000000000001`不满足integer，`9007199254740991.1`与`9.007199254740992e15`不能通过安全整数版本字段。校验成功后，精确的安全整数可规范成Go整数表示；不得用先舍入再校验的方式接纳小数或越界版本。

仅在JCS计算阶段按RFC8785进行binary64数字序列化；它不改变先前的Schema整数判断。此编解码边界不等同于接受全部RFC8785数值拼写：例如有限浮点输入`1e20`的JCS输出为`100000000000000000000`，后者作为新的纯整数字面量输入仍受到上述安全整数限制。S1定义与预算使用Schema限定的安全整数，不通过浮点拼写扩大范围；跨语言测试分别核对原输入的接纳结论、精确整数语义和JCS结果。

1.1.0继续采用[公共hash算法](../../HASHING.md)和[固定对象排除表](../../AGENT-CONTRACTS.md#固定哈希输入与无循环关系)，不改变16类对象的顶层排除清单。只有定义的自身及生命周期排除字段被移除，嵌套hash、null和数组顺序保留；集合排序和重复身份拒绝由归属对象先完成。生成hash本身不证明Schema合法、引用存在、批准有效或可以执行。模板、Schema、提示及JSONL来源范围仍按各自原字节计算，不对原字节重新序列化。

## 冻结预算

原五个计数上限不变，BudgetPolicy新增六个必需正整数。默认值由具体Definition声明，不由Schema自动注入：

| 字段 | 默认值 | 单位与累计边界 |
| --- | --- | --- |
| max_tool_seconds | 720 | 单条固定工具命令秒数；PostCommand.timeout_seconds不得大于此值 |
| max_execution_seconds | 7200 | 从首次批准应用起的业务总秒数，包含暂停/阻塞；恢复不重置，耗尽停止新写；原unknown核实另受独立核实期限约束 |
| max_ci_wait_seconds | 1800 | 同一首跑操作第一次等待起的累计秒数，跨Activity/服务恢复不重置 |
| max_ci_poll_attempts | 128 | 同一首跑操作实际读取尝试总数；失败也计入，同尝试回执不双扣 |
| max_reconciliation_seconds | 1800 | 同一sent/unknown操作开始核实起的累计秒数；期限耗尽保留unknown，不能解释为未执行 |
| max_reconciliation_attempts | 64 | 同一外部操作实际核实读取尝试总数，跨resume/执行段不重置 |

调用前持久化预留/尝试身份与起点，结果按同身份结算。首次应用/每步/恢复复核，local与Temporal都执行；Workflow历史长度不作为业务预算。S1保存完整政策，S2及运行收尾实现执行账本并单独验收，不因字段存在宣布预算行为完成。原1.0.0 Run不自动增加新授权/额度；缺完整来源的旧记录保持legacy恢复边界。

## S1 的公开观察和版本选择

本阶段增加下列受限legacy外围接口，正文直接输出完整1.1.0对象，不称为v2 RunSnapshot。公开错误沿用error/code；来源不完整返回409 legacy_contract_incomplete及legacy_read_url。只读接口不触发生成、模型或外部写入。

| 接口 | 输入与输出 |
| --- | --- |
| GET /api/definitions | 返回本服务已发布AgentDefinition数组；每项完整校验并具有真实definition_hash |
| GET /api/runs/{id}/definition | 返回该Run持久绑定的完整DefinitionSnapshot；缺失不从当前注册表补造 |
| GET /api/runs/{id}/definition/content | 必需query object_type、object_id、hash；类型只允许template_content/schema；核对该Run绑定依赖中的完整身份，返回UTF-8原字节及X-Content-SHA256；缺项400、越界/不存在404、损坏409 |
| POST /api/runs | 在legacy CreateOptions中可全量指定definition_id、definition_version、definition_hash；三者必须同时给出或全部省略。省略使用服务已发布默认定义，服务把实际选择持久化；重发同Run不得换定义 |

定义发布仅由可信服务配置/离线资产发布，不提供客户端任意prompt/工具注册写接口。产品内定义注册表保留各版本和内容blob，模板渲染消费绑定blob中的固定渲染程序；程序只允许已登记路径、条件与纯text/template函数，不执行命令。旧版完整快照跨重启可读；缺模板/Schema、同版异内容、撤销依赖和hash不符均显式失败。

## 独立验收

校验三份Schema的元Schema/离线引用与所有公开对象；继承1.0.0正负例需显式更新契约身份、文件内容引用和六个预算字段，不能把教学hash样本当内容证据。S1独立调用上述接口复核prompt/Skill/Definition/Snapshot及文件/清单hash，验证旧Run读取原提示/模板、两版本共存、同版冲突、缺依赖、错参数与越权内容读取拒绝。v2完整Run/消息/回执和SSE仍按S2–S4实施。
