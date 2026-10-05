"""Independent 1.3 receipt/candidate/report bindings over published objects.

This validates recorded facts. Raw downloads, actual processes and providers
are checked separately by public scenarios; no private product files are inputs.
"""
from copy import deepcopy
import hashlib
from urllib.parse import quote

from .decision_semantics import instant, same_fields, schema_hash
from .oracle import byte_hash, canonicalize
from .semantic_checks import ref_key, require, sorted_unique
from .s2_semantics import fixed_commands, ref_for, unique


def verify_release13(bundle, oracle, resolve, executions, approvals, proposals, operations):
    snapshot = bundle["definition_snapshot"]
    reservations = unique(bundle["budget"]["reservations"], lambda x: x["attempt_id"], "attempt")
    settlements = unique(bundle["budget"]["settlements"], lambda x: x["reservation_id"], "settlement")
    remote = unique(bundle["remote_receipts"], lambda x: x["receipt_id"], "remote receipt")
    raw = {}

    def provenance(q, name, *, completed="completed_at"):
        schema_hash(oracle, name, q)
        x = executions.get(q["execution_id"])
        require(x is not None, "receipt/candidate original execution absent")
        a = approvals[x["approval"]["approval_id"]]
        p = proposals[a["proposal_id"], a["proposal_version"]]
        same_fields(q, x, ("contract_version", "run_id", "source_version", "definition_snapshot_hash", "execution_id", "operation_id", "request_hash", "plan_hash", "workspace"), "receipt/request")
        same_fields(q, a, ("approval_id", "approval_hash", "proposal_snapshot_hash", "plan_hash"), "receipt/approval")
        r = reservations.get(q["attempt_id"])
        require(r is not None and r["kind"] == "action", "receipt original action attempt absent")
        same_fields(r, x, ("run_id", "source_version", "execution_id", "operation_id", "request_hash"), "action/request")
        require(r["scope_id"] == x["action"]["action_type"] and
                instant(x["created_at"]) <= instant(r["reserved_at"]) <= instant(q[completed]), "receipt original attempt scope/time")
        s = settlements.get(r["reservation_id"])
        require(s is not None and s["consumed_units"] == 1, "receipt original action charge missing")
        if name == "RemoteExecutionReceipt":
            require(instant(s["settled_at"]) <= instant(q[completed]), "remote reconciliation receipt predates original action settlement")
        else:
            require(instant(q[completed]) <= instant(s["settled_at"]), "local receipt/candidate created after its action settled")
        if "started_at" in q:
            require(instant(r["reserved_at"]) <= instant(q["started_at"]) <= instant(q[completed]), "receipt started outside its reserved action interval")
            if name == "RemoteExecutionReceipt":
                require(q["started_at"] == r["reserved_at"], "remote receipt start differs from original dispatch reservation")
        if "manifest_ref" in q:
            m = resolve(q["manifest_ref"])
            same_fields(m, p, ("run_id", "source_version", "proposal_id", "proposal_version", "definition_snapshot_hash", "plan_hash"), "receipt manifest/proposal")
            require(m["proposal_snapshot_hash"] == p["snapshot_hash"], "receipt manifest approval hash")
        return x, a, p, r, s

    def observed(q, kind, value, path=None):
        return any(o["kind"] == kind and o["payload"] == value and (path is None or o["path"] == path) for o in q["observations"])

    def raw_ref(ref, size, digest, format_, manifest=None):
        require(ref["hash"] == digest and ref["object_version"] == 1, "raw artifact reference/digest mismatch")
        value = {"ref": ref, "byte_count": size, "sha256": digest, "format": format_}
        if manifest is not None:
            value["manifest"] = manifest
        key = ref_key(ref)
        require(key not in raw or raw[key] == value, "raw artifact identity reused for different bytes")
        raw[key] = value

    for q in remote.values():
        x, a, p, reservation, settlement = provenance(q, "RemoteExecutionReceipt")
        kind = x["action"]["action_type"]
        require(q["action_type"] == kind and q["target"] == x["action"]["parameters"]["target"] == a["delivery_target"], "remote target/action binding")
        seen = set()
        for o in q["observations"]:
            require(o["method"] == "GET" and ref_key(o["reservation_ref"]) not in seen, "read method or duplicate reservation")
            seen.add(ref_key(o["reservation_ref"]))
            r = resolve(o["reservation_ref"])
            same_fields(r, x, ("run_id", "source_version", "execution_id", "operation_id", "request_hash"), "read/original request")
            require(r["kind"] == "reconciliation_read" or kind == "trigger_ci" and r["kind"] == "ci_poll", "remote read wrong budget kind")
            s = settlements.get(r["reservation_id"])
            require(s is not None and s["consumed_units"] == 1 and
                    instant(x["created_at"]) <= instant(r["reserved_at"]) <= instant(o["started_at"]) <= instant(o["completed_at"]) <= instant(s["settled_at"]) <= instant(q["completed_at"]), "read observation charge/timeline mismatch")
            if o["kind"] != "unavailable":
                require(s["outcome"] == "succeeded" and o["http_status"] is not None and 200 <= o["http_status"] < 300 and
                        o["facts_hash"] == byte_hash(canonicalize(o["payload"])) and instant(o["completed_at"]) <= instant(r["deadline_at"]), "unverified read facts/status/hash/deadline")
        for r in reservations.values():
            s = settlements.get(r["reservation_id"])
            if r["execution_id"] == x["execution_id"] and r["kind"] in ("ci_poll", "reconciliation_read") and s and s["outcome"] != "unknown" and instant(s["settled_at"]) <= instant(q["completed_at"]):
                require(ref_key(ref_for("BudgetReservation", r)) in seen, "completed GET omitted from receipt")
        for field, observation in (("project", "project"), ("commit", "commit"), ("pipeline", "pipeline"), ("pipeline_variables", "pipeline_variables")):
            if q[field] is not None:
                require(observed(q, observation, q[field]), "remote receipt claims unobserved " + field)
        if q["status"] != "succeeded":
            continue
        require(q["write_sent"] and settlement["outcome"] in ("succeeded", "unknown"), "successful remote receipt has no original send/consumption")
        target, project = q["target"], q["project"]
        require(project is not None and project["namespace_id"] == target["namespace_id"] and project["namespace_path"] == target["namespace_path"] and
                project["project_path"] == target["project_path"] and project["path_with_namespace"] == target["namespace_path"] + "/" + target["project_path"] and project["visibility"] == target["visibility"], "observed project differs from approved target")
        marker = "agentstudio:" + x["operation_id"] + ":" + x["request_hash"]
        prefix = "/projects/" + str(project["project_id"])
        if kind == "create_repository":
            require(project["ownership_marker"] == marker and project["empty_repository"], "created repository ownership/empty state mismatch")
            require(observed(q, "project", project, prefix) or observed(q, "project", project, "/projects/" + quote(target["namespace_path"] + "/" + target["project_path"], safe="")), "project read path mismatch")
            continue
        prior_kind = "create_repository" if kind == "push_repository" else "push_repository"
        prior = [v for v in remote.values() if v["action_type"] == prior_kind and v["status"] == "succeeded" and
                 all(v[k] == q[k] for k in ("source_version", "plan_hash", "approval_hash", "target", "manifest_ref"))]
        require(len(prior) == 1 and instant(prior[0]["completed_at"]) <= instant(x["created_at"]) and prior[0]["project"]["project_id"] == project["project_id"] and
                prior[0]["project"]["ownership_marker"] == project["ownership_marker"] and not project["empty_repository"] and project["default_branch"] == target["ref"], "remote predecessor/ownership missing or ambiguous")
        commit = q["commit"]
        require(commit is not None and commit["project_id"] == project["project_id"] and observed(q, "commit", commit, prefix + "/repository/commits/" + commit["commit_sha"]), "exact commit GET absent")
        require(observed(q, "branch", {"project_id": project["project_id"], "ref": target["ref"], "commit_sha": commit["commit_sha"]}, prefix + "/repository/branches/" + quote(target["ref"], safe="")), "exact branch/SHA GET absent")
        manifest = resolve(q["manifest_ref"])
        approved = {c["path"]: c["after"] for c in p["changes"] if c["after"] is not None}
        require(set(approved) == {f["path"] for f in manifest["files"]}, "remote manifest file set differs from approved files")
        expected_files = []
        for f in manifest["files"]:
            data = approved[f["path"]].encode()
            blob = hashlib.sha1(b"blob " + str(len(data)).encode() + b"\0" + data).hexdigest()
            require(len(data) == f["size"] and byte_hash(data) == f["sha256"], "remote approved bytes differ from manifest")
            expected_files.append({"path": f["path"], "size": len(data), "sha256": byte_hash(data), "git_blob_sha1": blob})
        require(commit["files"] == expected_files, "remote commit full file set/hash/blob mismatch")
        for f in expected_files:
            require(observed(q, "file", f, prefix + "/repository/files/" + quote(f["path"], safe="") + "?ref=" + commit["commit_sha"]), "exact file at approved SHA GET absent")
        # 1.3 has no tree payload branch. The product records the actual tree
        # GET as unavailable/null and puts verified file/blob facts in commit.
        # This checks the published read, not unexposed GitLab modes/headers.
        tree_path = prefix + "/repository/tree?ref=" + commit["commit_sha"] + "&recursive=true&per_page=100&page=1"
        require(any(o["path"] == tree_path and o["kind"] == "unavailable" and o["http_status"] == 200 for o in q["observations"]), "original remote tree GET absent")
        parameters = x["action"]["parameters"]
        if kind == "push_repository":
            require(parameters["project_id"] == project["project_id"] and parameters["expected_branch_sha"] is None and parameters["manifest_ref"] == q["manifest_ref"] and commit["parent_shas"] == [] and commit["message"] == marker, "push changed initial commit scope")
            commands = fixed_commands(snapshot, p["plan"], x)
            receipts = [resolve(ref) for ref in q["tool_receipt_refs"]]
            require([r["command_id"] for r in receipts] == list(commands) and all(r["status"] == "succeeded" and r["execution_id"] == x["execution_id"] for r in receipts), "push lacks exact complete successful Git tool chain")
        else:
            require(prior[0]["commit"] == commit and parameters["project_id"] == project["project_id"] and parameters["commit_sha"] == commit["commit_sha"] and
                    parameters["ci_config_sha256"] == next(f["sha256"] for f in expected_files if f["path"] == ".gitlab-ci.yml"), "CI code/config differs from approved push")
            pipeline, variables = q["pipeline"], q["pipeline_variables"]
            require(pipeline is not None and pipeline["project_id"] == project["project_id"] and pipeline["ref"] == target["ref"] and pipeline["commit_sha"] == commit["commit_sha"] and pipeline["source"] == "api" and pipeline["status"] == "success", "CI pipeline identity/status mismatch")
            require(variables == {"project_id": project["project_id"], "pipeline_id": pipeline["pipeline_id"], "operation_id": x["operation_id"], "request_hash": x["request_hash"]}, "CI original correlation variables mismatch")
            jobs = unique(q["jobs"], lambda j: j["name"], "CI job")
            require(set(jobs) == {"test", "build"}, "CI required job set missing/extra")
            metadata = deepcopy(q["jobs"])
            for job in metadata:
                job["artifacts"] = []
            require(observed(q, "jobs", {"project_id": project["project_id"], "pipeline_id": pipeline["pipeline_id"], "jobs": metadata}), "CI jobs lack complete paged observation")
            for job in q["jobs"]:
                require(job["project_id"] == project["project_id"] and job["pipeline_id"] == pipeline["pipeline_id"] and job["ref"] == target["ref"] and job["commit_sha"] == commit["commit_sha"] and job["status"] == "success" and job["allow_failure"] is False, "CI job scope/status mismatch")
                expected_path = "coverage.out" if job["name"] == "test" else "bin/server"
                require(len(job["artifacts"]) == 1 and job["artifacts"][0]["path"] == expected_path, "CI artifact required path mismatch")
                artifact = job["artifacts"][0]
                require(artifact["validated"] and artifact["artifact_ref"]["source_version"] == q["source_version"] and observed(q, "artifact", artifact, prefix + "/jobs/" + str(job["job_id"]) + "/artifacts/" + quote(expected_path, safe="")), "CI artifact not observed from exact original job")
                raw_ref(artifact["artifact_ref"], artifact["byte_count"], artifact["sha256"], artifact["format"])

    for q in bundle["package_receipts"]:
        x, _, _, _, s = provenance(q, "PackageReceipt")
        require(x["action"]["action_type"] == "package" and q["manifest_ref"] == x["action"]["parameters"]["manifest_ref"], "package action/manifest scope mismatch")
        if q["status"] == "succeeded":
            require(s["outcome"] == "succeeded" and q["file_count"] == len(resolve(q["manifest_ref"])["files"]) and q["package_ref"]["source_version"] == q["source_version"], "successful package charge/count/source mismatch")
            raw_ref(q["package_ref"], q["package_bytes"], q["package_sha256"], "zip", resolve(q["manifest_ref"]))

    for c in bundle["verification_candidates"]:
        x, _, p, _, s = provenance(c, "VerificationCandidate", completed="created_at")
        require(x["action"]["action_type"] == "verify" and s["outcome"] == "failed", "candidate did not arise from known failed VERIFY")
        before = resolve(c["input_manifest_ref"])
        same_fields(before, p, ("run_id", "source_version", "proposal_id", "proposal_version", "plan_hash"), "candidate input manifest")
        sorted_unique(c["files"], lambda f: f["path"], "candidate files")
        for f in c["files"]:
            require(byte_hash(f["content"].encode()) == f["sha256"] and len(f["content"].encode()) == f["size"], "candidate file bytes/hash/size")
        require(c["total_bytes"] == sum(f["size"] for f in c["files"]), "candidate total bytes")
        old = {f["path"]: (f["size"], f["sha256"]) for f in before["files"]}
        new = {f["path"]: (f["size"], f["sha256"]) for f in c["files"]}
        require(c["changed_paths"] == sorted(path for path in set(old) | set(new) if old.get(path) != new.get(path)), "candidate changed paths")
        require(bool(c["tool_receipt_refs"]), "candidate tool evidence absent")
        for ref in c["tool_receipt_refs"]:
            tool = resolve(ref)
            require(tool["execution_id"] == x["execution_id"] and tool["input_manifest_ref"] == c["input_manifest_ref"] and instant(tool["completed_at"]) <= instant(c["created_at"]), "candidate tool evidence mismatch")

    reports = unique(bundle["verification_reports"], lambda r: (r["report_id"], r["report_version"]), "verification report")
    for report in reports.values():
        schema_hash(oracle, "VerificationReport", report)
        manifest = resolve(report["manifest_ref"])
        same_fields(report, manifest, ("run_id", "source_version", "definition_snapshot_hash", "proposal_snapshot_hash"), "report manifest binding")
        expected = {c["check_id"] for c in snapshot["definition"]["completion_policy"]["required_checks"] if bundle["modes"]["delivery_mode"] in c["delivery_modes"]}
        checks = unique(report["checks"], lambda c: c["check_id"], "required check")
        require(set(checks) == expected, "report fixed required checks differ from frozen policy")
        require((report["outcome"] == "passed") == all(c["status"] == "passed" for c in checks.values()), "report outcome/check statuses disagree")
        for check in checks.values():
            same_fields(check, report, ("run_id", "source_version", "definition_snapshot_hash", "proposal_snapshot_hash"), "check report provenance")
            require(check["status"] != "passed" or bool(check["evidence_refs"]), "passed check has no evidence")
            for ref in check["evidence_refs"]:
                if ref["object_type"] == "ci_artifact":
                    require(ref_key(ref) in raw, "check artifact evidence absent")
                else:
                    evidence = resolve(ref)
                    require(ref["source_version"] == report["source_version"] and evidence["run_id"] == report["run_id"], "check evidence Run/source mismatch")
    current = bundle["verification_report"]
    if current is not None:
        require(reports.get((current["report_id"], current["report_version"])) == current and current["source_version"] == bundle["source_version"], "current verification report absent/stale")
    if bundle["state"] == "DONE":
        require(current is not None and current["outcome"] == "passed", "DONE lacks complete passed report")
        current_ops = [op for op in operations.values() if op["source_version"] == bundle["source_version"]]
        require(bool(current_ops) and all(op["status"] == "verified" for op in current_ops), "DONE contains unverified current operation")
    for x in executions.values():
        if operations[x["operation_id"]]["status"] == "verified" and x["action"]["action_type"] in ("create_repository", "push_repository", "trigger_ci", "package"):
            allowed = bundle["package_receipts"] if x["action"]["action_type"] == "package" else remote.values()
            require(any(q["status"] == "succeeded" and q["execution_id"] == x["execution_id"] and ref_for("PackageReceipt" if x["action"]["action_type"] == "package" else "RemoteExecutionReceipt", q) in operations[x["operation_id"]]["evidence_refs"] for q in allowed), "verified operation lacks original successful complete receipt")
    return {"remote_receipts": len(remote), "packages": len(bundle["package_receipts"]), "candidates": len(bundle["verification_candidates"]), "reports": len(reports), "raw_artifacts": list(raw.values())}
