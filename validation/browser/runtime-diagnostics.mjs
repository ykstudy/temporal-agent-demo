import fs from 'node:fs/promises';
import crypto from 'node:crypto';
const {chromium}=await import(process.env.PLAYWRIGHT_MODULE || 'playwright');
const out=process.env.AGENT_BROWSER_REPORT_DIR || '/tmp/agent-runtime-diagnostics',base=process.env.AGENT_BROWSER_URL || 'http://127.0.0.1:18172';await fs.mkdir(out,{recursive:true});
const browser=await chromium.launch({headless:true,channel:'chrome'}),page=await browser.newPage({viewport:{width:1440,height:1000}});page.setDefaultTimeout(30000);
const checks=[],errors=[];page.on('pageerror',e=>errors.push(String(e)));const ok=(name,value)=>{if(!value)throw Error(name);checks.push({name,passed:true})};
const canon=v=>v===null||typeof v!=='object'?JSON.stringify(v):Array.isArray(v)?'['+v.map(canon).join(',')+']':'{'+Object.keys(v).sort().map(k=>JSON.stringify(k)+':'+canon(v[k])).join(',')+'}';
try {
 const d=(await (await page.request.get(base+'/api/definitions')).json()).find(d=>d.definition_version==='3.3.0');const id='browser_runtime_'+Date.now();
 const created=await page.request.post(base+'/api/v2/runs',{data:{contract_version:'1.4.0',schema_version:2,command_id:'create_observed',run_id:id,title:'运行诊断验收',modes:{execution_mode:'local',decision_mode:'rules',delivery_mode:'local'},definition_selection:{definition_id:d.definition_id,definition_version:d.definition_version,definition_hash:d.definition_hash},initial_content:'创建 observed-service'}});ok('public_create_for_observation',created.status()===201);
 const r=await(await page.request.get(base+'/api/v2/runs/'+id)).json();const target={object_type:'run',object_id:id,object_version:r.state_sequence,source_version:r.source_version,hash:r.snapshot_hash};
 ok('public_cancel_processed',(await(await page.request.post(base+'/api/v2/runs/'+id+'/actions',{data:{command_id:'cancel_observed',action:'cancel',target}})).json()).status==='processed');
 await page.goto(base+'?run='+id,{waitUntil:'networkidle'});const panel=page.getByRole('region',{name:'运行诊断'});await panel.getByText('服务执行就绪',{exact:false}).waitFor();ok('actual_service_health',await panel.locator('[data-ready="true"]').count()===1);
 await panel.getByText('业务累计指标',{exact:false}).click();ok('cancel_not_failed_and_requirements_not_controls',(await panel.innerText()).includes('成功 0 · 失败 0')&&(await panel.innerText()).includes('有效需求 1 轮')&&(await panel.innerText()).includes('首次终止 CANCELLED'));
 ok('local_does_not_invent_temporal',(await panel.innerText()).includes('没有 Temporal 执行段'));
 await panel.getByText('追加审计',{exact:false}).click();await panel.getByRole('button',{name:'核验并读取完整审计链'}).click();await panel.locator('pre').waitFor();ok('full_audit_chain_validated',(await panel.locator('pre').innerText()).includes('event_hash'));
 await page.screenshot({path:out+'/runtime-desktop.png'});
 const view=await(await page.request.get(base+'/api/v2/runs/'+id+'/runtime')).json();view.run_id='other_run';delete view.view_hash;view.view_hash=crypto.createHash('sha256').update(canon(view)).digest('hex');
 await page.route('**/api/v2/runs/'+id+'/runtime',route=>route.fulfill({status:200,contentType:'application/json',body:JSON.stringify(view)}));await panel.getByRole('button',{name:'刷新运行诊断'}).click();await panel.getByText('运行诊断所属 Run 不同',{exact:false}).waitFor();ok('cross_run_observation_rejected',(await panel.innerText()).includes('上次观察')&&await panel.locator('[data-ready="false"]').count()===1);
 await page.unroute('**/api/v2/runs/'+id+'/runtime');await panel.getByRole('button',{name:'刷新运行诊断'}).click();await panel.locator('[data-ready="true"]').waitFor();
 await page.route('**/api/runtime/ready',route=>route.abort('failed'));await panel.getByRole('button',{name:'刷新运行诊断'}).click();await panel.getByText('当前就绪状态未知').waitFor();ok('unavailable_is_not_cached_success',await panel.locator('[data-ready="false"]').count()===1);
 await page.unroute('**/api/runtime/ready');await panel.getByRole('button',{name:'刷新运行诊断'}).click();await panel.locator('[data-ready="true"]').waitFor();
 await page.setViewportSize({width:390,height:844});await page.getByRole('button',{name:'打开运行详情'}).click();await page.screenshot({path:out+'/runtime-mobile.png'});ok('mobile_no_overflow',await page.evaluate(()=>document.documentElement.scrollWidth<=window.innerWidth));ok('no_page_exceptions',errors.length===0);
 await fs.writeFile(out+'/summary.json',JSON.stringify({at:new Date().toISOString(),browser_path:'Playwright: Browser plugin not available',run_id:id,checks,errors,scope:'Actual runtime HTTP/health/audit and explicit network faults; no provider/CI calls'},null,2));console.log(JSON.stringify({passed:true,checks:checks.length,run_id:id,out}));
}finally{await browser.close()}
