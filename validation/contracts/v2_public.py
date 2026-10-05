"""Public v2 transport, exact receipt, delivery and real generated runtime acceptance."""
import argparse
from copy import deepcopy
from datetime import datetime, timezone
import json
from pathlib import Path
import time
import urllib.request
import urllib.error
import urllib.parse
import uuid

from .oracle import Oracle, byte_hash, canonicalize, strict_loads
from .semantic_checks import require, verify_hash
from .v2_semantics import verify_snapshot
from .s2_semantics import verify_audit
from .s2_raw_artifacts import verify_raw
from .s2_public_scenarios import Scenarios


def main():
    p=argparse.ArgumentParser();p.add_argument('--base-url',required=True);p.add_argument('--mode',choices=['local','temporal'],required=True);p.add_argument('--report',type=Path,required=True);args=p.parse_args()
    directory=args.report.resolve();directory.mkdir(parents=True,exist_ok=False)
    oracle=Oracle(Path(__file__).parent/'releases/1.4.0',Path(__file__).parents[2]/'design/contracts/releases/1.4.0')
    def request(method,path,body=None,status=200,raw=False,headers=None):
        data=None if body is None else body.encode() if isinstance(body,str) else json.dumps(body,ensure_ascii=False).encode()
        req=urllib.request.Request(args.base_url+path,method=method,data=data,headers={'Content-Type':'application/json',**(headers or {})})
        try:r=urllib.request.urlopen(req,timeout=180)
        except urllib.error.HTTPError as e:r=e
        with r:
            b=r.read(16*1024*1024+1);h=dict(r.headers);code=r.status
        with (directory/'http.jsonl').open('a') as f:f.write(json.dumps({'method':method,'path':path,'status':code,'request':body,'response_sha256':byte_hash(b)},ensure_ascii=False)+'\n')
        require(code==status,'HTTP '+str(code)+' '+b[:700].decode(errors='replace'))
        return (b,h) if raw else strict_loads(b)
    definition=next(d for d in request('GET','/api/definitions') if d['definition_version']=='3.3.0')
    run='v2_'+uuid.uuid4().hex[:12];base='/api/v2/runs/'+run
    body=dict(contract_version='1.4.0',schema_version=2,command_id='create',run_id=run,title='v2 public acceptance',modes=dict(execution_mode=args.mode,decision_mode='rules',delivery_mode='local'),definition_selection={k:definition[k] for k in ('definition_id','definition_version','definition_hash')},initial_content='创建 public-project，SQLite + GORM、JWT 认证和 JSON 日志')
    receipt=request('POST','/api/v2/runs',body,201);oracle.validate('CommandReceipt',receipt);require(receipt['command_id']=='create' and receipt['command_fingerprint']==byte_hash(canonicalize(body)),'create wire receipt mismatch')
    require(request('POST','/api/v2/runs',body,201)==receipt,'create replay changed original receipt')
    def get():
        run=request('GET',base);verify_snapshot(run,oracle);return run
    def ref(r):return dict(object_type='run',object_id=r['run_id'],object_version=r['state_sequence'],source_version=r['source_version'],hash=r['snapshot_hash'])
    def object_(reference):
        query=urllib.parse.urlencode({k:reference[k] for k in ('object_version','source_version','hash')})
        return request('GET',base+'/objects/'+reference['object_type']+'/'+reference['object_id']+'?'+query)
    initial=get();require(object_(receipt['run_ref'])['snapshot_hash']==receipt['run_ref']['hash'],'initial result unreadable')
    message=dict(contract_version='1.4.0',schema_version=2,command_id='progress',target=ref(initial),content='查看进度')
    progress=request('POST',base+'/messages',message);oracle.validate('CommandReceipt',progress);require(progress['command_fingerprint']==byte_hash(canonicalize(message)),'message wire fingerprint mismatch')
    require(request('GET',base+'/commands/progress')==progress and request('POST',base+'/messages',message)==progress,'message replay changed original result')
    r=get();require(r['source_version']==initial['source_version'] and r['state_sequence']>initial['state_sequence'],'progress changed source')
    for label,change in [('unknown',lambda b:b.update(extra=1)),('bool',lambda b:b.update(schema_version=True)),('alias',lambda b:b.update(id=b.pop('command_id')))]:
        bad=deepcopy(message);change(bad);error=request('POST',base+'/messages',bad,400);oracle.validate('ErrorResponse',error)
    bad=deepcopy(message);bad['content']='different';request('POST',base+'/messages',bad,409)
    request('POST',base+'/messages','{"content":"a","content":"b"}',400)
    bad=deepcopy(message);bad['command_id']='wrong_reference';bad['target']['hash']='0'*64;request('POST',base+'/messages',bad,409)
    with urllib.request.urlopen(args.base_url+base+'/events',timeout=20) as stream:
        while True:
            line=stream.readline()
            if line.startswith(b'data: '):event=strict_loads(line[6:]);break
    oracle.validate('SnapshotEvent',event);require(event['run_id']==run and event['event_id']=='v2_'+byte_hash(run.encode())[:20]+'_'+str(event['state_sequence']),'event identity differs')
    snap=object_(event['run_ref']);verify_snapshot(snap,oracle);require(snap['state_sequence']==event['state_sequence'],'event reference differs')
    request('GET',base+'/events',status=409,headers={'Last-Event-ID':'wrong_run_1'})
    def action(r,kind,identity):
        a=dict(command_id=identity,action=kind,target=ref(r))
        if kind in ['accept','reject','revise']:
            proposal=r['proposal'];a['required_proposal_ref']=dict(object_type='proposal',object_id=proposal['proposal_id'],object_version=proposal['proposal_version'],source_version=proposal['source_version'],hash=proposal['snapshot_hash'])
        out=request('POST',base+'/actions',a);oracle.validate('CommandReceipt',out);require(out['status']=='processed','action rejected '+str(out));return out
    action(get(),'accept','approve_initial');deadline=time.monotonic()+900;last=None
    while time.monotonic()<deadline:
        r=get()
        if r['state']!=last:print('state',r['state'],'source',r['source_version'],flush=True);last=r['state']
        if r['state']=='DONE':break
        if r['state']=='PLAN':
            require(r['plan']['verification_candidate_ref'] is not None,'unexpected unapproved replan')
            action(r,'accept','approve_'+r['proposal']['proposal_id'])
        require(r['state'] not in ['FAILED','CANCELLED','BLOCKED'],'execution stopped '+str(r['error']))
        time.sleep(1)
    require(r['state']=='DONE','delivery timeout');verify_snapshot(r,oracle);require(len(r['plan']['selected_skills'])==3,'three skills omitted')
    (directory/'done.json').write_text(json.dumps(r,ensure_ascii=False,indent=2)+'\n')
    audit=request('GET','/api/runs/'+run+'/decision-audit')
    cache={}
    def load(reference):
        if reference['hash'] not in cache:
            b,h=request('GET','/api/runs/'+run+'/definition/content?'+urllib.parse.urlencode({k:reference[k] for k in ('object_type','object_id','hash')}),raw=True)
            require(byte_hash(b)==reference['hash'] and h.get('X-Content-Sha256',h.get('X-Content-SHA256'))==reference['hash'],'public frozen source hash differs');cache[reference['hash']]=b
        return cache[reference['hash']]
    facts=verify_audit(audit,oracle,load)
    for key in ['plan','proposal','approval','manifest','verification_report','messages','operations']:require(audit[key]==r[key],'snapshot differs from control '+key)
    package=next(p for p in reversed(audit['package_receipts']) if p['status']=='succeeded' and p['source_version']==r['source_version'])
    raw,headers=request('GET',base+'/download',raw=True)
    headers={k.lower():v for k,v in headers.items()}
    require(headers['x-manifest-hash']==r['manifest']['hash'] and headers['x-artifact-sha256']==byte_hash(raw) and headers['x-source-version']==str(r['source_version']),'download headers differ')
    verify_raw(raw,{'ref':package['package_ref'],'sha256':package['package_sha256'],'byte_count':package['package_bytes'],'format':'zip','manifest':r['manifest']})
    (directory/'delivery.zip').write_bytes(raw)
    for file in r['artifacts']:
        b,h=request('GET',base+'/artifacts?'+urllib.parse.urlencode({'path':file['path']}),raw=True);require(len(b)==file['size'] and byte_hash(b)==file['sha256'],'file bytes differ')
    request('GET',base+'/artifacts?path=../../escape',status=400)
    # Reuse a black-box consumer of this exact public ZIP. It builds and starts
    # the generated project only, and has no access to backend internal state.
    class Runtime:
        pass
    runtime=Runtime();runtime.directory=directory;Scenarios.runtime(runtime,raw,r['plan']['project_name'])
    result=action(r,'retry','redo_completed');newref=next(ref for ref in result['result_refs'] if ref['object_type']=='run')
    next_run=request('GET','/api/v2/runs/'+newref['object_id']);verify_snapshot(next_run,oracle)
    require(result['run_ref']['object_id']==run and next_run['retry_of']==run and next_run['approval'] is None and next_run['state']=='PLAN','redo identity or authority wrong')
    require(action(r,'retry','redo_completed')==result,'redo replay changed new identity')
    summary=dict(passed=True,at=datetime.now(timezone.utc).isoformat(),mode=args.mode,run_id=run,successor=next_run['run_id'],scope='S4 complete v2 HTTP/SSE, independent full objects, three-skill ZIP actual build/start; no live providers or browser claim',facts=facts,required_checks=len(r['required_checks']),package_sha256=byte_hash(raw))
    (directory/'summary.json').write_text(json.dumps(summary,ensure_ascii=False,indent=2)+'\n');print(json.dumps(summary,ensure_ascii=False))

if __name__=='__main__':main()
