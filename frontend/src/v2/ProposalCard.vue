<script setup lang="ts">
import { computed, onBeforeUnmount, ref, shallowRef, watch } from 'vue'
import type { ObjectReference, Proposal, RunSnapshot, ControlRequest } from '../contracts/types.generated'
import { proposal as readProposal, proposalRef } from './api'
import { runRef } from '../contracts/runtime'
const props = defineProps<{ reference: ObjectReference; run: RunSnapshot; busy: boolean }>()
const emit = defineEmits<{ action: [intent: ControlRequest] }>()
const value = shallowRef<Proposal | null>(null), error = ref(''), file = ref(''), clock = ref(Date.now())
const timer = window.setInterval(() => { clock.value = Date.now() }, 1000)
onBeforeUnmount(() => window.clearInterval(timer))
watch(() => JSON.stringify([props.run.run_id, props.reference.object_type, props.reference.object_id, props.reference.object_version, props.reference.source_version, props.reference.hash]), async (_v, _old, cleanup) => {
  let active = true; cleanup(() => { active = false }); value.value = null; error.value = ''
  try { const p = await readProposal(props.run.run_id, props.reference); if (active) value.value = p } catch (e) { if (active) error.value = e instanceof Error ? e.message : '历史方案读取失败' }
}, { immediate: true })
const current = computed(() => value.value?.snapshot_hash === props.run.proposal?.snapshot_hash && value.value?.source_version === props.run.source_version)
const pending = computed(() => current.value && props.run.state === 'PLAN')
const expired = computed(() => !value.value || Date.parse(value.value.expires_at) <= clock.value)
const selected = computed(() => value.value?.changes.find(c => c.path === file.value))
function act(action: 'accept' | 'reject' | 'revise') {
  if (!value.value || !pending.value) return
  emit('action', { command_id: crypto.randomUUID(), action, target: runRef(props.run), required_proposal_ref: proposalRef(value.value) })
}
</script>
<template>
  <section class="proposal v2-proposal" aria-label="项目方案预览" :data-proposal-hash="reference.hash">
    <p v-if="error" role="alert" class="inline-error">{{ error }}</p><p v-else-if="!value" role="status">正在核验历史方案…</p>
    <template v-else>
      <div class="proposal-heading"><div><h3>{{ value.plan.project_name }} · 项目方案</h3><small>方案 v{{ value.proposal_version }} · 来源 v{{ value.source_version }} · {{ current ? '当前方案' : '历史方案，只读' }}</small></div></div>
      <dl class="plan-grid"><div><dt>技术栈</dt><dd>{{ value.plan.tech_stack }}</dd></div><div><dt>交付</dt><dd>{{ value.plan.delivery_target.provider }}</dd></div><div><dt>能力</dt><dd>{{ value.plan.selected_skills.map(s => s.skill_id).join(' · ') || '基础服务' }}</dd></div><div><dt>文件</dt><dd>{{ value.changes.length }} 个</dd></div></dl>
      <details class="v2-details"><summary>批准范围与精确来源</summary><p class="hash">{{ value.snapshot_hash }}</p><p>服务端有效期：{{ new Date(value.issued_at).toLocaleString() }} 至 {{ new Date(value.expires_at).toLocaleString() }}</p><pre>{{ JSON.stringify(value.plan.delivery_target, null, 2) }}</pre><pre>{{ JSON.stringify(value.plan.selected_skills, null, 2) }}</pre></details>
      <button class="files-toggle" @click="file = file ? '' : value.changes[0]?.path || ''">{{ file ? '收起文件预览' : `预览 ${value.changes.length} 个文件` }}</button>
      <div v-if="file" class="preview-files"><select v-model="file" aria-label="选择预览文件"><option v-for="change in value.changes" :key="change.path" :value="change.path">{{ change.operation }} · {{ change.path }}</option></select><div class="v2-diff"><div v-if="selected?.before !== null"><small>修改前</small><pre>{{ selected?.before }}</pre></div><div v-if="selected?.after !== null"><small>修改后</small><pre>{{ selected?.after }}</pre></div></div></div>
      <template v-if="pending"><p class="approval-note">确认后按此方案生成文件、执行验证，并{{ run.modes.delivery_mode === 'gitlab' ? '创建准确目标的 GitLab 仓库与 CI' : '提供通过验证的 ZIP' }}。</p><div class="proposal-actions"><button class="primary" :disabled="busy || expired" @click="act('accept')">确认并生成</button><button :disabled="busy" @click="act('revise')">再改改</button><button :disabled="busy" @click="act('reject')">拒绝方案</button></div><p v-if="expired" role="status">方案已过期，请重新提交需求以签发新方案。</p></template>
      <p v-else class="v2-muted">{{ current ? '此方案已进入后续阶段，不能再次确认。' : '这张卡片始终显示原方案；不会替换为最新方案。' }}</p>
    </template>
  </section>
</template>
