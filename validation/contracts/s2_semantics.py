"""Published 1.2/1.3 independent public audit, receipt, and budget semantics.

Only immutable public objects and observed public state are inputs. A valid audit
does not prove historical stages, process isolation, or actual external execution.
Those require separate observations, returned explicitly as coverage limitations.
"""
from collections import Counter
from copy import deepcopy
from datetime import timedelta

from .decision_semantics import (instant, same_fields, schema_hash, verify_approval,
                                 verify_decision, verify_execution, verify_permission,
                                 verify_plan, verify_proposal, verify_validation)
from .oracle import Oracle, byte_hash, canonicalize
from .semantic_checks import object_hash, ref_key, require, sorted_unique, verify_definition_snapshot

KINDS = ("command", "model", "action", "tool", "ci_poll", "reconciliation_read")
STAGES = {"scaffold": "IMPLEMENT", "apply_skill": "IMPLEMENT", "verify": "VERIFY",
          "create_repository": "DELIVER", "push_repository": "DELIVER", "trigger_ci": "DELIVER", "package": "DELIVER"}
PERMISSIONS = {"scaffold": ["workspace_write"], "verify": ["tool_go", "workspace_read", "workspace_write"],
               "create_repository": ["gitlab_create"], "push_repository": ["gitlab_push", "workspace_read"],
               "trigger_ci": ["gitlab_ci"], "package": ["package_write", "workspace_read"]}
ARGV = {"go.mod_tidy": ["mod", "tidy"], "go.test": ["test", "./..."], "go.build": ["build", "./..."]}


def unique(items, key, label):
    result = {}
    for item in items:
        identity = key(item)
        require(identity not in result, label + " duplicate identity")
        result[identity] = item
    return result


def action_scope(execution):
    action = execution["action"]
    return ("apply_skill:" + action["parameters"]["selection"]["skill_id"]
            if action["action_type"] == "apply_skill" else action["action_type"])


def required_permissions(execution, snapshot):
    action = execution["action"]
    if action["action_type"] == "apply_skill":
        skills = {item["skill_id"]: item for item in snapshot["skills"]}
        return sorted(skills[action["parameters"]["selection"]["skill_id"]]["required_permissions"])
    return PERMISSIONS[action["action_type"]]


def verify_action_policy(execution, snapshot, delivery_mode, *, observed_state=None):
    action = execution["action"]["action_type"]
    require(action in snapshot["definition"]["action_protocol"]["allowed_actions"], "action outside frozen protocol")
    require(execution["required_permission_ids"] == required_permissions(execution, snapshot),
            "action permissions differ from exact published scope")
    require(delivery_mode != "local" or action not in ("create_repository", "push_repository", "trigger_ci"),
            "local delivery has remote execution")
    if observed_state is not None:
        require(observed_state == STAGES[action], "action issued in observed illegal phase")


def merged_commands(snapshot, plan):
    commands = {}
    skills = {item["skill_id"]: item for item in snapshot["skills"]}
    declarations = list(snapshot["definition"]["post_commands"])
    for selection in plan["selected_skills"]:
        declarations.extend(skills[selection["skill_id"]]["post_commands"])
    policy = snapshot["definition"]["budget_policy"]
    for item in declarations:
        require(item["arguments"] == ARGV[item["command_id"]], "fixed command argv mismatch")
        old = commands.get(item["command_id"])
        if old is None:
            commands[item["command_id"]] = deepcopy(item)
        else:
            same_fields(item, old, ("executable", "arguments", "cwd_scope", "environment_policy"), "conflicting post-command")
            old["timeout_seconds"] = min(old["timeout_seconds"], item["timeout_seconds"])
            old["max_attempts"] = min(old["max_attempts"], item["max_attempts"])
    for item in commands.values():
        require(item["timeout_seconds"] <= policy["max_tool_seconds"] and item["max_attempts"] <= policy["max_step_attempts"],
                "command limits exceed frozen policy")
    return commands


