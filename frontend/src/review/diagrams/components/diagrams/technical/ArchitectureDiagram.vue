<script setup lang="ts">
import { computed, useId } from 'vue'
import type { ArchitectureData } from '../../../data/diagrams/technical'
import type { DiagramSelection } from '../../../lib/diagram-types'
import TechnicalNode from './TechnicalNode.vue'
import TechnicalEdge from './TechnicalEdge.vue'
import TechnicalDefs from './TechnicalDefs.vue'
import { nodeSelection, edgeSelection, route, textWidth, type Box } from './layout'
import './technical.css'
const props = defineProps<{ data: ArchitectureData; selectedId?: string }>()
const emit = defineEmits<{ select: [payload: DiagramSelection] }>()
const prefix = useId().replace(/:/g, '')
const width = 1300
const layers = computed(() => {let y=55;return props.data.layers.map(layer=>{const rows=Math.max(1,Math.ceil(layer.nodes.length/3)),h=rows*146+24;const item={...layer,y,h};y+=h+34;return item})})
const height = computed(() => Math.max(500,...layers.value.map(layer=>layer.y+layer.h+90)))
const boxes = computed(() => {
  const map: Record<string, Box> = {}
  layers.value.forEach(layer => {
    layer.nodes.forEach((node, i) => { map[node.id] = { x: 300 + i%3*310, y: layer.y+24+Math.floor(i/3)*146, w:270, h:108 } })
  })
  return map
})
const edges = computed(() => props.data.connections.flatMap(edge => boxes.value[edge.source] && boxes.value[edge.target] ? [{ edge, ...route(boxes.value[edge.source]!, boxes.value[edge.target]!, true) }] : []))
function description(text=''){let short=text;while(textWidth(short,16)>190)short=short.slice(0,-1);return short===text?text:short+'…'}
</script>
<template><svg class="dg-svg" data-diagram="architecture" role="img" :aria-label="data.title" :viewBox="`0 0 ${width} ${height}`">
  <TechnicalDefs :prefix="prefix"/>
  <g v-for="layer in layers" :key="layer.id"><rect class="tt-boundary" x="275" :y="layer.y" :width="width-315" :height="layer.h" rx="17"/><text class="tt-section" x="42" :y="layer.y+57">{{ layer.label }}</text><text class="tt-section-sub" x="42" :y="layer.y+87">{{ description(layer.description) }}<title>{{layer.description}}</title></text></g>
  <TechnicalEdge v-for="item in edges" :key="item.edge.id" :id="item.edge.id" :path="item.d" :label="item.edge.label" :at="item.label" :prefix="prefix" :dashed="item.edge.dashed" :selected="selectedId === item.edge.id" @select="emit('select', edgeSelection(item.edge,'架构连接'))"/>
  <template v-for="layer in data.layers" :key="layer.id"><TechnicalNode v-for="node in layer.nodes" :key="node.id" :node="node" :box="boxes[node.id]!" :selected="selectedId===node.id" @select="emit('select',nodeSelection(node,layer.label,[`所属层：${layer.label}`]))"/></template>
  <text class="tt-caption" x="42" :y="height-32">产品模块 / 离线设计工具 / 独立场景调用器 · 连线不表示执行调度</text>
</svg></template>
