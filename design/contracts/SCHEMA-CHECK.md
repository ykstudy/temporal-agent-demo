# 目标Schema文档自检

这是S0的文档工具说明，不是产品内完成验证或新API实现。完整案例清单在 [examples/INDEX.json](examples/INDEX.json)，本轮输出见 [文档检查证据](../../docs/evidence/contract-document-check.json)。字段真值来自目标Schema；样例只验证JSON形状，不证明hash内容、权限、引用存在或任何运行成功。

## 独立环境和复现

本轮使用临时隔离Python环境、`jsonschema[format]==4.25.1`，启用Draft2020-12与FormatChecker并确认date-time检查器存在。不改变Agent、前端或现有标准库调用器依赖。后续S5建立独立契约验证进程时沿用显式依赖边界和公开Schema发布物。

```sh
python3 -m venv /tmp/agent-docs-schema-env
/tmp/agent-docs-schema-env/bin/python -m pip install 'jsonschema[format]==4.25.1'
/tmp/agent-docs-schema-env/bin/python - <<'PY'
from pathlib import Path
import json
from jsonschema import Draft202012Validator, FormatChecker
from referencing import Registry, Resource

assert 'date-time' in FormatChecker.checkers

base = Path('design/contracts')
schemas = {p.name: json.loads(p.read_text()) for p in (base / 'schemas').glob('*.json')}
registry = Registry().with_resources((s['$id'], Resource.from_contents(s)) for s in schemas.values())
def refs(node, schema):
    if isinstance(node, dict):
        if '$ref' in node:
            registry.resolver(schema['$id']).lookup(node['$ref'])
        for value in node.values():
            refs(value, schema)
    elif isinstance(node, list):
        for value in node:
            refs(value, schema)
for schema in schemas.values():
    Draft202012Validator.check_schema(schema)
    refs(schema, schema)
for case in json.loads((base / 'examples/INDEX.json').read_text())['cases']:
    schema = schemas[case['schema']]
    if case['definition']:
        schema = dict(schema, **{'$ref': '#/$defs/' + case['definition']})
    validator = Draft202012Validator(schema, registry=registry, format_checker=FormatChecker())
    errors = list(validator.iter_errors(json.loads((base / 'examples' / case['file']).read_text())))
    assert (not errors) == case['valid'], (case['id'], [e.message for e in errors])
    print('PASS', case['id'])
run = json.loads((base / 'examples/run-snapshot-intake.valid.json').read_text())
standalone = json.loads((base / 'examples/skill-definition.valid.json').read_text())
for skill in [standalone] + run['definition_snapshot']['skills']:
    Draft202012Validator.check_schema(skill['parameters_schema'])
    Draft202012Validator(skill['parameters_schema'], format_checker=FormatChecker()).validate(skill['default_parameters'])
PY
python3 design/_model/check.py --self-test --json
```

Schema资源按本地 `$id` 注册，不配置网络retrieval；未登记 `$ref` 必须报错。完整发布还核对所有静态引用、嵌入Schema定义及核心对象负例，结果和范围在报告中单独列明。上面命令负责meta-schema和显式样例，不自行检查业务跨对象语义。

## 判定边界

- 正例必须完整，负例分别破坏required、类型、枚举、额外字段、日期、来源和条件字段；不能让一个本来已失败的样本承担另一项负例证明。
- 示例占位hash只是形状材料，报告不能写成hash真值/批准验收。JCS/字节摘要见 [HASHING](HASHING.md)，其实现与独立向量验收是后续阶段。
- 未知已知组件的坏payload不能绕过到unknown分支；未知Part不属于canonical v2，通过只读兼容层处理。
- S0结果标记docs_ready；实际响应、存储、adapter与renderer仍需S1–S5的contract_ready/path_verified证据。现有调用器未运行这些新的契约用例。
