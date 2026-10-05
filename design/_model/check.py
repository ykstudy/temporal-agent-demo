#!/usr/bin/env python3
"""Validate the document model without claiming product runtime correctness."""
from __future__ import annotations

import argparse
from collections import Counter
import hashlib
import json
from pathlib import Path
import re
import shutil
import sys
import tempfile
from urllib.parse import unquote, urlsplit

FEATURE_SECTIONS = ["目标", "输入", "输出", "算法 / 流程", "边界与异常", "依赖", "被谁依赖", "验收标准"]
ADR_SECTIONS = ["状态", "背景", "决策", "理由", "备选方案", "影响", "关联", "历史"]
ENTITY_TYPES = {"domain", "feature", "adr", "invariant"}
RELATION_TYPES = {"belongs_to", "consumes", "submits_to", "decides", "constrains", "depends_on"}
BOUNDARY = "校验通过只证明文档结构、来源摘要与认知模型一致；不证明产品状态机、权限、预算、重试、外部副作用或运行验收已实现。"


class ModelError(ValueError):
    pass


def unique_pairs(pairs):
    result = {}
    for key, value in pairs:
        if key in result:
            raise ModelError(f"duplicate JSON/YAML key: {key}")
        result[key] = value
    return result


def strip_comment(text):
    quote, escaped = None, False
    for index, char in enumerate(text):
        if escaped:
            escaped = False
        elif char == "\\" and quote == '"':
            escaped = True
        elif char in "\"'":
            if quote == char:
                quote = None
            elif quote is None:
                quote = char
        elif char == "#" and quote is None and (index == 0 or text[index - 1].isspace()):
            return text[:index].rstrip()
    return text.rstrip()


def split_mapping(text):
    quote, escaped, depth = None, False, 0
    for index, char in enumerate(text):
        if escaped:
            escaped = False
        elif char == "\\" and quote == '"':
            escaped = True
        elif char in "\"'":
            if quote == char:
                quote = None
            elif quote is None:
                quote = char
        elif quote is None:
            if char in "[{":
                depth += 1
            elif char in "]}":
                depth -= 1
            elif char == ":" and depth == 0 and (index + 1 == len(text) or text[index + 1].isspace()):
                return text[:index].strip(), text[index + 1:].strip()
    return None


def split_inline(text):
    quote, escaped, depth, start, result = None, False, 0, 0, []
    for index, char in enumerate(text):
        if escaped:
            escaped = False
        elif char == "\\" and quote == '"':
            escaped = True
        elif char in "\"'":
            if quote == char:
                quote = None
            elif quote is None:
                quote = char
        elif quote is None:
            if char in "[{":
                depth += 1
            elif char in "]}":
                depth -= 1
            elif char == "," and depth == 0:
                result.append(text[start:index].strip())
                start = index + 1
    if quote is not None or depth != 0:
        raise ModelError("unterminated inline YAML value")
    if text[start:].strip():
        result.append(text[start:].strip())
    return result


def scalar(text):
    if text.startswith(("[", "{")):
        try:
            return json.loads(text, object_pairs_hook=unique_pairs)
        except json.JSONDecodeError:
            pass
    if text.startswith('"'):
        try:
            return json.loads(text)
        except ValueError as error:
            raise ModelError(f"quoted value must use JSON escapes: {error}") from error
    if text.startswith("'"):
        if not text.endswith("'") or len(text) < 2:
            raise ModelError("unterminated single-quoted YAML string")
        return text[1:-1].replace("''", "'")
    if text.startswith("["):
        if not text.endswith("]"):
            raise ModelError("unterminated inline YAML sequence")
        return [scalar(item) for item in split_inline(text[1:-1])]
    if text.startswith("{"):
        if not text.endswith("}"):
            raise ModelError("unterminated inline YAML mapping")
        result = {}
        for item in split_inline(text[1:-1]):
            pair = split_mapping(item)
            if pair is None:
                raise ModelError("inline YAML mappings need key: value")
            key, value = scalar(pair[0]), scalar(pair[1])
            if key in result:
                raise ModelError(f"duplicate YAML key: {key}")
            result[key] = value
        return result
    if text in {"", "null", "Null", "NULL", "~"}:
        return None
    if text in {"true", "True", "TRUE", "false", "False", "FALSE"}:
        return text.lower() == "true"
    if re.fullmatch(r"[-+]?\d+", text):
        return int(text)
    if text.startswith(("!", "&", "*", "|", ">")):
        raise ModelError("YAML tags, aliases and block strings require PyYAML; quote scalar strings instead")
    return text


