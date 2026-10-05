"""Independent semantic rules from public contracts, not product implementation types."""
from __future__ import annotations

from copy import deepcopy
from typing import Callable

from .oracle import ContractError, Oracle, byte_hash, canonicalize

# Fixed TOP-LEVEL exclusions from AGENT-CONTRACTS.md. Never recursively strip hashes.
HASH_RULES = {
    "AgentDefinition": ("definition_hash", ("definition_hash",)),
    "SkillDefinition": ("skill_hash", ("skill_hash",)),
    "DefinitionSnapshot": ("snapshot_hash", ("snapshot_hash",)),
    "ProjectPlan": ("plan_hash", ("plan_hash",)),
    "DecisionProposal": ("decision_hash", ("decision_hash",)),
    "ActionValidation": ("validation_hash", ("validation_hash", "execution_request")),
    "ExecutionRequest": ("request_hash", ("request_hash",)),
    "Proposal": ("snapshot_hash", ("snapshot_hash", "status")),
    "Approval": ("approval_hash", ("approval_hash", "status", "started_at", "revoked_at")),
    "PermissionSnapshot": ("permission_hash", ("permission_hash",)),
    "ArtifactManifest": ("hash", ("hash",)),
    "VerificationReport": ("report_hash", ("report_hash",)),
    "SummaryRecord": ("summary_hash", ("summary_hash",)),
    "SummaryAttempt": ("attempt_hash", ("attempt_hash",)),
    "ConversationAuditBundle": ("audit_hash", ("audit_hash",)),
    "RunSnapshot": ("snapshot_hash", ("snapshot_hash",)),
    "TemplateDependency": ("manifest_hash", ("manifest_hash",)),
    "MessageEnvelope": (None, ()),
    "BudgetReservation": ("reservation_hash", ("reservation_hash",)),
    "BudgetSettlement": ("settlement_hash", ("settlement_hash",)),
    "BudgetLedger": ("ledger_hash", ("ledger_hash",)),
    "ContextRecord": ("context_hash", ("context_hash",)),
    "ToolExecutionReceipt": ("receipt_hash", ("receipt_hash",)),
    "WorkspaceReceipt": ("receipt_hash", ("receipt_hash",)),
    "S2AuditBundle": ("audit_hash", ("audit_hash",)),
    "VerificationCandidate": ("candidate_hash", ("candidate_hash",)),
    "PackageReceipt": ("receipt_hash", ("receipt_hash",)),
    "RemoteExecutionReceipt": ("receipt_hash", ("receipt_hash",)),
}

# Existing Go delivery requirements, published by AGENT-GUIDE and the contract's
# completion semantics. Extra registered checks remain allowed; baseline evidence
# may not be removed by re-hashing a weaker Definition.
BASELINE_CHECKS = {
    "check.approved_sources": ("source", {"source_hash"}),
    "check.artifact_manifest": ("manifest", {"manifest_hash"}),
    "check.go_mod_tidy": ("tool", {"tool_exit"}),
    "check.go_test": ("tool", {"tool_exit"}),
    "check.go_build": ("tool", {"tool_exit"}),
    "check.go_mod_file": ("file", {"file_hash"}),
    "check.gitignore_file": ("file", {"file_hash"}),
    "check.ci_config_file": ("file", {"file_hash"}),
    "check.readme_file": ("file", {"file_hash"}),
    "check.secret_boundary": ("secret_boundary", {"secret_scan"}),
    "check.zip_integrity": ("package", {"zip_integrity"}),
    "check.remote_repository": ("remote_repository", {"remote_identity"}),
    "check.remote_commit": ("remote_commit", {"commit_tree"}),
    "check.gitlab_ci": ("ci", {"pipeline_jobs", "coverage_report", "build_artifact"}),
}


def require(condition: bool, message: str):
    if not condition:
        raise ContractError(message)


def object_hash(name: str, value: dict) -> str:
    require(name in HASH_RULES, "unknown object hash rule")
    excluded = HASH_RULES[name][1]
    return byte_hash(canonicalize({key: item for key, item in value.items() if key not in excluded}))


def verify_hash(name: str, value: dict):
    field = HASH_RULES[name][0]
    require(field is not None and value[field] == object_hash(name, value), name + " content hash mismatch")


def sorted_unique(items: list, key: Callable, label: str):
    keys = [key(item) for item in items]
    require(len(keys) == len(set(keys)), label + " duplicate identity")
    require(keys == sorted(keys), label + " not in canonical collection order")


def tool_key(value):
    return value["tool_id"], value["platform"] or ""


def template_key(value):
    return value["template_id"], value["template_version"]


def ref_key(value):
    return (value["object_type"], value["object_id"], value["object_version"],
            value["source_version"], value["hash"])