def fixed_commands(snapshot, plan, execution):
    if execution["action"]["action_type"] == "verify":
        return merged_commands(snapshot, plan)
    require(snapshot["contract_version"] in ("1.3.0", "1.4.0") and execution["action"]["action_type"] == "push_repository",
            "fixed tools used outside published action")
    parameters = execution["action"]["parameters"]
    target = parameters["target"]
    values = {"ref": target["ref"], "operation_marker": "agentstudio:" + execution["operation_id"] + ":" + execution["request_hash"],
              "remote_url": target["instance_url"].rstrip("/") + "/" + target["namespace_path"] + "/" + target["project_path"] + ".git"}
    commands = {}
    for frozen in snapshot["definition"]["repository_policy"]["git_execution"]["commands"]:
        command = deepcopy(frozen)
        command["arguments"] = list(command.pop("arguments_template"))
        for key, value in values.items():
            command["arguments"] = [argument.replace("{" + key + "}", value) for argument in command["arguments"]]
        require(all("{" not in argument for argument in command["arguments"]), "unresolved Git placeholder")
        commands[command["command_id"]] = command
    return commands


def verify_context(context, decision, snapshot, oracle, message_loader):
    schema_hash(oracle, "ContextRecord", context)
    same_fields(context, decision, ("contract_version", "run_id", "decision_id", "source_version", "state_sequence",
                                   "definition_snapshot_hash", "input_message_refs", "input_context_hash"), "context/decision binding")
    require(instant(context["created_at"]) <= instant(decision["created_at"]), "context registered after decision")
    verify_decision(decision, snapshot, oracle, run_id=context["run_id"], source_version=context["source_version"],
                    context=context["messages"], message_loader=message_loader)


def ref_for(name, value, source_version=None):
    table = {"DecisionProposal": ("decision", "decision_id", 1, "decision_hash"),
             "ActionValidation": ("validation", "validation_id", 1, "validation_hash"),
             "ExecutionRequest": ("execution", "execution_id", 1, "request_hash"),
             "Proposal": ("proposal", "proposal_id", value.get("proposal_version"), "snapshot_hash"),
             "Approval": ("approval", "approval_id", 1, "approval_hash"),
             "ContextRecord": ("context", "context_id", 1, "context_hash"),
             "BudgetReservation": ("budget_reservation", "reservation_id", 1, "reservation_hash"),
             "BudgetSettlement": ("budget_settlement", "settlement_id", 1, "settlement_hash"),
             "BudgetLedger": ("budget", "run_id", value.get("ledger_version"), "ledger_hash"),
             "ToolExecutionReceipt": ("evidence", "receipt_id", 1, "receipt_hash"),
             "WorkspaceReceipt": ("workspace", "receipt_id", 1, "receipt_hash"),
             "PackageReceipt": ("evidence", "receipt_id", 1, "receipt_hash"),
             "RemoteExecutionReceipt": ("evidence", "receipt_id", 1, "receipt_hash"),
             "VerificationCandidate": ("verification_candidate", "candidate_id", 1, "candidate_hash"),
             "VerificationReport": ("verification_report", "report_id", value.get("report_version"), "report_hash"),
             "ArtifactManifest": ("manifest", "manifest_id", value.get("manifest_version"), "hash"),
             "MessageEnvelope": ("message", "message_id", 1, None)}
    kind, identity, version, digest = table[name]
    return {"object_type": kind, "object_id": value[identity], "object_version": version,
            "source_version": value.get("source_version", source_version),
            "hash": value[digest] if digest else object_hash(name, value)}


def verify_operation(operation, execution, oracle):
    oracle.validate("OperationRecord", operation)
    same_fields(operation, execution, ("operation_id", "run_id", "source_version", "request_hash"), "operation/request binding")
    require(operation["action_type"] == execution["action"]["action_type"], "operation action mismatch")
    require(operation["target_hash"] == byte_hash(canonicalize(execution["action"]["parameters"])), "operation target hash mismatch")
    require(instant(operation["updated_at"]) >= instant(operation["created_at"]), "operation clock order invalid")


def budget_totals(reservations, settlements):
    totals = {group: {kind: 0 for kind in KINDS} for group in ("reserved", "consumed", "released", "unknown")}
    totals.update(measured_elapsed_ms=0, unknown_elapsed_attempts=0)
    by_id = {item["reservation_id"]: item for item in reservations}
    closed = {item["reservation_id"] for item in settlements}
    for item in reservations:
        if item["reservation_id"] not in closed:
            totals["reserved"][item["kind"]] += item["units"]
    for item in settlements:
        kind = by_id[item["reservation_id"]]["kind"]
        if item["outcome"] == "released":
            totals["released"][kind] += 1
        else:
            totals["consumed"][kind] += item["consumed_units"]
        if item["outcome"] == "unknown":
            totals["unknown"][kind] += item["consumed_units"]
        if item["measurement_source"] == "actual":
            totals["measured_elapsed_ms"] += item["elapsed_ms"]
        else:
            totals["unknown_elapsed_attempts"] += 1
    return totals


