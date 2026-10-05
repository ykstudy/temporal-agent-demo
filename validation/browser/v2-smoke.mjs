import fs from 'node:fs/promises';
import path from 'node:path';
const {chromium}=await import(process.env.PLAYWRIGHT_MODULE || 'playwright');
const out=process.env.AGENT_BROWSER_REPORT_DIR || '/tmp/agent-v2-browser';await fs.mkdir(out,{recursive:true});
const base=process.env.AGENT_BROWSER_URL || 'http://127.0.0.1:18164';
const browser=await chromium.launch({headless:true,channel:'chrome'});const page=await browser.newPage({viewport:{width:1440,height:1000}});page.setDefaultTimeout(30000);
const errors=[],httpErrors=[],checks=[],project='browser-v2-'+Date.now().toString().slice(-7);let runID;
page.on('response',async response=>{if(response.status()>=400 && response.url().includes('/api/')){httpErrors.push({url:response.url(),status:response.status(),at:new Date().toISOString(),body:await response.text().catch(()=>null)});await fs.writeFile(path.join(out,'http-errors.json'),JSON.stringify(httpErrors,null,2))}});
page.on('pageerror',e=>errors.push(String(e)));page.on('console',m=>{if(['error','warning'].includes(m.type()))errors.push(m.text())});
const ok=(name,value)=>{if(!value)throw Error(name);checks.push({name,passed:true})};
const current=async()=>{const id=new URL(page.url()).searchParams.get('run');return (await page.request.get(base+'/api/v2/runs/'+id)).json()};
try {
 await page.goto(base,{waitUntil:'networkidle'});await page.getByRole('heading',{name:'从需求到可运行的项目'}).waitFor();ok('identity_not_blank',(await page.title()).includes('项目创建 Agent') && await page.locator('main').isVisible());
 await page.getByRole('textbox',{name:'项目需求'}).fill('创建 '+project+'，SQLite + GORM、JWT 认证和 JSON 日志');await page.getByRole('button',{name:'发送需求',exact:true}).click();await page.getByRole('button',{name:'确认并生成',exact:true}).waitFor();
 const initial=await current();runID=initial.run_id;ok('P1_before_generation',initial.state==='PLAN' && initial.approval===null && initial.artifacts.length===0 && initial.plan.selected_skills.length===3);
 const originalHash=initial.proposal.snapshot_hash;
 await page.getByRole('button',{name:/预览 \d+ 个文件/}).click();await page.getByRole('combobox',{name:'选择预览文件'}).selectOption('README.md');ok('actual_frozen_preview',(await page.locator('.preview-files').innerText()).includes(project));
 await page.getByRole('textbox',{name:'项目需求'}).fill('改用 session 认证和 text 日志');await page.getByRole('button',{name:'发送需求',exact:true}).click();
 await page.waitForFunction(async id=>{const r=await(await fetch('/api/v2/runs/'+id)).json();return r.source_version>2 && r.plan.selected_skills.some(s=>s.skill_id==='add_auth' && JSON.stringify(s.parameters).includes('session'))},runID);
 await page.getByText('历史方案，只读',{exact:false}).waitFor();ok('old_card_bound_to_original',await page.locator('[data-proposal-hash="'+originalHash+'"]').count()===1 && await page.locator('[data-proposal-hash="'+originalHash+'"]').getByRole('button',{name:'确认并生成'}).count()===0);
 ok('one_current_confirmation',await page.getByRole('button',{name:'确认并生成',exact:true}).count()===1);
 await page.screenshot({path:path.join(out,'plan-desktop.png')});
 await page.getByRole('button',{name:'确认并生成',exact:true}).click();
 const deadline=Date.now()+420000;
 while(Date.now()<deadline){const r=await current();if(r.state==='DONE')break;if(['BLOCKED','FAILED','CANCELLED'].includes(r.state))throw Error(JSON.stringify(r.error));if(r.state==='PLAN' && r.plan.verification_candidate_ref){await page.getByRole('button',{name:'确认并生成',exact:true}).click()}await page.waitForTimeout(1000)}
 const done=await current();ok('real_DONE_complete_checks',done.state==='DONE' && done.required_checks.length===11 && done.required_checks.every(c=>c.status==='passed'));
 await page.getByRole('button',{name:'下载已验证 ZIP',exact:true}).waitFor();
 await page.getByRole('button',{name:'README.md',exact:true}).click();await page.getByRole('dialog').waitFor();ok('registered_file_modal',(await page.getByRole('dialog').innerText()).includes(project));await page.getByRole('dialog').getByRole('button',{name:'关闭',exact:true}).click();
 const downloadEvent=page.waitForEvent('download');await page.getByRole('button',{name:'下载已验证 ZIP',exact:true}).click();const download=await downloadEvent;await download.saveAs(path.join(out,'browser-delivery.zip'));ok('verified_zip_download',(await fs.stat(path.join(out,'browser-delivery.zip'))).size>1000);
 await page.screenshot({path:path.join(out,'done-desktop.png')});
 await page.setViewportSize({width:390,height:844});await page.getByRole('button',{name:'打开任务列表'}).click();await page.getByRole('navigation',{name:'运行记录'}).getByRole('button',{name:new RegExp(project)}).click();await page.getByRole('button',{name:'打开运行详情'}).click();await page.getByRole('button',{name:'下载已验证 ZIP'}).waitFor();await page.screenshot({path:path.join(out,'done-mobile.png')});await page.getByRole('button',{name:'关闭详情',exact:true}).click();
 ok('narrow_no_horizontal_overflow',await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth));
 await page.getByRole('button',{name:'重做为新任务',exact:true}).click();await page.waitForFunction(id=>new URL(location.href).searchParams.get('run')!==id,runID);const next=await current();ok('new_Run_requires_new_approval',next.retry_of===runID && next.approval===null && next.state==='PLAN');
 ok('no_framework_overlay',await page.locator('vite-error-overlay').count()===0);ok('console_clean',errors.length===0);
} catch(e) {checks.push({name:'failure',passed:false,error:String(e)});await page.screenshot({path:path.join(out,'failure.png')});console.error(String(e));process.exitCode=1}
finally{await fs.writeFile(path.join(out,'summary.json'),JSON.stringify({at:new Date().toISOString(),browser_path:'Playwright: Browser plugin not available',url:base,viewports:[[1440,1000],[390,844]],project,run_id:runID,checks,errors},null,2));console.log(JSON.stringify({passed:checks.every(c=>c.passed),checks:checks.length,run_id:runID,report:path.join(out,'summary.json')}));await browser.close()}
