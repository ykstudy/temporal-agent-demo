"""Synthetic legacy adapter negatives; not a runtime or v2 shape claim."""
import argparse,json
from copy import deepcopy
from pathlib import Path
from ..legacy_adapter import check_required,manifest_files

def main():
 p=argparse.ArgumentParser();p.add_argument('--report',type=Path,required=True);a=p.parse_args()
 names=['approved_sources','artifact_manifest','go mod tidy','go test ./...','go build ./...','file:go.mod','file:.gitignore','file:.gitlab-ci.yml','file:README.md','secret_boundary','zip_integrity'];r={'id':'legacy_fixture','source_version':1,'delivery_mode':'local','approval':{'snapshot_hash':'a'*64},'definition_snapshot':{'hash':'b'*64},'required_checks':[{'name':n,'detail':'synthetic check','passed':True,'status':'passed','source_version':1} for n in names],'manifest':{'run_id':'legacy_fixture','source_version':1,'snapshot_hash':'a'*64,'definition_hash':'b'*64,'files':[{'path':'go.mod','size':0,'sha256':'c'*64}]}}
 def check(v):check_required(v);manifest_files(v)
 check(r);cases=[]
 for label,mutate in [('contradictory false status',lambda x:x['required_checks'][0].update(status='failed')),('contradictory false boolean',lambda x:x['required_checks'][0].update(passed=False)),('bool source',lambda x:x['required_checks'][0].update(source_version=True)),('missing detail',lambda x:x['required_checks'][0].pop('detail')),('missing check',lambda x:x['required_checks'].pop()),('extra check',lambda x:x['required_checks'].append(x['required_checks'][0])),('manifest mapping alias',lambda x:x['manifest'].update(files={'go.mod':'c'*64})),('file hash alias',lambda x:x['manifest']['files'][0].update(hash=x['manifest']['files'][0].pop('sha256'))),('missing size',lambda x:x['manifest']['files'][0].pop('size')),('bool size',lambda x:x['manifest']['files'][0].update(size=True)),('canonical v2 input',lambda x:x.update(run_id=x['id']))]:
  v=deepcopy(r);mutate(v)
  try:check(v)
  except ValueError:cases.append(label)
  else:raise AssertionError('negative accepted '+label)
 a.report.parent.mkdir(parents=True,exist_ok=True);a.report.write_text(json.dumps({'passed':True,'kind':'synthetic legacy adapter self-test','negatives':cases},indent=2)+'\n');print(json.dumps({'passed':True,'negative_cases':len(cases)}))
if __name__=='__main__':main()
