"""Explicit legacy-only acceptance. Canonical v2 objects use the schema oracle."""
import re

def require(ok,reason):
 if not ok:raise ValueError(reason)

def check_required(run):
 require('id' in run and 'run_id' not in run,'legacy adapter cannot accept canonical v2')
 checks=run.get('required_checks');require(type(checks)is list and bool(checks),'legacy check set absent')
 expected={'approved_sources','artifact_manifest','go mod tidy','go test ./...','go build ./...','file:go.mod','file:.gitignore','file:.gitlab-ci.yml','file:README.md','secret_boundary','zip_integrity'}
 if run['delivery_mode']=='gitlab':expected.update({'remote_repository','remote_commit','gitlab_ci'})
 require(all(type(c)is dict for c in checks),'legacy checks must have explicit bodies')
 names=[c.get('name') for c in checks];require(len(names)==len(expected) and set(names)==expected,'legacy fixed check set differs')
 for c in checks:
  require(type(c.get('passed'))is bool and type(c.get('detail'))is str,'legacy check fields incomplete')
  if 'status' in c:
   require(c['status'] in ['not_ready','running','passed','failed','not_verified'],'unknown legacy status')
   require(c['passed']==(c['status']=='passed'),'legacy passed/status contradict')
  require(c['passed'] is True,'legacy required check failed')
  require(type(c.get('source_version'))is int and c['source_version']==run['source_version'],'legacy check source differs')

def manifest_files(run):
 require('id' in run and 'run_id' not in run,'legacy adapter cannot accept canonical v2')
 m=run.get('manifest');require(type(m)is dict,'legacy manifest absent');entries=m.get('files');require(type(entries)is list and entries,'legacy manifest files must be an array')
 result={}
 for f in entries:
  require(type(f)is dict and set(f)=={'path','size','sha256'},'legacy artifact fields differ; hash alias forbidden')
  name=f['path'];require(type(name)is str and name not in result and not name.startswith('/') and '..' not in name.split('/'),'invalid/duplicate artifact path')
  require(type(f['size'])is int and f['size']>=0,'artifact size must be a nonnegative integer')
  require(type(f['sha256'])is str and re.fullmatch('[0-9a-f]{64}',f['sha256']),'artifact sha256 invalid');result[name]=f['sha256']
 require('go.mod'in result,'manifest missing Go module')
 require(m.get('run_id')==run['id'] and m.get('source_version')==run['source_version'],'legacy manifest source differs')
 require(m.get('snapshot_hash')==run['approval']['snapshot_hash'] and m.get('definition_hash')==run['definition_snapshot']['hash'],'legacy manifest authority differs')
 return result
