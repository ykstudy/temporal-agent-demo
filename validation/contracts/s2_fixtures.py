"""Fresh, explicitly synthetic contract 1.2.0 fixtures, never product migration."""
from copy import deepcopy
from datetime import datetime, timedelta, timezone
import json

from .oracle import HERE, Oracle, byte_hash, canonicalize, strict_loads
from .semantic_checks import HASH_RULES, object_hash
from .s2_semantics import ARGV, STAGES, action_scope, budget_totals, ref_for, required_permissions

RELEASE = HERE / "releases/1.2.0"


def freeze(name, value):
    value[HASH_RULES[name][0]] = object_hash(name, value)
    return value


def timestamp(seconds):
    return (datetime(2026, 10, 2, 2, 0, tzinfo=timezone.utc) + timedelta(seconds=seconds)).isoformat().replace("+00:00", "Z")


def fresh_definition(oracle):
    # Reuse only an independently-authored synthetic shape. Publish new identities,
    # versions, prompt, template bytes and hashes. This is never an old Run adapter.
    snapshot = strict_loads((HERE / "fixtures/definition-snapshot.valid.json").read_bytes())
    def versions(value):
        if isinstance(value, dict):
            for key, child in value.items():
                if key == "contract_version": value[key] = "1.2.0"
                elif key in ("definition_version", "skill_version", "template_version"): value[key] = "2.0.0-synthetic"
                elif key == "schema_ref": value["file_sha256"] = byte_hash(oracle.raw[child.split("#")[0]])
                else: versions(child)
        elif isinstance(value, list):
            for child in value: versions(child)
    versions(snapshot)
    snapshot["definition_id"] = snapshot["definition"]["definition_id"] = "synthetic-s2-agent"
    snapshot["definition"]["system_prompt"]["content"] = "Independent S2 synthetic fixture. Advice never authorizes execution."
    snapshot["definition"]["system_prompt"]["sha256"] = byte_hash(snapshot["definition"]["system_prompt"]["content"].encode())
    content = {}
    for template in snapshot["template_dependencies"]:
        for file in template["files"]:
            raw = ("// Synthetic S2 oracle template; no product execution.\n// " + template["template_id"] + "/" + file["path"] + "\n").encode()
            digest = byte_hash(raw)
            file["sha256"] = digest
            file["content_ref"] = {"object_type": "template_content", "object_id": "template_content_" + digest,
                                   "object_version": 1, "source_version": 1, "hash": digest}
            content[digest] = raw
        freeze("TemplateDependency", template)
    templates = {item["template_id"]: item for item in snapshot["template_dependencies"]}
    for skill in snapshot["skills"]:
        skill["template_dependencies"] = [deepcopy(templates[item["template_id"]]) for item in skill["template_dependencies"]]
        freeze("SkillDefinition", skill)
    snapshot["definition"]["available_skills"] = [{key: item[key] for key in ("skill_id", "skill_version", "skill_hash")}
                                                   for item in snapshot["skills"]]
    for item in snapshot["schema_dependencies"]:
        item["sha256"] = byte_hash(oracle.raw[item["schema_uri"]])
        content[item["sha256"]] = oracle.raw[item["schema_uri"]]
    freeze("AgentDefinition", snapshot["definition"])
    snapshot["definition_hash"] = snapshot["definition"]["definition_hash"]
    freeze("DefinitionSnapshot", snapshot)
    return snapshot, content


