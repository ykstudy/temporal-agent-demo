<script setup lang="ts">
import {computed,useId} from 'vue'
import type {DataFlowData} from '../../../data/diagrams/technical'
import type {DiagramSelection} from '../../../lib/diagram-types'
import TechnicalEdge from './TechnicalEdge.vue'
import TechnicalDefs from './TechnicalDefs.vue'
import {nodeSelection,edgeSelection,route,type Box} from './layout'
import './technical.css'
const props=defineProps<{data:DataFlowData;selectedId?:string}>()
const emit=defineEmits<{select:[payload:DiagramSelection]}>()
const prefix=useId().replace(/:/g,'')
const groups=computed(()=>({external:props.data.nodes.filter(n=>n.kind==='external'),process:props.data.nodes.filter(n=>n.kind==='process'),store:props.data.nodes.filter(n=>n.kind==='store')}))
const rows=computed(()=>Math.max(1,groups.value.external.length,groups.value.process.length,groups.value.store.length))
const height=computed(()=>Math.max(660,rows.value*226+180))
const boxes=computed(()=>{
 const map:Record<string,Box>={}
 for(const kind of ['external','process','store'] as const){const items=groups.value[kind];items.forEach((node,i)=>{
   const midpoint=175+(i+.5)*(height.value-275)/Math.max(1,items.length)
   const w=kind==='process'?164:kind==='store'?262:232,h=kind==='process'?164:112,x=kind==='external'?86:kind==='store'?1054:618
   map[node.id]={x,y:midpoint-h/2,w,h}
 })}
 return map
})
const edges=computed(()=>props.data.connections.flatMap(edge=>boxes.value[edge.source]&&boxes.value[edge.target]?[{edge,...route(boxes.value[edge.source]!,boxes.value[edge.target]!)}]:[]))
const kindNames={external:'外部实体',process:'数据处理',store:'数据存储'}
</script>
<template><svg class="dg-svg" data-diagram="dataflow" role="img" :aria-label="data.title" :viewBox="`0 0 1400 ${height}`">
 <TechnicalDefs :prefix="prefix"/>
 <text class="tt-section" x="202" y="84" text-anchor="middle">外部实体</text><text class="tt-section" x="700" y="84" text-anchor="middle">数据处理</text><text class="tt-section" x="1185" y="84" text-anchor="middle">数据存储</text><line class="tt-divider" x1="80" x2="1320" y1="112" y2="112"/>
 <TechnicalEdge v-for="item in edges" :key="item.edge.id" :id="item.edge.id" :path="item.d" :label="item.edge.label" :at="item.label" :prefix="prefix" :selected="selectedId===item.edge.id" @select="emit('select',edgeSelection(item.edge,'数据流'))"/>
 <g v-for="node in data.nodes" :key="node.id" :transform="`translate(${boxes[node.id]!.x},${boxes[node.id]!.y})`" :class="['tt-node',`tt-${node.tone||'indigo'}`,{'tt-selected':selectedId===node.id}]" role="button" tabindex="0" :aria-label="node.label" :aria-pressed="selectedId===node.id" @click="emit('select',nodeSelection(node,kindNames[node.kind],[node.number?`编号：${node.number}`:kindNames[node.kind]]))" @keydown.enter="emit('select',nodeSelection(node,kindNames[node.kind],[node.number?`编号：${node.number}`:kindNames[node.kind]]))" @keydown.space.prevent="emit('select',nodeSelection(node,kindNames[node.kind],[node.number?`编号：${node.number}`:kindNames[node.kind]]))">
  <template v-if="node.kind==='process'"><circle class="tt-card" cx="82" cy="82" r="82"/><text class="tt-footer" x="82" y="48" text-anchor="middle">{{node.number}}</text><text class="tt-title" x="82" y="84" text-anchor="middle">{{node.label}}</text><text class="tt-subtitle" x="82" y="117" text-anchor="middle">{{node.subtitle}}</text></template>
  <template v-else-if="node.kind==='store'"><rect x="0" y="0" width="262" height="112" class="tt-icon-bg" opacity=".45"/><path class="tt-card" d="M262 0H0V112H262M52 0V112" fill="none"/><text class="tt-footer" x="26" y="62" text-anchor="middle">{{node.number}}</text><text class="tt-title" x="72" y="44">{{node.label}}</text><text class="tt-subtitle" x="72" y="79">{{node.subtitle}}</text></template>
  <template v-else><rect class="tt-card" width="232" height="112" rx="2"/><text class="tt-title" x="116" y="48" text-anchor="middle">{{node.label}}</text><text class="tt-subtitle" x="116" y="79" text-anchor="middle">{{node.subtitle}}</text></template>
 </g>
 <text class="tt-caption" x="86" :y="height-32">矩形：外部实体　　圆：数据处理　　开放存储符号：数据存储　　箭头：命名的数据流</text>
</svg></template>
