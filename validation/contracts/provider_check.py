"""Compare public mock-provider inputs to a Run's public frozen system prompt."""
import argparse
from datetime import datetime, timezone
import json
from pathlib import Path
import urllib.parse
import urllib.request

from .oracle import Oracle, byte_hash, strict_loads
from .semantic_checks import require, verify_hash


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--base-url", required=True)
    parser.add_argument("--fixture-url", required=True)
    parser.add_argument("--run-id", required=True)
    parser.add_argument("--request-marker", required=True, help="Exact unique substring in test user message")
    parser.add_argument("--minimum-requests", type=int, default=1)
    parser.add_argument("--report", required=True, type=Path)
    args = parser.parse_args()
    for url in (args.base_url, args.fixture_url):
        parts = urllib.parse.urlsplit(url)
        require(parts.scheme in ("http", "https") and parts.username is None and parts.password is None
                and not parts.query and not parts.fragment, "credential-free public test URLs required")
    def get(url):
        with urllib.request.urlopen(url, timeout=15) as response:
            raw = response.read(16 * 1024 * 1024 + 1)
            return strict_loads(raw)
    snapshot = get(args.base_url.rstrip("/") + "/api/runs/" + urllib.parse.quote(args.run_id, safe="") + "/definition")
    oracle = Oracle()
    oracle.validate("DefinitionSnapshot", snapshot)
    verify_hash("DefinitionSnapshot", snapshot)
    verify_hash("AgentDefinition", snapshot["definition"])
    state = get(args.fixture_url.rstrip("/") + "/fixture/state")
    require(state.get("provider_kind") == "mock", "this verifier expects explicit public mock request evidence")
    oracle.validate("DecisionAdvice", state["advice"])
    matched = [request for request in state["requests"] if any(
        message["role"] == "user" and args.request_marker in message["content"] for message in request["messages"])]
    require(len(matched) >= args.minimum_requests, "required matching actual model requests not observed")
    prompt = snapshot["definition"]["system_prompt"]
    require(byte_hash(prompt["content"].encode()) == prompt["sha256"], "frozen system prompt digest mismatch")
    for request in matched:
        require(request["messages"] and request["messages"][0]["role"] == "system", "first model input is not the frozen system layer")
        message = request["messages"][0]
        require(message["content"] == prompt["content"] and message["content_sha256"] == prompt["sha256"],
                "actual model system prompt differs from frozen Run definition")
        require(snapshot["provider_binding"] is not None and request["model"] == snapshot["provider_binding"]["model_id"],
                "actual requested model differs from frozen provider binding")
    report = {"status": "passed", "scope": "Actual mock-provider request inputs equal frozen prompt; not a live model or full decision chain",
              "recorded_at": datetime.now(timezone.utc).isoformat(), "run_id": args.run_id,
              "definition_version": snapshot["definition_version"], "snapshot_hash": snapshot["snapshot_hash"],
              "prompt_sha256": prompt["sha256"], "requests": matched}
    args.report.parent.mkdir(parents=True, exist_ok=True)
    args.report.write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n")
    print(json.dumps({key: value for key, value in report.items() if key != "requests"}, ensure_ascii=False))


if __name__ == "__main__":
    main()