def subset_yaml(text):
    """Strict YAML subset for these structural files, requiring no package."""
    tokens = []
    for number, raw in enumerate(text.splitlines(), 1):
        if "\t" in raw[:len(raw) - len(raw.lstrip())]:
            raise ModelError(f"line {number}: tabs are not valid indentation")
        line = strip_comment(raw)
        if not line.strip():
            continue
        if line.strip() == "---" and not tokens:
            continue
        if line.strip() in {"---", "..."}:
            raise ModelError(f"line {number}: only one YAML document is supported")
        tokens.append([len(line) - len(line.lstrip(" ")), line.lstrip(" "), number])
    if not tokens:
        return None

    def block(index, indent):
        sequence = tokens[index][1] == "-" or tokens[index][1].startswith("- ")
        result = [] if sequence else {}
        while index < len(tokens) and tokens[index][0] == indent:
            _, content, number = tokens[index]
            is_item = content == "-" or content.startswith("- ")
            if is_item != sequence:
                break
            if sequence:
                rest = content[1:].strip()
                if split_mapping(rest) is not None:
                    tokens[index] = [indent + 2, rest, number]
                    value, index = block(index, indent + 2)
                elif rest:
                    value, index = scalar(rest), index + 1
                elif index + 1 < len(tokens) and tokens[index + 1][0] > indent:
                    value, index = block(index + 1, tokens[index + 1][0])
                else:
                    value, index = None, index + 1
                result.append(value)
            else:
                pair = split_mapping(content)
                if pair is None:
                    raise ModelError(f"line {number}: expected key: value")
                key, rest = pair
                key = scalar(key)
                if not isinstance(key, str) or not key:
                    raise ModelError(f"line {number}: keys must be nonempty strings")
                if key in result:
                    raise ModelError(f"line {number}: duplicate YAML key: {key}")
                index += 1
                nested = index < len(tokens) and (tokens[index][0] > indent or (tokens[index][0] == indent and tokens[index][1].startswith("- ")))
                if not rest and nested:
                    value, index = block(index, tokens[index][0])
                else:
                    value = scalar(rest)
                result[key] = value
        return result, index

    result, end = block(0, tokens[0][0])
    if end != len(tokens):
        raise ModelError(f"line {tokens[end][2]}: unexpected indentation or collection")
    return result


def load_yaml(path):
    text = path.read_text(encoding="utf-8")
    if text.lstrip().startswith(("{", "[")):
        return json.loads(text, object_pairs_hook=unique_pairs)
    try:
        import yaml
    except ImportError:
        return subset_yaml(text)

    class UniqueLoader(yaml.SafeLoader):
        pass

    def mapping(loader, node, deep=False):
        result = {}
        for key_node, value_node in node.value:
            key = loader.construct_object(key_node, deep=deep)
            if key in result:
                raise ModelError(f"duplicate YAML key: {key}")
            result[key] = loader.construct_object(value_node, deep=deep)
        return result
    UniqueLoader.add_constructor(yaml.resolver.BaseResolver.DEFAULT_MAPPING_TAG, mapping)
    return yaml.load(text, Loader=UniqueLoader)


def visible_markdown(text):
    text = re.sub(r"<!--[\s\S]*?-->", lambda match: "\n" * match.group(0).count("\n"), text)
    lines, fence, indented, previous_blank, list_indent = [], None, False, True, None
    for line in text.splitlines():
        marker = re.match(r"^ {0,3}(`{3,}|~{3,})(.*)$", line)
        if fence is not None:
            if marker and marker.group(1)[0] == fence[0] and len(marker.group(1)) >= fence[1] and not marker.group(2).strip():
                fence = None
            lines.append("")
            previous_blank = True
            continue
        if marker and (marker.group(1)[0] != "`" or "`" not in marker.group(2)):
            fence = (marker.group(1)[0], len(marker.group(1)))
            lines.append("")
            previous_blank = True
            continue
        indentation = len(line.expandtabs(4)) - len(line.expandtabs(4).lstrip(" "))
        list_item = re.match(r"^ {0,3}(?:[-+*]|\d+[.)])\s+", line)
        if list_item:
            list_indent = len(list_item.group(0))
        elif line.strip() and indentation < (list_indent or 0):
            list_indent = None
        threshold = (list_indent or 0) + 4
        if line.strip() and indentation >= threshold and (indented or previous_blank):
            indented = True
            lines.append("")
        elif indented and not line.strip():
            lines.append("")
        else:
            indented = False
            lines.append(line)
        previous_blank = not line.strip()
    return "\n".join(lines)


