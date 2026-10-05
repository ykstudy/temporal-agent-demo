<script setup lang="ts">
import { computed,useId } from 'vue'
import type { RelationshipData,DiagramSelection } from '../../../lib/diagram-types'
import { graphColors } from './layout'
const props=defineProps<{data:RelationshipData;selectedId?:string;mode:'relationship'|'knowledge'}>()
const emit=defineEmits<{select:[value:DiagramSelection]}>();const marker=useId().replace(/:/g,'')+'arrow'
const layout=computed(()=>{
 const count=props.data.nodes.length;const radius=Math.max(265,count*39);const width=Math.max(1160,radius*2.36+410),height=Math.max(760,radius*2+270);const cx=width/2,cy=height/2
 const groups=[...new Set(props.data.nodes.map(n=>n.type))]
 const nodes=props.data.nodes.map((node,index)=>{const angle=-Math.PI/2+Math.PI*2*(index-1)/Math.max(1,count-1);return{node,x:index===0?cx:cx+Math.cos(angle)*radius*1.18,y:index===0?cy:cy+Math.sin(angle)*radius,color:node.color||graphColors[groups.indexOf(node.type)%graphColors.length]!,index}})
 const edges=props.data.edges.map((edge,index)=>{const source=nodes.find(n=>n.node.id===edge.source),target=nodes.find(n=>n.node.id===edge.target);if(!source||!target)return null;const dx=target.x-source.x,dy=target.y-source.y;const length=Math.hypot(dx,dy)||1;const sx=source.x+dx/length*70,sy=source.y+dy/length*70,tx=target.x-dx/length*74,ty=target.y-dy/length*74;const cross=source.index!==0&&target.index!==0;const bend=cross?(index%2?1:-1)*90:0;const mx=(sx+tx)/2-dy/length*bend,my=(sy+ty)/2+dx/length*bend;const offset=cross?(index%2?1:-1)*26:0;return{edge,path:`M ${sx} ${sy} Q ${mx} ${my} ${tx} ${ty}`,lx:(sx+2*mx+tx)/4-dy/length*offset,ly:(sy+2*my+ty)/4+dx/length*offset}}).filter(n=>n!==null)
 return{nodes,edges,width,height,groups}
})
function selectNode(item:typeof layout.value.nodes[number]){emit('select',{id:item.node.id,label:item.node.label,type:item.node.type,description:item.node.description,details:[`关系数量：${props.data.edges.filter(e=>e.source===item.node.id||e.target===item.node.id).length}`]})}
function labelLines(text:string){const lines:string[]=[];let line='',units=0;for(const char of text){const size=/[^\x00-\xff]/.test(char)?2:1;if(units+size>13){lines.push(line);line='';units=0}line+=char;units+=size}if(line)lines.push(line);return lines.length>3?[...lines.slice(0,2),lines[2]!.slice(0,-1)+'…']:lines}
</script>
<template>
<svg class="dg-svg" :data-diagram="mode" role="img" :aria-label="data.title" :viewBox="`0 0 ${layout.width} ${layout.height}`"><title>{{data.title}}</title>
 <defs><marker :id="marker" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="7" markerHeight="7" orient="auto-start-reverse"><path d="M 0 0 L 10 5 L 0 10" fill="var(--dg-muted)"/></marker></defs>
 <g v-for="item in layout.edges" :key="item.edge.id" tabindex="0" role="button" :aria-label="item.edge.label" class="relation-edge" @click="emit('select',{id:item.edge.id,label:item.edge.label,type:'关系',details:[`${data.nodes.find(n=>n.id===item.edge.source)?.label} → ${data.nodes.find(n=>n.id===item.edge.target)?.label}`]})" @keydown.enter="emit('select',{id:item.edge.id,label:item.edge.label,type:'关系'})">
  <path :d="item.path" stroke="var(--dg-border-strong, #bbc1d4)" stroke-width="2" fill="none" :marker-end="mode==='knowledge'?`url(#${marker})`:undefined"/>
  <rect :x="item.lx-item.edge.label.length*8-9" :y="item.ly-15" :width="item.edge.label.length*16+18" height="30" rx="7" fill="var(--dg-canvas)"/>
  <text :x="item.lx" :y="item.ly+6" text-anchor="middle" fill="var(--dg-muted)" font-size="16">{{item.edge.label}}</text>
 </g>
 <g v-for="item in layout.nodes" :key="item.node.id" tabindex="0" role="button" :aria-label="item.node.label" class="relation-node" @click="selectNode(item)" @keydown.enter="selectNode(item)">
  <circle :cx="item.x" :cy="item.y" :r="item.index===0?73:65" :fill="`color-mix(in srgb, ${item.color} 9%, var(--dg-surface))`" :stroke="selectedId===item.node.id?'var(--dg-text)':item.color" :stroke-width="selectedId===item.node.id?3:1.7"/>
  <circle :cx="item.x" :cy="item.y-30" r="5" :fill="item.color"/>
  <text :x="item.x" :y="item.y-12" text-anchor="middle" font-size="18" fill="var(--dg-text)" font-weight="600"><tspan v-for="(line,index) in labelLines(item.node.label)" :key="index" :x="item.x" :dy="index?20:0">{{line}}</tspan></text>
  <text :x="item.x" :y="item.y+51" text-anchor="middle" font-size="13" fill="var(--dg-muted)">{{item.node.type}}</text>
 </g>
 <g v-for="(group,index) in layout.groups" :key="group" :transform="`translate(${50+index*150},${layout.height-32})`"><circle r="5" :fill="graphColors[index%graphColors.length]"/><text x="14" y="5" font-size="16" fill="var(--dg-muted)">{{group}}</text></g>
 <text v-if="!data.nodes.length" :x="layout.width/2" :y="layout.height/2" fill="var(--dg-muted)" text-anchor="middle" font-size="22">没有符合筛选条件的实体</text>
</svg>
</template>
<style scoped>.relation-node,.relation-edge{cursor:pointer;outline:none}.relation-node:hover circle:first-child,.relation-node:focus-visible circle:first-child{stroke:var(--dg-accent);stroke-width:3}.relation-edge:hover path,.relation-edge:focus-visible path{stroke:var(--dg-accent);stroke-width:3}</style>
