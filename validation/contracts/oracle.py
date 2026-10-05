"""Strict JSON and complete Draft 2020-12 validation over a pinned offline release."""
from __future__ import annotations

import hashlib
import json
import math
from decimal import Decimal
from pathlib import Path
from typing import Any
from urllib.parse import urljoin

from jsonschema import Draft202012Validator, FormatChecker, validators
from referencing import Registry, Resource
from referencing.exceptions import NoSuchResource
import rfc8785

HERE = Path(__file__).resolve().parent
MAX_SAFE_INTEGER = 2**53 - 1
DEFAULT_MAX_BYTES = 16 * 1024 * 1024
MAX_DEPTH = 128
MAX_NODES = 500000


def _schema_integer(_checker, value):
    if type(value) is int:
        return True
    if isinstance(value, Decimal):
        return value.is_finite() and value == value.to_integral_value()
    return type(value) is float and math.isfinite(value) and value.is_integer()


# Keep the entire standard validator. Exact decimals prevent a fractional version
# from rounding to an integer before the type/range keywords see it.
ExactDraft202012Validator = validators.extend(
    Draft202012Validator,
    type_checker=Draft202012Validator.TYPE_CHECKER.redefine("integer", _schema_integer),
)


class ContractError(ValueError):
    """Invalid input, release, object shape, or independently checked semantics."""


def byte_hash(raw: bytes) -> str:
    return hashlib.sha256(raw).hexdigest()


def _pairs(pairs):
    result = {}
    for key, value in pairs:
        if key in result:
            raise ContractError("duplicate JSON property")
        result[key] = value
    return result


def _integer(raw: str) -> int:
    if len(raw.lstrip("-")) > 16:
        raise ContractError("integer outside interoperable safe range")
    value = int(raw)
    if abs(value) > MAX_SAFE_INTEGER:
        raise ContractError("integer outside interoperable safe range")
    return value


def _constant(_raw: str):
    raise ContractError("non-JSON numeric constant")


def _decimal(raw: str) -> Decimal:
    if len(raw) > 128:
        raise ContractError("numeric literal length limit exceeded")
    exponent = raw.lower().partition("e")[2]
    if exponent and abs(int(exponent)) > 400:
        raise ContractError("numeric exponent limit exceeded")
    value = Decimal(raw)
    if not value.is_finite() or not math.isfinite(float(value)):
        raise ContractError("non-finite JSON number")
    return value


def check_value(value: Any, depth: int = 0, _nodes=None) -> None:
    if _nodes is None:
        _nodes = [0]
    _nodes[0] += 1
    if _nodes[0] > MAX_NODES:
        raise ContractError("JSON value node limit exceeded")
    if depth > MAX_DEPTH:
        raise ContractError("JSON nesting limit exceeded")
    if value is None or type(value) is bool:
        return
    if type(value) is str:
        try:
            value.encode("utf-8", "strict")
        except UnicodeError as error:
            raise ContractError("invalid Unicode scalar value") from error
    elif type(value) is int:
        if abs(value) > MAX_SAFE_INTEGER:
            raise ContractError("integer outside interoperable safe range")
    elif type(value) is float:
        if not math.isfinite(value):
            raise ContractError("non-finite JSON number")
    elif isinstance(value, Decimal):
        if not value.is_finite() or not math.isfinite(float(value)):
            raise ContractError("non-finite JSON number")
    elif type(value) is list:
        for item in value:
            check_value(item, depth + 1, _nodes)
    elif type(value) is dict:
        for key, item in value.items():
            if type(key) is not str:
                raise ContractError("JSON object key must be a string")
            try:
                key.encode("utf-8", "strict")
            except UnicodeError as error:
                raise ContractError("invalid Unicode object key") from error
            check_value(item, depth + 1, _nodes)
    else:
        raise ContractError("non-JSON value type")


def strict_loads(raw: bytes | str, max_bytes: int = DEFAULT_MAX_BYTES) -> Any:
    try:
        encoded = raw.encode("utf-8", "strict") if isinstance(raw, str) else raw
        if len(encoded) > max_bytes:
            raise ContractError("JSON byte limit exceeded")
        text = encoded.decode("utf-8", "strict")
        # Reject excessive depth before invoking the recursive parser; ignore quoted brackets.
        depth, quoted, escaped = 0, False, False
        for char in text:
            if quoted:
                if escaped:
                    escaped = False
                elif char == "\\":
                    escaped = True
                elif char == '"':
                    quoted = False
            elif char == '"':
                quoted = True
            elif char in "[{":
                depth += 1
                if depth > MAX_DEPTH + 1:
                    raise ContractError("JSON nesting limit exceeded")
            elif char in "]}":
                depth -= 1
        value = json.loads(text, object_pairs_hook=_pairs, parse_int=_integer, parse_float=_decimal,
                           parse_constant=_constant)
        check_value(value)
        return value
    except (UnicodeError, json.JSONDecodeError, RecursionError) as error:
        raise ContractError("invalid UTF-8 JSON document") from error


