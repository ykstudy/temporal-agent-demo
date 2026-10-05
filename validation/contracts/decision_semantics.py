"""Independent S2 object-chain checks derived from the published contract.

These functions consume complete public objects. They never read product state or
make an execution decision for the product. Transport, stage-to-action mappings,
and action-specific permission/operation-target mappings need a published S2
contract before a transport runner can claim those checks.
"""
from __future__ import annotations

from datetime import datetime, timezone
from dataclasses import dataclass
from decimal import Decimal
import re
from typing import Callable

from .oracle import Oracle, byte_hash, canonicalize
from .semantic_checks import object_hash, require, sorted_unique, template_key, verify_hash


@dataclass(frozen=True)
class Duration:
    nanoseconds: int
    def total_seconds(self):
        return Decimal(self.nanoseconds) / Decimal(1_000_000_000)


@dataclass(frozen=True, order=True)
class Instant:
    nanoseconds: int
    def __sub__(self, other):
        return Duration(self.nanoseconds - other.nanoseconds)
    def __add__(self, delta):
        micros=(delta.days*86400+delta.seconds)*1_000_000+delta.microseconds
        return Instant(self.nanoseconds + micros*1000)


def instant(value: str):
    # Go publishes RFC3339Nano with 1-9 fractional digits. Python 3.9 only
    # parses 3/6 digits; truncation would silently erase authorization timing.
    match=re.fullmatch(r"(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2})(?:\.(\d{1,9}))?(Z|[+-]\d{2}:\d{2})",value)
    require(match is not None,"invalid precise RFC3339 timestamp")
    base=datetime.fromisoformat(match[1]+("+00:00" if match[3]=="Z" else match[3])).astimezone(timezone.utc)
    elapsed=base-datetime(1970,1,1,tzinfo=timezone.utc)
    seconds=elapsed.days*86400+elapsed.seconds
    return Instant(seconds*1_000_000_000+int((match[2] or "").ljust(9,"0") or "0"))


def schema_hash(oracle: Oracle, name: str, value: dict):
    oracle.validate(name, value)
    verify_hash(name, value)


def same_fields(left: dict, right: dict, fields, label: str):
    for field in fields:
        require(left[field] == right[field], label + " mismatch: " + field)


def verify_plan(plan: dict, snapshot: dict, oracle: Oracle):
    """A normalized plan binds the frozen skills/templates, not provider digests."""
    schema_hash(oracle, "ProjectPlan", plan)
    require(plan["contract_version"] == snapshot["contract_version"], "plan contract version mismatch")
    require(plan["tech_stack"] in snapshot["definition"]["supported_stacks"], "unsupported plan stack")
    sorted_unique(plan["selected_skills"], lambda item: item["skill_id"], "plan skills")
    sorted_unique(plan["template_dependencies"], template_key, "plan templates")
    skills = {item["skill_id"]: item for item in snapshot["skills"]}
    selected_ids = {item["skill_id"] for item in plan["selected_skills"]}
    templates = {template_key(item): item for item in snapshot["template_dependencies"]}
    selected_templates = {template_key(item): item for item in plan["template_dependencies"]}
    for key, item in selected_templates.items():
        require(templates.get(key) == item, "plan template differs from frozen dependency")
    for selection in plan["selected_skills"]:
        skill = skills.get(selection["skill_id"])
        require(skill is not None, "plan selected unknown skill")
        same_fields(selection, skill, ("skill_id", "skill_version", "skill_hash"), "plan skill binding")
        oracle.validate_parameters(skill, selection["parameters"])
        oracle.schema_reference(skill["input_contract"])
        input_file, input_fragment = skill["input_contract"]["schema_ref"].split("#/$defs/", 1)
        oracle.validate(input_fragment, selection["parameters"], schema_file=input_file)
        for dependency in skill["dependencies"]:
            require(dependency["skill_id"] in selected_ids, "selected skill dependency absent")
        require(not selected_ids.intersection(skill["conflicts"]), "selected skills conflict")
        for dependency in skill["template_dependencies"]:
            require(selected_templates.get(template_key(dependency)) == dependency,
                    "selected skill template absent from plan")
    if plan["delivery_target"]["provider"] == "gitlab":
        require(plan["delivery_target"]["project_path"] == plan["project_name"],
                "plan project and GitLab path differ")


