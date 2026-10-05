"""S2 C07-C10 public HTTP scenarios; actual Temporal and launcher are explicit."""
from concurrent.futures import ThreadPoolExecutor
from datetime import datetime, timezone
import argparse
import io
import json
import os
from pathlib import Path
import signal
import socket
import subprocess
import time
import urllib.error
import urllib.parse
import urllib.request
import uuid
import zipfile

from .oracle import Oracle, byte_hash, strict_loads
from .semantic_checks import require, verify_hash
from .s2_semantics import verify_audit, audit_objects
from .decision_semantics import instant

HERE = Path(__file__).resolve().parent


class Scenarios:
    def __init__(self, args):
        self.args = args
        self.directory = args.report.resolve(); self.directory.mkdir(parents=True, exist_ok=False)
        version="1.4.0" if getattr(args,"definition_version","3.2.0")=="3.3.0" else "1.3.0"
        self.oracle = Oracle(HERE / ("releases/"+version), HERE.parents[1] / ("design/contracts/releases/"+version))
        self.prefix = "s2_" + uuid.uuid4().hex[:10]
        self.report = {"at": datetime.now(timezone.utc).isoformat(), "evidence_kind": "public_http_s2", "execution_mode": args.mode, "provider_kind": "mock", "contract_version": version, "cases": [], "runs": [], "observations": [], "not_covered": ["S3-S5 complete v2", "live model/GitLab/Runner", "browser", "production"]}
        self.counter = 0
        self.content = {}
        definitions = self.http("GET", "/api/definitions", expected=200)
        self.definition = next(d for d in definitions if d["definition_version"] == getattr(args,"definition_version","3.2.0"))

    def http(self, method, path, payload=None, expected=200, origin=None, private_headers=None):
        body = None if payload is None else json.dumps(payload, ensure_ascii=False).encode()
        headers = {"Content-Type": "application/json", **(private_headers or {})}
        req = urllib.request.Request((origin or self.args.base_url).rstrip("/") + path, data=body, method=method, headers=headers)
        try: response = urllib.request.urlopen(req, timeout=210)
        except urllib.error.HTTPError as error: response = error
        with response:
            raw = response.read(16*1024*1024+1)
            require(len(raw)<=16*1024*1024, "response size limit")
            record = {"at": datetime.now(timezone.utc).isoformat(), "method":method, "path":path, "status":response.status, "request":payload, "response_sha256":byte_hash(raw)}
            try: value = strict_loads(raw); record["response"] = value
            except ValueError: value = raw
            # Authentication headers never enter evidence, including launcher.
            with (self.directory/"http.jsonl").open("a") as f: f.write(json.dumps(record, ensure_ascii=False)+"\n")
            require(response.status==expected, "unexpected status "+str(response.status)+" "+str(value)[:700])
            return value

    def configure(self, run, mode):
        self.http("POST", "/fixture/control", {"run_id":run,"mode":mode}, origin=self.args.fixture_url)

    def options(self, label, mode="model", content=None):
        return {"id":self.prefix+"_"+label, "content":content or "创建 s2-project", "execution_mode":self.args.mode, "decision_mode":mode, "delivery_mode":"local", **{k:self.definition[k] for k in ("definition_id","definition_version","definition_hash")}}

    def create(self, options, expected=201):
        result = self.http("POST", "/api/runs", options, expected=expected)
        if options["id"] not in self.report["runs"]: self.report["runs"].append(options["id"])
        return result

    def command(self, run, id_, action=None, content=None, proposal=None, expected=200):
        body = {"id":id_}
        if action: body["action"]=action
        if content is not None: body["content"]=content
        if proposal:
            body.update(proposal_id=proposal["proposal_id"],proposal_version=proposal["proposal_version"],source_version=proposal["source_version"],snapshot_hash=proposal["snapshot_hash"])
        return self.http("POST", "/api/runs/"+run+("/actions" if action else "/messages"), body, expected=expected)

    def audit(self, run, label="audit", full=False):
        value = self.http("GET", "/api/runs/"+run+"/decision-audit")
        self.oracle.validate("S2AuditBundle",value); verify_hash("S2AuditBundle",value)
        self.counter+=1
        path=self.directory/(f"{self.counter:03d}-"+run+"-"+label+".json")
        path.write_text(json.dumps(value,ensure_ascii=False,indent=2)+"\n")
        if full:
            # Content hashes require original HTTP bytes, never JSON re-encode.
            def bytes_load(ref):
                if ref["hash"] not in self.content:
                    query=urllib.parse.urlencode({k:ref[k] for k in ("object_type","object_id","hash")})
                    with urllib.request.urlopen(self.args.base_url+"/api/runs/"+run+"/definition/content?"+query,timeout=30) as response:
                        raw=response.read(16*1024*1024+1)
                        require(byte_hash(raw)==ref["hash"] and response.headers.get("X-Content-SHA256")==ref["hash"],"frozen public content bytes/header mismatch")
                        self.content[ref["hash"]]=raw
                return self.content[ref["hash"]]
            result=verify_audit(value,self.oracle,bytes_load)
            result.update(run_id=run,state=value["state"],evidence=str(path))
            self.report["observations"].append(result)
        return value

    def wait(self, run, states, seconds=480, reapprove=False):
        deadline=time.monotonic()+seconds; accepted=set()
        while time.monotonic()<deadline:
            value=self.http("GET","/api/runs/"+run)
            if value["state"] in states: return value
            if reapprove and value["state"]=="PLAN":
                audit=self.audit(run,"candidate-plan",True); p=audit["proposal"]
                require(p["plan"].get("verification_candidate_ref") is not None and p["proposal_id"] not in accepted,"unexpected repeated unapproved plan")
                accepted.add(p["proposal_id"])
                self.command(run,"accept_candidate_"+p["proposal_id"],"accept",proposal=p)
            if value["state"] in ("FAILED","BLOCKED","CANCELLED"): raise AssertionError("unexpected execution stop "+json.dumps(value)[-1000:])
            time.sleep(.25)
        raise AssertionError("Run timeout "+run)

    def case(self,name,fn):
        print("running "+name,flush=True)
        try: facts=fn(); self.report["cases"].append({"case":name,"status":"passed","facts":facts})
        except Exception as error: self.report["cases"].append({"case":name,"status":"failed","error":str(error)})
        self.save()

    def save(self):
        self.report["passed"]=all(c["status"]=="passed" for c in self.report["cases"])
        (self.directory/"summary.json").write_text(json.dumps(self.report,ensure_ascii=False,indent=2)+"\n")

    def model_failure(self):
        opts=self.options("failure"); self.configure(opts["id"],"fail_next")
        self.create(opts,503)
        failed=self.audit(opts["id"],"provider-failed",True)
        inputs=[m for m in failed["messages"] if m["role"]=="user"]
        require(len(inputs)==1 and failed["plan"] is None and failed["budget"]["totals"]["consumed"]["model"]==1,"failed advice not persisted/charged")
        r=self.create(opts); require(r["state"]=="PLAN","same-identity retry did not recover")
        before=self.audit(r["id"],"retried",True)
        self.create(opts); after=self.audit(r["id"],"create-replayed")
        require(before==after,"successful create replay changed canonical objects")
        self.create({**opts,"content":"创建 other-project"},409)
        require(before==self.audit(r["id"],"conflict"),"conflicting Create changed Run")
        require(next(m for m in before["messages"] if m["role"]=="user")==inputs[0],"retry replaced original input")
        return {"run_id":r["id"],"same_input_message":inputs[0]["message_id"],"failed_usage":"charged"}

    def malformed(self):
        ids=[]
        for mode in ("forged_approval","shell","fake_execution"):
            opts=self.options(mode); self.configure(opts["id"],mode); self.create(opts,400)
            a=self.audit(opts["id"],mode,True)
            require(not a["executions"] and a["approval"] is None and not a["operations"] and a["manifest"] is None,"malformed provider authorized side effect")
            ids.append(opts["id"])
        return {"run_ids":ids,"side_effect_requests":0}

    def bindings(self):
        r=self.create(self.options("bindings",content="创建 bindings-project")); run=r["id"]
        a=self.audit(run,"first-plan",True); old=a["proposal"]
        self.command(run,"change_plan",content="创建 bindings-project，JSON 日志")
        a=self.audit(run,"changed-plan",True); new=a["proposal"]
        require(a["source_version"]==old["source_version"]+1 and new["snapshot_hash"]!=old["snapshot_hash"],"new scope did not change source")
        self.command(run,"stale_accept","accept",proposal=old,expected=409)
        bad=dict(new); bad["snapshot_hash"]="f"*64
        self.command(run,"bad_hash_accept","accept",proposal=bad,expected=409)
        unchanged=self.audit(run,"rejected-confirmations",True)
        require(unchanged["approval"] is None and not unchanged["executions"],"unbound approval started execution")
        self.command(run,"cancel_boundaries","cancel")
        return {"run_id":run,"old_source":old["source_version"],"new_source":new["source_version"]}

    def invalid_action(self):
        r=self.create(self.options("invalid_action")); run=r["id"]; a=self.audit(run)
        self.configure(run,"wrong_plan")
        self.command(run,"accept_for_rejection","accept",proposal=a["proposal"])
        self.wait(run,{"BLOCKED"})
        a=self.audit(run,"rejected-action",True)
        require(not a["executions"] and not a["operations"] and a["manifest"] is None and any(v["verdict"]=="rejected" for v in a["validations"]),"valid-schema unauthorized action reached executor")
        time.sleep(5)
        stable=self.audit(run,"refusal-not-auto-retried",True)
        require(stable["state"]=="BLOCKED" and len(stable["decisions"])==len(a["decisions"]) and len(stable["budget"]["reservations"])==len(a["budget"]["reservations"]) and not stable["executions"],"business refusal automatically retried model or executor")
        self.command(run,"cancel_invalid_action","cancel")
        return {"run_id":run,"rejected_validations":sum(v["verdict"]=="rejected" for v in a["validations"])}

    def expiry(self):
        r=self.create(self.options("expiry","rules")); run=r["id"]; a=self.audit(run); p=a["proposal"]
        deadline=instant(p["expires_at"])
        while instant(datetime.now(timezone.utc).isoformat())<deadline: time.sleep(.25)
        self.command(run,"accept_expired","accept",proposal=p,expected=409)
        a=self.audit(run,"expired",True); require(a["approval"] is None and not a["executions"],"expired approval authorized action")
        self.command(run,"refresh_plan",content="创建 s2-project")
        new=self.audit(run,"reissued",True)
        require(new["source_version"]==a["source_version"] and new["proposal"]["proposal_version"]==p["proposal_version"]+1 and new["proposal"]["issued_at"]!=p["issued_at"],"same-plan expiry refreshed wrong identity")
        self.command(run,"old_accept_again","accept",proposal=p,expected=409)
        self.command(run,"cancel_expiry","cancel")
        return {"run_id":run,"same_source":a["source_version"],"new_proposal_version":new["proposal"]["proposal_version"]}

    def completion(self, all_skills=False, existing_run=None):
        if existing_run:
            run=existing_run; self.report["runs"].append(run)
            self.report["continued_public_run"]=run
        else:
            r=self.create(self.options("complete", "rules" if all_skills else "model", "创建 s2-project，SQLite + GORM、JWT 认证和 JSON 日志" if all_skills else None)); run=r["id"]
        a=self.audit(run,"preview")
        if a["state"]!="DONE":
            require(a["approval"] is None and not a["executions"],"planning executed without P1")
            if instant(a["proposal"]["expires_at"])<=instant(datetime.now(timezone.utc).isoformat()):
                old=a["proposal"]
                self.command(run,"refresh_expired_delivery_"+old["proposal_id"],content=a["messages"][0]["parts"][0]["text"])
                a=self.audit(run,"reissued-delivery")
                require(a["source_version"]==old["source_version"] and a["proposal"]["proposal_version"]==old["proposal_version"]+1,"delivery expiry changed original scope")
            self.command(run,"approve_complete_"+a["proposal"]["proposal_id"],"accept",proposal=a["proposal"])
            self.wait(run,{"DONE"},reapprove=True)
        a=self.audit(run,"DONE",True)
        require(len(a["verification_report"]["checks"])==11 and all(c["status"]=="passed" for c in a["verification_report"]["checks"]),"DONE report missing required checks")
        if all_skills: require(len(a["plan"]["selected_skills"])==3,"skills omitted from actual complete path")
        package=a["package_receipts"][-1]; ref=package["package_ref"]
        raw=self.http("GET","/api/runs/"+run+"/objects?"+urllib.parse.urlencode(ref))
        from .s2_raw_artifacts import verify_raw
        verify_raw(raw,{"ref":ref,"sha256":package["package_sha256"],"byte_count":package["package_bytes"],"format":"zip","manifest":a["manifest"]})
        (self.directory/"delivery.zip").write_bytes(raw)
        if all_skills: self.runtime(raw,a["plan"]["project_name"])
        retry=self.command(run,"retry_completed","retry")
        require(retry["id"]!=run and retry["retry_of"]==run and retry["state"]=="PLAN","terminal redo did not create fresh complete Run")
        self.report["runs"].append(retry["id"])
        new=self.audit(retry["id"],"redo",True)
        require(new["approval"] is None and not new["executions"] and new["plan"]["plan_hash"]==a["plan"]["plan_hash"] and not new["budget"]["ci_windows"],"terminal redo inherited authority")
        replay=self.command(run,"retry_completed","retry"); require(replay["id"]==retry["id"],"retry duplicate changed identity")
        return {"run_id":run,"successor":retry["id"],"complete_checks":11,"package_sha256":byte_hash(raw),"all_skills":all_skills}

    def runtime(self, raw, name):
        project=self.directory/"generated"; project.mkdir()
        with zipfile.ZipFile(io.BytesIO(raw)) as archive: archive.extractall(project)
        root=project/name; binary=self.directory/"generated-server"
        env={**os.environ,"GOTOOLCHAIN":"local","CGO_ENABLED":"0"}
        with (self.directory/"generated-runtime.log").open("w") as log:
            build=subprocess.run(["go","build","-o",str(binary),"./cmd/server"],cwd=root,env=env,stdout=log,stderr=subprocess.STDOUT,timeout=240)
            require(build.returncode==0,"delivered project runtime build failed")
            with socket.socket() as probe: probe.bind(("127.0.0.1",0)); port=probe.getsockname()[1]
            env.update(PORT=str(port),HTTP_ADDR="127.0.0.1:"+str(port),JWT_SECRET="s2-runtime-test-secret-32-characters")
            process=subprocess.Popen([str(binary)],cwd=root,env=env,stdout=log,stderr=subprocess.STDOUT,start_new_session=True)
            try:
                deadline=time.monotonic()+20
                while time.monotonic()<deadline:
                    require(process.poll() is None,"generated runtime exited")
                    try:
                        with urllib.request.urlopen("http://127.0.0.1:"+str(port)+"/healthz",timeout=1) as response: health=json.load(response)
                        require(health.get("status")=="ok" and health.get("project")==name,"generated health status/project mismatch")
                        require(health.get("logging",{}).get("ready") is True,"generated logging is not ready")
                        (self.directory/"runtime-health.json").write_text(json.dumps({"body":health,"binary_sha256":byte_hash(binary.read_bytes())})+"\n")
                        return
                    except OSError: time.sleep(.1)
                raise AssertionError("generated health timeout")
            finally:
                if process.poll() is None: os.killpg(process.pid,signal.SIGTERM); process.wait(timeout=10)

    def temporal_can(self):
        r=self.create(self.options("can","rules")); run=r["id"]
        old=self.http("GET","/api/runs/"+run+"/workflow")
        first=self.audit(run,"CAN-before",True)
        for i in range(21): self.command(run,"can_input_"+str(i),content="查看进度")
        deadline=time.monotonic()+20
        while time.monotonic()<deadline:
            new=self.http("GET","/api/runs/"+run+"/workflow")
            if new["execution_run_id"]!=old["execution_run_id"]: break
            time.sleep(.2)
        require(new["execution_run_id"]!=old["execution_run_id"] and new["workflow_id"]==old["workflow_id"],"real Continue-As-New absent")
        self.command(run,"can_input_0",content="查看进度")
        def submit(i): return self.command(run,"parallel_"+str(i),content="查看进度")
        with ThreadPoolExecutor(max_workers=11) as pool: list(pool.map(submit,[0,1,2,3,4,5,6,7,8,0,1]))
        a=self.audit(run,"CAN-after",True)
        inputs=[m for m in a["messages"] if m["role"]=="user"]
        require(a["messages"][:len(first["messages"])]==first["messages"] and len(inputs)==31 and len({m["command_id"] for m in inputs})==31 and a["source_version"]==first["source_version"],"CAN/parallel changed original messages, duplicates or source")
        self.command(run,"cancel_can","cancel")
        return {"run_id":run,"workflow_id":new["workflow_id"],"before_execution":old["execution_run_id"],"after_execution":new["execution_run_id"],"unique_messages":31}

    def crash(self):
        require(self.args.launcher_state is not None,"explicit owned launcher required")
        launcher=json.loads(self.args.launcher_state.read_text())
        require(launcher["owned_process"] and launcher["base_url"]==self.args.base_url,"launcher ownership mismatch")
        r=self.create(self.options("crash","rules", "创建 crash-project，SQLite + GORM、JWT 认证和 JSON 日志")); run=r["id"]; a=self.audit(run)
        self.command(run,"approve_crash","accept",proposal=a["proposal"])
        deadline=time.monotonic()+240; pending=[]
        while time.monotonic()<deadline:
            current=self.http("GET","/api/runs/"+run)
            if current["state"]=="VERIFY" and current.get("active_step"):
                a=self.audit(run,"VERIFY-before-crash")
                settled={s["reservation_id"] for s in a["budget"]["settlements"]}
                pending=[r for r in a["budget"]["reservations"] if r["kind"] in ("action","tool") and r["reservation_id"] not in settled]
                if any(r["kind"]=="tool" for r in pending): break
            require(current["state"] not in ("DONE","BLOCKED","FAILED"),"crash boundary missed")
            time.sleep(.05)
        require(any(r["kind"]=="tool" for r in pending),"durable VERIFY tool reservation not observed")
        # The controller accepts no PID or command and owns the precise child.
        self.http("POST","/owner/restart",{"crash":True},origin=launcher["control_url"],private_headers={"Authorization":"Bearer "+launcher["control_token"]})
        a=self.audit(run,"crash-restarted",True)
        unknown={s["reservation_id"] for s in a["budget"]["settlements"] if s["outcome"]=="unknown"}
        require(a["state"]=="BLOCKED" and all(r["reservation_id"] in unknown for r in pending),"restart lost unknown budget or replayed original tool")
        originals={r["reservation_id"] for r in a["budget"]["reservations"] if r["kind"] in ("action","tool")}
        self.command(run,"resume_unknown","resume",expected=400)
        after=self.audit(run,"unknown-not-replayed",True)
        require({r["reservation_id"] for r in after["budget"]["reservations"] if r["kind"] in ("action","tool")}==originals,"unknown resume dispatched new tool")
        self.command(run,"cancel_crash","cancel")
        retry=self.command(run,"redo_crash","retry"); self.report["runs"].append(retry["id"])
        new=self.audit(retry["id"],"new-identity-after-crash",True)
        require(new["state"]=="PLAN" and new["approval"] is None and not new["executions"],"crash redo inherited unknown attempt")
        self.command(retry["id"],"approve_fresh_crash_redo","accept",proposal=new["proposal"])
        self.wait(retry["id"],{"DONE"},reapprove=True)
        completed=self.audit(retry["id"],"fresh-crash-redo-DONE",True)
        require(all(c["status"]=="passed" for c in completed["verification_report"]["checks"]) and completed["plan"]["plan_hash"]==a["plan"]["plan_hash"],"fresh redo did not complete the original full plan")
        original=self.audit(run,"original-remains-cancelled",True)
        require(original["state"]=="CANCELLED" and {r["reservation_id"] for r in original["budget"]["reservations"] if r["kind"] in ("action","tool")}==originals,"redo resurrected original unknown attempt")
        return {"run_id":run,"successor":retry["id"],"lost_attempts":len(pending),"complete_checks":len(completed["verification_report"]["checks"]),"recovery":"original unknown CANCELLED; fresh Run completed after new P1"}