def verify_budget(ledger, snapshot, executions, oracle, *, run_id, plans_by_hash=None):
    schema_hash(oracle, "BudgetLedger", ledger)
    require(ledger["run_id"] == run_id and ledger["definition_snapshot_hash"] == snapshot["snapshot_hash"],
            "budget Run/definition mismatch")
    policy = snapshot["definition"]["budget_policy"]
    require(ledger["policy_hash"] == byte_hash(canonicalize(policy)), "budget raised or changed frozen policy")
    requests = unique(executions, lambda item: item["execution_id"], "execution")
    reservations = unique(ledger["reservations"], lambda item: item["reservation_id"], "reservation")
    unique(ledger["reservations"], lambda item: item["attempt_id"], "budget attempt")
    unique(ledger["settlements"], lambda item: item["settlement_id"], "settlement")
    settled = unique(ledger["settlements"], lambda item: item["reservation_id"], "reservation settlement")
    unique(ledger["settlements"], lambda item: item["attempt_id"], "settlement attempt")
    if ledger["execution_started_at"] is not None:
        require(instant(ledger["execution_deadline_at"]) == instant(ledger["execution_started_at"]) + timedelta(seconds=policy["max_execution_seconds"]),
                "execution deadline differs from frozen cumulative budget")
    windows = {}
    all_windows = ledger["ci_windows"] + ledger["reconciliation_windows"]
    unique(all_windows, lambda item: item["window_id"], "budget window")
    for item in all_windows:
        kind = item["kind"]
        prefix = "ci" if kind == "ci_poll" else "reconciliation"
        key = kind, item["operation_id"]
        require(key not in windows, "operation window duplicated or reset")
        windows[key] = item
        require(item["scope_id"] == kind + ":" + item["operation_id"], "window scope identity drift")
        require(0 < (instant(item["deadline_at"]) - instant(item["started_at"])).total_seconds() <= policy["max_" + prefix + ("_wait_seconds" if prefix == "ci" else "_seconds")],
                "window deadline exceeds frozen budget")
        require(item["max_attempts"] <= policy["max_" + prefix + ("_poll_attempts" if prefix == "ci" else "_attempts")],
                "window attempts exceed frozen budget")
    counts = Counter()
    tool_limits = {}
    for item in ledger["reservations"]:
        schema_hash(oracle, "BudgetReservation", item)
        same_fields(item, ledger, ("contract_version", "run_id", "definition_snapshot_hash", "policy_hash"), "reservation/ledger binding")
        require(instant(item["reserved_at"]) <= instant(ledger["updated_at"]), "reservation after ledger snapshot")
        kind = item["kind"]
        if kind == "command":
            expected_scope = item["command_id"]
        elif kind == "model":
            expected_scope = "decision:model"
        else:
            request = requests.get(item["execution_id"])
            require(request is not None, "budget refers to unknown execution")
            same_fields(item, request, ("run_id", "source_version", "execution_id", "operation_id", "request_hash", "command_id"),
                        "reservation/request binding")
            require(instant(item["reserved_at"]) >= instant(request["created_at"]), "budget reservation predates frozen request")
            expected_scope = action_scope(request) if kind == "action" else item["scope_id"]
            if kind in ("action", "tool"):
                require(ledger["execution_started_at"] is not None and
                        instant(item["reserved_at"]) >= instant(ledger["execution_started_at"]),
                        "execution attempt lacks prior cumulative start")
            if kind == "tool":
                command_id = item["scope_id"].removeprefix("tool:")
                plan = (plans_by_hash or {}).get(request["plan_hash"])
                allowed = fixed_commands(snapshot, plan, request) if plan is not None else merged_commands(snapshot, {"selected_skills": []})
                require(item["scope_id"].startswith("tool:") and command_id in allowed, "invalid tool scope or action")
                limit = policy["max_tool_seconds"]
                if plans_by_hash is not None:
                    command = allowed.get(command_id)
                    require(command is not None, "tool command absent from frozen declarations")
                    limit = command["timeout_seconds"]
                    tool_limits[item["scope_id"]] = min(tool_limits.get(item["scope_id"], policy["max_step_attempts"]), command["max_attempts"])
                require((instant(item["deadline_at"]) - instant(item["reserved_at"])).total_seconds() <= limit,
                        "tool deadline exceeds fixed command limit")
            if kind in ("ci_poll", "reconciliation_read"):
                expected_scope = kind + ":" + item["operation_id"]
                window = windows.get((kind, item["operation_id"]))
                require(window is not None, "read reservation lacks stable budget window")
                require(window["source_version"] == item["source_version"] and
                        instant(window["started_at"]) <= instant(item["reserved_at"]) < instant(window["deadline_at"]) and
                        instant(item["deadline_at"]) <= instant(window["deadline_at"]), "read reservation exceeds window")
                if kind == "ci_poll":
                    require(request["action"]["action_type"] == "trigger_ci", "CI poll bound to non-CI action")
                    require(ledger["execution_deadline_at"] is not None and
                            instant(item["reserved_at"]) < instant(ledger["execution_deadline_at"]) and
                            instant(item["deadline_at"]) <= instant(ledger["execution_deadline_at"]), "CI poll exceeds execution deadline")
            elif ledger["execution_deadline_at"] is not None:
                require(instant(item["reserved_at"]) < instant(ledger["execution_deadline_at"]) and
                        instant(item["deadline_at"]) <= instant(ledger["execution_deadline_at"]),
                        "execution attempt exceeds cumulative deadline")
        require(item["scope_id"] == expected_scope, "budget scope identity drift")
        require(kind == "command" or item["deadline_at"] is not None, "timed attempt missing deadline")
        if item["deadline_at"] is not None:
            require(instant(item["deadline_at"]) > instant(item["reserved_at"]), "attempt deadline is not after reservation")
        settlement = settled.get(item["reservation_id"])
        if settlement is None or settlement["outcome"] != "released":
            counts[kind, item["scope_id"]] += 1
    for item in ledger["settlements"]:
        schema_hash(oracle, "BudgetSettlement", item)
        reservation = reservations.get(item["reservation_id"])
        require(reservation is not None, "settlement has no original reservation")
        same_fields(item, reservation, ("contract_version", "reservation_id", "reservation_hash", "attempt_id", "run_id", "source_version"),
                    "settlement/reservation binding")
        require(instant(reservation["reserved_at"]) <= instant(item["settled_at"]) <= instant(ledger["updated_at"]),
                "settlement timestamp outside ledger history")
    require(ledger["totals"] == budget_totals(ledger["reservations"], ledger["settlements"]), "budget totals not derived from full history")
    for kind, limit in (("command", policy["max_commands"]), ("model", policy["max_model_attempts"])):
        require(sum(count for (name, _), count in counts.items() if name == kind) <= limit, "global " + kind + " budget exceeded")
    for (kind, scope), count in counts.items():
        if kind == "action":
            require(count <= policy["max_step_attempts"], "action attempt scope exceeded")
        elif kind == "tool":
            require(count <= tool_limits.get(scope, policy["max_step_attempts"]), "tool scope exceeded frozen maximum")
        elif kind in ("ci_poll", "reconciliation_read"):
            require(count <= windows[kind, scope[len(kind) + 1:]]["max_attempts"], "read attempt window exhausted")
    return {"reservations": len(reservations), "settlements": len(settled), "unsettled": len(reservations) - len(settled)}