def headings(text):
    lines = visible_markdown(text).splitlines()
    result, index = [], 0
    while index < len(lines):
        match = re.match(r"^ {0,3}(#{1,6})\s+(.+?)\s*#*\s*$", lines[index])
        if match:
            result.append((index, len(match.group(1)), match.group(2), 1))
        elif index + 1 < len(lines) and lines[index].strip() and len(lines[index]) - len(lines[index].lstrip(" ")) < 4:
            underline = re.match(r"^ {0,3}(=+|-+)\s*$", lines[index + 1])
            if underline and not re.match(r"^\s*(?:>|[-+*]\s|\d+[.)]\s)", lines[index]):
                result.append((index, 1 if underline.group(1)[0] == "=" else 2, lines[index].strip(), 2))
                index += 1
        index += 1
    return result


def slug(title):
    title = re.sub(r"!?\[([^\]]+)\]\([^)]*\)", r"\1", title)
    title = re.sub(r"<[^>]+>", "", title)
    title = title.strip().lower()
    title = re.sub(r"[^\w\- ]", "", title)
    return title.replace(" ", "-")


def anchors(text):
    found, counts = set(), Counter()
    for _, _, title, _ in headings(text):
        name = slug(title)
        index = counts[name]
        counts[name] += 1
        found.add(name if index == 0 else f"{name}-{index}")
    for line in visible_markdown(text).splitlines():
        for explicit in re.finditer(r"<(?:a|h[1-6])\b[^>]*\b(?:id|name)=[\"']([^\"']+)", line, re.I):
            found.add(explicit.group(1))
    return found


def sections(text):
    result, order, current = {}, [], None
    titles = {index: (title, span) for index, level, title, span in headings(text) if level == 2}
    underlines = {index + 1 for index, (_, span) in titles.items() if span == 2}
    for index, line in enumerate(visible_markdown(text).splitlines()):
        if index in titles:
            current = titles[index][0]
            order.append(current)
            result[current] = []
        elif current and index not in underlines:
            result[current].append(line)
    return order, {name: "\n".join(body) for name, body in result.items()}


def normalize_reference(label):
    return re.sub(r"\s+", " ", label).strip().casefold()


def reference_definitions(text):
    text = visible_markdown(text)
    text = re.sub(r"(`+)(.*?)\1", "", text, flags=re.S)
    result = {}
    for match in re.finditer(r"(?m)^ {0,3}\[([^\]]+)\]:\s*(<[^>]+>|\S+)", text):
        result.setdefault(normalize_reference(match.group(1)), match.group(2).strip("<>"))
    return result


def markdown_links(text, definitions=None):
    definitions = reference_definitions(text) if definitions is None else definitions
    text = visible_markdown(text)
    text = re.sub(r"(`+)(.*?)\1", "", text, flags=re.S)
    found = []
    for match in re.finditer(r"(?<!\\)!?\[[^\]\n]+\]\(\s*(<[^>]+>|[^\s)]+)(?:\s+['\"][^\n]*?['\"])?\s*\)", text):
        found.append(match.group(1).strip("<>"))
    for match in re.finditer(r"(?<!\\)\[([^\]\n]+)\]\[([^\]\n]*)\]", text):
        key = normalize_reference(match.group(2) or match.group(1))
        found.append(definitions.get(key, "reference-missing:" + key))
    for match in re.finditer(r"(?<![\\\]])!?\[([^\]\n]+)\](?!\(|\[|\s*:)", text):
        key = normalize_reference(match.group(1))
        if key in definitions:
            found.append(definitions[key])
    for match in re.finditer(r"<(?:a|img)\b[^>]*\b(?:href|src)=[\"']([^\"']+)", text, re.I):
        found.append(match.group(1))
    return found


