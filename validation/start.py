#!/usr/bin/env python3
"""Independent Agent startup; optional authenticated loopback control of owned Popen only."""
import argparse
from datetime import datetime, timezone
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import json
import os
from pathlib import Path
import secrets
import signal
import socket
import subprocess
import threading
import time
import urllib.request
import uuid

ROOT = Path(__file__).resolve().parent.parent


def parser():
    value = argparse.ArgumentParser(description=__doc__)
    value.add_argument("--execution-mode", choices=("local", "temporal"), default="local")
    value.add_argument("--decision-mode", choices=("rules", "model"), default="rules")
    value.add_argument("--delivery-mode", choices=("local", "gitlab"), default="local")
    value.add_argument("--port", type=int, default=18090)
    value.add_argument("--approval-ttl", default="1m")
    value.add_argument("--output-dir", type=Path, help="new owned evidence directory; existing contents are never overwritten")
    value.add_argument("--data-dir", type=Path, help="new/empty owned Agent data directory")
    value.add_argument("--temporal-address", default="127.0.0.1:17333")
    value.add_argument("--namespace", default="default")
    value.add_argument("--task-queue")
    value.add_argument("--ai-base-url")
    value.add_argument("--ai-model")
    value.add_argument("--gitlab-base-url", help="GitLab origin URL; backend appends /api/v4")
    value.add_argument("--gitlab-namespace-id", default="1")
    value.add_argument("--startup-timeout", type=float, default=45)
    return value


class OwnedAgent:
    def __init__(self, args, output, data, binary, env, tag):
        self.args, self.output, self.data, self.binary, self.env = args, output, data, binary, env
        self.tag, self.token = tag, secrets.token_urlsafe(32)
        self.base = f"http://127.0.0.1:{args.port}"
        self.process = None
        self.generation = 0
        self.health = None
        self.lock = threading.RLock()
        self.stopping = threading.Event()
        self.control_url = None
        self.log = (output / "agent.log").open("a")

    def public_state(self):
        return {"base_url": self.base, "pid": self.process.pid if self.process else None,
                "owned_process": True, "owner_id": self.tag, "generation": self.generation,
                "execution_mode": self.args.execution_mode, "decision_mode": self.args.decision_mode,
                "delivery_mode": self.args.delivery_mode, "data_dir": str(self.data),
                "binary": str(self.binary), "output_dir": str(self.output),
                "approval_ttl": self.args.approval_ttl, "task_queue": self.env["TEMPORAL_TASK_QUEUE"],
                "health": self.health, "control_url": self.control_url}

    def save(self):
        state = {**self.public_state(), "control_token": self.token}
        temporary = self.output / "launcher.json.tmp"
        descriptor = os.open(temporary, os.O_CREAT | os.O_WRONLY | os.O_TRUNC, 0o600)
        with os.fdopen(descriptor, "w") as stream:
            json.dump(state, stream, indent=2)
            stream.write("\n")
        os.chmod(temporary, 0o600)
        temporary.replace(self.output / "launcher.json")

    def event(self, kind, **facts):
        with (self.output / "control-events.jsonl").open("a") as stream:
            stream.write(json.dumps({"at": datetime.now(timezone.utc).isoformat(), "event": kind, "owner_id": self.tag, **facts}) + "\n")

    def start(self):
        with self.lock:
            self.process = subprocess.Popen([str(self.binary)], cwd=ROOT / "backend", env=self.env,
                                            stdout=self.log, stderr=subprocess.STDOUT, start_new_session=True)
            self.generation += 1
            self.health = None
            self.save()
            deadline = time.monotonic() + self.args.startup_timeout
            while time.monotonic() < deadline:
                if self.stopping.is_set():
                    raise RuntimeError("startup interrupted")
                if self.process.poll() is not None:
                    raise RuntimeError(f"owned Agent exited {self.process.returncode}; inspect {self.output / 'agent.log'}")
                try:
                    with urllib.request.urlopen(self.base + "/api/health", timeout=1) as response:
                        health = json.load(response)
                    if self.args.execution_mode != "temporal" or health.get("temporal_connected"):
                        self.health = health
                        self.save()
                        self.event("ready", pid=self.process.pid, generation=self.generation)
                        return self.public_state()
                except (OSError, ValueError):
                    pass
                time.sleep(0.1)
            raise TimeoutError("Agent did not become ready; inspect agent.log")

    def stop_child(self, crash=False):
        with self.lock:
            owned = self.process
            if owned is not None and owned.poll() is None:
                os.killpg(owned.pid, signal.SIGKILL if crash else signal.SIGTERM)
                try:
                    owned.wait(timeout=15)
                except subprocess.TimeoutExpired:
                    os.killpg(owned.pid, signal.SIGKILL)
                    owned.wait(timeout=5)
                self.event("owned_child_stopped", pid=owned.pid, crash=crash)

    def restart(self, crash):
        with self.lock:
            if self.stopping.is_set():
                raise RuntimeError("launcher is stopping")
            self.stop_child(crash)
            return self.start()