def verify_permission(permission: dict, oracle: Oracle):
    schema_hash(oracle, "PermissionSnapshot", permission)
    sorted_unique(permission["allowed_permissions"], lambda value: value, "allowed permissions")
    if permission["delivery_target"]["provider"] == "local":
        require(not any(value.startswith("gitlab_") for value in permission["allowed_permissions"]),
                "local delivery granted remote write permissions")


def verify_proposal(proposal: dict, permission: dict, snapshot: dict, oracle: Oracle,
                    *, run_id: str, source_version: int):
    schema_hash(oracle, "Proposal", proposal)
    verify_permission(permission, oracle)
    verify_plan(proposal["plan"], snapshot, oracle)
    require(proposal["run_id"] == run_id and proposal["source_version"] == source_version,
            "proposal Run/source binding mismatch")
    require(proposal["definition_snapshot_hash"] == snapshot["snapshot_hash"],
            "proposal definition snapshot mismatch")
    require(proposal["plan_hash"] == proposal["plan"]["plan_hash"], "proposal plan hash mismatch")
    require(proposal["permission_hash"] == permission["permission_hash"], "proposal permission mismatch")
    require(proposal["plan"]["delivery_target"] == permission["delivery_target"],
            "proposal permission target mismatch")
    sorted_unique(proposal["required_permission_ids"], lambda value: value, "proposal permissions")
    require(set(proposal["required_permission_ids"]) <= set(permission["allowed_permissions"]),
            "proposal permission scope exceeds grant")
    require(set(proposal["allowed_actions"]) <= set(snapshot["definition"]["action_protocol"]["allowed_actions"]),
            "proposal action scope exceeds frozen definition")
    require(60 <= (instant(proposal["expires_at"]) - instant(proposal["issued_at"])).total_seconds() <= 86400,
            "proposal lifetime outside published limits")
    sorted_unique(proposal["changes"], lambda item: item["path"], "proposal changes")
    for change in proposal["changes"]:
        for side in ("before", "after"):
            text, digest = change[side], change[side + "_sha256"]
            require(text is None and digest is None or
                    text is not None and byte_hash(text.encode("utf-8")) == digest,
                    "proposal file content hash mismatch")
    check_ids = [check["check_id"] for check in proposal["checks"]]
    require(len(check_ids) == len(set(check_ids)), "proposal preflight check identity duplicated")


def verify_approval(approval: dict, proposal: dict, permission: dict, snapshot: dict,
                    oracle: Oracle, *, run_id: str, source_version: int):
    """Check fixed authorization contents, independently of current lifecycle."""
    verify_proposal(proposal, permission, snapshot, oracle, run_id=run_id, source_version=source_version)
    schema_hash(oracle, "Approval", approval)
    same_fields(approval, proposal, ("contract_version", "run_id", "proposal_id", "proposal_version",
                                   "source_version", "definition_snapshot_hash", "plan_hash", "permission_hash",
                                   "allowed_actions", "issued_at", "expires_at"), "approval/proposal binding")
    require(approval["proposal_snapshot_hash"] == proposal["snapshot_hash"], "approval proposal hash mismatch")
    require(approval["delivery_target"] == proposal["plan"]["delivery_target"], "approval target mismatch")
    require(approval["actor_id"] == permission["actor_id"], "approval actor differs from permission actor")
    require(instant(approval["issued_at"]) <= instant(approval["accepted_at"]) < instant(approval["expires_at"]),
            "approval accepted outside issued lifetime")
    require(all(check["passed"] for check in proposal["checks"]), "approval granted with failing preflight")
    if approval["started_at"] is not None:
        require(instant(approval["accepted_at"]) <= instant(approval["started_at"]) < instant(approval["expires_at"]),
                "approval first start outside accepted lifetime")
    if approval["revoked_at"] is not None:
        require(instant(approval["revoked_at"]) >= instant(approval["accepted_at"]),
                "approval revoked before acceptance")


