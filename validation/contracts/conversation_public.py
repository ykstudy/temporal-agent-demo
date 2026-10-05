"""Independent S3 public HTTP regression; no backend modules or private files."""
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

from .oracle import Oracle, ContractError, byte_hash, strict_loads
from .semantic_checks import require, object_hash
from .conversation_semantics import verify_conversation


def main():
    parser=argparse.ArgumentParser();parser.add_argument('--base-url',required=True);parser.add_argument('--report',type=Path,required=True);parser.add_argument('--mode',choices=['local','temporal'],required=True);parser.add_argument('--decision-mode',choices=['rules','model'],default='rules')
    args=parser.parse_args();args.report.mkdir(parents=True,exist_ok=False)
    oracle=Oracle(Path(__file__).parent/'releases/1.4.0',Path(__file__).parents[2]/'design/contracts/releases/1.4.0')
    transcript=[]
    def http(method,path,data=None,status=200):
        body=None if data is None else json.dumps(data,ensure_ascii=False).encode();request=urllib.request.Request(args.base_url+path,data=body,method=method,headers={'Content-Type':'application/json'})
        try: response=urllib.request.urlopen(request,timeout=180)
        except urllib.error.HTTPError as e:response=e
        with response:
            raw=response.read(16*1024*1024+1);require(response.status==status,'unexpected HTTP status '+str(response.status)+' '+raw[:500].decode(errors='replace'))
            transcript.append({'method':method,'path':path,'status':response.status,'request':data,'response_sha256':byte_hash(raw)})
            with (args.report/"http.jsonl").open("a") as f: f.write(json.dumps(transcript[-1],ensure_ascii=False)+"\n")
            try:return strict_loads(raw)
            except ValueError:return raw
    definition=next(x for x in http('GET','/api/definitions') if x['definition_version']=='3.3.0')
    run='s3_'+uuid.uuid4().hex[:12]
    initial=http('POST','/api/runs',dict(id=run,title='S3 provenance regression',content='创建 provenance-project',execution_mode=args.mode,decision_mode=args.decision_mode,delivery_mode='local',**{k:definition[k] for k in ['definition_id','definition_version','definition_hash']}),201)
    first=http('GET','/api/runs/'+run+'/conversation-audit');original=http('GET','/api/runs/'+run+'/transcript')
    require(type(original) is bytes,'original source is not JSONL')
    workflow_before=http('GET','/api/runs/'+run+'/workflow') if args.mode=='temporal' else None
    for i in range(21): http('POST','/api/runs/'+run+'/messages',{'id':'progress_'+str(i),'content':'查看进度'})
    before_replay=http('GET','/api/runs/'+run+'/conversation-audit');http('POST','/api/runs/'+run+'/messages',{'id':'progress_0','content':'查看进度'})
    http('POST','/api/runs/'+run+'/messages',{'id':'progress_0','content':'different'},409)
    audit=http('GET','/api/runs/'+run+'/conversation-audit');raw=http('GET','/api/runs/'+run+'/transcript')
    require(audit['messages']==before_replay['messages'],'replay appended input or reply')
    require(raw.startswith(original),'compression rewrote original bytes')
    require(len(audit['summaries'])>=2 and all(a['status']=='succeeded' for a in audit['summary_attempts']),'multiple real summaries absent')
    require(audit['source_version']==first['source_version'],'progress changed business source')
    objects={}
    def resolve(ref):
        key=json.dumps(ref,sort_keys=True)
        if key not in objects:objects[key]=http('GET','/api/runs/'+run+'/objects?'+urllib.parse.urlencode(ref))
        return objects[key]
    facts=verify_conversation(audit,oracle,raw,resolve)
    if args.mode=='temporal':
        after=http('GET','/api/runs/'+run+'/workflow')
        require(after['workflow_id']==workflow_before['workflow_id'] and after['execution_run_id']!=workflow_before['execution_run_id'],'real CAN not observed')
        facts['workflow_before']=workflow_before;facts['workflow_after']=after
    negatives=[]
    def reject(label,mutate,semantic=False):
        changed=deepcopy(audit);mutate(changed)
        if semantic:
            for summary in changed['summaries']:summary['summary_hash']=object_hash('SummaryRecord',summary)
            changed['audit_hash']=object_hash('ConversationAuditBundle',changed)
            oracle.validate('ConversationAuditBundle',changed)
        try:verify_conversation(changed,oracle,raw,resolve)
        except (ContractError,ValueError,KeyError):negatives.append(label);return
        raise AssertionError('negative accepted: '+label)
    reject('unknown envelope field',lambda x:x['messages'][0].update(extra='bad'))
    reject('boolean message sequence',lambda x:x['messages'][0].update(sequence=True))
    reject('unknown Part',lambda x:x['messages'][0]['parts'][0].update(type='executable'))
    reject('foreign Run',lambda x:x['messages'][0].update(run_id='other_run'),True)
    reject('changed source bytes',lambda x:x['summaries'][0]['source'].update(raw_jsonl_sha256='0'*64),True)
    reject('summary range identity',lambda x:x['summaries'][0]['source']['message_ids'].__setitem__(0,'invented_message'),True)
    reject('summary predecessor',lambda x:x['summaries'][1].update(previous_summary_ref=None),True)
    reject('command receipt Run',lambda x:x['command_receipts'][0].update(run_id='other_run'),True)
    http('GET','/api/runs/'+run+'/transcript?first_sequence=0&last_sequence=3',status=400)
    http('GET','/api/runs/'+run+'/transcript?unknown=1',status=400)
    (args.report/'audit.json').write_text(json.dumps(audit,ensure_ascii=False,indent=2)+'\n');(args.report/'transcript.jsonl').write_bytes(raw)
    (args.report/'objects.json').write_text(json.dumps(objects,ensure_ascii=False,indent=2)+'\n');(args.report/'http.json').write_text(json.dumps(transcript,ensure_ascii=False,indent=2)+'\n')
    report=dict(passed=True,at=datetime.now(timezone.utc).isoformat(),run_id=run,mode=args.mode,decision_mode=args.decision_mode,contract_version='1.4.0',scope='S3 public provenance; not v2 API or live provider acceptance',facts=facts,negatives=negatives)
    (args.report/'summary.json').write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n');print(json.dumps(report,ensure_ascii=False))


if __name__=='__main__':main()
