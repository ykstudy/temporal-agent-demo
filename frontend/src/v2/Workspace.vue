<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, shallowRef, watch } from 'vue'
import { ArrowUp, Menu, PanelRight, Plus, RefreshCw, Sparkles } from 'lucide-vue-next'
import type { AgentDefinition, RunSnapshot, Modes, ControlRequest, ObjectReference, BudgetLedger, ArtifactEntry, S2AuditBundle, MessageEnvelope } from '../contracts/types.generated'
import { checkEvent, checkHash, checkRun, runRef, sha256, strictJSON, validate } from '../contracts/runtime'
import * as api from './api'
import { stateLabel } from '../lib/api'
import PartRenderer from './PartRenderer.vue'
import ProposalCard from './ProposalCard.vue'
import RuntimePanel from './RuntimePanel.vue'
import './workspace.css'

type IndexEntry = { id: string; title: string; state: string; legacy: boolean }
type Pending = { id: string; path: string; body: api.WireRequest; status: string }
const definitions = shallowRef<AgentDefinition[]>([]), runs = shallowRef<RunSnapshot[]>([]), legacyEntries = shallowRef<IndexEntry[]>([])
const selected = ref(new URLSearchParams(location.search).get('run') || ''), run = shallowRef<RunSnapshot | null>(null), legacy = shallowRef<unknown>(null)
const input = ref(''), busy = ref(false), error = ref(''), connected = ref(false), loading = ref(false), sidebar = ref(false), inspector = ref(false)
const modes = ref<Modes>({ execution_mode: 'local', decision_mode: 'rules', delivery_mode: 'local' }), definitionHash = ref('')
const serverMode = ref(''), modelConfigured = ref(false), gitlabConfigured = ref(false)
const budget = shallowRef<BudgetLedger | null>(null), pending = shallowRef<Pending | null>(null)
const budgetError = ref('')
const conversation = ref<HTMLElement | null>(null), followLatest = ref(true)
const notice = ref(''), modal = ref<HTMLDialogElement | null>(null), modalTitle = ref(''), modalText = ref(''), composer = ref<HTMLTextAreaElement | null>(null)
let alive = true, modalRequest = 0
let conversationObserver: MutationObserver | null = null
const entries = computed<IndexEntry[]>(() => [...runs.value.map(r => ({ id: r.run_id, title: r.title, state: r.state, legacy: false })), ...legacyEntries.value.filter(e => !runs.value.some(r => r.run_id === e.id))])
const terminal = computed(() => !!run.value && ['DONE', 'FAILED', 'CANCELLED'].includes(run.value.state))
const activePending = computed(() => pending.value && (pending.value.id === selected.value || !selected.value) ? pending.value : null)
const canSend = computed(() => !busy.value && !loading.value && !legacy.value && !terminal.value && !!input.value.trim() && (!!run.value || definitions.value.length > 0) && !activePending.value)
const stateNames: Record<string, string> = { not_ready: '尚未就绪', running: '执行中', passed: '已通过', failed: '未通过', not_verified: '尚未验证' }
const choose = (id: string) => { selected.value = id; sidebar.value = false; inspector.value = false; error.value = ''; notice.value = ''; const url = new URL(location.href); if (id) url.searchParams.set('run', id); else url.searchParams.delete('run'); history.replaceState(null, '', url) }
const message = (e: unknown) => e instanceof Error ? e.message : '请求失败'
function savePending(value: Pending | null) { pending.value = value; try { if (value) sessionStorage.setItem('agentstudio.v2.pending', JSON.stringify(value)); else sessionStorage.removeItem('agentstudio.v2.pending') } catch { /* Request identity remains available in memory. */ } }
function apply(next: RunSnapshot) {
  if (!alive) return
  const prior = runs.value.find(r => r.run_id === next.run_id)
  if (prior && next.state_sequence < prior.state_sequence) return
  if (prior && next.state_sequence === prior.state_sequence && next.snapshot_hash !== prior.snapshot_hash) throw Error('同一快照版本出现不同内容')
  if (prior && next.source_version < prior.source_version) throw Error('业务来源版本倒退')
  runs.value = [next, ...runs.value.filter(r => r.run_id !== next.run_id)].sort((a, b) => b.updated_at.localeCompare(a.updated_at))
  if (selected.value === next.run_id && (!run.value || next.state_sequence >= run.value.state_sequence)) run.value = next
}
async function refresh() {
  try {
    const [values, defs, old, config] = await Promise.all([api.json('/api/v2/runs'), api.definitions(), api.json('/api/runs'), api.json('/api/bootstrap')])
    if (!Array.isArray(values) || !Array.isArray(old) || !config || typeof config !== 'object') throw Error('服务目录格式错误')
    const snapshots = await Promise.all(values.map(checkRun)); if (!alive) return
    definitions.value = defs; if (!definitionHash.value) definitionHash.value = defs[0]?.definition_hash || ''
    snapshots.forEach(apply)
    legacyEntries.value = old.flatMap((r: unknown) => r && typeof r === 'object' && typeof Reflect.get(r, 'id') === 'string' && typeof Reflect.get(r, 'title') === 'string' ? [{ id: Reflect.get(r, 'id'), title: Reflect.get(r, 'title'), state: typeof Reflect.get(r, 'state') === 'string' ? Reflect.get(r, 'state') : '未知', legacy: true }] : [])
    const execution = Reflect.get(config, 'execution_mode'); if (!['local', 'temporal'].includes(execution)) throw Error('执行模式配置错误')
    if (!serverMode.value) { const decision = Reflect.get(config, 'decision_mode'), delivery = Reflect.get(config, 'delivery_mode'); if (decision === 'model' || decision === 'rules') modes.value.decision_mode = decision; if (delivery === 'local' || delivery === 'gitlab') modes.value.delivery_mode = delivery }
    serverMode.value = execution; modes.value.execution_mode = execution
    modelConfigured.value = Reflect.get(config, 'model_configured') === true; gitlabConfigured.value = Reflect.get(config, 'gitlab_configured') === true
  } catch (e) { if (alive) error.value = message(e) }
}
onMounted(() => {
  conversationObserver = new MutationObserver(() => { if (followLatest.value && conversation.value) conversation.value.scrollTop = conversation.value.scrollHeight }); if (conversation.value) conversationObserver.observe(conversation.value, { childList:true, subtree:true })
  void refresh()
  try { const stored = sessionStorage.getItem('agentstudio.v2.pending'); if (stored) { const v = strictJSON(stored); if (v && typeof v === 'object' && typeof Reflect.get(v, 'id') === 'string' && typeof Reflect.get(v, 'path') === 'string') { const p = v as Pending; validate(p.path === '/api/v2/runs' ? 'CreateRunRequest' : p.path.endsWith('/messages') ? 'MessageRequest' : 'ControlRequest', p.body); if (p.path === '/api/v2/runs' || p.path === '/api/v2/runs/' + p.id + '/messages' || p.path === '/api/v2/runs/' + p.id + '/actions') pending.value = p } } } catch { sessionStorage.removeItem('agentstudio.v2.pending') }
})
onBeforeUnmount(() => { alive = false; conversationObserver?.disconnect() })
watch(selected, (id, _old, cleanup) => {
  followLatest.value = true; modalRequest++; modal.value?.close(); run.value = null; legacy.value = null; budget.value = null; budgetError.value = ''; connected.value = false
  if (!id) { loading.value = false; void nextTick(() => composer.value?.focus()); return }
  let active = true; let stream: EventSource | null = null
  const current = () => alive && active && selected.value === id
  loading.value = true
  const fetchRun = async () => {
    try { const next = await api.snapshot(id); if (current()) { apply(next); legacy.value = null; loading.value = false } }
    catch (e) {
      if (!current()) return
      if (e instanceof api.V2Error && e.code === 'legacy_contract_incomplete' && e.legacyURL === '/api/runs/' + id) {
        stream?.close(); try { const old = await api.json(e.legacyURL); if (current()) legacy.value = old } catch (err) { if (current()) error.value = message(err) }
      } else error.value = message(e)
      if (current()) loading.value = false
    }
  }
  void fetchRun()
  stream = new EventSource('/api/v2/runs/' + encodeURIComponent(id) + '/events')
  stream.onopen = () => { if (current()) { connected.value = true; void fetchRun() } }
  stream.onerror = () => { if (current()) connected.value = false }
  stream.addEventListener('snapshot', async event => {
    if (!current()) return
    try {
      const evt = await checkEvent(strictJSON((event as MessageEvent<string>).data), id)
      if (!current() || run.value && evt.state_sequence <= run.value.state_sequence) return
      if (!run.value || evt.state_sequence > run.value.state_sequence + 1) { await fetchRun(); return }
      const next = await api.historicalRun(id, evt.run_ref); if (current()) apply(next)
    } catch (e) { if (current()) error.value = message(e) }
  })
  const timer = window.setInterval(() => { if (current() && !legacy.value && !connected.value) void fetchRun() }, 5000)
  cleanup(() => { active = false; stream?.close(); window.clearInterval(timer) })
}, { immediate: true, flush: 'sync' })
watch(() => JSON.stringify([run.value?.run_id, run.value?.state_sequence]), (_value, _old, cleanup) => {
  const r = run.value; if (!r) return
  let active = true; const controller = new AbortController()
  // A burst of durable progress events must not queue a full ledger read for
  // every intermediate snapshot and delay the command's own receipt.
  const timer = window.setTimeout(async () => {
    try { const b = await api.budget(r.run_id, controller.signal); if (active && selected.value === r.run_id && b.definition_snapshot_hash === r.definition_snapshot.snapshot_hash) { budget.value = b; budgetError.value = '' } }
    catch (e) { if (active && selected.value === r.run_id) budgetError.value = '预算读取不可用：' + message(e) }
  }, 1500)
  cleanup(() => { active = false; window.clearTimeout(timer); controller.abort() })
})
async function finish(p: Pending, receipt: Awaited<ReturnType<typeof api.submit>>, selection: string) {
  if (receipt.status === 'processed') {
    savePending(null); if (selected.value === selection) notice.value = '命令已处理，结果已保存。'
    const nextRef = receipt.result_refs.find(r => r.object_type === 'run' && r.object_id !== p.id)
    if (nextRef) { const next = await api.historicalRun(nextRef.object_id, nextRef); if (next.retry_of !== p.id) throw Error('重做结果缺少原 Run 关系'); apply(next); if (selected.value === selection) choose(next.run_id) }
    else { const next = await api.snapshot(p.id); apply(next); if (p.path === '/api/v2/runs' && selected.value === selection) choose(p.id) }
  } else {
    savePending({ ...p, status: receipt.status })
    if (selected.value === selection) notice.value = receipt.status === 'rejected' ? receipt.error?.message || '原命令被拒绝' : '原命令已记录，正在等待可核验结果。'
  }
}
async function operate(p: Pending) {
  if (busy.value) return; busy.value = true; error.value = ''; const selection = selected.value; savePending(p)
  try { await finish(p, await api.submit(p.id, p.path, p.body), selection) }
  catch (e) { if (selected.value === selection) error.value = message(e); savePending({ ...p, status: e instanceof api.V2Error && [400, 409, 413, 404].includes(e.status) ? 'not_received' : 'transport_unknown' }) }
  finally { busy.value = false }
}
async function queryPending() {
  const p = pending.value; if (!p || busy.value) return; busy.value = true
  try { await finish(p, await api.receipt(p.id, p.body.command_id, p.body), selected.value) }
  catch (e) { if (e instanceof api.V2Error && e.status === 404) savePending({ ...p, status: 'not_received' }); error.value = message(e) }
  finally { busy.value = false }
}
async function send() {
  if (!canSend.value) return; followLatest.value = true; const content = input.value.trim(), original = selected.value; const r = run.value; let createdID = ''
  if (r) await operate({ id: r.run_id, path: '/api/v2/runs/' + r.run_id + '/messages', status: 'sending', body: { contract_version: '1.4.0', schema_version: 2, command_id: crypto.randomUUID(), target: runRef(r), content } })
  else {
    const d = definitions.value.find(d => d.definition_hash === definitionHash.value); if (!d) return
    const id = 'run_' + crypto.randomUUID().replaceAll('-', ''); createdID = id
    await operate({ id, path: '/api/v2/runs', status: 'sending', body: { contract_version: '1.4.0', schema_version: 2, command_id: crypto.randomUUID(), run_id: id, title: content.slice(0, 80), modes: { ...modes.value }, definition_selection: { definition_id: d.definition_id, definition_version: d.definition_version, definition_hash: d.definition_hash }, initial_content: content } })
  }
  if (!pending.value && (selected.value === original || selected.value === createdID) && input.value.trim() === content) input.value = ''
}
async function act(intent: ControlRequest) { if (!run.value || intent.target.object_id !== run.value.run_id) return; await operate({ id: run.value.run_id, path: '/api/v2/runs/' + run.value.run_id + '/actions', status: 'sending', body: intent }) }
function control(action: ControlRequest['action']) {
  const r = run.value; if (!r) return
  const intent: ControlRequest = { command_id: crypto.randomUUID(), action, target: runRef(r) }
  if (['accept', 'reject', 'revise'].includes(action) && r.proposal) intent.required_proposal_ref = api.proposalRef(r.proposal)
  void act(intent)
}
async function show(title: string, text: string) { modalTitle.value = title; modalText.value = text; await nextTick(); modal.value?.showModal() }
async function inspectObject(reference: ObjectReference, schema: string) {
  const r = run.value; if (!r) return; const request = ++modalRequest
  try { const value = validate<Record<string,unknown>>(schema, await api.json(api.objectURL(r.run_id, reference))); const hashField = schema === 'ArtifactManifest' ? 'hash' : 'report_hash'; await checkHash(value, hashField); if (value.run_id !== r.run_id || value.source_version !== reference.source_version || value[hashField] !== reference.hash) throw Error('对象引用不一致'); if (selected.value === r.run_id && request === modalRequest) await show(schema + ' · 来源 v' + reference.source_version, JSON.stringify(value, null, 2)) } catch (e) { if (selected.value === r.run_id && request === modalRequest) error.value = message(e) }
}
async function artifact(file?: ArtifactEntry) {
  const r = run.value; if (!r || !r.manifest) return; const request = ++modalRequest
  try {
    const response = await fetch('/api/v2/runs/' + r.run_id + (file ? '/artifacts?' + new URLSearchParams({ path: file.path }) : '/download'))
    if (!response.ok) throw Error('工件暂不可用，请查询最新状态')
    const data = new Uint8Array(await response.arrayBuffer()), hash = await sha256(data)
    if (response.headers.get('X-Manifest-Hash') !== r.manifest.hash || response.headers.get('X-Source-Version') !== String(r.source_version) || response.headers.get('X-Artifact-SHA256') !== hash || file && (file.size !== data.length || file.sha256 !== hash)) throw Error('下载内容与本来源清单不一致')
    if (file) { if (selected.value === r.run_id && request === modalRequest) await show(file.path + ' · ' + file.size + ' B', new TextDecoder('utf-8', { fatal: true }).decode(data)) }
    else {
      const audit = validate<S2AuditBundle>('S2AuditBundle', await api.json('/api/runs/' + r.run_id + '/decision-audit')); await checkHash(audit, 'audit_hash')
      if (!audit.package_receipts?.some(p => p.status === 'succeeded' && p.source_version === r.source_version && p.manifest_ref.hash === r.manifest?.hash && p.package_sha256 === hash && p.package_bytes === data.length)) throw Error('下载包缺少准确执行回执')
      const url = URL.createObjectURL(new Blob([data], { type: 'application/zip' })), link = document.createElement('a'); link.href = url; link.download = (r.plan?.project_name || r.run_id) + '.zip'; link.click(); window.setTimeout(() => URL.revokeObjectURL(url), 1000)
    }
  } catch (e) { if (selected.value === r.run_id && request === modalRequest) error.value = message(e) }
}
function visibleParts(message: MessageEnvelope) {
  return message.parts.filter(part => !(part.type === 'component' && Reflect.get(part, 'component_type') === 'progress' && part.payload && typeof part.payload === 'object' && message.parts.some(p => p.type === 'text' && p.text === Reflect.get(part.payload as object, 'summary'))))
}
const currentCardMissing = computed(() => run.value?.proposal && !run.value.messages.some(m => m.parts.some(p => {
  if (p.type !== 'component' || Reflect.get(p, 'component_type') !== 'proposal_diff' || !p.payload || typeof p.payload !== 'object') return false
  const ref = Reflect.get(p.payload, 'proposal_ref'); return ref && typeof ref === 'object' && Reflect.get(ref, 'hash') === run.value?.proposal?.snapshot_hash
})))
const checkLabels: Record<string,string> = { approved_sources:'批准文件一致性',artifact_manifest:'文件清单完整性',go_mod_tidy:'依赖整理',go_test:'项目测试',go_build:'项目构建',go_mod_file:'模块声明',gitignore_file:'忽略规则',ci_config_file:'CI 配置',readme_file:'项目说明',secret_boundary:'凭据检查',zip_integrity:'ZIP 完整性',remote_repository:'远程仓库',remote_commit:'远程提交',gitlab_ci:'GitLab CI' }

