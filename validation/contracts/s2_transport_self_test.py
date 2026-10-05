"""Exercise the collector over owned synthetic HTTP, explicitly not product evidence."""
import argparse
from copy import deepcopy
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import json
from pathlib import Path
import subprocess
import sys
import threading
from urllib.parse import parse_qs, urlencode, urlsplit

from .oracle import byte_hash, canonicalize
from .semantic_checks import ref_key
from .s2_fixtures import build, freeze, timestamp
from .s2_semantics import audit_objects, ref_for


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--report-dir", type=Path, required=True)
    args = parser.parse_args()
    directory = args.report_dir.resolve(); directory.mkdir(parents=True, exist_ok=True)
    bundle, content, _stages = build()
    # Current source changes on permission revocation; the unchanged ledger's
    # canonical reference must keep its original registered source, which is 1.
    bundle.update(source_version=2, state_sequence=51, state="CLARIFY", proposal=None, created_at=timestamp(110))
    permission = deepcopy(bundle["permission_snapshot"])
    permission.update(permission_version=2, revoked=True, updated_at=timestamp(101)); freeze("PermissionSnapshot", permission)
    bundle["permission_snapshot"] = permission; bundle["permission_history"].append(permission)
    bundle["approval"].update(status="revoked", revoked_at=timestamp(101)); freeze("S2AuditBundle", bundle)
    objects = {ref_key(ref): canonicalize(value) for _name, ref, value in audit_objects(bundle)}
    budget_ref = ref_for("BudgetLedger", bundle["budget"], source_version=1)
    objects[ref_key(budget_ref)] = canonicalize(bundle["budget"])
    run_path = "/api/runs/" + bundle["run_id"]
    allowed_content = {}
    for template in bundle["definition_snapshot"]["template_dependencies"]:
        for file in template["files"]:
            ref = file["content_ref"]; allowed_content[ref["object_type"], ref["object_id"], ref["hash"]] = content[ref["hash"]]
    for dep in bundle["definition_snapshot"]["schema_dependencies"]:
        allowed_content["schema", dep["schema_uri"], dep["sha256"]] = content[dep["sha256"]]
    requests = []
    class Handler(BaseHTTPRequestHandler):
        def log_message(self, *_args): pass
        def send_body(self, status, data, headers=None):
            self.send_response(status)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(data)))
            for key, value in (headers or {}).items(): self.send_header(key, value)
            self.end_headers(); self.wfile.write(data)
        def do_GET(self):
            parsed = urlsplit(self.path); values = parse_qs(parsed.query)
            requests.append({"method": "GET", "path": self.path})
            if parsed.path == run_path + "/decision-audit": return self.send_body(200, canonicalize(bundle))
            if parsed.path == run_path + "/budget":
                link = "<" + run_path + "/objects?" + urlencode(budget_ref) + '>; rel="canonical"'
                return self.send_body(200, canonicalize(bundle["budget"]), {"Link": link})
            if parsed.path == run_path + "/objects":
                if set(values) != set(budget_ref) or any(len(item) != 1 for item in values.values()):
                    return self.send_body(400, canonicalize({"code": "invalid_reference", "error": "Exact reference fields required"}))
                ref = {key: item[0] for key, item in values.items()}
                try: ref["object_version"] = int(ref["object_version"]); ref["source_version"] = int(ref["source_version"])
                except ValueError: return self.send_body(400, canonicalize({"code": "invalid_reference", "error": "Invalid reference version"}))
                if ref_key(ref) in objects: return self.send_body(200, objects[ref_key(ref)])
            if parsed.path == run_path + "/definition/content":
                key = tuple(values.get(item, [None])[0] for item in ("object_type", "object_id", "hash"))
                if key in allowed_content:
                    data = allowed_content[key]
                    return self.send_body(200, data, {"X-Content-Sha256": byte_hash(data)})
            return self.send_body(404, canonicalize({"code": "not_found", "error": "Public object not found"}))
    server = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
    thread = threading.Thread(target=server.serve_forever, daemon=True); thread.start()
    try:
        result = subprocess.run([sys.executable, "-m", "validation.contracts.s2_collect", "--endpoint-kind", "mock", "--base-url",
            "http://127.0.0.1:" + str(server.server_address[1]), "--run-id", bundle["run_id"], "--samples", "2", "--interval", "0",
            "--report-dir", str(directory / "collector")], capture_output=True, text=True, timeout=60)
        (directory / "collector-output.txt").write_text(result.stdout + result.stderr)
        if result.returncode: raise RuntimeError("synthetic collector transport failed; see collector-output.txt")
        report = json.loads((directory / "collector/summary.json").read_text())
        assert report["evidence_kind"] == "mock_http_contract_transport" and report["status"] == "passed"
        assert len(report["observations"]) == 2
        for observation in report["observations"]:
            assert observation["source_version"] == 2 and observation["budget_ref"]["source_version"] == "1"
            assert observation["observed_execution_stages"] == 0
        summary = {"status": "passed", "evidence_kind": "synthetic_http_collector_self_test", "requests": len(requests),
            "all_requests_read_only": all(item["method"] == "GET" for item in requests), "current_run_source": 2,
            "budget_registered_source": 1, "scope": "Owned synthetic HTTP protocol only; no Agent job/provider/tool execution"}
        (directory / "requests.json").write_text(json.dumps(requests, indent=2) + "\n")
        (directory / "summary.json").write_text(json.dumps(summary, ensure_ascii=False, indent=2) + "\n")
        print(json.dumps(summary, ensure_ascii=False))
    finally:
        server.shutdown(); server.server_close(); thread.join(timeout=5)


if __name__ == "__main__": main()
