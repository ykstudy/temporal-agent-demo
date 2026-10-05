import fs from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
const { chromium } = await import(process.env.PLAYWRIGHT_MODULE || 'playwright');
const reportDir = path.resolve(process.env.AGENT_BROWSER_REPORT_DIR || fileURLToPath(new URL('../reports/browser/', import.meta.url)));
await fs.mkdir(reportDir,{recursive:true});
const browserURL = process.env.AGENT_BROWSER_URL || 'http://127.0.0.1:15173/';
const apiURL = (process.env.AGENT_API_URL || 'http://127.0.0.1:18091').replace(/\/$/,'');
const browser=await chromium.launch({headless:true,channel:'chrome'});
const page=await browser.newPage({viewport:{width:1440,height:1000}});
const checks=[];const errors=[];
page.on('pageerror',err=>errors.push(String(err)));
const delay=ms=>new Promise(resolve=>setTimeout(resolve,ms));
const ok=(name,value)=>{if(!value)throw new Error(name);checks.push({name,passed:true})};
try{
  await page.goto(browserURL,{waitUntil:'networkidle'});
  const list=await (await page.request.get(`${apiURL}/api/runs`)).json();
  if(list.filter(run=>run.state==='DONE').length<2){
    const initial=await (await page.request.post(`${apiURL}/api/runs`,{data:{id:'browser-secondary',content:'创建 secondary-service'}})).json();
    const p=initial.proposal;
    const response=await page.request.post(`${apiURL}/api/runs/${initial.id}/actions`,{data:{id:'accept-secondary',action:'accept',proposal_id:p.id,proposal_version:p.proposal_version,source_version:p.source_version,snapshot_hash:p.snapshot_hash}});
    if(!response.ok())throw new Error(await response.text());
    for(let i=0;i<120;i++){const run=await(await page.request.get(`${apiURL}/api/runs/${initial.id}`)).json();if(run.state==='DONE')break;if(['FAILED','BLOCKED'].includes(run.state))throw new Error(run.error);await delay(500)}
  }
  const completed=await(await page.request.get(`${apiURL}/api/runs`)).json();
  const [b,a]=completed.filter(run=>run.state==='DONE');if(!a||!b)throw new Error('Need two real completed runs');
  await page.getByRole('button',{name:'刷新任务'}).click();
  const choose=async run=>{await page.getByRole('navigation',{name:'项目任务'}).getByRole('button',{name:new RegExp(run.plan.project_name)}).click();await page.getByRole('heading',{name:run.plan.project_name,exact:true}).waitFor();};
  await choose(a);
  await page.route(`**/api/runs/${a.id}`,async route=>{await delay(1500);await route.continue()});
  await page.getByRole('button',{name:'刷新任务'}).click();
  await choose(b);await delay(1900);
  ok('late_snapshot_keeps_selected_run',await page.getByRole('heading',{name:b.plan.project_name,exact:true}).isVisible());
  await page.unroute(`**/api/runs/${a.id}`);
  await page.route(`**/api/runs/${b.id}/artifacts?path=README.md`,async route=>{await delay(1500);await route.continue()});
  await page.getByRole('button',{name:'README.md',exact:true}).click();
  await page.getByRole('button',{name:'cmd/server/main.go',exact:true}).click();
  await page.locator('.artifact-content strong').filter({hasText:'cmd/server/main.go'}).waitFor();await delay(1900);
  ok('late_file_keeps_latest_file',(await page.locator('.artifact-content strong').innerText())==='cmd/server/main.go');
  await page.getByRole('button',{name:'README.md',exact:true}).click();await choose(a);await delay(1900);
  ok('late_artifact_cannot_cross_run',await page.locator('.artifact-content').count()===0);
  const expiry=await(await page.request.post(`${apiURL}/api/runs`,{data:{id:`browser-expiry-${Date.now().toString().slice(-6)}`,content:'创建 expiry-ui-service'}})).json();
  await page.getByRole('button',{name:'刷新任务'}).click();await choose(expiry);
  await page.getByRole('button',{name:'确认并生成',exact:true}).waitFor();
  await page.evaluate(expires=>{Date.now=()=>Date.parse(expires)+1000},expiry.proposal.expires_at);
  await page.getByText('方案已过期，请选择“再改改”后重新生成方案。',{exact:true}).waitFor();
  ok('client_clock_expiry_disables_confirmation',await page.getByRole('button',{name:'确认并生成',exact:true}).isDisabled());
  ok('race_console_health',errors.length===0);
  await fs.writeFile(`${reportDir}/browser-races.json`,JSON.stringify({status:'passed',checks,errors},null,2));
  console.log(JSON.stringify({checks,errors}));
}finally{await browser.close()}
