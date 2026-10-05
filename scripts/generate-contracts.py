#!/usr/bin/env python3
"""Derive the Go wire types and embedded release from the published JSON Schemas.

This does not implement JSON Schema validation: every boundary uses the complete
embedded Draft 2020-12 documents. Union values retain their exact JSON and expose
all concrete branches as generated types, rather than losing discriminators or
nullable fields through a flattened DTO. --check never rewrites files.
"""
import argparse
import hashlib
import json
from pathlib import Path
import re
import subprocess
import sys

ROOT = Path(__file__).resolve().parents[1]
VERSION = "1.4.0"
RUNTIME_DEFAULT = "1.2.0"
VERSIONS = ["1.1.0", "1.2.0", "1.3.0", VERSION]
SOURCE = ROOT / "design/contracts/releases" / VERSION
DEST = ROOT / "backend/internal/contracts"
FILES = ["common.schema.json", "agent.schema.json", "message.schema.json"]
DOCS = {f: json.loads((SOURCE / f).read_bytes()) for f in FILES}
INITIALISMS = {"id": "ID", "ids": "IDs", "url": "URL", "uri": "URI", "sha256": "SHA256", "ci": "CI", "jsonl": "JSONL", "ttl": "TTL", "orm": "ORM", "http": "HTTP", "api": "API", "ms": "MS"}


def pascal(name):
    parts = re.sub(r"([a-z])([A-Z])", r"\1_\2", name.lstrip("$")).split("_")
    return "".join(INITIALISMS.get(p.lower(), p[:1].upper() + p[1:]) for p in parts)


names, occupied = {}, set()
for file in FILES:
    for name in DOCS[file]["$defs"]:
        preferred = name
        if preferred in occupied:
            preferred = file.split(".")[0].title() + name
        names[(file, name)] = preferred
        occupied.add(preferred)


def deref(file, ref):
    target, pointer = ref.split("#", 1)
    target = target or file
    assert target in DOCS and pointer.startswith("/$defs/"), ref
    return target, pointer.removeprefix("/$defs/")


def shape(file, schema):
    """Collect unconditional allOf properties; constraints stay in the Schema."""
    out = dict(schema)
    props, required = {}, []
    for part in schema.get("allOf", []):
        if "$ref" in part:
            rf, rn = deref(file, part["$ref"])
            target = DOCS[rf]["$defs"][rn]
            # allOf may refer through one or more alias definitions before the
            # actual object. Keep every inherited field, not just the local
            # discriminator constraint (e.g. typed message/manifest refs).
            seen = set()
            while "$ref" in target:
                key = (rf, rn)
                if key in seen:
                    raise ValueError(f"cyclic object alias in allOf: {key}")
                seen.add(key)
                rf, rn = deref(rf, target["$ref"])
                target = DOCS[rf]["$defs"][rn]
            resolved = shape(rf, target)
            for key in ("type", "items"):
                if key in resolved:
                    out.setdefault(key, resolved[key])
            # Referenced properties retain the file in which refs were defined.
            for key, value in resolved.get("properties", {}).items():
                props[key] = dict(value, __file=value.get("__file", rf))
            required.extend(resolved.get("required", []))
        elif not any(k in part for k in ("if", "then", "else")):
            for key in ("type", "items"):
                if key in part:
                    out.setdefault(key, part[key])
            props.update(part.get("properties", {}))
            required.extend(part.get("required", []))
    props.update(schema.get("properties", {}))
    required.extend(schema.get("required", []))
    if props:
        out.update(type="object", properties=props, required=sorted(set(required)))
    return out


declarations, metadata, union_methods = {}, {}, []
version_methods = []
OLD_DOCS = {v: {f: json.loads((ROOT / "design/contracts/releases" / v / f).read_bytes()) for f in FILES} for v in VERSIONS if v != VERSION}

def older_properties(file, pointer):
    out = {}
    for version, documents in OLD_DOCS.items():
        node = documents[file]
        try:
            for segment in pointer.lstrip("/").split("/"):
                segment = segment.replace("~1", "/").replace("~0", "~")
                node = node[int(segment)] if isinstance(node, list) else node[segment]
            if "properties" in node:
                out[version] = node["properties"]
        except (KeyError, IndexError, ValueError, TypeError):
            pass
    return out



