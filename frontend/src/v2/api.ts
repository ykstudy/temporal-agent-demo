import type { AgentDefinition, RunSnapshot, ObjectReference, Proposal, CommandReceipt, ErrorResponse, ControlRequest, MessageRequest, CreateRunRequest, BudgetLedger } from '../contracts/types.generated'
import { checkRun, strictJSON, validate, checkHash, checkReceipt, runRef } from '../contracts/runtime'

export class V2Error extends Error {
  constructor(message: string, public status: number, public code: string, public legacyURL?: string) { super(message) }
}
export async function json(path: string, init?: RequestInit): Promise<unknown> {
  const response = await fetch(path, { ...init, headers: { 'Content-Type': 'application/json', ...init?.headers } })
  const data = strictJSON(await response.text())
  if (!response.ok) {
    const error = validate<ErrorResponse>('ErrorResponse', data)
    throw new V2Error(error.error, response.status, error.code, error.legacy_read_url)
  }
  return data
}
export const snapshot = async (id: string): Promise<RunSnapshot> => {
  const r = await checkRun(await json('/api/v2/runs/' + encodeURIComponent(id)))
  if (r.run_id !== id) throw Error('读取返回了其他 Run')
  return r
}
export const objectURL = (id: string, ref: ObjectReference) => '/api/v2/runs/' + encodeURIComponent(id) + '/objects/' + encodeURIComponent(ref.object_type) + '/' + encodeURIComponent(ref.object_id) + '?' + new URLSearchParams({ object_version: String(ref.object_version), source_version: String(ref.source_version), hash: ref.hash })
export async function proposal(id: string, ref: ObjectReference): Promise<Proposal> {
  const p = validate<Proposal>('Proposal', await json(objectURL(id, ref)))
  await checkHash(p, 'snapshot_hash', ['snapshot_hash', 'status'])
  if (p.run_id !== id || p.proposal_id !== ref.object_id || p.proposal_version !== ref.object_version || p.source_version !== ref.source_version || p.snapshot_hash !== ref.hash) throw Error('历史方案与卡片引用不同')
  return p
}
export async function historicalRun(id: string, ref: ObjectReference) {
  const r = await checkRun(await json(objectURL(id, ref)))
  if (JSON.stringify(runRef(r)) !== JSON.stringify(ref)) {
    if (r.run_id !== ref.object_id || r.state_sequence !== ref.object_version || r.source_version !== ref.source_version || r.snapshot_hash !== ref.hash) throw Error('历史快照引用不同')
  }
  return r
}
export async function definitions(): Promise<AgentDefinition[]> {
  const list = await json('/api/definitions')
  if (!Array.isArray(list)) throw Error('定义目录格式错误')
  return list.filter(d => d && typeof d === 'object' && Reflect.get(d, 'contract_version') === '1.4.0').map(d => validate<AgentDefinition>('AgentDefinition', d))
}
export async function budget(id: string, signal?: AbortSignal): Promise<BudgetLedger> {
  const b = validate<BudgetLedger>('BudgetLedger', await json('/api/runs/' + id + '/budget', { signal }))
  await checkHash(b, 'ledger_hash'); if (b.run_id !== id) throw Error('预算所属 Run 错误'); return b
}
export type WireRequest = CreateRunRequest | MessageRequest | ControlRequest
export async function submit(id: string, path: string, body: WireRequest): Promise<CommandReceipt> {
  const name = path === '/api/v2/runs' ? 'CreateRunRequest' : path.endsWith('/messages') ? 'MessageRequest' : 'ControlRequest'
  validate(name, body)
  return checkReceipt(await json(path, { method: 'POST', body: JSON.stringify(body) }), id, body)
}
export async function receipt(id: string, command: string, body: WireRequest) {
  const result = await checkReceipt(await json('/api/v2/runs/' + id + '/commands/' + command), id, body)
  if (result.command_id !== body.command_id) throw Error('原命令回执身份错误')
  return result
}
export function proposalRef(p: Proposal): ObjectReference & { object_type: 'proposal' } { return { object_type: 'proposal', object_id: p.proposal_id, object_version: p.proposal_version, source_version: p.source_version, hash: p.snapshot_hash } }
