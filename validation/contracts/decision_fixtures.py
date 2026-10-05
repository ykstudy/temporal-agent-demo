"""Synthetic complete S2 examples. These are not observations of a running Agent."""
from copy import deepcopy

from .oracle import HERE, byte_hash, canonicalize, strict_loads
from .semantic_checks import object_hash


def freeze(name, value):
    fields = {"ProjectPlan": "plan_hash", "PermissionSnapshot": "permission_hash",
              "Proposal": "snapshot_hash", "Approval": "approval_hash", "DecisionProposal": "decision_hash",
              "ActionValidation": "validation_hash", "ExecutionRequest": "request_hash"}
    value[fields[name]] = object_hash(name, value)
    return value


def build():
    snapshot = strict_loads((HERE / "fixtures/definition-snapshot.valid.json").read_bytes())
    version = snapshot["contract_version"]
    run_id = "synthetic-s2-run"
    issued, accepted, created = "2026-10-02T02:00:00Z", "2026-10-02T02:00:01Z", "2026-10-02T02:00:02Z"
    expires = "2026-10-02T02:30:00Z"
    target = {"provider": "local", "destination": "run_package"}
    plan = freeze("ProjectPlan", {"contract_version": version, "project_name": "synthetic-project",
        "description": "Synthetic oracle fixture, not generated product evidence", "tech_stack": "golang",
        "selected_skills": [{"skill_id": skill["skill_id"], "skill_version": skill["skill_version"],
                             "skill_hash": skill["skill_hash"], "parameters": deepcopy(skill["default_parameters"])}
                            for skill in snapshot["skills"]],
        "template_dependencies": deepcopy(snapshot["template_dependencies"]), "delivery_target": target})
    permissions = ["package_write", "tool_go", "workspace_read", "workspace_write"]
    permission = freeze("PermissionSnapshot", {"permission_version": 1, "actor_id": "synthetic-actor",
        "allowed_permissions": permissions, "delivery_target": target, "revoked": False, "updated_at": issued})
    actions = ["scaffold", "apply_skill", "verify", "package"]
    content = "# Synthetic project\n"
    proposal = freeze("Proposal", {"contract_version": version, "proposal_id": "synthetic-proposal", "run_id": run_id,
        "proposal_version": 1, "source_version": 1, "definition_snapshot_hash": snapshot["snapshot_hash"],
        "plan_hash": plan["plan_hash"], "title": "Synthetic proposal", "status": "pending", "plan": plan,
        "changes": [{"path": "README.md", "operation": "create", "before": None, "before_sha256": None,
                     "after": content, "after_sha256": byte_hash(content.encode())}],
        "impact": ["Synthetic isolated example"], "allowed_actions": actions,
        "required_permission_ids": permissions, "permission_hash": permission["permission_hash"],
        "checks": [{"check_id": "check.synthetic", "passed": True, "detail": "Synthetic check fixture"}],
        "issued_at": issued, "expires_at": expires})
    approval = freeze("Approval", {"contract_version": version, "approval_id": "synthetic-approval", "run_id": run_id,
        "actor_id": permission["actor_id"], "proposal_id": proposal["proposal_id"], "proposal_version": 1,
        "source_version": 1, "proposal_snapshot_hash": proposal["snapshot_hash"],
        "definition_snapshot_hash": snapshot["snapshot_hash"], "plan_hash": plan["plan_hash"],
        "permission_hash": permission["permission_hash"], "allowed_actions": actions, "delivery_target": target,
        "issued_at": issued, "expires_at": expires, "accepted_at": accepted, "started_at": None,
        "revoked_at": None, "status": "accepted"})
    text = "Create a synthetic Go project."
    message = {"schema_version": 2, "contract_version": version, "run_id": run_id,
        "message_id": "synthetic-message", "sequence": 1, "source_version": 1, "state_sequence": 1,
        "role": "user", "created_at": issued, "modes": {"execution_mode": "local", "decision_mode": "rules", "delivery_mode": "local"},
        "parts": [{"part_id": "synthetic-text", "part_version": 1, "type": "text", "fallback_text": text,
                   "text": text, "format": "plain"}]}
    ref = {"object_type": "message", "object_id": message["message_id"], "object_version": 1,
           "source_version": 1, "hash": object_hash("MessageEnvelope", message)}
    context = [{"role": "system", "content": snapshot["definition"]["system_prompt"]["content"]},
               {"role": "user", "content": text}]
    advice = {"kind": "action", "question": None, "plan": None, "action": {"action_type": "scaffold",
              "parameters": {"plan_hash": plan["plan_hash"]}}, "diagnosis": None, "completion_hint": None,
              "reason": "Synthetic fixed action"}
    decision = freeze("DecisionProposal", {"contract_version": version, "decision_id": "synthetic-decision",
        "run_id": run_id, "source_version": 1, "state_sequence": 2,
        "definition_snapshot_hash": snapshot["snapshot_hash"], "decision_mode": "rules", "advice": advice,
        "model_usage": {"status": "not_applicable", "input_tokens": None, "output_tokens": None,
                        "total_tokens": None, "provider_request_id": None},
        "input_message_refs": [ref], "input_context_hash": byte_hash(canonicalize(context)), "created_at": created})
    validation = freeze("ActionValidation", {"contract_version": version, "validation_id": "synthetic-validation",
        "decision_id": decision["decision_id"], "run_id": run_id, "source_version": 1, "state_sequence": 2,
        "definition_snapshot_hash": snapshot["snapshot_hash"], "decision_kind": "action", "verdict": "allowed",
        "findings": [{"rule_id": "rule.synthetic", "passed": True, "code": "allowed", "detail": "Synthetic allowed case"}],
        "normalized_advice": advice, "execution_request": None, "validated_at": created})
    binding = {key: approval[key] for key in ("approval_id", "proposal_id", "proposal_version", "source_version",
        "proposal_snapshot_hash", "definition_snapshot_hash", "permission_hash", "issued_at", "expires_at")}
    execution = freeze("ExecutionRequest", {"contract_version": version, "execution_id": "synthetic-execution",
        "operation_id": "synthetic-operation", "command_id": "synthetic-command", "run_id": run_id,
        "source_version": 1, "state_sequence_at_validation": 2, "definition_snapshot_hash": snapshot["snapshot_hash"],
        "plan_hash": plan["plan_hash"], "validation_id": validation["validation_id"],
        "validation_hash": validation["validation_hash"], "approval": binding,
        "workspace": {"workspace_id": "synthetic-workspace", "run_id": run_id, "storage_key": "synthetic/project"},
        "action": advice["action"], "required_permission_ids": ["workspace_read", "workspace_write"], "created_at": created})
    validation["execution_request"] = execution
    return {"snapshot": snapshot, "permission": permission, "plan": plan, "proposal": proposal,
            "approval": approval, "message": message, "context": context, "decision": decision,
            "validation": validation, "execution": execution}


def advice_case(kind):
    fixture = build()
    decision, validation = fixture["decision"], fixture["validation"]
    advice = {"kind": kind, "question": None, "plan": None, "action": None, "diagnosis": None,
              "completion_hint": None, "reason": "Synthetic " + kind + " branch"}
    if kind == "clarification":
        advice["question"] = "Which project name?"
    elif kind == "plan":
        advice["plan"] = deepcopy(fixture["plan"])
    elif kind == "action":
        return fixture
    elif kind == "diagnosis":
        advice["diagnosis"] = {"category": "business", "code": "synthetic_diagnosis", "message": "Synthetic diagnosis",
                               "evidence_refs": [], "suggested_remedy": "Ask for a narrower request"}
    elif kind == "completion":
        advice["completion_hint"] = "The model suggests completion; no authority is implied."
    else:
        raise ValueError("Unknown synthetic advice kind")
    decision["advice"] = advice
    freeze("DecisionProposal", decision)
    validation.update(decision_kind=kind, normalized_advice=advice, execution_request=None)
    freeze("ActionValidation", validation)
    fixture["execution"] = None
    return fixture