def build(oracle=None):
    oracle = oracle or Oracle(RELEASE)
    snapshot, content = fresh_definition(oracle)
    run_id, version = "synthetic-s2-run", "1.2.0"
    target = {"provider": "gitlab", "instance_url": "http://127.0.0.1:1", "namespace_id": "1", "namespace_path": "synthetic",
              "project_path": "synthetic-project", "ref": "main", "visibility": "private"}
    plan = freeze("ProjectPlan", {"contract_version": version, "project_name": "synthetic-project", "description": "Synthetic S2 oracle fixture",
        "tech_stack": "golang", "selected_skills": [{"skill_id": item["skill_id"], "skill_version": item["skill_version"],
        "skill_hash": item["skill_hash"], "parameters": deepcopy(item["default_parameters"])} for item in snapshot["skills"]],
        "template_dependencies": deepcopy(snapshot["template_dependencies"]), "delivery_target": target})
    permissions = sorted({"workspace_read", "workspace_write", "tool_go", "gitlab_create", "gitlab_push", "gitlab_ci", "package_write"})
    permission = freeze("PermissionSnapshot", {"permission_version": 1, "actor_id": "synthetic-actor", "allowed_permissions": permissions,
        "delivery_target": target, "revoked": False, "updated_at": timestamp(0)})
    paths = ["README.md"] + [item["write_paths"][0] for item in snapshot["skills"]]
    file_bytes = {path: ("Synthetic approved file: " + path + "\n").encode() for path in paths}
    changes = [{"path": path, "operation": "create", "before": None, "before_sha256": None,
                "after": raw.decode(), "after_sha256": byte_hash(raw)} for path, raw in sorted(file_bytes.items())]
    proposal = freeze("Proposal", {"contract_version": version, "proposal_id": "synthetic-proposal", "run_id": run_id,
        "proposal_version": 1, "source_version": 1, "definition_snapshot_hash": snapshot["snapshot_hash"], "plan_hash": plan["plan_hash"],
        "title": "Synthetic seven-action proposal", "status": "pending", "plan": plan, "changes": changes,
        "impact": ["Synthetic oracle objects, not observed execution"], "allowed_actions": list(STAGES),
        "required_permission_ids": permissions, "permission_hash": permission["permission_hash"],
        "checks": [{"check_id": "check.synthetic", "passed": True, "detail": "Synthetic fixture check"}],
        "issued_at": timestamp(0), "expires_at": timestamp(1800)})
    approval = freeze("Approval", {"contract_version": version, "approval_id": "synthetic-approval", "run_id": run_id,
        "actor_id": permission["actor_id"], "proposal_id": proposal["proposal_id"], "proposal_version": 1, "source_version": 1,
        "proposal_snapshot_hash": proposal["snapshot_hash"], "definition_snapshot_hash": snapshot["snapshot_hash"],
        "plan_hash": plan["plan_hash"], "permission_hash": permission["permission_hash"], "allowed_actions": list(STAGES),
        "delivery_target": target, "issued_at": timestamp(0), "expires_at": timestamp(1800), "accepted_at": timestamp(1),
        "started_at": None, "revoked_at": None, "status": "accepted"})
    message = {"schema_version": 2, "contract_version": version, "run_id": run_id, "message_id": "synthetic-message",
        "sequence": 1, "source_version": 1, "state_sequence": 1, "role": "user", "created_at": timestamp(0),
        "modes": {"execution_mode": "local", "decision_mode": "rules", "delivery_mode": "gitlab"}, "command_id": "synthetic-command",
        "parts": [{"part_id": "synthetic-text", "part_version": 1, "type": "text", "fallback_text": "Create a synthetic project.",
                   "text": "Create a synthetic project.", "format": "plain"}]}
    context_messages = [{"role": "system", "content": snapshot["definition"]["system_prompt"]["content"]},
                        {"role": "user", "content": message["parts"][0]["text"]}]
    workspace = {"workspace_id": "synthetic-workspace", "run_id": run_id, "storage_key": "synthetic/project"}
    ledger = {"contract_version": version, "run_id": run_id, "definition_snapshot_hash": snapshot["snapshot_hash"],
        "policy_hash": byte_hash(canonicalize(snapshot["definition"]["budget_policy"])), "ledger_version": 1,
        "execution_started_at": timestamp(10), "execution_deadline_at": timestamp(7210), "ci_windows": [], "reconciliation_windows": [],
        "reservations": [], "settlements": [], "totals": {}, "updated_at": timestamp(100)}
    bundle = {"contract_version": version, "schema_version": 2, "run_id": run_id, "modes": message["modes"], "state": "DELIVER",
        "source_version": 1, "state_sequence": 50, "definition_snapshot": snapshot, "plan": plan, "proposal": deepcopy(proposal),
        "approval": deepcopy(approval), "permission_snapshot": permission, "proposal_history": [proposal], "approval_history": [approval],
        "permission_history": [permission], "decisions": [], "validations": [], "executions": [], "messages": [message], "contexts": [],
        "operations": [], "manifest": None, "manifest_history": [], "budget": ledger, "tool_receipts": [], "workspace_receipts": [],
        "created_at": timestamp(100)}
    bundle["proposal"]["status"] = "accepted"
    bundle["approval"].update(status="started", started_at=timestamp(10))
    binding = {key: approval[key] for key in ("approval_id", "proposal_id", "proposal_version", "source_version", "proposal_snapshot_hash",
                                            "definition_snapshot_hash", "permission_hash", "issued_at", "expires_at")}
    states = {}
    def reserve(kind, scope, seconds, execution=None, outcome="succeeded", elapsed=1000):
        n = len(ledger["reservations"]) + 1
        item = {"contract_version": version, "reservation_id": "synthetic-reservation-" + str(n), "run_id": run_id, "source_version": 1,
            "definition_snapshot_hash": snapshot["snapshot_hash"], "policy_hash": ledger["policy_hash"], "command_id": "synthetic-command",
            "operation_id": execution["operation_id"] if execution else None, "execution_id": execution["execution_id"] if execution else None,
            "request_hash": execution["request_hash"] if execution else None, "attempt_id": "synthetic-attempt-" + str(n), "kind": kind,
            "scope_id": scope, "units": 1, "reserved_at": timestamp(seconds),
            "deadline_at": None if kind == "command" else timestamp(seconds + 720 if kind == "tool" else 7210)}
        freeze("BudgetReservation", item)
        ledger["reservations"].append(item)
        settlement = {"contract_version": version, "settlement_id": "synthetic-settlement-" + str(n), "reservation_id": item["reservation_id"],
            "reservation_hash": item["reservation_hash"], "attempt_id": item["attempt_id"], "run_id": run_id, "source_version": 1,
            "consumed_units": 0 if outcome == "released" else 1, "outcome": outcome, "elapsed_ms": elapsed,
            "measurement_source": "unknown" if elapsed is None else "actual", "settled_at": timestamp(seconds + 1)}
        freeze("BudgetSettlement", settlement)
        ledger["settlements"].append(settlement)
        return item
    reserve("command", "synthetic-command", 0)
    specs = [("scaffold", {"plan_hash": plan["plan_hash"]})]
    specs.extend(("apply_skill", {"plan_hash": plan["plan_hash"], "selection": selection}) for selection in plan["selected_skills"])
    specs += [("verify", None), ("create_repository", {"target": target}), ("push_repository", None),
              ("trigger_ci", {"target": target, "project_id": 1, "commit_sha": "a" * 40, "ci_config_sha256": "b" * 64}), ("package", None)]
    for index, (action_type, parameters) in enumerate(specs):
        number, seconds, state_sequence = index + 1, 10 + index * 3, 10 + index
        input_manifest = ref_for("ArtifactManifest", bundle["manifest"]) if bundle["manifest"] else None
        if action_type == "verify": parameters = {"manifest_ref": input_manifest, "required_check_ids": ["check.go_build", "check.go_mod_tidy", "check.go_test"]}
        elif action_type == "push_repository": parameters = {"target": target, "project_id": 1, "manifest_ref": input_manifest, "expected_branch_sha": None}
        elif action_type == "package": parameters = {"manifest_ref": input_manifest}
        action = {"action_type": action_type, "parameters": parameters}
        advice = {"kind": "action", "question": None, "plan": None, "action": action, "diagnosis": None, "completion_hint": None,
                  "reason": "Synthetic deterministic scheduled action"}
        decision = freeze("DecisionProposal", {"contract_version": version, "decision_id": "synthetic-decision-" + str(number),
            "run_id": run_id, "source_version": 1, "state_sequence": state_sequence, "definition_snapshot_hash": snapshot["snapshot_hash"],
            "decision_mode": "rules", "advice": advice, "model_usage": {"status": "not_applicable", "input_tokens": None, "output_tokens": None,
            "total_tokens": None, "provider_request_id": None}, "input_message_refs": [ref_for("MessageEnvelope", message)],
            "input_context_hash": byte_hash(canonicalize(context_messages)), "created_at": timestamp(seconds)})
        context = freeze("ContextRecord", {key: decision[key] for key in ("contract_version", "run_id", "decision_id", "source_version",
            "state_sequence", "definition_snapshot_hash", "input_message_refs", "input_context_hash", "created_at")} |
            {"context_id": "synthetic-context-" + str(number), "messages": context_messages})
        validation = freeze("ActionValidation", {"contract_version": version, "validation_id": "synthetic-validation-" + str(number),
            "decision_id": decision["decision_id"], "run_id": run_id, "source_version": 1, "state_sequence": state_sequence,
            "definition_snapshot_hash": snapshot["snapshot_hash"], "decision_kind": "action", "verdict": "allowed",
            "findings": [{"rule_id": "rule.synthetic", "passed": True, "code": "allowed", "detail": "Synthetic rule fixture"}],
            "normalized_advice": advice, "execution_request": None, "validated_at": timestamp(seconds)})
        execution = {"contract_version": version, "execution_id": "synthetic-execution-" + str(number), "operation_id": "synthetic-operation-" + str(number),
            "command_id": "synthetic-command", "run_id": run_id, "source_version": 1, "state_sequence_at_validation": state_sequence,
            "definition_snapshot_hash": snapshot["snapshot_hash"], "plan_hash": plan["plan_hash"], "validation_id": validation["validation_id"],
            "validation_hash": validation["validation_hash"], "approval": binding, "workspace": workspace, "action": action,
            "required_permission_ids": [], "created_at": timestamp(seconds)}
        execution["required_permission_ids"] = required_permissions(execution, snapshot)
        freeze("ExecutionRequest", execution)
        validation["execution_request"] = execution
        bundle["decisions"].append(decision); bundle["contexts"].append(context); bundle["validations"].append(validation); bundle["executions"].append(execution)
        states[state_sequence] = STAGES[action_type]
        reservation = reserve("action", action_scope(execution), seconds, execution)
        operation = {"operation_id": execution["operation_id"], "run_id": run_id, "source_version": 1, "action_type": action_type,
            "operation_sequence": number, "request_hash": execution["request_hash"], "target_hash": byte_hash(canonicalize(parameters)),
            "status": "verified", "external_object_id": str(number) if action_type in ("create_repository", "push_repository", "trigger_ci") else None,
            "evidence_refs": [], "created_at": timestamp(seconds), "updated_at": timestamp(seconds + 1)}
        bundle["operations"].append(operation)
        if action_type in ("scaffold", "apply_skill"):
            files = [{"artifact_id": "synthetic-artifact-" + str(n), "path": path, "size": len(file_bytes[path]),
                      "sha256": byte_hash(file_bytes[path]), "media_type": "text/plain"} for n, path in enumerate(paths[:number], 1)]
            manifest = freeze("ArtifactManifest", {"contract_version": version, "manifest_id": "synthetic-manifest-" + str(number),
                "run_id": run_id, "source_version": 1, "proposal_id": proposal["proposal_id"], "proposal_version": 1,
                "proposal_snapshot_hash": proposal["snapshot_hash"], "definition_snapshot_hash": snapshot["snapshot_hash"], "plan_hash": plan["plan_hash"],
                "files": sorted(files, key=lambda item: item["path"]), "created_at": timestamp(seconds + 1), "manifest_version": number})
            bundle["manifest"] = manifest; bundle["manifest_history"].append(manifest)
            receipt = {"contract_version": version, "receipt_id": "synthetic-workspace-receipt-" + str(number), "run_id": run_id,
                "source_version": 1, "definition_snapshot_hash": snapshot["snapshot_hash"], "execution_id": execution["execution_id"],
                "operation_id": execution["operation_id"], "attempt_id": reservation["attempt_id"], "request_hash": execution["request_hash"],
                "approval_id": approval["approval_id"], "approval_hash": approval["approval_hash"], "proposal_snapshot_hash": proposal["snapshot_hash"],
                "plan_hash": plan["plan_hash"], "workspace": workspace, "action_type": action_type, "written_paths": [paths[index]],
                "input_manifest_ref": input_manifest, "output_manifest_ref": ref_for("ArtifactManifest", manifest), "started_at": timestamp(seconds),
                "completed_at": timestamp(seconds + 1), "status": "succeeded"}
            freeze("WorkspaceReceipt", receipt); bundle["workspace_receipts"].append(receipt)
            operation["evidence_refs"].append(ref_for("WorkspaceReceipt", receipt))
        if action_type == "verify":
            operation["updated_at"] = timestamp(seconds + 3)
            action_settlement = next(item for item in ledger["settlements"] if item["reservation_id"] == reservation["reservation_id"])
            action_settlement.update(settled_at=timestamp(seconds + 3), elapsed_ms=3000)
            freeze("BudgetSettlement", action_settlement)
            for command_index, (command_id, arguments) in enumerate(ARGV.items()):
                tool_second = seconds + command_index
                tool_reservation = reserve("tool", "tool:" + command_id, tool_second, execution)
                receipt = {"contract_version": version, "receipt_id": "synthetic-tool-receipt-" + str(command_index + 1), "run_id": run_id,
                    "source_version": 1, "definition_snapshot_hash": snapshot["snapshot_hash"], "execution_id": execution["execution_id"],
                    "operation_id": execution["operation_id"], "attempt_id": tool_reservation["attempt_id"], "request_hash": execution["request_hash"],
                    "approval_id": approval["approval_id"], "approval_hash": approval["approval_hash"], "proposal_snapshot_hash": proposal["snapshot_hash"],
                    "plan_hash": plan["plan_hash"], "workspace": workspace, "tool": deepcopy(snapshot["tool_bindings"][0]), "command_id": command_id,
                    "arguments": arguments, "cwd_scope": "run_workspace", "environment_policy": "isolated", "input_manifest_ref": input_manifest,
                    "output_manifest_ref": input_manifest, "started_at": timestamp(tool_second), "completed_at": timestamp(tool_second + 1),
                    "exit_code": 0, "status": "succeeded", "output": "synthetic redacted output\n", "output_sha256": byte_hash(b"synthetic redacted output\n"),
                    "output_truncated": False}
                freeze("ToolExecutionReceipt", receipt); bundle["tool_receipts"].append(receipt)
                operation["evidence_refs"].append(ref_for("ToolExecutionReceipt", receipt))
    for kind, action, seconds in (("ci_poll", "trigger_ci", 80), ("reconciliation_read", "create_repository", 85)):
        execution = next(item for item in bundle["executions"] if item["action"]["action_type"] == action)
        window = {"window_id": "synthetic-window-" + kind, "kind": kind, "operation_id": execution["operation_id"],
            "scope_id": kind + ":" + execution["operation_id"], "source_version": 1, "started_at": timestamp(seconds),
            "deadline_at": timestamp(seconds + 1800), "max_attempts": 128 if kind == "ci_poll" else 64}
        ledger["ci_windows" if kind == "ci_poll" else "reconciliation_windows"].append(window)
        reservation = reserve(kind, window["scope_id"], seconds, execution)
        reservation["deadline_at"] = window["deadline_at"]; freeze("BudgetReservation", reservation)
        ledger["settlements"][-1]["reservation_hash"] = reservation["reservation_hash"]; freeze("BudgetSettlement", ledger["settlements"][-1])
    ledger["reservations"].sort(key=lambda item: item["reserved_at"])
    ledger["settlements"].sort(key=lambda item: item["settled_at"])
    ledger["totals"] = budget_totals(ledger["reservations"], ledger["settlements"])
    freeze("BudgetLedger", ledger); freeze("S2AuditBundle", bundle)
    # Serialize to remove Python aliases: each public occurrence has its own body.
    return strict_loads(canonicalize(bundle)), content, states


def main():
    bundle, content, states = build()
    destination = HERE / "fixtures/s2"
    (destination / "content").mkdir(parents=True, exist_ok=True)
    (destination / "audit-bundle.valid.json").write_text(json.dumps(bundle, ensure_ascii=False, indent=2) + "\n")
    (destination / "synthetic-state-observations.json").write_text(json.dumps(states, indent=2) + "\n")
    for digest, raw in content.items():
        (destination / "content" / digest).write_bytes(raw)
    provenance = {"kind": "synthetic_oracle_fixture", "not_product_evidence": True, "contract_version": "1.2.0",
        "shape_source": "validation/contracts/fixtures/definition-snapshot.valid.json",
        "shape_source_sha256": byte_hash((HERE / "fixtures/definition-snapshot.valid.json").read_bytes()),
        "publication": "validation/contracts/releases/1.2.0/publication.json",
        "audit_hash": bundle["audit_hash"], "content": [{"sha256": key, "size": len(raw)} for key, raw in sorted(content.items())]}
    (destination / "provenance.json").write_text(json.dumps(provenance, ensure_ascii=False, indent=2) + "\n")
    print("built fresh independent synthetic S2 audit; no product execution")


if __name__ == "__main__": main()
