"""Independent full runtime Schema and provenance oracle, consuming published bytes."""
from copy import deepcopy
from pathlib import Path
from jsonschema import Draft202012Validator, FormatChecker
from referencing import Registry, Resource
from .oracle import Oracle, ExactDraft202012Validator, ContractError, strict_loads, canonicalize, byte_hash, check_value, _offline

class RuntimeOracle(Oracle):
    def __init__(self):
        self.release=Path(__file__).parent/'runtime/1.0.0'
        publication=strict_loads((self.release/'publication.json').read_bytes())
        self.schemas,self.raw={},{}
        for name in ['runtime.schema.json','common.schema.json','agent.schema.json','message.schema.json']:
            raw=(self.release/name).read_bytes()
            digest=publication['sha256'] if name=='runtime.schema.json' else publication['references'][name]
            if byte_hash(raw)!=digest:raise ContractError('runtime publication byte mismatch')
            schema=strict_loads(raw);Draft202012Validator.check_schema(schema)
            self.schemas[name]=schema;self.raw[name]=raw
        self.registry=Registry(retrieve=_offline).with_resources((s['$id'],Resource.from_contents(s)) for s in self.schemas.values())
        self.formats=FormatChecker();self.static_refs=0
        for s in self.schemas.values():self._check_refs(s,s['$id'])
    def validate(self,name,value,schema_file='runtime.schema.json'):
        super().validate(name,value,schema_file)

def require(condition,reason):
    if not condition:raise ContractError(reason)
def hashed(v,key):require(v[key]==byte_hash(canonicalize({k:x for k,x in v.items() if k!=key})),'runtime content hash differs: '+key)
def verify_audit(a,oracle):
    oracle.validate('RuntimeAuditTrail',a);hashed(a,'audit_hash')
    require(a['run_ref']['object_id']==a['run_id'] and a['run_ref']['object_type']=='run','audit Run ref differs')
    prior=None;state=0
    for i,e in enumerate(a['events']):
        hashed(e,'event_hash')
        require(e['sequence']==i+1 and e['run_id']==a['run_id'] and e['previous_hash']==prior and state<=e['state_sequence']<=a['run_ref']['object_version'],'audit sequence or identity differs')
        require(e['parameters']['summary_hash']==byte_hash(canonicalize({k:e['parameters'][k] for k in ['action','argument_keys']})),'redacted parameters hash differs')
        prior=e['event_hash'];state=e['state_sequence']
    require(a['head_hash']==prior,'audit head differs')
    if a['coverage']=='complete':require(a['events'] and a['events'][0]['kind']=='run_created','complete audit missing creation')
    else:require(not a['events'],'old history fabricated')

def verify_view(v,a,oracle):
    oracle.validate('RuntimeView',v);hashed(v,'view_hash');verify_audit(a,oracle)
    require(v['run_id']==a['run_id'] and v['run_ref']['object_id']==v['run_id'],'view identity differs')
    if v['audit_coverage']=='unavailable':require(v['metrics']['availability']=='unavailable','unknown metrics guessed');return
    require(v['metrics']['availability']=='complete','complete audit missing metrics')
    # Caller reads settled data for exact cross-response comparison.
    require(v['audit_head_hash']==a['head_hash'],'audit moved during observation; retry a coherent settled read')
    terminal=next((e for e in a['events'] if e['kind']=='run_terminal'),None)
    metrics=v['metrics'];require(metrics['successful_runs']==int(bool(terminal and terminal['outcome']=='DONE')),'success is not first terminal')
    require(metrics['failed_runs']==int(bool(terminal and terminal['outcome']=='FAILED')),'pause/cancel/block counted as failure')
    rounds={e['correlation']['command_id'] for e in a['events'] if e['kind']=='business_input'}
    require(metrics['effective_business_rounds']==len(rounds),'round identities differ')
    attempts=v['activity_attempts'];require(len({x['attempt_id'] for x in attempts})==len(attempts),'attempt duplicated')
    require(metrics['activity_retries']==len([x for x in attempts if x['mode']=='temporal' and x['attempt']>1]),'actual retry count differs')
    for x in attempts:
        require(x['run_id']==v['run_id'],'cross Run attempt')
        if x['mode']=='temporal':require(x['workflow_id'] and x['execution_run_id'],'missing actual Temporal identity')
        else:require(x['workflow_id'] is None and x['execution_run_id'] is None and x['attempt']==1,'local invented Temporal provenance')
    failures={r['hash'] for e in a['events'] if e['kind']=='verification' and e['outcome']=='failed' for r in e['evidence_refs']}
    require(metrics['verification_failures']==len(failures),'verification failure count differs')


def negative_cases(name,value,oracle):
    oracle.validate(name,value);count=0
    for key in value:
        bad=deepcopy(value);del bad[key]
        try:oracle.validate(name,bad)
        except ContractError:count+=1
        else:raise ContractError('required property accepted: '+name+'.'+key)
    for patch in [{'runtime_version':'9.9.9'},{'extra':True}]:
        if 'runtime_version' in patch and 'runtime_version' not in value:continue
        bad=deepcopy(value);bad.update(patch)
        try:oracle.validate(name,bad)
        except ContractError:count+=1
        else:raise ContractError('closed object accepted: '+name)
    return count
