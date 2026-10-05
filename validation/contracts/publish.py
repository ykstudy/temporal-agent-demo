"""Explicit build-time copy of the public schemas; never derives from Agent DTOs."""
import argparse
import hashlib
import json
from pathlib import Path

HERE = Path(__file__).resolve().parent
ROOT = HERE.parents[1]
SOURCE = ROOT / "design" / "contracts" / "releases" / "1.1.0"
NAMES = ("agent.schema.json", "common.schema.json", "message.schema.json")


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--check", action="store_true")
    parser.add_argument("--source", type=Path, default=SOURCE)
    parser.add_argument("--destination", type=Path, default=HERE / "schemas")
    args = parser.parse_args()
    destination = args.destination
    source_label = args.source.resolve().relative_to(ROOT).as_posix()
    files = []
    for name in NAMES:
        raw = (args.source / name).read_bytes()
        files.append({"path": name, "source": source_label + "/" + name,
                      "schema_id": json.loads(raw)["$id"], "size": len(raw),
                      "sha256": hashlib.sha256(raw).hexdigest()})
        if args.check:
            if (destination / name).read_bytes() != raw:
                raise SystemExit("derived schema differs: " + name)
        else:
            destination.mkdir(parents=True, exist_ok=True)
            (destination / name).write_bytes(raw)
    manifest = (json.dumps({"publication_source": source_label, "files": files},
                           ensure_ascii=False, indent=2) + "\n").encode()
    if args.check:
        if (destination / "publication.json").read_bytes() != manifest:
            raise SystemExit("derived schema publication manifest differs")
    else:
        (destination / "publication.json").write_bytes(manifest)
    print("public schema release matches source" if args.check else "published independent schema release")


if __name__ == "__main__":
    main()
