"""Independent 1.4/runtime fixtures for shapes absent from normal DONE output.

All identities below are synthetic. These validate the published language, not
historical provenance, actual side effects or provider success.
"""
import argparse
from datetime import datetime, timezone
import json
from pathlib import Path
from .oracle import ContractError, byte_hash
from .runtime_oracle import RuntimeOracle
from .self_test import mutation_cases, replace


def fixtures():
    h='a'*64
    ref=lambda kind:dict(object_type=kind,object_id='synthetic_'+kind,object_version=1,source_version=1,hash=h)
    part=lambda kind:dict(part_id='synthetic_'+kind,part_version=1,type=kind,fallback_text='Synthetic contract example')
    intent=dict(command_id='synthetic_accept',action='accept',target=ref('run'),required_proposal_ref=ref('proposal'))
    choice=dict(choice_id='synthetic_choice',label='Example',description='Synthetic')
    diff=dict(path='go.mod',operation='update',before='old',after='new')
    selection=dict(definition_id='synthetic_agent',definition_version='1.0.0',definition_hash=h)
    candidate_file=dict(path='go.mod',content='module example.invalid/synthetic\n',size=33,sha256=h)
    candidate=dict(contract_version='1.4.0',run_id='synthetic_run',source_version=1,definition_snapshot_hash=h,
                   execution_id='synthetic_execution',operation_id='synthetic_operation',attempt_id='synthetic_attempt',request_hash=h,
                   approval_id='synthetic_approval',approval_hash=h,proposal_snapshot_hash=h,plan_hash=h,
                   workspace=dict(workspace_id='synthetic_workspace',run_id='synthetic_run',storage_key='synthetic/workspace'),
                   candidate_id='synthetic_candidate',input_manifest_ref=ref('manifest'),tool_receipt_refs=[ref('evidence')],
                   files=[candidate_file],changed_paths=['go.mod'],total_bytes=33,created_at='2026-10-05T00:00:00Z',candidate_hash=h)
    return {
        'ActionIntent':intent,'ControlRequest':intent,'ActionPart':dict(**part('action'),label='Example',intent=intent),
        'Choice':choice,'ChoicePart':dict(**part('choice'),prompt='Choose an example',selection_mode='single',choices=[choice]),
        'DiffChange':diff,'DiffPart':dict(**part('diff'),object_ref=ref('proposal'),summary='Synthetic diff',changes=[diff]),
        'ArtifactManifestPayload':dict(manifest_ref=ref('manifest'),artifact_count=1,summary='Synthetic artifact'),
        'VerificationReportPayload':dict(report_ref=ref('verification_report'),result='not_ready',summary='Synthetic report'),
        'ClarificationPayload':dict(question='Provide a name',field_ids=['project_name']),
        'DefinitionSelection':selection,
        'CreateRunRequest':dict(contract_version='1.4.0',schema_version=2,command_id='synthetic_create',run_id='synthetic_run',title='Synthetic',modes=dict(execution_mode='local',decision_mode='rules',delivery_mode='local'),definition_selection=selection,initial_content='Synthetic project'),
        'MessageRequest':dict(contract_version='1.4.0',schema_version=2,command_id='synthetic_message',target=ref('run'),content='Synthetic message'),
        'Diagnosis':dict(category='business',code='synthetic_diagnosis',message='Synthetic failure',evidence_refs=[],suggested_remedy='Inspect evidence'),
        'Failure':dict(category='business',code='synthetic_failure',message='Synthetic failure',operation_id=None,evidence_refs=[]),
        'ErrorResponse':dict(error='Synthetic legacy error',code='legacy_contract_incomplete',legacy_read_url='/api/runs/synthetic_run'),
        'SnapshotEvent':dict(contract_version='1.4.0',schema_version=2,event_id='synthetic_event',run_id='synthetic_run',source_version=1,state_sequence=1,run_ref=ref('run'),occurred_at='2026-10-05T00:00:00Z'),
        'SummaryAdvice':dict(items=[dict(message_id='synthetic_message',excerpt='Synthetic source quote')]),
        'VerificationCandidate':candidate,'VerificationCandidateFile':candidate_file,
        'WorkflowCheckpoint':dict(workflow_id='synthetic_workflow',execution_run_id='synthetic_execution',commands_processed=1,query_state='PLAN',checkpoint_state_sequence=1),
        'JsonValue':{'example':[None,True,1,'text']},
    }


def main():
    p=argparse.ArgumentParser();p.add_argument('--report',type=Path,required=True);args=p.parse_args()
    oracle=RuntimeOracle();cases=[];values=fixtures()
    for name,value in values.items():
        file=next(f for f,s in oracle.schemas.items() if name in s['$defs']);oracle.validate(name,value,file)
        # JsonValue deliberately allows every JSON type; test its actual limits.
        if name=='JsonValue':
            mutations=[('string bound',(), 'x'*8193),('array bound',(),[None]*65),('object bound',(),{str(i):None for i in range(65)}),('property bound',(),{'x'*65:None})]
        else:mutations=mutation_cases(oracle,value,name,file,False)
        for keyword,path,bad in mutations:
            try:oracle.validate(name,bad,file)
            except ContractError:pass
            else:
                if keyword not in ['const','type']:raise AssertionError(f'accepted {name} {keyword} {path}')
                for alternative in [True,0,[],{'__undeclared__':True},None,'__invalid__']:
                    try:oracle.validate(name,replace(value,path,alternative),file)
                    except ContractError:break
                else:raise AssertionError('union has no invalid test value '+name)
            cases.append(dict(schema=name,keyword=keyword,path=list(path)))
    report=dict(passed=True,at=datetime.now(timezone.utc).isoformat(),scope='explicit synthetic full-schema instances and structural negatives; no product runtime or semantic provenance claim',schema_sha256={k:byte_hash(v) for k,v in oracle.raw.items()},shapes=sorted(values),negative_count=len(cases),cases=cases)
    args.report.parent.mkdir(parents=True,exist_ok=True);args.report.write_text(json.dumps(report,indent=2)+'\n');print(json.dumps({k:v for k,v in report.items() if k!='cases'}))


if __name__=='__main__':main()