class BudgetHistory:
    """Cross-observation checks: immutable attempts, cumulative limits, no reset."""
    def __init__(self):
        self.previous = {}

    def observe(self, ledger):
        old = self.previous.get(ledger["run_id"])
        if old is not None:
            same_fields(ledger, old, ("contract_version", "definition_snapshot_hash", "policy_hash"), "budget history identity")
            require(ledger["ledger_version"] >= old["ledger_version"], "budget version moved backwards")
            if ledger["ledger_version"] == old["ledger_version"]:
                require(ledger == old, "budget same version changed content")
            else:
                for name in ("reservations", "settlements", "ci_windows", "reconciliation_windows"):
                    require(ledger[name][:len(old[name])] == old[name], "budget history removed or rewrote " + name)
                if old["execution_started_at"] is not None:
                    same_fields(ledger, old, ("execution_started_at", "execution_deadline_at"), "cumulative execution window reset")
        self.previous[ledger["run_id"]] = deepcopy(ledger)


def verify_manifest(manifest, proposal, oracle):
    schema_hash(oracle, "ArtifactManifest", manifest)
    same_fields(manifest, proposal, ("contract_version", "run_id", "source_version", "proposal_id", "proposal_version",
                                    "definition_snapshot_hash", "plan_hash"), "manifest/proposal binding")
    require(manifest["proposal_snapshot_hash"] == proposal["snapshot_hash"], "manifest proposal hash mismatch")
    sorted_unique(manifest["files"], lambda item: item["path"], "manifest files")
    unique(manifest["files"], lambda item: item["artifact_id"], "artifact")


