"""Read-only S2 public audit/object/budget collector; never controls Agent execution."""
from __future__ import annotations

import argparse
from datetime import datetime, timezone
import json
from pathlib import Path
import re
import time
import urllib.error
import urllib.parse
import urllib.request

from .oracle import ContractError, DEFAULT_MAX_BYTES, Oracle, byte_hash, canonicalize, strict_loads
from .semantic_checks import FrozenVersions, require, verify_hash
from .s2_fixtures import RELEASE
from .s2_semantics import BudgetHistory, audit_objects, verify_audit
from .s2_raw_artifacts import verify_raw


def canonical_budget_link(header, base, run_path, ledger):
    """Accept only the exact same-origin published object route, never arbitrary URLs."""
    matches = re.findall(r'<([^<>]+)>\s*;\s*rel="canonical"', header or "")
    require(len(matches) == 1, "budget requires one published canonical Link")
    parsed_base = urllib.parse.urlsplit(base)
    parsed = urllib.parse.urlsplit(urllib.parse.urljoin(base + "/", matches[0]))
    require(parsed.scheme == parsed_base.scheme and parsed.netloc == parsed_base.netloc and
            parsed.username is None and parsed.password is None and not parsed.fragment and
            parsed.path == run_path + "/objects", "budget canonical Link is outside the same Run object route")
    query = urllib.parse.parse_qs(parsed.query, keep_blank_values=True, strict_parsing=True)
    require(set(query) == {"object_type", "object_id", "object_version", "source_version", "hash"} and
            all(len(values) == 1 for values in query.values()), "budget canonical Link requires exact reference fields")
    ref = {key: values[0] for key, values in query.items()}
    require(ref["object_type"] == "budget" and ref["object_id"] == ledger["run_id"] and
            ref["object_version"] == str(ledger["ledger_version"]) and ref["hash"] == ledger["ledger_hash"] and
            re.fullmatch(r"[1-9][0-9]*", ref["source_version"]) and int(ref["source_version"]) <= 2**53 - 1,
            "budget canonical Link does not identify the returned immutable version")
    return parsed.path + "?" + parsed.query, ref


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--release", type=Path, default=RELEASE)
    parser.add_argument("--source", type=Path)
    parser.add_argument("--base-url")
    parser.add_argument("--endpoint-kind", choices=("product", "mock"), default="product")
    parser.add_argument("--run-id", action="append", default=[])
    parser.add_argument("--samples", type=int, default=1)
    parser.add_argument("--interval", type=float, default=0.5)
    parser.add_argument("--prior-observation", type=Path, action="append", default=[], help="Previously saved public audit JSON, for restart comparison")
    parser.add_argument("--export", type=Path, help="Explicit public/synthetic export containing audit-bundle.json and content/<sha256>")
    parser.add_argument("--audit-name", default="audit-bundle.json")
    parser.add_argument("--report-dir", type=Path, required=True)
    args = parser.parse_args()
    require(bool(args.base_url) != bool(args.export), "choose public HTTP or an explicitly supplied export")
    require(1 <= args.samples <= 100 and 0 <= args.interval <= 5, "bounded sample count/interval required")
    oracle = Oracle(args.release, args.source)
    directory = args.report_dir.resolve(); directory.mkdir(parents=True, exist_ok=True)
    (directory / "objects").mkdir(exist_ok=True); (directory / "content").mkdir(exist_ok=True)
    report = {"evidence_kind": ("mock_http_contract_transport" if args.endpoint_kind == "mock" else "public_http_read_only") if args.base_url else "supplied_export_no_live_http",
              "at": datetime.now(timezone.utc).isoformat(), "scope": "S2 audit/object/budget observation, not complete v2 or live provider acceptance",
              "status": "failed", "schema_publication": oracle.publication, "observations": [], "http": [], "prior_observations": []}
    budget_history, frozen = BudgetHistory(), FrozenVersions()
    stage_observations, immutable, budget_refs = {}, {}, {}

    def observe(bundle, content_loader, label):
        oracle.validate("S2AuditBundle", bundle); verify_hash("S2AuditBundle", bundle)
        stages = stage_observations.setdefault(bundle["run_id"], {})
        sequence = bundle["state_sequence"]
        require(sequence not in stages or stages[sequence] == bundle["state"], "same state sequence changed authoritative state")
        stages[sequence] = bundle["state"]
        result = verify_audit(bundle, oracle, content_loader, observed_stages=stages)
        frozen.observe(bundle["definition_snapshot"])
        budget_history.observe(bundle["budget"])
        for _name, ref, value in audit_objects(bundle):
            # Include Run in identity; Approval excludes lifecycle in its digest but
            # archived bytes must still be unchanged on repeated exact reads.
            identity = (bundle["run_id"], ref["object_type"], ref["object_id"], ref["object_version"])
            encoded = canonicalize(value)
            require(identity not in immutable or immutable[identity] == encoded, "immutable public object changed across observations")
            immutable[identity] = encoded
        result.update(run_id=bundle["run_id"], source_version=bundle["source_version"], state_sequence=sequence,
                      state=bundle["state"], audit_hash=bundle["audit_hash"], input=label)
        return result

    try:
        if args.export:
            require(Path(args.audit_name).name == args.audit_name, "audit filename must be a basename")
            raw = (args.export / args.audit_name).read_bytes(); bundle = strict_loads(raw)
            def export_content(ref):
                data = (args.export / "content" / ref["hash"]).read_bytes()
                require(byte_hash(data) == ref["hash"], "export content digest mismatch")
                return data
            report["observations"].append(observe(bundle, export_content, str(args.export / args.audit_name)))
            (directory / "audit-000.json").write_bytes(raw)
            for dep in bundle["definition_snapshot"]["schema_dependencies"]:
                require((args.export / "content" / dep["sha256"]).read_bytes() == oracle.raw[dep["schema_uri"]], "exported schema differs from selected release")
        else:
            parsed = urllib.parse.urlsplit(args.base_url)
            require(parsed.scheme in ("http", "https") and parsed.netloc and parsed.username is None and parsed.password is None and
                    parsed.path in ("", "/") and not parsed.query and not parsed.fragment, "base URL must be a credential-free HTTP origin")
            require(bool(args.run_id), "at least one Run ID required")
            base = args.base_url.rstrip("/")
            class NoRedirect(urllib.request.HTTPRedirectHandler):
                def redirect_request(self, *_args, **_kwargs): return None
            opener = urllib.request.build_opener(NoRedirect())
            def get(path, expected=200):
                try:
                    response = opener.open(urllib.request.Request(base + path, headers={"Accept": "application/json, text/plain"}), timeout=15)
                except urllib.error.HTTPError as error: response = error
                with response:
                    raw = response.read(DEFAULT_MAX_BYTES + 1)
                    require(len(raw) <= DEFAULT_MAX_BYTES, "response byte limit exceeded")
                    report["http"].append({"method": "GET", "path": path, "status": response.status, "bytes": len(raw), "sha256": byte_hash(raw)})
                    require(response.status == expected, "unexpected HTTP status " + str(response.status) + " for " + path.split("?")[0])
                    return raw, {key.lower(): value for key, value in response.headers.items()}
            def loader(run_path):
                def load(ref):
                    target = directory / "content" / ref["hash"]
                    if target.exists(): return target.read_bytes()
                    query = urllib.parse.urlencode({key: ref[key] for key in ("object_type", "object_id", "hash")})
                    raw, headers = get(run_path + "/definition/content?" + query)
                    require(byte_hash(raw) == ref["hash"] and headers.get("x-content-sha256") == ref["hash"], "frozen content/header digest mismatch")
                    target.write_bytes(raw); return raw
                return load
            for prior in args.prior_observation:
                raw = prior.read_bytes(); bundle = strict_loads(raw)
                require(bundle["run_id"] in args.run_id, "prior observation belongs to an unselected Run")
                run_path = "/api/runs/" + urllib.parse.quote(bundle["run_id"], safe="")
                report["prior_observations"].append(observe(bundle, loader(run_path), str(prior)))
            run_examples = {}
            for sample in range(args.samples):
                if sample: time.sleep(args.interval)
                for run_number, run_id in enumerate(args.run_id):
                    run_path = "/api/runs/" + urllib.parse.quote(run_id, safe="")
                    raw, _headers = get(run_path + "/decision-audit")
                    bundle = strict_loads(raw)
                    require(bundle["run_id"] == run_id, "public audit crossed Run")
                    filename = f"run-{run_number}-audit-{sample:03d}.json"; (directory / filename).write_bytes(raw)
                    result = observe(bundle, loader(run_path), filename)
                    report["observations"].append(result)
                    for dependency in bundle["definition_snapshot"]["schema_dependencies"]:
                        data = loader(run_path)({"object_type": "schema", "object_id": dependency["schema_uri"], "hash": dependency["sha256"]})
                        require(data == oracle.raw[dependency["schema_uri"]], "public frozen schema differs from release")
                    for name, ref, item in audit_objects(bundle):
                        path = run_path + "/objects?" + urllib.parse.urlencode(ref)
                        object_raw, _ = get(path)
                        value = oracle.decode(name, object_raw, "message.schema.json" if name == "MessageEnvelope" else "agent.schema.json")
                        require(value == item, "exact object read differs from immutable audit archive")
                        (directory / "objects" / (byte_hash(canonicalize({"run_id": run_id, "ref": ref})) + ".json")).write_bytes(object_raw)
                        run_examples.setdefault(run_id, ref)
                    budget_raw, headers = get(run_path + "/budget")
                    budget = oracle.decode("BudgetLedger", budget_raw); verify_hash("BudgetLedger", budget)
                    canonical_path, budget_ref = canonical_budget_link(headers.get("link"), base, run_path, budget)
                    exact_raw, _ = get(canonical_path)
                    require(oracle.decode("BudgetLedger", exact_raw) == budget, "canonical budget read differs from returned ledger")
                    ref_id = (run_id, budget["ledger_version"])
                    require(ref_id not in budget_refs or budget_refs[ref_id] == budget_ref, "budget registration source changed for same version")
                    budget_refs[ref_id] = budget_ref
                    if budget["ledger_version"] == bundle["budget"]["ledger_version"]:
                        require(budget == bundle["budget"], "same budget version differs across public endpoints")
                    else:
                        require(budget["ledger_version"] > bundle["budget"]["ledger_version"], "budget endpoint moved backwards")
                        result["budget_read_advanced"] = {"version": budget["ledger_version"], "scope": "schema/hash/exact reference only; full semantic check needs matching audit"}
                    (directory / f"run-{run_number}-budget-{sample:03d}.json").write_bytes(budget_raw)
                    result["budget_ref"] = budget_ref
                    result["raw_artifacts_verified"] = []
                    for item in result.get("release13", {}).get("raw_artifacts", []):
                        artifact_raw, artifact_headers = get(run_path + "/objects?" + urllib.parse.urlencode(item["ref"]))
                        require(artifact_headers.get("x-content-sha256") == item["sha256"], "raw artifact response digest header mismatch")
                        result["raw_artifacts_verified"].append(verify_raw(artifact_raw, item))
                        (directory / "objects" / item["sha256"]).write_bytes(artifact_raw)
                    if sample == 0:
                        error_raw, _ = get(run_path + "/objects", 400); oracle.decode("ErrorResponse", error_raw)
                        if run_id in run_examples:
                            bad = dict(run_examples[run_id]); bad["hash"] = "a" * 64 if bad["hash"] != "a" * 64 else "b" * 64
                            error_raw, _ = get(run_path + "/objects?" + urllib.parse.urlencode(bad), 404); oracle.decode("ErrorResponse", error_raw)
            if len(args.run_id) > 1:
                for index, run_id in enumerate(args.run_id):
                    source_run = args.run_id[(index + 1) % len(args.run_id)]
                    if source_run not in run_examples: continue
                    path = "/api/runs/" + urllib.parse.quote(run_id, safe="") + "/objects?" + urllib.parse.urlencode(run_examples[source_run])
                    raw, _ = get(path, 404); oracle.decode("ErrorResponse", raw)
        report["status"] = "passed"
    except (ContractError, OSError, ValueError, KeyError) as error:
        report["error"] = str(error)
    finally:
        report["completed_at"] = datetime.now(timezone.utc).isoformat()
        (directory / "summary.json").write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n")
    print(json.dumps({key: report.get(key) for key in ("status", "evidence_kind", "scope", "observations", "error")}, ensure_ascii=False))
    return 0 if report["status"] == "passed" else 1


if __name__ == "__main__": raise SystemExit(main())
