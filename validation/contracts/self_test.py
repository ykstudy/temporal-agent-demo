"""Independent validator self-tests. Synthetic data is not product runtime evidence."""
from __future__ import annotations

import argparse
from copy import deepcopy
from datetime import datetime, timezone
from importlib.metadata import version
import json
from pathlib import Path
import tempfile
from urllib.parse import urldefrag, urljoin

from jsonschema import Draft202012Validator

from .oracle import ContractError, HERE, Oracle, byte_hash, canonicalize, strict_loads
from .semantic_checks import FrozenVersions, object_hash, verify_definition_snapshot

ROOT = HERE.parents[1]


def replace(value, path, replacement=None, delete=False):
    result = deepcopy(value)
    if not path:
        return replacement
    parent = result
    for segment in path[:-1]:
        parent = parent[segment]
    if delete:
        del parent[path[-1]]
    else:
        parent[path[-1]] = replacement
    return result


def mutation_cases(oracle, instance, definition, schema_file="agent.schema.json", descend_refs=True):
    """Derive structural negatives from published constraints, never the observed API."""
    root = oracle.schemas[schema_file]
    seen = set()
    def matches(schema, value, base):
        def absolute(node):
            if isinstance(node, dict):
                return {key: urljoin(base, item) if key == "$ref" else absolute(item)
                        for key, item in node.items()}
            if isinstance(node, list):
                return [absolute(item) for item in node]
            return node
        return Draft202012Validator(absolute(schema), registry=oracle.registry,
                                     format_checker=oracle.formats).is_valid(value)
    def visit(schema, value, base, path):
        if isinstance(schema, bool):
            return
        if "$ref" in schema:
            if path and not descend_refs:
                return  # This named child is swept separately against its exact release.
            full = urljoin(base, schema["$ref"])
            base, _ = urldefrag(full)
            yield from visit(oracle.registry.resolver(base).lookup(full).contents, value, base, path)
            return
        for option in schema.get("allOf", []):
            yield from visit(option, value, base, path)
        for keyword in ("anyOf", "oneOf"):
            for option in schema.get(keyword, []):
                if matches(option, value, base):
                    yield from visit(option, value, base, path)
        if "if" in schema:
            branch = "then" if matches(schema["if"], value, base) else "else"
            if branch in schema:
                yield from visit(schema[branch], value, base, path)
        changes = []
        for closed in ("additionalProperties", "unevaluatedProperties"):
            if schema.get(closed) is False and isinstance(value, dict):
                changes.append((closed, path + ("__undeclared__",), True, False))
        for key in schema.get("required", []):
            if isinstance(value, dict) and key in value:
                changes.append(("required", path + (key,), None, True))
        if "type" in schema and isinstance(schema["type"], str):
            bad = {"integer": True, "number": True, "string": False, "boolean": 0,
                   "object": [], "array": {}, "null": "not-null"}[schema["type"]]
            changes.append(("type", path, bad, False))
        if "enum" in schema:
            changes.append(("enum", path, "__invalid_enum__", False))
        if "const" in schema:
            bad = None if schema["const"] is not None else "__not_null__"
            changes.append(("const", path, bad, False))
        if "pattern" in schema:
            changes.append(("pattern", path, "! invalid value /", False))
        if "minimum" in schema:
            changes.append(("minimum", path, schema["minimum"] - 1, False))
        if "maximum" in schema:
            changes.append(("maximum", path, schema["maximum"] + 1, False))
        if schema.get("format") == "date-time":
            changes.append(("format", path, "2026-02-30T00:00:00Z", False))
        if schema.get("minLength", 0) > 0:
            changes.append(("minLength", path, "", False))
        if "maxLength" in schema:
            changes.append(("maxLength", path, "x" * (schema["maxLength"] + 1), False))
        if schema.get("minItems", 0) > 0:
            changes.append(("minItems", path, [], False))
        if "maxItems" in schema and isinstance(value, list) and value:
            changes.append(("maxItems", path, [deepcopy(value[0])] * (schema["maxItems"] + 1), False))
        for keyword, target, bad, delete in changes:
            identity = (keyword, target)
            if identity not in seen:
                seen.add(identity)
                yield keyword, target, replace(instance, target, bad, delete)
        if isinstance(value, dict):
            for key, child in schema.get("properties", {}).items():
                if key in value:
                    yield from visit(child, value[key], base, path + (key,))
        elif isinstance(value, list) and "items" in schema:
            for index, item in enumerate(value):
                yield from visit(schema["items"], item, base, path + (index,))
    yield from visit(root["$defs"][definition], instance, root["$id"], ())


