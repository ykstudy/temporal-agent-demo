<script setup lang="ts">
import { computed, useId } from 'vue'
import type { DependencyData } from '../../../data/diagrams/technical'
import type { DiagramSelection } from '../../../lib/diagram-types'
import TechnicalNode from './TechnicalNode.vue'
import TechnicalEdge from './TechnicalEdge.vue'
import TechnicalDefs from './TechnicalDefs.vue'
import { nodeSelection, edgeSelection, route, type Box } from './layout'
import './technical.css'
const props = defineProps<{ data: DependencyData; selectedId?: string }>()
const emit = defineEmits<{ select: [payload: DiagramSelection] }>()
const prefix = useId().replace(/:/g,'')
const width = computed(() => Math.max(1000,props.data.columns.length*460+80))
const rows = computed(() => Math.max(1,...props.data.columns.map(c=>c.nodes.length)))
const height = computed(() => Math.max(680,rows.value*190+220))
const boxes = computed(() => {
  const map:Record<string,Box>={}
  props.data.columns.forEach((column,c)=>column.nodes.forEach((node,r)=>{map[node.id]={x:80+c*460,y:155+r*190+(rows.value-column.nodes.length)*95,w:280,h:132}}))
  return map
})
const edges=computed(()=>props.data.connections.flatMap(edge=>boxes.value[edge.source]&&boxes.value[edge.target]?[{edge,...route(boxes.value[edge.source]!,boxes.value[edge.target]!)}]:[]))
</script>
<template><svg class="dg-svg" data-diagram="dependency" role="img" :aria-label="data.title" :viewBox="`0 0 ${width} ${height}`">
  <TechnicalDefs :prefix="prefix"/>
  <g v-for="(column,i) in data.columns" :key="column.id"><text class="tt-section" :x="220+i*460" y="83" text-anchor="middle">{{ column.label }}</text><line class="tt-divider" :x1="80+i*460" :x2="360+i*460" y1="109" y2="109"/></g>
  <TechnicalEdge v-for="item in edges" :key="item.edge.id" :id="item.edge.id" :path="item.d" :label="item.edge.label" :at="item.label" :prefix="prefix" :dashed="item.edge.kind!=='runtime'" :open="item.edge.kind==='optional'" :selected="selectedId===item.edge.id" @select="emit('select',{...edgeSelection(item.edge,'模块依赖'),details:[`依赖类型：${{runtime:'运行时',build:'构建时',optional:'可选'}[item.edge.kind]}`]})"/>
  <template v-for="column in data.columns" :key="column.id"><TechnicalNode v-for="node in column.nodes" :key="node.id" :node="node" :box="boxes[node.id]!" :footer="node.version" :selected="selectedId===node.id" @select="emit('select',nodeSelection(node,'模块',[`所属分层：${column.label}`,`版本：${node.version||'未指定'}`]))"/></template>
  <g :transform="`translate(80,${height-46})`"><line class="tt-edge-line" x1="0" x2="36" y1="0" y2="0"/><text class="tt-caption" x="48" y="5">运行时依赖</text><line class="tt-edge-line" x1="226" x2="262" y1="0" y2="0" stroke-dasharray="7 6"/><text class="tt-caption" x="274" y="5">构建时 / 可选依赖</text><text class="tt-caption" :x="width-425" y="5">箭头指向被依赖模块</text></g>
</svg></template>
