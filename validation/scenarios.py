#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""Independently exercise public Agent HTTP/events/ZIP and the delivered project."""
import argparse
from concurrent.futures import ThreadPoolExecutor
from datetime import datetime, timezone
import hashlib
import io
import json
import os
from pathlib import Path, PurePosixPath
import re
import shutil
import signal
import socket
import stat
import subprocess
import time
import urllib.parse
import urllib.request
import uuid
import zipfile

from http_client import Evidence, NotRun, PublicClient, ValidationFailure, normalized_url, require, utc_now
from launcher_client import OwnedLauncherClient
from legacy_adapter import check_required as check_legacy_required, manifest_files as legacy_manifest_files

ROOT = Path(__file__).resolve().parent.parent
STATES = {"INTAKE", "CLARIFY", "PLAN", "IMPLEMENT", "VERIFY", "DELIVER", "DONE", "PAUSED", "BLOCKED", "FAILED", "CANCELLED"}


def parse_timestamp(value):
    require(isinstance(value, str) and value, "server timestamp is missing")
    normalized = value.replace("Z", "+00:00")
    normalized = re.sub(r"\.(\d+)(?=[+-]\d\d:\d\d$)", lambda match: "." + (match.group(1) + "000000")[:6], normalized)
    return datetime.fromisoformat(normalized)


