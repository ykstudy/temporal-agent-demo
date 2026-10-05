<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue';
import { Check, ChevronDown, ChevronRight, FileCode2 } from 'lucide-vue-next';
import type { Run } from '../types';
import { date } from '../lib/api';

const props = defineProps<{ run: Run; disabled: boolean }>();
const emit = defineEmits<{ act: [action: string] }>();
const proposal = computed(() => props.run.proposal);
const file = ref('');
const clock = ref(Date.now());
const timer = window.setInterval(() => { clock.value = Date.now(); }, 1000);
onBeforeUnmount(() => window.clearInterval(timer));
watch(() => proposal.value?.id, () => { file.value = ''; }, { flush: 'sync' });
const expired = computed(() => {
  const expiry = Date.parse(proposal.value?.expires_at || '');
  return !Number.isFinite(expiry) || expiry <= clock.value;
});
const pending = computed(() => proposal.value?.status === 'pending' && props.run.state === 'PLAN');
const selectedFile = computed(() => proposal.value?.changes.find(change => change.path === file.value));
const skillLabels: Record<string, string> = { add_database: '数据库', add_auth: '认证', add_logging: '结构化日志' };
const checkLabels: Record<string, string> = { plan_schema: '参数校验', secret_boundary: '秘密边界' };
</script>

<template>
  <section v-if="proposal" class="proposal" aria-label="项目方案预览">
    <div class="proposal-heading"><span class="card-symbol"><FileCode2 :size="18" /></span><div><h3>{{ proposal.title || '项目方案预览' }}</h3><small>方案 v{{ proposal.proposal_version }} · 来源 v{{ proposal.source_version }} · {{ pending ? '等待确认' : proposal.status }}</small></div></div>
    <dl class="plan-grid">
      <div><dt>项目</dt><dd>{{ run.plan.project_name }}</dd></div><div><dt>技术栈</dt><dd>Go 1.25</dd></div>
      <div><dt>能力</dt><dd>{{ run.plan.skills.map(id => skillLabels[id] || id).join(' · ') || '基础服务' }}</dd></div><div><dt>交付</dt><dd>{{ run.delivery_mode || run.plan.repo_provider }}</dd></div>
      <div v-if="run.plan.skills.includes('add_database')"><dt>数据库</dt><dd>{{ run.plan.database }} / {{ run.plan.orm }}</dd></div>
      <div v-if="run.plan.skills.includes('add_auth')"><dt>认证</dt><dd>{{ run.plan.auth }} / {{ run.plan.token_ttl }}</dd></div>
      <div v-if="run.plan.skills.includes('add_logging')"><dt>日志</dt><dd>{{ run.plan.log_level }} / {{ run.plan.log_format }} / {{ run.plan.log_output || 'console' }}</dd></div>
    </dl>
    <div v-if="run.delivery_mode === 'gitlab'" class="approval-scope"><strong>GitLab 授权目标</strong><p>实例：{{ proposal.target?.gitlab_url || '未配置' }}</p><p>命名空间：{{ proposal.target?.namespace_id || '未配置，需要明确目标' }}</p><p>项目路径：{{ proposal.target?.project_path }} · 分支：{{ proposal.target?.ref }}</p></div>
    <div v-if="run.plan.skills.includes('add_logging') && run.plan.log_output === 'file'" class="approval-scope"><strong>文件日志配置</strong><p>路径：{{ run.plan.log_path }} · 大小：{{ run.plan.log_max_size }} 字节</p><p>保留：{{ run.plan.log_max_backups }} 份 / {{ run.plan.log_max_age }} · 错误策略：{{ run.plan.log_on_write_error }}</p></div>
    <button class="files-toggle" @click="file = file ? '' : proposal.changes[0]?.path || ''"><ChevronDown v-if="file" :size="16" /><ChevronRight v-else :size="16" />预览 {{ proposal.changes.length }} 个文件<span>确认后生成</span></button>
    <div v-if="file" class="preview-files"><select v-model="file" aria-label="选择预览文件"><option v-for="change in proposal.changes" :key="change.path" :value="change.path">{{ change.path }}</option></select><pre>{{ selectedFile?.after }}</pre></div>
    <div class="proposal-checks"><p v-for="(check, index) in proposal.checks" :key="`${check.name}-${index}`" :class="{ 'failed-check': !check.passed }"><Check :size="14" />{{ checkLabels[check.name] || check.name }}<span>{{ check.detail }}</span></p></div>
    <template v-if="pending">
      <p class="approval-note">确认将生成以上文件{{ run.delivery_mode === 'gitlab' ? '，并创建 GitLab 仓库、推送代码及验证 CI' : '，完成测试、构建并提供 ZIP 下载' }}。方案有效至 {{ date(proposal.expires_at) }}。</p>
      <div class="proposal-actions"><button class="primary" :disabled="disabled || expired" @click="emit('act', 'accept')">确认并生成</button><button :disabled="disabled" @click="emit('act', 'revise')">再改改</button><button class="text-button" :disabled="disabled" @click="emit('act', 'reject')">拒绝方案</button></div>
      <p v-if="expired" role="status" class="inline-error">方案已过期，请选择“再改改”后重新生成方案。</p>
    </template>
  </section>
</template>
