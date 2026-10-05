<script setup lang="ts">
import {computed} from 'vue'
import type {TimelineData,TimelineEvent,DiagramSelection} from '../../../lib/diagram-types'
const props=defineProps<{data:TimelineData;selectedId?:string}>()
const emit=defineEmits<{select:[payload:DiagramSelection]}>()
const day=86400000, cardWidth=284
const formatDate=(date:number)=>new Intl.DateTimeFormat('zh-CN',{year:'numeric',month:'2-digit',day:'2-digit',timeZone:'UTC'}).format(date)
const wrap=(text:string,size:number,available:number)=>{
  const lines:string[]=[];let line='',used=0
  for(const character of text){const w=/[^\u0000-\u007f]/.test(character)?size:size*.56;if(used+w>available&&line){lines.push(line);line='';used=0}line+=character;used+=w}
  if(line)lines.push(line)
  return lines
}
const ordered=computed(()=>props.data.events.map(event=>({event,date:Date.parse(event.date)})).filter(item=>Number.isFinite(item.date)).sort((a,b)=>a.date-b.date))
const width=computed(()=>Math.max(1500,Math.min(4200,ordered.value.length*224+200)))
const domain=computed(()=>{
  const dates=ordered.value.map(item=>item.date),start=dates.length?Math.min(...dates):Date.UTC(2026,9,1),end=dates.length?Math.max(...dates):start+60*day
  return {start,end:Math.max(start+day,end),actualEnd:end,single:dates.length>0&&end===start}
})
const x=(date:number)=>domain.value.single?width.value/2:190+(date-domain.value.start)/(domain.value.end-domain.value.start)*(width.value-380)
const layout=computed(()=>{
  const lanes:{above:number[];below:number[]}={above:[],below:[]}
  const events=ordered.value.map((item,index)=>{
    const side=index%2===0?'above':'below',point=x(item.date),left=Math.max(48,Math.min(width.value-cardWidth-48,point-cardWidth/2)),titles=wrap(item.event.label,21,cardWidth-40),descriptions=wrap(item.event.description||'',18,cardWidth-40)
    const h=92+Math.max(1,titles.length)*28+Math.max(1,descriptions.length)*24
    let lane=lanes[side].findIndex(right=>right+30<=left)
    if(lane<0){lane=lanes[side].length;lanes[side].push(0)}
    lanes[side][lane]=left+cardWidth
    return {...item,side,lane,x:point,left,h,titles,descriptions}
  })
  const tallest=Math.max(176,...events.map(item=>item.h)),above=Math.max(1,lanes.above.length),below=Math.max(1,lanes.below.length),step=tallest+46
  const natural=254+(above+below)*step,height=Math.max(780,natural),axis=138+above*step+(height-natural)/2
  return {events:events.map(item=>({...item,y:item.side==='above'?axis-74-item.h-item.lane*step:axis+82+item.lane*step})),height,axis}
})
const ticks=computed(()=>{
  if(domain.value.single)return [{date:domain.value.start,x:width.value/2,label:new Intl.DateTimeFormat('zh-CN',{month:'numeric',day:'numeric',timeZone:'UTC'}).format(domain.value.start)}]
  const span=(domain.value.end-domain.value.start)/day,step=Math.max(1,Math.ceil(span/Math.max(2,Math.floor((width.value-380)/130)))),list=[]
  for(let date=domain.value.start;date<=domain.value.end;date+=step*day)list.push({date,x:x(date),label:new Intl.DateTimeFormat('zh-CN',{month:'numeric',day:'numeric',timeZone:'UTC'}).format(date)})
  return list
})
const statusLabel=(status:TimelineEvent['status'])=>({complete:'已完成',current:'当前里程碑',planned:'计划中'}[status])
const selection=(event:TimelineEvent):DiagramSelection=>({id:event.id,label:event.label,type:'时间里程碑',description:event.description,details:[`日期：${event.date}`,`状态：${statusLabel(event.status)}`]})
</script>
<template><svg class="dg-svg timeline-svg" data-diagram="timeline" role="img" :aria-label="data.title" :viewBox="`0 0 ${width} ${layout.height}`">
  <rect class="t-canvas" :width="width" :height="layout.height"/>
  <text class="t-summary" x="70" y="62">{{data.events.length}} 个里程碑</text><text v-if="ordered.length" class="t-caption" :x="width-70" y="62" text-anchor="end">{{formatDate(domain.start)}} — {{formatDate(domain.actualEnd)}}</text>
  <line class="t-axis" x1="88" :x2="width-88" :y1="layout.axis" :y2="layout.axis"/>
  <g v-for="tick in ticks" :key="tick.date"><line class="t-tick" :x1="tick.x" :x2="tick.x" :y1="layout.axis+13" :y2="layout.axis+21"/><text class="t-caption" :x="tick.x" :y="layout.axis+46" text-anchor="middle">{{tick.label}}</text></g>
  <g v-for="item in layout.events" :key="item.event.id" class="t-event" :class="[`t-${item.event.status}`,{'is-selected':selectedId===item.event.id}]" role="button" tabindex="0" :aria-label="`${item.event.date} ${item.event.label}`" :aria-pressed="selectedId===item.event.id" @click="emit('select',selection(item.event))" @keydown.enter="emit('select',selection(item.event))" @keydown.space.prevent="emit('select',selection(item.event))">
    <path class="t-connector" :d="item.side==='above'?`M ${item.x} ${layout.axis-13} V ${item.y+item.h+22} H ${item.left+cardWidth/2} V ${item.y+item.h}`:`M ${item.x} ${layout.axis+13} V ${item.y-22} H ${item.left+cardWidth/2} V ${item.y}`"/>
    <circle class="t-point-ring" :cx="item.x" :cy="layout.axis" :r="item.event.status==='current'?17:13"/><circle class="t-point" :cx="item.x" :cy="layout.axis" r="7"/>
    <g :transform="`translate(${item.left},${item.y})`"><rect class="t-card" :width="cardWidth" :height="item.h" rx="14"/><text class="t-date" x="20" y="31">{{formatDate(item.date)}}</text><text v-for="(line,i) in item.titles" :key="`title-${i}`" class="t-title" x="20" :y="65+i*28">{{line}}</text><text v-for="(line,i) in item.descriptions" :key="`description-${i}`" class="t-description" x="20" :y="88+Math.max(0,item.titles.length-1)*28+i*24">{{line}}</text><line class="t-rule" x1="20" :x2="cardWidth-20" :y1="item.h-45" :y2="item.h-45"/><circle class="t-status-dot" cx="25" :cy="item.h-22" r="4"/><text class="t-status" x="39" :y="item.h-16">{{statusLabel(item.event.status)}}</text></g>
  </g>
  <g v-if="!ordered.length"><text class="t-empty" :x="width/2" :y="layout.axis-50" text-anchor="middle">暂无时间事件</text><text class="t-caption" :x="width/2" :y="layout.axis-15" text-anchor="middle">提供事件与真实日期后，时间线将自动展开</text></g>
  <text class="t-caption" x="70" :y="layout.height-42">事件位置对应真实时间间隔 · 相近日期的卡片自动分层</text>
