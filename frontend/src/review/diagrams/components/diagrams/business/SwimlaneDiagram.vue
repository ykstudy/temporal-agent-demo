<script setup lang="ts">
import { computed, useId } from 'vue'
import type { SwimlaneData, SwimlaneNode } from '../../../data/diagrams/business'
import type { DiagramSelection } from '../../../lib/diagram-types'
import { route, selection } from './geometry'
import './business.css'
const props = defineProps<{ data: SwimlaneData; selectedId?: string }>()
const emit = defineEmits<{ select: [payload: DiagramSelection] }>()
const uid = `lane-${useId().replaceAll(':', '')}`
const laneHeight = 158
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
const lanes = computed(() => props.data.lanes.map(lane => ({ ...lane, labelLines: wrap(lane.label, 122, 17, 2, true) })))
const boxes = computed(() => props.data.nodes.map(node => {
  const titleLines = wrap(node.label, 144, 18, 3, true)
  const caption = wrap(node.caption || '', 144, 13, 1)[0] || ''
  const textHeight = titleLines.length * 21 + (caption ? 20 : 0)
  return { ...node, titleLines, shortCaption: caption, titleOffset: -textHeight / 2 + 16, captionOffset: textHeight / 2 - 4, x: 286 + node.column * 206, y: 138 + Math.max(0, props.data.lanes.findIndex(lane => lane.id === node.lane)) * laneHeight, width: 168, height: 108 }
}))
const width = computed(() => Math.max(1100, ...boxes.value.map(node => node.x + 142)))
const height = computed(() => 100 + Math.max(1, props.data.lanes.length) * laneHeight + 70)
const edges = computed(() => props.data.edges.map(edge => ({ ...edge, labelLines: wrap(edge.label || '', 172, 13, 2), drawing: route(edge, boxes.value) })).filter(edge => edge.drawing))
function select(node: SwimlaneNode) { const lane = props.data.lanes.find(lane => lane.id === node.lane); emit('select', { ...selection(node, node.kind === 'event' ? '里程碑事件' : '泳道任务'), details: [lane ? `负责角色：${lane.label}` : '', ...(node.details || [])].filter(Boolean) }) }
</script>
<template>
  <svg class="dg-svg dg-business" data-diagram="swimlane" role="img" :aria-label="data.title" :viewBox="`0 0 ${width} ${height}`">
    <title>{{ data.title }}</title><desc>{{ data.description }}</desc><defs><marker :id="uid" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="7" markerHeight="7" orient="auto"><path d="M 0 0 L 10 5 L 0 10 Z" class="b-marker" /></marker></defs>
    <text x="50" y="35" class="b-note">角色与职责</text><text x="220" y="35" class="b-note">业务交接沿时间从左向右展开</text>
    <g v-for="(lane, index) in lanes" :key="lane.id" class="b-node" role="button" tabindex="0" :aria-label="lane.label" @click="emit('select', selection(lane, '职责泳道'))" @keydown.enter="emit('select', selection(lane, '职责泳道'))">
      <rect x="28" :y="59 + index * laneHeight" :width="width - 56" :height="laneHeight" :fill="index % 2 ? 'var(--dg-canvas, #fff)' : 'var(--dg-soft, #f5f5fc)'" stroke="var(--dg-border, #dee3ed)" />
      <rect x="28" :y="59 + index * laneHeight" width="154" :height="laneHeight" :fill="lane.color || '#6265e8'" fill-opacity=".055" />
      <rect x="28" :y="59 + index * laneHeight" width="4" :height="laneHeight" :fill="lane.color || '#6265e8'" />
      <circle cx="61" :cy="118 + index * laneHeight" r="15" :fill="lane.color || '#6265e8'" fill-opacity=".12" /><text x="61" :y="124 + index * laneHeight" font-size="15" text-anchor="middle" fill="var(--dg-text, #20263e)">{{ index + 1 }}</text>
      <text x="48" :y="162 + index * laneHeight - (lane.labelLines.length - 1) * 10" class="lane-name"><tspan v-for="(line, lineIndex) in lane.labelLines" :key="lineIndex" x="48" :dy="lineIndex ? 20 : 0">{{ line }}</tspan></text>
    </g>
    <g v-for="edge in edges" :key="edge.id"><path :d="edge.drawing!.path" class="b-edge" :marker-end="`url(#${uid})`" /><text v-if="edge.label" :x="edge.drawing!.label.x" :y="edge.drawing!.label.y - (edge.labelLines.length - 1) * 8" class="b-edge-label"><tspan v-for="(line, index) in edge.labelLines" :key="index" :x="edge.drawing!.label.x" :dy="index ? 16 : 0">{{ line }}</tspan></text></g>
    <g v-for="node in boxes" :key="node.id" :class="['b-node', { 'is-selected': selectedId === node.id }]" role="button" tabindex="0" :aria-label="node.label" @click="select(node)" @keydown.enter="select(node)">
      <rect :x="node.x - node.width / 2" :y="node.y - node.height / 2" :width="node.width" :height="node.height" :rx="node.kind === 'event' ? 34 : 11" :class="['b-shape', { 'b-success': node.kind === 'event' }]" />
      <rect :x="node.x - node.width / 2" :y="node.y - node.height / 2" :width="node.width" height="4" rx="2" :fill="data.lanes.find(lane => lane.id === node.lane)?.color || '#6265e8'" fill-opacity=".5" />
      <text :x="node.x" :y="node.y + node.titleOffset" class="b-title lane-node-title"><tspan v-for="(line, index) in node.titleLines" :key="index" :x="node.x" :dy="index ? 21 : 0">{{ line }}</tspan></text><text v-if="node.shortCaption" :x="node.x" :y="node.y + node.captionOffset" class="b-caption">{{ node.shortCaption }}</text>
    </g>
    <text x="32" :y="height - 22" class="b-note">只读职责模型 · 跨泳道连线表示职责交接 · 完整文字见来源详情</text>
  </svg>
</template>
<style scoped>.lane-name { font-size: 17px; font-weight: 600; fill: var(--dg-text, #20263e); }.lane-node-title { font-size: 18px; }.dg-business .b-caption { font-size: 13px; }.dg-business .b-edge-label { font-size: 13px; }</style>