class Validator:
    def __init__(self, root):
        self.root, self.errors = root.resolve(), []
        self.design = self.root / "design"
        self.models, self.entities, self.files, self.body = {}, {}, {}, {}
        self.relations = []

    def error(self, code, message):
        self.errors.append({"code": code, "message": message})

    def local(self, source, target):
        parsed = urlsplit(target)
        if parsed.scheme or parsed.netloc:
            if parsed.scheme == "reference-missing":
                self.error("link_missing", f"{source.relative_to(self.root)}: undefined reference link {target}")
            return None
        path = unquote(parsed.path)
        if path.startswith("/"):
            self.error("path_escape", f"{source.relative_to(self.root)}: local links must be relative: {target}")
            return None
        resolved = (source.parent / path).resolve() if path else source.resolve()
        if not resolved.is_relative_to(self.root):
            self.error("path_escape", f"{source.relative_to(self.root)}: link escapes project: {target}")
            return None
        return resolved, unquote(parsed.fragment)

    def fields(self, value, allowed, required, context):
        if not isinstance(value, dict):
            self.error("schema", f"{context}: expected mapping")
            return False
        extra, missing = set(value) - allowed, required - set(value)
        if extra:
            self.error("structure_only", f"{context}: unsupported fields {sorted(extra)}; model must not copy prose or runtime state")
        if missing:
            self.error("schema", f"{context}: missing fields {sorted(missing)}")
        return not missing

    def load(self):
        for name, collection in [("entities", "entities"), ("relations", "relations"), ("constraints", "checks")]:
            path = self.design / "_model" / (name + ".yaml")
            try:
                data = load_yaml(path)
            except Exception as error:
                self.error("schema", f"{path.relative_to(self.root)}: {error}")
                continue
            keys = {"schema_version", collection} | ({"baseline"} if name == "constraints" else set())
            if not self.fields(data, keys, keys, name):
                continue
            if type(data["schema_version"]) is not int or data["schema_version"] != 1:
                self.error("schema", f"{name}: schema_version must be integer 1")
            if not isinstance(data[collection], list):
                self.error("schema", f"{name}.{collection}: expected sequence")
                continue
            self.models[name] = data

    def baseline(self):
        data = self.models.get("constraints", {})
        baseline = data.get("baseline", {})
        required = {"source", "source_sha256", "invariants", "invariants_sha256"}
        if self.fields(baseline, required, required, "baseline"):
            for key in ("source", "invariants"):
                location, expected = baseline[key], baseline[key + "_sha256"]
                canonical = "docs/PRODUCT-GUIDE.md" if key == "source" else "design/00-invariants/INVARIANTS.md"
                if location != canonical:
                    self.error("baseline", f"baseline.{key}: must reference {canonical}, not another document")
                if not isinstance(location, str) or not isinstance(expected, str) or not re.fullmatch(r"[0-9a-f]{64}", expected):
                    self.error("baseline", f"baseline.{key}: need relative path and lowercase SHA-256")
                    continue
                path = (self.root / location).resolve()
                if Path(location).is_absolute() or not path.is_relative_to(self.root) or not path.is_file():
                    self.error("baseline", f"baseline.{key}: missing file or path outside project")
                    continue
                actual = hashlib.sha256(path.read_bytes()).hexdigest()
                if actual != expected:
                    self.error("baseline", f"baseline.{key}: frozen bytes changed; expected {expected}, actual {actual}")
        seen = set()
        for index, check in enumerate(data.get("checks", [])):
            if not self.fields(check, {"id", "kind", "description"}, {"id", "kind", "description"}, f"checks[{index}]"):
                continue
            if any(not isinstance(check[key], str) or not check[key].strip() for key in ("id", "kind", "description")):
                self.error("schema", f"checks[{index}]: id, kind and description must be nonempty strings")
                continue
            if check["id"] in seen:
                self.error("duplicate_id", f"duplicate check ID: {check['id']}")
            seen.add(check["id"])

    def check_entities(self):
        for index, entity in enumerate(self.models.get("entities", {}).get("entities", [])):
            if not self.fields(entity, {"id", "type", "name", "path", "domain"}, {"id", "type", "name", "path"}, f"entities[{index}]"):
                continue
            if any(not isinstance(entity[key], str) or not entity[key].strip() for key in ("id", "type", "name", "path")):
                self.error("schema", f"entities[{index}]: required fields must be nonempty strings")
                continue
            identifier = entity["id"]
            if identifier in self.entities:
                self.error("duplicate_id", f"duplicate entity ID: {identifier}")
                continue
            self.entities[identifier] = entity
            if entity["type"] not in ENTITY_TYPES:
                self.error("schema", f"{identifier}: unknown type {entity['type']}")
            parsed = urlsplit(entity["path"])
            path = (self.design / unquote(parsed.path)).resolve()
            if parsed.scheme or parsed.netloc or Path(parsed.path).is_absolute() or parsed.query or not path.is_relative_to(self.design):
                self.error("path_escape", f"{identifier}: entity path must remain inside design/: {entity['path']}")
                continue
            if not path.is_file():
                self.error("missing_path", f"{identifier}: missing {entity['path']}")
                continue
            if path.suffix.lower() != ".md":
                self.error("schema", f"{identifier}: entity content must be Markdown")
                continue
            self.files[identifier] = path
            text = path.read_text(encoding="utf-8")
            self.body[identifier] = text
            if parsed.fragment and unquote(parsed.fragment) not in anchors(text):
                self.error("link_anchor", f"{identifier}: missing anchor #{parsed.fragment}")
            if entity["type"] == "feature":
                if not isinstance(entity.get("domain"), str) or not entity["domain"]:
                    self.error("feature_domain", f"{identifier}: domain is required")
                if sections(text)[0] != FEATURE_SECTIONS:
                    self.error("feature_sections", f"{identifier}: expected exactly {FEATURE_SECTIONS}, received {sections(text)[0]}")
            elif "domain" in entity:
                self.error("schema", f"{identifier}: domain field is reserved for features")
            if entity["type"] == "adr" and sections(text)[0] != ADR_SECTIONS:
                self.error("adr_sections", f"{identifier}: expected exactly {ADR_SECTIONS}, received {sections(text)[0]}")
        domains = [entity for entity in self.entities.values() if entity["type"] == "domain"]
        if len(domains) != 9:
            self.error("domain_count", f"expected 9 domains, received {len(domains)}")
        for identifier, entity in self.entities.items():
            if entity["type"] == "feature" and self.entities.get(entity.get("domain"), {}).get("type") != "domain":
                self.error("feature_domain", f"{identifier}: domain must resolve to a registered domain")
        feature_paths = [self.files[identifier] for identifier, entity in self.entities.items() if entity["type"] == "feature" and identifier in self.files]
        for path, count in Counter(feature_paths).items():
            if count > 1:
                self.error("duplicate_path", f"multiple features use {path.relative_to(self.root)}")
        registered_features = {path for identifier, path in self.files.items() if self.entities[identifier]["type"] == "feature"}
        registered_adrs = {path for identifier, path in self.files.items() if self.entities[identifier]["type"] == "adr"}
        for path in self.design.glob("*/features/*.md"):
            if path.resolve() not in registered_features:
                self.error("unregistered", f"feature document is not registered: {path.relative_to(self.root)}")
        for path in (self.design / "10-decisions").glob("ADR-*.md"):
            if path.resolve() not in registered_adrs:
                self.error("unregistered", f"ADR document is not registered: {path.relative_to(self.root)}")
        invariant_file = self.design / "00-invariants" / "INVARIANTS.md"
        if invariant_file.is_file():
            seen_invariants = set()
            for _, _, title, _ in headings(invariant_file.read_text(encoding="utf-8")):
                match = re.match(r"^(INV-\d{3})(?:\s|$)", title)
                if not match:
                    continue
                identifier = match.group(1)
                if identifier in seen_invariants:
                    self.error("duplicate_id", f"duplicate invariant heading ID: {identifier}")
                seen_invariants.add(identifier)
                entity = self.entities.get(identifier, {})
                fragment = unquote(urlsplit(entity.get("path", "")).fragment)
                if (entity.get("type") != "invariant" or self.files.get(identifier) != invariant_file.resolve()
                        or fragment != slug(title)):
                    self.error("invariant_coverage", f"{identifier}: invariant heading must be registered with its matching invariant type, file and #{slug(title)} anchor")

    def check_relations(self):
        seen = set()
        for index, relation in enumerate(self.models.get("relations", {}).get("relations", [])):
            if not self.fields(relation, {"from", "to", "type", "contract"}, {"from", "to", "type"}, f"relations[{index}]"):
                continue
            if any(not isinstance(relation[key], str) or not relation[key] for key in ("from", "to", "type")):
                self.error("relation", f"relations[{index}]: from/to/type must be strings")
                continue
            source, target, kind = relation["from"], relation["to"], relation["type"]
            fingerprint = (source, target, kind, str(relation.get("contract", "")))
            if fingerprint in seen:
                self.error("relation", f"duplicate relation: {fingerprint}")
            seen.add(fingerprint)
            if source not in self.entities or target not in self.entities:
                self.error("dangling_relation", f"unresolved relation: {source} {kind} {target}")
                continue
            left, right = self.entities[source]["type"], self.entities[target]["type"]
            if kind not in RELATION_TYPES:
                self.error("relation", f"unknown relation type: {kind}")
                continue
            if source == target:
                self.error("relation", f"self relation is not allowed: {source} {kind}")
            if kind == "belongs_to" and (left != "feature" or right != "domain"):
                self.error("relation", f"belongs_to needs feature -> domain: {source} -> {target}")
            if kind == "decides" and (left != "adr" or right not in {"domain", "feature"}):
                self.error("relation", f"decides needs ADR -> domain/feature: {source} -> {target}")
            if kind == "constrains" and (left != "invariant" or right not in {"domain", "feature"}):
                self.error("relation", f"constrains needs invariant -> domain/feature: {source} -> {target}")
            if kind == "depends_on" and left == right == "feature":
                self.error("internal_dependency", f"{source} depends_on {target}: feature dependencies must use consumes/submits_to and a public #输入/#输出 contract")
            if kind in {"consumes", "submits_to"} and left == right == "feature":
                expected = "输出" if kind == "consumes" else "输入"
                if relation.get("contract") != expected:
                    self.error("internal_dependency", f"{source} {kind} {target}: contract must be {expected}; internal sections are not dependency contracts")
            elif "contract" in relation:
                self.error("relation", f"contract is only valid on feature consumes/submits_to: {source} {kind} {target}")
            self.relations.append(relation)
        for identifier, entity in self.entities.items():
            if entity["type"] == "feature":
                belongs = [r["to"] for r in self.relations if r["type"] == "belongs_to" and r["from"] == identifier]
                if belongs != [entity.get("domain")]:
                    self.error("feature_domain", f"{identifier}: belongs_to must match exactly its domain; received {belongs}")

    def links(self):
        for source in sorted(self.design.rglob("*.md")):
            if not source.resolve().is_relative_to(self.root):
                self.error("path_escape", f"Markdown file resolves outside project: {source}")
                continue
            for target in markdown_links(source.read_text(encoding="utf-8")):
                resolved = self.local(source, target)
                if resolved is None:
                    continue
                path, fragment = resolved
                if not path.is_file():
                    self.error("link_missing", f"{source.relative_to(self.root)}: missing target {target}")
                elif fragment and (path.suffix.lower() != ".md" or fragment not in anchors(path.read_text(encoding="utf-8"))):
                    self.error("link_anchor", f"{source.relative_to(self.root)}: missing anchor {target}")

    def contracts(self):
        by_path = {path: identifier for identifier, path in self.files.items() if self.entities[identifier]["type"] == "feature"}
        actual, reverse = set(), set()
        for identifier, entity in self.entities.items():
            if entity["type"] != "feature" or identifier not in self.files:
                continue
            bodies = sections(self.body[identifier])[1]
            definitions = reference_definitions(self.body[identifier])
            for section, destination in [("依赖", actual), ("被谁依赖", reverse)]:
                for target in markdown_links(bodies.get(section, ""), definitions):
                    resolved = self.local(self.files[identifier], target)
                    if resolved and resolved[0] in by_path:
                        if resolved[1] not in {"输入", "输出"}:
                            self.error("internal_dependency", f"{identifier} / {section}: feature dependency must link #输入 or #输出: {target}")
                            continue
                        destination.add((identifier, by_path[resolved[0]], resolved[1]))
        expected, expected_reverse = set(), set()
        for relation in self.relations:
            source, target, kind = relation["from"], relation["to"], relation["type"]
            if kind in {"consumes", "submits_to"} and self.entities[source]["type"] == self.entities[target]["type"] == "feature":
                contract = "输出" if kind == "consumes" else "输入"
                expected.add((source, target, contract))
                expected_reverse.add((target, source, "输入" if contract == "输出" else "输出"))
        for name, found, wanted in [("依赖", actual, expected), ("被谁依赖", reverse, expected_reverse)]:
            for item in sorted(wanted - found):
                self.error("dependency_missing", f"{name} missing modeled contract reference: {item}")
            for item in sorted(found - wanted):
                self.error("dependency_unmodeled", f"{name} reference has no matching model relation: {item}")

    def adr(self):
        for identifier, entity in self.entities.items():
            if entity["type"] != "adr":
                continue
            decisions = [relation["to"] for relation in self.relations if relation["from"] == identifier and relation["type"] == "decides"]
            kinds = {self.entities[target]["type"] for target in decisions}
            if not {"domain", "feature"}.issubset(kinds):
                self.error("adr_association", f"{identifier}: at least one domain and one feature decides relation required")
            if identifier not in self.files:
                continue
            associated = set()
            definitions = reference_definitions(self.body[identifier])
            for target in markdown_links(sections(self.body[identifier])[1].get("关联", ""), definitions):
                resolved = self.local(self.files[identifier], target)
                if resolved:
                    associated.add(resolved[0])
            for target in decisions:
                if target in self.files and self.files[target] not in associated:
                    self.error("adr_association", f"{identifier}: 关联 must contain a relative link to decided entity {target}")

    def run(self):
        try:
            self.load()
            self.baseline()
            self.check_entities()
            self.check_relations()
            self.links()
            self.contracts()
            self.adr()
        except Exception as error:
            self.error("validator_input", f"invalid model or Markdown input: {type(error).__name__}: {error}")
        return self.errors


