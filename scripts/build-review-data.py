#!/usr/bin/env python3
"""Publish source-bound, read-only design review data; never execute Agent code."""
from __future__ import annotations
import argparse
import copy
from datetime import datetime, timedelta, timezone
import hashlib
import importlib.util
from importlib.metadata import version
import json
from pathlib import Path
import re
import sys

ROOT = Path(__file__).resolve().parents[1]
OUTPUT = ROOT / 'frontend/public/review/bundle.json'
REPORT = ROOT / 'docs/evidence/vue-review-data-check.json'
SCHEMA = ROOT / 'design/review/review-bundle.schema.json'
PUBLICATION = ROOT / 'frontend/src/review/publication.generated.json'
MODEL_IDS = {'state', 'responsibility', 'sequence', 'contracts', 'lineage', 'architecture', 'coverage', 'versions', 'knowledge', 'skills', 'context'}
INVARIANTS = 'design/00-invariants/INVARIANTS.md'
AGENT_SCHEMA = 'design/contracts/schemas/agent.schema.json'
MESSAGE_SCHEMA = 'design/contracts/schemas/message.schema.json'
COMMON_SCHEMA = 'design/contracts/schemas/common.schema.json'
MODELS = 'design/review/MODELS.md'
MAP = 'design/contracts/MODULE-MAP.md'
GAPS = 'docs/analysis/CONTRACT-GAP-MATRIX.md'
PLAN = 'design/IMPLEMENTATION-PLAN.md'
AGENT_DOC = 'design/contracts/AGENT-CONTRACTS.md'
BUSINESS = 'design/BUSINESS-FLOW.md'
HASHING = 'design/contracts/HASHING.md'
EVIDENCE = ['docs/evidence/contract-document-check.json', 'docs/evidence/contract-design-check.json', 'docs/evidence/validation-local-summary.json', 'docs/evidence/validation-temporal-summary.json', 'docs/evidence/validation-model-retry-summary.json', 'docs/evidence/browser-summary.json', 'docs/evidence/browser-races.json']
# Public ID/hash joins are semantic invariants, not JSON Schema $refs or scheduling.
SEMANTIC_BINDINGS = [
    ('DefinitionSnapshot', 'AgentDefinition', [('definition_id', 'definition_id'), ('definition_version', 'definition_version'), ('definition_hash', 'definition_hash')], '冻结包的登记身份与内嵌完整定义一致。'),
    *[(name, 'DefinitionSnapshot', [('definition_snapshot_hash', 'snapshot_hash')], '同一Run绑定的完整依赖闭包；不能用逻辑definition_hash替代。') for name in ['DecisionProposal', 'Proposal', 'Approval', 'ActionValidation', 'ExecutionRequest', 'ArtifactManifest', 'VerificationReport', 'SummaryRecord']],
    ('RequiredCheck', 'DefinitionSnapshot', [('definition_snapshot_hash', 'snapshot_hash')], '检查绑定来源定义；不是模型自报完成。'),
    ('Approval', 'Proposal', [('proposal_id', 'proposal_id'), ('proposal_version', 'proposal_version'), ('source_version', 'source_version'), ('proposal_snapshot_hash', 'snapshot_hash'), ('plan_hash', 'plan_hash'), ('permission_hash', 'permission_hash')], '批准单向绑定既有不可变方案及具体scope/hash；生命周期status不产生新授权。'),
    ('ActionValidation', 'DecisionProposal', [('decision_id', 'decision_id'), ('run_id', 'run_id'), ('source_version', 'source_version'), ('definition_snapshot_hash', 'definition_snapshot_hash'), ('decision_kind', 'advice.kind')], '裁决绑定来源建议；ActionValidation没有decision_hash字段，不能捏造该绑定。'),
    ('ExecutionRequest', 'ActionValidation', [('validation_id', 'validation_id'), ('validation_hash', 'validation_hash'), ('run_id', 'run_id'), ('source_version', 'source_version'), ('state_sequence_at_validation', 'state_sequence')], '执行请求引用冻结校验主体；校验自身hash排除execution_request，避免循环。'),
    ('ExecutionRequest', 'Approval', [('approval.approval_id', 'approval_id'), ('approval.proposal_id', 'proposal_id'), ('approval.proposal_version', 'proposal_version'), ('approval.source_version', 'source_version'), ('approval.proposal_snapshot_hash', 'proposal_snapshot_hash'), ('approval.definition_snapshot_hash', 'definition_snapshot_hash'), ('approval.permission_hash', 'permission_hash'), ('approval.issued_at', 'issued_at'), ('approval.expires_at', 'expires_at')], '嵌套ApprovalBinding核对已有批准；该Binding没有approval_hash，仍须读取权威批准并复核当前有效性。'),
    ('ArtifactManifest', 'Proposal', [('proposal_id', 'proposal_id'), ('proposal_version', 'proposal_version'), ('source_version', 'source_version'), ('proposal_snapshot_hash', 'snapshot_hash'), ('plan_hash', 'plan_hash')], '实际文件集合绑定批准方案来源；Manifest没有approval字段，不虚构直接批准引用。'),
    ('VerificationReport', 'ArtifactManifest', [('manifest_ref.object_id', 'manifest_id'), ('manifest_ref.object_version', 'manifest_version'), ('manifest_ref.source_version', 'source_version'), ('manifest_ref.hash', 'hash'), ('run_id', 'run_id')], 'manifest_ref.object_type须为manifest，按公开ObjectRef索引核对存在性及同Run来源；不是任意对象引用或最近成功工件。'),
    ('VerificationReport', 'Proposal', [('proposal_snapshot_hash', 'snapshot_hash'), ('source_version', 'source_version'), ('run_id', 'run_id')], '报告检查集合与批准快照同源；outcome=passed不能代替来源/hash核验。'),
    ('RequiredCheck', 'Proposal', [('proposal_snapshot_hash', 'snapshot_hash'), ('source_version', 'source_version'), ('run_id', 'run_id')], '仅proposal_snapshot_hash非null时绑定具体提案；检查状态与固定完整集合另行裁决。'),
]



def load_json(path):
    def unique(pairs):
        result = {}
        for k, v in pairs:
            if k in result:
                raise ValueError('duplicate key: ' + k)
            result[k] = v
        return result
    return json.loads(path.read_text(encoding='utf-8'), object_pairs_hook=unique, parse_constant=lambda v: (_ for _ in ()).throw(ValueError('non-JSON number: ' + v)))


def sha(raw):
    return hashlib.sha256(raw).hexdigest()


def stable(prefix, *parts):
    return prefix + '.' + sha(json.dumps(parts, ensure_ascii=False, separators=(',', ':')).encode())[:20]


def plain(text):
    return re.sub(r'\[([^\]]+)\]\([^)]+\)', r'\1', text).replace('`', '').strip()


def table(text, heading):
    section = text.split('## ' + heading, 1)[1].split('\n## ', 1)[0]
    rows = []
    for line in section.splitlines():
        if line.startswith('|'):
            parts = [p.strip() for p in line.strip().strip('|').split('|')]
            if not all(re.fullmatch(r'[- :]+', p) for p in parts):
                rows.append(parts)
    return rows[1:]


def yaml_parser():
    # Only reuse the strict YAML reader; this does not run a document check.
    spec = importlib.util.spec_from_file_location('review_yaml_reader', ROOT / 'design/_model/check.py')
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module.load_yaml


