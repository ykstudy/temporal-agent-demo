"""Independent complete RunSnapshot semantics, layered on the full published schema."""
from .semantic_checks import require, verify_hash


def verify_snapshot(run, oracle):
    oracle.validate('RunSnapshot', run)
    verify_hash('RunSnapshot', run)
    source, identity, definition = run['source_version'], run['run_id'], run['definition_snapshot']
    verify_hash('DefinitionSnapshot', definition)
    require(run['contract_version'] == definition['contract_version'], 'snapshot release differs from frozen definition')
    if run['modes']['decision_mode']=='rules': require(definition['provider_binding'] is None, 'rules Run unexpectedly binds a model provider')
    else: require(definition['provider_binding'] is not None, 'model Run lacks frozen provider identity')
    if run['plan'] is not None:
        verify_hash('ProjectPlan', run['plan']); require(run['plan_hash'] == run['plan']['plan_hash'], 'plan digest differs')
    else: require(run['plan_hash'] is None, 'no plan but plan hash present')
    for field, name in [('proposal','Proposal'),('approval','Approval'),('permission_snapshot','PermissionSnapshot'),('manifest','ArtifactManifest'),('verification_report','VerificationReport')]:
        item=run[field]
        if item is not None:
            verify_hash(name,item)
            if 'run_id' in item: require(item['run_id']==identity,field+' foreign Run')
            if 'source_version' in item: require(item['source_version']==source,field+' wrong source')
            if 'definition_snapshot_hash' in item: require(item['definition_snapshot_hash']==definition['snapshot_hash'],field+' wrong definition')
    if run['proposal'] is not None: require(run['proposal']['plan']==run['plan'],'proposal plan differs')
    if run['approval'] is not None:
        approval,proposal=run['approval'],run['proposal']
        require(proposal is not None and approval['proposal_id']==proposal['proposal_id'] and approval['proposal_version']==proposal['proposal_version'] and approval['proposal_snapshot_hash']==proposal['snapshot_hash'],'approval is not exact proposal')
    checks=run['required_checks'];expected={c['check_id'] for c in definition['definition']['completion_policy']['required_checks'] if run['modes']['delivery_mode'] in c['delivery_modes']}
    require(len(checks)==len(expected) and {c['check_id'] for c in checks}==expected,'required check set differs from frozen completion policy')
    for check in checks:
        require(check['run_id']==identity and check['source_version']==source and check['definition_snapshot_hash']==definition['snapshot_hash'],'check provenance differs')
        if check['status']=='passed': require(bool(check['evidence_refs']) and check['completed_at'] is not None,'passed check lacks evidence')
    manifest=run['manifest']
    require(run['artifacts']==(manifest['files'] if manifest else []),'artifact list differs from manifest')
    if manifest:
        require(manifest['proposal_snapshot_hash']==run['proposal']['snapshot_hash'] and manifest['plan_hash']==run['plan_hash'],'manifest approval source differs')
    for message in run['messages']:
        require(message['run_id']==identity and message['source_version']<=source and message['state_sequence']<=run['state_sequence'] and message['modes']==run['modes'],'message identity or progress differs')
    require([m['sequence'] for m in run['messages']]==list(range(1,len(run['messages'])+1)),'message source has gaps')
    if run['verification_report'] is not None: require(run['required_checks']==run['verification_report']['checks'],'display checks differ from authoritative report')
    if run['state']=='DONE':
        require(run['verification_report'] is not None and run['verification_report']['outcome']=='passed' and all(c['status']=='passed' for c in checks),'DONE lacks complete passing report')
        require(run['approval'] is not None and manifest is not None and run['error'] is None,'DONE lacks approval or delivery source')
    return {'run_id':identity,'state':run['state'],'source_version':source,'state_sequence':run['state_sequence'],'checks':len(checks)}
