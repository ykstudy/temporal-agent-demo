<script setup lang="ts">
import { onBeforeUnmount, ref, watch } from 'vue';
import { Check, Download, FileCode2, History, X } from 'lucide-vue-next';
import type { Run } from '../types';
import { date, request } from '../lib/api';
import { safeMarkdownUrl } from '../lib/markdown';

const props = defineProps<{ run: Run }>();
const emit = defineEmits<{ close: [] }>();
const tab = ref<'artifacts' | 'history'>('artifacts');
const file = ref<{ path: string; content: string } | null>(null);
const error = ref('');
let requestSequence = 0;
let alive = true;
const stepLabels: Record<string, string> = { pending: '等待执行', completed: '已完成', running: '执行中', skipped: '本次交付不适用' };

watch(() => props.run.id, () => {
  requestSequence++;
  file.value = null;
  error.value = '';
  tab.value = 'artifacts';
}, { flush: 'sync' });
onBeforeUnmount(() => { alive = false; requestSequence++; });

async function inspect(path: string) {
  error.value = '';
  const sequence = ++requestSequence;
  const id = props.run.id;
  try {
    const result = await request<{ path: string; content: string }>(`/runs/${id}/artifacts?path=${encodeURIComponent(path)}`);
    if (alive && sequence === requestSequence && props.run.id === id) file.value = result;
  } catch (err) {
    if (alive && sequence === requestSequence && props.run.id === id) error.value = err instanceof Error ? err.message : '读取失败';
  }
}
</script>

<template>
  <div class="inspector-heading"><h2>运行与产物</h2><button class="icon-button close-inspector" aria-label="关闭产物面板" @click="emit('close')"><X :size="18" /></button></div>
  <div class="progress-list"><div v-for="step in run.steps" :key="step.id" class="step" :class="step.state"><span><Check v-if="['done', 'completed'].includes(step.state)" :size="12" /><template v-else>{{ step.state === 'running' ? '•' : '·' }}</template></span><div>{{ step.title }}<small>{{ step.detail || stepLabels[step.state] || step.state }}</small></div></div></div>
  <div class="tabs"><button :class="{ active: tab === 'artifacts' }" @click="tab = 'artifacts'">产物 <span>{{ run.artifacts.length }}</span></button><button :class="{ active: tab === 'history' }" @click="tab = 'history'"><History :size="14" />历史</button></div>
  <template v-if="tab === 'artifacts'">
    <div class="artifact-list"><p v-if="run.artifacts.length === 0" class="muted">确认并生成后，文件将显示在这里。</p><button v-for="artifact in run.artifacts" :key="artifact.path" :class="{ active: file?.path === artifact.path }" @click="inspect(artifact.path)"><FileCode2 :size="14" /><span>{{ artifact.path }}</span></button></div>
    <a v-if="run.state === 'DONE'" class="download" :href="`/api/runs/${run.id}/download`"><Download :size="16" />下载项目 ZIP</a>
    <div v-if="file" class="artifact-content"><strong>{{ file.path }}</strong><pre>{{ file.content }}</pre></div>
    <p v-if="error" role="alert" class="inline-error">{{ error }}</p>
  </template>
  <ol v-else class="history-list"><li v-for="item in run.history.slice().reverse()" :key="item.id"><small>{{ date(item.created_at) }}</small><strong>{{ item.action }}</strong><p>{{ item.detail }}</p></li></ol>
  <div class="run-facts"><strong>当前运行</strong><p>执行 <span>{{ run.execution_mode }}</span></p><p>决策 <span>{{ run.decision_mode || 'rules' }}</span></p><p>交付 <span>{{ run.delivery_mode || run.plan.repo_provider }}</span></p><p>来源版本 <span>{{ run.source_version }}</span></p><p>状态序号 <span>{{ run.state_sequence }}</span></p>
    <a v-if="run.repo_url && safeMarkdownUrl(run.repo_url)" :href="run.repo_url" target="_blank" rel="noreferrer noopener">打开 GitLab 仓库 ↗</a><a v-if="run.pipeline_url && safeMarkdownUrl(run.pipeline_url)" :href="run.pipeline_url" target="_blank" rel="noreferrer noopener">查看 CI 结果 ↗</a>
  </div>
</template>
