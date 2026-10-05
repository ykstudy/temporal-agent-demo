<script setup lang="ts">
import { computed,useId } from 'vue'
import type { LineageData } from '../../../data/diagrams/technical'
import type { DiagramSelection } from '../../../lib/diagram-types'
import TechnicalIcon from './TechnicalIcon.vue'
import TechnicalEdge from './TechnicalEdge.vue'
import TechnicalDefs from './TechnicalDefs.vue'
import { nodeSelection,edgeSelection,route,type Box } from './layout'
import './technical.css'
const props=defineProps<{data:LineageData;selectedId?:string}>()
const emit=defineEmits<{select:[payload:DiagramSelection]}>()
const prefix=useId().replace(/:/g,'')
const width=computed(()=>Math.max(1080,props.data.stages.length*350+40))
const rows=computed(()=>Math.max(1,...props.data.stages.map(s=>s.nodes.length)))
const nodeHeight=(node:LineageData['stages'][number]['nodes'][number])=>Math.max(174,122+node.fields.length*24)
const rowStep=computed(()=>Math.max(240,...props.data.stages.flatMap(stage=>stage.nodes.map(node=>nodeHeight(node)+54))))
const height=computed(()=>Math.max(760,rows.value*rowStep.value+270))
const boxes=computed(()=>{
  const map:Record<string,Box>={}
  props.data.stages.forEach((stage,c)=>stage.nodes.forEach((node,r)=>{map[node.id]={x:65+c*350,y:190+r*rowStep.value+(rows.value-stage.nodes.length)*rowStep.value/2,w:250,h:nodeHeight(node)}}))
  return map
})
const edges=computed(()=>props.data.connections.flatMap(edge=>boxes.value[edge.source]&&boxes.value[edge.target]?[{edge,...route(boxes.value[edge.source]!,boxes.value[edge.target]!)}]:[]))
</script>
<template><svg class="dg-svg" data-diagram="lineage" role="img" :aria-label="data.title" :viewBox="`0 0 ${width} ${height}`">
  <TechnicalDefs :prefix="prefix"/>
  <g v-for="(stage,c) in data.stages" :key="stage.id"><text class="tt-section" :x="190+c*350" y="91" text-anchor="middle">{{ stage.label }}</text><text class="tt-section-sub" :x="190+c*350" y="122" text-anchor="middle">{{ stage.description }}</text><line class="tt-divider" :x1="65+c*350" :x2="315+c*350" y1="146" y2="146"/></g>
  <TechnicalEdge v-for="item in edges" :key="item.edge.id" :id="item.edge.id" :path="item.d" :label="item.edge.label" :at="item.label" :prefix="prefix" :dashed="item.edge.dashed" :selected="selectedId===item.edge.id" @select="emit('select',edgeSelection(item.edge,'血缘关系'))"/>
  <template v-for="stage in data.stages" :key="stage.id"><g v-for="node in stage.nodes" :key="node.id" :transform="`translate(${boxes[node.id]!.x},${boxes[node.id]!.y})`" :class="['tt-node',`tt-${node.tone||'indigo'}`,{'tt-selected':selectedId===node.id}]" role="button" tabindex="0" :aria-label="node.label" :aria-pressed="selectedId===node.id" @click="emit('select',nodeSelection(node,'血缘节点',[`阶段：${stage.label}`,...node.fields]))" @keydown.enter="emit('select',nodeSelection(node,'血缘节点',[`阶段：${stage.label}`,...node.fields]))" @keydown.space.prevent="emit('select',nodeSelection(node,'血缘节点',[`阶段：${stage.label}`,...node.fields]))">
    <rect class="tt-card" width="250" :height="boxes[node.id]!.h" rx="12"/><rect class="tt-icon-bg" x="18" y="17" width="38" height="38" rx="10"/><TechnicalIcon class="tt-icon" :name="node.icon" :size="24" x="25" y="24"/><text class="tt-title" x="69" y="42">{{ node.label }}</text><text class="tt-subtitle" x="18" y="81">{{ node.subtitle }}</text><line class="tt-divider" x1="18" x2="232" y1="99" y2="99"/>
    <text v-for="(field,i) in node.fields" :key="field" class="tt-code" x="18" :y="128+i*24">{{ field }}</text><text v-if="node.operation" class="tt-key" x="230" y="22" text-anchor="end">{{ node.operation }}</text>
  </g></template>
  <text class="tt-caption" x="65" :y="height-45">实线：主血缘路径</text><text class="tt-caption" x="310" :y="height-45">虚线：辅助关联输入</text>
</svg></template>