def verify_receipt(receipt, name, execution, approval, proposal, snapshot, budget, oracle, manifest_loader):
    schema_hash(oracle, name, receipt)
    same_fields(receipt, execution, ("contract_version", "run_id", "source_version", "definition_snapshot_hash", "execution_id",
                                    "operation_id", "request_hash", "plan_hash", "workspace"), "receipt/execution binding")
    same_fields(receipt, approval, ("approval_id", "approval_hash", "proposal_snapshot_hash", "plan_hash"), "receipt/approval binding")
    require(instant(execution["created_at"]) <= instant(receipt["started_at"]) <= instant(receipt["completed_at"]), "receipt time order invalid")
    reservations = [item for item in budget["reservations"] if item["attempt_id"] == receipt["attempt_id"]]
    require(len(reservations) == 1, "receipt lacks unique budget attempt")
    reservation = reservations[0]
    same_fields(receipt, reservation, ("run_id", "source_version", "execution_id", "operation_id", "request_hash", "attempt_id"), "receipt/reservation binding")
    require(instant(reservation["reserved_at"]) <= instant(receipt["started_at"]), "receipt began before budget reservation")
    manifests = {}
    for field in ("input_manifest_ref", "output_manifest_ref"):
        if receipt[field] is not None:
            manifest = manifest_loader(receipt[field])
            require(ref_for("ArtifactManifest", manifest) == receipt[field], "receipt manifest reference mismatch")
            verify_manifest(manifest, proposal, oracle)
            manifests[field] = manifest
    changed_paths = set()
    if "output_manifest_ref" in manifests:
        before = {item["path"]: (item["sha256"], item["size"]) for item in manifests.get("input_manifest_ref", {}).get("files", [])}
        after = {item["path"]: (item["sha256"], item["size"]) for item in manifests["output_manifest_ref"]["files"]}
        changed_paths = {path for path in set(before) | set(after) if before.get(path) != after.get(path)}
        approved_changes = {item["path"]: item for item in proposal["changes"]}
        for path in changed_paths:
            change = approved_changes.get(path)
            require(change is not None, "receipt output changed unapproved file")
            if change["after"] is None:
                require(path not in after, "receipt did not preserve approved deletion")
            else:
                require(after.get(path) == (change["after_sha256"], len(change["after"].encode("utf-8"))),
                        "receipt output differs from exact approved bytes")
    if name == "ToolExecutionReceipt":
        require(reservation["kind"] == "tool" and reservation["scope_id"] == "tool:" + receipt["command_id"], "tool receipt budget kind/scope mismatch")
        command = fixed_commands(snapshot, proposal["plan"], execution).get(receipt["command_id"])
        require(command is not None, "tool receipt undeclared fixed command")
        same_fields(receipt, command, ("command_id", "arguments", "cwd_scope", "environment_policy"), "tool receipt command binding")
        require(receipt["tool"] in snapshot["tool_bindings"] and receipt["tool"]["tool_id"] == command["executable"], "tool receipt identity not frozen")
        output = receipt["output"].encode("utf-8")
        require(len(output) <= 65536, "tool output exceeds UTF-8 byte bound")
        require(byte_hash(output) == receipt["output_sha256"], "tool output byte hash mismatch")
    else:
        require(reservation["kind"] == "action", "workspace receipt lacks action reservation")
        require(receipt["action_type"] == execution["action"]["action_type"], "workspace receipt action mismatch")
        sorted_unique(receipt["written_paths"], lambda value: value, "workspace written paths")
        require(changed_paths <= set(receipt["written_paths"]), "workspace receipt omitted changed output paths")
        approved = {item["path"] for item in proposal["changes"]}
        require(set(receipt["written_paths"]) <= approved, "workspace wrote outside approved changes")
        if receipt["action_type"] == "apply_skill":
            skill_id = execution["action"]["parameters"]["selection"]["skill_id"]
            skill = next(item for item in snapshot["skills"] if item["skill_id"] == skill_id)
            require(set(receipt["written_paths"]) <= set(skill["write_paths"]), "workspace wrote outside frozen Skill paths")
    settlements = [item for item in budget["settlements"] if item["reservation_id"] == reservation["reservation_id"]]
    if settlements:
        require(settlements[0]["outcome"] == receipt["status"], "receipt status contradicts budget settlement")
        require(instant(settlements[0]["settled_at"]) >= instant(receipt["completed_at"]), "budget settled before receipt completed")
    return {"settled": bool(settlements)}


