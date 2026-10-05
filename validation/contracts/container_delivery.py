"""Build and start the exact public ZIP's Dockerfile; clean up only owned resources."""
import argparse,hashlib,io,json,os,subprocess,time,urllib.request,uuid,zipfile
from datetime import datetime,timezone
from pathlib import Path,PurePosixPath
from .oracle import strict_loads,byte_hash
from .semantic_checks import require

def main():
 p=argparse.ArgumentParser();p.add_argument('--delivery-report',type=Path,required=True);p.add_argument('--report-dir',type=Path,required=True);a=p.parse_args();out=a.report_dir.resolve();out.mkdir(parents=True,exist_ok=False)
 run=strict_loads((a.delivery_report/'done.json').read_bytes());raw=(a.delivery_report/'delivery.zip').read_bytes();root=run['plan']['project_name'];expected={x['path']:x for x in run['manifest']['files']};project=out/'project';project.mkdir();tag='agentstudio-s5-'+uuid.uuid4().hex[:12];image=tag+':acceptance';container=None;report={'at':datetime.now(timezone.utc).isoformat(),'status':'running','run_id':run['run_id'],'package_sha256':byte_hash(raw),'manifest_hash':run['manifest']['hash'],'image_tag':image,'evidence_kind':'actual_container_build_and_loopback_runtime'}
 def cmd(args,log,timeout=120):
  with (out/log).open('ab') as f:r=subprocess.run(args,cwd=project,stdout=f,stderr=subprocess.STDOUT,timeout=timeout)
  require(r.returncode==0,'command failed; inspect '+log)
 try:
  names=set()
  with zipfile.ZipFile(io.BytesIO(raw)) as z:
   for item in z.infolist():
    parts=PurePosixPath(item.filename).parts;require(parts and parts[0]==root and '..' not in parts and not item.filename.startswith('/') and not item.is_dir(),'invalid package path');rel='/'.join(parts[1:]);require(rel not in names,'duplicate archive entry');names.add(rel);body=z.read(item)
    if rel=='AGENT-MANIFEST.json':require(strict_loads(body)==run['manifest'],'embedded manifest differs')
    else:require(rel in expected and len(body)==expected[rel]['size'] and byte_hash(body)==expected[rel]['sha256'],'package file mismatch')
    dest=project/rel;dest.parent.mkdir(parents=True,exist_ok=True);dest.write_bytes(body)
  require(names==set(expected)|{'AGENT-MANIFEST.json'},'package file set differs')
  cmd(['docker','build','--iidfile',str(out/'image-id'),'-t',image,'.'],'build.log',900)
  report['image_id']=(out/'image-id').read_text().strip()
  r=subprocess.run(['docker','run','-d','--name',tag,'--read-only','--tmpfs','/tmp','--cap-drop','ALL','--security-opt','no-new-privileges','-e','DATABASE_DSN=/tmp/validation.db','-p','127.0.0.1::8080',image],capture_output=True,text=True,timeout=60)
  (out/'run.log').write_text(r.stdout+r.stderr);require(r.returncode==0,'container start failed');container=r.stdout.strip();report['container_id']=container
  mapping=json.loads(subprocess.check_output(['docker','inspect','--format','{{json .NetworkSettings.Ports}}',container],text=True));port=mapping['8080/tcp'][0]['HostPort'];url='http://127.0.0.1:'+port+'/healthz';deadline=time.monotonic()+40
  while True:
   try:
    with urllib.request.urlopen(url,timeout=2) as res:health=json.load(res);require(res.status==200,'health failed')
    break
   except Exception:
    if time.monotonic()>=deadline:raise
    time.sleep(.5)
  report.update(status='passed',health=health,health_url=url,source_files=len(expected),dockerfile_sha256=byte_hash((project/'Dockerfile').read_bytes()))
 except Exception as e:report.update(status='failed',error=str(e));raise
 finally:
  if container:
   (out/'container.log').write_bytes(subprocess.check_output(['docker','logs',container],stderr=subprocess.STDOUT))
   r=subprocess.run(['docker','rm','-f',container],capture_output=True,text=True,timeout=60);report['owned_container_removed']=r.returncode==0
  r=subprocess.run(['docker','image','rm',image],capture_output=True,text=True,timeout=60);report['owned_image_tag_removed']=r.returncode==0
  (out/'summary.json').write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n');print(json.dumps(report,ensure_ascii=False))
if __name__=='__main__':main()
