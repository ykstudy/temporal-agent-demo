"""Check documented validation CLI examples and the public mock health route."""
import argparse
from datetime import datetime, timezone
from http.server import ThreadingHTTPServer
import json
from pathlib import Path
import re
import shlex
import subprocess
import sys
import tempfile
import threading
import urllib.error
import urllib.request
from validation.fixtures import FixtureState, handler

ROOT = Path(__file__).resolve().parents[2]


def main():
    p=argparse.ArgumentParser();p.add_argument('--report',type=Path,required=True);args=p.parse_args()
    readme=(ROOT/'validation/README.md').read_text(); examples=[]; help_flags={}
    for block in re.findall(r'```sh\n(.*?)```',readme,re.S):
        for line in block.splitlines():
            words=shlex.split(line)
            if 'python3' not in words:continue
            words=words[words.index('python3')+1:];script=words[0]
            if not script.startswith('validation/'):continue
            if script not in help_flags:
                helptext=subprocess.check_output([sys.executable,str(ROOT/script),'--help'],text=True)
                help_flags[script]=set(re.findall(r'--[a-z][a-z-]*',helptext))
            unknown={w.split('=',1)[0] for w in words[1:] if w.startswith('--')}-help_flags[script]
            assert not unknown, 'documentation has unknown CLI flags: '+str(unknown)
            examples.append({'script':script,'flags_verified':True})
    assert examples and '/fixture/health' in readme and '/fixture/healthz' not in readme
    with tempfile.TemporaryDirectory() as directory:
        server=ThreadingHTTPServer(('127.0.0.1',0),handler(FixtureState(Path(directory),False)))
        thread=threading.Thread(target=server.serve_forever,daemon=True);thread.start()
        base='http://127.0.0.1:'+str(server.server_port)
        try:
            with urllib.request.urlopen(base+'/fixture/health',timeout=5) as r:
                identity=json.load(r);assert r.status==200 and identity['provider_kind']=='mock'
            try:urllib.request.urlopen(base+'/fixture/healthz',timeout=5)
            except urllib.error.HTTPError as e:assert e.code==404
            else:raise AssertionError('typo route accepted')
        finally:server.shutdown();server.server_close();thread.join()
    report=dict(passed=True,at=datetime.now(timezone.utc).isoformat(),scope='G21 documentation CLI flags and actual mock public health route; no live or product acceptance',examples=examples,correct_route='/fixture/health',typo_route_rejected=True)
    args.report.parent.mkdir(parents=True,exist_ok=True);args.report.write_text(json.dumps(report,indent=2)+'\n');print(json.dumps(report))


if __name__=='__main__':main()
