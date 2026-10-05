"""Synthetic S2 oracle verification; explicitly not public-runtime acceptance."""
from __future__ import annotations

import argparse
from copy import deepcopy
from datetime import datetime, timezone
import json
from pathlib import Path

from .decision_fixtures import advice_case, build, freeze
from .decision_semantics import (ImmutableObjects, verify_approval, verify_authorized_at,
                                 verify_decision, verify_execution, verify_validation)
from .oracle import ContractError, HERE, Oracle, byte_hash, canonicalize
from .self_test import mutation_cases, replace
from .semantic_checks import object_hash


def check_chain(fixture, oracle):
    decision, snapshot = fixture["decision"], fixture["snapshot"]
    verify_decision(decision, snapshot, oracle, run_id="synthetic-s2-run", source_version=1,
                    context=fixture["context"], message_loader=lambda _ref: fixture["message"])
    verify_validation(fixture["validation"], decision, oracle)
    verify_approval(fixture["approval"], fixture["proposal"], fixture["permission"], snapshot,
                    oracle, run_id="synthetic-s2-run", source_version=1)
    if fixture["execution"] is not None:
        verify_execution(fixture["execution"], fixture["validation"], decision, fixture["proposal"],
                         fixture["approval"], fixture["permission"], snapshot, oracle,
                         run_id="synthetic-s2-run", source_version=1)


def refresh_validation(fixture):
    freeze("ActionValidation", fixture["validation"])
    if fixture["execution"] is not None:
        fixture["execution"]["validation_hash"] = fixture["validation"]["validation_hash"]
        freeze("ExecutionRequest", fixture["execution"])
        fixture["validation"]["execution_request"] = fixture["execution"]


