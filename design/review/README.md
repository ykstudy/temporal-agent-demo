# 只读可视化审查发布

日期：2026-10-02。规范由 [ADR-047](../10-decisions/ADR-047-vue-readonly-review.md) 冻结本阶段范围：Vue 等价迁移、十一类审查视图、来源详情、同源 SVG/PNG 导出。它是文档审查入口，不替代 Agent 运行工作区。

阅读 [MODELS](MODELS.md) 明确每图语义，[ReviewBundle Schema](review-bundle.schema.json) 明确发布结构；有效业务要求仍来自冻结不变量、功能公开契约及 ADR。图数据可在本项目内独立生成，构建/运行不依赖参考项目路径。

## 发布与复现

```sh
python3 -m venv /tmp/agent-review-schema-env
/tmp/agent-review-schema-env/bin/python -m pip install 'jsonschema[format]==4.25.1'
/tmp/agent-review-schema-env/bin/python scripts/build-review-data.py --self-test
/tmp/agent-review-schema-env/bin/python scripts/build-review-data.py --check
python3 design/_model/check.py --self-test --json
```

生成器将本项目公开规范派生到 `frontend/public/review/bundle.json`，Schema原字节副本发布到 `frontend/public/review/schema.json`；发布源与副本摘要必须一致。验证后生成`frontend/src/review/publication.generated.json`，其中bundle_sha256/schema_sha256按发布文件原字节计算，并编译进前端；加载器在解析/渲染前与此构建时可信摘要核对，不能仅靠载荷自带的来源摘要接受被改动的图投影。每次发布数据变更须重新构建前端，使静态束和编译摘要同版本；开发模式按新生成清单重新编译模块。该清单和检查报告不作为bundle来源，避免摘要循环。报告写入 `docs/evidence/vue-review-data-check.json`。`--check` 验证已发布bundle与最新来源投影、Schema副本和构建摘要清单全部一致，不重写文件；自检在临时对象上注入错误，不修改冻结来源。生成时 JSON Schema Draft 2020-12 完整验证使用隔离环境的 `jsonschema[format]`，显式断言时间格式；无验证器时明确报错，不能以手写字段检查冒称元 Schema 验证。生成器不导入 backend/validation 产品或 oracle。

bundle 的 `schema_version=1` 只标识审查发布格式；Agent wire 仍为 2、公共契约版本仍为 1.0.0，不能混用。冻结原始设计和不变量摘要与 constraints 核对，全部来源相对路径限制在仓库内；公开来源包不包含数据目录、运行凭据或私有原始报告。来源正文、路径及摘要使独立静态部署仍能查看出处。

## 证明口径

每个模型、元素都有 `scope=design/current/evidence`，必要时元素覆盖模型范围。`docs_ready` 表示规范可以审查；`planned` 表示未执行目标验收；`historical_path_verified` / `historical_docs_verified` 仅指已保存历史报告。当前证据须独立登记具体代码时点，不因迁移、重生成 bundle 或选择图节点自动升级。

图中的版本演进是明确设计场景，不是真实 Run 历史；契约图表示 JSON 组成和引用，不表示数据库 PK/FK；覆盖图使用离散状态，未验证不是数字零或失败；Skill 参数与依赖来自公开 Schema/有效规范，示例依赖包仅说明完整形状，样例 hash 永不作为批准或真实冻结证据。

前端仅获取静态 bundle，不发布设计 apply/回退或运行命令。导出以同一份图数据及实际 SVG 为源，包含模型标识、baseline、生成时间、范围与状态；图片的时间戳不证明实现已通过。