def write_model(path, data):
    path.write_text(json.dumps(data, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")


def model_copy_ignore(path, names):
    # Self-tests mutate only document-model inputs. Runtime reports contain
    # generated workspaces, module caches and private launcher state; none are
    # model inputs and copying them makes each negative probe needlessly huge.
    ignored = set(shutil.ignore_patterns("node_modules", ".git", ".data", ".venv", ".contract-env", "__pycache__", "dist", "coverage")(path, names))
    if Path(path).name == "validation":
        ignored.update({"reports"}.intersection(names))
    return ignored


def self_test(root):
    baseline = Validator(root)
    if baseline.run():
        return [{"name": "valid baseline prerequisite", "passed": False, "errors": baseline.errors}]
    tests = []
    expected_codes = ["baseline", "missing_path", "duplicate_id", "dangling_relation", "internal_dependency", "adr_association", "structure_only"]
    names = ["source and frozen invariant hashes", "missing entity path", "duplicate entity ID", "dangling relation", "internal-section dependency", "ADR missing associations", "structural model content pollution"]
    for number, (name, expected) in enumerate(zip(names, expected_codes)):
        with tempfile.TemporaryDirectory(prefix="document-model-self-test-") as temporary:
            copied = Path(temporary) / "project"
            shutil.copytree(root, copied, ignore=model_copy_ignore)
            model = copied / "design" / "_model"
            entities, relations, constraints = [load_yaml(model / (file + ".yaml")) for file in ("entities", "relations", "constraints")]
            if number == 0:
                for field in ("source", "invariants"):
                    frozen = copied / constraints["baseline"][field]
                    frozen.write_bytes(frozen.read_bytes() + b"\n<!-- injected frozen document change -->\n")
            elif number == 1:
                entities["entities"][0]["path"] = "missing-self-test-document.md"
                write_model(model / "entities.yaml", entities)
            elif number == 2:
                entities["entities"].append(dict(entities["entities"][0]))
                write_model(model / "entities.yaml", entities)
            elif number == 3:
                relations["relations"].append({"from": entities["entities"][0]["id"], "to": "missing.self-test", "type": "depends_on"})
                write_model(model / "relations.yaml", relations)
            elif number == 4:
                edge = next((relation for relation in relations["relations"] if relation["type"] in {"consumes", "submits_to"} and "contract" in relation), None)
                if edge is None:
                    tests.append({"name": name, "passed": False, "error": "no feature contract edge available for injection"})
                    continue
                index = {entity["id"]: entity for entity in entities["entities"]}
                source = copied / "design" / index[edge["from"]]["path"].split("#", 1)[0]
                target_file = (copied / "design" / index[edge["to"]]["path"].split("#", 1)[0]).resolve()
                content = source.read_text(encoding="utf-8")
                body = sections(content)[1]["依赖"]
                definitions = reference_definitions(content)
                link = next((item for item in markdown_links(body, definitions) if (source.parent / unquote(urlsplit(item).path)).resolve() == target_file and unquote(urlsplit(item).fragment) == edge["contract"]), None)
                if link is None:
                    tests.append({"name": name, "passed": False, "error": "no corresponding dependency link available for injection"})
                    continue
                replacement = link.split("#", 1)[0] + "#" + slug("算法 / 流程")
                if link in body:
                    start = content.find(body)
                    if start < 0:
                        tests.append({"name": name, "passed": False, "error": "cannot locate dependency section in raw Markdown"})
                        continue
                    changed = content[:start] + content[start:].replace(link, replacement, 1)
                else:
                    declaration = next((match for match in re.finditer(r"(?m)^ {0,3}\[([^\]]+)\]:\s*(<[^>]+>|\S+)", content) if definitions.get(normalize_reference(match.group(1))) == link and match.group(2).strip("<>") == link), None)
                    if declaration is None:
                        tests.append({"name": name, "passed": False, "error": "cannot locate dependency reference definition for injection"})
                        continue
                    changed = content[:declaration.start(2)] + declaration.group(2).replace(link, replacement) + content[declaration.end(2):]
                source.write_text(changed, encoding="utf-8")
            elif number == 5:
                adr_id = next((entity["id"] for entity in entities["entities"] if entity["type"] == "adr"), None)
                if adr_id is None:
                    tests.append({"name": name, "passed": False, "error": "no ADR available for injection"})
                    continue
                relations["relations"] = [relation for relation in relations["relations"] if not (relation["from"] == adr_id and relation["type"] == "decides")]
                write_model(model / "relations.yaml", relations)
            else:
                entities["entities"][0]["workflow_steps"] = [{"id": "A", "depends_on": ["B"]}, {"id": "B", "depends_on": ["A"]}]
                write_model(model / "entities.yaml", entities)
            errors = Validator(copied).run()
            passed = expected in {error["code"] for error in errors}
            if number == 0:
                passed = passed and sum(error["code"] == "baseline" for error in errors) >= 2
            tests.append({"name": name, "passed": passed, "expected_code": expected, "caught_codes": sorted({error["code"] for error in errors})})
    return tests


def guard_self_test(root):
    """Extra regressions for registration and public-contract bypass attempts."""
    baseline = Validator(root)
    if baseline.run():
        return [{"name": "valid baseline prerequisite", "passed": False, "errors": baseline.errors}]
    tests = []
    cases = [("feature depends_on bypass", "internal_dependency"),
             ("unregistered invariant heading", "invariant_coverage"),
             ("feature type spoofing", "unregistered"), ("ADR type spoofing", "unregistered")]
    for number, (name, expected) in enumerate(cases):
        with tempfile.TemporaryDirectory(prefix="document-guard-self-test-") as temporary:
            copied = Path(temporary) / "project"
            shutil.copytree(root, copied, ignore=model_copy_ignore)
            model = copied / "design" / "_model"
            entities, relations = [load_yaml(model / (file + ".yaml")) for file in ("entities", "relations")]
            by_id = {entity["id"]: entity for entity in entities["entities"]}
            if number == 0:
                edge = next((relation for relation in relations["relations"] if relation["type"] in {"consumes", "submits_to"} and "contract" in relation), None)
                if edge is None:
                    tests.append({"name": name, "passed": False, "error": "no feature contract edge available"})
                    continue
                source, target = [(copied / "design" / by_id[edge[key]]["path"].split("#", 1)[0]).resolve() for key in ("from", "to")]

                def remove_contract(path, section, goal, fragment):
                    text = path.read_text(encoding="utf-8")
                    definitions = reference_definitions(text)
                    titles = {index: title for index, level, title, _ in headings(text) if level == 2}
                    inside, result = False, []
                    for index, line in enumerate(text.splitlines()):
                        if index in titles:
                            inside = titles[index] == section
                        matches = inside and any((path.parent / unquote(urlsplit(link).path)).resolve() == goal
                                                 and unquote(urlsplit(link).fragment) == fragment
                                                 for link in markdown_links(line, definitions))
                        if not matches:
                            result.append(line)
                    path.write_text("\n".join(result) + "\n", encoding="utf-8")

                remove_contract(source, "依赖", target, edge["contract"])
                remove_contract(target, "被谁依赖", source, "输入" if edge["contract"] == "输出" else "输出")
                edge["type"] = "depends_on"
                edge.pop("contract")
            else:
                kind = "invariant" if number == 1 else "feature" if number == 2 else "adr"
                entity = next((item for item in entities["entities"] if item["type"] == kind), None)
                if entity is None:
                    tests.append({"name": name, "passed": False, "error": f"no {kind} available"})
                    continue
                identifier = entity["id"]
                if number == 1:
                    entities["entities"] = [item for item in entities["entities"] if item["id"] != identifier]
                else:
                    entity["type"] = "invariant"
                    entity.pop("domain", None)
                relations["relations"] = [relation for relation in relations["relations"] if identifier not in (relation["from"], relation["to"])]
            write_model(model / "entities.yaml", entities)
            write_model(model / "relations.yaml", relations)
            errors = Validator(copied).run()
            codes = sorted({error["code"] for error in errors})
            tests.append({"name": name, "passed": expected in codes, "expected_code": expected, "caught_codes": codes})
    return tests


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--root", type=Path, default=Path(__file__).resolve().parents[2], help="project root (default: two levels above this checker)")
    parser.add_argument("--json", action="store_true", help="emit structured JSON diagnostics")
    parser.add_argument("--self-test", action="store_true", help="inject seven error classes into temporary copies; never modify source files")
    args = parser.parse_args()
    root = args.root.resolve()
    validator = Validator(root)
    errors = validator.run()
    result = {"passed": not errors, "entity_count": len(validator.entities), "relation_count": len(validator.relations), "errors": errors, "boundary": BOUNDARY}
    if args.self_test:
        result["self_tests"] = self_test(root)
        result["guard_tests"] = guard_self_test(root)
        result["passed"] = result["passed"] and all(test["passed"] for test in result["self_tests"] + result["guard_tests"])
        result["self_test_scope"] = "This model contains no runtime Task DAG; seventh injection rejects workflow content pollution instead of asserting runtime cycle detection."
    if args.json:
        print(json.dumps(result, ensure_ascii=False, indent=2))
    else:
        print(f"{'PASS' if result['passed'] else 'FAIL'}: {result['entity_count']} entities, {result['relation_count']} relations")
        for error in errors:
            print(f"[{error['code']}] {error['message']}")
        for test in result.get("self_tests", []):
            print(f"SELF-TEST {'PASS' if test['passed'] else 'FAIL'}: {test['name']}")
            if not test["passed"]:
                print(json.dumps(test, ensure_ascii=False))
        for test in result.get("guard_tests", []):
            print(f"GUARD-TEST {'PASS' if test['passed'] else 'FAIL'}: {test['name']}")
            if not test["passed"]:
                print(json.dumps(test, ensure_ascii=False))
        if args.self_test:
            print(result["self_test_scope"])
        print(BOUNDARY)
    return 0 if result["passed"] else 1


if __name__ == "__main__":
    sys.exit(main())
