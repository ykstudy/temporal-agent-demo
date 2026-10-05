<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, shallowRef, watch } from 'vue';
import { ArrowUp, Menu, PanelRight, RefreshCw, Sparkles } from 'lucide-vue-next';
import type { Bootstrap, Run } from './types';
import { post, request, stateLabel, time, uniqueId } from './lib/api';
import Inspector from './components/Inspector.vue';
import MarkdownMessage from './components/MarkdownMessage.vue';
import ProposalCard from './components/ProposalCard.vue';
import Sidebar from './components/Sidebar.vue';

const terminal = (state: string) => ['DONE', 'FAILED', 'CANCELLED'].includes(state);
const executing = (state: string) => ['IMPLEMENT', 'VERIFY', 'DELIVER'].includes(state);
const stages = ['理解需求', '方案确认', '项目生成', '交付验证'];
const stageIndexes: Record<string, number> = { INTAKE: 0, CLARIFY: 0, PLAN: 1, IMPLEMENT: 2, VERIFY: 3, DELIVER: 3, DONE: 3 };
const bootstrap = shallowRef<Bootstrap | null>(null);
const runs = shallowRef<Run[]>([]);
const selected = ref('');
const run = shallowRef<Run | null>(null);
const content = ref('');
const error = ref('');
const busy = ref(false);
const sidebarOpen = ref(false);
const inspectorOpen = ref(false);
const connected = ref(false);
const composing = ref(false);
const bottom = ref<HTMLDivElement | null>(null);
const pending = new Map<string, string>();
let alive = true;

function choose(id: string) {
  selected.value = id;
  sidebarOpen.value = false;
  error.value = '';
}

function receive(next: Run) {
  if (!alive) return;
  if (selected.value === next.id && !(run.value?.id === next.id && (run.value.state_sequence || 0) > (next.state_sequence || 0))) run.value = next;
  const prior = runs.value.find(item => item.id === next.id);
  if (prior && prior.state_sequence > next.state_sequence) return;
  runs.value = [next, ...runs.value.filter(item => item.id !== next.id)].sort((a, b) => b.updated_at.localeCompare(a.updated_at));
}

async function refresh() {
  try {
    const [config, list] = await Promise.all([request<Bootstrap>('/bootstrap'), request<Run[]>('/runs')]);
    if (!alive) return;
    bootstrap.value = config;
    runs.value = list.map(next => {
      const prior = runs.value.find(item => item.id === next.id);
      return prior && prior.state_sequence > next.state_sequence ? prior : next;
    }).concat(runs.value.filter(item => !list.some(next => next.id === item.id)));
  } catch (err) {
    if (alive) error.value = err instanceof Error ? err.message : '无法连接服务';
  }
}

onMounted(() => { void refresh(); });
onBeforeUnmount(() => { alive = false; });

watch(selected, (id, _previous, onCleanup) => {
  connected.value = false;
  if (!id) { run.value = null; return; }
  if (run.value?.id !== id) run.value = null;
  let cancelled = false;
  const current = () => alive && !cancelled && selected.value === id;
  const fetchRun = async () => {
    try {
      const next = await request<Run>(`/runs/${id}`);
      if (current()) receive(next);
    } catch (err) {
      if (current()) error.value = err instanceof Error ? err.message : '读取失败';
    }
  };
  void fetchRun();
  const stream = new EventSource(`/api/runs/${id}/events`);
  stream.onopen = () => {
    if (!current()) return;
    connected.value = true;
    // Re-read after subscribing to cover changes between the initial GET and stream establishment.
    void fetchRun();
  };
  stream.onerror = () => { if (current()) connected.value = false; };
  stream.addEventListener('snapshot', event => {
    if (!current()) return;
    try {
      const next = JSON.parse((event as MessageEvent).data) as Run;
      if (next.id !== id) { error.value = '状态事件所属任务错误，请刷新'; return; }
      receive(next);
    }
    catch { error.value = '状态事件格式错误，请刷新'; }
  });
  const timer = window.setInterval(() => { if (stream.readyState !== EventSource.OPEN) void fetchRun(); }, 3000);
  onCleanup(() => { cancelled = true; stream.close(); window.clearInterval(timer); });
}, { flush: 'sync' });

watch([() => run.value?.messages.length, () => run.value?.proposal?.id], () => {
  bottom.value?.scrollIntoView({ block: 'end' });
}, { flush: 'post' });

async function operate(key: string, fn: (id: string) => Promise<Run>, selectResult = false) {
  if (busy.value) return;
  busy.value = true;
  error.value = '';
  const originSelection = selected.value;
  const id = pending.get(key) || uniqueId();
  pending.set(key, id);
  try {
    const next = await fn(id);
    pending.delete(key);
    if (!alive) return;
    if (selectResult && selected.value === originSelection) choose(next.id);
    receive(next);
    return next;
  } catch (err) {
    if (alive && selected.value === originSelection) error.value = err instanceof Error ? err.message : '请求失败，请重试相同操作';
  } finally {
    if (alive) busy.value = false;
  }
}

async function create() {
  await operate('create', id => post<Run>('/runs', { id }), true);
  sidebarOpen.value = false;
}

async function send() {
  if (disabled.value) return;
  const message = content.value.trim();
  if (!message) return;
  const originSelection = selected.value;
  const originalDraft = content.value;
  const result = await operate(`message:${originSelection}:${message}`, id => originSelection
    ? post<Run>(`/runs/${originSelection}/messages`, { id, content: message })
    : post<Run>('/runs', { id, content: message }), !originSelection);
  if (result && content.value === originalDraft && (selected.value === originSelection || (!originSelection && selected.value === result.id))) content.value = '';
}

