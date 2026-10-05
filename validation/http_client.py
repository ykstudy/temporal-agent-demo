"""Standard-library public HTTP and durable evidence helpers; no Agent imports."""
from datetime import datetime, timezone
import hashlib
import json
from pathlib import Path
import threading
import urllib.error
import urllib.parse
import urllib.request
import uuid


def utc_now():
    return datetime.now(timezone.utc).isoformat()


class ValidationFailure(AssertionError):
    pass


class NotRun(Exception):
    pass


def require(condition, message):
    if not condition:
        raise ValidationFailure(message)


def normalized_url(value):
    parsed = urllib.parse.urlsplit(value)
    if parsed.scheme not in ("http", "https") or not parsed.netloc or parsed.username or parsed.password or parsed.query or parsed.fragment:
        raise ValueError("base URL must be http(s) with no credentials, query or fragment")
    return value.rstrip("/")


class Evidence:
    def __init__(self, directory, settings):
        self.directory = Path(directory).resolve()
        self.directory.mkdir(parents=True, exist_ok=False)
        self.lock = threading.Lock()
        self.counter = 0
        self.data = {"schema_version": 1, "started_at": utc_now(), "settings": settings,
                     "evidence_dir": str(self.directory), "cases": [], "runs": [],
                     "proof_boundary": "Public HTTP, event and delivered artifact observations only. Fixtures are mock providers; this does not prove live GitLab/Runner/model, hidden filesystem effects or production deployment."}

    def exchange(self, method, url, payload, status, raw, headers):
        with self.lock:
            self.counter += 1
            content_type = headers.get("Content-Type", "").split(";")[0]
            if content_type in ("application/zip", "application/octet-stream"):
                response = {"byte_count": len(raw), "sha256": hashlib.sha256(raw).hexdigest()}
            else:
                try:
                    response = json.loads(raw) if raw else None
                except (ValueError, UnicodeError):
                    response = {"byte_count": len(raw), "text_excerpt": raw[:1000].decode("utf-8", "replace")}
            record = {"sequence": self.counter, "at": utc_now(), "method": method, "url": url,
                      "request": payload, "status": status, "response": response,
                      "content_type": headers.get("Content-Type", "")}
            with (self.directory / "http.jsonl").open("a", encoding="utf-8") as stream:
                stream.write(json.dumps(record, ensure_ascii=False) + "\n")
            return self.counter

    def finish(self, exit_code):
        self.data.update(finished_at=utc_now(), exit_code=exit_code,
                         passed=exit_code == 0, http_exchange_count=self.counter)
        destination = self.directory / "summary.json"
        temporary = destination.with_suffix(".json.tmp")
        temporary.write_text(json.dumps(self.data, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
        temporary.replace(destination)
        return destination


class PublicClient:
    def __init__(self, base_url, evidence, timeout=30):
        self.base = normalized_url(base_url)
        self.evidence = evidence
        self.timeout = timeout

    def raw(self, path, payload=None, expected=(200,), timeout=None):
        encoded = None if payload is None else json.dumps(payload, ensure_ascii=False).encode("utf-8")
        method = "GET" if payload is None else "POST"
        request = urllib.request.Request(self.base + path, data=encoded,
                                         headers={"Content-Type": "application/json", "Accept": "application/json"})
        try:
            with urllib.request.urlopen(request, timeout=timeout or self.timeout) as response:
                status, raw, headers = response.status, response.read(), dict(response.headers)
        except urllib.error.HTTPError as error:
            status, raw, headers = error.code, error.read(), dict(error.headers)
        except (OSError, urllib.error.URLError) as error:
            self.evidence.exchange(method, self.base + path, payload, 0,
                                   json.dumps({"transport_error": str(error)}).encode(), {})
            raise ValidationFailure(f"{path}: HTTP transport failed: {error}") from error
        sequence = self.evidence.exchange(method, self.base + path, payload, status, raw, headers)
        expected = (expected,) if isinstance(expected, int) else expected
        if expected is not None:
            require(status in expected, f"{path}: expected {expected}, got {status}: {raw[:600]!r}; exchange #{sequence}")
        return status, raw, headers

    def json(self, path, payload=None, expected=(200,), timeout=None):
        status, raw, _ = self.raw(path, payload, expected, timeout)
        try:
            body = json.loads(raw) if raw else None
        except (ValueError, UnicodeError) as error:
            raise ValidationFailure(f"{path}: response is not JSON: {error}") from error
        return body

    def snapshot(self, run_id, timeout=5):
        path = f"/api/runs/{urllib.parse.quote(run_id, safe='')}/events"
        with urllib.request.urlopen(self.base + path, timeout=timeout) as response:
            require(response.headers.get_content_type() == "text/event-stream", "event endpoint is not SSE")
            event_id, event_type, data = None, None, []
            for _ in range(100):
                line = response.readline(1 << 20).decode("utf-8").rstrip("\r\n")
                if line.startswith("id:"):
                    event_id = line[3:].strip()
                elif line.startswith("event:"):
                    event_type = line[6:].strip()
                elif line.startswith("data:"):
                    data.append(line[5:].lstrip())
                elif not line and data:
                    raw = "\n".join(data).encode("utf-8")
                    self.evidence.exchange("GET SSE", self.base + path, None, response.status, raw, dict(response.headers))
                    require(event_type == "snapshot", f"expected snapshot event, got {event_type}")
                    require(event_id, "SSE snapshot lacks event identity")
                    return event_id, json.loads(raw)
            raise ValidationFailure("SSE did not supply a bounded snapshot")


def unique_command(prefix):
    return prefix + "_" + uuid.uuid4().hex[:12]
