"""Build explicit synthetic fixtures from public S0 examples, never product outputs.

All placeholder hashes are replaced from independent synthetic template bytes and
the selected published schemas. These fixtures prove the oracle, not an Agent Run.
"""
from copy import deepcopy
import json
from pathlib import Path

from .oracle import HERE, Oracle, byte_hash, canonicalize, strict_loads
from .semantic_checks import object_hash, ref_key

ROOT = HERE.parents[1]


def rewrite_versions(value):
    if isinstance(value, dict):
        for key, child in value.items():
            if key == "contract_version":
                value[key] = "1.1.0"
            else:
                rewrite_versions(child)
    elif isinstance(value, list):
        for child in value:
            rewrite_versions(child)


def main():
    oracle = Oracle()
    seed = ROOT / "design/contracts/examples/run-snapshot-intake.valid.json"
    snapshot = strict_loads(seed.read_bytes())["definition_snapshot"]
    rewrite_versions(snapshot)
    fixture_dir = HERE / "fixtures"
    content_dir = fixture_dir / "content"
    content_dir.mkdir(parents=True, exist_ok=True)
    entries = []
    for template in snapshot["template_dependencies"]:
        for file in template["files"]:
            raw = ("// Independent oracle fixture, never a generated project.\n"
                   + "// " + template["template_id"] + "/" + file["path"] + "\n").encode()
            digest = byte_hash(raw)
            ref = {"object_type": "template_content", "object_id": "template_content_" + digest,
                   "object_version": 1, "source_version": 1, "hash": digest}
            file["sha256"], file["content_ref"] = digest, ref
            (content_dir / digest).write_bytes(raw)
            entries.append({"ref": ref, "path": "content/" + digest, "size": len(raw)})
        template["files"].sort(key=lambda item: item["path"])
        template["manifest_hash"] = object_hash("TemplateDependency", template)
    snapshot["template_dependencies"].sort(key=lambda item: (item["template_id"], item["template_version"]))
    templates = {(item["template_id"], item["template_version"]): item for item in snapshot["template_dependencies"]}
    for dependency in snapshot["schema_dependencies"]:
        dependency["sha256"] = byte_hash(oracle.raw[dependency["schema_uri"]])
    snapshot["schema_dependencies"].sort(key=lambda item: item["schema_uri"])
    definition = snapshot["definition"]
    definition["budget_policy"].update(max_tool_seconds=720, max_execution_seconds=7200,
                                        max_ci_wait_seconds=1800, max_ci_poll_attempts=128,
                                        max_reconciliation_seconds=1800, max_reconciliation_attempts=64)
    definition["system_prompt"]["content"] = "Independent oracle fixture. Suggest only; never grant authorization."
    definition["system_prompt"]["sha256"] = byte_hash(definition["system_prompt"]["content"].encode())
    def bind_refs(value):
        if isinstance(value, dict):
            if "schema_ref" in value:
                value["file_sha256"] = byte_hash(oracle.raw[value["schema_ref"].split("#")[0]])
            for child in value.values():
                bind_refs(child)
        elif isinstance(value, list):
            for child in value:
                bind_refs(child)
    bind_refs(snapshot)
    definition["completion_policy"]["required_checks"].sort(key=lambda item: item["check_id"])
    snapshot["skills"].sort(key=lambda item: item["skill_id"])
    for skill in snapshot["skills"]:
        skill["template_dependencies"] = [deepcopy(templates[(item["template_id"], item["template_version"])])
                                            for item in skill["template_dependencies"]]
        skill["required_permissions"].sort()
        skill["required_check_ids"].sort()
        skill["skill_hash"] = object_hash("SkillDefinition", skill)
    definition["available_skills"] = [{key: item[key] for key in ("skill_id", "skill_version", "skill_hash")}
                                       for item in snapshot["skills"]]
    definition["definition_hash"] = object_hash("AgentDefinition", definition)
    snapshot["definition_hash"] = definition["definition_hash"]
    snapshot["snapshot_hash"] = object_hash("DefinitionSnapshot", snapshot)
    (fixture_dir / "definition-snapshot.valid.json").write_text(json.dumps(snapshot, ensure_ascii=False, indent=2) + "\n")
    provenance = {"kind": "synthetic_oracle_fixture", "not_product_evidence": True,
                  "shape_source": str(seed.relative_to(ROOT)), "shape_source_sha256": byte_hash(seed.read_bytes()),
                  "contract_version": "1.1.0", "files": sorted(entries, key=lambda item: ref_key(item["ref"]))}
    (fixture_dir / "content-manifest.json").write_text(json.dumps(provenance, ensure_ascii=False, indent=2) + "\n")
    build_vectors(fixture_dir)
    print("built independent synthetic closure and published JCS vectors")


def build_vectors(directory):
    # Expected canonical strings are explicit reference results, not copied from the Agent.
    vectors = [
        ("property-order", '{"z":1, "a": [true, null, "x"]}', '{"a":[true,null,"x"],"z":1}', "RFC 8785 section 3.2.3"),
        ("negative-zero-and-exponents", '[-0.0,1e-7,1e-6,1e20,1e21,4.50,2e-3]', '[0,1e-7,0.000001,100000000000000000000,1e+21,4.5,0.002]', "RFC 8785 section 3.2.2.3"),
        ("rfc-rounding", '[333333333.33333329,1E30,4.50,2e-3,0.000000000000000000000000001]', '[333333333.3333333,1e+30,4.5,0.002,1e-27]', "RFC 8785 section 3.2.2"),
        ("utf16-property-order", '{"דּ":7,"😀":6,"€":5,"ö":4,"\\u0080":3,"1":2,"\\r":1}', '{"\\r":1,"1":2,"\u0080":3,"ö":4,"€":5,"😀":6,"דּ":7}', "RFC 8785 section 3.2.3"),
        ("unicode-not-normalized", '{"é":"é","é":"é"}', '{"é":"é","é":"é"}', "RFC 8785 section 3.1"),
        ("escaping", '{"x":"<>&/\\u000f\\n\\t\\\\\\\""}', '{"x":"<>&/\\u000f\\n\\t\\\\\\\""}', "RFC 8785 section 3.2.2.2"),
        ("safe-integer-maximum", '[9007199254740991,-9007199254740991]', '[9007199254740991,-9007199254740991]', "HASHING.md interoperable integer limit"),
    ]
    entries = []
    for name, raw, expected, source in vectors:
        assert canonicalize(strict_loads(raw)).decode() == expected, name
        entries.append({"id": name, "input_json": raw, "expected_canonical": expected,
                        "expected_sha256": byte_hash(expected.encode()), "source": source})
    data = {"kind": "cross_language_jcs_vectors", "reference": "https://www.rfc-editor.org/rfc/rfc8785",
            "positive": entries,
            "strict_json_negative": [
                {"id": "duplicate-key", "input_json": '{"x":1,"x":2}'},
                {"id": "escaped-duplicate-key", "input_json": '{"x":1,"\\u0078":2}'},
                {"id": "nan", "input_json": '[NaN]'},
                {"id": "infinity", "input_json": '[Infinity]'},
                {"id": "overflow-exponent", "input_json": '[1e999]'},
                {"id": "unsafe-integer", "input_json": '[9007199254740992]'},
                {"id": "lone-surrogate-value", "input_json": '{"x":"\\udead"}'},
                {"id": "lone-surrogate-key", "input_json": '{"\\ud800":1}'},
            ]}
    (directory / "jcs-vectors.json").write_text(json.dumps(data, ensure_ascii=False, indent=2) + "\n")


if __name__ == "__main__":
    main()