def structural_cases(oracle, value, name):
    for keyword, path, bad in mutation_cases(oracle, value, name):
        # The shared S1 generator changes null to string. Nullable string unions
        # accept that legitimate alternative; S2 nullable unions all exclude arrays.
        original = value
        try:
            for segment in path:
                original = original[segment]
        except (KeyError, IndexError, TypeError):
            original = object()
        if keyword == "type" and original is None:
            bad = replace(value, path, [])
        yield keyword, path, bad


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--release", type=Path, default=HERE / "schemas")
    parser.add_argument("--report", type=Path)
    args = parser.parse_args()
    oracle = Oracle(args.release)
    report = {"evidence_kind": "synthetic_oracle_self_test", "at": datetime.now(timezone.utc).isoformat(),
              "contract_version": oracle.schemas["agent.schema.json"]["$defs"]["DecisionProposal"]["properties"]["contract_version"]["const"], "cases": [],
              "scope": "Published S2 object shapes and independent chain semantics; no Agent/provider/Temporal execution",
              "not_covered": ["S2 public observation transport pending publication", "stage/action permission mappings pending publication",
                              "external unknown-write replay", "live provider or GitLab", "complete v2 RunSnapshot"]}

    def case(name, function, rejected=False, kind="semantic"):
        try:
            function()
            if rejected:
                raise AssertionError("negative accepted: " + name)
            report["cases"].append({"name": name, "kind": kind, "status": "passed"})
        except ContractError:
            if rejected:
                report["cases"].append({"name": name, "kind": kind, "status": "passed"})
            else:
                raise

    for kind in ("clarification", "plan", "action", "diagnosis", "completion"):
        fixture = advice_case(kind)
        case("complete " + kind + " decision chain", lambda f=fixture: check_chain(f, oracle))
        for name, value in (("DecisionProposal", fixture["decision"]), ("ActionValidation", fixture["validation"])):
            for keyword, path, bad in structural_cases(oracle, value, name):
                case(kind + ":" + name + ":" + keyword + ":" + "/".join(map(str, path)),
                     lambda n=name, v=bad: oracle.validate(n, v), True, "schema_negative")
    fixture = build()
    for name, key in (("ProjectPlan", "plan"), ("Proposal", "proposal"), ("Approval", "approval"),
                      ("PermissionSnapshot", "permission"), ("ExecutionRequest", "execution")):
        for keyword, path, bad in structural_cases(oracle, fixture[key], name):
            case(name + ":" + keyword + ":" + "/".join(map(str, path)),
                 lambda n=name, v=bad: oracle.validate(n, v), True, "schema_negative")

    # Recalculate the changed object's hash so these are not simple checksum tests.
    mutations = [
        ("cross Run decision", "decision", "DecisionProposal", ("run_id",), "another-run"),
        ("future source decision", "decision", "DecisionProposal", ("source_version",), 2),
        ("wrong frozen definition", "decision", "DecisionProposal", ("definition_snapshot_hash",), "a" * 64),
        ("wrong actual input hash", "decision", "DecisionProposal", ("input_context_hash",), "a" * 64),
        ("wrong immutable message", "decision", "DecisionProposal", ("input_message_refs", 0, "hash"), "a" * 64),
        ("nonmessage input ref", "decision", "DecisionProposal", ("input_message_refs", 0, "object_type"), "proposal"),
        ("input message version forged", "decision", "DecisionProposal", ("input_message_refs", 0, "object_version"), 2),
        ("validation wrong decision", "validation", "ActionValidation", ("decision_id",), "another-decision"),
        ("validation earlier state", "validation", "ActionValidation", ("state_sequence",), 1),
        ("validation wrong kind", "validation", "ActionValidation", ("decision_kind",), "plan"),
        ("allowed failed finding", "validation", "ActionValidation", ("findings", 0, "passed"), False),
        ("proposal different Run", "proposal", "Proposal", ("run_id",), "another-run"),
        ("proposal different source", "proposal", "Proposal", ("source_version",), 2),
        ("proposal definition mismatch", "proposal", "Proposal", ("definition_snapshot_hash",), "a" * 64),
        ("proposal plan mismatch", "proposal", "Proposal", ("plan_hash",), "a" * 64),
        ("proposal permission mismatch", "proposal", "Proposal", ("permission_hash",), "a" * 64),
        ("proposal corrupt file bytes", "proposal", "Proposal", ("changes", 0, "after"), "changed bytes"),
        ("proposal too short TTL", "proposal", "Proposal", ("expires_at",), "2026-10-02T02:00:59Z"),
        ("proposal too long TTL", "proposal", "Proposal", ("expires_at",), "2026-10-03T02:00:01Z"),
        ("approval fake actor", "approval", "Approval", ("actor_id",), "another-actor"),
        ("approval expired at acceptance", "approval", "Approval", ("accepted_at",), "2026-10-02T02:30:00Z"),
        ("approval client extended TTL", "approval", "Approval", ("expires_at",), "2026-10-02T03:30:00Z"),
        ("approval fake proposal", "approval", "Approval", ("proposal_snapshot_hash",), "a" * 64),
        ("approval expanded actions", "approval", "Approval", ("allowed_actions",), ["scaffold", "create_repository"]),
        ("execution wrong workspace Run", "execution", "ExecutionRequest", ("workspace", "run_id"), "another-run"),
        ("execution wrong plan", "execution", "ExecutionRequest", ("plan_hash",), "a" * 64),
        ("execution wrong validation state", "execution", "ExecutionRequest", ("state_sequence_at_validation",), 99),
        ("execution wrong approval", "execution", "ExecutionRequest", ("approval", "approval_id"), "another-approval"),
        ("execution expanded permission", "execution", "ExecutionRequest", ("required_permission_ids",), ["gitlab_push"]),
        ("first execution at expiry", "execution", "ExecutionRequest", ("created_at",), "2026-10-02T02:30:00Z"),
    ]
    for label, key, name, path, bad in mutations:
        f = build()
        f[key] = replace(f[key], path, bad)
        freeze(name, f[key])
        if key == "validation":
            refresh_validation(f)
        if key == "execution":
            f["validation"]["execution_request"] = f["execution"]
        case(label, lambda v=f: check_chain(v, oracle), True)

    f = build()
    f["context"][0]["content"] += " malicious prompt override"
    f["decision"]["input_context_hash"] = byte_hash(canonicalize(f["context"]))
    freeze("DecisionProposal", f["decision"])
    case("self-consistent input hash still freezes system prompt", lambda: check_chain(f, oracle), True)
    f = build()
    f["decision"]["input_message_refs"] *= 2
    freeze("DecisionProposal", f["decision"])
    case("duplicate input identity", lambda: check_chain(f, oracle), True)
    f = build()
    f["message"]["run_id"] = "another-run"
    f["decision"]["input_message_refs"][0]["hash"] = object_hash("MessageEnvelope", f["message"])
    freeze("DecisionProposal", f["decision"])
    case("self-consistent cross Run message", lambda: check_chain(f, oracle), True)
    f = build()
    f["validation"]["normalized_advice"] = deepcopy(f["validation"]["normalized_advice"])
    f["validation"]["normalized_advice"]["action"]["parameters"]["plan_hash"] = "a" * 64
    refresh_validation(f)
    case("different normalized action and execution", lambda: check_chain(f, oracle), True)
    f["execution"]["action"] = deepcopy(f["validation"]["normalized_advice"]["action"])
    refresh_validation(f)
    case("same normalized action but wrong approved plan", lambda: check_chain(f, oracle), True)

    for verdict in ("rejected", "needs_clarification"):
        f = build()
        f["validation"].update(verdict=verdict, normalized_advice=None, execution_request=None)
        f["validation"]["findings"][0]["passed"] = False
        f["execution"] = None
        refresh_validation(f)
        case(verdict + " preserves decision without execution", lambda v=f: check_chain(v, oracle))
        f["validation"]["findings"][0]["passed"] = True
        refresh_validation(f)
        case(verdict + " cannot hide failing finding", lambda v=f: check_chain(v, oracle), True)

    for status, counts in (("unknown", (None, None, None)), ("known", (0, 0, 0)), ("known", (4, 6, 10))):
        f = build()
        f["snapshot"]["provider_binding"] = {"provider_kind": "openai_compatible", "endpoint": "http://127.0.0.1:1/v1",
                                                  "model_id": "synthetic-model"}
        f["snapshot"]["snapshot_hash"] = object_hash("DefinitionSnapshot", f["snapshot"])
        decision = f["decision"]
        decision["decision_mode"] = "model"
        decision["definition_snapshot_hash"] = f["snapshot"]["snapshot_hash"]
        decision["model_usage"].update(status=status, input_tokens=counts[0], output_tokens=counts[1], total_tokens=counts[2])
        freeze("DecisionProposal", decision)
        def check_model(v=f):
            verify_decision(v["decision"], v["snapshot"], oracle, run_id="synthetic-s2-run", source_version=1,
                            context=v["context"], message_loader=lambda _ref: v["message"])
        case("model usage " + status + " " + str(counts), check_model)
        if status == "known":
            decision["model_usage"]["total_tokens"] += 1
            freeze("DecisionProposal", decision)
            case("known token total inconsistent " + str(counts), check_model, True)

    f = build()
    f["approval"].update(status="started", started_at="2026-10-02T02:00:02Z")
    case("started lifecycle excluded from fixed authorization hash",
         lambda: verify_approval(f["approval"], f["proposal"], f["permission"], f["snapshot"], oracle,
                                  run_id="synthetic-s2-run", source_version=1))
    case("started fixed chain may continue after initial TTL",
         lambda: verify_authorized_at(f["approval"], f["permission"], at="2026-10-02T03:30:00Z"))
    f["approval"]["revoked_at"] = "2026-10-02T02:01:00Z"
    f["approval"]["status"] = "revoked"
    case("revoked lifecycle still invalid despite same approval hash",
         lambda: verify_authorized_at(f["approval"], f["permission"], at="2026-10-02T03:30:00Z"), True)
    f = build()
    f["permission"]["revoked"] = True
    freeze("PermissionSnapshot", f["permission"])
    case("changed current permissions cannot reuse approval",
         lambda: verify_authorized_at(f["approval"], f["permission"], at="2026-10-02T02:00:02Z"), True)

    for name, key, changed_field, changed_value in (
        ("DecisionProposal", "decision", "input_context_hash", "a" * 64),
        ("ActionValidation", "validation", "state_sequence", 3),
        ("ExecutionRequest", "execution", "command_id", "another-command"),
        ("Proposal", "proposal", "title", "another-title"),
        ("Approval", "approval", "actor_id", "another-actor"),
        ("PermissionSnapshot", "permission", "revoked", True),
    ):
        tracker = ImmutableObjects()
        value = build()[key]
        tracker.observe(name, value)
        case("immutable identical replay " + name, lambda n=name, v=value, t=tracker: t.observe(n, v))
        altered = deepcopy(value)
        altered[changed_field] = changed_value
        freeze(name, altered)
        case("self-consistent immutable identity reuse " + name,
             lambda n=name, v=altered, t=tracker: t.observe(n, v), True)

    report["passed"] = len(report["cases"])
    report["schema_negative"] = sum(item["kind"] == "schema_negative" for item in report["cases"])
    report["semantic"] = report["passed"] - report["schema_negative"]
    if args.report:
        args.report.parent.mkdir(parents=True, exist_ok=True)
        args.report.write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n")
    print(json.dumps({key: report[key] for key in ("evidence_kind", "contract_version", "passed", "schema_negative", "semantic", "scope")}, ensure_ascii=False))


if __name__ == "__main__":
    main()
