"""Authenticated loopback controller client; never accepts or controls an arbitrary PID."""
import json
from pathlib import Path
import urllib.parse
import urllib.request

from http_client import normalized_url, require


class OwnedLauncherClient:
    def __init__(self, path, agent_url, evidence):
        state = json.loads(Path(path).read_text())
        require(state.get("owned_process") is True, "launcher does not own the Agent process")
        require(normalized_url(state["base_url"]) == normalized_url(agent_url), "launcher belongs to another Agent URL")
        require(state.get("execution_mode") == "temporal", "crash recovery requires a Temporal launcher")
        self.base = normalized_url(state["control_url"])
        parsed = urllib.parse.urlsplit(self.base)
        require(parsed.scheme == "http" and parsed.hostname == "127.0.0.1", "launcher controller must be loopback HTTP")
        require(state.get("owner_id") and state.get("control_token"), "launcher identity/credential missing")
        output = Path(state["output_dir"]).resolve()
        require(Path(state["binary"]).resolve() == output / "agent-server", "launcher binary is outside its owned output")
        require(Path(state["data_dir"]).resolve().is_relative_to(output), "crash scenario requires launcher-owned data")
        self.owner_id, self._token, self.evidence = state["owner_id"], state["control_token"], evidence
        self.agent_url = normalized_url(agent_url)

    def request(self, path, payload=None):
        method = "GET" if payload is None else "POST"
        raw_payload = None if payload is None else json.dumps(payload).encode()
        request = urllib.request.Request(self.base + path, data=raw_payload,
                                         headers={"Authorization": "Bearer " + self._token,
                                                  "Content-Type": "application/json"})
        # Authorization and launcher state are intentionally absent from public evidence.
        with urllib.request.urlopen(request, timeout=60) as response:
            raw, status, headers = response.read(), response.status, dict(response.headers)
        value = json.loads(raw)
        require("control_token" not in value and self._token.encode() not in raw, "controller leaked private credential")
        self.evidence.exchange(method + " owned-launcher", self.base + path, payload, status, raw, headers)
        require(value.get("owned_process") is True and value.get("owner_id") == self.owner_id,
                "controller ownership changed")
        require(normalized_url(value["base_url"]) == self.agent_url, "controller target changed")
        return value

    def status(self):
        return self.request("/owner/status")

    def crash_restart(self):
        return self.request("/owner/restart", {"crash": True})
