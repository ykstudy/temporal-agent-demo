"""Read-only S1 public definition/byte verification and strict standalone object CLI."""
from __future__ import annotations

import argparse
from datetime import datetime, timezone
import json
from pathlib import Path
import urllib.error
import urllib.parse
import urllib.request

from .oracle import ContractError, HERE, Oracle, byte_hash, strict_loads, DEFAULT_MAX_BYTES
from .semantic_checks import FrozenVersions, verify_definition_snapshot, verify_hash, require


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--release", type=Path, default=HERE / "schemas")
    parser.add_argument("--source", type=Path, help="Also require derived bytes equal this public source directory")
    parser.add_argument("--base-url", help="Public Agent HTTP origin; credentials in URL are forbidden")
    parser.add_argument("--run-id", action="append", default=[], help="Repeat to verify frozen versions across Runs")
    parser.add_argument("--legacy-run-id", action="append", default=[], help="Existing source-incomplete Run; expect read-only S1 perimeter 409")
    parser.add_argument("--export", type=Path, help="Public exported definition-snapshot.json and content/<sha256>")
    parser.add_argument("--snapshot", type=Path, help="Strict standalone JSON object file")
    parser.add_argument("--definition", default="DefinitionSnapshot", help="Public $defs name for --snapshot")
    parser.add_argument("--schema-file", default="agent.schema.json")
    parser.add_argument("--expected-decision-mode", choices=("model", "rules"))
    parser.add_argument("--report-dir", type=Path, required=True)
    args = parser.parse_args()
    require(sum(bool(item) for item in (args.base_url, args.export, args.snapshot)) == 1,
            "select exactly one source: --base-url, --export, --snapshot")
    oracle = Oracle(args.release, args.source)
    report_dir = args.report_dir.resolve()
    report_dir.mkdir(parents=True, exist_ok=True)
    exchanges, results = [], []
    report = {"scope": "S1 public definition structure/hash/dependency validation; not complete v2 Run, execution, or live provider evidence",
              "recorded_at": datetime.now(timezone.utc).isoformat(), "schema_publication": oracle.publication,
              "status": "failed", "results": results, "http": exchanges}
    try:
        if args.snapshot:
            value = oracle.decode(args.definition, args.snapshot.read_bytes(), args.schema_file)
            results.append({"object": args.definition, "status": "passed", "proof": "strict structure only",
                            "input_sha256": byte_hash(args.snapshot.read_bytes())})
        elif args.export:
            snapshot = strict_loads((args.export / "definition-snapshot.json").read_bytes())
            def content_loader(ref):
                raw = (args.export / "content" / ref["hash"]).read_bytes()
                require(byte_hash(raw) == ref["hash"], "exported content hash mismatch")
                return raw
            results.append(verify_definition_snapshot(snapshot, oracle, content_loader, args.expected_decision_mode))
            # Public exported Schema bytes are part of a real complete closure, too.
            for dependency in snapshot["schema_dependencies"]:
                raw = (args.export / "content" / dependency["sha256"]).read_bytes()
                require(raw == oracle.raw[dependency["schema_uri"]], "exported schema dependency differs from release")
        else:
            parsed = urllib.parse.urlsplit(args.base_url)
            require(parsed.scheme in ("http", "https") and parsed.netloc and
                    parsed.username is None and parsed.password is None and parsed.path in ("", "/") and
                    not parsed.query and not parsed.fragment,
                    "base URL must be a credential-free HTTP origin")
            require(bool(args.run_id or args.legacy_run_id), "at least one public Run identity required")
            base = args.base_url.rstrip("/")
            class NoRedirect(urllib.request.HTTPRedirectHandler):
                def redirect_request(self, *_args, **_kwargs):
                    return None
            opener = urllib.request.build_opener(NoRedirect())
            def get(path, expected=200, content_hash=None):
                try:
                    response = opener.open(urllib.request.Request(base + path, headers={"Accept": "application/json, text/plain"}), timeout=15)
                except urllib.error.HTTPError as error:
                    response = error
                with response:
                    raw = response.read(DEFAULT_MAX_BYTES + 1)
                    require(len(raw) <= DEFAULT_MAX_BYTES, "public response too large")
                    exchanges.append({"method": "GET", "path": path, "status": response.status,
                                      "bytes": len(raw), "sha256": byte_hash(raw)})
                    require(response.status == expected, "unexpected public response status: " + str(response.status))
                    if content_hash is not None:
                        require(response.headers.get("X-Content-SHA256") == content_hash, "content digest header mismatch")
                        require(byte_hash(raw) == content_hash, "public byte content digest mismatch")
                    return raw
            definitions_raw = get("/api/definitions")
            definitions = strict_loads(definitions_raw)
            require(type(definitions) is list and definitions, "definitions must be a nonempty array")
            identities = set()
            for definition in definitions:
                oracle.validate("AgentDefinition", definition)
                verify_hash("AgentDefinition", definition)
                identity = (definition["definition_id"], definition["definition_version"])
                require(identity not in identities, "duplicate published definition identity")
                identities.add(identity)
            (report_dir / "definitions.json").write_bytes(definitions_raw)
            frozen = FrozenVersions()
            for index, run_id in enumerate(args.run_id):
                path = "/api/runs/" + urllib.parse.quote(run_id, safe="") + "/definition"
                raw = get(path)
                snapshot = strict_loads(raw)
                oracle.validate("DefinitionSnapshot", snapshot)
                (report_dir / f"run-{index}-definition.json").write_bytes(raw)
                content_dir = report_dir / f"run-{index}-content"
                content_dir.mkdir(exist_ok=True)
                def load_content(ref):
                    query = urllib.parse.urlencode({"object_type": ref["object_type"], "object_id": ref["object_id"], "hash": ref["hash"]})
                    content = get(path + "/content?" + query, content_hash=ref["hash"])
                    (content_dir / ref["hash"]).write_bytes(content)
                    return content
                result = verify_definition_snapshot(snapshot, oracle, load_content, args.expected_decision_mode)
                for dependency in snapshot["schema_dependencies"]:
                    body = load_content({"object_type": "schema", "object_id": dependency["schema_uri"], "hash": dependency["sha256"]})
                    require(body == oracle.raw[dependency["schema_uri"]], "published schema bytes differ from selected contract release")
                frozen.observe(snapshot)
                # Identity is exact: valid digest alone does not authorize arbitrary content read.
                first = snapshot["template_dependencies"][0]["files"][0]["content_ref"]
                for query, expected in [
                    ({}, 400),
                    ({"object_type": "template_content", "object_id": "template_content_" + "a" * 64, "hash": "a" * 64}, 404),
                    ({"object_type": "template_content", "object_id": first["object_id"], "hash": "b" * 64}, 404),
                ]:
                    error = strict_loads(get(path + "/content?" + urllib.parse.urlencode(query), expected))
                    oracle.validate("ErrorResponse", error)
                again = strict_loads(get(path))
                require(again == snapshot, "read-only observation changed the bound snapshot")
                result.update(run_id=run_id, status="passed", scope="public HTTP frozen closure and invalid content reads")
                results.append(result)
            for run_id in args.legacy_run_id:
                path = "/api/runs/" + urllib.parse.quote(run_id, safe="") + "/definition"
                error = strict_loads(get(path, 409))
                oracle.validate("ErrorResponse", error)
                require(error["code"] == "legacy_contract_incomplete" and
                        error["legacy_read_url"] == "/api/runs/" + run_id,
                        "legacy source-incomplete read boundary is not explicit")
                results.append({"run_id": run_id, "status": "passed", "scope": "legacy S1 definition perimeter 409 only; not v2 Run API migration"})
        report["status"] = "passed"
    except (ContractError, OSError) as error:
        report["error"] = str(error)
    finally:
        report["completed_at"] = datetime.now(timezone.utc).isoformat()
        (report_dir / "summary.json").write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n")
    print(json.dumps({"status": report["status"], "scope": report["scope"], "results": results,
                      "error": report.get("error"), "report": str(report_dir / "summary.json")}, ensure_ascii=False))
    return 0 if report["status"] == "passed" else 1


if __name__ == "__main__":
    raise SystemExit(main())
