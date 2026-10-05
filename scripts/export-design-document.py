#!/usr/bin/env python3
"""Reproducibly assemble public design documents; exclude products/private reports."""
from __future__ import annotations
import argparse
import hashlib
import json
from pathlib import Path
import re
import sys

ROOT = Path(__file__).resolve().parents[1]
OUTPUT = ROOT / 'docs/DESIGN-REFACTOR-OUTPUT.md'
FINAL_QA = ROOT / 'docs/evidence/VUE-REVIEW-QA.md'
SUFFIXES = {'.md', '.yaml', '.yml', '.json'}
STEPS = [
    '第1步：当前项目冻结来源与不变量',
    '第2步：九个领域与功能公开契约',
    '第3步：当前有效ADR与裁决索引',
    '第4步：结构模型、公共契约与只读审查发布规范',
    '第5步：实施路线、维护约束、说明及分阶段证据',
]
EXPLICIT = [
    'README.md', 'AGENTS.md', 'backend/README.md', 'backend/AGENTS.md',
    'validation/README.md', 'validation/AGENTS.md', 'frontend/AGENTS.md',
    'frontend/src/review/diagrams/PROVENANCE.json',
    'frontend/src/review/publication.generated.json',
]


def digest(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def public_files():
    files, referenced = {}, []
    for dirname in ['design', 'docs']:
        for path in sorted((ROOT / dirname).rglob('*')):
            if not path.is_file() or path == OUTPUT:
                continue
            relative = path.relative_to(ROOT)
            if any(p in {'node_modules', 'dist', '.data', '__pycache__', 'reports'} for p in relative.parts):
                continue
            if path.name in {'launcher.json', 'controller.json'}:
                continue
            if path.suffix not in SUFFIXES and relative.as_posix() != 'design/_model/check.py':
                continue
            if not path.resolve().is_relative_to(ROOT):
                raise ValueError('public document resolves outside project: ' + str(relative))
            if str(relative).startswith('docs/evidence/vue-review/') and path.suffix == '.json' and not (path.name == 'summary.json' or path.name.endswith('-summary.json')):
                referenced.append(path)
                continue
            files[path] = None
    for relative in EXPLICIT:
        path = ROOT / relative
        if path.is_file():
            if not path.resolve().is_relative_to(ROOT):
                raise ValueError('explicit document resolves outside project: ' + relative)
            files[path] = None
    return sorted(files, key=lambda p: p.relative_to(ROOT).as_posix()), referenced


def group(path):
    relative = path.relative_to(ROOT).as_posix()
    if relative in {'docs/PRODUCT-GUIDE.md', 'design/00-invariants/INVARIANTS.md'}:
        return 0
    if re.match(r'^design/0[1-9]-[^/]+/', relative):
        return 1
    if relative.startswith('design/10-decisions/'):
        return 2
    if relative.startswith(('design/_model/', 'design/contracts/', 'design/review/')):
        return 3
    return 4


def language(path):
    return {'.md': 'markdown', '.yaml': 'yaml', '.yml': 'yaml', '.json': 'json', '.py': 'python'}.get(path.suffix, 'text')


def block(path):
    text = path.read_text(encoding='utf-8')
    longest = max([len(m.group()) for m in re.finditer(r'`+', text)] or [0])
    fence = '`' * max(4, longest + 1)
    return f'### {path}\n\n来源原字节 SHA-256：`{digest(path)}`\n\n{fence}{language(path)}\n{text.rstrip(chr(10))}\n{fence}\n'


def render():
    if not FINAL_QA.is_file():
        raise ValueError('final Vue/review QA has not been archived; wait for docs/evidence/VUE-REVIEW-QA.md')
    counts = json.loads((ROOT / 'docs/analysis/MODEL-COUNTS.json').read_text())
    frozen = (ROOT / 'design/_model/constraints.yaml').read_text()
    for filename, key in [('docs/PRODUCT-GUIDE.md', 'source_sha256'), ('design/00-invariants/INVARIANTS.md', 'invariants_sha256')]:
        expected = re.search(r'\b' + key + r':\s*[\"\']?([a-f0-9]{64})', frozen)
        if expected is None or digest(ROOT / filename) != expected.group(1):
            raise ValueError('frozen source mismatch: ' + filename)
    wire = json.loads((ROOT / 'design/contracts/schemas/agent.schema.json').read_text())['$defs']['RunSnapshot']['properties']
    bundle = json.loads((ROOT / 'frontend/public/review/bundle.json').read_text())
    date = bundle['generated_at'].split('T', 1)[0]
    files, referenced = public_files()
    c = counts['counts']
    lines = [
        '# 当前项目五步完整公开文档交付', '',
        f"日期：{date}。本导出同步ADR-047及当前文档/只读审查基线：{c['domain']}个领域、{c['feature']}份功能、{c['adr']}份ADR、{c['invariant']}条不变量，共{counts['entities']}个实体、{counts['relations']}条关系。正式规则以原文件为准；本文件是可复现的公开文本汇编。", '',
        f"本阶段Vue等价工作区、十一类静态只读审查及同源SVG/PNG导出已实现。工作区继续使用legacy公开API；目标wire schema_version={wire['schema_version']['const']}、contract_version={wire['contract_version']['const']}的S1–S5运行契约实施/验收仍为planned。前端迁移、图可见或审查发布检查不能代替v2运行对象/权限符合性。浏览器与导出的实际结果以[本阶段最终QA](<{FINAL_QA}>)和对应报告为准。", '',
        '旧20/20场景、旧15/15浏览器与S0 136/519/46保持原时点标签；“32份运行源文件未改”仅是S0历史事实，不用于当前Vue修改。文档认知模型、发布Schema与束摘要、产品内部检查、外部场景、浏览器及live第三方分别登记。冻结原设计与不变量字节未变。', '',
        f'本次全文包含{len(files)}份公开文本；另有{len(referenced)}份嵌套QA JSON仅保留引用及原字节摘要。', '',
        '不收录产品或场景源码、私有validation reports/launcher/controller资料、node_modules/dist、二进制图像/ZIP；约1.9MB的审查bundle不重复全文收录，其公开Schema、规范、构建摘要清单与QA仍包含。第三方组件PROVENANCE仅收录公开来源记录。', '',
        '复现：在项目根目录运行 `python3 scripts/export-design-document.py`；`--check`核对当前汇编与公开来源完全一致，`--list`列出输入。无需第三方Python依赖。', '',
    ]
    for index, title in enumerate(STEPS):
        lines.append('## ' + title + '\n')
        for path in files:
            if group(path) == index:
                lines.append(block(path))
    if referenced:
        lines.append('## 大体量或嵌套QA记录引用\n')
        lines.append('下列公开记录不在本文重复展开；可通过本地链接查看，引用摘要按原文件字节核对。\n')
        for path in referenced:
            label = path.relative_to(ROOT).as_posix()
            lines.append(f'- [{label}](<{path}>) · SHA-256 `{digest(path)}`')
    return '\n'.join(lines).rstrip() + '\n', files, referenced


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--check', action='store_true', help='compare without rewriting')
    parser.add_argument('--list', action='store_true', help='list public inputs without generating')
    args = parser.parse_args()
    try:
        if args.list:
            files, referenced = public_files()
            print(json.dumps({'included': [str(p.relative_to(ROOT)) for p in files], 'reference_only': [str(p.relative_to(ROOT)) for p in referenced]}, ensure_ascii=False, indent=2))
            return 0
        text, files, referenced = render()
        if args.check:
            if not OUTPUT.is_file() or OUTPUT.read_bytes() != text.encode('utf-8'):
                raise ValueError('export differs from current public sources; regenerate')
        else:
            temporary = OUTPUT.with_suffix('.md.tmp')
            temporary.write_text(text, encoding='utf-8')
            temporary.replace(OUTPUT)
        print(json.dumps({'passed': True, 'output': str(OUTPUT.relative_to(ROOT)), 'included_files': len(files), 'reference_only': len(referenced), 'bytes': len(text.encode('utf-8')), 'sha256': hashlib.sha256(text.encode('utf-8')).hexdigest(), 'check_only': args.check}, ensure_ascii=False, indent=2))
        return 0
    except (ValueError, OSError, KeyError, UnicodeError) as exc:
        print(json.dumps({'passed': False, 'error': str(exc)}, ensure_ascii=False), file=sys.stderr)
        return 1


if __name__ == '__main__':
    raise SystemExit(main())
