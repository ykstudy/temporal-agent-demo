import Ajv2020 from 'ajv/dist/2020'
import addFormats from 'ajv-formats'
import common from './common.schema.json'
import agent from './agent.schema.json'
import message from './message.schema.json'
import runtime from './runtime.schema.json'
import type { RunSnapshot, SnapshotEvent, ObjectReference, CommandReceipt } from './types.generated'
const ajv = new Ajv2020({ strict: false, allErrors: true, validateFormats: true, coerceTypes: false, useDefaults: false, removeAdditional: false })
addFormats(ajv)
for (const schema of [common, agent, message, runtime]) ajv.addSchema(schema)
const documents = [agent, message, common, runtime]
const validators = new Map<string, ReturnType<typeof ajv.compile>>()

// Scan original bytes before JSON.parse can discard duplicate keys or round
// unsafe integers. Complete schema validation follows this lexical boundary.
export function strictJSON(text: string): unknown {
  if (new TextEncoder().encode(text).length > 16 * 1024 * 1024) throw Error('响应超过字节上限')
  let pos = 0, nodes = 0
  const white = () => { while (pos < text.length && ' \n\r\t'.includes(text[pos])) pos++ }
  const string = (): string => {
    const start = pos++
    while (pos < text.length) {
      const c = text[pos++]
      if (c === '\\') { pos++; continue }
      if (c === '"') {
        const value: unknown = JSON.parse(text.slice(start, pos))
        if (typeof value !== 'string') throw Error('无效 JSON 字符串')
        for (let i = 0; i < value.length; i++) {
          const code = value.charCodeAt(i)
          if (code >= 0xd800 && code <= 0xdbff) { const next = value.charCodeAt(++i); if (!(next >= 0xdc00 && next <= 0xdfff)) throw Error('无效 Unicode') }
          else if (code >= 0xdc00 && code <= 0xdfff) throw Error('无效 Unicode')
        }
        return value
      }
    }
    throw Error('JSON 字符串不完整')
  }
  const value = (depth: number): void => {
    white(); if (depth > 128 || ++nodes > 500000) throw Error('响应结构超过上限')
    const c = text[pos]
    if (c === '"') { string(); return }
    if (c === '{' || c === '[') {
      const object = c === '{', end = object ? '}' : ']'; pos++; white(); const keys = new Set<string>()
      if (text[pos] === end) { pos++; return }
      for (;;) {
        white()
        if (object) { if (text[pos] !== '"') throw Error('无效 JSON 属性'); const key = string(); if (keys.has(key)) throw Error('响应存在重复属性'); keys.add(key); white(); if (text[pos++] !== ':') throw Error('无效 JSON 属性分隔') }
        value(depth + 1); white(); const token = text[pos++]; if (token === end) return; if (token !== ',') throw Error('无效 JSON 分隔')
      }
    }
    const match = /^(?:true|false|null|-?(?:0|[1-9]\d*)(?:\.\d+)?(?:[eE][+-]?\d+)?)/.exec(text.slice(pos))
    if (!match) throw Error('无效 JSON 值')
    const raw = match[0]; pos += raw.length
    if (/^-?\d/.test(raw)) { const n = Number(raw); const [mantissa, exponent = '0'] = raw.toLowerCase().split('e'); const digits = mantissa.replace(/[-.]/g, ''); const scale = (mantissa.split('.')[1]?.length || 0) - Number(exponent); const exactInteger = scale <= 0 || /^0*$/.test(digits.slice(-scale)); if (Number.isInteger(n) && !exactInteger) throw Error('JSON 数值精度丢失'); const exp = /e([+-]?\d+)/i.exec(raw); if (raw.length > 128 || exp && Math.abs(Number(exp[1])) > 400 || !Number.isFinite(n) || Number.isInteger(n) && !Number.isSafeInteger(n)) throw Error('无效或不安全的 JSON 数值') }
  }
  value(0); white(); if (pos !== text.length) throw Error('响应包含多余 JSON 数据')
  return JSON.parse(text)
}
export function validate<T>(name: string, value: unknown): T {
  let validator = validators.get(name)
  if (!validator) {
    const source = documents.find(d => name in d.$defs)
    if (!source) throw Error('未知发布对象类型')
    validator = ajv.compile({ $ref: source.$id + '#/$defs/' + name }); validators.set(name, validator)
  }
  if (!validator(value)) throw Error('服务响应不符合 ' + name + ' 契约；已保留此前有效状态')
  return value as T // The entire offline Draft 2020-12 contract has passed.
}
export function canonical(value: unknown): string {
  if (value === null || typeof value !== 'object') return JSON.stringify(value)
  if (Array.isArray(value)) return '[' + value.map(canonical).join(',') + ']'
  return '{' + Object.keys(value).sort().map(key => JSON.stringify(key) + ':' + canonical(Reflect.get(value, key))).join(',') + '}'
}
export async function sha256(value: Uint8Array | string): Promise<string> {
  const bytes = typeof value === 'string' ? new TextEncoder().encode(value) : value
  const digest = await crypto.subtle.digest('SHA-256', new Uint8Array(bytes))
  return Array.from(new Uint8Array(digest), b => b.toString(16).padStart(2, '0')).join('')
}
export async function checkHash(value: Record<string, unknown>, field: string, excludes: string[] = [field]) {
  const body = Object.fromEntries(Object.entries(value).filter(([key]) => !excludes.includes(key)))
  if (value[field] !== await sha256(canonical(body))) throw Error('对象内容摘要不一致；已拒绝更新')
}
export const runRef = (r: RunSnapshot): ObjectReference & { object_type: 'run' } => ({ object_type: 'run', object_id: r.run_id, object_version: r.state_sequence, source_version: r.source_version, hash: r.snapshot_hash })
export async function checkRun(input: unknown): Promise<RunSnapshot> {
  const r = validate<RunSnapshot>('RunSnapshot', input)
  await checkHash(r, 'snapshot_hash'); await checkHash(r.definition_snapshot, 'snapshot_hash')
  if (r.contract_version !== r.definition_snapshot.contract_version) throw Error('冻结契约版本不一致')
  if (r.plan) { await checkHash(r.plan, 'plan_hash'); if (r.plan_hash !== r.plan.plan_hash) throw Error('计划摘要不一致') }
  const expected = r.definition_snapshot.definition.completion_policy.required_checks.filter(c => c.delivery_modes.includes(r.modes.delivery_mode)).map(c => c.check_id).sort()
  if (canonical(expected) !== canonical(r.required_checks.map(c => c.check_id).sort())) throw Error('必需检查集合不完整')
  for (const check of r.required_checks) { if (check.run_id !== r.run_id || check.source_version !== r.source_version || check.definition_snapshot_hash !== r.definition_snapshot.snapshot_hash) throw Error('检查来源不一致') }
  if (r.manifest) { await checkHash(r.manifest, 'hash'); if (r.manifest.run_id !== r.run_id || r.manifest.source_version !== r.source_version || r.manifest.proposal_snapshot_hash !== r.proposal?.snapshot_hash || canonical(r.artifacts) !== canonical(r.manifest.files)) throw Error('清单来源不一致') }
  if (r.verification_report && canonical(r.verification_report.checks) !== canonical(r.required_checks)) throw Error('验证报告与显示检查不同')
  if (r.state === 'DONE' && (!r.approval || !r.manifest || r.verification_report?.outcome !== 'passed' || r.required_checks.some(c => c.status !== 'passed'))) throw Error('完成状态缺少完整交付证据')
  if (r.messages.some((m, i) => m.run_id !== r.run_id || m.sequence !== i + 1 || m.source_version > r.source_version || m.state_sequence > r.state_sequence || canonical(m.modes) !== canonical(r.modes))) throw Error('消息来源或顺序不一致')
  const componentVersions = new Map<string,string>()
  const knownComponents = new Set(['clarification','proposal_diff','progress','artifact_manifest','verification_report'])
  const depth = (v: unknown): number => v && typeof v === 'object' ? 1 + Math.max(0, ...Object.values(v).map(depth)) : 0
  for (const envelope of r.messages) {
    if (new TextEncoder().encode(canonical(envelope)).length > 256 * 1024) throw Error('单条原始消息超过字节上限')
    const partIds = new Set<string>()
    for (const part of envelope.parts) {
      if (partIds.has(part.part_id)) throw Error('消息 Part 身份重复'); partIds.add(part.part_id)
      if (part.type === 'component') {
        const key = String(part.component_id) + '/' + String(part.component_version), body = canonical(part)
        if (componentVersions.has(key) && componentVersions.get(key) !== body) throw Error('同一组件版本内容被替换')
        componentVersions.set(key, body)
        if (!knownComponents.has(String(part.component_type)) && (new TextEncoder().encode(canonical(part.payload)).length > 64 * 1024 || depth(part.payload) > 8)) throw Error('未知组件超过只读回退边界')
      }
    }
  }
  let previous: ObjectReference | null = null
  for (let index = 0; index < r.summary_records.length; index++) {
    const summary = r.summary_records[index]
    await checkHash(summary, 'summary_hash')
    if (summary.run_id !== r.run_id || summary.definition_snapshot_hash !== r.definition_snapshot.snapshot_hash || summary.summary_version !== index + 1 || summary.generator.mode !== r.modes.decision_mode || canonical(summary.previous_summary_ref) !== canonical(previous)) throw Error('派生摘要的身份或版本链错误')
    const source = r.messages.slice(summary.source.first_sequence - 1, summary.source.last_sequence)
    if (source.length !== summary.source.last_sequence - summary.source.first_sequence + 1 || canonical(source.map(m => m.message_id)) !== canonical(summary.source.message_ids) || summary.source.raw_jsonl_sha256 !== await sha256(source.map(m => canonical(m) + '\n').join('')) || summary.content_sha256 !== await sha256(summary.content)) throw Error('摘要与完整原文来源不同')
    previous = { object_type:'summary', object_id:summary.summary_id, object_version:summary.summary_version, source_version:summary.source_version, hash:summary.summary_hash }
  }
  return r
}
export async function checkEvent(input: unknown, id: string): Promise<SnapshotEvent> {
  const event = validate<SnapshotEvent>('SnapshotEvent', input)
  const expected = 'v2_' + (await sha256(id)).slice(0, 20) + '_' + event.state_sequence
  if (event.run_id !== id || event.event_id !== expected || event.run_ref.object_id !== id || event.run_ref.object_version !== event.state_sequence || event.run_ref.source_version !== event.source_version) throw Error('状态事件所属 Run 或版本错误')
  return event
}
export async function checkReceipt(input: unknown, id: string, body?: unknown): Promise<CommandReceipt> {
  const receipt = validate<CommandReceipt>('CommandReceipt', input)
  if (receipt.run_id !== id || receipt.run_ref && receipt.run_ref.object_id !== id) throw Error('命令回执所属任务错误')
  if (body !== undefined && receipt.command_fingerprint !== await sha256(canonical(body))) throw Error('命令回执与原请求不符')
  return receipt
}
