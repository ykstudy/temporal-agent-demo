<script setup lang="ts">
import { onBeforeUnmount, ref, shallowRef, watch } from 'vue'
import type { RunSnapshot } from '../contracts/types.generated'
import type { RuntimeView, RuntimeHealth, RuntimeAuditTrail } from '../contracts/runtime.generated'
import { canonical, checkHash, sha256, strictJSON, validate } from '../contracts/runtime'
import { json } from './api'
const props = defineProps<{ run: RunSnapshot }>()
const view = shallowRef<RuntimeView | null>(null), health = shallowRef<RuntimeHealth | null>(null), audit = shallowRef<RuntimeAuditTrail | null>(null)
const error = ref(''), unavailable = ref(false), loading = ref(false)
let alive = true, request = 0
const status: Record<string,string> = { available:'可查询', stale:'检查点滞后', unavailable:'不可用', not_applicable:'不适用', fresh:'近期上报', running:'执行中', succeeded:'调用完成', failed:'调用失败', unknown:'结果未知', ready:'就绪', not_ready:'未就绪', configured_unverified:'已配置，尚未验证连接', complete:'完整采集' }
const labels: Record<string,string> = { api:'接口', storage:'持久存储', temporal:'Temporal 连接', worker:'当前 Worker', toolchain:'Go 工具链', sandbox:'进程沙箱', model_configuration:'模型配置', gitlab_configuration:'GitLab 配置' }
const date = (s: string | null) => s ? new Date(s).toLocaleString() : '没有记录'
async function refresh() {
 const id=props.run.run_id, epoch=++request; loading.value=true
 try {
  const [raw, hr] = await Promise.all([json('/api/v2/runs/'+id+'/runtime'), fetch('/api/runtime/ready')])
  if (![200,503].includes(hr.status)) throw Error('就绪状态暂不可读取')
  const h=validate<RuntimeHealth>('RuntimeHealth', strictJSON(await hr.text())), v=validate<RuntimeView>('RuntimeView',raw)
  await Promise.all([checkHash(h,'health_hash'),checkHash(v,'view_hash')])
  if (v.run_id!==id || v.run_ref.object_id!==id || v.run_ref.object_type!=='run' || v.activity_attempts.some(a=>a.run_id!==id)) throw Error('运行诊断所属 Run 不同')
  if (new Set(h.checks.map(c=>c.name)).size!==8 || hr.status===200&&!h.ready || hr.status===503&&h.ready) throw Error('就绪响应与检查结果不一致')
  if (v.audit_coverage==='unavailable' && (v.metrics.availability!=='unavailable' || v.activity_attempts.length)) throw Error('缺历史来源时不能补造指标')
  if (!alive || request!==epoch || props.run.run_id!==id) return
  view.value=v;health.value=h;error.value='';unavailable.value=false
 } catch(e) {if(alive&&request===epoch){error.value=e instanceof Error?e.message:'运行诊断暂不可用';unavailable.value=true}}
 finally {if(alive&&request===epoch)loading.value=false}
}
async function readAudit() {
 const id=props.run.run_id
 try {
  const a=validate<RuntimeAuditTrail>('RuntimeAuditTrail',await json('/api/v2/runs/'+id+'/audit'));await checkHash(a,'audit_hash')
  if(a.run_id!==id||a.run_ref.object_id!==id)throw Error('审计所属 Run 错误')
  let prior:string|null=null
  for(let i=0;i<a.events.length;i++){const e=a.events[i];await checkHash(e,'event_hash');if(e.sequence!==i+1||e.run_id!==id||e.previous_hash!==prior||e.parameters.summary_hash!==await sha256(canonical({action:e.parameters.action,argument_keys:e.parameters.argument_keys})))throw Error('审计链或脱敏摘要不一致');prior=e.event_hash}
  if(a.head_hash!==prior)throw Error('审计末端不一致')
  if(alive&&props.run.run_id===id)audit.value=a
 }catch(e){if(alive&&props.run.run_id===id)error.value=e instanceof Error?e.message:'审计读取失败'}
}
watch(()=>props.run.run_id,()=>{request++;view.value=null;health.value=null;audit.value=null;error.value='';unavailable.value=false;void refresh()},{immediate:true})
const timer=window.setInterval(()=>{if(!loading.value)void refresh()},10000)
onBeforeUnmount(()=>{alive=false;request++;window.clearInterval(timer)})
</script>
<template>
<section class="v2-inspect-section v2-runtime" aria-label="运行诊断">
 <h3>运行诊断 <span v-if="loading" role="status">· 查询中</span></h3>
 <p v-if="error" role="status" class="run-error">{{ error }}<span v-if="view">；以下为上次观察，不代表当前可用。</span></p>
 <p v-if="health" :data-ready="!unavailable && health.ready">{{ unavailable ? '当前就绪状态未知' : health.ready ? '服务执行就绪' : '服务尚未就绪' }}<small>检查于 {{ date(health.checked_at) }}</small></p>
 <details v-if="health"><summary>依赖检查与影响</summary><ul class="v2-check-list"><li v-for="check in health.checks" :key="check.name"><strong>{{ labels[check.name] }}</strong><span>{{ status[check.status] }}</span><small>{{ check.reason }}</small></li></ul></details>
 <template v-if="view">
  <small>观察对应进度 {{ view.run_ref.object_version }} · 当前进度 {{ run.state_sequence }}<br>观察于 {{ date(view.observed_at) }}</small>
  <details open><summary>Workflow 检查点 · {{ status[view.checkpoint.availability] }}</summary><p v-if="run.modes.execution_mode==='local'">本地模式使用持久业务快照；没有 Temporal 执行段。</p><template v-else><p>{{ view.checkpoint.state || '尚不可读取' }} · {{ view.checkpoint.reason }}</p><small>检查点 {{ date(view.checkpoint.checkpoint_at) }}<br>查询 {{ date(view.checkpoint.sampled_at) }}</small><p class="hash">{{ view.checkpoint.execution_run_id || '执行段不可用' }}</p></template></details>
  <details><summary>实际活动尝试 · {{ view.activity_attempts.length }}</summary><p>调用结果与业务完成分开；心跳只表示存活。</p><ol class="v2-history"><li v-for="a in view.activity_attempts.slice(-12)" :key="a.attempt_id"><strong>{{ a.step }} · {{ status[a.status] }}</strong><small>{{ a.mode }} · 尝试 {{ a.attempt }} · {{ a.elapsed_ms===null?'尚无结束耗时':a.elapsed_ms+' ms' }}</small><small>心跳 {{ status[a.heartbeat_status] }} · {{ date(a.last_heartbeat_at) }}</small><p class="hash">{{ a.execution_run_id || a.activity_id }}</p></li></ol></details>
  <details><summary>业务累计指标 · {{ status[view.metrics.availability] }}</summary><template v-if="view.metrics.availability==='complete'"><p>成功 {{ view.metrics.successful_runs }} · 失败 {{ view.metrics.failed_runs }} · 活动重试 {{ view.metrics.activity_retries }}</p><p>验证失败 {{ view.metrics.verification_failures }} · 有效需求 {{ view.metrics.effective_business_rounds }} 轮</p><p>总耗时 {{ view.metrics.total_elapsed_ms }} ms（含暂停和阻塞）</p><p v-if="view.metrics.first_terminal_state">首次终止 {{ view.metrics.first_terminal_state }} · {{ date(view.metrics.first_terminal_at) }}</p><small>取消、暂停、阻塞不计失败。控制命令和重投不计需求轮次。</small></template><p v-else>此历史任务未完整采集运行事件，指标不作推断。</p></details>
  <details v-if="view.diagnosis" open><summary>恢复条件 · {{ view.diagnosis.category }}</summary><p>{{ view.diagnosis.message }}</p><p>{{ view.diagnosis.code }} · {{ view.diagnosis.step }}</p><ul><li v-for="item in view.diagnosis.missing_requirements" :key="item">{{ item }}</li></ul><small>建议 {{ view.diagnosis.suggested_action }} · {{ view.diagnosis.retryable?'依赖恢复后可重新校验':'不授权直接重试' }}</small><p v-if="view.diagnosis.operation_id" class="hash">原操作 {{ view.diagnosis.operation_id }}</p></details>
  <details><summary>冻结期限与计量</summary><p>单工具 {{ view.budget.policy.max_tool_seconds }} s · 总执行 {{ view.budget.policy.max_execution_seconds }} s</p><p>原累计截止 {{ date(view.budget.execution_deadline) }}</p><small>恢复不会延长期限；上下文使用保守字节上界和冻结预留，实际供应商 token 另记。</small></details>
  <details><summary>追加审计 · {{ status[view.audit_coverage] }}</summary><button @click="readAudit">核验并读取完整审计链</button><p v-if="audit">{{ audit.events.length }} 项 · {{ status[audit.coverage] }}</p><pre v-if="audit">{{ JSON.stringify(audit, null, 2) }}</pre></details>
 </template>
 <button @click="refresh" :disabled="loading">刷新运行诊断</button>
</section>
</template>
