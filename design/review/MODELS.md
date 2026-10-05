# 十一类审查模型与验收

本阶段由 [ADR-047](../10-decisions/ADR-047-vue-readonly-review.md) 与 [发布 Schema](review-bundle.schema.json) 定义。所有图来自当前项目的规范、公开对象、登记关系、差异或有时间标记的历史报告；布局可以由生成器维护，业务语义不能靠外部图示例补造。

## 模型目录

| 稳定 ID | 图类型 | 规范来源 | 审查语义与界限 |
| --- | --- | --- | --- |
| state | state | 冻结不变量“状态与事件基线”及门禁 | 十一态完整列出所有合法事件；PAUSED/BLOCKED恢复到保存的非终止态，仅核实且复校后放行；终态重做新Run。未列转移拒绝。 |
| responsibility | swimlane | 冻结“权限与职责矩阵” | 角色泳道标示可做/不可做，交接引用业务主线；模型只建议，控制平面裁决，执行器受控执行，用户具体批准。运维和只读审查不产生Run授权。 |
| sequence | sequence | 业务主线与Agent公开传输契约、异常功能 | 设计场景展示创建、建议、预览、确认、校验、受控执行、核验及事件；旧批准/未知外部写/断流重取单列消息说明。消息顺序表示协议场景，不是实际日志。 |
| contracts | class | 公共三Schema的$defs/properties/$ref | JSON对象与字段完整展示，内部$ref引用递归派生，并按真实ID/hash字段增加标注“字段绑定”的规范语义关系（包括嵌套ApprovalBinding及manifest ObjectRef）；两者不表示运行调度。必需/可空/数组语义可查；不赋予数据库主外键、方法或继承事实。 |
| lineage | lineage | 公共Schema/hash表/批准/证据不变量 | 定义与依赖、候选、批准、校验、执行、manifest与报告的绑定字段；hash内容按归属契约，不用样例hash证明授权。模型话语不成为批准证据。 |
| architecture | architecture | MODULE-MAP、认知模型领域、ADR-045 | 目标模块职责、产品/文档工具/独立validation边界。当前代码路径是定位事实，目录未创建不标实现。关系说明build-time或公开消费，不表示已拆包。 |
| coverage | coverage | 21项差异、C01–C18、历史证据 | 离散矩阵列为文档/目标契约/路径/浏览器/live。历史路径单列且注明时点，不能关闭目标差异；未执行显示planned/not_run。 |
| versions | timeline | source/state版本不变量、不可变提案、批准期限、CAN身份 | 日期为2026-10-02起的设计场景坐标，全部planned；普通进度只增state_sequence，方案范围变化增source_version，旧批准失效，CAN业务身份保持，终态重做新身份。 |
| knowledge | knowledge | entities.yaml与relations.yaml | 展示全部登记的domain/feature/adr/invariant及关系。consumes/submits_to只关联公开输入输出，反馈环不是运行DAG。 |
| skills | dependency | SkillDefinition Schema、三Skill公开定义示例与有效功能 | 三技能配置、默认值、权限/工具、模板/参数Schema依赖；示例是形状不是已发布运行注册表。缺依赖/冲突拒绝，版本内容不可漂移。 |
| context | dataflow | context-building/compression/JSONL/预算功能及SummaryRecord | 原文追加、来源范围冻结、派生摘要历史、上下文预算与稳定引用；批准/权限来自权威对象，不能从摘要生成。失败保留上一摘要与原文。 |

## 数据与元素绑定

每图 `data` 尽量采用同名 SVG 组件专属接口。图 envelope 的 `bindings` 以 data 内元素稳定 ID 为 key，内容为 label、description、details、source_ids、scope/status。选中节点/关系/字段/矩阵单元都可追溯来源；来源 `path` 是仓库内公开文件，相同源文件摘要计算原始字节。字段虚拟ID采用 `ClassName.field`，关系ID按来源/目标/谓词内容摘要生成，不依赖数组下标。

coverage 专属数据为 `rows:[{id,label,priority,description,case_ids}]`、`columns:[{id,label}]`、`cells:[{id,row,column,status,label,detail,case_ids,evidence_ids}]`、`cases:[{id,feature_id,contract,expectation,phase,status}]`、`evidence:[{id,label,recorded_at,status,scope,source_id,description}]`。单元状态是离散类别，不能当连续热力值。历史报告单独列于 evidence，不根据路径 passed 为 G01–G21 目标契约推断通过。

## 离线检查

发布时验证完整投影后将bundle/Schema原字节SHA-256固定到前端构建清单；运行加载器先核对编译摘要再解析，防止来源和Schema仍一致但图data被替换。数据重新发布后必须重新构建前端，不从载荷自身取得可信摘要；清单不进入来源包，运行只读边界保持不变。

检查发布Schema自身、时间格式、闭合字段与版本；基线摘要和所有来源摘要匹配；模型ID唯一且十一类齐全；每个元素有绑定且source_ids存在；关系端点、泳道归属、时序参与者/索引合法；契约字段和$ref来自Schema，语义绑定两端字段经本地$ref解析后存在、predicate和Schema/正文来源一致，知识实体/关系与登记一致；状态恢复受保存态约束，终态无当前Run执行出口；21差异/18用例不能丢失，设计timeline不能标actual/complete；未覆盖live明确not_run；整个文件、节点、边、文本和数值有界。

自检至少注入悬空来源、未知元素端点、错误版本、冻结摘要变化、目标覆盖伪通过和终态原地恢复，均须失败。生成和验证报告只证明审查发布物一致性，不能证明Agent状态、批准或外部副作用实现。
