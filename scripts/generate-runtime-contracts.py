#!/usr/bin/env python3
"""Derive all fields of the independent runtime publication; exact offline copies."""
import argparse,hashlib,json,re,subprocess
from pathlib import Path
ROOT=Path(__file__).resolve().parents[1]
SOURCE=ROOT/'design/contracts/runtime/1.0.0/runtime.schema.json'
b=SOURCE.read_bytes();doc=json.loads(b);defs=doc['$defs']
init={'id':'ID','ids':'IDs','sha256':'SHA256','ms':'MS','url':'URL','ci':'CI'}
def pascal(k):return ''.join(init.get(x,x[:1].upper()+x[1:]) for x in k.split('_'))
def go(s):
 if '$ref' in s:return s['$ref'].split('/')[-1]
 if 'anyOf' in s:
  other=[x for x in s['anyOf'] if x.get('type')!='null'];assert len(other)==1
  return '*'+go(other[0])
 if s.get('type')=='array':return '[]'+go(s['items'])
 if 'const' in s or 'enum' in s:return 'string'
 return {'string':'string','integer':'int64','boolean':'bool','number':'float64'}[s['type']]
def ts(s):
 if '$ref' in s:return ('' if s['$ref'].startswith('#') else 'C.')+s['$ref'].split('/')[-1]
 if 'anyOf' in s:return '('+' | '.join(ts(x) for x in s['anyOf'])+')'
 if 'const' in s:return json.dumps(s['const'])
 if 'enum' in s:return '('+' | '.join(json.dumps(v) for v in s['enum'])+')'
 if s.get('type')=='array':return 'Array<'+ts(s['items'])+'>'
 if s.get('type')=='object':return '{ '+'; '.join(json.dumps(k)+': '+ts(v) for k,v in s['properties'].items())+' }'
 return {'string':'string','integer':'number','boolean':'boolean','number':'number','null':'null'}[s['type']]
g=['// Code generated from runtime/1.0.0/runtime.schema.json; DO NOT EDIT.','package contracts','']
t=['// GENERATED complete runtime publication. Validate every boundary.',"import type * as C from './types.generated'",'']
for n,s in defs.items():
 assert s['type']=='object' and set(s['required'])==set(s['properties'])
 g+=['type '+n+' struct {']+['\t'+pascal(k)+' '+go(v)+' `json:"'+k+'"`' for k,v in s['properties'].items()]+['}','']
 t+=['export type '+n+' = '+ts(s),'']
g+=['const RuntimeSchemaHash = "'+hashlib.sha256(b).hexdigest()+'"','const RuntimeSchemaID = "'+doc['$id']+'"','']
gofmt=subprocess.run(['gofmt'],input='\n'.join(g).encode(),capture_output=True,check=True).stdout
out={ROOT/'backend/internal/contracts/runtime_types_generated.go':gofmt,ROOT/'backend/internal/contracts/runtime_schemas/runtime.schema.json':b,ROOT/'frontend/src/contracts/runtime.generated.ts':'\n'.join(t).encode(),ROOT/'frontend/src/contracts/runtime.schema.json':b,ROOT/'validation/contracts/runtime/1.0.0/runtime.schema.json':b}
pub={'runtime_version':'1.0.0','source':str(SOURCE.relative_to(ROOT)),'sha256':hashlib.sha256(b).hexdigest(),'references':{}}
for f in ['agent.schema.json','common.schema.json','message.schema.json']:
 src=ROOT/'design/contracts/releases/1.4.0'/f;out[ROOT/'validation/contracts/runtime/1.0.0'/f]=src.read_bytes();pub['references'][f]=hashlib.sha256(src.read_bytes()).hexdigest()
for path in [ROOT/'frontend/src/contracts/runtime-publication.json',ROOT/'validation/contracts/runtime/1.0.0/publication.json']:out[path]=(json.dumps(pub,indent=2)+'\n').encode()
a=argparse.ArgumentParser();a.add_argument('--check',action='store_true');o=a.parse_args()
for p,v in out.items():
 if o.check:
  if not p.exists() or p.read_bytes()!=v:raise SystemExit('stale runtime publication: '+str(p))
 else:p.parent.mkdir(parents=True,exist_ok=True);p.write_bytes(v)
print(json.dumps({'passed':True,'complete_types':len(defs),'schema_sha256':pub['sha256'],'check_only':o.check}))