async function act(action: string) {
  const currentRun = run.value;
  if (!currentRun) return;
  const proposal = currentRun.proposal;
  const binding = ['accept', 'reject', 'revise'].includes(action) && proposal ? {
    proposal_id: proposal.id, proposal_version: proposal.proposal_version,
    source_version: proposal.source_version, snapshot_hash: proposal.snapshot_hash,
  } : {};
  await operate(`action:${currentRun.id}:${action}:${proposal?.id}:${currentRun.state_sequence}`,
    id => post<Run>(`/runs/${currentRun.id}/actions`, { id, action, ...binding }), action === 'retry');
}

async function refreshSelected() {
  void refresh();
  const id = selected.value;
  if (!id) return;
  try { receive(await request<Run>(`/runs/${id}`)); }
  catch (err) { if (alive && selected.value === id) error.value = err instanceof Error ? err.message : '读取失败'; }
}

function keydown(event: KeyboardEvent) {
  if (event.key === 'Enter' && !event.shiftKey && !event.isComposing && event.keyCode !== 229 && !composing.value) {
    event.preventDefault();
    void send();
  }
}

const disabled = computed(() => busy.value || !!run.value && (executing(run.value.state) || terminal(run.value.state) || ['PAUSED', 'BLOCKED'].includes(run.value.state)));
const stage = computed(() => stageIndexes[run.value?.state || ''] ?? 1);
const modeHint = computed(() => bootstrap.value ? `${bootstrap.value.execution_mode || bootstrap.value.mode} 执行 · ${bootstrap.value.decision_mode || bootstrap.value.provider} 决策 · ${bootstrap.value.delivery_mode || 'local'} 交付` : '');
</script>

<template>
  <div class="workspace">
    <aside class="sidebar" :class="{ open: sidebarOpen }"><Sidebar :runs="runs" :selected="selected" @create="create" @choose="choose" /></aside>
    <button v-if="sidebarOpen || inspectorOpen" class="scrim" aria-label="关闭侧面板" @click="sidebarOpen = false; inspectorOpen = false" />
    <main class="main">
      <header class="topbar"><button class="icon-button mobile-menu" aria-label="打开任务列表" @click="sidebarOpen = true"><Menu :size="20" /></button><div><h1>{{ run?.plan.project_name || run?.title || '项目创建工作区' }}</h1><p v-if="run"><span class="dot" :class="run.state.toLowerCase()" />{{ stateLabel(run.state) }}<span class="connection">{{ connected ? '状态已同步' : '正在同步状态' }}</span></p><p v-else>通过对话创建可运行的 Go 项目</p></div><button class="icon-button" aria-label="刷新任务" @click="refreshSelected"><RefreshCw :size="17" /></button><button v-if="run" class="icon-button panel-toggle" aria-label="打开产物面板" @click="inspectorOpen = !inspectorOpen"><PanelRight :size="19" /></button></header>
      <div class="conversation" aria-live="polite">
        <div v-if="!run" class="empty"><div class="empty-icon"><Sparkles :size="24" /></div><h2>从一个项目需求开始</h2><p>描述项目名称、数据库、认证和日志需求。<br />先审查方案与文件预览，确认后开始生成。</p><button class="example" @click="content = '创建 order-service，使用 SQLite + GORM、JWT 认证和 JSON 日志'">创建 order-service，使用 SQLite、JWT 和 JSON 日志 <span aria-hidden="true">↗</span></button><div class="mode-hint">{{ modeHint }}</div></div>
        <template v-else>
          <div class="stage-strip"><span v-for="(label, index) in stages" :key="label" :class="{ current: index <= stage }">{{ index + 1 }}<b>{{ label }}</b></span></div>
          <article v-for="message in run.messages" :key="message.id" class="message" :class="message.role"><div class="avatar">{{ message.role === 'user' ? '你' : '◇' }}</div><div class="message-body"><div class="message-meta">{{ message.role === 'user' ? '你' : '项目创建 Agent' }}<time>{{ time(message.created_at) }}</time></div><MarkdownMessage :content="message.content" /></div></article>
          <ProposalCard v-if="run.proposal" :run="run" :disabled="busy" @act="act" />
          <div v-if="run.error" class="run-error" role="alert"><strong>{{ stateLabel(run.state) }}</strong><p>{{ run.error }}</p></div>
          <div class="run-actions"><button v-if="executing(run.state)" :disabled="busy" @click="act('pause')">暂停执行</button><button v-if="['PAUSED', 'BLOCKED'].includes(run.state)" :disabled="busy" @click="act('resume')">恢复执行</button><button v-if="!terminal(run.state)" :disabled="busy" @click="act('cancel')">取消任务</button><button v-if="terminal(run.state)" :disabled="busy" @click="act('retry')">新建重做任务</button></div>
        </template>
        <div ref="bottom" />
      </div>
      <div class="composer-area"><p v-if="error" class="inline-error" role="alert">{{ error }}</p><div class="composer"><textarea v-model="content" aria-label="项目需求" :disabled="disabled" :placeholder="disabled ? '当前任务请使用阶段操作，或新建对话' : '描述需求，或继续调整方案…'" @keydown="keydown" @compositionstart="composing = true" @compositionend="composing = false" /><button class="send" aria-label="发送需求" :disabled="disabled || !content.trim()" @click="send"><ArrowUp :size="20" /></button></div><div class="composer-footer"><span>Enter 发送 · Shift + Enter 换行</span><span>{{ busy ? '处理请求中…' : '确认后执行 · 原始对话保留' }}</span></div></div>
    </main>
    <aside v-if="run" class="inspector" :class="{ open: inspectorOpen }"><Inspector :run="run" @close="inspectorOpen = false" /></aside>
  </div>
</template>