def verify_authorized_at(approval: dict, permission: dict, *, at: str):
    """Inputs MUST be the authoritative lifecycle at that time, not archive values."""
    now = instant(at)
    require(not permission["revoked"], "current permissions revoked")
    require(approval["permission_hash"] == permission["permission_hash"], "current permissions changed")
    require(approval["status"] in ("accepted", "started") and approval["revoked_at"] is None,
            "current approval inactive")
    require(now >= instant(approval["accepted_at"]), "execution precedes approval")
    if approval["started_at"] is None:
        require(approval["status"] == "accepted" and now < instant(approval["expires_at"]),
                "first execution after approval expiry")
    else:
        require(approval["status"] == "started" and
                instant(approval["accepted_at"]) <= instant(approval["started_at"]) < instant(approval["expires_at"]) and
                instant(approval["started_at"]) <= now, "invalid started approval lifecycle")


def verify_decision(decision: dict, snapshot: dict, oracle: Oracle, *, run_id: str,
                    source_version: int, context: list, message_loader: Callable[[dict], dict]):
    schema_hash(oracle, "DecisionProposal", decision)
    require(decision["run_id"] == run_id and decision["source_version"] == source_version,
            "decision Run/source binding mismatch")
    require(decision["definition_snapshot_hash"] == snapshot["snapshot_hash"], "decision definition snapshot mismatch")
    require(decision["contract_version"] == snapshot["contract_version"], "decision contract mismatch")
    require(type(context) is list and bool(context), "actual decision context absent")
    for message in context:
        require(type(message) is dict and set(message) == {"role", "content"} and
                message["role"] in {"system", "user", "assistant", "tool"} and type(message["content"]) is str,
                "context is not the published ordered role/content format")
    require(context[0] == {"role": "system", "content": snapshot["definition"]["system_prompt"]["content"]},
            "actual context does not start with the frozen system prompt")
    require(decision["input_context_hash"] == byte_hash(canonicalize(context)), "actual decision context hash mismatch")
    identities, sequences = set(), []
    for ref in decision["input_message_refs"]:
        require(ref["object_type"] == "message" and ref["object_version"] == 1,
                "decision input does not reference an immutable message")
        require(ref["object_id"] not in identities, "decision input message duplicated")
        identities.add(ref["object_id"])
        message = message_loader(ref)
        oracle.validate("MessageEnvelope", message, schema_file="message.schema.json")
        require(message["run_id"] == run_id and message["message_id"] == ref["object_id"] and
                message["source_version"] == ref["source_version"] and
                object_hash("MessageEnvelope", message) == ref["hash"], "input message reference mismatch")
        require(message["source_version"] <= source_version and
                message["state_sequence"] <= decision["state_sequence"], "decision uses future message")
        sequences.append(message["sequence"])
    require(sequences == sorted(sequences) and len(sequences) == len(set(sequences)),
            "decision inputs violate original message order")
    usage = decision["model_usage"]
    if usage["status"] == "known":
        require(usage["total_tokens"] == usage["input_tokens"] + usage["output_tokens"], "model token total mismatch")
    if decision["decision_mode"] == "rules":
        require(snapshot["provider_binding"] is None or
                snapshot["contract_version"] == "1.2.0" and decision["advice"]["kind"] == "action",
                "rules semantic parsing replaced a frozen model provider")
    else:
        require(snapshot["provider_binding"] is not None, "model decision lacks frozen provider")
    if decision["advice"]["kind"] == "plan":
        verify_plan(decision["advice"]["plan"], snapshot, oracle)


