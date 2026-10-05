#!/usr/bin/env python3
"""Read-only live prerequisites. Never prints credentials or approves a Run."""
import argparse
from datetime import datetime, timezone
import json
import os
from pathlib import Path
from urllib.parse import urlsplit


def inspect_environment(env, namespace=None, project=None):
    keys=['AI_BASE_URL','AI_API_KEY','AI_MODEL','GITLAB_URL','GITLAB_TOKEN','GITLAB_NAMESPACE_ID']
    present={key:bool(env.get(key,'').strip()) for key in keys}
    blockers=[{'code':'missing_configuration','field':key} for key,yes in present.items() if not yes]
    for key in ['AI_BASE_URL','GITLAB_URL']:
        if not present[key]:continue
        value=urlsplit(env[key])
        if value.scheme not in ['http','https'] or not value.hostname or value.username or value.password or value.query or value.fragment:
            blockers.append({'code':'invalid_or_credential_bearing_endpoint','field':key})
    if not namespace or not project:
        blockers.append({'code':'authorized_test_target_missing','fields':['namespace_id','project_path']})
    elif namespace!=env.get('GITLAB_NAMESPACE_ID'):
        blockers.append({'code':'configured_namespace_differs_from_authorized_target'})
    return {'configuration_present':present,'blockers':blockers,
            'status':'external_blocked' if blockers else 'configured_unverified',
            'live_model':'not_run','gitlab_permissions':'not_run','runner':'not_run',
            'per_run_approval':'required_after_exact_proposal','external_writes':0}


def main():
    p=argparse.ArgumentParser(description=__doc__)
    p.add_argument('--report',type=Path,required=True)
    p.add_argument('--authorized-namespace-id',help='Only after the user specifies this test namespace')
    p.add_argument('--project-path',help='Only after the user specifies this dedicated test project path')
    args=p.parse_args()
    report=inspect_environment(os.environ,args.authorized_namespace_id,args.project_path)
    report.update(at=datetime.now(timezone.utc).isoformat(),scope='local presence and target binding only; no provider request, real Runner, model inference or remote write')
    args.report.parent.mkdir(parents=True,exist_ok=True);args.report.write_text(json.dumps(report,indent=2)+'\n')
    print(json.dumps(report))
    return 2 if report['blockers'] else 0


if __name__=='__main__':raise SystemExit(main())