def control_handler(owner):
    class Control(BaseHTTPRequestHandler):
        def log_message(self, *_):
            pass

        def reply(self, status, data):
            raw = json.dumps(data).encode()
            self.send_response(status)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(raw)))
            self.end_headers()
            self.wfile.write(raw)

        def authorize(self):
            if not secrets.compare_digest(self.headers.get("Authorization", ""), "Bearer " + owner.token):
                self.reply(401, {"error": "unauthorized"})
                return False
            return True

        def do_GET(self):
            if self.authorize():
                if self.path == "/owner/status":
                    self.reply(200, owner.public_state())
                else:
                    self.reply(404, {"error": "not_found"})

        def do_POST(self):
            if not self.authorize():
                return
            if self.path != "/owner/restart":
                return self.reply(404, {"error": "not_found"})
            try:
                length = int(self.headers.get("Content-Length", "0"))
                if length > 1024:
                    return self.reply(400, {"error": "request_too_large"})
                body = json.loads(self.rfile.read(length))
                if set(body) != {"crash"} or type(body["crash"]) is not bool:
                    return self.reply(400, {"error": "invalid_control_request"})
                self.reply(200, owner.restart(body["crash"]))
            except Exception as error:
                self.reply(500, {"error": str(error)})
    return Control


def main():
    args = parser().parse_args()
    with socket.socket() as probe:
        if probe.connect_ex(("127.0.0.1", args.port)) == 0:
            raise SystemExit(f"Port {args.port} already in use; its owner is untouched")
    tag = datetime.now(timezone.utc).strftime("%Y%m%dT%H%M%SZ") + "-" + uuid.uuid4().hex[:8]
    output = (args.output_dir or ROOT / "validation" / "reports" / ("agent-" + tag)).resolve()
    output.mkdir(parents=True, exist_ok=False)
    data = (args.data_dir or output / "agent-data").resolve()
    if data.exists() and any(data.iterdir()):
        raise SystemExit("--data-dir must be new or empty; only owned validation data is controlled")
    data.mkdir(parents=True, exist_ok=True)
    binary = output / "agent-server"
    with (output / "build.log").open("w") as log:
        result = subprocess.run(["go", "build", "-o", str(binary), "./cmd/server"], cwd=ROOT / "backend", stdout=log, stderr=subprocess.STDOUT)
    if result.returncode:
        raise SystemExit(f"Backend build failed; inspect {output / 'build.log'}")
    env = os.environ.copy()
    for key in list(env):
        if key.startswith(("HARNESS_", "COGNITIVE_")):
            env.pop(key, None)
    env.update(HTTP_ADDR=f"127.0.0.1:{args.port}", AGENT_DATA_DIR=str(data),
               AGENT_EXECUTION_MODE=args.execution_mode, AGENT_MODE=args.execution_mode,
               AGENT_DECISION_MODE=args.decision_mode, AGENT_DELIVERY_MODE=args.delivery_mode,
               AGENT_APPROVAL_TTL=args.approval_ttl, TEMPORAL_ADDRESS=args.temporal_address,
               TEMPORAL_NAMESPACE=args.namespace, TEMPORAL_TASK_QUEUE=args.task_queue or "validation-" + tag)
    if args.decision_mode == "rules":
        for key in ("AI_BASE_URL", "AI_API_KEY", "AI_MODEL"):
            env.pop(key, None)
    else:
        if args.ai_base_url:
            env["AI_BASE_URL"] = args.ai_base_url
        if args.ai_model:
            env["AI_MODEL"] = args.ai_model
    if args.delivery_mode == "local":
        for key in ("GITLAB_TOKEN", "GITLAB_URL", "GITLAB_BASE_URL", "GITLAB_NAMESPACE_ID"):
            env.pop(key, None)
    else:
        if args.gitlab_base_url:
            env["GITLAB_URL"] = args.gitlab_base_url
        env["GITLAB_NAMESPACE_ID"] = args.gitlab_namespace_id
    owner = OwnedAgent(args, output, data, binary, env, tag)
    server = ThreadingHTTPServer(("127.0.0.1", 0), control_handler(owner))
    owner.control_url = f"http://127.0.0.1:{server.server_port}"
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()
    def stop(*_):
        owner.stopping.set()
    signal.signal(signal.SIGINT, stop)
    signal.signal(signal.SIGTERM, stop)
    try:
        print(json.dumps({"ready": True, **owner.start(), "launcher_state": str(output / 'launcher.json')}), flush=True)
        while not owner.stopping.wait(0.25):
            with owner.lock:
                if owner.process.poll() is not None:
                    raise RuntimeError(f"owned child exited {owner.process.returncode}; no automatic restart")
        return 0
    finally:
        owner.stopping.set()
        server.shutdown()
        server.server_close()
        owner.stop_child()
        owner.log.close()


if __name__ == "__main__":
    raise SystemExit(main())
