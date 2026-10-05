"""Read/write public runtime acceptance, including actual Continue-As-New."""
import argparse,json,time,urllib.request,urllib.error,uuid
from datetime import datetime,timezone
from pathlib import Path
from .oracle import strict_loads,canonicalize,byte_hash
from .runtime_oracle import RuntimeOracle,verify_view,hashed,require,negative_cases
from .v2_semantics import verify_snapshot
from .oracle import Oracle

def main():
 p=argparse.ArgumentParser();p.add_argument('--base-url',required=True);p.add_argument('--report-dir',type=Path,required=True);p.add_argument('--can',action='store_true');args=p.parse_args();args.report_dir.mkdir(parents=True,exist_ok=False)
 oracle=RuntimeOracle();business=Oracle(Path(__file__).parent/'releases/1.4.0');requests=[]
 def req(method,path,body=None,status=200):
  data=None if body is None else json.dumps(body,ensure_ascii=False).encode();r=urllib.request.Request(args.base_url+path,data=data,method=method,headers={'Content-Type':'application/json'})
  try:response=urllib.request.urlopen(r,timeout=180)
  except urllib.error.HTTPError as e:response=e
  with response:out=strict_loads(response.read());code=response.status
  requests.append({'method':method,'path':path,'status':code});require(code==status,'HTTP mismatch '+path+' '+str(out));return out
 health=req('GET','/api/runtime/ready');oracle.validate('RuntimeHealth',health);hashed(health,'health_hash');require(health['ready'],'dependency readiness failed')
 live=req('GET','/api/runtime/live');oracle.validate('RuntimeHealth',live);require(live['live'] and not live['ready'],'liveness claims dependency readiness')
 config=req('GET','/api/bootstrap');definition=next(x for x in req('GET','/api/definitions') if x['definition_version']=='3.3.0')
 run='runtime_'+uuid.uuid4().hex[:12];base='/api/v2/runs/'+run
 body={'contract_version':'1.4.0','schema_version':2,'command_id':'initial_runtime','run_id':run,'title':run,'modes':{'execution_mode':config['execution_mode'],'decision_mode':'rules','delivery_mode':'local'},'definition_selection':{k:definition[k] for k in ['definition_id','definition_version','definition_hash']},'initial_content':'创建 runtime-service，SQLite GORM JWT JSON 日志'}
 receipt=req('POST','/api/v2/runs',body,201)
 def settled():
  for _ in range(15):
   v=req('GET',base+'/runtime');a=req('GET',base+'/audit')
   if v['audit_head_hash']==a['head_hash']:verify_view(v,a,oracle);return v,a
   time.sleep(.1)
  raise AssertionError('observations never settled')
 def ref(r):return {'object_type':'run','object_id':run,'object_version':r['state_sequence'],'source_version':r['source_version'],'hash':r['snapshot_hash']}
 initial,a=settled();first_execution=initial['checkpoint']['execution_run_id']
 require(initial['metrics']['effective_business_rounds']==1,'initial request not counted')
 for i in range(21 if args.can else 2):
  r=req('GET',base);verify_snapshot(r,business)
  wire={'contract_version':'1.4.0','schema_version':2,'command_id':'progress_'+str(i),'target':ref(r),'content':'进度'}
  out=req('POST',base+'/messages',wire);require(out==req('POST',base+'/messages',wire),'processed command receipt changed on replay')
 v,a=settled();require(v['metrics']['effective_business_rounds']==1,'progress inflated requirements')
 if args.can:
  deadline=time.monotonic()+20
  while v['checkpoint']['execution_run_id']==first_execution and time.monotonic()<deadline:time.sleep(.5);v,a=settled()
  require(v['checkpoint']['execution_run_id'] and v['checkpoint']['execution_run_id']!=first_execution,'actual CAN did not happen')
  require(len({x['execution_run_id'] for x in v['activity_attempts']})>=2,'attempt provenance did not span CAN')
 pre=a
 for action in ['pause','cancel']:
  r=req('GET',base);wire={'command_id':action+'_runtime','action':action,'target':ref(r)};req('POST',base+'/actions',wire)
  v,a=settled();require(v['metrics']['failed_runs']==0,'pause or cancel counted as failure')
 require(v['metrics']['first_terminal_state']=='CANCELLED' and v['diagnosis']['category']=='user_cancelled','cancel diagnosis incorrect')
 require([e['event_hash'] for e in a['events'][:len(pre['events'])]]==[e['event_hash'] for e in pre['events']],'audit prefix changed')
 elapsed=v['metrics']['total_elapsed_ms'];time.sleep(.05);v,a=settled();require(v['metrics']['total_elapsed_ms']==elapsed,'terminal elapsed not frozen')
 metrics=req('GET','/api/runtime/metrics');oracle.validate('RuntimeServiceMetrics',metrics);hashed(metrics,'metrics_hash');require(run in metrics['included_run_ids'],'new Run excluded')
 negative=sum(negative_cases(name,value,oracle) for name,value in [('RuntimeView',v),('RuntimeHealth',health),('RuntimeAuditTrail',a),('RuntimeServiceMetrics',metrics),('RuntimeMetrics',v['metrics']),('RuntimeCheckpoint',v['checkpoint']),('RuntimeBudgetStatus',v['budget']),('RuntimeFailureDiagnosis',v['diagnosis']),('RuntimeHealthCheck',health['checks'][0]),('RuntimeActivityAttempt',v['activity_attempts'][0]),('RuntimeAuditEvent',a['events'][0]),('RuntimeCorrelation',a['events'][0]['correlation']),('RuntimeParameters',a['events'][0]['parameters'])])
 for name,value in [('view',v),('audit',a),('ready',health),('metrics',metrics)]: (args.report_dir/(name+'.json')).write_text(json.dumps(value,ensure_ascii=False,indent=2)+'\n')
 summary={'at':datetime.now(timezone.utc).isoformat(),'status':'passed','run_id':run,'mode':config['execution_mode'],'actual_can':args.can,'events':len(a['events']),'activity_attempts':len(v['activity_attempts']),'required_and_closed_negatives':negative,'schema_sha256':byte_hash(oracle.raw['runtime.schema.json']),'requests':requests,'boundaries':['No live model/GitLab','Metrics count audit source, no direct product imports','Healthy dependency probe; fault injection separately recorded']}
 (args.report_dir/'summary.json').write_text(json.dumps(summary,indent=2)+'\n');print(json.dumps({k:x for k,x in summary.items() if k!='requests'}),flush=True)
if __name__=='__main__':main()