class Publisher:
    def __init__(self):
        self.sources = {}
        self.models = []
        read_yaml = yaml_parser()
        self.entities = read_yaml(ROOT / 'design/_model/entities.yaml')['entities']
        self.relations = read_yaml(ROOT / 'design/_model/relations.yaml')['relations']
        self.constraints = read_yaml(ROOT / 'design/_model/constraints.yaml')['baseline']
        self.entity_map = {e['id']: e for e in self.entities}
        self.schemas = {p: load_json(ROOT / p) for p in [AGENT_SCHEMA, MESSAGE_SCHEMA, COMMON_SCHEMA]}
        self.by_schema_id = {s['$id']: (p, s) for p, s in self.schemas.items()}
        for path, name in [(AGENT_SCHEMA, 'RunSnapshot'), (MESSAGE_SCHEMA, 'MessageEnvelope')]:
            properties = self.schemas[path]['$defs'][name]['properties']
            if properties['schema_version'].get('const') != 2 or properties['contract_version'].get('const') != '1.0.0':
                raise ValueError('public wire/contract version mismatch')
        if any('/contracts/1.0.0/' not in sid for sid in self.by_schema_id):
            raise ValueError('public Schema release mismatch')
        self.source('design/review/review-bundle.schema.json')
        self.source('design/review/README.md')
        self.source(MODELS)
        self.source('design/10-decisions/ADR-047-vue-readonly-review.md')
        self.source('design/_model/constraints.yaml')
        self.source('docs/PRODUCT-GUIDE.md')

    def source(self, path):
        path = str(path)
        resolved = (ROOT / path).resolve()
        if not resolved.is_relative_to(ROOT) or not resolved.is_file():
            raise ValueError('missing or unsafe source: ' + path)
        if not (path.startswith(('design/', 'docs/')) or path in ['AGENTS.md', 'backend/README.md', 'validation/README.md']):
            raise ValueError('source not public documentation: ' + path)
        if path.startswith(('docs/evidence/vue-review-', 'docs/DESIGN-REFACTOR-OUTPUT')):
            raise ValueError('recursive generated source: ' + path)
        raw = resolved.read_bytes()
        if len(raw) > 2000000:
            raise ValueError('source too large: ' + path)
        sid = stable('source', path)
        self.sources[sid] = {'id': sid, 'path': path, 'sha256': sha(raw), 'text': raw.decode('utf-8')}
        return sid

    def text(self, path):
        self.source(path)
        return (ROOT / path).read_text(encoding='utf-8')

    def bind(self, model, id_, label, paths, details=None, description=None, status=None, scope=None):
        if id_ in model['bindings']:
            raise ValueError('duplicate element binding: ' + id_)
        binding = {'label': label, 'details': [str(x)[:1900] for x in (details or [])], 'source_ids': list(dict.fromkeys(self.source(p) for p in paths))}
        if description:
            binding['description'] = description
        if status:
            binding['status'] = status
        if scope:
            binding['scope'] = scope
        model['bindings'][id_] = binding
        return id_

    def model(self, id_, title, type_, category, description, paths, semantics, scope='design', status='docs_ready'):
        model = {'id': id_, 'title': title, 'diagram_type': type_, 'category': category, 'description': description, 'scope': scope, 'status': status, 'source_ids': [self.source(p) for p in paths], 'semantics': semantics, 'data': {'id': id_, 'title': title, 'description': description}, 'bindings': {}}
        self.models.append(model)
        return model

    def state(self):
        text = self.text(INVARIANTS)
        rows = table(text, '状态与事件基线')
        m = self.model('state', '状态、恢复与门禁', 'state', '行为与权限', '冻结十一态与全部合法事件；恢复是受约束的保存态，终态重做创建新 Run。', [INVARIANTS, BUSINESS], ['箭头标示事件与条件，不是运行记录。', '未列转移默认拒绝；全局版本、权限、预算与未知写核实优先。', '保存态仅来自被打断的非终止阶段；终态没有当前 Run 执行出口。'])
        nodes = []
        edges = []
        positions = {'INTAKE': (0, 1), 'CLARIFY': (1, 0), 'PLAN': (2, 1), 'IMPLEMENT': (3, 1), 'VERIFY': (4, 1), 'DELIVER': (5, 1), 'DONE': (6, 1), 'PAUSED': (3, 0), 'BLOCKED': (4, 2), 'FAILED': (5, 3), 'CANCELLED': (2, 3)}
        for state, transitions, guard in rows:
            col, row = positions[state]
            nodes.append({'id': state, 'label': state, 'kind': 'state', 'column': col, 'row': row, 'actions': [plain(guard)], 'tone': 'success' if state == 'DONE' else 'warning' if state in {'PAUSED', 'BLOCKED', 'FAILED', 'CANCELLED'} else 'accent'})
            self.bind(m, state, state, [INVARIANTS], [transitions, guard], status='docs_ready')
            for event in transitions.split('；'):
                if '→' not in event:
                    continue
                label, target = event.split('→', 1)
                target = 'saved_nonterminal' if target == '已保存非终止状态' else 'new_run' if target == '关联新 Run' else target
                id_ = stable('transition', state, label, target)
                detail = '跨 Run 创建；不改变当前终态，不继承旧批准。' if target == 'new_run' else '仅恢复保存的非终止阶段，重新检查版本、批准、权限、预算；未知写未核实不放行。' if target == 'saved_nonterminal' else plain(guard)
                edges.append({'id': id_, 'label': label, 'source': state, 'target': target, 'description': detail, 'tone': 'warning' if target in {'PAUSED', 'BLOCKED', 'FAILED', 'CANCELLED'} else 'accent'})
                self.bind(m, id_, state + ' → ' + target, [INVARIANTS], [event, guard, detail])
        for id_, label, row in [('saved_nonterminal', '保存的非终止阶段', 0), ('new_run', '关联的新 Run', 3)]:
            nodes.append({'id': id_, 'label': label, 'kind': 'history', 'column': 6, 'row': row, 'actions': ['受保存态 / 新身份约束']})
            self.bind(m, id_, label, [INVARIANTS], ['这是范围说明节点，不是额外业务状态。'])
        for gate, owner, required, failure in table(text, '确认与质量门禁'):
            self.bind(m, 'gate.' + gate.split()[0], gate, [INVARIANTS], [owner, required, failure])
        m['data'].update(nodes=nodes, edges=edges, groups=[{'id': 'terminal', 'label': '终态：当前 Run 只读', 'description': '重做创建新身份；核实既有副作用不等于复活。', 'members': ['DONE', 'FAILED', 'CANCELLED']}])
        self.bind(m, 'terminal', '终态只读', [INVARIANTS], ['DONE / FAILED / CANCELLED 不发生当前 Run 执行转移。'])

    def responsibility(self):
        rows = table(self.text(INVARIANTS), '权限与职责矩阵')
        m = self.model('responsibility', '职责与权限泳道', 'swimlane', '行为与权限', '八类角色的可做/不可做边界与确定性执行交接。', [INVARIANTS, BUSINESS, AGENT_DOC], ['泳道对应规范角色；节点描述职责，非实际执行日志。', '交接必须经过确定性裁决，普通需求和文档查看不产生授权。'])
        lanes, nodes, edges = [], [], []
        ids = ['user', 'client', 'model', 'control', 'executor', 'verifier', 'ops', 'reviewer']
        for id_, (role, allowed, forbidden) in zip(ids, rows):
            lanes.append({'id': id_, 'label': role, 'description': '不可做：' + forbidden})
            node_id = 'role.' + id_
            nodes.append({'id': node_id, 'label': allowed.split('、')[0], 'lane': id_, 'column': 0, 'caption': '规范职责', 'details': ['可以：' + allowed, '禁止：' + forbidden]})
            self.bind(m, id_, role, [INVARIANTS], ['可以：' + allowed, '禁止：' + forbidden])
            self.bind(m, node_id, role + '职责', [INVARIANTS], ['可以：' + allowed, '禁止：' + forbidden])
        handoffs = [('user', 'client', '具体范围意图'), ('client', 'control', '身份/版本绑定命令'), ('model', 'control', 'DecisionProposal（建议）'), ('control', 'executor', 'ExecutionRequest（获准范围）'), ('executor', 'verifier', '实际工件/操作证据'), ('verifier', 'control', '固定检查与同源报告'), ('control', 'client', '权威快照/回执')]
        for a, b, label in handoffs:
            id_ = stable('handoff', a, b, label)
            edges.append({'id': id_, 'source': 'role.' + a, 'target': 'role.' + b, 'label': label})
            self.bind(m, id_, label, [INVARIANTS, AGENT_DOC], ['公开对象和职责投影；运维与文档审查不加入运行授权链。'])
        m['data'].update(lanes=lanes, nodes=nodes, edges=edges)

    def sequence(self):
        m = self.model('sequence', '创建与异常核实时序', 'sequence', '行为与权限', '目标公开契约的设计交互场景；包含旧批准拒绝、未知写核实和断流重取。', [AGENT_DOC, BUSINESS, INVARIANTS], ['这是设计协议场景，非真实请求记录。', '模型仅返回建议；只有获准请求交付执行器。', '未知写按原 operation/command 查询，禁止盲重放。'])
        participants = [{'id': id_, 'label': label, 'kind': kind} for id_, label, kind in [('user', '用户', 'actor'), ('web', 'Vue 客户端', 'service'), ('api', '公开API/编排', 'service'), ('model', 'Agent建议', 'service'), ('control', '确定性治理', 'service'), ('executor', '受控执行器', 'service'), ('verifier', '产品内验证', 'service')]]
        for p in participants:
            self.bind(m, p['id'], p['label'], [INVARIANTS, AGENT_DOC], ['角色与公开职责；不代表实际服务部署。'])
        scenes = [('user', 'web', '提交需求与模式', 'sync', BUSINESS), ('web', 'api', 'POST /api/v2/runs · CreateRunRequest', 'sync', AGENT_DOC), ('api', 'model', '冻结 DefinitionSnapshot + ContextEnvelope', 'sync', 'design/02-agent/features/context-building.md'), ('model', 'control', 'DecisionAdvice → DecisionProposal', 'return', AGENT_DOC), ('control', 'api', '方案规则结果 / 未应用Proposal', 'return', AGENT_DOC), ('api', 'web', 'CommandReceipt / run_ref', 'return', AGENT_DOC), ('web', 'user', '展示具体版本/范围/有效期', 'return', 'design/05-conversation/features/preview-confirmation.md'), ('user', 'web', '确认当前 proposal_ref', 'sync', 'design/05-conversation/features/preview-confirmation.md'), ('web', 'api', 'POST actions · ControlRequest', 'sync', AGENT_DOC), ('api', 'control', '版本/期限/权限/预算复核', 'sync', INVARIANTS), ('control', 'web', '旧批准失配：拒绝且不执行', 'return', 'design/05-conversation/features/preview-confirmation.md'), ('control', 'executor', '有效分支：ExecutionRequest', 'sync', AGENT_DOC), ('executor', 'api', '未知外部写：BLOCKED + 原 operation_id', 'return', 'design/04-integrations/features/unknown-results.md'), ('api', 'executor', '原身份核实；未核实不重放', 'sync', 'design/04-integrations/features/unknown-results.md'), ('executor', 'verifier', 'manifest / commit / pipeline 同源证据', 'sync', 'design/07-observability/features/verification-evidence.md'), ('verifier', 'api', 'RequiredCheck / VerificationReport', 'return', AGENT_DOC), ('api', 'web', 'SSE SnapshotEvent → run_ref', 'async', AGENT_DOC), ('web', 'api', '断流/缺口：GET快照或原命令回执', 'sync', 'design/08-frontend/features/state-stream-sync.md')]
        messages = []
        for a, b, label, kind, path in scenes:
            id_ = stable('message', a, b, label)
            messages.append({'id': id_, 'from': a, 'to': b, 'label': label, 'kind': kind})
            self.bind(m, id_, label, [path, AGENT_DOC], ['场景条件分支，拒绝/核实与有效执行不会同时成为已发生事实。'])
        m['data'].update(participants=participants, messages=messages, activations=[], notes=[{'after': 10, 'label': '拒绝分支结束；有效分支继续。全部为设计场景。', 'from': 'control', 'to': 'executor'}])

    def refs(self, value, path=()):
        if isinstance(value, dict):
            if '$ref' in value:
                yield path, value['$ref']
            for k, v in value.items():
                yield from self.refs(v, path + (k,))
        elif isinstance(value, list):
            for index, v in enumerate(value):
                yield from self.refs(v, path + (str(index),))

    def type_label(self, value):
        if isinstance(value, bool):
            return 'any JSON' if value else 'forbidden'
        if '$ref' in value:
            return value['$ref'].split('/')[-1]
        if 'const' in value:
            return '=' + json.dumps(value['const'], ensure_ascii=False)
        if 'enum' in value:
            return ' | '.join(str(x) for x in value['enum'])
        if value.get('type') == 'array':
            return self.type_label(value.get('items', {})) + '[]'
        if 'type' in value:
            return str(value['type'])
        for k in ['oneOf', 'anyOf', 'allOf']:
            if k in value:
                return ' / '.join(self.type_label(v) for v in value[k])[:160]
        return 'JSON Schema'

    def resolve_schema_ref(self, value, path):
        seen = set()
        while isinstance(value, dict) and '$ref' in value:
            key = (path, value['$ref'])
            if key in seen:
                raise ValueError('cyclic semantic field ref')
            seen.add(key)
            uri, _, fragment = value['$ref'].partition('#')
            if uri:
                path, _schema = self.by_schema_id[uri] if uri in self.by_schema_id else next((p, s) for p, s in self.schemas.items() if p.endswith('/' + uri))
            value = self.schemas[path]
            for segment in fragment.lstrip('/').split('/'):
                value = value[segment.replace('~1', '/').replace('~0', '~')]
        return value, path

    def semantic_field(self, definition, field):
        value, path = self.schemas[AGENT_SCHEMA]['$defs'][definition], AGENT_SCHEMA
        for segment in field.split('.'):
            value, path = self.resolve_schema_ref(value, path)
            if segment not in value.get('properties', {}):
                raise ValueError('semantic binding invents field: ' + definition + '.' + field)
            value = value['properties'][segment]
        return value

    def semantic_relationships(self):
        relationships = []
        for source, target, pairs, note in SEMANTIC_BINDINGS:
            for a, b in pairs:
                self.semantic_field(source, a)
                self.semantic_field(target, b)
            predicate = ';'.join(a + '=' + b for a, b in pairs)
            id_ = stable('semantic_binding', source, target, predicate)
            description = '字段绑定；来自公开对象语义，区别于$ref结构依赖，不表示执行调度。' + note
            relationships.append(({'id': id_, 'source': 'agent.' + source, 'target': 'agent.' + target, 'label': '字段绑定：' + pairs[0][0], 'kind': 'dependency', 'description': description}, predicate, note))
        return relationships

    def contracts(self):
        m = self.model('contracts', '公共 JSON 契约对象', 'class', '结构与来源', '完整三份 Schema 的全部 $defs、字段和引用；不赋予数据库主外键含义。', [AGENT_SCHEMA, MESSAGE_SCHEMA, COMMON_SCHEMA, AGENT_DOC], ['每个类是公开 JSON Schema 定义；属性保留必需/可选与类型。', '$ref结构依赖与字段ID/hash语义绑定分开标注；字段详情可查原始Schema。', '这不是数据库 ER 或实际 Go/TS 类，方法保持为空。'])
        classes, relationships = [], []
        for path, schema in self.schemas.items():
            namespace = path.rsplit('/', 1)[-1].split('.')[0]
            for name, definition in schema['$defs'].items():
                id_ = namespace + '.' + name
                required = definition.get('required', [])
                attributes = []
                for field, field_schema in definition.get('properties', {}).items():
                    label = field + ('' if field in required else '?') + ': ' + self.type_label(field_schema)
                    attributes.append(label)
                    self.bind(m, id_ + '.' + field, label, [path], [json.dumps(field_schema, ensure_ascii=False, sort_keys=True)], description='required' if field in required else 'optional')
                if not attributes:
                    attributes = [self.type_label(definition)]
                classes.append({'id': id_, 'label': name, 'stereotype': namespace + ' JSON Schema', 'attributes': attributes, 'methods': [], 'description': definition.get('description', '来自已发布 $defs/' + name)[:1900]})
                self.bind(m, id_, name, [path], ['Schema ID: ' + schema['$id'], '$defs/' + name, 'required: ' + ', '.join(required), 'additionalProperties: ' + str(definition.get('additionalProperties', 'inherited'))], description=json.dumps(definition, ensure_ascii=False, sort_keys=True)[:1900])
                for location, reference in self.refs(definition):
                    uri, _, fragment = reference.partition('#')
                    target_path, _target_schema = (self.by_schema_id[uri] if uri in self.by_schema_id else next((p, s) for p, s in self.schemas.items() if p.endswith('/' + uri))) if uri else (path, schema)
                    if not fragment.startswith('/$defs/'):
                        raise ValueError('unexpected public Schema reference: ' + reference)
                    target_name = fragment.split('/')[2]
                    target = target_path.rsplit('/', 1)[-1].split('.')[0] + '.' + target_name
                    rid = stable('ref', id_, location, reference)
                    relationships.append({'id': rid, 'source': id_, 'target': target, 'label': '.'.join(location).replace('properties.', '')[:160], 'kind': 'dependency'})
                    self.bind(m, rid, id_ + ' → ' + target, [path, target_path], ['$ref=' + reference, 'location=' + '/'.join(location)])
        for relationship, predicate, note in self.semantic_relationships():
            relationships.append(relationship)
            self.bind(m, relationship['id'], relationship['label'], [AGENT_SCHEMA, COMMON_SCHEMA, AGENT_DOC], ['predicate=' + predicate, note, '规范字段绑定，不是JSON Schema $ref，不是运行调度；实际身份/hash与权限仍须独立语义校验。'], description=relationship['description'])
        m['data'].update(classes=classes, relationships=relationships)

    def lineage(self):
        m = self.model('lineage', '批准、工件与证据来源链', 'lineage', '结构与来源', '字段/hash绑定的目标血缘链；仅结构和规则，不显示样例hash为真实批准。', [AGENT_SCHEMA, AGENT_DOC, HASHING, INVARIANTS], ['节点字段从公共 Schema 派生，连线标注被绑定的来源字段。', '逻辑 JCS 与原字节摘要分开；各对象排除字段清单以归属契约为准。', '批准是用户+服务端签发事实；模型建议不是批准。'])
        stages = []
        groups = [('definition', '冻结依赖', ['AgentDefinition', 'SkillDefinition', 'DefinitionSnapshot']), ('suggestion', '建议与具体方案', ['DecisionProposal', 'Proposal']), ('authorization', '授权与确定性裁决', ['Approval', 'ActionValidation', 'ExecutionRequest']), ('proof', '实际结果与完成证据', ['ArtifactManifest', 'RequiredCheck', 'VerificationReport'])]
        defs = self.schemas[AGENT_SCHEMA]['$defs']
        for stage_id, label, names in groups:
            stage = {'id': stage_id, 'label': label, 'description': '目标结构；未声称已完成实现', 'nodes': []}
            self.bind(m, stage_id, label, [AGENT_DOC, AGENT_SCHEMA], ['目标数据绑定阶段，不表示运行 Task DAG。'])
            for name in names:
                fields = [k for k in defs[name].get('properties', {}) if any(s in k for s in ['hash', 'version', '_id', '_ref'])]
                stage['nodes'].append({'id': name, 'label': name, 'fields': fields, 'subtitle': '规范绑定字段'})
                self.bind(m, name, name, [AGENT_SCHEMA, AGENT_DOC, HASHING], fields + ['样例hash仅为结构占位，不是运行批准。'])
            stages.append(stage)
        links = [('AgentDefinition', 'DefinitionSnapshot', 'definition / definition_hash'), ('SkillDefinition', 'DefinitionSnapshot', 'skills / skill_hash'), ('DefinitionSnapshot', 'DecisionProposal', 'definition_snapshot_hash'), ('DecisionProposal', 'ActionValidation', 'decision_id'), ('Proposal', 'Approval', 'proposal_id / version / snapshot_hash'), ('Approval', 'ExecutionRequest', 'approval'), ('ActionValidation', 'ExecutionRequest', 'validation_id / validation_hash'), ('ExecutionRequest', 'ArtifactManifest', 'run/source/definition/plan批准链'), ('ArtifactManifest', 'VerificationReport', 'manifest_ref'), ('RequiredCheck', 'VerificationReport', 'checks'), ('Approval', 'RequiredCheck', 'proposal_snapshot_hash')]
        connections = []
        for a, b, label in links:
            id_ = stable('binding', a, b, label)
            connections.append({'id': id_, 'source': a, 'target': b, 'label': label})
            self.bind(m, id_, label, [AGENT_SCHEMA, AGENT_DOC, INVARIANTS], ['关联范围按 Run/source/definition 与批准快照共同校验，Schema字段存在不证明内容真实。'])
        m['data'].update(stages=stages, connections=connections)

    def architecture(self):
        m = self.model('architecture', '模块职责与三条独立边界', 'architecture', '结构与来源', '由目标模块映射、领域关系和隔离约束派生；当前定位不表示目标包已经存在。', [MAP, 'AGENTS.md', 'design/_model/entities.yaml', 'design/_model/relations.yaml'], ['目标目录只表职责归属，未实施包不显示已拆分。', '设计工具生成静态审查发布物；产品不导入checker/validation。', '连接是公开契约消费或构建依赖，不代表运行调度。'])
        domains = [e for e in self.entities if e['type'] == 'domain']
        layers = [{'id': 'product', 'label': '产品职责（目标）', 'description': 'Go/Temporal产品 + Vue公开消费', 'nodes': []}, {'id': 'design_tool', 'label': '离线设计工具', 'description': '不参与Run授权、状态与DONE', 'nodes': []}, {'id': 'blackbox', 'label': '独立黑盒调用器', 'description': '只消费公开接口与工件', 'nodes': []}]
        for d in domains:
            path = 'design/' + d['path']
            layers[0]['nodes'].append({'id': d['id'], 'label': d['name'], 'subtitle': '目标职责', 'description': plain(self.text(path).split('## ', 1)[0])[:1900]})
            self.bind(m, d['id'], d['name'], [path, MAP], ['对应MODULE-MAP的目标目录；当前实现定位是迁移入口。'], status='planned')
        extra = [('design.cognitive', '认知模型/checker', 'design_tool'), ('design.publisher', '审查数据发布器', 'design_tool'), ('validation.blackbox', '公开HTTP/SSE/ZIP验收', 'blackbox')]
        for id_, label, layer in extra:
            next(l for l in layers if l['id'] == layer)['nodes'].append({'id': id_, 'label': label, 'subtitle': '独立边界'})
            self.bind(m, id_, label, ['AGENTS.md', MAP], ['Agent运行不导入此模块；只读审查静态发布物与外部验收证据分开。'])
        for l in layers:
            self.bind(m, l['id'], l['label'], ['AGENTS.md', MAP], [l['description']])
        domain_map = {e['id']: e.get('domain') for e in self.entities}
        pairs = {}
        for r in self.relations:
            a, b = domain_map.get(r['from']), domain_map.get(r['to'])
            if a and b and a != b and r['type'] in {'consumes', 'submits_to'}:
                pairs.setdefault((a, b), []).append(r)
        connections = []
        for (a, b), rels in pairs.items():
            id_ = stable('domain_contract', a, b)
            connections.append({'id': id_, 'source': a, 'target': b, 'label': str(len(rels)) + '个公开契约关系'})
            paths = ['design/_model/relations.yaml'] + ['design/' + self.entity_map[r['from']]['path'] for r in rels] + ['design/' + self.entity_map[r['to']]['path'] for r in rels]
            self.bind(m, id_, a + ' → ' + b, list(dict.fromkeys(paths)), [r['from'] + ' ' + r['type'] + ' ' + r['to'] + '#' + r.get('contract', '') for r in rels])
        for a, b, label in [('design.cognitive', 'design.publisher', '离线结构输入'), ('design.publisher', 'domain.frontend', '构建发布静态bundle'), ('validation.blackbox', 'domain.deploy', '公开服务/包黑盒调用')]:
            id_ = stable('boundary', a, b, label)
            connections.append({'id': id_, 'source': a, 'target': b, 'label': label, 'dashed': True})
            self.bind(m, id_, label, ['AGENTS.md', MAP, MODELS], ['构建/公开消费关系；不导入产品内部对象或作为DONE来源。'])
        m['data'].update(layers=layers, connections=connections)

    def coverage(self):
        m = self.model('coverage', '设计—实现—验收覆盖', 'coverage', '验收与演进', '21个目标差异与18个计划用例；历史证据独立显示，不能关闭当前目标契约缺口。', [GAPS, PLAN] + EVIDENCE, ['状态是离散类别，不是通过百分比或热力分数。', '历史路径/浏览器结果保留原代码时点；目标契约C01–C18仍planned。', '文档检查与live第三方分别登记；报告存在不代表当前代码重跑。'], scope='current', status='planned')
        rows = []
        for line in self.text(GAPS).splitlines():
            if re.match(r'^\| G\d{2} /', line):
                cells = [c.strip() for c in line.strip('|').split('|')]
                match = re.fullmatch(r'(G\d{2}) / (P[01])', cells[0])
                if not match:
                    raise ValueError('gap parser drift')
                id_, priority = match.groups()
                cases = sorted(set(re.findall(r'C\d{2}', ' '.join(cells))))
                label = plain(cells[1])[:95]
                rows.append({'id': id_, 'label': label, 'priority': priority, 'description': plain(' | '.join(cells[1:]))[:1900], 'case_ids': cases})
                self.bind(m, id_, id_ + ' ' + label, [GAPS], [plain(c) for c in cells[1:]], status='docs_fixed' if id_ == 'G21' else 'planned', scope='current')
        cases = []
        for line in self.text(PLAN).splitlines():
            if re.match(r'^\| C\d{2} \|', line):
                c = [plain(x) for x in line.strip('|').split('|')]
                cases.append({'id': c[0], 'feature_id': c[1], 'contract': c[2], 'expectation': c[3], 'phase': c[4], 'status': 'planned'})
                self.bind(m, c[0], c[0] + ' ' + c[2], [PLAN], c[1:], status='planned', scope='design')
        evidence = []
        for path in EVIDENCE:
            report = load_json(ROOT / path)
            label = path.rsplit('/', 1)[-1]
            id_ = stable('evidence', path)
            recorded_at = report.get('recorded_at') or report.get('started_at') or report.get('generated_at') or report.get('finished_at') or '原报告未登记时间；历史保存物'
            is_docs = 'contract-' in label
            passed = report.get('passed') is True or report.get('status') == 'passed'
            status = ('historical_docs_verified' if is_docs else 'historical_path_verified') if passed else 'historical_not_verified'
            desc = '历史S0文档结构/样例证据；不证明运行v2。' if is_docs else '历史legacy路径/浏览器报告；不继承为当前代码或目标v2结果。'
            evidence.append({'id': id_, 'label': label, 'recorded_at': recorded_at, 'status': status, 'scope': 'evidence', 'source_id': self.source(path), 'description': desc})
            self.bind(m, id_, label, [path], ['时点：' + recorded_at, desc, '原报告结果：' + str(report.get('passed', report.get('summary', '见完整来源')))], status=status, scope='evidence')
        columns = [{'id': id_, 'label': label} for id_, label in [('document', '文档依据'), ('contract', '目标契约'), ('path', '当前路径'), ('browser', '当前浏览器'), ('live', '真实第三方')]]
        for c in columns:
            self.bind(m, c['id'], c['label'], [GAPS, PLAN], ['证据维度分别计算，不能将文档或历史路径挪作目标契约。'])
        cells = []
        doc_evidence = [e['id'] for e in evidence if e['status'] == 'historical_docs_verified']
        for row in rows:
            for column in columns:
                status = 'docs_fixed' if row['id'] == 'G21' and column['id'] == 'document' else 'docs_ready' if column['id'] == 'document' else 'planned' if column['id'] == 'contract' else 'not_run'
                id_ = row['id'] + '.' + column['id']
                detail = '公开规范及差异已登记；历史S0检查单独保留时点。' if column['id'] == 'document' else '未执行本目标的当前代码验收；不能由历史20/20或15/15推断。'
                cells.append({'id': id_, 'row': row['id'], 'column': column['id'], 'status': status, 'label': status, 'detail': detail, 'case_ids': row['case_ids'], 'evidence_ids': doc_evidence if column['id'] == 'document' else []})
                self.bind(m, id_, row['id'] + ' · ' + column['label'], [GAPS, PLAN], [status, detail] + row['case_ids'], status=status, scope='current')
        m['data'].update(rows=rows, columns=columns, cells=cells, cases=cases, evidence=evidence)

    def versions(self):
        m = self.model('versions', '身份与版本演进设计场景', 'timeline', '验收与演进', '日期仅是设计场景坐标；普通进度、方案修订、批准失效、CAN与重做分别表达。', [INVARIANTS, AGENT_DOC, 'design/01-orchestrator/features/continue-as-new.md'], ['全部事件planned，日期不是已发生历史或交付排期。', '业务范围只增source_version；普通进度只增state_sequence。', 'CAN保持业务Run/operation身份；终态重做新身份且不继承旧批准。'])
        scenarios = [('create', '创建并冻结', 'run=A；source=1；state_sequence=1；绑定完整Definition/Skill依赖。', 'INV-004'), ('preview', '预览与具体批准', 'proposal_version=1内容不可变；服务端issued_at/expires_at；批准绑定source/hash/scope。', 'INV-010'), ('progress', '普通执行进度', 'source保持1；state_sequence增加；同源授权不会因普通进度自动失效。', 'INV-012'), ('revision', '业务范围修订', 'source增加为2；新proposal_version；旧批准不能授权新内容，先重新预览确认。', 'INV-011'), ('expiry', '批准期限到达', '未开始的新动作不能以旧批准启动；已开始原范围链按规范逐步复核，不扩大权限。', 'INV-010'), ('can', 'Continue-As-New', 'Temporal执行身份改变；run=A、operation/command身份及冻结快照/账本保持。', 'INV-029'), ('redo', '终态重做', 'DONE/FAILED/CANCELLED只读；新run=B带retry_of=A，新确认，不原地复活。', 'INV-014')]
        events = []
        base = datetime(2026, 10, 2)
        for index, (id_, label, desc, invariant) in enumerate(scenarios):
            id_ = 'scenario.' + id_
            events.append({'id': id_, 'date': (base + timedelta(days=index)).date().isoformat(), 'label': label, 'description': '设计场景：' + desc, 'status': 'planned'})
            self.bind(m, id_, label, [INVARIANTS, AGENT_DOC, 'design/' + self.entity_map[invariant]['path'].split('#')[0]], [desc, '场景序列，非实际Run历史。'], status='planned', scope='design')
        m['data']['events'] = events

    def knowledge(self):
        m = self.model('knowledge', '全部文档实体与公开关系', 'knowledge', '结构与来源', '完整登记关系；可通过目录/搜索查看领域、功能、ADR与不变量，不能当作运行DAG。', ['design/_model/entities.yaml', 'design/_model/relations.yaml', 'design/_model/SCHEMA.md'], ['四种实体类型和全部关系直接来自登记，不加入运行对象。', 'consumes关联目标输出；submits_to关联目标输入。', '反馈环允许存在，但不说明Workflow或Task调度可有环。'])
        colors = {'domain': '#6265e8', 'feature': '#47a58b', 'adr': '#d5a34d', 'invariant': '#a8689a'}
        nodes, edges = [], []
        for e in self.entities:
            path = 'design/' + e['path'].split('#')[0]
            nodes.append({'id': e['id'], 'label': e['name'], 'type': e['type'], 'description': e['path'], 'color': colors[e['type']]})
            self.bind(m, e['id'], e['name'], [path, 'design/_model/entities.yaml'], ['type=' + e['type'], 'path=' + e['path']] + (['domain=' + e['domain']] if 'domain' in e else []))
        for r in self.relations:
            id_ = stable('relation', r['from'], r['to'], r['type'], r.get('contract', ''))
            edges.append({'id': id_, 'source': r['from'], 'target': r['to'], 'label': r['type'] + (' #' + r['contract'] if 'contract' in r else '')})
            paths = ['design/_model/relations.yaml', 'design/' + self.entity_map[r['from']]['path'].split('#')[0], 'design/' + self.entity_map[r['to']]['path'].split('#')[0]]
            self.bind(m, id_, r['type'], list(dict.fromkeys(paths)), [r['from'] + ' → ' + r['to'], 'contract=' + r.get('contract', 'n/a'), '登记关联；不表示运行调度。'])
        m['data'].update(nodes=nodes, edges=edges)

    def skills(self):
        sample_path = 'design/contracts/examples/run-snapshot-intake.valid.json'
        snapshot = load_json(ROOT / sample_path)['definition_snapshot']
        m = self.model('skills', 'Skill能力与冻结依赖', 'dependency', '结构与来源', '三Skill公开定义形状及参数/工具/模板依赖；示例hash为占位，不代表当前运行注册表。', [sample_path, AGENT_SCHEMA, 'design/03-skills/features/skill-contract.md'], ['依赖与默认值从完整公开示例派生，明确属于目标设计。', '空dependencies表示示例未声明Skill间依赖，不能凭业务想象连线。', 'parameter_schema、模板、权限、工具及检查均需进入真实冻结包。'], status='planned')
        columns = [{'id': 'skill', 'label': 'Skill公开定义（目标示例）', 'nodes': []}, {'id': 'parameters', 'label': '参数/权限/检查', 'nodes': []}, {'id': 'resources', 'label': '模板/工具', 'nodes': []}]
        connections = []
        for skill in snapshot['skills']:
            id_ = skill['skill_id']
            columns[0]['nodes'].append({'id': id_, 'label': skill['name'], 'version': skill['skill_version'], 'subtitle': id_, 'description': skill['description']})
            details = [key + ': ' + json.dumps(skill[key], ensure_ascii=False) for key in ['supported_stacks', 'default_parameters', 'dependencies', 'conflicts', 'write_paths', 'required_permissions', 'required_check_ids', 'post_commands']]
            self.bind(m, id_, skill['name'], [sample_path, AGENT_SCHEMA], details + ['示例hash不是发布或批准事实。'], status='planned')
            params = id_ + '.parameters'
            columns[1]['nodes'].append({'id': params, 'label': '参数与确定性约束', 'subtitle': id_})
            self.bind(m, params, skill['name'] + '参数', [sample_path, AGENT_SCHEMA], [json.dumps(skill['parameters_schema'], ensure_ascii=False), 'defaults=' + json.dumps(skill['default_parameters'], ensure_ascii=False)])
            connections.append({'id': id_ + '.parameter_ref', 'source': id_, 'target': params, 'kind': 'runtime', 'label': 'Schema / 默认值'})
            self.bind(m, id_ + '.parameter_ref', '参数Schema', [sample_path, AGENT_SCHEMA], ['规范化后重校验；default注释不自动补值。'])
            for index, item in enumerate(skill['template_dependencies'] + skill['required_tools']):
                resource_id = id_ + '.resource.' + str(index)
                label = item.get('template_id', item.get('tool_id'))
                version = item.get('template_version', item.get('version'))
                columns[2]['nodes'].append({'id': resource_id, 'label': label, 'version': version, 'subtitle': '模板' if 'template_id' in item else '工具'})
                self.bind(m, resource_id, label, [sample_path, AGENT_SCHEMA], [json.dumps(item, ensure_ascii=False), '这是设计示例依赖；占位digest不是真实内容核验。'])
                edge = resource_id + '.bind'
                connections.append({'id': edge, 'source': id_, 'target': resource_id, 'kind': 'build' if 'template_id' in item else 'runtime', 'label': '完整版本绑定'})
                self.bind(m, edge, '完整版本绑定', [sample_path, AGENT_SCHEMA], ['Skill hash / DefinitionSnapshot涵盖真实依赖，示例digest不可作授权。'])
            for dependency in skill['dependencies']:
                target = dependency['skill_id']
                edge = stable('skill_dependency', id_, target)
                connections.append({'id': edge, 'source': id_, 'target': target, 'kind': 'runtime', 'label': '登记依赖'})
                self.bind(m, edge, '登记依赖', [sample_path, AGENT_SCHEMA], [json.dumps(dependency, ensure_ascii=False)])
        for c in columns:
            self.bind(m, c['id'], c['label'], [sample_path, AGENT_SCHEMA], ['设计分组；不是已执行步骤。'])
        m['data'].update(columns=columns, connections=connections)

    def context(self):
        p_build = 'design/02-agent/features/context-building.md'
        p_summary = 'design/02-agent/features/context-compression.md'
        p_raw = 'design/05-conversation/features/jsonl-message-store.md'
        p_budget = 'design/06-governance/features/budget-limits.md'
        m = self.model('context', '原文、派生摘要与模型上下文', 'dataflow', '结构与来源', '授权对象与原始对话分层注入，摘要只作派生材料，预算硬约束保留当前完整请求。', [p_build, p_summary, p_raw, p_budget, AGENT_SCHEMA], ['连线表示数据流，不表示执行控制或批准。', '原JSONL追加保存；SummaryRecord冻结来源范围并保留历史。', '摘要失败保留上一版/原文；权限和完成条件不从摘要恢复。'])
        items = [('user', '当前完整用户请求', 'external', p_build), ('raw', '追加原文JSONL', 'store', p_raw), ('summary_source', '冻结摘要来源范围', 'process', p_summary), ('summaries', 'SummaryRecord历史', 'store', p_summary), ('authority', '权威批准/权限/状态', 'store', p_build), ('definition', '固定规则与动作协议', 'store', p_build), ('references', '同Run稳定工件引用', 'store', p_build), ('builder', '分层ContextBuilder', 'process', p_build), ('budget', 'token预算/输出预留', 'process', p_budget), ('model', '模型只读上下文', 'external', p_build)]
        nodes = []
        for id_, label, kind, path in items:
            nodes.append({'id': id_, 'label': label, 'kind': kind, 'description': plain(self.text(path).split('## 输出', 1)[1].split('## ', 1)[0])[:1900]})
            self.bind(m, id_, label, [path, AGENT_SCHEMA], [plain(self.text(path).split('## 输出', 1)[1].split('## ', 1)[0])[:1900]])
        links = [('user', 'raw', '追加完整原文'), ('raw', 'summary_source', 'message_ids/sequence/raw_jsonl_sha256'), ('summary_source', 'summaries', '成功派生版本；失败保留旧版'), ('summaries', 'builder', '同Run摘要引用（非授权）'), ('raw', 'builder', '最近消息与完整请求'), ('user', 'builder', '当前完整请求不可静默截断'), ('authority', 'builder', '权威批准/预算/完成状态'), ('definition', 'builder', '绑定的系统规则/协议'), ('references', 'builder', '允许读取的稳定引用'), ('builder', 'budget', '分层输入 + 裁剪清单'), ('budget', 'model', '仅预算通过的ContextEnvelope')]
        connections = []
        for a, b, label in links:
            id_ = stable('dataflow', a, b, label)
            connections.append({'id': id_, 'source': a, 'target': b, 'label': label})
            self.bind(m, id_, label, [p_build, p_summary, p_budget, AGENT_SCHEMA], ['保护原文、当前请求和权威授权；越界引用/必要层超限拒绝，不静默降级。'])
        m['data'].update(nodes=nodes, connections=connections)

    def build(self):
        for method in [self.state, self.responsibility, self.sequence, self.contracts, self.lineage, self.architecture, self.coverage, self.versions, self.knowledge, self.skills, self.context]:
            method()
        return {'schema_version': 1, 'generated_at': datetime.now(timezone.utc).isoformat().replace('+00:00', 'Z'), 'baseline': {'source_sha256': self.constraints['source_sha256'], 'invariants_sha256': self.constraints['invariants_sha256'], 'contract_version': '1.0.0'}, 'sources': sorted(self.sources.values(), key=lambda s: s['path']), 'models': self.models}