def audit_objects(bundle):
    """Exact immutable objects enumerable from the published bundle (no aliases)."""
    for name, field in (("Proposal", "proposal_history"), ("Approval", "approval_history"),
                        ("DecisionProposal", "decisions"), ("ActionValidation", "validations"),
                        ("ExecutionRequest", "executions"), ("MessageEnvelope", "messages"),
                        ("ContextRecord", "contexts"), ("ArtifactManifest", "manifest_history"),
                        ("ToolExecutionReceipt", "tool_receipts"), ("WorkspaceReceipt", "workspace_receipts")):
        for item in bundle[field]:
            yield name, ref_for(name, item), item
    for name, field in (("BudgetReservation", "reservations"), ("BudgetSettlement", "settlements")):
        for item in bundle["budget"][field]:
            yield name, ref_for(name, item), item
    if bundle["contract_version"] in ("1.3.0", "1.4.0"):
        for name, field in (("VerificationCandidate", "verification_candidates"), ("RemoteExecutionReceipt", "remote_receipts"),
                            ("PackageReceipt", "package_receipts"), ("VerificationReport", "verification_reports")):
            for item in bundle[field]:
                yield name, ref_for(name, item), item


def verify_audit(bundle, oracle, content_loader, *, observed_stages=None):
    """Check a real or synthetic public bundle without promoting it to RunSnapshot."""
    schema_hash(oracle, "S2AuditBundle", bundle)
    snapshot = bundle["definition_snapshot"]
    verify_definition_snapshot(snapshot, oracle, content_loader, bundle["modes"]["decision_mode"])
    run_id = bundle["run_id"]
    source = bundle["source_version"]
    sequence = bundle["state_sequence"]
    require(bundle["contract_version"] == snapshot["contract_version"] and snapshot["contract_version"] in ("1.2.0", "1.3.0", "1.4.0"), "audit release mismatch")
    objects = {}
    for name, ref, item in audit_objects(bundle):
        key = ref_key(ref)
        require(key not in objects, "duplicate audit object reference")
        objects[key] = item
        require(item["run_id"] == run_id and item["source_version"] <= source, "audit historical object Run/source mismatch")
        oracle.validate(name, item, schema_file="message.schema.json" if name == "MessageEnvelope" else "agent.schema.json")
        if name != "MessageEnvelope":
            schema_hash(oracle, name, item)

    def resolve(ref):
        item = objects.get(ref_key(ref))
        require(item is not None, "audit contains an unresolved exact object reference")
        return item

    permissions = unique(bundle["permission_history"], lambda item: item["permission_hash"], "permission hash")
    unique(bundle["permission_history"], lambda item: (item["actor_id"], item["permission_version"]), "permission version")
    for permission in permissions.values():
        verify_permission(permission, oracle)
    proposals = unique(bundle["proposal_history"], lambda item: (item["proposal_id"], item["proposal_version"]), "proposal")
    approvals = unique(bundle["approval_history"], lambda item: item["approval_id"], "approval")
    decisions = unique(bundle["decisions"], lambda item: item["decision_id"], "decision")
    validations = unique(bundle["validations"], lambda item: item["validation_id"], "validation")
    executions = unique(bundle["executions"], lambda item: item["execution_id"], "execution")
    contexts = unique(bundle["contexts"], lambda item: item["decision_id"], "context decision")
    unique(bundle["contexts"], lambda item: item["context_id"], "context")
    unique(bundle["messages"], lambda item: item["message_id"], "message")
    message_sequences = [item["sequence"] for item in bundle["messages"]]
    require(message_sequences == sorted(set(message_sequences)), "messages not in original unique sequence")
    for message in bundle["messages"]:
        require(message["modes"] == bundle["modes"] and message["state_sequence"] <= sequence, "message authority/modes mismatch")
        if message["role"] == "user":
            require("command_id" in message, "S2 input Envelope missing real command identity")
    for proposal in proposals.values():
        permission = permissions.get(proposal["permission_hash"])
        require(permission is not None, "proposal permission history absent")
        verify_proposal(proposal, permission, snapshot, oracle, run_id=run_id, source_version=proposal["source_version"])
    for approval in approvals.values():
        proposal = proposals.get((approval["proposal_id"], approval["proposal_version"]))
        require(proposal is not None, "approval proposal history absent")
        verify_approval(approval, proposal, permissions[proposal["permission_hash"]], snapshot, oracle,
                        run_id=run_id, source_version=approval["source_version"])
    for decision in decisions.values():
        require(decision["state_sequence"] <= sequence, "audit contains future decision")
        require(decision["decision_id"] in contexts, "decision has no actual context")
        verify_context(contexts[decision["decision_id"]], decision, snapshot, oracle, resolve)
        require(decision["decision_mode"] == bundle["modes"]["decision_mode"] or
                snapshot["contract_version"] == "1.2.0" and decision["decision_mode"] == "rules" and decision["advice"]["kind"] == "action",
                "Run semantic decision mode silently changed")
    require(set(contexts) == set(decisions), "context exists without decision")
    for validation in validations.values():
        decision = decisions.get(validation["decision_id"])
        require(decision is not None, "validation has no source decision")
        require(validation["state_sequence"] <= sequence, "audit contains future validation")
        verify_validation(validation, decision, oracle)
        request = validation["execution_request"]
        if request is not None:
            require(executions.get(request["execution_id"]) == request, "allowed validation execution absent from audit")
    stage_observed = 0
    for execution in executions.values():
        validation = validations.get(execution["validation_id"])
        require(validation is not None, "execution lacks frozen validation")
        approval = approvals.get(execution["approval"]["approval_id"])
        require(approval is not None, "execution lacks immutable approval")
        proposal = proposals[approval["proposal_id"], approval["proposal_version"]]
        verify_execution(execution, validation, decisions[validation["decision_id"]], proposal, approval,
                         permissions[approval["permission_hash"]], snapshot, oracle,
                         run_id=run_id, source_version=execution["source_version"], check_current_lifecycle=False)
        require(instant(execution["created_at"]) >= instant(approval["accepted_at"]), "execution request predates approval")
        observed = (observed_stages or {}).get(execution["state_sequence_at_validation"])
        verify_action_policy(execution, snapshot, bundle["modes"]["delivery_mode"], observed_state=observed)
        stage_observed += observed is not None
        for field in ("manifest_ref",):
            ref = execution["action"]["parameters"].get(field)
            if ref is not None:
                require(ref["object_type"] == "manifest", "action uses wrong manifest reference type")
                verify_manifest(resolve(ref), proposal, oracle)
    operation_map = unique(bundle["operations"], lambda item: item["operation_id"], "operation")
    unique(bundle["operations"], lambda item: item["operation_sequence"], "operation sequence")
    for execution in executions.values():
        operation = operation_map.get(execution["operation_id"])
        require(operation is not None, "execution operation missing")
        verify_operation(operation, execution, oracle)
        resolved_evidence = []
        for ref in operation["evidence_refs"]:
            evidence = resolve(ref)
            resolved_evidence.append((ref["object_type"], evidence))
            if ref["object_type"] in ("evidence", "workspace"):
                same_fields(evidence, operation, ("run_id", "source_version", "operation_id", "request_hash"), "operation evidence binding")
                if operation["status"] == "verified":
                    require(instant(operation["updated_at"]) >= instant(evidence["completed_at"]),
                            "operation verified before referenced receipt completed")
        if operation["status"] == "verified":
            action = execution["action"]["action_type"]
            if action in ("scaffold", "apply_skill"):
                require(any(kind == "workspace" and item["status"] == "succeeded" and item["execution_id"] == execution["execution_id"]
                            for kind, item in resolved_evidence), "verified file action lacks successful workspace receipt")
            elif action == "verify":
                approved = approvals[execution["approval"]["approval_id"]]
                proposal = proposals[approved["proposal_id"], approved["proposal_version"]]
                successful_commands = {item["command_id"] for kind, item in resolved_evidence
                                       if kind == "evidence" and item["status"] == "succeeded"}
                require(set(merged_commands(snapshot, proposal["plan"])) <= successful_commands,
                        "verified tool action lacks required successful fixed-command receipts")
    require(set(operation_map) == {item["operation_id"] for item in executions.values()}, "operation missing execution")
    for manifest in bundle["manifest_history"]:
        proposal = proposals.get((manifest["proposal_id"], manifest["proposal_version"]))
        require(proposal is not None, "manifest proposal absent")
        verify_manifest(manifest, proposal, oracle)
    plans = {item["plan_hash"]: item["plan"] for item in proposals.values()}
    budget = verify_budget(bundle["budget"], snapshot, bundle["executions"], oracle, run_id=run_id, plans_by_hash=plans)
    if bundle["modes"]["decision_mode"] == "rules":
        require(not any(item["kind"] == "model" for item in bundle["budget"]["reservations"]),
                "rules Run recorded a model attempt")
    completed_receipts = 0
    for name, field in (("ToolExecutionReceipt", "tool_receipts"), ("WorkspaceReceipt", "workspace_receipts")):
        unique(bundle[field], lambda item: item["receipt_id"], field)
        unique(bundle[field], lambda item: item["attempt_id"], field + " attempt")
        for receipt in bundle[field]:
            execution = executions.get(receipt["execution_id"])
            require(execution is not None, "receipt execution absent")
            approval = approvals[execution["approval"]["approval_id"]]
            proposal = proposals[approval["proposal_id"], approval["proposal_version"]]
            result = verify_receipt(receipt, name, execution, approval, proposal, snapshot, bundle["budget"], oracle, resolve)
            completed_receipts += result["settled"]
            if name == "ToolExecutionReceipt":
                action_attempts = {item["reservation_id"] for item in bundle["budget"]["reservations"]
                                   if item["kind"] == "action" and item["execution_id"] == execution["execution_id"]}
                succeeded = [item for item in bundle["budget"]["settlements"]
                             if item["reservation_id"] in action_attempts and item["outcome"] == "succeeded"]
                if succeeded:
                    require(any(instant(item["settled_at"]) >= instant(receipt["completed_at"]) for item in succeeded),
                            "successful action budget settled before subordinate tool completed")
    for field, name, digest, history in (("proposal", "Proposal", "snapshot_hash", bundle["proposal_history"]),
                                       ("approval", "Approval", "approval_hash", bundle["approval_history"]),
                                       ("manifest", "ArtifactManifest", "hash", bundle["manifest_history"]),
                                       ("permission_snapshot", "PermissionSnapshot", "permission_hash", bundle["permission_history"])):
        current = bundle[field]
        if current is not None:
            schema_hash(oracle, name, current)
            require(any(item[digest] == current[digest] for item in history), "current " + field + " has no immutable history")
    if bundle["plan"] is not None:
        verify_plan(bundle["plan"], snapshot, oracle)
    if bundle["proposal"] is not None:
        require(bundle["proposal"]["source_version"] == source and bundle["proposal"]["plan"] == bundle["plan"], "current plan/proposal/source mismatch")
    additional = {}
    if snapshot["contract_version"] in ("1.3.0", "1.4.0"):
        from .s2_release13_semantics import verify_release13
        additional = verify_release13(bundle, oracle, resolve, executions, approvals, proposals, operation_map)
    return {"kind": "public_s2_audit_semantics", "decisions": len(decisions), "executions": len(executions), "release13": additional,
            "exact_objects": len(objects), "settled_receipts": completed_receipts, "budget": budget,
            "observed_execution_stages": stage_observed, "unobserved_execution_stages": len(executions) - stage_observed,
            "limitations": ["An archive does not prove current permission lifecycle or that a tool/provider actually ran.",
                            "Unobserved historical stages are not inferred from ContextRecord text.",
                            "Released reservation requires separate no-dispatch evidence; the ledger alone cannot prove it.",
                            "This is not full v2 RunSnapshot, S3 message persistence, or live third-party acceptance."]}