class Scenarios:
    def __init__(self, args):
        self.args = args
        self.tag = uuid.uuid4().hex[:10]
        directory = args.report_dir / (datetime.now(timezone.utc).strftime("%Y%m%dT%H%M%SZ") + "-" + self.tag)
        settings = {"base_url": normalized_url(args.base_url), "suite": args.suite,
                    "expected_execution_mode": args.expect_execution_mode,
                    "max_expiry_wait_seconds": args.max_expiry_wait,
                    "gitlab_agent_url": args.gitlab_agent_url, "malformed_agent_url": args.malformed_agent_url,
                    "fixture_url": args.fixture_url}
        settings["owned_crash_requested"] = bool(args.launcher_state)
        self.evidence = Evidence(directory, settings)
        self.client = PublicClient(args.base_url, self.evidence)
        self.done = None
        self.accept_payload = None
        self.expiring = None
        self.expiry_target = None
        self.zip_sha = None
        self.bootstrap = {}
        self.preflight_failed = False
        self.execution_mode = None

    def case(self, name, operation, proof_kind="public-http"):
        start = time.monotonic()
        first_exchange = self.evidence.counter + 1
        record = {"name": name, "proof_kind": proof_kind, "started_at": utc_now()}
        try:
            details = operation() or {}
            record.update(status="passed", **details)
        except NotRun as error:
            record.update(status="not_run", reason=str(error))
        except Exception as error:
            record.update(status="failed", error=f"{type(error).__name__}: {error}")
        record.update(duration_ms=round((time.monotonic() - start) * 1000),
                      first_exchange=first_exchange, last_exchange=self.evidence.counter)
        self.evidence.data["cases"].append(record)
        print(f"{record['status'].upper()} {name}" + (": " + record.get("reason", record.get("error", "")) if record["status"] != "passed" else ""), flush=True)
        self.evidence.finish(1 if record["status"] == "failed" else 2)
        return record

    def contract(self, run):
        require(isinstance(run, dict), "API must return a Run object")
        for key in ("id", "state", "source_version", "state_sequence", "execution_mode", "decision_mode", "delivery_mode", "definition_snapshot"):
            require(key in run, f"new public Run contract missing {key}")
        require(run["state"] in STATES, f"unknown state {run['state']}")
        require(isinstance(run["source_version"], int) and run["source_version"] > 0, "source_version must be positive")
        require(isinstance(run["state_sequence"], int) and run["state_sequence"] >= 0, "state_sequence must be nonnegative")
        require(run["execution_mode"] in ("temporal", "local"), "invalid execution mode")
        require(run["decision_mode"] in ("rules", "model"), "invalid decision mode")
        require(run["delivery_mode"] in ("local", "gitlab"), "invalid delivery mode")
        require(run["definition_snapshot"], "definition snapshot is not bound")
        return run

    def create(self, label, content, client=None, delivery="local", decision="rules"):
        client = client or self.client
        run_id = "run_validation_" + self.tag + "_" + label
        payload = {"id": run_id, "content": content, "decision_mode": decision, "delivery_mode": delivery}
        run = self.contract(client.json("/api/runs", payload, 201))
        self.evidence.data["runs"].append({"id": run_id, "base_url": client.base})
        return run

    def current(self, run, client=None):
        client = client or self.client
        return self.contract(client.json("/api/runs/" + run["id"]))

    def message(self, run, command_id, content):
        return self.contract(self.client.json(f"/api/runs/{run['id']}/messages", {"id": command_id, "content": content}))

    def approval(self, run, command_id):
        proposal = run.get("proposal")
        require(proposal and proposal.get("status") == "pending", "no pending proposal to approve")
        for key in ("id", "proposal_version", "snapshot_hash", "issued_at", "expires_at", "definition_hash"):
            require(proposal.get(key) is not None, f"new public Proposal contract missing {key}")
        require(re.fullmatch(r"[0-9a-f]{64}", proposal["snapshot_hash"]), "snapshot hash must be SHA-256")
        require(parse_timestamp(proposal["expires_at"]) > parse_timestamp(proposal["issued_at"]), "proposal lifetime is invalid")
        return {"id": command_id, "action": "accept", "proposal_id": proposal["id"],
                "proposal_version": proposal["proposal_version"], "source_version": run["source_version"],
                "snapshot_hash": proposal["snapshot_hash"]}

    def action(self, run, payload, client=None, expected=200):
        client = client or self.client
        value = client.json(f"/api/runs/{run['id']}/actions", payload, expected)
        return self.contract(value) if expected == 200 else value

    def wait(self, run, states, client=None):
        deadline = time.monotonic() + self.args.timeout
        current = run
        while time.monotonic() < deadline:
            current = self.current(run, client)
            if current["state"] in states:
                return current
            time.sleep(0.15)
        raise ValidationFailure(f"Run {run['id']} did not reach {sorted(states)}; last {current['state']}: {current.get('error')}")

    def preflight(self):
        health = self.client.json("/api/health", timeout=5)
        self.bootstrap = self.client.json("/api/bootstrap", timeout=5)
        mode = health.get("execution_mode", health.get("mode"))
        self.execution_mode = mode
        require(mode in ("local", "temporal"), "health must identify execution mode")
        if self.args.expect_execution_mode:
            require(mode == self.args.expect_execution_mode, f"expected {self.args.expect_execution_mode}, got {mode}")
        if mode == "temporal":
            require(health.get("temporal_connected") is True, "Temporal mode lacks a real connection")
        require(shutil.which("go"), "Go is required for actual delivered-project build/start")
        decision = self.bootstrap.get("decision_mode", self.bootstrap.get("provider"))
        expected_decision = "model" if self.args.suite == "model-retry" else "rules"
        require(decision == expected_decision, f"{self.args.suite} requires explicit {expected_decision} mode")
        self.evidence.data["service"] = {"health": health, "bootstrap": self.bootstrap}
        return {"execution_mode": mode, "decision_mode": decision, "go_available": True}

    def prepare_expiry(self):
        self.expiring = self.create("expiry", f"创建 expiry-{self.tag}，JSON 日志，仅本地交付")
        self.approval(self.expiring, "expiry-check")
        proposal = self.expiring["proposal"]
        lifetime = (parse_timestamp(proposal["expires_at"]) - parse_timestamp(proposal["issued_at"])).total_seconds()
        if lifetime + 2 > self.args.max_expiry_wait:
            raise NotRun(f"public proposal TTL is {lifetime:.1f}s, above --max-expiry-wait {self.args.max_expiry_wait}s; use an explicitly short-TTL validation Agent")
        self.expiry_target = time.monotonic() + lifetime + 2
        return {"run_id": self.expiring["id"], "lifetime_seconds": lifetime, "issued_at": proposal["issued_at"], "expires_at": proposal["expires_at"]}

    def preview(self, run):
        require(run["state"] == "PLAN" and run.get("proposal", {}).get("status") == "pending", "requirements did not yield an unapproved plan")
        require(not run.get("artifacts") and not run.get("manifest") and not run.get("approval"), "preview exposed applied artifacts/manifest/approval")
        artifacts = self.client.json(f"/api/runs/{run['id']}/artifacts")
        require(not artifacts, "unapproved preview has public artifacts")
        status, raw, _ = self.client.raw(f"/api/runs/{run['id']}/download", expected=(400, 404, 409))
        require(json.loads(raw).get("code") == "artifact_not_ready", "unapproved download was not rejected as not ready")
        return {"run_id": run["id"], "changes": len(run["proposal"].get("changes", [])), "download_status": status,
                "boundary": "No public artifact/ZIP/manifest. Hidden private filesystem effects are not inspected."}

    def delivery(self):
        run = self.create("delivery", "创建 Go 项目，需要 JWT 认证和 JSON 日志，仅本地交付")
        require(run["state"] == "CLARIFY" and not run.get("proposal"), "missing project name must request clarification")
        command = "name_" + self.tag
        content = f"项目名为 validation-{self.tag}"
        run = self.message(run, command, content)
        original_count = len(run["messages"])
        replay = self.message(run, command, content)
        require(len(replay["messages"]) == original_count and replay["source_version"] == run["source_version"], "duplicate message changed authority or conversation")
        conflict = self.client.json(f"/api/runs/{run['id']}/messages", {"id": command, "content": "不同内容"}, 409)
        require(conflict.get("code") == "idempotency_conflict", "same identity/different payload must conflict")
        previous = run
        run = self.message(run, "revise_" + self.tag, "日志级别 debug，JWT token_ttl=12h")
        require(run["plan"]["log_level"] == "debug" and run["source_version"] > previous["source_version"], "multi-turn revision did not update requirement version")
        stale = self.action(previous, self.approval(previous, "stale_" + self.tag), expected=409)
        require(stale.get("code") == "stale_proposal", f"stale confirmation was not explicit: {stale}")
        malformed_hash = self.approval(run, "hash_mismatch_" + self.tag)
        malformed_hash["snapshot_hash"] = "0" * 64
        mismatch = self.action(run, malformed_hash, expected=409)
        require(mismatch.get("code") == "snapshot_mismatch", "incorrect hash was not rejected")
        run = self.current(run)
        preview_evidence = self.preview(run)
        event_id, snapshot = self.client.snapshot(run["id"])
        self.contract(snapshot)
        require(snapshot["id"] == run["id"] and snapshot["source_version"] == run["source_version"] and snapshot["state_sequence"] >= run["state_sequence"], "SSE crossed Run or source/state sequence")
        payload = self.approval(run, "accept_" + self.tag)
        accepted = self.action(run, payload)
        self.action(run, payload)
        self.accept_payload = payload
        done = self.wait(accepted, {"DONE", "FAILED", "BLOCKED"})
        require(done["state"] == "DONE", f"confirmed generation did not complete: {done['state']}: {done.get('error')}")
        require(done["delivery_mode"] == "local" and not done.get("repo_url") and not done.get("pipeline_url"), "local delivery claimed remote success")
        require(done.get("approval") and done.get("manifest"), "completed Run lacks authority approval/manifest")
        self.check_required(done)
        self.done = done
        return {**preview_evidence, "source_version": done["source_version"], "state_sequence": done["state_sequence"],
                "snapshot_hash": payload["snapshot_hash"], "event_id": event_id, "required_checks": done["required_checks"],
                "generation": "actual Agent execution; not a model/provider fixture"}

    def check_required(self, run):
        check_legacy_required(run)

    def manifest_files(self, run):
        return legacy_manifest_files(run)

    def artifact_runtime(self):
        if self.done is None:
            raise NotRun("confirmed generation prerequisite failed")
        done = self.current(self.done)
        files = self.manifest_files(done)
        status, archive, headers = self.client.raw(f"/api/runs/{done['id']}/download")
        require(headers.get("Content-Type", "").split(";")[0] == "application/zip", "download content type is not ZIP")
        destination = self.evidence.directory / "artifacts"
        destination.mkdir()
        (destination / "delivery.zip").write_bytes(archive)
        self.zip_sha = hashlib.sha256(archive).hexdigest()
        project = destination / "project"
        project.mkdir()
        zip_files = {}
        with zipfile.ZipFile(io.BytesIO(archive)) as zipped:
            require(len(zipped.infolist()) < 2000 and sum(info.file_size for info in zipped.infolist()) < 64 << 20, "ZIP exceeds bounded validation limits")
            for info in zipped.infolist():
                if info.is_dir():
                    continue
                name = PurePosixPath(info.filename)
                require(not name.is_absolute() and ".." not in name.parts and "\\" not in info.filename, "unsafe ZIP path")
                require(not stat.S_ISLNK(info.external_attr >> 16), "ZIP symlink is not a permitted delivered file")
                if name.parts and name.parts[0] == done["plan"]["project_name"]:
                    name = PurePosixPath(*name.parts[1:])
                require(name.parts and str(name) not in zip_files, "ZIP has empty or duplicate normalized paths")
                content = zipped.read(info)
                zip_files[str(name)] = content
                path = project.joinpath(*name.parts)
                path.parent.mkdir(parents=True, exist_ok=True)
                path.write_bytes(content)
        for name, digest in files.items():
            require(name in zip_files and hashlib.sha256(zip_files[name]).hexdigest() == digest, f"ZIP/manifest hash mismatch: {name}")
        require(set(zip_files) == set(files) | {"AGENT-MANIFEST.json"}, "ZIP includes files outside public manifest")
        require(json.loads(zip_files["AGENT-MANIFEST.json"])["hash"] == done["manifest"]["hash"], "ZIP manifest and public manifest differ")
        artifact_name = next((p for p in files if p.endswith("/auth.go")), "go.mod")
        artifact = self.client.json(f"/api/runs/{done['id']}/artifacts?" + urllib.parse.urlencode({"path": artifact_name}))
        require(artifact["content"].encode() == zip_files[artifact_name], "artifact endpoint differs from ZIP")
        invalid = self.client.json(f"/api/runs/{done['id']}/artifacts?path=..%2F..%2Fruns%2Foutside.json", expected=400)
        require(invalid.get("code") == "invalid_path", "artifact traversal not rejected")
        main_files = sorted(project.glob("cmd/*/main.go"))
        require(len(main_files) == 1, "delivered project must declare one cmd entry point for runtime validation")
        entry = "./" + str(main_files[0].parent.relative_to(project))
        executable = destination / "generated-server"
        env = {key: os.environ[key] for key in ("PATH", "HOME", "TMPDIR", "GOCACHE", "GOPATH", "GOMODCACHE", "GOPROXY", "GOSUMDB") if key in os.environ}
        env["GOTOOLCHAIN"] = "local"
        runtime_log = self.evidence.directory / "project-runtime.log"
        with runtime_log.open("w") as log:
            built = subprocess.run(["go", "build", "-o", str(executable), entry], cwd=project, env=env, stdout=log, stderr=subprocess.STDOUT, timeout=self.args.timeout)
            require(built.returncode == 0, "delivered project failed actual Go build; inspect project-runtime.log")
            with socket.socket() as probe:
                probe.bind(("127.0.0.1", 0))
                port = probe.getsockname()[1]
            secret = "validation-generated-" + uuid.uuid4().hex
            env.update(PORT=str(port), HTTP_ADDR=f"127.0.0.1:{port}", JWT_SECRET=secret, AUTH_SECRET=secret, SESSION_SECRET=secret)
            process = subprocess.Popen([str(executable)], cwd=project, env=env, stdout=log, stderr=subprocess.STDOUT, start_new_session=True)
            health = None
            url = f"http://127.0.0.1:{port}/healthz"
            try:
                deadline = time.monotonic() + 20
                while time.monotonic() < deadline:
                    require(process.poll() is None, "generated server exited before health; inspect project-runtime.log")
                    try:
                        with urllib.request.urlopen(url, timeout=1) as response:
                            require(response.status == 200, "generated health did not return HTTP 200")
                            health = json.load(response)
                        break
                    except (OSError, ValueError):
                        time.sleep(0.1)
                require(isinstance(health, dict) and (health.get("ok") is True or health.get("status") in ("ok", "healthy")), f"generated health is invalid: {health}")
                (destination / "runtime-health.json").write_text(json.dumps({"url": url, "body": health, "entrypoint": entry, "binary_sha256": hashlib.sha256(executable.read_bytes()).hexdigest()}, indent=2) + "\n")
            finally:
                if process.poll() is None:
                    os.killpg(process.pid, signal.SIGTERM)
                    try:
                        process.wait(timeout=5)
                    except subprocess.TimeoutExpired:
                        os.killpg(process.pid, signal.SIGKILL)
                        process.wait(timeout=5)
        require(secret not in runtime_log.read_text(), "runtime secret appeared in generated log")
        return {"run_id": done["id"], "zip_sha256": self.zip_sha, "manifest_file_count": len(files),
                "zip_file_count": len(zip_files), "entrypoint": entry, "runtime_health": health,
                "actual_project_started": True, "provider_kind": "actual generated project"}

    def lifecycle(self):
        if self.done is None:
            raise NotRun("terminal Run prerequisite failed")
        payload = {"id": "retry_" + self.tag, "action": "retry"}
        successor = self.action(self.done, payload)
        replay = self.action(self.done, payload)
        require(successor["id"] != self.done["id"] and successor["state"] == "PLAN", "retry must create one new unapproved Run")
        require(replay["id"] == successor["id"] and self.current(self.done)["state"] == "DONE", "retry replay duplicated successor or revived terminal parent")
        self.evidence.data["runs"].append({"id": successor["id"], "base_url": self.client.base})
        paused = self.action(successor, {"id": "pause_" + self.tag, "action": "pause"})
        require(paused["state"] == "PAUSED" and not paused.get("approval"), "unapproved pause must be PAUSED")
        resumed = self.action(paused, {"id": "resume_" + self.tag, "action": "resume"})
        require(resumed["state"] == "PLAN", "resume did not preserve pending proposal stage")
        rejected = self.approval(resumed, "reject_" + self.tag)
        rejected["action"] = "reject"
        clarified = self.action(resumed, rejected)
        require(clarified["state"] == "CLARIFY", "reject must allow clarification")
        require(not clarified.get("artifacts") and not clarified.get("manifest"), "lifecycle applied an unapproved proposal")
        cancelled = self.action(clarified, {"id": "cancel_" + self.tag, "action": "cancel"})
        require(cancelled["state"] == "CANCELLED", "cancel must be terminal")
        denied = self.action(cancelled, {"id": "bad_resume_" + self.tag, "action": "resume"}, expected=409)
        require(denied.get("code") in ("terminal_run", "invalid_state"), "terminal resume must be rejected")
        return {"parent_run_id": self.done["id"], "successor_run_id": successor["id"], "final_state": cancelled["state"]}

    def blocked(self):
        if self.bootstrap.get("gitlab_configured"):
            raise NotRun("primary Agent has GitLab credentials; missing-provider test requires isolated credential-free Agent")
        run = self.create("blocked", f"创建 blocked-{self.tag}，JSON 日志，GitLab 交付", delivery="gitlab")
        accepted = self.action(run, self.approval(run, "blocked_accept_" + self.tag))
        result = self.wait(accepted, {"DONE", "FAILED", "BLOCKED"})
        require(result["state"] == "BLOCKED" and not result.get("repo_url") and not result.get("pipeline_url"), "missing GitLab must block, without invented URLs")
        require(result.get("error"), "blocked Run lacks diagnostic reason")
        return {"run_id": result["id"], "state": result["state"], "reason": result["error"], "provider_kind": "unconfigured, not mock success"}

    def expiry(self):
        if self.expiry_target is None:
            raise NotRun("short-TTL expiry prerequisite not available")
        while time.monotonic() < self.expiry_target:
            time.sleep(min(0.5, self.expiry_target - time.monotonic()))
        response = self.action(self.expiring, self.approval(self.expiring, "expired_" + self.tag), expected=409)
        require(response.get("code") == "proposal_expired", f"expired approval not explicit: {response}")
        current = self.current(self.expiring)
        require(not current.get("approval") and not current.get("artifacts") and not current.get("manifest"), "expiry rejection applied proposal")
        if self.done and self.accept_payload:
            repeated = self.action(self.done, self.accept_payload)
            require(repeated["id"] == self.done["id"] and repeated["state"] == "DONE", "registered confirmation replay after expiry failed")
            if self.zip_sha:
                _, archive, _ = self.client.raw(f"/api/runs/{self.done['id']}/download")
                require(hashlib.sha256(archive).hexdigest() == self.zip_sha, "registered replay changed delivered ZIP")
        return {"run_id": current["id"], "code": response["code"], "registered_approval_replay_after_ttl": bool(self.done)}

    def fixture_client(self):
        if not self.args.fixture_url:
            raise NotRun("--fixture-url is required; no live provider is substituted")
        fixture = PublicClient(self.args.fixture_url, self.evidence)
        identity = fixture.json("/fixture/health")
        require(identity.get("provider_kind") == "mock" and identity.get("fixture_id"), "provider does not identify as an explicit fixture")
        return fixture, identity

    def gitlab_fixture(self):
        if not self.args.gitlab_agent_url:
            raise NotRun("--gitlab-agent-url not supplied")
        fixture, identity = self.fixture_client()
        client = PublicClient(self.args.gitlab_agent_url, self.evidence)
        config = client.json("/api/bootstrap")
        require(config.get("gitlab_configured") is True, "fixture Agent is not configured for GitLab")
        configured_url = config.get("gitlab_base_url")
        if not configured_url:
            raise NotRun("public bootstrap lacks gitlab_base_url; cannot verify fixture target before authorizing writes")
        require(normalized_url(configured_url) == normalized_url(self.args.fixture_url), "Agent target is not the explicit GitLab fixture origin")
        name = "fixture-" + self.tag
        run = self.create("gitlab", f"创建 {name}，JSON 日志，GitLab 交付", client, delivery="gitlab")
        before = fixture.json("/fixture/state")
        require(not any(p["path"] == name for p in before["projects"]), "unapproved preview created fixture project")
        payload = self.approval(run, "gitlab_accept_" + self.tag)
        accepted = self.action(run, payload, client)
        self.action(run, payload, client)
        done = self.wait(accepted, {"DONE", "FAILED", "BLOCKED"}, client)
        require(done["state"] == "DONE", f"mock GitLab protocol did not finish: {done.get('error')}")
        self.check_required(done)
        after = fixture.json("/fixture/state")
        projects = [p for p in after["projects"] if p["path"] == name]
        require(len(projects) == 1 and len(projects[0]["commits"]) == 1 and len(projects[0]["pipelines"]) == 1, "confirmation repeated mock project/commit/pipeline")
        pipeline = projects[0]["pipelines"][0]
        require(pipeline["sha"] == projects[0]["commits"][0]["id"] and pipeline["ref"] == "main", "pipeline does not match confirmed commit/ref")
        require(done.get("repo_url") and done.get("pipeline_url"), "confirmed mock result references are absent")
        return {"provider_kind": "mock", "fixture_id": identity["fixture_id"], "run_id": done["id"],
                "project_id": projects[0]["id"], "commit_sha": pipeline["sha"], "pipeline_id": pipeline["id"],
                "fixture_jobs": ["test", "build"], "fixture_artifacts": ["coverage.out", "bin/server"],
                "actual_runner_executed": False, "actual_gitlab_called": False}

    def workflow(self, run):
        result = self.client.json(f"/api/runs/{run['id']}/workflow")
        require(result.get("applicable") is True and result.get("execution_mode") == "temporal", "public workflow endpoint lacks real Temporal identity")
        require(result.get("workflow_id") and result.get("execution_run_id"), "Temporal execution identity is missing")
        require(result.get("query", {}).get("run_id") == run["id"], "Temporal Query crossed business Run")
        return result

    def workflow_settled(self, run, expected_state):
        deadline = time.monotonic() + 15
        result = None
        while time.monotonic() < deadline:
            result = self.workflow(run)
            if result["query"].get("state") == expected_state and result["query"].get("source_version") == result.get("source_version"):
                return result
            time.sleep(0.1)
        raise ValidationFailure(f"Temporal Query did not settle at {expected_state}: {result}")

    def temporal_conversation(self):
        run = self.create("long", f"创建 long-{self.tag}，JSON日志，本地交付")
        initial = self.workflow_settled(run, "PLAN")
        commands = []
        for index in range(21):
            identity = f"long_{self.tag}_{index}"
            content = f"日志级别 {'debug' if index % 2 else 'info'}，补充需求编号 {index}"
            commands.append((identity, content))
            run = self.message(run, identity, content)
        deadline = time.monotonic() + 15
        while time.monotonic() < deadline:
            advanced = self.workflow_settled(run, "PLAN")
            if advanced["execution_run_id"] != initial["execution_run_id"]:
                break
            time.sleep(0.1)
        require(advanced["execution_run_id"] != initial["execution_run_id"], "21 accepted messages did not Continue-As-New")
        require(advanced["workflow_id"] == initial["workflow_id"] and advanced["query"]["run_id"] == run["id"], "Continue-As-New changed business identity")
        require(run.get("context", {}).get("compressed") is True and run["context"].get("summary_count", 0) > 0,
                "long conversation did not publish compression evidence")
        for identity, content in commands:
            require(sum(m["id"] == identity and m["content"] == content for m in run["messages"]) == 1,
                    "compression lost or duplicated authoritative message")
        count, source = len(run["messages"]), run["source_version"]
        replay = self.message(run, *commands[0])
        require(len(replay["messages"]) == count and replay["source_version"] == source, "replay across Continue-As-New duplicated command")
        requests = [(f"concurrent_{self.tag}_{index}", f"日志级别 info，独立并发需求 {index}") for index in range(8)]
        shared = ("concurrent_shared_" + self.tag, "日志级别 debug，并发同一命令")
        requests.extend([shared, shared, shared])
        with ThreadPoolExecutor(max_workers=8) as pool:
            results = list(pool.map(lambda command: self.message(run, *command), requests))
        require(all(value["id"] == run["id"] for value in results), "concurrent response crossed Run")
        final = self.current(run)
        for identity, content in set(requests):
            require(sum(m["id"] == identity and m["content"] == content for m in final["messages"]) == 1,
                    "concurrency lost or duplicated accepted message")
        require(final["source_version"] == source + len(set(requests)), "concurrent commands did not advance authority exactly once")
        settled = self.workflow_settled(final, "PLAN")
        return {"run_id": final["id"], "workflow_id": settled["workflow_id"],
                "initial_execution_run_id": initial["execution_run_id"], "continued_execution_run_id": advanced["execution_run_id"],
                "long_messages": len(commands), "concurrent_requests": len(requests), "unique_concurrent_commands": len(set(requests)),
                "summary_count": final["context"]["summary_count"], "actual_temporal": True}

    def temporal_crash(self):
        launcher = OwnedLauncherClient(self.args.launcher_state, self.client.base, self.evidence)
        before = launcher.status()
        run = self.create("crash", f"创建 crash-{self.tag}，JWT认证和JSON日志，本地交付")
        workflow_before = self.workflow_settled(run, "PLAN")
        payload = self.approval(run, "crash_accept_" + self.tag)
        self.action(run, payload)
        deadline = time.monotonic() + 25
        observed = None
        while time.monotonic() < deadline:
            observed = self.current(run)
            if observed["state"] == "VERIFY" and observed.get("active_step") == "verify":
                break
            require(observed["state"] not in {"DONE", "FAILED", "BLOCKED"}, "VERIFY completed before crash trigger; no crash proof")
            time.sleep(0.01)
        require(observed and observed["state"] == "VERIFY" and observed.get("active_step") == "verify", "did not observe a running VERIFY activity before crash")
        restarted = launcher.crash_restart()
        require(restarted["generation"] == before["generation"] + 1 and restarted["pid"] != before["pid"], "owned process was not replaced")
        replay = self.action(run, payload)
        done = self.wait(replay, {"DONE", "FAILED", "BLOCKED"})
        require(done["state"] == "DONE", f"Temporal did not recover actual VERIFY crash: {done['state']} {done.get('error')}")
        self.check_required(done)
        workflow_after = self.workflow_settled(done, "DONE")
        require(workflow_before["workflow_id"] == workflow_after["workflow_id"] and
                workflow_before["execution_run_id"] == workflow_after["execution_run_id"], "worker restart changed workflow execution identity")
        require(done["approval"]["proposal_id"] == payload["proposal_id"] and
                done["approval"]["snapshot_hash"] == payload["snapshot_hash"], "crash replay replaced registered approval")
        return {"run_id": done["id"], "workflow_id": workflow_after["workflow_id"],
                "execution_run_id": workflow_after["execution_run_id"], "observed_stage_at_crash": observed["state"],
                "owned_launcher_generation_before": before["generation"], "owned_launcher_generation_after": restarted["generation"],
                "actual_process_crash": True, "actual_temporal_activity_recovery": True}

    def malformed_model(self):
        if not self.args.malformed_agent_url:
            raise NotRun("--malformed-agent-url not supplied")
        fixture, identity = self.fixture_client()
        client = PublicClient(self.args.malformed_agent_url, self.evidence)
        config = client.json("/api/bootstrap")
        require(config.get("decision_mode", config.get("provider")) == "model", "malformed-model Agent silently uses rules")
        before = fixture.json("/fixture/state")["model_call_count"]
        run_id = "run_validation_" + self.tag + "_malformed"
        status, raw, _ = client.raw("/api/runs", {"id": run_id, "content": "创建 malformed-service，JSON日志", "decision_mode": "model", "delivery_mode": "local"}, expected=None)
        body = json.loads(raw)
        require(status >= 400 and "model" in body.get("code", ""), f"malformed model did not produce explicit model error: {status} {body}")
        after = fixture.json("/fixture/state")["model_call_count"]
        require(after > before, "mock model was not actually requested")
        status, raw, _ = client.raw("/api/runs/" + run_id, expected=(200, 404))
        if status == 200:
            run = self.contract(json.loads(raw))
            require(run["state"] != "DONE" and not run.get("approval") and not run.get("artifacts") and not run.get("manifest"), "malformed model created completed/applied result")
        return {"provider_kind": "mock", "fixture_id": identity["fixture_id"], "error_code": body["code"],
                "model_calls": after - before, "actual_model_called": False}

    def model_create_retry(self):
        fixture, identity = self.fixture_client()
        before = fixture.json("/fixture/state")
        require(before.get("model_failures_remaining") == 1, "use a fresh --model-fail-first fixture for this targeted case")
        run_id = "run_validation_" + self.tag + "_model_retry"
        payload = {"id": run_id, "content": "创建 fixture-service，JSON 日志，本地交付",
                   "decision_mode": "model", "delivery_mode": "local"}
        status, raw, _ = self.client.raw("/api/runs", payload, expected=None)
        body = json.loads(raw)
        require(status == 503 and body.get("code") == "model_unavailable", "transient model 503 did not produce explicit model_unavailable failure")
        failed_status, failed_raw, _ = self.client.raw("/api/runs/" + run_id, expected=(200, 404))
        if failed_status == 200:
            failed = self.contract(json.loads(failed_raw))
            require(failed["decision_mode"] == "model" and failed["state"] != "DONE" and
                    not failed.get("approval") and not failed.get("artifacts"), "failed model request silently applied a plan")
        first = fixture.json("/fixture/state")
        require(first["model_call_count"] == before["model_call_count"] + 1 and first["model_failures_remaining"] == 0,
                "first provider failure was not actually exercised")
        run = self.contract(self.client.json("/api/runs", payload, expected=(200, 201)))
        require(run["state"] == "PLAN" and run["decision_mode"] == "model" and run.get("proposal"), "same-ID create retry did not recover to model PLAN")
        require(not run.get("approval") and not run.get("artifacts") and not run.get("manifest"), "retry applied unapproved proposal")
        second = fixture.json("/fixture/state")
        require(second["model_call_count"] == first["model_call_count"] + 1, "same business ID retry did not call recovered provider exactly once")
        repeat = self.contract(self.client.json("/api/runs", payload, expected=(200, 201)))
        require(repeat["id"] == run_id and repeat["source_version"] == run["source_version"] and
                len(repeat["messages"]) == len(run["messages"]), "successful create replay duplicated authority or messages")
        conflict_payload = {**payload, "content": "创建 changed-payload-service，JSON 日志"}
        conflict = self.client.json("/api/runs", conflict_payload, expected=409)
        require(conflict.get("code") == "idempotency_conflict", "same Run ID with different initial payload must conflict")
        final = fixture.json("/fixture/state")
        require(final["model_call_count"] == second["model_call_count"], "replay/conflict caused extra provider invocation")
        workflow = self.workflow_settled(run, "PLAN") if self.execution_mode == "temporal" else None
        self.evidence.data["runs"].append({"id": run_id, "base_url": self.client.base})
        return {"provider_kind": "mock", "fixture_id": identity["fixture_id"], "run_id": run_id,
                "first_http_status": status, "first_error_code": body["code"], "state_after_retry": run["state"],
                "business_command_id_stable": True, "model_requests": final["model_call_count"] - before["model_call_count"],
                "workflow": workflow, "actual_temporal": self.execution_mode == "temporal", "actual_model_called": False}

    def run(self):
        preflight = self.case("public API preflight", self.preflight)
        if preflight["status"] != "passed":
            self.preflight_failed = True
            return 2
        if self.args.suite == "model-retry":
            self.case("model create transient 503, same-ID recovery/replay and payload conflict", self.model_create_retry, "mock-provider-and-public-retry")
            statuses = {item["status"] for item in self.evidence.data["cases"]}
            return 1 if "failed" in statuses else 2 if "not_run" in statuses else 0
        self.case("short-TTL expiry setup", self.prepare_expiry)
        self.case("requirements, revision, preview, stale/hash/idempotent approval and actual generation", self.delivery)
        self.case("delivered ZIP hashes, artifact boundary and real project /healthz", self.artifact_runtime, "actual-delivered-project")
        self.case("terminal retry, pause/resume/reject/cancel", self.lifecycle)
        self.case("missing GitLab dependency blocks", self.blocked)
        if self.args.suite == "full":
            self.case("explicit mock GitLab protocol and write idempotence", self.gitlab_fixture, "mock-provider")
            self.case("explicit malformed-model rejection", self.malformed_model, "mock-provider")
        if self.execution_mode == "temporal":
            self.case("real Temporal Continue-As-New, compression and concurrent HTTP commands", self.temporal_conversation, "actual-temporal")
            if self.args.launcher_state:
                self.case("real Temporal running VERIFY crash and owned worker restart", self.temporal_crash, "actual-temporal-and-owned-process")
            else:
                self.evidence.data["crash_coverage"] = "not_selected: --launcher-state required; no arbitrary external process is controlled"
        self.case("expired proposal and registered confirmation replay", self.expiry)
        statuses = {item["status"] for item in self.evidence.data["cases"]}
        return 1 if "failed" in statuses else 2 if "not_run" in statuses else 0


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--base-url", required=True)
    parser.add_argument("--suite", choices=("core", "full", "model-retry"), default="core")
    parser.add_argument("--expect-execution-mode", choices=("local", "temporal"))
    parser.add_argument("--report-dir", type=Path, default=ROOT / "validation" / "reports")
    parser.add_argument("--timeout", type=float, default=180, help="generation/build timeout seconds")
    parser.add_argument("--max-expiry-wait", type=float, default=90)
    parser.add_argument("--gitlab-agent-url")
    parser.add_argument("--malformed-agent-url")
    parser.add_argument("--fixture-url")
    parser.add_argument("--launcher-state", type=Path, help="private state from validation/start.py; select owned Temporal crash recovery")
    args = parser.parse_args()
    try:
        scenarios = Scenarios(args)
    except (OSError, ValueError) as error:
        parser.error(str(error))
    print(f"Evidence: {scenarios.evidence.directory}", flush=True)
    exit_code = 1
    try:
        exit_code = scenarios.run()
    except KeyboardInterrupt:
        scenarios.evidence.data["interrupted"] = True
        exit_code = 2
    except Exception as error:
        scenarios.evidence.data["runner_error"] = f"{type(error).__name__}: {error}"
    finally:
        report = scenarios.evidence.finish(exit_code)
        print(f"Summary: {report}", flush=True)
    return exit_code


if __name__ == "__main__":
    raise SystemExit(main())
