"""Check public 1.3 audit exports, without claiming HTTP or full semantic acceptance."""
from __future__ import annotations

import argparse
from copy import deepcopy
from datetime import datetime, timezone
import json
from pathlib import Path

from .decision_self_test import structural_cases
from .oracle import ContractError, Oracle, byte_hash, strict_loads
from .s2_semantics import audit_objects
from .semantic_checks import HASH_RULES, require, verify_hash

HERE = Path(__file__).resolve().parent
ROOT = HERE.parents[1]
NEW_ARRAYS = {
    "verification_candidates": "VerificationCandidate",
    "package_receipts": "PackageReceipt",
    "remote_receipts": "RemoteExecutionReceipt",
    "verification_reports": "VerificationReport",
}


def objects(bundle):
    yield "S2AuditBundle", bundle
    snapshot = bundle["definition_snapshot"]
    yield "DefinitionSnapshot", snapshot
    yield "AgentDefinition", snapshot["definition"]
    for skill in snapshot["skills"]:
        yield "SkillDefinition", skill
    yield "BudgetLedger", bundle["budget"]
    for name, _ref, value in audit_objects(bundle):
        yield name, value
        if name == "Proposal":
            yield "ProjectPlan", value["plan"]
    for value in bundle["permission_history"]:
        yield "PermissionSnapshot", value
    for field, name in NEW_ARRAYS.items():
        for value in bundle[field]:
            yield name, value


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--audit-export", type=Path, action="append", required=True)
    parser.add_argument("--report", type=Path, required=True)
    args = parser.parse_args()
    oracle = Oracle(HERE / "releases/1.3.0", ROOT / "design/contracts/releases/1.3.0")
    report = {
        "at": datetime.now(timezone.utc).isoformat(), "contract_version": "1.3.0",
        "evidence_kind": "independent_public_export_structure_and_hash_slice",
        "schema_refs": oracle.static_refs, "publication": oracle.publication,
        "exports": [], "cases": [],
        "not_covered": ["public HTTP collection and creation", "complete cross-object semantic oracle",
                        "remote branches absent from supplied exports", "Temporal, browser and live third-party acceptance"],
    }
    samples = {}
    for path in args.audit_export:
        raw = path.read_bytes()
        record = strict_loads(raw)
        bundle = record["audit"]
        require(bundle["contract_version"] == "1.3.0", "export must be original 1.3 publication")
        oracle.validate("S2AuditBundle", bundle)
        count = 0
        for name, value in objects(bundle):
            oracle.validate(name, value, "message.schema.json" if name == "MessageEnvelope" else "agent.schema.json")
            if HASH_RULES[name][0] is not None:
                verify_hash(name, value)
            count += 1
            if name in NEW_ARRAYS.values():
                variant = ":".join(part for part in (
                    value.get("action_type", ""),
                    value.get("status", value.get("outcome", "")),
                ) if part)
                samples.setdefault((name, variant), value)
        definition = bundle["definition_snapshot"]["definition"]
        samples.setdefault(("GoModulePolicy", ""), definition["go_module_policy"])
        samples.setdefault(("GitExecutionPolicy", ""), definition["repository_policy"]["git_execution"])
        report["exports"].append({"path": str(path), "sha256": byte_hash(raw), "run_id": bundle["run_id"],
                                  "source_version": bundle["source_version"], "state": bundle["state"],
                                  "validated_objects": count,
                                  "initial_proposal": record.get("initial_proposal", "see original test provenance"),
                                  "present_new_objects": {field: len(bundle[field]) for field in NEW_ARRAYS}})
    for (name, variant), value in samples.items():
        for keyword, path, bad in structural_cases(oracle, value, name):
            try:
                oracle.validate(name, bad)
            except ContractError:
                report["cases"].append({"name": name, "variant": variant, "keyword": keyword, "path": list(path), "status": "passed"})
            else:
                raise AssertionError(f"structural negative accepted: {name} {keyword} {path}")
        if name in HASH_RULES:
            bad = deepcopy(value)
            field = HASH_RULES[name][0]
            bad[field] = "f" * 64 if bad[field] != "f" * 64 else "e" * 64
            oracle.validate(name, bad)
            try:
                verify_hash(name, bad)
            except ContractError:
                report["cases"].append({"name": name, "variant": variant, "keyword": "content_hash", "path": [field], "status": "passed"})
            else:
                raise AssertionError(f"wrong content hash accepted: {name}")
    report["structural_negative_cases"] = sum(case["keyword"] != "content_hash" for case in report["cases"])
    report["hash_negative_cases"] = sum(case["keyword"] == "content_hash" for case in report["cases"])
    report["negative_cases"] = len(report["cases"])
    report["tested_new_shapes"] = sorted({name for name, _ in samples})
    report["tested_variants"] = sorted(name + (":" + variant if variant else "") for name, variant in samples)
    report["passed"] = True
    args.report.parent.mkdir(parents=True, exist_ok=True)
    args.report.write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n")
    print(json.dumps({key: report[key] for key in ("passed", "schema_refs", "structural_negative_cases", "hash_negative_cases", "negative_cases", "tested_new_shapes")}))


if __name__ == "__main__":
    main()
