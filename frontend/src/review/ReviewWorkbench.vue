<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import { ArrowLeft, Box, CheckSquare, FileCode2, Download, GitBranch, Layers, ListChecks, Maximize, Menu, Minus, Moon, Network, PanelRightClose, PanelRightOpen, Plus, Search, ShieldCheck, Sun, Workflow, X } from 'lucide-vue-next'
import DiagramRenderer from './DiagramRenderer.vue'
import { loadReview } from './load'
import { exportReview } from './export'
import type { ReviewBundle, Source } from './types'
import type { ClassData } from './diagrams/data/diagrams/technical'
import type { RelationshipData, DiagramSelection } from './diagrams/lib/diagram-types'
import './diagrams/diagram.css'
import './review.css'

const bundle = ref<ReviewBundle>(), error = ref(''), notification = ref(''), search = ref('')
const requested = new URLSearchParams(location.search).get('review') || 'contracts'
const current = ref(requested), selected = ref<DiagramSelection>(), source = ref<Source>()
const theme = ref(localStorage.getItem('agent-review-theme') === 'dark' ? 'dark' : 'light')
const showInspector = ref(innerWidth > 1150), showNavigation = ref(false), stage = ref<HTMLElement>(), zoom = ref(1), pan = ref({ x: 0, y: 0 }), exporting = ref(false)
const domain = ref('domain.agent'), entityType = ref('feature'), relationType = ref('all'), allContracts = ref(false)
const contractSearch = ref('')
const icons = [ShieldCheck, Workflow, GitBranch, Box, FileCode2, Layers, CheckSquare, GitBranch, Network, Box, ListChecks]
const names: Record<string,string> = { state:'状态与门禁', responsibility:'职责与权限', sequence:'交互时序', contracts:'契约对象', lineage:'来源与证据', architecture:'模块边界', coverage:'验收覆盖', versions:'身份与版本', knowledge:'文档关系', skills:'Skill 能力', context:'上下文与摘要' }
const model = computed(() => bundle.value?.models.find(item => item.id === current.value))
const unknown = ref(false)
const models = computed(() => bundle.value?.models.filter(item => current.value === 'knowledge' || `${names[item.id]} ${item.title} ${item.description}`.toLowerCase().includes(search.value.toLowerCase())) || [])
const knowledge = computed(() => bundle.value?.models.find(item => item.id === 'knowledge')?.data as unknown as RelationshipData | undefined)
const domains = computed(() => knowledge.value?.nodes.filter(node => node.type === 'domain') || [])
const predicates = computed(() => [...new Set(knowledge.value?.edges.map(edge => edge.label) || [])])
const binding = computed(() => selected.value && model.value?.bindings[selected.value.id])
const sourceIDs = computed(() => binding.value?.source_ids || model.value?.source_ids || [])
const sources = computed(() => bundle.value?.sources.filter(item => sourceIDs.value.includes(item.id)) || [])
const filteredData = computed(() => {
  const active = model.value
  if (!active) return {}
  if (active.id === 'contracts') {
    const data = active.data as unknown as ClassData
    const core = ['DefinitionSnapshot','DecisionProposal','ActionValidation','Proposal','Approval','ExecutionRequest','ArtifactManifest','VerificationReport']
    const classes = data.classes.filter(item => (allContracts.value || core.includes(item.label)) && item.label.toLowerCase().includes(contractSearch.value.toLowerCase()))
      .map(item => ({ ...item, attributes: item.attributes.slice(0,4).map(text=>text.split(':')[0]!.slice(0,28)), methods: [] }))
    if(!allContracts.value)classes.sort((a,b)=>core.indexOf(a.label)-core.indexOf(b.label))
    const ids = new Set(classes.map(item => item.id)), labels = new Map(data.classes.map(item=>[item.id,item.label]))
    const chain = new Set(['DecisionProposal:DefinitionSnapshot','ActionValidation:DecisionProposal','ExecutionRequest:ActionValidation','ExecutionRequest:Approval','Approval:Proposal','ArtifactManifest:Proposal','VerificationReport:ArtifactManifest'])
    return { ...data, classes, relationships: data.relationships.filter(edge => ids.has(edge.source) && ids.has(edge.target) && (allContracts.value || (edge.id.startsWith('semantic_binding.') && chain.has(`${labels.get(edge.source)}:${labels.get(edge.target)}`)))) }
  }
  if (active.id === 'knowledge') {
    const data = active.data as unknown as RelationshipData
    const members = new Set(data.edges.filter(edge => edge.label === 'belongs_to' && edge.target === domain.value).map(edge => edge.source))
    members.add(domain.value)
    // Domain-scoped ADR/INV are explicit registered edges to that domain or its features.
    for (const edge of data.edges) if (members.has(edge.target) && ['decides','constrains'].includes(edge.label)) members.add(edge.source)
    const nodes = data.nodes.filter(node => (domain.value === 'all' || members.has(node.id)) && (entityType.value === 'all' || node.type === entityType.value || node.id === domain.value) && (!search.value || `${node.label} ${node.description||''}`.toLowerCase().includes(search.value.toLowerCase())))
    const ids = new Set(nodes.map(node => node.id))
    return { ...data, nodes, edges: data.edges.filter(edge => ids.has(edge.source) && ids.has(edge.target) && (relationType.value === 'all' || edge.label === relationType.value)) }
  }
  return active.data
})
const visibleCount = computed(() => {
  const data = filteredData.value as Record<string, unknown>
  const full = model.value?.data || {}
  const key = model.value?.id === 'contracts' ? 'classes' : model.value?.id === 'knowledge' ? 'nodes' : ''
  return key ? `${(data[key] as unknown[]).length} / ${(full[key] as unknown[]).length} 个对象` : ''
})
let drag: { x: number; y: number; ox: number; oy: number } | null = null
let moved = false, timer: ReturnType<typeof setTimeout> | undefined
let sourceOrigin: HTMLElement | null = null
function fit() { zoom.value = 1; pan.value = { x: 0, y: 0 } }
function resize(value: number) { zoom.value = Math.max(.25, Math.min(12, value)) }
function select(value: DiagramSelection) { if (moved) return; selected.value = value; showInspector.value = true }
function change(id: string) { current.value = id; unknown.value=false;selected.value = undefined; source.value = undefined; showNavigation.value = false; fit(); const url = new URL(location.href); url.searchParams.set('review', id); history.replaceState(null,'',url) }
function pointerDown(event: PointerEvent) { if (event.button !== 0 || (event.target as Element).closest('button,[role="button"]')) return; drag = { x:event.clientX, y:event.clientY, ox:pan.value.x, oy:pan.value.y }; moved = false;stage.value?.setPointerCapture(event.pointerId) }
function pointerMove(event: PointerEvent) { if(!event.buttons){drag=null;return}if (!drag) return; if (Math.hypot(event.clientX-drag.x,event.clientY-drag.y)>4) { moved = true; pan.value={x:drag.ox+event.clientX-drag.x,y:drag.oy+event.clientY-drag.y} } }
function clearDrag(){drag=null}
function pointerUp(event: PointerEvent) { if (stage.value?.hasPointerCapture(event.pointerId)) stage.value.releasePointerCapture(event.pointerId); drag=null; if(moved)setTimeout(()=>moved=false,0) }
function key(event: KeyboardEvent) {
  if(event.key==='Escape') {
    if(source.value){source.value=undefined;return}
    selected.value=undefined;showNavigation.value=false
    if(innerWidth<=1150)showInspector.value=false
  }
  if(event.key==='Tab'&&source.value) {
    const controls=Array.from(document.querySelectorAll<HTMLElement>('.review-source-dialog button, .review-source-dialog [tabindex="0"]'))
    const first=controls[0],last=controls[controls.length-1]
    if(event.shiftKey&&document.activeElement===first){event.preventDefault();last?.focus()}
    else if(!event.shiftKey&&document.activeElement===last){event.preventDefault();first?.focus()}
  }
}
function toggleTheme() { theme.value = theme.value === 'light' ? 'dark' : 'light'; localStorage.setItem('agent-review-theme',theme.value) }
async function markCanvas() { await nextTick(); stage.value?.querySelector('svg.dg-svg')?.setAttribute('data-review-canvas','') }
async function exportFile(format: 'svg' | 'png') {
  const svg = stage.value?.querySelector<SVGSVGElement>('svg.dg-svg'); if(!svg || !model.value || !bundle.value) return
  exporting.value=true; error.value=''
  try { await exportReview(svg,format,model.value,bundle.value,{search:search.value,domain:domain.value,entity_type:entityType.value,relation_type:relationType.value,all_contracts:allContracts.value,contract_search:contractSearch.value}); notification.value=`已导出完整 ${format.toUpperCase()} 图表（当前筛选）`; if(timer)clearTimeout(timer);timer=setTimeout(()=>notification.value='',4000) }
  catch(err){ error.value=err instanceof Error?err.message:String(err) } finally { exporting.value=false }
}
watch(filteredData,()=>{selected.value=undefined;fit();void markCanvas();if(model.value)document.title=`${model.value.title} · 设计审查`},{flush:'post'})
watch(source,async(value,previous)=>{
  if(value&&!previous){sourceOrigin=document.activeElement instanceof HTMLElement?document.activeElement:null;await nextTick();document.querySelector<HTMLElement>('.review-source-dialog button')?.focus()}
  else if(!value&&previous){await nextTick();sourceOrigin?.focus();sourceOrigin=null}
})
onMounted(async()=>{
  window.addEventListener('keydown',key)
  window.addEventListener('blur',clearDrag)
  try { bundle.value=await loadReview();if(!model.value){unknown.value=true;current.value='contracts'}await markCanvas() }
  catch(err){error.value=err instanceof Error?err.message:String(err)}
})
onUnmounted(()=>{window.removeEventListener('keydown',key);window.removeEventListener('blur',clearDrag);if(timer)clearTimeout(timer)})
</script>

