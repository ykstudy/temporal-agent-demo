<script setup lang="ts">
import { textWidth } from './layout'
import type { Point } from './layout'
defineProps<{ id: string; path: string; label?: string; at?: Point; prefix: string; dashed?: boolean; selected?: boolean; open?: boolean; arrow?: boolean }>()
const emit = defineEmits<{ select: [] }>()
</script>
<template><g :class="['tt-edge', { 'tt-selected-edge': selected }]" role="button" tabindex="0" :aria-label="label || '查看关系'" :aria-pressed="selected" @click="emit('select')" @keydown.enter="emit('select')" @keydown.space.prevent="emit('select')">
  <path :d="path" class="tt-edge-hit"/><path :d="path" class="tt-edge-line" :stroke-dasharray="dashed ? '7 6' : undefined" :marker-end="arrow === false ? undefined : `url(#${prefix}-${open ? 'open' : 'arrow'})`"/>
  <g v-if="label && at" :transform="`translate(${at.x},${at.y})`"><rect :x="-textWidth(label) / 2 - 9" y="-13" :width="textWidth(label) + 18" height="26" rx="6" class="tt-label-bg"/><text class="tt-edge-label" text-anchor="middle" y="5">{{ label }}</text></g>
</g></template>
