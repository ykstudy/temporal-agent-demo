#!/usr/bin/env python3
"""Generate complete TS shapes and exact offline AJV schema inputs from release source."""
import argparse
import hashlib
import json
from pathlib import Path
import sys

ROOT=Path(__file__).resolve().parents[1]
VERSION='1.4.0'
SOURCE=ROOT/'design/contracts/releases'/VERSION
OUT=ROOT/'frontend/src/contracts'
FILES=['common.schema.json','agent.schema.json','message.schema.json']
docs={f:json.loads((SOURCE/f).read_bytes()) for f in FILES}
names={};used=set()
for f in FILES:
    for name in docs[f]['$defs']:
        preferred=name if name not in used else f.split('.')[0].title()+name
        names[f,name]=preferred;used.add(preferred)


def expr(f,s):
    if s is True:return 'unknown'
    if s is False:return 'never'
    if '$ref' in s:
        target,pointer=s['$ref'].split('#');target=target or f
        return names[target,pointer.removeprefix('/$defs/')]
    if 'const' in s:return json.dumps(s['const'],ensure_ascii=False)
    if 'enum' in s:return '('+' | '.join(json.dumps(x,ensure_ascii=False) for x in s['enum'])+')'
    parts=[]
    for union in ['anyOf','oneOf']:
        if union in s:parts.append('('+' | '.join(expr(f,x) for x in s[union])+')')
    if s.get('type')=='object' or 'properties' in s:
        required=s.get('required',[]);props=[]
        for key,child in s.get('properties',{}).items():props.append(json.dumps(key)+('' if key in required else '?')+': '+expr(f,child))
        additional=s.get('additionalProperties',True)
        if additional is not False:props.append('[key: string]: '+expr(f,additional))
        parts.append('{ '+('; '.join(props))+' }')
    elif s.get('type')=='array':parts.append('Array<'+expr(f,s.get('items',True))+'>')
    elif 'type' in s:
        types=s['type'] if isinstance(s['type'],list) else [s['type']]
        parts.append('('+' | '.join({'integer':'number','number':'number','string':'string','boolean':'boolean','null':'null'}.get(t,'unknown') for t in types)+')')
    # Conditionals are retained verbatim in AJV. Unconditional allOf constraints
    # are type intersections, preserving every base and discriminator field.
    for p in s.get('allOf',[]):
        if '$ref' in p or 'type' in p or 'properties' in p:parts.append(expr(f,p))
    return ' & '.join('('+p+')' for p in parts) if parts else 'unknown'

outputs={}
lines=['// GENERATED from complete published JSON Schemas. Do not hand edit.', '// Runtime validation is mandatory; TS cannot encode all conditional constraints.','']
for f in FILES:
    for name,s in docs[f]['$defs'].items():
        lines += ['// '+f+'#/$defs/'+name, 'export type '+names[f,name]+' = '+expr(f,s), '']
outputs[OUT/'types.generated.ts']='\n'.join(lines).encode()
for f in FILES:outputs[OUT/f]=(SOURCE/f).read_bytes()
publication={'contract_version':VERSION,'source':'design/contracts/releases/'+VERSION,'files':[{'file':f,'sha256':hashlib.sha256((SOURCE/f).read_bytes()).hexdigest()} for f in FILES]}
outputs[OUT/'publication.json']=(json.dumps(publication,indent=2)+'\n').encode()
args=argparse.ArgumentParser();args.add_argument('--check',action='store_true');opts=args.parse_args()
for p,b in outputs.items():
    if opts.check:
        if not p.exists() or p.read_bytes()!=b:raise SystemExit('stale web contract: '+str(p))
    else:p.parent.mkdir(parents=True,exist_ok=True);p.write_bytes(b)
print(json.dumps({'passed':True,'contract_version':VERSION,'types':len(names),'exact_schema_files':len(FILES),'check_only':opts.check}))