</svg></template>
<style scoped>
.timeline-svg{font-family:Inter,'PingFang SC','Microsoft YaHei',sans-serif}.t-canvas{fill:var(--dg-canvas,#fff)}.t-summary{font-size:25px;font-weight:650;fill:var(--dg-text,#20263e)}.t-caption,.t-date{font-size:18px;fill:var(--dg-muted,#8990a3)}.t-axis{stroke:var(--dg-border,#dde3ef);stroke-width:3;stroke-linecap:round}.t-tick,.t-rule{stroke:var(--dg-border,#dde3ef);stroke-width:1}.t-event{--t-color:var(--dg-muted,#8990a3);cursor:pointer;outline:none}.t-complete{--t-color:var(--dg-success,#47a58b)}.t-current{--t-color:var(--dg-accent,#6265e8)}.t-connector{fill:none;stroke:var(--dg-border,#d9dfeb);stroke-width:1.8;stroke-linecap:round;stroke-linejoin:round}.t-point-ring{fill:var(--dg-canvas,#fff);stroke:var(--t-color);stroke-width:2}.t-point,.t-status-dot{fill:var(--t-color)}.t-planned .t-point{fill:var(--dg-canvas,#fff);stroke:var(--t-color);stroke-width:1.5}.t-card{fill:var(--dg-surface,#fff);stroke:var(--dg-border,#e1e5ee);stroke-width:1.5}.t-title{font-size:21px;font-weight:650;fill:var(--dg-text,#20263e)}.t-description{font-size:18px;fill:var(--dg-muted,#8990a3)}.t-status{font-size:18px;font-weight:500;fill:var(--t-color)}.t-event:hover .t-card,.t-event:focus-visible .t-card,.t-event.is-selected .t-card{stroke:var(--t-color);stroke-width:2.5}.t-event.is-selected .t-connector{stroke:var(--t-color);stroke-width:2.5}.t-empty{font-size:25px;fill:var(--dg-text,#20263e)}
</style>