</script>

<template>
  <div class="workspace v2-workspace">
    <button v-if="sidebar || inspector" class="scrim" aria-label="关闭侧栏" @click="sidebar = false; inspector = false" />
    <aside class="sidebar" :class="{ open: sidebar }"><div class="v2-brand"><Sparkles :size="20" /><strong>项目创建 Agent</strong></div><button class="new-task" @click="choose('')"><Plus :size="16" />新建任务</button><div class="v2-list-heading">运行记录<button aria-label="刷新任务列表" @click="refresh"><RefreshCw :size="13" /></button></div><nav class="v2-run-list" aria-label="运行记录"><button v-for="entry in entries" :key="entry.id" :data-run-id="entry.id" :title="entry.id" :class="{ selected: entry.id === selected }" @click="choose(entry.id)"><strong>{{ entry.title }}</strong><small>{{ stateLabel(entry.state) }}{{ entry.legacy ? ' · legacy 只读' : '' }} · {{ entry.id.slice(-6) }}</small></button><p v-if="!entries.length" class="v2-muted">还没有任务。描述项目需求即可开始。</p></nav><a class="review-entry" href="/?review">查看设计规范</a></aside>
    <main class="v2-main" :data-run-id="run?.run_id || selected" :data-snapshot-hash="run?.snapshot_hash"><header class="topbar"><button class="mobile-menu" aria-label="打开任务列表" @click="sidebar = true"><Menu :size="18" /></button><div><h1>{{ run?.title || (legacy ? '历史任务 · 只读' : '创建一个新项目') }}</h1><p v-if="run">{{ run.modes.execution_mode }} / {{ run.modes.decision_mode }} / {{ run.modes.delivery_mode }} · 来源 v{{ run.source_version }} · 进度 {{ run.state_sequence }}</p><p v-else>需求 → 方案确认 → 生成与验证 → 交付</p></div><span v-if="run" class="connection" role="status">{{ connected ? '状态已连接' : '正在重新连接' }}</span><button v-if="run" class="panel-toggle" aria-label="打开运行详情" @click="inspector = true"><PanelRight :size="18" /></button></header>
      <div class="v2-notifications"><p v-if="error" class="run-error" role="alert">{{ error }}</p><p v-if="notice" class="v2-notice" role="status">{{ notice }}</p><section v-if="activePending" class="v2-pending" aria-label="原命令结果"><p>原命令：{{ activePending.body.command_id }} · {{ activePending.status }}</p><button :disabled="busy" @click="queryPending">查询原命令结果</button><button v-if="['not_received', 'rejected'].includes(activePending.status)" :disabled="busy" @click="operate(activePending)">重试原命令</button><button v-if="['not_received', 'rejected'].includes(activePending.status)" :disabled="busy" @click="savePending(null)">保留历史，关闭提示</button><p v-if="activePending.status === 'result_unknown'">结果未知，先核实原身份。请勿重复提交外部写入。</p></section></div>
      <section ref="conversation" class="conversation" @scroll="followLatest = !!conversation && conversation.scrollHeight - conversation.scrollTop - conversation.clientHeight < 180" aria-label="任务对话" :aria-busy="loading"><p v-if="loading" role="status">正在核验运行快照…</p>
        <section v-else-if="legacy" class="v2-legacy"><h2>legacy 历史记录 · 只读</h2><p>此任务没有完整 v2 来源，历史记录保持原样。可新建任务重新审查与确认。</p><pre>{{ JSON.stringify(legacy, null, 2) }}</pre></section>
        <template v-else-if="run"><div class="v2-stage" role="status"><strong>{{ stateLabel(run.state) }}</strong><span v-if="run.pause_requested">暂停请求已记录，等待安全边界</span><span v-if="run.cancel_requested && run.state !== 'CANCELLED'">取消请求已记录，等待在途结果</span></div><div v-if="run.error" class="run-error"><strong>{{ run.error.category }} · {{ run.error.code }}</strong><p>{{ run.error.message }}</p><small v-if="run.error.operation_id">原操作 {{ run.error.operation_id }}</small></div><article v-for="entry in run.messages" :key="entry.message_id" class="message" :data-message-id="entry.message_id"><div class="avatar">{{ entry.role === 'user' ? '我' : 'A' }}</div><div class="message-body"><div class="v2-message-meta">{{ entry.role === 'user' ? '你' : '项目创建 Agent' }} · 来源 v{{ entry.source_version }} · {{ new Date(entry.created_at).toLocaleTimeString() }}</div><PartRenderer v-for="part in visibleParts(entry)" :key="part.part_id" :part="part" :run="run" :busy="busy || !!activePending" @action="act" @choose="input = $event; composer?.focus()" @inspect="inspectObject" /></div></article><ProposalCard v-if="currentCardMissing && run.proposal" :reference="api.proposalRef(run.proposal)" :run="run" :busy="busy || !!activePending" @action="act" /><div class="run-actions"><button v-if="!terminal && !['PAUSED','BLOCKED','PLAN','INTAKE','CLARIFY'].includes(run.state)" :disabled="busy || !!activePending" @click="control('pause')">暂停</button><button v-if="['PAUSED','BLOCKED'].includes(run.state)" :disabled="busy || !!activePending" @click="control('resume')">恢复</button><button v-if="run.operations.some(o => ['unknown','sent'].includes(o.status))" :disabled="busy || !!activePending" @click="control('reconcile')">核实原操作</button><button v-if="run.state === 'VERIFY' || run.resume_to === 'VERIFY'" :disabled="busy || !!activePending" @click="control('revise')">修订验证方案</button><button v-if="!terminal" :disabled="busy || !!activePending" @click="control('cancel')">取消任务</button><button v-if="terminal" :disabled="busy || !!activePending" @click="control('retry')">重做为新任务</button></div></template>
        <section v-else class="empty"><div class="empty-icon"><Sparkles :size="23" /></div><h2>从需求到可运行的项目</h2><p>告诉我项目名称和所需能力。我会先给出文件预览，经过你的确认后再生成和验证。</p><button class="example" @click="input = '创建 order-service，SQLite + GORM、JWT 认证和 JSON 日志'; composer?.focus()">创建 order-service，包含数据库、认证和结构化日志 →</button><div class="v2-mode-form"><label>执行方式<select v-model="modes.execution_mode" aria-label="执行方式" disabled><option value="local">local · 当前服务本地执行</option><option value="temporal">temporal · 持久工作流</option></select></label><label>决策方式<select v-model="modes.decision_mode" aria-label="决策方式"><option value="rules">rules · 明确规则</option><option value="model">model · 冻结模型</option></select></label><label>交付方式<select v-model="modes.delivery_mode" aria-label="交付方式"><option value="local">local · 文件包</option><option value="gitlab">gitlab · 仓库与 CI</option></select></label><label>Agent 定义<select v-model="definitionHash" aria-label="Agent 定义"><option v-for="d in definitions" :key="d.definition_hash" :value="d.definition_hash">{{ d.definition_id }} · {{ d.definition_version }}</option></select></label></div><p v-if="!serverMode" role="status">正在读取服务配置…</p><p v-if="modes.decision_mode === 'model' && !modelConfigured" class="run-error">模型配置不完整；选择 model 创建会明确报错。</p><p v-if="modes.delivery_mode === 'gitlab' && !gitlabConfigured" class="run-error">GitLab 实例凭据或命名空间尚未就绪，远程交付前需要补齐。</p></section>
      </section>
      <footer v-if="!legacy" class="composer-area"><form class="composer" @submit.prevent="send"><textarea ref="composer" v-model="input" aria-label="项目需求" placeholder="描述项目需求，或询问当前进度…" :disabled="busy || terminal" @keydown.enter.exact="if (!$event.isComposing) { $event.preventDefault(); send() }" /><button class="primary" type="submit" aria-label="发送需求" :disabled="!canSend"><ArrowUp :size="18" /></button></form><p class="v2-composer-note">{{ terminal ? '任务已终止；重做会创建新的身份与确认。' : 'Enter 发送 · Shift+Enter 换行 · 方案确认前不生成文件' }}</p></footer>
    </main>
    <aside v-if="run" class="inspector" :class="{ open: inspector }" aria-label="运行详情"><button class="close-inspector" @click="inspector = false">关闭详情</button><h2>运行详情</h2><RuntimePanel :run="run" /><section class="v2-inspect-section"><h3>执行进度</h3><ul class="v2-check-list"><li v-for="step in run.steps" :key="step.step_id"><strong>{{ step.title }}</strong><span>{{ step.state }}</span><small>{{ step.detail }}</small></li></ul></section><section class="v2-inspect-section"><h3>必需验证 · {{ run.required_checks.filter(c => c.status === 'passed').length }}/{{ run.required_checks.length }}</h3><ul class="v2-check-list"><li v-for="check in run.required_checks" :key="check.check_id" :data-check-status="check.status"><strong>{{ checkLabels[check.check_id.replace('check.', '')] || check.check_id }}</strong><span>{{ stateNames[check.status] }}</span><small>{{ check.detail }}</small><details v-if="check.evidence_refs.length"><summary>证据 {{ check.evidence_refs.length }} 项</summary><pre>{{ JSON.stringify(check.evidence_refs, null, 2) }}</pre></details></li></ul></section><section class="v2-inspect-section"><h3>交付文件 · {{ run.artifacts.length }}</h3><button v-if="run.state === 'DONE'" class="primary" @click="artifact()">下载已验证 ZIP</button><ul class="v2-files"><li v-for="file in run.artifacts" :key="file.artifact_id"><button @click="artifact(file)">{{ file.path }}</button><small>{{ file.size }} B · <span :title="file.sha256">{{ file.sha256.slice(0, 12) }}…</span></small></li></ul><details v-if="run.manifest"><summary>完整 manifest 与来源</summary><pre>{{ JSON.stringify(run.manifest, null, 2) }}</pre></details></section><section class="v2-inspect-section"><h3>批准与定义</h3><p v-if="run.approval">批准 {{ run.approval.approval_id }} · 来源 v{{ run.approval.source_version }}</p><p v-else>尚无执行批准</p><p>{{ run.definition_snapshot.definition_id }} · {{ run.definition_snapshot.definition_version }}</p><details><summary>冻结来源与批准详情</summary><pre>{{ JSON.stringify({ definition_snapshot_hash: run.definition_snapshot.snapshot_hash, approval: run.approval, permission: run.permission_snapshot }, null, 2) }}</pre></details></section><section class="v2-inspect-section"><h3>预算与原操作</h3><p v-if="budgetError" class="run-error" role="status">{{ budgetError }}</p><p>业务命令 {{ run.usage.commands }} · 模型尝试 {{ run.usage.model_attempts }} · 模型失败 {{ run.usage.model_failures }}</p><details v-if="budget"><summary>预留、结算与累计期限</summary><pre>{{ JSON.stringify(budget, null, 2) }}</pre></details><details v-for="operation in run.operations" :key="operation.operation_id"><summary>{{ operation.action_type }} · {{ operation.status }}</summary><pre>{{ JSON.stringify(operation, null, 2) }}</pre></details></section><section class="v2-inspect-section"><h3>对话来源与派生摘要</h3><a :href="'/api/runs/' + run.run_id + '/transcript'" target="_blank" rel="noopener">读取完整原始 JSONL</a><p v-if="!run.summary_records.length">尚未生成摘要；原始消息完整保留。</p><details v-for="summary in run.summary_records" :key="summary.summary_hash"><summary>派生摘要 v{{ summary.summary_version }} · 消息 {{ summary.source.first_sequence }}–{{ summary.source.last_sequence }}</summary><p>来源 v{{ summary.source_version }} · {{ summary.generator.mode }} · {{ summary.generator.generator_version }}</p><pre>{{ summary.content }}</pre><p class="hash">{{ summary.source.raw_jsonl_sha256 }}</p><a :href="'/api/runs/' + run.run_id + '/transcript?first_sequence=' + summary.source.first_sequence + '&last_sequence=' + summary.source.last_sequence" target="_blank" rel="noopener">读取这段原文</a></details><details v-if="run.summary_attempts.length"><summary>摘要尝试记录</summary><pre>{{ JSON.stringify(run.summary_attempts, null, 2) }}</pre></details></section><section class="v2-inspect-section"><h3>历史记录</h3><ol class="v2-history"><li v-for="event in run.history" :key="event.event_id"><strong>{{ event.action }}</strong><small>来源 v{{ event.source_version }} · 进度 {{ event.state_sequence }}</small><p>{{ event.detail }}</p></li></ol></section></aside>
    <dialog ref="modal" class="v2-dialog"><header><h2>{{ modalTitle }}</h2><button @click="modal?.close()">关闭</button></header><pre>{{ modalText }}</pre></dialog>
  </div>
</template>