def canonicalize(value: Any) -> bytes:
    check_value(value)
    def binary64(item):
        if isinstance(item, Decimal):
            return float(item)
        if isinstance(item, dict):
            return {key: binary64(child) for key, child in item.items()}
        if isinstance(item, list):
            return [binary64(child) for child in item]
        return item
    try:
        return rfc8785.dumps(binary64(value))
    except (ValueError, TypeError) as error:
        raise ContractError("JCS canonicalization failed") from error


def _offline(uri: str):
    # Explicitly disabled, including HTTPS references. Registry lookup is the only path.
    raise NoSuchResource(ref=uri)


class Oracle:
    def __init__(self, release: Path = HERE / "schemas", source: Path | None = None):
        self.release = Path(release)
        self.publication = strict_loads((self.release / "publication.json").read_bytes())
        self.schemas, self.raw = {}, {}
        entries = self.publication["files"]
        names = [entry["path"] for entry in entries]
        if sorted(names) != ["agent.schema.json", "common.schema.json", "message.schema.json"]:
            raise ContractError("release must contain exactly the three public schemas")
        for entry in entries:
            name = entry["path"]
            raw = (self.release / name).read_bytes()
            if byte_hash(raw) != entry["sha256"] or len(raw) != entry["size"]:
                raise ContractError("published schema bytes do not match release manifest")
            if source is not None and raw != (Path(source) / name).read_bytes():
                raise ContractError("derived schema differs from publication source")
            schema = strict_loads(raw)
            if schema.get("$id") != entry["schema_id"]:
                raise ContractError("published schema identity mismatch")
            Draft202012Validator.check_schema(schema)
            self.schemas[name], self.raw[name] = schema, raw
        self.registry = Registry(retrieve=_offline).with_resources(
            (schema["$id"], Resource.from_contents(schema)) for schema in self.schemas.values())
        self.formats = FormatChecker()
        if "date-time" not in self.formats.checkers:
            raise ContractError("date-time assertion support is required")
        self.static_refs = 0
        for schema in self.schemas.values():
            self._check_refs(schema, schema["$id"])

    def _check_refs(self, value: Any, base: str):
        if isinstance(value, dict):
            base = urljoin(base, value.get("$id", ""))
            if "$ref" in value:
                try:
                    self.registry.resolver(base).lookup(value["$ref"])
                except Exception as error:
                    raise ContractError("unresolved offline schema reference") from error
                self.static_refs += 1
            for child in value.values():
                self._check_refs(child, base)
        elif isinstance(value, list):
            for child in value:
                self._check_refs(child, base)

    def validate(self, name: str, value: Any, schema_file: str = "agent.schema.json") -> None:
        check_value(value)
        schema = self.schemas[schema_file]
        if name not in schema.get("$defs", {}):
            raise ContractError("unknown public schema definition")
        validator = ExactDraft202012Validator({"$ref": schema["$id"] + "#/$defs/" + name},
                                         registry=self.registry, format_checker=self.formats)
        error = next(validator.iter_errors(value), None)
        if error is not None:
            # Do not echo values (payloads can contain accidental secrets).
            path = "/".join(map(str, error.absolute_path)) or "$"
            raise ContractError(f"{name}: {error.validator} at {path}")

    def decode(self, name: str, raw: bytes | str, schema_file: str = "agent.schema.json"):
        value = strict_loads(raw)
        self.validate(name, value, schema_file)
        return value

    def validate_parameters(self, skill: dict, parameters: dict) -> None:
        schema = skill["parameters_schema"]
        Draft202012Validator.check_schema(schema)
        check_value(parameters)
        validator = ExactDraft202012Validator(schema, registry=self.registry, format_checker=self.formats)
        if next(validator.iter_errors(parameters), None) is not None:
            raise ContractError("skill parameter schema rejected values")

    def schema_reference(self, ref: dict) -> None:
        name, fragment = ref["schema_ref"].split("#", 1)
        if name not in self.schemas or byte_hash(self.raw[name]) != ref["file_sha256"]:
            raise ContractError("schema reference source digest mismatch")
        try:
            self.registry.resolver(self.schemas[name]["$id"]).lookup("#" + fragment)
        except Exception as error:
            raise ContractError("unknown schema reference target") from error