def element_ids(data):
    ids = []
    def walk(value):
        if isinstance(value, dict):
            if 'id' in value:
                ids.append(value['id'])
            for v in value.values():
                walk(v)
        elif isinstance(value, list):
            for v in value:
                walk(v)
    for key, value in data.items():
        if key != 'id':
            walk(value)
    return ids


def check(bundle, publisher, source_check=True):
    try:
        import jsonschema
    except ImportError as exc:
        raise ValueError('需要完整Draft2020-12验证器：用隔离环境安装 jsonschema[format]，不能省略Schema检查') from exc
    format_checker = jsonschema.FormatChecker()
    if 'date-time' not in format_checker.checkers:
        raise ValueError('date-time格式断言不可用；安装jsonschema[format]')
    schema = load_json(SCHEMA)
    jsonschema.Draft202012Validator.check_schema(schema)
    jsonschema.Draft202012Validator(schema, format_checker=format_checker).validate(bundle)
    if bundle['baseline'] != {'source_sha256': publisher.constraints['source_sha256'], 'invariants_sha256': publisher.constraints['invariants_sha256'], 'contract_version': '1.0.0'}:
        raise ValueError('frozen baseline mismatch')
    for path, expected in [('docs/PRODUCT-GUIDE.md', bundle['baseline']['source_sha256']), (INVARIANTS, bundle['baseline']['invariants_sha256'])]:
        if sha((ROOT / path).read_bytes()) != expected:
            raise ValueError('frozen source changed: ' + path)
    sources = {s['id']: s for s in bundle['sources']}
    if len(sources) != len(bundle['sources']):
        raise ValueError('duplicate source IDs')
    for s in sources.values():
        path = (ROOT / s['path']).resolve()
        if not path.is_relative_to(ROOT) or not path.is_file() or s['id'] != stable('source', s['path']):
            raise ValueError('unsafe or missing source')
        if sha(s['text'].encode()) != s['sha256'] or (source_check and sha(path.read_bytes()) != s['sha256']):
            raise ValueError('source digest drift: ' + s['path'])
    models = {m['id']: m for m in bundle['models']}
    if len(models) != 11 or set(models) != MODEL_IDS:
        raise ValueError('eleven model identities required')
    for m in models.values():
        for sid in m['source_ids']:
            if sid not in sources:
                raise ValueError('dangling model source')
        ids = element_ids(m['data'])
        if len(ids) != len(set(ids)):
            raise ValueError('duplicate data element IDs: ' + m['id'])
        if any(id_ not in m['bindings'] for id_ in ids):
            raise ValueError('unbound diagram element: ' + m['id'])
        for b in m['bindings'].values():
            if any(s not in sources for s in b['source_ids']):
                raise ValueError('dangling element source')
        data = m['data']
        if data['id'] != m['id']:
            raise ValueError('data identity mismatch')
        links = data.get('edges', data.get('connections', data.get('relationships', [])))
        node_ids = {n['id'] for n in data.get('nodes', data.get('classes', []))}
        for section in ['layers', 'columns', 'stages']:
            for group in data.get(section, []):
                node_ids.update(n['id'] for n in group.get('nodes', []))
        if len(node_ids) > 500 or len(links) > 2000:
            raise ValueError('graph size limit exceeded')
        for edge in links:
            if edge['source'] not in node_ids or edge['target'] not in node_ids:
                raise ValueError('dangling endpoint: ' + edge['id'])
        if m['diagram_type'] == 'swimlane':
            lanes = {x['id'] for x in data['lanes']}
            if any(n['lane'] not in lanes for n in data['nodes']):
                raise ValueError('invalid swimlane')
        if m['diagram_type'] == 'sequence':
            participants = {x['id'] for x in data['participants']}
            if any(x['from'] not in participants or x['to'] not in participants for x in data['messages'] + data['notes']):
                raise ValueError('invalid sequence participant')
            if any(a['participant'] not in participants or not 0 <= a['start'] <= a['end'] < len(data['messages']) for a in data['activations']):
                raise ValueError('invalid activation')
    knowledge = models['knowledge']['data']
    expected_nodes = {e['id'] for e in publisher.entities}
    expected_edges = {stable('relation', r['from'], r['to'], r['type'], r.get('contract', '')) for r in publisher.relations}
    if {n['id'] for n in knowledge['nodes']} != expected_nodes or {e['id'] for e in knowledge['edges']} != expected_edges:
        raise ValueError('knowledge registry coverage mismatch')
    expected_semantics = {r['id']: (r, predicate, note) for r, predicate, note in publisher.semantic_relationships()}
    actual_semantics = {r['id']: r for r in models['contracts']['data']['relationships'] if r['id'].startswith('semantic_binding.')}
    if set(expected_semantics) != set(actual_semantics):
        raise ValueError('contract semantic binding coverage mismatch')
    required_semantic_sources = {stable('source', path) for path in [AGENT_SCHEMA, COMMON_SCHEMA, AGENT_DOC]}
    for id_, (expected, predicate, note) in expected_semantics.items():
        binding = models['contracts']['bindings'].get(id_, {})
        if actual_semantics[id_] != expected or not required_semantic_sources <= set(binding.get('source_ids', [])) or 'predicate=' + predicate not in binding.get('details', []):
            raise ValueError('contract semantic binding source/predicate mismatch')
    state = models['state']['data']
    expected_transitions = set()
    for current, events, guard in table((ROOT / INVARIANTS).read_text(), '状态与事件基线'):
        for event in events.split('；'):
            if '→' in event:
                label, target = event.split('→', 1)
                target = 'saved_nonterminal' if target == '已保存非终止状态' else 'new_run' if target == '关联新 Run' else target
                expected_transitions.add((current, target, label))
    if {(e['source'], e['target'], e['label']) for e in state['edges']} != expected_transitions:
        raise ValueError('state transitions differ from frozen table')
    if any(e['source'] in {'DONE', 'FAILED', 'CANCELLED'} and e['target'] != 'new_run' for e in state['edges']):
        raise ValueError('terminal run cannot resume execution')
    for e in state['edges']:
        if e['target'] == 'saved_nonterminal' and (e['source'] not in {'PAUSED', 'BLOCKED'} or '保存' not in e.get('description', '') or '核实' not in e.get('description', '')):
            raise ValueError('unsafe recovery')
    coverage = models['coverage']['data']
    if {r['id'] for r in coverage['rows']} != {f'G{i:02}' for i in range(1, 22)} or {c['id'] for c in coverage['cases']} != {f'C{i:02}' for i in range(1, 19)}:
        raise ValueError('gap/case coverage incomplete')
    rows, columns = {r['id'] for r in coverage['rows']}, {c['id'] for c in coverage['columns']}
    case_ids = {c['id'] for c in coverage['cases']}
    evidence_ids = {e['id'] for e in coverage['evidence']}
    if len(coverage['cells']) != len(rows) * len(columns):
        raise ValueError('matrix cell coverage incomplete')
    for c in coverage['cells']:
        if c['row'] not in rows or c['column'] not in columns or not set(c['case_ids']) <= case_ids or not set(c['evidence_ids']) <= evidence_ids:
            raise ValueError('invalid matrix ref')
        if c['column'] in {'contract', 'path', 'browser', 'live'} and c['status'] not in {'planned', 'not_run'}:
            raise ValueError('target evidence falsely promoted')
    if any(e['status'] != 'planned' for e in models['versions']['data']['events']) or models['versions']['scope'] != 'design':
        raise ValueError('design timeline cannot claim actual history')
    raw = json.dumps(bundle, ensure_ascii=False).encode()
    if len(raw) > 12000000:
        raise ValueError('bundle size limit exceeded')
    return {'meta_schema': True, 'publication_schema': True, 'format_assertions': True, 'offline': True, 'frozen_baseline': True, 'source_digests': True, 'all_element_bindings': True, 'endpoints_and_ownership': True, 'eleven_models': True, 'knowledge_registry': {'entities': len(expected_nodes), 'relations': len(expected_edges)}, 'contract_semantic_bindings': len(expected_semantics), 'state_events': len(expected_transitions), 'gaps': len(rows), 'cases': len(case_ids), 'coverage_dimensions': len(columns), 'design_timeline_only': True, 'bundle_bytes': len(raw)}


