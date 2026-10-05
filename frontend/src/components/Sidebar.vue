<script setup lang="ts">
import { Folder, Plus } from 'lucide-vue-next';
import type { Run } from '../types';
import { stateLabel } from '../lib/api';

defineProps<{ runs: Run[]; selected: string }>();
const emit = defineEmits<{ create: []; choose: [id: string] }>();
</script>

<template>
  <div class="brand"><span class="brand-icon">◇</span><div>项目创建 Agent<small>Temporal Project Agent</small></div></div>
  <button class="new-run" @click="emit('create')"><Plus :size="17" />新建对话</button>
  <div class="section-label">项目任务 <span>{{ runs.length }}</span></div>
  <nav aria-label="项目任务">
    <button v-for="run in runs" :key="run.id" class="run-item" :class="{ selected: selected === run.id }" @click="emit('choose', run.id)">
      <Folder :size="16" /><span>{{ run.plan.project_name || run.title }}<small>{{ stateLabel(run.state) }}</small></span><i class="dot" :class="run.state.toLowerCase()" />
    </button>
  </nav>
  <div class="sidebar-footer"><a class="review-entry" href="?review">设计审查 ↗</a><span class="status-light" />独立项目工作区<small>需求 → 审查 → 生成 → 交付</small></div>
</template>

<style scoped>
.review-entry { display: block; margin-bottom: 12px; }
</style>