class FrozenVersions:
    """Tracks observed public content, and rejects same-version/different-content reuse."""
    def __init__(self):
        self._seen = {}

    def observe(self, snapshot: dict):
        definition = snapshot["definition"]
        entries = [(('definition', definition["definition_id"], definition["definition_version"]),
                    definition["definition_hash"])]
        entries += [(('skill', item["skill_id"], item["skill_version"]), item["skill_hash"])
                    for item in snapshot["skills"]]
        entries += [(('template', item["template_id"], item["template_version"]), item["manifest_hash"])
                    for item in snapshot["template_dependencies"]]
        for key, digest in entries:
            require(key not in self._seen or self._seen[key] == digest,
                    "published version reused for different content")
        self._seen.update(entries)


def verify_definition_snapshot(snapshot: dict, oracle: Oracle,
                               content_loader: Callable[[dict], bytes],
                               expected_decision_mode: str | None = None):
    """Verify full closure; content_loader must retrieve published bytes, never private assets.

    Returns byte/hash counts, never claims template execution, authorization, or Run v2.
    """
    oracle.validate("DefinitionSnapshot", snapshot)
    definition = snapshot["definition"]
    verify_hash("AgentDefinition", definition)
    verify_hash("DefinitionSnapshot", snapshot)
    for field in ("contract_version", "definition_id", "definition_version", "definition_hash"):
        require(snapshot[field] == definition[field], "snapshot/definition identity mismatch")
    prompt = definition["system_prompt"]
    require(byte_hash(prompt["content"].encode("utf-8")) == prompt["sha256"], "system prompt byte hash mismatch")

    # A published snapshot is canonical, so order is checked rather than silently repaired.
    sorted_unique(snapshot["skills"], lambda item: item["skill_id"], "skills")
    sorted_unique(definition["available_skills"], lambda item: item["skill_id"], "available skills")
    skills = {item["skill_id"]: item for item in snapshot["skills"]}
    refs = [{key: item[key] for key in ("skill_id", "skill_version", "skill_hash")}
            for item in snapshot["skills"]]
    require(refs == definition["available_skills"], "available skills do not bind exact frozen skills")

    deps = snapshot["schema_dependencies"]
    sorted_unique(deps, lambda item: item["schema_uri"], "schema dependencies")
    require({item["schema_uri"] for item in deps} == set(oracle.raw), "incomplete schema dependency closure")
    for item in deps:
        require(item["contract_version"] == snapshot["contract_version"], "schema dependency version mismatch")
        require(item["sha256"] == byte_hash(oracle.raw[item["schema_uri"]]), "schema dependency byte digest mismatch")
    protocol = definition["action_protocol"]
    for field, target in {"model_output_schema": "DecisionAdvice", "decision_proposal_schema": "DecisionProposal",
                          "action_validation_schema": "ActionValidation", "execution_request_schema": "ExecutionRequest"}.items():
        require(protocol[field]["schema_ref"] == "agent.schema.json#/$defs/" + target,
                "action protocol bound to the wrong schema definition")
        oracle.schema_reference(protocol[field])

    sorted_unique(snapshot["template_dependencies"], template_key, "template dependencies")
    templates = {template_key(item): item for item in snapshot["template_dependencies"]}
    published_contents = {}
    file_count = 0
    for item in templates.values():
        verify_hash("TemplateDependency", item)
        sorted_unique(item["files"], lambda f: f["path"], "template files")
        for file in item["files"]:
            ref = file["content_ref"]
            # 1.0.0's ambiguous template-content reference is intentionally not inferred.
            require(ref["object_type"] == "template_content", "template bytes require explicit template_content reference")
            require(ref["hash"] == file["sha256"], "template content reference hash mismatch")
            require(ref["object_id"] == "template_content_" + file["sha256"] and
                    ref["object_version"] == 1 and ref["source_version"] == 1,
                    "template content reference identity/version mismatch")
            raw = content_loader(ref)
            require(type(raw) is bytes, "content loader must return exact published bytes")
            require(byte_hash(raw) == file["sha256"], "template file byte digest mismatch")
            try:
                raw.decode("utf-8", "strict")
            except UnicodeError as error:
                raise ContractError("template content must be UTF-8") from error
            key = ref_key(ref)
            require(key not in published_contents or published_contents[key] == raw,
                    "immutable content reference changed")
            published_contents[key] = raw
            file_count += 1

    sorted_unique(snapshot["tool_bindings"], tool_key, "tool bindings")
    bound_tools = {tool_key(item): item for item in snapshot["tool_bindings"]}
    sorted_unique(definition["required_tools"], tool_key, "definition tools")
    for tool in definition["required_tools"]:
        require(bound_tools.get(tool_key(tool)) == tool, "definition tool binding mismatch")
    checks = definition["completion_policy"]["required_checks"]
    sorted_unique(checks, lambda item: item["check_id"], "completion checks")
    check_ids = {item["check_id"] for item in checks}
    require(set(BASELINE_CHECKS).issubset(check_ids), "completion policy omits required Go delivery checks")
    for check in checks:
        if check["check_id"] in BASELINE_CHECKS:
            kind, evidence = BASELINE_CHECKS[check["check_id"]]
            modes = {"gitlab"} if kind in {"remote_repository", "remote_commit", "ci"} else {"local", "gitlab"}
            require(check["kind"] == kind and evidence.issubset(check["required_evidence_types"]) and
                    set(check["delivery_modes"]) == modes, "required Go check evidence or delivery scope weakened")
    command_ids = [item["command_id"] for item in definition["post_commands"]]
    require(len(command_ids) == len(set(command_ids)), "duplicate fixed post command")
    commands = {item["command_id"]: item for item in definition["post_commands"]}
    fixed_arguments = {"go.mod_tidy": ["mod", "tidy"], "go.test": ["test", "./..."], "go.build": ["build", "./..."]}
    for command in commands.values():
        require(command["arguments"] == fixed_arguments[command["command_id"]],
                "fixed command identity/arguments mismatch")
        require(any(tool["tool_id"] == command["executable"] for tool in definition["required_tools"]),
                "post command executable lacks frozen tool identity")
        require(command["timeout_seconds"] <= definition["budget_policy"]["max_tool_seconds"],
                "fixed command timeout exceeds frozen tool budget")
    input_names = {"add_auth": "AuthenticationParameters", "add_database": "DatabaseParameters", "add_logging": "LoggingParameters"}
    for skill in skills.values():
        verify_hash("SkillDefinition", skill)
        require(skill["contract_version"] == snapshot["contract_version"], "skill contract version mismatch")
        oracle.validate_parameters(skill, skill["default_parameters"])
        oracle.validate(input_names[skill["skill_id"]], skill["default_parameters"])
        defaults = {key: item["default"] for key, item in skill["parameters_schema"]["properties"].items()}
        require(defaults == skill["default_parameters"], "embedded parameter defaults disagree with normalized defaults")
        require(skill["input_contract"]["schema_ref"] == "agent.schema.json#/$defs/" + input_names[skill["skill_id"]], "skill input schema identity mismatch")
        require(skill["output_contract"]["schema_ref"] == "agent.schema.json#/$defs/ArtifactManifest", "skill output schema identity mismatch")
        oracle.schema_reference(skill["input_contract"])
        oracle.schema_reference(skill["output_contract"])
        sorted_unique(skill["dependencies"], lambda item: item["skill_id"], "skill dependencies")
        sorted_unique(skill["template_dependencies"], template_key, "skill templates")
        sorted_unique(skill["required_tools"], tool_key, "skill tools")
        sorted_unique(skill["required_permissions"], lambda item: item, "skill permissions")
        sorted_unique(skill["required_check_ids"], lambda item: item, "skill check identifiers")
        require(set(skill["required_check_ids"]).issubset(check_ids), "skill required check is absent from completion policy")
        for dependency in skill["dependencies"]:
            actual = skills.get(dependency["skill_id"])
            require(actual is not None and all(actual[key] == dependency[key] for key in dependency), "skill dependency version/hash not resolved")
            require(dependency["skill_id"] not in skill["conflicts"], "skill dependency conflicts with required skill")
        require(skill["skill_id"] not in skill["conflicts"], "skill conflicts with itself")
        for item in skill["template_dependencies"]:
            require(templates.get(template_key(item)) == item, "skill template dependency differs from snapshot")
        for tool in skill["required_tools"]:
            require(bound_tools.get(tool_key(tool)) == tool, "skill tool dependency not frozen")
        for command in skill["post_commands"]:
            require(commands.get(command["command_id"]) == command, "skill post command does not bind exact definition command")

    visited, active = set(), set()
    def visit(skill_id):
        require(skill_id not in active, "cyclic skill dependency")
        if skill_id in visited:
            return
        active.add(skill_id)
        for dependency in skills[skill_id]["dependencies"]:
            visit(dependency["skill_id"])
        active.remove(skill_id)
        visited.add(skill_id)
    for skill_id in skills:
        visit(skill_id)
    if expected_decision_mode is not None:
        require(expected_decision_mode in ("rules", "model"), "invalid expected decision mode")
        require((snapshot["provider_binding"] is None) == (expected_decision_mode == "rules"), "provider binding does not match explicit decision mode")
    return {"scope": "DefinitionSnapshot structure and frozen dependency bytes", "skills": len(skills),
            "schemas": len(deps), "template_files": file_count, "snapshot_hash": snapshot["snapshot_hash"],
            "definition_hash": definition["definition_hash"], "contract_version": snapshot["contract_version"]}