def main():
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--base-url",required=True); parser.add_argument("--fixture-url",required=True)
    parser.add_argument("--mode",choices=("local","temporal"),required=True); parser.add_argument("--report",type=Path,required=True)
    parser.add_argument("--launcher-state",type=Path); parser.add_argument("--suite",choices=("full","quick","authorization","temporal","completion","can","crash"),default="full")
    parser.add_argument("--existing-run",help="Continue an explicit public PLAN/DONE completion Run; preserves prior failed evidence")
    parser.add_argument("--definition-version",choices=("3.2.0","3.3.0"),default="3.2.0")
    args=parser.parse_args(); s=Scenarios(args)
    require(args.existing_run is None or args.suite=="completion","existing Run is only valid for explicit completion continuation")
    if args.suite in ("full","quick","authorization","temporal"):
        s.case("C07/C09 real model failure and stable retry",s.model_failure)
        s.case("C07/C08 forged advice/shell/execution rejected",s.malformed)
        s.case("C10 stale version and wrong hash",s.bindings)
        s.case("C08 valid-schema unauthorized action",s.invalid_action)
    if args.suite=="full":
        s.case("local three-skill completion and actual ZIP runtime",lambda:s.completion(True))
    if args.suite in ("full","authorization"):
        s.case("C10 server expiry and same-source reissue",s.expiry)
    if args.suite=="temporal":
        s.case("real Temporal model action/completion and terminal retry",s.completion)
        s.case("real Temporal CAN and concurrent idempotency",s.temporal_can)
        s.case("real Temporal VERIFY crash and fail-closed recovery",s.crash)
    if args.suite=="completion":s.case("continued public completion and terminal retry",lambda:s.completion(args.mode=="local",args.existing_run))
    if args.suite=="can":s.case("real Temporal CAN and concurrent idempotency",s.temporal_can)
    if args.suite=="crash":s.case("real Temporal VERIFY crash and fail-closed recovery",s.crash)
    state=s.http("GET","/fixture/state",origin=args.fixture_url)
    (s.directory/"provider-state.json").write_text(json.dumps(state,ensure_ascii=False,indent=2)+"\n")
    s.save(); print(json.dumps({"passed":s.report["passed"],"cases":len(s.report["cases"]),"report":str(s.directory/"summary.json")}))
    raise SystemExit(0 if s.report["passed"] else 1)


if __name__=="__main__":main()
