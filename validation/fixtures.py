#!/usr/bin/env python3
"""Explicit mock model/GitLab HTTP provider. No CI runner or real model is invoked."""
import argparse
import base64
from datetime import datetime, timezone
import hashlib
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import json
from pathlib import Path
import threading
import urllib.parse
import uuid


class FixtureState:
    def __init__(self, output, auto_pipeline, model_fail_first=False):
        self.output = output
        self.auto_pipeline = auto_pipeline
        self.identifier = "fixture-" + uuid.uuid4().hex[:12]
        self.lock = threading.RLock()
        self.projects = {}
        self.calls = []
        self.model_calls = []
        self.model_failures_remaining = int(model_fail_first)
        self.next_project = 100
        self.next_pipeline = 1000

    def pipeline(self, project, sha, ref="main"):
        self.next_pipeline += 1
        value = {"id": self.next_pipeline, "sha": sha, "ref": ref,
                 "status": "failed" if "fail-ci" in project["path"] else "success",
                 "web_url": f"http://fixture.invalid/{project['path_with_namespace']}/-/pipelines/{self.next_pipeline}"}
        project["pipelines"].append(value)
        return value

    def record(self, method, path, payload, status, authenticated):
        with self.lock:
            value = {"at": datetime.now(timezone.utc).isoformat(), "method": method, "path": path,
                     "fixture_id": self.identifier, "payload": payload, "status": status, "authenticated": authenticated}
            self.calls.append(value)
            with (self.output / "requests.jsonl").open("a", encoding="utf-8") as stream:
                stream.write(json.dumps(value, ensure_ascii=False) + "\n")
            self.save()

    def snapshot(self):
        return {"provider_kind": "mock", "fixture_id": self.identifier, "auto_pipeline": self.auto_pipeline,
                "model_failures_remaining": self.model_failures_remaining,
                "project_count": len(self.projects), "model_call_count": len(self.model_calls),
                "projects": list(self.projects.values()), "requests": self.calls}

    def save(self):
        target = self.output / "state.json"
        temporary = target.with_suffix(".json.tmp")
        temporary.write_text(json.dumps(self.snapshot(), ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
        temporary.replace(target)


def handler(state):
    class Provider(BaseHTTPRequestHandler):
        def log_message(self, *_):
            pass

        def do_GET(self):
            self.handle_request()

        def do_POST(self):
            self.handle_request()

        def handle_request(self):
            raw_path = urllib.parse.urlsplit(self.path)
            path = raw_path.path
            query = urllib.parse.parse_qs(raw_path.query)
            payload = None
            if self.command == "POST":
                length = int(self.headers.get("Content-Length", "0"))
                if length > 1 << 20:
                    return self.respond(413, {"message": "fixture request too large"}, None)
                try:
                    payload = json.loads(self.rfile.read(length))
                except (ValueError, UnicodeError):
                    return self.respond(400, {"message": "fixture requires JSON"}, None)
            with state.lock:
                if path == "/fixture/health":
                    return self.respond(200, {"provider_kind": "mock", "fixture_id": state.identifier}, payload, False)
                if path == "/fixture/state":
                    return self.respond(200, state.snapshot(), payload, False)
                if path.endswith("/chat/completions") and self.command == "POST":
                    state.model_calls.append({"mode": "malformed" if path.startswith("/malformed/") else "deterministic", "message_count": len(payload.get("messages", []))})
                    if state.model_failures_remaining and not path.startswith("/malformed/"):
                        state.model_failures_remaining -= 1
                        return self.respond(503, {"error": {"message": "explicit mock transient provider failure"}},
                                            {"message_count": len(payload.get("messages", [])), "provider_kind": "mock"})
                    if path.startswith("/malformed/"):
                        content = "{malformed-model-fixture"
                    else:
                        # A deterministic protocol response, explicitly a mock, never an LLM assertion.
                        plan = {"project_name": "fixture-service", "description": "explicit deterministic mock provider",
                                "tech_stack": "golang", "skills": ["add_logging"], "database": "postgres", "orm": "gorm",
                                "auth": "jwt", "token_ttl": "24h", "log_level": "info", "log_format": "json",
                                "log_output": "console", "log_path": "logs/app.log", "log_max_size": 10485760,
                                "log_max_backups": 3, "log_max_age": "168h", "log_on_write_error": "fail_closed",
                                "repo_provider": "local", "repo_visibility": "private"}
                        content = json.dumps({"plan": plan, "explanation": "mock response; no model call"})
                    return self.respond(200, {"id": "mock-completion", "model": "validation-mock", "choices": [{"message": {"role": "assistant", "content": content}, "finish_reason": "stop"}]}, {"message_count": len(payload.get("messages", [])), "provider_kind": "mock"})
                if path == "/api/v4/user":
                    return self.respond(200, {"id": 1, "username": "validation"}, payload)
                if path in ("/api/v4/namespaces/1", "/api/v4/groups/1"):
                    return self.respond(200, {"id": 1, "full_path": "validation", "path": "validation"}, payload)
                if path == "/api/v4/projects" and self.command == "POST":
                    project_name = payload.get("path") or payload.get("name")
                    target = "validation/" + project_name
                    if target in state.projects:
                        return self.respond(400, {"message": "project exists"}, payload)
                    state.next_project += 1
                    project = {"id": state.next_project, "name": project_name, "path": project_name,
                               "path_with_namespace": target, "description": payload.get("description", ""),
                               "default_branch": "main", "namespace": {"id": 1, "full_path": "validation"},
                               "visibility": "private",
                               "web_url": "http://fixture.invalid/" + target,
                               "http_url_to_repo": "http://fixture.invalid/" + target + ".git",
                               "files": {}, "commits": [], "pipelines": []}
                    state.projects[target] = project
                    return self.respond(201, project, payload)
                if path == "/api/v4/projects" and self.command == "GET":
                    needle = query.get("search", [""])[0]
                    return self.respond(200, [p for p in state.projects.values() if needle in p["path"]], payload)
                prefix = "/api/v4/projects/"
                if path.startswith(prefix):
                    rest = path[len(prefix):]
                    raw_identifier, _, suffix = rest.partition("/")
                    identifier = urllib.parse.unquote(raw_identifier)
                    project = next((p for p in state.projects.values() if str(p["id"]) == identifier or p["path_with_namespace"] == identifier), None)
                    if project is None:
                        return self.respond(404, {"message": "project not found"}, payload)
                    if not suffix:
                        return self.respond(200, project, payload)
                    if suffix.startswith("repository/files/"):
                        file_path = urllib.parse.unquote(suffix[len("repository/files/"):])
                        if file_path not in project["files"]:
                            return self.respond(404, {"message": "file not found"}, payload)
                        content = project["files"][file_path]
                        return self.respond(200, {"file_path": file_path, "content": base64.b64encode(content.encode()).decode(), "encoding": "base64",
                                                  "last_commit_id": project["commits"][0]["id"]}, payload)
                    if suffix == "repository/commits" and self.command == "GET":
                        return self.respond(200, project["commits"], payload)
                    if suffix == "repository/commits" and self.command == "POST":
                        for action in payload.get("actions", []):
                            content = action.get("content", "")
                            if action.get("encoding") == "base64":
                                content = base64.b64decode(content).decode()
                            project["files"][action["file_path"]] = content
                        sha = hashlib.sha1(json.dumps({"files": project["files"], "message": payload.get("commit_message")}, sort_keys=True).encode()).hexdigest()
                        commit = {"id": sha, "message": payload.get("commit_message", ""), "web_url": project["web_url"] + "/-/commit/" + sha}
                        project["commits"].insert(0, commit)
                        if state.auto_pipeline:
                            state.pipeline(project, sha, payload.get("branch", "main"))
                        return self.respond(201, commit, payload)
                    if suffix.startswith("repository/branches/"):
                        if not project["commits"]:
                            return self.respond(404, {"message": "branch not found"}, payload)
                        return self.respond(200, {"name": "main", "commit": project["commits"][0]}, payload)
                    if suffix == "repository/tree":
                        return self.respond(200, [{"path": name, "type": "blob", "name": name.rsplit("/", 1)[-1]} for name in sorted(project["files"])], payload)
                    if suffix.startswith("jobs/"):
                        segments = suffix.split("/")
                        if len(segments) >= 4 and segments[2] == "artifacts":
                            # These are fixed protocol fixtures. They do not assert Runner execution.
                            if segments[1] == "1" and "/".join(segments[3:]) == "coverage.out":
                                return self.respond_bytes(200, b"mode: set\nfixture/main.go:1.1,2.1 1 1\n", payload)
                            if segments[1] == "2" and "/".join(segments[3:]) == "bin/server":
                                return self.respond_bytes(200, b"\x7fELFexplicit-mock-build-artifact-not-runnable", payload)
                    if suffix == "pipelines":
                        sha, ref = query.get("sha", [None])[0], query.get("ref", [None])[0]
                        return self.respond(200, [p for p in project["pipelines"] if (sha is None or p["sha"] == sha) and (ref is None or p["ref"] == ref)], payload)
                    if suffix == "pipeline" and self.command == "POST":
                        if not project["commits"]:
                            return self.respond(400, {"message": "no commit"}, payload)
                        return self.respond(201, state.pipeline(project, project["commits"][0]["id"], payload.get("ref", "main")), payload)
                    if suffix.startswith("pipelines/"):
                        segments = suffix.split("/")
                        pipeline = next((p for p in project["pipelines"] if str(p["id"]) == segments[1]), None)
                        if pipeline is None:
                            return self.respond(404, {"message": "pipeline not found"}, payload)
                        if len(segments) == 3 and segments[2] == "jobs":
                            return self.respond(200, [{"id": i, "name": name, "status": pipeline["status"], "stage": name,
                                                      "artifacts_file": {"filename": "artifacts.zip"}, "commit": {"id": pipeline["sha"]}}
                                                     for i, name in enumerate(("test", "build"), 1)], payload)
                        return self.respond(200, pipeline, payload)
                return self.respond(404, {"message": "fixture endpoint not implemented", "path": path}, payload)

        def respond(self, status, body, payload, record=True):
            if record:
                authenticated = bool(self.headers.get("PRIVATE-TOKEN") or self.headers.get("Authorization"))
                state.record(self.command, self.path, payload, status, authenticated)
            raw = json.dumps(body, ensure_ascii=False).encode("utf-8")
            self.send_response(status)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(raw)))
            self.end_headers()
            self.wfile.write(raw)

        def respond_bytes(self, status, raw, payload):
            state.record(self.command, self.path, payload, status,
                         bool(self.headers.get("PRIVATE-TOKEN") or self.headers.get("Authorization")))
            self.send_response(status)
            self.send_header("Content-Type", "application/octet-stream")
            self.send_header("Content-Length", str(len(raw)))
            self.end_headers()
            self.wfile.write(raw)
    return Provider


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--port", type=int, default=18101)
    parser.add_argument("--output-dir", type=Path, help="new fixture evidence directory")
    parser.add_argument("--manual-pipeline", action="store_true", help="do not simulate commit-created pipeline")
    parser.add_argument("--model-fail-first", action="store_true", help="first normal mock model request returns HTTP 503")
    args = parser.parse_args()
    output = (args.output_dir or Path(__file__).resolve().parent / "reports" /
              ("fixture-" + datetime.now(timezone.utc).strftime("%Y%m%dT%H%M%SZ") + "-" + uuid.uuid4().hex[:8])).resolve()
    output.mkdir(parents=True, exist_ok=False)
    state = FixtureState(output, not args.manual_pipeline, args.model_fail_first)
    state.save()
    server = ThreadingHTTPServer(("127.0.0.1", args.port), handler(state))
    print(json.dumps({"ready": True, "provider_kind": "mock", "fixture_id": state.identifier, "base_url": f"http://127.0.0.1:{args.port}", "output_dir": str(state.output)}), flush=True)
    try:
        server.serve_forever()
    except KeyboardInterrupt:
        pass
    finally:
        server.server_close()
        state.save()


if __name__ == "__main__":
    main()
