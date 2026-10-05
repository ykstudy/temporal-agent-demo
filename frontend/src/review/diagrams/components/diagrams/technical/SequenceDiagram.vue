<script setup lang="ts">
import { computed,useId } from 'vue'
import type { SequenceData } from '../../../data/diagrams/technical'
import type { DiagramSelection } from '../../../lib/diagram-types'
import TechnicalIcon from './TechnicalIcon.vue'
import TechnicalDefs from './TechnicalDefs.vue'
import { nodeSelection,textWidth } from './layout'
import './technical.css'
const props=defineProps<{data:SequenceData;selectedId?:string}>()
const emit=defineEmits<{select:[payload:DiagramSelection]}>()
const prefix=useId().replace(/:/g,'')
const width=computed(()=>Math.max(1400,props.data.participants.length*260+100))
const participantX=computed(()=>Object.fromEntries(props.data.participants.map((participant,i)=>[participant.id,props.data.participants.length===1?width.value/2:140+i*(width.value-280)/Math.max(1,props.data.participants.length-1)])))
const messageY=(index:number)=>180+index*68+(props.data.notes||[]).filter(note=>note.after<index).length*68
const height=computed(()=>Math.max(540,messageY(Math.max(0,props.data.messages.length-1))+148+(props.data.notes||[]).filter(n=>n.after>=props.data.messages.length-1).length*68))
const active=(participant:string,index:number)=>props.data.activations.some(activation=>activation.participant===participant&&activation.start<=index&&activation.end>=index)
const messages=computed(()=>props.data.messages.flatMap((message,i)=>{
 const from=participantX.value[message.from],to=participantX.value[message.to]
 if(from===undefined||to===undefined)return []
 const y=messageY(i),right=to>=from,sx=from+(active(message.from,i)?right?8:-8:0),tx=to+(active(message.to,i)?right?-8:8:0)
 return [{message,index:i,y,d:from===to?`M ${sx} ${y} h 65 v 27 H ${tx}`:`M ${sx} ${y} H ${tx}`,labelX:from===to?from+100:(from+to)/2,labelY:from===to?y+15:y-14}]
}))
const messageSelection=(message:SequenceData['messages'][number],index:number):DiagramSelection=>({id:message.id,label:message.label,type:{sync:'同步调用',async:'异步消息',return:'返回消息'}[message.kind],description:message.description,details:[`顺序：${index+1}`,`发送方：${props.data.participants.find(p=>p.id===message.from)?.label||message.from}`,`接收方：${props.data.participants.find(p=>p.id===message.to)?.label||message.to}`]})
</script>
<template><svg class="dg-svg" data-diagram="sequence" role="img" :aria-label="data.title" :viewBox="`0 0 ${width} ${height}`">
 <TechnicalDefs :prefix="prefix"/>
 <g v-for="participant in data.participants" :key="participant.id"><line class="tt-lifeline" :x1="participantX[participant.id]" :x2="participantX[participant.id]" y1="129" :y2="height-104"/></g>
 <g v-for="(activation,i) in data.activations" :key="`${activation.participant}-${i}`"><rect v-if="participantX[activation.participant]!==undefined" class="tt-activation" :x="participantX[activation.participant]!-8" :y="messageY(activation.start)-12" width="16" :height="Math.max(30,messageY(activation.end)-messageY(activation.start)+30)" rx="3"/></g>
 <g v-for="participant in data.participants" :key="participant.id" :class="['tt-node',`tt-${participant.tone||'indigo'}`,{'tt-selected':selectedId===participant.id}]" role="button" tabindex="0" :aria-label="participant.label" :aria-pressed="selectedId===participant.id" @click="emit('select',nodeSelection(participant,'交互参与者',[`类型：${participant.kind}`]))" @keydown.enter="emit('select',nodeSelection(participant,'交互参与者',[`类型：${participant.kind}`]))" @keydown.space.prevent="emit('select',nodeSelection(participant,'交互参与者',[`类型：${participant.kind}`]))">
   <rect class="tt-card" :x="participantX[participant.id]!-98" y="49" width="196" height="75" rx="12"/><TechnicalIcon class="tt-icon" :name="participant.kind==='actor'?'person':participant.kind==='database'?'database':'server'" :size="24" :x="participantX[participant.id]!-78" y="75"/><text class="tt-title" :x="participantX[participant.id]!-41" y="92">{{ participant.label }}</text>
   <rect class="tt-card" :x="participantX[participant.id]!-98" :y="height-100" width="196" height="49" rx="10"/><text class="tt-subtitle" :x="participantX[participant.id]" :y="height-69" text-anchor="middle">{{ participant.label }}</text>
 </g>
 <g v-for="item in messages" :key="item.message.id" :class="['tt-edge',{'tt-selected-edge':selectedId===item.message.id}]" role="button" tabindex="0" :aria-label="item.message.label" :aria-pressed="selectedId===item.message.id" @click="emit('select',messageSelection(item.message,item.index))" @keydown.enter="emit('select',messageSelection(item.message,item.index))" @keydown.space.prevent="emit('select',messageSelection(item.message,item.index))">
  <path :d="item.d" class="tt-edge-hit"/><path :d="item.d" class="tt-edge-line" :stroke-dasharray="item.message.kind==='return'?'7 6':undefined" :marker-end="`url(#${prefix}-${item.message.kind==='sync'?'arrow':'open'})`"/>
  <rect class="tt-label-bg" :x="item.labelX-textWidth(item.message.label,20)/2-5" :y="item.labelY-20" :width="textWidth(item.message.label,20)+10" height="28" rx="5"/><text class="tt-edge-label tt-message-label" :x="item.labelX" :y="item.labelY" text-anchor="middle">{{ item.message.label }}</text><text class="tt-caption" x="31" :y="item.y+5">{{ String(item.index+1).padStart(2,'0') }}</text>
 </g>
 <g v-for="(note,i) in data.notes||[]" :key="i"><rect class="tt-note-bg" :x="Math.min(participantX[note.from]||140,participantX[note.to]||width-140)-76" :y="messageY(note.after)+24" :width="Math.abs((participantX[note.from]||140)-(participantX[note.to]||width-140))+152" height="44" rx="8"/><text class="tt-caption tt-note-text" :x="((participantX[note.from]||140)+(participantX[note.to]||width-140))/2" :y="messageY(note.after)+52" text-anchor="middle">{{ note.label }}</text></g>
 <text class="tt-caption" x="42" :y="height-20">实心箭头：同步调用　　空心箭头：异步消息　　虚线：返回　　窄条：执行激活</text>
</svg></template>
<style scoped>.tt-message-label{font-size:20px}.tt-note-bg{fill:color-mix(in srgb,var(--dg-warning,#d5a34d) 16%,var(--dg-surface,#fff));stroke:var(--dg-warning,#d5a34d);stroke-opacity:.45}.tt-note-text{fill:var(--dg-text,#20263e)}</style>
