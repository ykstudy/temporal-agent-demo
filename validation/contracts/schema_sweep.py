"""Independent S5 named-shape sweep of original public HTTP objects.

The schemas decide structure. Observations supply legitimate instances only;
objects are never adapted to a different release or used to invent expectations.
"""
import argparse,json,urllib.request,urllib.parse
from collections import Counter
from copy import deepcopy
from datetime import datetime,timezone
from pathlib import Path
from urllib.parse import urldefrag,urljoin
from jsonschema import Draft202012Validator
from .oracle import Oracle,ContractError,strict_loads,byte_hash
from .runtime_oracle import RuntimeOracle,verify_view
from .self_test import mutation_cases,replace
from .semantic_checks import require,object_hash
from .v2_semantics import verify_snapshot
from .s2_semantics import verify_audit
from .conversation_semantics import verify_conversation


def main():
 p=argparse.ArgumentParser();p.add_argument('--base-url',required=True);p.add_argument('--run-id',action='append',required=True);p.add_argument('--public-export',action='append',type=Path,default=[]);p.add_argument('--report-dir',type=Path,required=True);args=p.parse_args();out=args.report_dir;out.mkdir(parents=True,exist_ok=False)
 oracle=RuntimeOracle();business=Oracle(Path(__file__).parent/'releases/1.4.0');samples={};sources=[];cases=[];cache={}
 files={s['$id']:name for name,s in oracle.schemas.items()}
 def valid(schema,value,base):
  def absolute(v):
   if isinstance(v,dict):return {k:urljoin(base,x) if k=='$ref' else absolute(x) for k,x in v.items()}
   if isinstance(v,list):return [absolute(x) for x in v]
   return v
  return Draft202012Validator(absolute(schema),registry=oracle.registry,format_checker=oracle.formats).is_valid(value)
 def collect(schema,value,base):
  if isinstance(schema,bool):return
  if '$ref' in schema:
   ref=urljoin(base,schema['$ref']);base,fragment=urldefrag(ref);target=oracle.registry.resolver(base).lookup(ref).contents
   if fragment.startswith('/$defs/'):
    name=fragment.split('/')[-1];variant=':'.join(str(value.get(k,'')) for k in ['type','kind','status','outcome','action_type']) if isinstance(value,dict) else type(value).__name__
    key=(files[base],name,variant)
    samples.setdefault(key,deepcopy(value))
   collect(target,value,base);return
  for item in schema.get('allOf',[]):collect(item,value,base)
  for keyword in ['anyOf','oneOf']:
   for item in schema.get(keyword,[]):
    if valid(item,value,base):collect(item,value,base)
  if 'if' in schema:
   branch='then' if valid(schema['if'],value,base) else 'else'
   if branch in schema:collect(schema[branch],value,base)
  if isinstance(value,dict):
   for key,child in schema.get('properties',{}).items():
    if key in value:collect(child,value[key],base)
  if isinstance(value,list) and 'items' in schema:
   for item in value:collect(schema['items'],item,base)
 def add(name,value,source):
  file=next(f for f,s in oracle.schemas.items() if name in s['$defs']);oracle.validate(name,value,file)
  collect({'$ref':oracle.schemas[file]['$id']+'#/$defs/'+name},value,oracle.schemas[file]['$id']);sources.append({'schema':name,'source':source,'sha256':byte_hash(json.dumps(value,ensure_ascii=False).encode())})
 def embedded(r,audit):
  payloads={'clarification':'ClarificationPayload','proposal_diff':'ProposalDiffPayload','progress':'ProgressPayload','artifact_manifest':'ArtifactManifestPayload','verification_report':'VerificationReportPayload'}
  for m in r['messages']:
   for part in m['parts']:
    if part.get('component_type') in payloads:add(payloads[part['component_type']],part['payload'],'explicit published component payload')
  params={'scaffold':'ScaffoldParameters','apply_skill':'ApplySkillParameters','verify':'VerifyParameters','create_repository':'CreateRepositoryParameters','push_repository':'PushRepositoryParameters','trigger_ci':'TriggerCIParameters','package':'PackageParameters'}
  for x in audit['executions']:
   action=x['action'];add(params[action['action_type']],action['parameters'],'published action parameters')
  for skill in r['plan']['selected_skills'] if r['plan'] else []:
   name={'add_database':'DatabaseParameters','add_auth':'AuthenticationParameters','add_logging':'LoggingParameters'}[skill['skill_id']]
   add(name,skill['parameters'],'published skill parameters')
  if r['modes']['decision_mode']=='model':add('ProviderBinding',r['definition_snapshot']['provider_binding'],'frozen model identity')
 def get(path,raw=False):
  with urllib.request.urlopen(args.base_url+path,timeout=180) as r:b=r.read(16*1024*1024+1)
  return b if raw else strict_loads(b)
 for run in args.run_id:
  base='/api/v2/runs/'+run;r=get(base);add('RunSnapshot',r,base);verify_snapshot(r,business)
  audit=get('/api/runs/'+run+'/decision-audit');add('S2AuditBundle',audit,'public decision-audit')
  embedded(r,audit)
  def load(ref):
   if ref['hash'] not in cache:cache[ref['hash']]=get('/api/runs/'+run+'/definition/content?'+urllib.parse.urlencode({k:ref[k] for k in ['object_type','object_id','hash']}),True)
   require(byte_hash(cache[ref['hash']])==ref['hash'],'content bytes differ');return cache[ref['hash']]
  verify_audit(audit,business,load)
  conversation=get('/api/runs/'+run+'/conversation-audit');add('ConversationAuditBundle',conversation,'public conversation-audit');raw=get('/api/runs/'+run+'/transcript',True)
  resolve=lambda ref:get('/api/runs/'+run+'/objects?'+urllib.parse.urlencode(ref))
  verify_conversation(conversation,business,raw,resolve)
  v=get(base+'/runtime');a=get(base+'/audit');verify_view(v,a,oracle);add('RuntimeView',v,base+'/runtime');add('RuntimeAuditTrail',a,base+'/audit')
  (out/(run+'.json')).write_text(json.dumps({'snapshot':r,'audit':audit,'conversation':conversation,'runtime':v,'runtime_audit':a},ensure_ascii=False,indent=2)+'\n')
  for label,mutate in [
    ('missing check',lambda x:x['required_checks'].pop()),
    ('duplicate check',lambda x:x['required_checks'].append(deepcopy(x['required_checks'][0]))),
    ('foreign check',lambda x:x['required_checks'][0].update(run_id='foreign_run')),
    ('false check with passed alias',lambda x:x['required_checks'][0].update(status='failed',passed=True)),
    ('future message',lambda x:x['messages'][0].update(source_version=x['source_version']+1)),
    ('message sequence gap',lambda x:x['messages'][0].update(sequence=9)),
  ]:
   bad=deepcopy(r);mutate(bad);bad['snapshot_hash']=object_hash('RunSnapshot',bad)
   try:verify_snapshot(bad,business)
   except ContractError:cases.append({'kind':'semantic','name':label,'run_id':run})
   else:raise AssertionError('semantic violation accepted '+label)
  if r['manifest']:
   for label,mutate in [('file sha256 alias',lambda x:x['manifest']['files'][0].update(hash=x['manifest']['files'][0].pop('sha256'))),('missing size',lambda x:x['manifest']['files'][0].pop('size')),('manifest dictionary alias',lambda x:x['manifest'].update(files={v['path']:v['sha256'] for v in x['manifest']['files']}))]:
    bad=deepcopy(r);mutate(bad)
    try:verify_snapshot(bad,business)
    except ContractError:cases.append({'kind':'schema','name':label,'run_id':run})
    else:raise AssertionError('alias accepted '+label)
 for path,name in [('/api/runtime/ready','RuntimeHealth'),('/api/runtime/live','RuntimeHealth'),('/api/runtime/metrics','RuntimeServiceMetrics')]:add(name,get(path),path)
 for path in args.public_export:
  record=strict_loads(path.read_bytes());r=record['snapshot'];audit=record['audit']
  for key,name in [('snapshot','RunSnapshot'),('audit','S2AuditBundle'),('conversation','ConversationAuditBundle'),('runtime','RuntimeView'),('runtime_audit','RuntimeAuditTrail')]:add(name,record[key],str(path))
  verify_snapshot(r,business);verify_audit(audit,business,lambda ref:(path.parent/'content'/ref['hash']).read_bytes());verify_view(record['runtime'],record['runtime_audit'],oracle);embedded(r,audit)
 # One sweep per named shape/observed variant. Referenced children are tested
 # separately, avoiding exponentially repeated mutations of the same full tree.
 for (file,name,variant),value in sorted(samples.items()):
  oracle.validate(name,value,file)
  for keyword,path,bad in mutation_cases(oracle,value,name,file,False):
   try:oracle.validate(name,bad,file)
   except ContractError:pass
   else:
    if keyword not in ['type','const']:raise AssertionError(f'negative accepted {file} {name} {keyword} {path}')
    found=False
    for alternate in [True,0,[],{'__undeclared__':True},None,'__invalid__']:
     bad=replace(value,path,alternate)
     try:oracle.validate(name,bad,file)
     except ContractError:found=True;break
    require(found,'nullable union has no invalid test value '+name)
   cases.append({'kind':'structure','schema':name,'variant':variant,'keyword':keyword,'path':list(path)})
 observed={name for _,name,_ in samples};wanted={name for schema in oracle.schemas.values() for name in schema['$defs']}
 report={'at':datetime.now(timezone.utc).isoformat(),'passed':True,'scope':'original public 1.4/runtime1.0 instances, full Schema, cross-object semantics and named-shape negatives; unobserved shapes explicitly listed','schema_sha256':{k:byte_hash(v) for k,v in oracle.raw.items()},'sources':sources,'sample_variants':len(samples),'observed_named_shapes':sorted(observed),'unobserved_named_shapes':sorted(wanted-observed),'counts':dict(Counter(x['kind'] for x in cases)),'cases':cases}
 (out/'summary.json').write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n');print(json.dumps({k:v for k,v in report.items() if k not in ['cases','sources','observed_named_shapes']},ensure_ascii=False))
if __name__=='__main__':main()
