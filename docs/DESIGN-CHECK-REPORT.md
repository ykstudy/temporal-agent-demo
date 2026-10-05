# 设计文档校验报告

日期：2026-10-02。校验对象为本项目的 [design 文档体系](../design/README.md) 与[结构模型](../design/_model/SCHEMA.md)。

## 校验结论

当前ADR-047文档/静态审查基线校验通过。在Vue迁移与只读十一模型规范、目标公共契约、差异矩阵、阶段路线和INDEX/entities/relations同步后，真实项目执行 `python3 design/_model/check.py --self-test --json`，退出码为0，`passed: true`，基线 `errors: []`。七类错误注入均检出预期错误，四项防绕过回归也全部通过；完整[当前JSON结果](evidence/vue-review-design-check.json)保留。历史S0的[原JSON结果](evidence/contract-design-check.json)为136实体/519关系/46 ADR，维持原时点而非重写。

历史S0另用独立 `jsonschema[format]==4.25.1` 检查三份Draft2020-12 Schema、381处静态引用、37个结构正负例（12正例、25反例）及四份技能默认参数声明，全部符合预期。检查器确认date-time断言启用，全引用由本地注册表解析，不获取远程资源。[结果](evidence/contract-document-check.json) 和 [复现步骤](../design/contracts/SCHEMA-CHECK.md) 单独登记。结论为docs_ready，未验证v2服务、JCS实现、hash真值、跨对象权限或任何新的运行/live结果。

验收对象是当前项目根模型；本次没有用参考副本代替当前模型，也没有通过刷新冻结摘要使检查通过。

## 实体与关系计数

| 实体类型 | 实际数量 |
|---|---:|
| 领域 | 9 |
| 功能 | 45 |
| ADR | 47 |
| 不变量 | 36 |
| 合计 | 137 |

| 关系类型 | 实际数量 |
|---|---:|
| belongs_to | 45 |
| consumes | 206 |
| submits_to | 10 |
| decides | 154 |
| constrains | 114 |
| depends_on | 0 |
| 合计 | 529 |

[约束目录](../design/_model/constraints.yaml) 登记 15 个检查项。目录用于说明检查分类，不作为关闭内置校验的开关。

## 检查分类

| 分类 | 实际核对内容 |
|---|---|
| 来源与冻结 | 原设计和有效不变量的完整字节 SHA-256；固定规范路径，无自动更新摘要 |
| 实体与覆盖 | 唯一身份、合法类型和字段白名单、文件存在、路径不越界；九域和唯一功能归属；功能、ADR 与全部 INV 标题登记 |
| 文档章节 | 45个功能固定八节，47个ADR固定八节；识别合法缩进及Setext标题，排除代码示例 |
| 关系与契约 | 端点存在、方向和类型合法；功能只能通过 consumes/submits_to 引用公开输入或输出，不能使用 depends_on 绕过 |
| 双向引用 | 正向依赖及被依赖引用与同一模型关系精确一致，无漏项或额外未登记引用 |
| ADR 关联 | 每个 ADR 至少决定一个领域与一个功能，关联章节存在对应相对链接 |
| 链接与结构边界 | 本地相对目标及中文锚点存在，不能越出项目；模型字段不能复制正文或运行状态 |
| 错误注入 | 七类规定负例及四项防绕过回归均在临时副本执行，不修改原文或冻结文件 |

## 冻结摘要

| 文件 | SHA-256 |
|---|---|
| [原设计](PRODUCT-GUIDE.md) | `51c889d53a694e1047d9a9a168ef2fe753faa50a68519c0ebbcaa59a6895a45b` |
| [有效不变量](../design/00-invariants/INVARIANTS.md) | `dfe0f6eb2a707e640099ffc405d63266e0f457991184e4f5b3e5b68d452a2952` |

原设计与冻结不变量没有因校验器增强、正文联调或错误注入被重写，摘要未自动刷新。

## 七类负例

| 注入 | 必须检出的错误 | 结果 |
|---|---|---|
| 改动原设计和冻结不变量字节 | 两个文件各自产生 baseline 错误 | 已拒绝 |
| 实体路径指向不存在文件 | missing_path | 已拒绝 |
| 追加重复实体 ID | duplicate_id | 已拒绝 |
| 关系引用不存在实体 | dangling_relation | 已拒绝 |
| 功能依赖改指算法内部章节 | internal_dependency，并检出缺失公开依赖 | 已拒绝 |
| 移除一个 ADR 的全部 decides 关联 | adr_association | 已拒绝 |
| 实体混入循环 Workflow 数据 | structure_only | 已拒绝 |

当前认知模型不包含运行 Task DAG。第七项验证的是结构内容污染被拒绝，不能作为运行 DAG 循环检测的证明。

## 防绕过回归

| 注入 | 必须检出的错误 | 结果 |
|---|---|---|
| 删除公开正向/逆向引用，再把功能契约关系替换为 depends_on | internal_dependency | 已拒绝 |
| 删除一个不变量实体及其所有关系，冻结正文保持原样 | invariant_coverage | 已拒绝 |
| 把真实功能文件登记为 invariant | unregistered | 已拒绝 |
| 把真实 ADR 文件登记为 invariant | unregistered | 已拒绝 |

## 复现

在项目根目录运行[校验器](../design/_model/check.py)：

```sh
python3 design/_model/check.py --json
python3 design/_model/check.py --self-test --json
```

正常退出码为 0；基线、七类负例或四项回归任一失败均为 1。错误注入使用自动清理的临时项目副本，当前项目中的原文、不变量、模型和正文保持原样。

本次使用系统 Python 3.9.6，未安装 PyYAML，实际走标准库严格 YAML 后备解析。上述复现命令无需安装额外依赖。

## 证明范围

校验通过只证明文档结构、来源摘要与认知模型一致；不证明产品状态机、权限、预算、重试、外部副作用或运行验收已实现。

历史S0未修改运行代码，32份运行源文件摘要当时与阶段前一致；该事实不用于后续Vue修改。当前已完成Vue工作区等价迁移及十一类静态审查，原文/不变量冻结字节仍保持不变。此前产品/内部验证/外部场景/浏览器事实仍见 [实现状态](analysis/IMPLEMENTATION-STATUS.md)，不能从本次报告推导新目标已经运行通过。契约Schema由功能与ADR说明定位，不作为认知模型中的运行payload。


## ADR-047静态审查发布检查

[审查发布规范](../design/review/README.md)定义ReviewBundle schema_version=1，与Agent wire v2分开。生成器从127+公开来源派生全部十一模型（当前128来源），核对源字节摘要、冻结基线、元Schema/结构/format、全部元素binding、关系端点及专属语义。状态表44事件、知识137实体/529关系、21差异/18计划用例与五维离散矩阵完整；契约108类/397关系包括379个$ref与18个真实字段语义绑定。

[当前数据检查](evidence/vue-review-data-check.json)记录12项数据错误注入与2项发布摘要错误均拒绝，发布Schema副本与源同字节，bundle与最新来源投影一致。此检查不证明Vue浏览器、SVG/PNG图片质量、Run批准或v2运行符合。新迁移浏览器15/15由实际路径回归得出，独立于本报告；最终审查187/187和来源边界15/15及1探针的独立结果见[本阶段验收](evidence/VUE-REVIEW-QA.md)，不由本检查推导。S1–S5目标对象和live第三方仍为planned/not_run。