def rehash(snapshot):
    """Re-sign a forged fixture to ensure semantic negatives do not fail only on outer hash."""
    for template in snapshot["template_dependencies"]:
        template["manifest_hash"] = object_hash("TemplateDependency", template)
    for skill in snapshot["skills"]:
        skill["skill_hash"] = object_hash("SkillDefinition", skill)
    definition = snapshot["definition"]
    definition["available_skills"] = [{key: item[key] for key in ("skill_id", "skill_version", "skill_hash")}
                                       for item in snapshot["skills"]]
    definition["definition_hash"] = object_hash("AgentDefinition", definition)
    snapshot["definition_hash"] = definition["definition_hash"]
    snapshot["snapshot_hash"] = object_hash("DefinitionSnapshot", snapshot)
    return snapshot


def run():
    oracle = Oracle(source=ROOT / "design/contracts/releases/1.1.0")
    snapshot = strict_loads((HERE / "fixtures/definition-snapshot.valid.json").read_bytes())
    load_content = lambda ref: (HERE / "fixtures/content" / ref["hash"]).read_bytes()
    records = []
    def case(name, function, rejects=False, kind="oracle-self-test"):
        try:
            function()
        except ContractError:
            if not rejects:
                raise
        else:
            if rejects:
                raise AssertionError("negative was accepted: " + name)
        records.append({"name": name, "status": "passed", "expected": "reject" if rejects else "accept", "kind": kind})

    case("synthetic_definition_closure", lambda: verify_definition_snapshot(snapshot, oracle, load_content, "rules"))
    case("schema/network-reference-denied", lambda: oracle._check_refs(
        {"$ref": "https://not-registered.invalid/schema.json"}, oracle.schemas["agent.schema.json"]["$id"]), True)
    # The public S0 examples are shape-only fixtures. Explicitly migrate their release
    # fields, retaining all deliberate errors and placeholder hashes; never call them
    # trusted product snapshots or use them for semantic acceptance.
    def migrate_example(value):
        if isinstance(value, dict):
            if "contract_version" in value:
                value["contract_version"] = "1.1.0"
            if "max_commands" in value and "max_model_attempts" in value:
                value.update(max_tool_seconds=720, max_execution_seconds=7200,
                             max_ci_wait_seconds=1800, max_ci_poll_attempts=128,
                             max_reconciliation_seconds=1800, max_reconciliation_attempts=64)
            if isinstance(value.get("content_ref"), dict) and value["content_ref"].get("object_type") == "template":
                ref = value["content_ref"]
                ref.update(object_type="template_content", object_id="template_content_" + ref["hash"],
                           object_version=1, source_version=1)
            for child in value.values():
                migrate_example(child)
        elif isinstance(value, list):
            for child in value:
                migrate_example(child)
    example_dir = ROOT / "design/contracts/examples"
    index = strict_loads((example_dir / "INDEX.json").read_bytes())
    for entry in index["cases"]:
        name = entry["definition"] or ("MessageEnvelope" if entry["schema"] == "message.schema.json" else "RunSnapshot")
        def check_example(e=entry, n=name):
            value = strict_loads((example_dir / e["file"]).read_bytes())
            migrate_example(value)
            oracle.validate(n, value, e["schema"])
        case("published-example/" + entry["id"],
             check_example,
             not entry["valid"], "published-schema-example")
    for keyword, path, mutated in mutation_cases(oracle, snapshot, "DefinitionSnapshot"):
        case("schema/" + keyword + "/" + "/".join(map(str, path)),
             lambda item=mutated: oracle.validate("DefinitionSnapshot", item), True, "schema-negative")
    for skill in snapshot["skills"]:
        # Test actual embedded parameter validators, separately from the SkillDefinition envelope.
        case("parameters/" + skill["skill_id"], lambda s=skill: oracle.validate_parameters(s, s["default_parameters"]))
        for key in skill["default_parameters"]:
            bad = deepcopy(skill["default_parameters"])
            del bad[key]
            case("parameters/missing/" + skill["skill_id"] + "/" + key,
                 lambda s=skill, p=bad: oracle.validate_parameters(s, p), True)
        bad = dict(skill["default_parameters"], __unknown__=True)
        case("parameters/unknown/" + skill["skill_id"], lambda s=skill, p=bad: oracle.validate_parameters(s, p), True)

    vectors = strict_loads((HERE / "fixtures/jcs-vectors.json").read_bytes())
    for vector in vectors["positive"]:
        actual = canonicalize(strict_loads(vector["input_json"]))
        assert actual.decode() == vector["expected_canonical"] and byte_hash(actual) == vector["expected_sha256"]
        records.append({"name": "jcs/" + vector["id"], "status": "passed", "kind": "reference-vector"})
    for vector in vectors["strict_json_negative"]:
        case("json/" + vector["id"], lambda v=vector: strict_loads(v["input_json"]), True)
    case("json/byte-bound", lambda: strict_loads('"12345"', max_bytes=3), True)
    case("json/depth-bound", lambda: strict_loads("[" * 129 + "0" + "]" * 129), True)
    case("json/depth-boundary-valid", lambda: strict_loads("[" * 128 + "0" + "]" * 128))
    case("json/value-node-bound", lambda: strict_loads("[" + ",".join(["0"] * 500000) + "]"), True)
    case("json/number-lexeme-bound", lambda: strict_loads("0." + "0" * 127 + "1"), True)
    case("json/exponent-bound", lambda: strict_loads("1e-401"), True)
    case("json/exponent-boundary-valid", lambda: strict_loads("1.23e-400"))
    case("json/invalid-utf8", lambda: strict_loads(b'{"x":"\xff"}'), True)
    case("json/multiple-documents", lambda: strict_loads('{} {}'), True)
    case("json/nested-duplicate", lambda: strict_loads('{"x":{"y":1,"y":2}}'), True)
    for literal in ("1.0", "1e0"):
        raw = json.dumps(snapshot).replace('"snapshot_version": 1', '"snapshot_version": ' + literal, 1)
        case("json/exact-integer/" + literal, lambda r=raw: oracle.decode("DefinitionSnapshot", r))
    for literal in ("1.00000000000000000001", "9007199254740991.1", "9.007199254740992e15"):
        raw = json.dumps(snapshot).replace('"snapshot_version": 1', '"snapshot_version": ' + literal, 1)
        case("json/fractional-or-unsafe-integer/" + literal, lambda r=raw: oracle.decode("DefinitionSnapshot", r), True)

    def semantic(name, edit, preserve_hashes=False):
        mutated = deepcopy(snapshot)
        edit(mutated)
        if not preserve_hashes:
            rehash(mutated)
        # A semantic negative must independently pass structural validation first.
        oracle.validate("DefinitionSnapshot", mutated)
        case("semantic/" + name, lambda: verify_definition_snapshot(mutated, oracle, load_content, "rules"), True, "semantic-negative")
    semantic("prompt-byte-mismatch", lambda s: s["definition"]["system_prompt"].update(content="changed"))
    semantic("schema-byte-mismatch", lambda s: s["schema_dependencies"][0].update(sha256="a" * 64))
    semantic("schema-missing-closure", lambda s: s["schema_dependencies"].__setitem__(1, deepcopy(s["schema_dependencies"][0])))
    semantic("wrong-protocol-target", lambda s: s["definition"]["action_protocol"]["model_output_schema"].update(schema_ref="agent.schema.json#/$defs/ProjectPlan"))
    semantic("wrong-protocol-source", lambda s: s["definition"]["action_protocol"]["model_output_schema"].update(file_sha256="a" * 64))
    semantic("template-byte-mismatch", lambda s: s["template_dependencies"][0]["files"][0].update(sha256="a" * 64))
    semantic("skill-template-not-bound", lambda s: s["template_dependencies"].pop(0))
    semantic("tool-not-bound", lambda s: s["tool_bindings"][0].update(version="go0.0.0"))
    semantic("parameter-default-drift", lambda s: s["skills"][0]["parameters_schema"]["properties"]["auth"].update(default="session"))
    semantic("duplicate-skill-id", lambda s: s["skills"].__setitem__(1, deepcopy(s["skills"][0])))
    semantic("collection-order", lambda s: s["skills"].reverse())
    semantic("check-not-in-policy", lambda s: s["skills"][0]["required_check_ids"].append("check.zzz"))
    semantic("completion-policy-weakened", lambda s: s["definition"]["completion_policy"].update(
        required_checks=[item for item in s["definition"]["completion_policy"]["required_checks"] if item["check_id"] != "check.gitignore_file"]))
    semantic("required-check-delivery-scope-weakened", lambda s: s["definition"]["completion_policy"]["required_checks"][0].update(delivery_modes=["local"]))
    semantic("required-check-evidence-weakened", lambda s: next(item for item in s["definition"]["completion_policy"]["required_checks"]
        if item["check_id"] == "check.gitlab_ci").update(required_evidence_types=["pipeline_jobs"]))
    semantic("post-command-arguments-drift", lambda s: s["skills"][0]["post_commands"][0].update(arguments=["build", "./..."]))
    semantic("definition-command-arguments-drift", lambda s: s["definition"]["post_commands"][0].update(arguments=["build", "./..."]))
    semantic("tool-budget-mismatch", lambda s: s["definition"]["budget_policy"].update(max_tool_seconds=1))
    semantic("snapshot-id-mismatch", lambda s: s.update(definition_version="8.0.0"))
    semantic("outer-hash-mismatch", lambda s: s.update(snapshot_hash="a" * 64), True)
    case("semantic/template-content-corruption", lambda: verify_definition_snapshot(snapshot, oracle, lambda _ref: b"corrupt", "rules"), True)
    case("semantic/provider-mode-mismatch", lambda: verify_definition_snapshot(snapshot, oracle, load_content, "model"), True)
    history = FrozenVersions()
    history.observe(snapshot)
    history.observe(snapshot)
    changed = deepcopy(snapshot)
    changed["definition"]["system_prompt"]["content"] = "new frozen content"
    changed["definition"]["system_prompt"]["sha256"] = byte_hash(b"new frozen content")
    rehash(changed)
    verify_definition_snapshot(changed, oracle, load_content, "rules")
    case("semantic/same-version-changed-content", lambda: history.observe(changed), True)
    changed["definition"]["definition_version"] = changed["definition_version"] = "2.0.0"
    rehash(changed)
    case("semantic/new-version-coexists", lambda: history.observe(changed))

    # Exclusion tests apply public hash rules independently of DTO shape.
    value = {"snapshot_hash": "a", "status": "proposed", "nested": {"hash": "first"}, "order": [1, 2]}
    base_hash = object_hash("Proposal", value)
    assert object_hash("Proposal", dict(value, snapshot_hash="b", status="accepted")) == base_hash
    assert object_hash("Proposal", dict(value, nested={"hash": "second"})) != base_hash
    assert object_hash("Proposal", dict(value, order=[2, 1])) != base_hash
    assert byte_hash(b"template\n") != byte_hash(b"template \n")
    records.append({"name": "hash/fixed-top-level-exclusions-and-byte-sensitivity", "status": "passed", "kind": "semantic-negative"})

    # The original public 1.0.0 remains immutable across new release work.
    prior = strict_loads((ROOT / "docs/evidence/agent-job-prompts-source-audit.json").read_bytes())
    original = [entry for entry in prior["files"] if entry["path"].startswith("design/contracts/schemas/")]
    assert len(original) == 3
    for entry in original:
        assert byte_hash((ROOT / entry["path"]).read_bytes()) == entry["sha256"]
    records.append({"name": "publication/1.0.0-original-bytes-preserved", "status": "passed", "kind": "publication-integrity"})
    # A changed derived copy must fail before any business object is accepted.
    with tempfile.TemporaryDirectory(prefix="oracle-tamper-") as temporary:
        import shutil
        shutil.copytree(HERE / "schemas", Path(temporary) / "schemas")
        path = Path(temporary) / "schemas/agent.schema.json"
        path.write_bytes(path.read_bytes() + b" ")
        case("publication/derived-copy-tamper", lambda: Oracle(Path(temporary) / "schemas"), True)
    return {"status": "passed", "scope": "independent oracle self-test, synthetic fixtures; not product or live evidence",
            "recorded_at": datetime.now(timezone.utc).isoformat(), "contract_version": "1.1.0",
            "dependencies": {name: version(name) for name in ("jsonschema", "referencing", "rfc8785", "rfc3339-validator")},
            "schema_publication": oracle.publication, "static_refs": oracle.static_refs,
            "counts": {"passed": len(records), "failed": 0,
                       "schema_negative": sum(item["kind"] == "schema-negative" for item in records)}, "cases": records}


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--report", type=Path)
    args = parser.parse_args()
    report = run()
    if args.report:
        args.report.parent.mkdir(parents=True, exist_ok=True)
        args.report.write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n")
    print(json.dumps({key: value for key, value in report.items() if key not in ("cases", "schema_publication")}, ensure_ascii=False))


if __name__ == "__main__":
    main()