def self_tests(bundle, publisher):
    tests = []
    def reject(name, mutate):
        clone = copy.deepcopy(bundle)
        mutate(clone)
        try:
            check(clone, publisher)
        except Exception as exc:
            tests.append({'name': name, 'passed': True, 'rejected': str(exc)[:260]})
        else:
            raise ValueError('self-test accepted: ' + name)
    reject('dangling source', lambda b: b['models'][0]['source_ids'].append('source.unknown'))
    reject('unknown endpoint', lambda b: b['models'][0]['data']['edges'][0].update(target='UNKNOWN'))
    reject('wrong review schema version', lambda b: b.update(schema_version=2))
    reject('changed frozen baseline', lambda b: b['baseline'].update(invariants_sha256='0' * 64))
    reject('false target pass', lambda b: next(m for m in b['models'] if m['id'] == 'coverage')['data']['cells'][1].update(status='contract_verified'))
    reject('terminal resume', lambda b: next(m for m in b['models'] if m['id'] == 'state')['data']['edges'][-1].update(target='IMPLEMENT'))
    reject('design timeline called historical', lambda b: next(m for m in b['models'] if m['id'] == 'versions')['data']['events'][0].update(status='complete'))
    reject('source text changed', lambda b: b['sources'][0].update(text='modified'))
    reject('missing element binding', lambda b: b['models'][0]['bindings'].pop('INTAKE'))
    reject('missing gap', lambda b: next(m for m in b['models'] if m['id'] == 'coverage')['data']['rows'].pop())
    reject('semantic predicate source missing', lambda b: next(m for m in b['models'] if m['id'] == 'contracts')['bindings'][next(r['id'] for r in next(m for m in b['models'] if m['id'] == 'contracts')['data']['relationships'] if r['id'].startswith('semantic_binding.'))].update(source_ids=[stable('source', MODELS)]))
    reject('invented semantic field predicate', lambda b: next(r for r in next(m for m in b['models'] if m['id'] == 'contracts')['data']['relationships'] if r['id'].startswith('semantic_binding.')).update(label='字段绑定：approval_hash'))

    return tests


