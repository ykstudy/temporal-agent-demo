<script setup lang="ts">
import TechnicalIcon from './TechnicalIcon.vue'
import type { TechnicalNode } from '../../../data/diagrams/technical'
import type { Box } from './layout'
defineProps<{ node: TechnicalNode; box: Box; selected?: boolean; footer?: string }>()
const emit = defineEmits<{ select: [] }>()
</script>
<template><g :transform="`translate(${box.x}, ${box.y})`" :class="['tt-node', `tt-${node.tone || 'indigo'}`, { 'tt-selected': selected }]" role="button" tabindex="0" :aria-label="node.label" :aria-pressed="selected" @click="emit('select')" @keydown.enter="emit('select')" @keydown.space.prevent="emit('select')">
  <rect class="tt-card" :width="box.w" :height="box.h" rx="12"/>
  <rect class="tt-icon-bg" x="18" y="18" width="42" height="42" rx="11"/>
  <TechnicalIcon :name="node.icon" :x="26" :y="26" :size="26" class="tt-icon"/>
  <text class="tt-title" x="74" y="36">{{ node.label }}</text>
  <text v-if="node.subtitle" class="tt-subtitle" x="18" :y="footer ? 86 : box.h - 22">{{ node.subtitle }}</text>
  <text v-if="footer" class="tt-footer" x="18" :y="box.h - 17">{{ footer }}</text>
</g></template>
