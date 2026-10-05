"""Explicit S1 mock provider: schema-valid clarification and public prompt evidence.

This is not a live model or an Agent implementation. It never records auth headers.
"""
import argparse
from datetime import datetime, timezone
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import json
from pathlib import Path
import threading
from urllib.parse import urlsplit

from .oracle import Oracle, byte_hash, strict_loads


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--port", type=int, required=True)
    parser.add_argument("--output", type=Path, required=True)
    parser.add_argument("--fail-first", action="store_true")
    args = parser.parse_args()
    oracle = Oracle()
    advice = {"kind": "clarification", "question": "请提供项目名称和需要的能力。", "plan": None,
              "action": None, "diagnosis": None, "completion_hint": None,
              "reason": "Independent S1 fixture for frozen system-prompt request evidence."}
    oracle.validate("DecisionAdvice", advice)
    state = {"provider_kind": "mock", "scope": "S1 prompt binding, never live model evidence",
             "contract_version": "1.1.0", "advice": advice, "requests": []}
    args.output.mkdir(parents=True, exist_ok=True)
    lock = threading.Lock()

    class Handler(BaseHTTPRequestHandler):
        def log_message(self, *_args):
            pass

        def send(self, status, value):
            raw = (json.dumps(value, ensure_ascii=False) + "\n").encode()
            self.send_response(status)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(raw)))
            self.end_headers()
            self.wfile.write(raw)

        def do_GET(self):
            path = urlsplit(self.path).path
            if path == "/fixture/health":
                self.send(200, {"provider_kind": "mock", "ready": True})
            elif path == "/fixture/state":
                with lock:
                    self.send(200, state)
            else:
                self.send(404, {"error": "not_found"})

        def do_POST(self):
            if urlsplit(self.path).path != "/v1/chat/completions":
                return self.send(404, {"error": "not_found"})
            try:
                length = int(self.headers.get("Content-Length", "0"))
                if not 0 < length <= 2 * 1024 * 1024:
                    return self.send(413, {"error": "request_size"})
                payload = strict_loads(self.rfile.read(length))
                messages = payload["messages"]
                if not isinstance(messages, list) or not all(
                    isinstance(item, dict) and type(item.get("role")) is str and type(item.get("content")) is str
                    for item in messages):
                    return self.send(400, {"error": "invalid_messages"})
                # Capture only actual model input and its digests; never Authorization,
                # environment, endpoint credentials, or arbitrary request extra fields.
                captured = [{"role": item["role"], "content": item["content"],
                             "content_sha256": byte_hash(item["content"].encode())} for item in messages]
                with lock:
                    sequence = len(state["requests"]) + 1
                    status = 503 if args.fail_first and sequence == 1 else 200
                    state["requests"].append({"sequence": sequence, "received_at": datetime.now(timezone.utc).isoformat(),
                                              "model": payload.get("model"), "messages": captured, "status": status})
                    (args.output / "state.json").write_text(json.dumps(state, ensure_ascii=False, indent=2) + "\n")
                if status == 503:
                    return self.send(status, {"error": {"message": "explicit mock transient failure", "type": "mock_unavailable"}})
                self.send(200, {"id": "s1-mock-" + str(sequence), "object": "chat.completion",
                                "choices": [{"index": 0, "message": {"role": "assistant", "content": json.dumps(advice, ensure_ascii=False)}, "finish_reason": "stop"}]})
            except (ValueError, KeyError, TypeError):
                self.send(400, {"error": "invalid_request"})

    print(json.dumps({"provider_kind": "mock", "listen": f"http://127.0.0.1:{args.port}", "scope": state["scope"]}), flush=True)
    ThreadingHTTPServer(("127.0.0.1", args.port), Handler).serve_forever()


if __name__ == "__main__":
    main()
