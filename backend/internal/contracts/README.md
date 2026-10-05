# Published runtime contracts

This package embeds releases `1.1.0` and `1.2.0`, wire `schema_version=2`, with
`1.2.0` as the current publication. Immutable sources are the corresponding
`design/contracts/releases/<version>/` directories. Runtime code reads only embedded assets;
it does not read `design/_model`, run Python, or load external schema references.
Release `1.0.0` remains unchanged on disk and is not silently upgraded by this package.

From the repository root:

```sh
python3 scripts/generate-contracts.py
python3 scripts/generate-contracts.py --check
```

The generator derives every published `$defs` type, nested object and concrete
union object branch. Unions retain their JSON bytes using named `json.RawMessage`
types with JSON methods, so the wire value is never a base64 string or a reduced
union of optional fields. The generated file and `schemas/release.json` register
their exact schema locations and per-release source digests. Inherited `allOf`
object fields are resolved through aliases, including the complete five-field
typed object references. Do not edit generated files.

Use `Decode(name, bytes, &value)` for untrusted wire input. It first rejects invalid
JSON and then applies the entire published Schema, before Go decoding can lose
unknown or missing fields. `ValidateJSON` validates raw bytes; `Validate` validates
already constructed values; `ValidateDocument` validates Skill parameters against
the actual frozen parameter Schema. Direct `encoding/json.Unmarshal` into a struct
is not a contract boundary. Nullable required fields have no `omitempty` tag.
Exact integer forms such as `1.0` and `1e0` are accepted and normalized for Go
integer decoding; fractional values are checked before any binary64 rounding.

`Decode`, `Validate` and `ValidateJSON` select a root object's declared
`contract_version`; unsupported versions fail closed. A public child value with
no version uses the active release. Call `DecodeVersion(name, version, bytes,
&value)`, `ValidateVersion(name, version, value)` or
`ValidateJSONVersion(name, version, bytes)` when its enclosing object supplies
the version. Each release has its own offline compiler. Nested old objects in a
new root fail rather than being upgraded, even though generated union decoders
can preserve either supported release after full parent-boundary validation.
`SchemaFilesFor(version)` and `SchemaDigestsFor(version)` return owned maps plus
an error for unknown versions. `SchemaRefFor(name, version)` resolves that exact
release; `ValidateDocumentVersion(version, schema, value)` validates frozen Skill
parameters within it. Old fixture bytes and old object hashes remain unchanged.

The validator is [jsonschema/v6](https://github.com/santhosh-tekuri/jsonschema),
Draft 2020-12, with explicit format assertions and an offline loader. The published
path patterns use lookahead, so the validator uses the documented ECMAScript
`regexp2` adapter with a bounded match timeout. JSON input limits are 16 MiB,
128 nested containers, 500,000 values, 128 bytes per numeric literal, and exponent
magnitude 400. Nonfinite numbers, unsafe integer literals, malformed UTF-8,
unpaired surrogate escapes, duplicate decoded keys and trailing data are rejected.

`Canonicalize` uses the [RFC 8785 author's Go implementation](https://github.com/cyberphone/json-canonicalization/tree/master/go),
with the same strict input checks. `HashObject` applies only the published top-level
exclusion list. It preserves child hashes, nulls, and array order; the owning domain
must first normalize defined sets and reject duplicate identities. `HashValue`
hashes an entire ordered context/target/receipt; `BytesHash` hashes exact source
bytes. Hash generation does not prove Schema validity or authorization: compute
an object's own hash, validate its completed structure, then check its domain
bindings and permissions before using it.

Tests include an independently generated DefinitionSnapshot, all-field round-trip,
real content identities, definition required/unknown fields, keyword and date
assertions, precision-sensitive integer negatives, offline reference refusal and
shared Go/Python RFC 8785 vectors. Test fixture provenance is in
`testdata/provenance.json`; production code never imports or reads validation.
S2 fixtures are synthetic complete contract objects with independently computed
RFC8785 hashes, not claims of actual tool execution. They cover budget state
conditions, receipt command/argv/status conditions, full inherited references,
nullable initial permissions and cross-release refusal.

```sh
cd backend
go test -race ./internal/contracts
go vet ./internal/contracts
go build ./internal/contracts
```