<template>
 <div class="review-workbench flowlens-diagrams" :data-theme="theme">
  <header class="review-header"><a class="review-brand" href="?review"><Network/><strong>Agent Studio</strong><span>/</span><b>设计审查</b></a><div class="review-actions"><a href="?" class="review-return" aria-label="返回工作区" title="返回工作区"><ArrowLeft/><span>返回工作区</span></a><button class="review-icon" aria-label="切换主题" @click="toggleTheme"><Sun v-if="theme==='light'"/><Moon v-else/></button><button class="review-secondary" :disabled="exporting || !model" @click="exportFile('svg')"><FileCode2/>导出 SVG</button><button class="review-primary" :disabled="exporting || !model" @click="exportFile('png')"><Download/>{{exporting?'正在导出…':'导出 PNG'}}</button></div></header>
  <button v-if="showNavigation" class="review-scrim" aria-label="关闭审查目录" @click="showNavigation=false"/>
  <div class="review-body" :class="{'no-inspector':!showInspector}">
   <nav class="review-navigation" :class="{open:showNavigation}" aria-label="设计审查"><label class="review-search"><Search/><input v-model="search" aria-label="搜索审查项" placeholder="搜索审查项"/></label><a v-for="(item,index) in models" :key="item.id" :href="`?review=${item.id}`" :aria-current="item.id===current?'page':undefined" @click.prevent="change(item.id)"><component :is="icons[index % icons.length]"/><span>{{names[item.id]}}</span></a><p v-if="!models.length" class="review-empty">没有匹配的审查项。</p><footer>只读规范与证据<br/>运行批准在项目工作区进行。</footer></nav>
   <main class="review-main">
    <div class="review-heading"><button class="review-icon review-mobile-nav" aria-label="打开审查目录" @click="showNavigation=true"><Menu/></button><div><h1>{{model?.title||'设计审查'}}</h1><p>{{model?.description||'正在校验发布结构与来源摘要…'}}</p></div></div>
    <p v-if="unknown" class="review-notice" role="status">未知审查视图，已返回契约对象；请从目录选择已发布类型。</p>
    <p v-if="error" class="review-error" role="alert">{{error}}</p>
    <div v-if="model?.id==='knowledge'" class="review-filters"><label>领域<select v-model="domain" aria-label="筛选领域"><option value="all">全部领域</option><option v-for="item in domains" :key="item.id" :value="item.id">{{item.label}}</option></select></label><label>实体<select v-model="entityType" aria-label="筛选实体类型"><option value="all">全部类型</option><option value="feature">功能</option><option value="domain">领域</option><option value="adr">ADR</option><option value="invariant">不变量</option></select></label><label>关系<select v-model="relationType" aria-label="筛选关系类型"><option value="all">全部关系</option><option v-for="item in predicates" :key="item" :value="item">{{item}}</option></select></label><span>{{visibleCount}}</span></div>
    <div v-if="model?.id==='contracts'" class="review-filters"><label class="review-inline-check"><input v-model="allContracts" type="checkbox"/>全部 Schema 对象</label><label>查找对象<input v-model="contractSearch" aria-label="搜索契约对象" placeholder="Definition / Message…"/></label><span>{{visibleCount}} · {{allContracts?'完整引用与字段绑定':'核心字段绑定链'}} · 字段名称列前四项，详情保留完整 Schema</span></div>
    <section class="review-sheet" aria-label="图表画布">
     <div ref="stage" class="review-stage" data-testid="review-canvas" @wheel.prevent="resize(zoom*($event.deltaY<0?1.08:1/1.08))" @pointerdown="pointerDown" @pointermove="pointerMove" @pointerup="pointerUp" @pointercancel="pointerUp" @lostpointercapture="clearDrag" @click.capture="moved && $event.stopPropagation()"><div v-if="model" class="review-transform" :style="{transform:`translate(${pan.x}px,${pan.y}px) scale(${zoom})`}"><DiagramRenderer :key="model.id" :type="model.diagram_type" :data="filteredData" :selected-id="selected?.id" @select="select"/></div><p v-else class="review-loading">{{error?'发布数据未通过校验。':'正在读取当前文档模型…'}}</p></div>
     <footer class="review-toolbar"><span>{{model?.scope==='design'?'规范目标':model?.scope}} · contract {{bundle?.baseline.contract_version}}<small>{{model?.status}}</small></span><div><button class="review-icon" aria-label="缩小" @click="resize(zoom/1.2)"><Minus/></button><output aria-label="画布缩放比例">{{Math.round(zoom*100)}}%</output><button class="review-icon" aria-label="放大" @click="resize(zoom*1.2)"><Plus/></button><button class="review-secondary" @click="fit"><Maximize/><span>适应画布</span></button><button class="review-icon" :aria-label="showInspector?'隐藏说明':'显示说明'" @click="showInspector=!showInspector"><PanelRightClose v-if="showInspector"/><PanelRightOpen v-else/></button></div></footer>
    </section>
   </main>
   <aside v-if="showInspector&&model" class="review-inspector" role="region" aria-label="来源详情"><button class="review-icon review-close-inspector" aria-label="关闭审查说明" @click="showInspector=false"><X/></button><div class="review-inspector-tabs"><b>审查说明</b><span>选中元素</span><span>来源文档</span></div><section><h2>{{selected?'选中元素':'审查说明'}}</h2><h3 v-if="selected">{{binding?.label||selected.label}}</h3><p>{{binding?.description||selected?.description||model.description}}</p><ul><li v-for="item in binding?.details||selected?.details||model.semantics" :key="item">{{item}}</li></ul><small>{{binding?.scope||model.scope}} · {{binding?.status||model.status}}</small></section><section class="review-source-list"><h2>来源文档</h2><button v-for="item in sources" :key="item.id" @click="source=item"><FileCode2/><span>{{item.path}}</span></button><p class="review-source-hint">来源正文与原字节摘要已校验。选择节点或关系可收窄来源。</p></section></aside>
  </div>
  <div v-if="notification" class="review-toast" role="status">{{notification}}</div>
  <div v-if="source" class="review-source-backdrop" @click.self="source=undefined"><section role="dialog" aria-modal="true" aria-label="来源详情" class="review-source-dialog"><header><div><h2>来源详情</h2><p>{{source.path}}</p></div><button class="review-icon" aria-label="关闭来源详情" @click="source=undefined"><X/></button></header><p class="review-digest">SHA-256 {{source.sha256}}</p><pre tabindex="0" aria-label="来源正文">{{source.text}}</pre></section></div>
 </div>
</template>