def type_expr(file, schema, hint, pointer):
    if isinstance(schema, bool):
        # Component payload's actual branch constraints are in its parent's oneOf.
        return "json.RawMessage"
    file = schema.get("__file", file)
    schema = shape(file, schema)
    if "$ref" in schema:
        return names[deref(file, schema["$ref"])]
    branches = schema.get("anyOf", schema.get("oneOf", []))
    nonnull = [v for v in branches if v.get("type") != "null"]
    if len(branches) == 2 and len(nonnull) == 1:
        return "*" + type_expr(file, nonnull[0], hint, pointer + "/" + ("anyOf" if "anyOf" in schema else "oneOf") + "/" + str(branches.index(nonnull[0])))
    if branches:
        emit(file, schema, hint, pointer)
        return hint
    t = schema.get("type")
    if "const" in schema:
        value = schema["const"]
        t = "boolean" if isinstance(value, bool) else "string" if isinstance(value, str) else "integer" if isinstance(value, int) else "array" if isinstance(value, list) else "null"
        if t == "array":
            assert all(isinstance(v, str) for v in value), hint
            return "[]string"
    if "enum" in schema and not t:
        t = "string" if all(isinstance(v, str) for v in schema["enum"]) else "integer"
    if isinstance(t, list):
        if len(t) == 2 and "null" in t:
            return "*" + type_expr(file, dict(schema, type=next(v for v in t if v != "null")), hint, pointer)
        raise ValueError(f"unsupported type union {hint}")
    if t in ("string", "integer", "number", "boolean"):
        return {"string": "string", "integer": "int64", "number": "float64", "boolean": "bool"}[t]
    if t == "array":
        return "[]" + type_expr(file, schema["items"], hint + "Item", pointer + "/items")
    if t == "object" or schema.get("properties") or schema.get("allOf"):
        emit(file, schema, hint, pointer)
        return hint
    if t == "null":
        return "json.RawMessage"
    raise ValueError(f"unhandled schema shape {file}:{pointer}: {schema}")


def emit(file, original, name, pointer):
    if name in declarations:
        return
    declarations[name] = ""  # permit recursive JsonValue
    metadata[name] = DOCS[file]["$id"] + "#" + pointer
    schema = shape(file, original)
    branches = schema.get("oneOf", schema.get("anyOf", [])) if not schema.get("properties") else []
    # Object branches constrain a shared base as well as introduce new fields.
    if branches:
        for i, branch in enumerate(branches):
            branch_name = name + "Variant" + str(i + 1)
            type_expr(file, branch, branch_name, pointer + ("/oneOf/" if "oneOf" in schema else "/anyOf/") + str(i))
        declarations[name] = f"// {name} is the complete JSON Schema union; use Decode/Validate at boundaries.\ntype {name} json.RawMessage\n"
        union_methods.append(f'''func (v {name}) MarshalJSON() ([]byte, error) {{ return marshalUnion([]byte(v)) }}
func (v *{name}) UnmarshalJSON(data []byte) error {{
    if err := validateUnion("{name}", data); err != nil {{ return err }}
    *v = append((*v)[:0], data...)
    return nil
}}
''')
        return
    if schema.get("type") == "object" or schema.get("properties"):
        properties = schema.get("properties", {})
        if not properties and isinstance(schema.get("additionalProperties"), dict):
            value_type = type_expr(file, schema["additionalProperties"], name + "Value", pointer + "/additionalProperties")
            declarations[name] = f"type {name} map[string]{value_type}\n"
            return
        lines = [f"// {name} derives from {file}#{pointer}.", f"type {name} struct {{"]
        required = set(schema.get("required", []))
        earlier = older_properties(file, pointer)
        missing = {version: [key for key in properties if key not in old] for version, old in earlier.items()}
        new_fields = {key for keys in missing.values() for key in keys}
        versioned = "contract_version" in properties
        for key, value in properties.items():
            field_type = type_expr(file, value, name + pascal(key), pointer + "/properties/" + key.replace("~", "~0").replace("/", "~1"))
            tag = key
            if key not in required:
                if not field_type.startswith("*"):
                    field_type = "*" + field_type
                tag += ",omitempty"
            elif key in new_fields and not versioned:
                tag += ",omitzero"
            lines.append(f'    {pascal(key)} {field_type} `json:"{tag}"`')
        lines.append("}\n")
        declarations[name] = "\n".join(lines)
        if new_fields and versioned:
            method = [f"func (v {name}) MarshalJSON() ([]byte, error) {{", f"type plain {name}", "data, err := json.Marshal(plain(v))", "if err != nil { return nil, err }", "var remove []string", "switch v.ContractVersion {"]
            for version, keys in missing.items():
                if not keys:
                    continue
                method.append(f'case "{version}":')
                for key in keys:
                    method.append(f'if !reflect.ValueOf(&v.{pascal(key)}).Elem().IsZero() {{ return nil, fmt.Errorf("{name}.{key} is unavailable in this contract version") }}')
                method.append("remove = []string{" + ",".join(json.dumps(k) for k in keys) + "}")
            method.extend(["}", "if len(remove) == 0 { return data, nil }", "var fields map[string]json.RawMessage", "if err = json.Unmarshal(data, &fields); err != nil { return nil, err }", "for _, key := range remove { delete(fields, key) }", "return json.Marshal(fields)", "}"])
            version_methods.append("\n".join(method))
    else:
        declarations[name] = f"type {name} = {type_expr(file, schema, name, pointer)}\n"