def publication_digests(bundle_raw, schema_raw):
    return {'bundle_sha256': sha(bundle_raw), 'schema_sha256': sha(schema_raw)}


def check_publication(manifest, bundle_raw, schema_raw):
    expected = publication_digests(bundle_raw, schema_raw)
    if manifest != expected:
        raise ValueError('build-fixed publication digest mismatch; regenerate and rebuild frontend')
    return expected


def publication_self_tests(bundle_raw, schema_raw):
    expected = publication_digests(bundle_raw, schema_raw)
    tests = []
    for name, key in [('modified published bundle digest', 'bundle_sha256'), ('modified published Schema digest', 'schema_sha256')]:
        manifest = dict(expected)
        manifest[key] = '0' * 64
        try:
            check_publication(manifest, bundle_raw, schema_raw)
        except ValueError as exc:
            tests.append({'name': name, 'passed': True, 'rejected': str(exc)})
        else:
            raise ValueError('publication self-test accepted: ' + name)
    return tests


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--check', action='store_true', help='validate published data without rewriting')
    parser.add_argument('--self-test', action='store_true', help='inject invalid objects into temporary clones')
    args = parser.parse_args()
    try:
        p = Publisher()
        fresh = p.build()
        bundle = load_json(OUTPUT) if args.check else fresh
        checks = check(bundle, p)
        if args.check:
            if (OUTPUT.parent / 'schema.json').read_bytes() != SCHEMA.read_bytes():
                raise ValueError('published review Schema digest mismatch')
            a, b = copy.deepcopy(bundle), copy.deepcopy(fresh)
            a.pop('generated_at'); b.pop('generated_at')
            if a != b:
                raise ValueError('published bundle differs from current normative projection; regenerate')
        tests = self_tests(bundle, p) if args.self_test else []
        if not args.check:
            OUTPUT.parent.mkdir(parents=True, exist_ok=True)
            OUTPUT.write_text(json.dumps(bundle, ensure_ascii=False, indent=2) + '\n')
            (OUTPUT.parent / 'schema.json').write_bytes(SCHEMA.read_bytes())
            PUBLICATION.parent.mkdir(parents=True, exist_ok=True)
            PUBLICATION.write_text(json.dumps(publication_digests(OUTPUT.read_bytes(), (OUTPUT.parent / 'schema.json').read_bytes()), indent=2) + '\n')
        publication = check_publication(load_json(PUBLICATION), OUTPUT.read_bytes(), (OUTPUT.parent / 'schema.json').read_bytes())
        checks['build_fixed_publication_digests'] = True
        publication_tests = publication_self_tests(OUTPUT.read_bytes(), (OUTPUT.parent / 'schema.json').read_bytes()) if args.self_test else []
        report = {'recorded_at': datetime.now(timezone.utc).isoformat(), 'passed': True, 'scope': 'readonly design review publication; not Agent behavior or v2 conformance', 'validator': {'package': 'jsonschema[format]', 'version': version('jsonschema'), 'draft': '2020-12', 'date_time_assertion': True}, 'command': 'build-review-data.py' + (' --check' if args.check else '') + (' --self-test' if args.self_test else ''), 'bundle_sha256': sha(OUTPUT.read_bytes()), 'schema_sha256': sha(SCHEMA.read_bytes()), 'published_schema_sha256': sha((OUTPUT.parent / 'schema.json').read_bytes()), 'publication': {'path': str(PUBLICATION.relative_to(ROOT)), **publication}, 'publication_manifest_sha256': sha(PUBLICATION.read_bytes()), 'publication_self_tests': publication_tests, 'baseline': bundle['baseline'], 'checks': checks, 'self_tests': tests, 'sources': len(bundle['sources']), 'models': [{'id': m['id'], 'diagram_type': m['diagram_type'], 'scope': m['scope'], 'status': m['status'], 'elements': len(element_ids(m['data'])), 'bindings': len(m['bindings'])} for m in bundle['models']], 'not_proven': ['Vue browser functionality', 'export SVG/PNG fidelity', 'S1-S5 runtime v2 conformance', 'real model/GitLab Runner or production']}
        if not args.check:
            REPORT.write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n')
        print(json.dumps({'passed': True, 'bundle': str(OUTPUT.relative_to(ROOT)), 'report': str(REPORT.relative_to(ROOT)), 'models': len(bundle['models']), 'sources': len(bundle['sources']), 'checks': checks, 'self_tests_passed': len(tests), 'publication_self_tests_passed': len(publication_tests)}, ensure_ascii=False, indent=2))
        return 0
    except Exception as exc:
        print(json.dumps({'passed': False, 'error': str(exc)}, ensure_ascii=False, indent=2), file=sys.stderr)
        return 1


if __name__ == '__main__':
    raise SystemExit(main())