def verify_validation(validation: dict, decision: dict, oracle: Oracle):
    schema_hash(oracle, "ActionValidation", validation)
    same_fields(validation, decision, ("contract_version", "decision_id", "run_id", "source_version",
                                       "definition_snapshot_hash"), "validation/decision binding")
    require(validation["state_sequence"] >= decision["state_sequence"], "validation predates decision state")
    require(instant(validation["validated_at"]) >= instant(decision["created_at"]), "validation predates decision")
    require(validation["decision_kind"] == decision["advice"]["kind"], "validation decision kind mismatch")
    rules = [finding["rule_id"] for finding in validation["findings"]]
    require(len(rules) == len(set(rules)), "validation finding identity duplicated")
    if validation["verdict"] == "allowed":
        require(all(finding["passed"] for finding in validation["findings"]), "allowed validation contains failed finding")
        require(validation["normalized_advice"]["kind"] == decision["advice"]["kind"],
                "normalization changed advice kind")
        if validation["decision_kind"] == "action":
            require(validation["normalized_advice"]["action"]["action_type"] == decision["advice"]["action"]["action_type"],
                    "normalization changed action type")
    else:
        require(any(not finding["passed"] for finding in validation["findings"]), "denial lacks failed finding")


def verify_execution(execution: dict, validation: dict, decision: dict, proposal: dict,
                     approval: dict, permission: dict, snapshot: dict, oracle: Oracle,
                     *, run_id: str, source_version: int, check_current_lifecycle: bool = True):
    """Validate the frozen chain at execution creation; rechecks need current inputs."""
    verify_validation(validation, decision, oracle)
    verify_approval(approval, proposal, permission, snapshot, oracle, run_id=run_id, source_version=source_version)
    schema_hash(oracle, "ExecutionRequest", execution)
    require(validation["verdict"] == "allowed" and validation["decision_kind"] == "action",
            "execution is not an allowed action")
    require(validation["execution_request"] == execution, "validation presents a different execution request")
    same_fields(execution, validation, ("contract_version", "run_id", "source_version", "definition_snapshot_hash",
                                       "validation_id", "validation_hash"), "execution/validation binding")
    require(execution["state_sequence_at_validation"] == validation["state_sequence"], "execution validation state mismatch")
    require(execution["plan_hash"] == proposal["plan_hash"], "execution plan mismatch")
    require(execution["workspace"]["run_id"] == run_id, "execution workspace belongs to another Run")
    require(execution["action"] == validation["normalized_advice"]["action"], "normalized action/execution mismatch")
    require(execution["action"]["action_type"] in approval["allowed_actions"], "execution exceeds approved action scope")
    for key, value in execution["approval"].items():
        require(approval[key] == value, "execution approval binding mismatch: " + key)
    sorted_unique(execution["required_permission_ids"], lambda item: item, "execution permissions")
    require(set(execution["required_permission_ids"]) <= set(proposal["required_permission_ids"]),
            "execution permission scope exceeds proposal")
    if check_current_lifecycle:
        verify_authorized_at(approval, permission, at=execution["created_at"])
    require(instant(execution["created_at"]) >= instant(validation["validated_at"]), "execution predates validation")
    action, parameters = execution["action"]["action_type"], execution["action"]["parameters"]
    if action in ("scaffold", "apply_skill"):
        require(parameters["plan_hash"] == proposal["plan_hash"], "action plan binding mismatch")
    if action == "apply_skill":
        require(parameters["selection"] in proposal["plan"]["selected_skills"], "action skill is not the approved selection")
    if action in ("create_repository", "push_repository", "trigger_ci"):
        require(parameters["target"] == approval["delivery_target"], "action target exceeds approval")


class ImmutableObjects:
    """Detect identity reuse even if every altered object has a self-consistent hash."""
    def __init__(self):
        self.seen = {}

    def observe(self, name: str, value: dict):
        identity_fields = {
            "DecisionProposal": ("decision_id",), "ActionValidation": ("validation_id",),
            "ExecutionRequest": ("execution_id",), "Proposal": ("proposal_id", "proposal_version"),
            "Approval": ("approval_id",), "PermissionSnapshot": ("actor_id", "permission_version"),
        }
        require(name in identity_fields, "unregistered immutable object type")
        key = (name, value.get("run_id"), *(value[field] for field in identity_fields[name]))
        digest = object_hash(name, value)
        require(key not in self.seen or self.seen[key] == digest, "immutable object identity reused with new content")
        self.seen[key] = digest
