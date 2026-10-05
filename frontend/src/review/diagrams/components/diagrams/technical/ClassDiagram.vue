<script setup lang="ts">
import {computed,useId} from 'vue'
import type {ClassData} from '../../../data/diagrams/technical'
import type {DiagramSelection} from '../../../lib/diagram-types'
import TechnicalDefs from './TechnicalDefs.vue'
import {nodeSelection,edgeSelection,route,textWidth,type Box} from './layout'
import './technical.css'
const props=defineProps<{data:ClassData;selectedId?:string}>()
const emit=defineEmits<{select:[payload:DiagramSelection]}>()
const prefix=useId().replace(/:/g,'')
const columns=computed(()=>Math.min(3,Math.max(1,props.data.classes.length)))
const cardWidth=computed(()=>Math.max(280,...props.data.classes.flatMap(node=>[node.label,...node.attributes,...node.methods].map(text=>textWidth(text,18)+36))))
const nodeHeight=(node:ClassData['classes'][number])=>76+Math.max(1,node.attributes.length)*23
const cardHeight=computed(()=>Math.max(168,...props.data.classes.map(nodeHeight)))
const width=computed(()=>Math.max(1080,columns.value*(cardWidth.value+130)+10))
const rows=computed(()=>Math.ceil(props.data.classes.length/columns.value))
const baseHeight=computed(()=>Math.max(620,rows.value*(cardHeight.value+86)+96))
const boxes=computed(()=>Object.fromEntries(props.data.classes.map((node,i)=>[node.id,{x:70+i%columns.value*(cardWidth.value+130),y:55+Math.floor(i/columns.value)*(cardHeight.value+86),w:cardWidth.value,h:nodeHeight(node)} satisfies Box])))
const rawRelationships=computed(()=>props.data.relationships.flatMap((edge,i)=>{
 const a=boxes.value[edge.source],b=boxes.value[edge.target]
 if(!a||!b)return []
 if(Math.abs(a.x-b.x)>cardWidth.value+160&&Math.abs(a.y-b.y)<cardHeight.value/2){const sy=a.y+a.h,ty=b.y+b.h,sx=a.x+a.w/2,tx=b.x+b.w/2,lane=Math.max(sy,ty)+61+i*7;return [{edge,d:`M ${sx} ${sy} V ${lane} H ${tx} V ${ty}`,label:{x:(sx+tx)/2,y:lane},sourceLabel:{x:sx+16,y:sy+23,anchor:'start'},targetLabel:{x:tx+16,y:ty+23,anchor:'start'}}]}
 const peers=props.data.relationships.filter(other=>other.source===edge.source&&other.target===edge.target)
 const offset=(peers.findIndex(other=>other.id===edge.id)-(peers.length-1)/2)*44
 const horizontal=Math.abs(a.y-b.y)<cardHeight.value/2
 const shiftedA={...a,x:a.x+(horizontal?0:offset),y:a.y+(horizontal?offset:0)}
 const shiftedB={...b,x:b.x+(horizontal?0:offset),y:b.y+(horizontal?offset:0)}
 const geometry=route(shiftedA,shiftedB,true)
 if(Math.abs(a.y-b.y)<cardHeight.value/2){const right=b.x>a.x;return [{edge,...geometry,sourceLabel:{x:right?a.x+a.w+12:a.x-12,y:a.y+a.h/2-18,anchor:right?'start':'end'},targetLabel:{x:right?b.x-12:b.x+b.w+12,y:b.y+b.h/2-18,anchor:right?'end':'start'}}]}
 const down=b.y>a.y;return [{edge,...geometry,sourceLabel:{x:a.x+a.w/2+16,y:down?a.y+a.h+25:a.y-18,anchor:'start'},targetLabel:{x:b.x+b.w/2+16,y:down?b.y-18:b.y+b.h+25,anchor:'start'}}]
}))
const visibleLabel=(label:string)=>{const text=label.replace(/^字段绑定：/,'绑定 · ');return text.length>30?text.slice(0,28)+'…':text}
const relationships=computed(()=>{
 const occupied:Box[]=[]
 const obstacles=Object.values(boxes.value).map(box=>({x:box.x-12,y:box.y-12,w:box.w+24,h:box.h+24}))
 const overlaps=(a:Box,b:Box)=>a.x<b.x+b.w&&a.x+a.w>b.x&&a.y<b.y+b.h&&a.y+a.h>b.y
 return rawRelationships.value.map(item=>{
  const w=textWidth(visibleLabel(item.edge.label||''),14)+18
  let chosen:Box|undefined
  for(let ring=0;ring<25&&!chosen;ring++)for(const direction of ring?[1,-1]:[1]){
   const x=Math.max(28+w/2,Math.min(width.value-28-w/2,item.label.x))
   const y=item.label.y+ring*35*direction
   const candidate={x:x-w/2-3,y:y-16,w:w+6,h:32}
   if(y<28||y>baseHeight.value-60||[...obstacles,...occupied].some(box=>overlaps(candidate,box)))continue
   chosen=candidate;break
  }
  chosen ||= {x:28,y:baseHeight.value+occupied.filter(box=>box.y>=baseHeight.value).length*35,w:w+6,h:32}
  occupied.push(chosen)
  return {...item,originLabel:item.label,label:{x:chosen.x+chosen.w/2,y:chosen.y+16},labelWidth:w}
 })
})
const height=computed(()=>Math.max(baseHeight.value,...relationships.value.map(item=>item.label.y+70)))
const relationshipName=(kind:ClassData['relationships'][number]['kind'])=>({association:'关联',dependency:'依赖',inheritance:'继承',realization:'接口实现',aggregation:'聚合',composition:'组合'}[kind])
function selectEdge(edge:ClassData['relationships'][number]){emit('select',{...edgeSelection(edge,'契约关系'),details:[`关系类型：${relationshipName(edge.kind)}`]})}
</script>
<template><svg class="dg-svg" data-diagram="class" role="img" :aria-label="data.title" :viewBox="`0 0 ${width} ${height}`">
 <TechnicalDefs :prefix="prefix"/>
 <g v-for="item in relationships" :key="item.edge.id" :class="['tt-edge-graphic',{'tt-selected-edge':selectedId===item.edge.id}]" @click="selectEdge(item.edge)">
  <path :d="item.d" class="tt-edge-hit"/><path :d="item.d" class="tt-edge-line" :stroke-dasharray="item.edge.kind==='dependency'||item.edge.kind==='realization'?'7 6':undefined" :marker-end="['inheritance','realization'].includes(item.edge.kind)?`url(#${prefix}-triangle)`:item.edge.kind==='dependency'?`url(#${prefix}-open)`:undefined" :marker-start="item.edge.kind==='composition'?`url(#${prefix}-diamond)`:item.edge.kind==='aggregation'?`url(#${prefix}-empty-diamond)`:undefined"/>
  <text v-if="item.edge.sourceMultiplicity" class="tt-caption" :x="item.sourceLabel.x" :y="item.sourceLabel.y" :text-anchor="item.sourceLabel.anchor">{{item.edge.sourceMultiplicity}}</text><text v-if="item.edge.targetMultiplicity" class="tt-caption" :x="item.targetLabel.x" :y="item.targetLabel.y" :text-anchor="item.targetLabel.anchor">{{item.edge.targetMultiplicity}}</text>
 </g>
 <path v-for="item in relationships.filter(item=>Math.hypot(item.label.x-item.originLabel.x,item.label.y-item.originLabel.y)>28)" :key="`leader-${item.edge.id}`" :d="`M${item.originLabel.x} ${item.originLabel.y} L${item.label.x} ${item.label.y}`" class="tt-label-leader"/>
 <g v-for="item in relationships" :key="`label-${item.edge.id}`" :data-edge-id="item.edge.id" :transform="`translate(${item.label.x},${item.label.y})`" :class="['tt-edge',{'tt-selected-edge':selectedId===item.edge.id}]" role="button" tabindex="0" :aria-label="item.edge.label||relationshipName(item.edge.kind)" :aria-pressed="selectedId===item.edge.id" @click="selectEdge(item.edge)" @keydown.enter="selectEdge(item.edge)" @keydown.space.prevent="selectEdge(item.edge)"><rect class="tt-label-bg" :x="-item.labelWidth/2" y="-13" :width="item.labelWidth" height="27" rx="6"/><text class="tt-edge-label" y="5" text-anchor="middle">{{visibleLabel(item.edge.label||relationshipName(item.edge.kind))}}</text><title>{{item.edge.label}}</title></g>
 <g v-for="node in data.classes" :key="node.id" :data-node-id="node.id" :transform="`translate(${boxes[node.id]!.x},${boxes[node.id]!.y})`" :class="['tt-node',`tt-${node.tone||'indigo'}`,{'tt-selected':selectedId===node.id}]" role="button" tabindex="0" :aria-label="node.label" :aria-pressed="selectedId===node.id" @click="emit('select',nodeSelection(node,node.stereotype==='interface'?'接口':'类',[...node.attributes,...node.methods]))" @keydown.enter="emit('select',nodeSelection(node,node.stereotype==='interface'?'接口':'类',[...node.attributes,...node.methods]))" @keydown.space.prevent="emit('select',nodeSelection(node,node.stereotype==='interface'?'接口':'类',[...node.attributes,...node.methods]))">
  <rect class="tt-card" :width="cardWidth" :height="boxes[node.id]!.h" rx="12"/><path :d="`M12 0H${cardWidth-12}Q${cardWidth} 0 ${cardWidth} 12V49H0V12Q0 0 12 0Z`" class="tt-icon-bg"/>
  <text v-if="node.stereotype" class="tt-caption" :x="cardWidth/2" y="18" text-anchor="middle">«{{node.stereotype}}»</text><text class="tt-title" :font-style="node.abstract?'italic':undefined" :x="cardWidth/2" :y="node.stereotype?38:32" text-anchor="middle">{{node.label}}</text><line class="tt-divider" x1="0" :x2="cardWidth" y1="49" y2="49"/>
  <text v-for="(attribute,i) in node.attributes" :key="attribute" class="tt-code" x="18" :y="72+i*23">{{attribute}}</text><text v-if="!node.attributes.length" class="tt-caption" x="18" y="72">—</text>
  <text class="tt-caption" x="18" :y="72+Math.max(1,node.attributes.length)*23">完整字段与约束见来源详情</text>
 </g>
 <text class="tt-caption" x="70" :y="height-38">公开 JSON 对象 · 字段与引用由 Schema 派生 · 不表示数据库或类方法</text>
</svg></template>

<style scoped>.tt-code{font-size:18px}.tt-title{font-size:21px}.tt-caption{font-size:13px}.tt-edge-label{font-size:14px}.tt-edge-graphic{cursor:pointer}.tt-label-leader{fill:none;stroke:var(--dg-muted);stroke-width:1;stroke-dasharray:3 5;opacity:.4;pointer-events:none}.tt-edge:hover .tt-label-bg,.tt-edge:focus-visible .tt-label-bg,.tt-selected-edge .tt-label-bg{stroke:var(--dg-accent);stroke-width:2}</style>