for file in FILES:
    for name, schema in DOCS[file]["$defs"].items():
        emit(file, schema, names[(file, name)], "/$defs/" + name)

source_hashes = {file: hashlib.sha256((SOURCE / file).read_bytes()).hexdigest() for file in FILES}
version_hashes, version_refs = {}, {}
for version in VERSIONS:
    version_source = ROOT / "design/contracts/releases" / version
    version_docs = {file: json.loads((version_source / file).read_bytes()) for file in FILES}
    version_hashes[version] = {file: hashlib.sha256((version_source / file).read_bytes()).hexdigest() for file in FILES}
    version_refs[version] = {}
    for name, uri in metadata.items():
        base_uri, pointer = uri.split("#", 1)
        file = base_uri.rsplit("/", 1)[1]
        node = version_docs[file]
        try:
            for segment in pointer.lstrip("/").split("/"):
                segment = segment.replace("~1", "/").replace("~0", "~")
                node = node[int(segment)] if isinstance(node, list) else node[segment]
        except (KeyError, IndexError, ValueError, TypeError):
            continue
        version_refs[version][name] = version_docs[file]["$id"] + "#" + pointer
release = {"contract_version": VERSION, "runtime_default_version": RUNTIME_DEFAULT, "supported_contract_versions": VERSIONS, "schema_version": 2, "generator": "scripts/generate-contracts.py", "sources": source_hashes, "types": metadata, "releases": {version: {"sources": version_hashes[version], "types": version_refs[version]} for version in VERSIONS}}
head = "// Code generated by scripts/generate-contracts.py; DO NOT EDIT.\n// Full structural constraints are enforced by the embedded published Schemas.\npackage contracts\n\nimport (\"encoding/json\"; \"fmt\"; \"reflect\")\n\n"
body = head + "\n".join(declarations.values()) + "\n" + "\n".join(union_methods) + "\n" + "\n".join(version_methods)
body += f'\nconst ContractVersion = "{RUNTIME_DEFAULT}"\nconst LatestContractVersion = "{VERSION}"\nconst SchemaVersion = 2\n'
body += "\nvar generatedSchemaRefs = map[string]string{\n" + "".join(f'    "{name}": "{uri}",\n' for name, uri in sorted(metadata.items())) + "}\n"
body += "\nvar generatedSourceHashes = map[string]string{\n" + "".join(f'    "{name}": "{digest}",\n' for name, digest in source_hashes.items()) + "}\n"
body += "\nvar generatedVersionSourceHashes = map[string]map[string]string{\n" + "".join(f'    "{version}": {{\n' + "".join(f'        "{name}": "{digest}",\n' for name, digest in hashes.items()) + "    },\n" for version, hashes in version_hashes.items()) + "}\n"
body += "\nvar generatedVersionSchemaRefs = map[string]map[string]string{\n" + "".join(f'    "{version}": {{\n' + "".join(f'        "{name}": "{uri}",\n' for name, uri in sorted(refs.items())) + "    },\n" for version, refs in version_refs.items()) + "}\n"
formatted = subprocess.run(["gofmt"], input=body.encode(), stdout=subprocess.PIPE, check=True).stdout
outputs = {DEST / "types_generated.go": formatted, DEST / "schemas/release.json": (json.dumps(release, ensure_ascii=False, sort_keys=True, indent=2) + "\n").encode()}
outputs.update({DEST / "schemas" / file: (SOURCE / file).read_bytes() for file in FILES})
for version in VERSIONS:
    version_source = ROOT / "design/contracts/releases" / version
    outputs.update({DEST / "schemas" / version / file: (version_source / file).read_bytes() for file in FILES})
parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument("--check", action="store_true")
args = parser.parse_args()
stale = []
for path, content in outputs.items():
    if args.check:
        if not path.is_file() or path.read_bytes() != content:
            stale.append(str(path.relative_to(ROOT)))
    else:
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_bytes(content)
if stale:
    print("stale generated release: " + ", ".join(stale), file=sys.stderr)
    sys.exit(1)
print(f"{'Verified' if args.check else 'Generated'} {len(metadata)} schema-derived Go types and {len(FILES) * len(VERSIONS)} versioned exact-byte schemas")
