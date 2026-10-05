<script setup lang="ts">
import { computed, useId } from 'vue'
import type { StateData, StateNode } from '../../../data/diagrams/business'
import type { DiagramSelection } from '../../../lib/diagram-types'
import { route, selection, selectionLabel } from './geometry'
import './business.css'
const props = defineProps<{ data: StateData; selectedId?: string }>()
const emit = defineEmits<{ select: [payload: DiagramSelection] }>()
const uid = `state-${useId().replaceAll(':', '')}`
const context = document.createElement('canvas').getContext('2d')
function wrap(text: string, maxWidth: number, fontSize: number, maxLines: number, bold = false): string[] {
  if (!text) return []
  if (context) context.font = `${bold ? '600 ' : ''}${fontSize}px Inter,-apple-system,BlinkMacSystemFont,"PingFang SC",sans-serif`
  const measure = (value: string) => context?.measureText(value).width ?? [...value].reduce((sum, char) => sum + fontSize * (/[\x00-\xff]/.test(char) ? .7 : 1), 0)
  const result: string[] = []; let line = ''
  for (const char of text) {
    if (line && measure(line + char) > maxWidth) { result.push(line); line = ''; }
    line += char
  }
  if (line) result.push(line)
  if (result.length > maxLines) {
    result.length = maxLines
    let last = result[maxLines - 1]!
    while (last && measure(last + '…') > maxWidth) last = [...last].slice(0, -1).join('')
    result[maxLines - 1] = last + '…'
  }
  return result
}
const boxes = computed(() => props.data.nodes.map(node => {
  const titleLines = wrap(node.label, 158, 18, 2, true)
  const actionLines = wrap((node.actions || []).join(' · '), 158, 13, 3)
  const header = titleLines.length > 1 ? 56 : 39
  return { ...node, titleLines, actionLines, header, x: 100 + node.column * 240, y: 150 + node.row * 180, width: node.kind === 'state' ? 184 : 30, height: node.kind === 'state' ? Math.max(94, header + 32 + actionLines.length * 17) : 30 }
}))
const width = computed(() => Math.max(1100, ...boxes.value.map(node => node.x + node.width / 2 + 65)))
const height = computed(() => Math.max(640, ...boxes.value.map(node => node.y + node.height / 2 + 100)))
const groups = computed(() => (props.data.groups || []).map(group => { const members = boxes.value.filter(node => group.members.includes(node.id)); if (!members.length) return undefined; const left = Math.min(...members.map(node => node.x - node.width / 2)) - 28, top = Math.min(...members.map(node => node.y - node.height / 2)) - 74; return { ...group, x: left, y: top, width: Math.max(...members.map(node => node.x + node.width / 2)) - left + 28, height: Math.max(...members.map(node => node.y + node.height / 2)) - top + 25 } }).filter(group => group !== undefined))
const edges = computed(() => props.data.edges.map(edge => { const drawing = route(edge, boxes.value); const source = boxes.value.find(node => node.id === edge.source), target = boxes.value.find(node => node.id === edge.target); if (drawing && source && target && Math.abs(source.y - target.y) < 1 && !edge.labelPosition) drawing.label.y = source.y - 65; return { ...edge, drawing } }).filter(edge => edge.drawing))
function select(node: StateNode) { emit('select', { ...selection(node, node.kind === 'state' ? '状态' : node.kind === 'initial' ? '初始伪状态' : node.kind === 'history' ? '历史伪状态' : '终止伪状态'), details: [...(node.actions || []), ...(node.details || [])] }) }
</script>
<template>
  <svg class="dg-svg dg-business" data-diagram="state" role="img" :aria-label="data.title" :viewBox="`0 0 ${width} ${height}`">
    <title>{{ data.title }}</title><desc>{{ data.description }}</desc><defs><marker v-for="tone in ['accent', 'success', 'warning', 'muted']" :id="`${uid}-${tone}`" :key="tone" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="7" markerHeight="7" orient="auto"><path d="M 0 0 L 10 5 L 0 10 Z" :class="`b-marker${tone === 'accent' ? '' : `-${tone}`}`" /></marker></defs>
    <g v-for="group in groups" :key="group!.id" class="b-node" tabindex="0" role="button" :aria-label="group!.label" @click="emit('select', selection(group!, '复合状态'))" @keydown.enter="emit('select', selection(group!, '复合状态'))"><rect :x="group!.x" :y="group!.y" :width="group!.width" :height="group!.height" rx="19" class="b-region" stroke-width="1.5" stroke-dasharray="6 4" /><text :x="group!.x + 19" :y="group!.y + 25" class="state-group-name">{{ group!.label }}</text></g>
    <g v-for="edge in edges" :key="edge.id" class="b-node" role="button" tabindex="0" :aria-label="selectionLabel(edge, data.nodes, '状态转换')" @click="emit('select', selection(edge, '状态转换', data.nodes))" @keydown.enter="emit('select', selection(edge, '状态转换', data.nodes))"><path :d="edge.drawing!.path" :class="['b-edge', `tone-${edge.tone || 'accent'}`]" :marker-end="`url(#${uid}-${edge.tone || 'accent'})`" /><path :d="edge.drawing!.path" fill="none" stroke="transparent" stroke-width="14" /><text v-if="edge.label" :x="edge.drawing!.label.x" :y="edge.drawing!.label.y" class="b-edge-label">{{ edge.label }}</text></g>
    <g v-for="node in boxes" :key="node.id" :class="['b-node', { 'is-selected': selectedId === node.id }]" role="button" tabindex="0" :aria-label="node.label" @click="select(node)" @keydown.enter="select(node)">
      <template v-if="node.kind === 'initial'"><circle :cx="node.x" :cy="node.y" r="13" fill="var(--dg-text, #20263e)" /><circle :cx="node.x" :cy="node.y" r="22" fill="transparent" /></template>
      <template v-else-if="node.kind === 'final'"><circle :cx="node.x" :cy="node.y" r="17" fill="var(--dg-canvas, #fff)" stroke="var(--dg-text, #20263e)" stroke-width="2" /><circle :cx="node.x" :cy="node.y" r="10" fill="var(--dg-text, #20263e)" /></template>
      <template v-else-if="node.kind === 'history'"><circle :cx="node.x" :cy="node.y" r="18" class="b-shape" /><text :x="node.x" :y="node.y + 6" class="b-title">H</text><text :x="node.x" :y="node.y + 37" class="state-history-label"><tspan v-for="(line, index) in wrap(node.label, 130, 12, 2)" :key="index" :x="node.x" :dy="index ? 16 : 0">{{ line }}</tspan></text></template>
      <template v-else><rect :x="node.x - node.width / 2" :y="node.y - node.height / 2" :width="node.width" :height="node.height" rx="13" :class="['b-shape', `b-${node.tone || 'soft'}`]" /><line :x1="node.x - node.width / 2" :x2="node.x + node.width / 2" :y1="node.y - node.height / 2 + node.header" :y2="node.y - node.height / 2 + node.header" stroke="var(--dg-border, #dee3ed)" /><text :x="node.x" :y="node.y - node.height / 2 + 26" class="b-title"><tspan v-for="(line, index) in node.titleLines" :key="index" :x="node.x" :dy="index ? 20 : 0">{{ line }}</tspan></text><text :x="node.x - node.width / 2 + 13" :y="node.y - node.height / 2 + node.header + 23" class="state-action"><tspan v-for="(line, index) in node.actionLines" :key="index" :x="node.x - node.width / 2 + 13" :dy="index ? 17 : 0">{{ line }}</tspan></text></template>
    </g>
    <text x="48" :y="height - 26" class="b-note">只读规范 · 事件 [守卫条件] / 动作 · 完整约束见来源详情</text>
  </svg>
</template>
<style scoped>.state-group-name { font-size: 16px; font-weight: 600; fill: var(--dg-accent, #6265e8); }.state-action { font-size: 13px; fill: var(--dg-muted, #8890a5); }.state-history-label { font-size: 12px; text-anchor: middle; fill: var(--dg-muted, #8890a5); }</style>
