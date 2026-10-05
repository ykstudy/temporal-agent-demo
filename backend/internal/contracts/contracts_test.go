package contracts

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func readTestFile(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func snapshotObject(t *testing.T) map[string]any {
	t.Helper()
	value, err := ParseJSON(readTestFile(t, "definition-snapshot.valid.json"))
	if err != nil {
		t.Fatal(err)
	}
	return value.(map[string]any)
}

func TestPublishedDefinitionSnapshotRoundTrip(t *testing.T) {
	data := readTestFile(t, "definition-snapshot.valid.json")
	if err := ValidateJSON("DefinitionSnapshot", data); err != nil {
		t.Fatal(err)
	}
	var snapshot DefinitionSnapshot
	if err := Decode("DefinitionSnapshot", data, &snapshot); err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateJSON("DefinitionSnapshot", encoded); err != nil {
		t.Fatal(err)
	}
	a, _ := Canonicalize(data)
	b, _ := Canonicalize(encoded)
	if !bytes.Equal(a, b) {
		t.Fatal("generated types lost or changed published fields")
	}
	for name, value := range map[string]any{"DefinitionSnapshot": snapshot, "AgentDefinition": snapshot.Definition} {
		actual, err := HashObject(name, value)
		if err != nil {
			t.Fatal(err)
		}
		expected := snapshot.SnapshotHash
		if name == "AgentDefinition" {
			expected = snapshot.Definition.DefinitionHash
		}
		if actual != expected {
			t.Fatalf("independent fixture %s hash: got %s want %s", name, actual, expected)
		}
	}
	for _, skill := range snapshot.Skills {
		if err := Validate("SkillDefinition", skill); err != nil {
			t.Fatal(err)
		}
		hash, err := HashObject("SkillDefinition", skill)
		if err != nil || hash != skill.SkillHash {
			t.Fatalf("independent skill hash mismatch %s: %v", skill.SkillID, err)
		}
		if err := ValidateDocument(skill.ParametersSchema, json.RawMessage(skill.DefaultParameters)); err != nil {
			t.Fatal(err)
		}
	}
}

func TestDefinitionRequiredAndClosedFields(t *testing.T) {
	base := snapshotObject(t)
	definition := base["definition"].(map[string]any)
	data, _ := json.Marshal(definition)
	if err := ValidateJSON("AgentDefinition", data); err != nil {
		t.Fatal(err)
	}
	for field := range definition {
		t.Run("missing_"+field, func(t *testing.T) {
			copy, _ := ParseJSON(data)
			delete(copy.(map[string]any), field)
			if err := Validate("AgentDefinition", copy); err == nil {
				t.Fatalf("missing required %s accepted", field)
			}
		})
	}
	for _, name := range []string{"DefinitionSnapshot", "AgentDefinition", "SkillDefinition"} {
		t.Run("unknown_"+name, func(t *testing.T) {
			var obj map[string]any
			switch name {
			case "DefinitionSnapshot":
				obj = snapshotObject(t)
			case "AgentDefinition":
				obj = snapshotObject(t)["definition"].(map[string]any)
			case "SkillDefinition":
				obj = snapshotObject(t)["skills"].([]any)[0].(map[string]any)
			}
			obj["unpublished"] = true
			if err := Validate(name, obj); err == nil {
				t.Fatal("unknown field accepted")
			}
		})
	}
}

func TestFullSchemaKeywordsAndFormats(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(map[string]any)
	}{
		{"bool_version", func(v map[string]any) { v["snapshot_version"] = true }},
		{"date", func(v map[string]any) { v["bound_at"] = "2026-02-30T00:00:00Z" }},
		{"not_utc", func(v map[string]any) { v["bound_at"] = "2026-10-02T00:00:00+08:00" }},
		{"unsafe_exponent_version", func(v map[string]any) { v["snapshot_version"] = json.Number("9.007199254740992e15") }},
		{"extra_skill", func(v map[string]any) { v["skills"] = append(v["skills"].([]any), v["skills"].([]any)[0]) }},
		{"invalid_union", func(v map[string]any) {
			v["skills"].([]any)[0].(map[string]any)["default_parameters"] = map[string]any{"auth": "jwt", "database": "postgres"}
		}},
		{"wrong_skill_branch", func(v map[string]any) { v["skills"].([]any)[0].(map[string]any)["skill_id"] = "add_logging" }},
		{"path_traversal", func(v map[string]any) {
			v["template_dependencies"].([]any)[0].(map[string]any)["files"].([]any)[0].(map[string]any)["path"] = "../secret"
		}},
		{"content_ref_type", func(v map[string]any) {
			v["template_dependencies"].([]any)[0].(map[string]any)["files"].([]any)[0].(map[string]any)["content_ref"].(map[string]any)["object_type"] = "template"
		}},
		{"legacy_contract", func(v map[string]any) { v["contract_version"] = "1.0.0" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			value := snapshotObject(t)
			tc.mutate(value)
			if err := Validate("DefinitionSnapshot", value); err == nil {
				t.Fatal("invalid contract accepted")
			}
		})
	}
	if err := ValidateDocument(json.RawMessage(`{"$ref":"https://example.invalid/remote.json"}`), true); err == nil {
		t.Fatal("remote schema reference accepted")
	}
	if err := ValidateDocument(json.RawMessage(`{"$schema":"https://json-schema.org/draft/2020-12/schema","type":"string","format":"date-time"}`), "2026-02-30T00:00:00Z"); err == nil {
		t.Fatal("format assertion ignored")
	}
	if err := ValidateDocument(json.RawMessage(`{"type":"object","allOf":[{"properties":{"x":{"type":"string"}}}],"unevaluatedProperties":false}`), map[string]any{"x": "ok", "y": true}); err == nil {
		t.Fatal("unevaluatedProperties ignored")
	}
}

func TestStrictJSONRejectsInvalidInput(t *testing.T) {
	for _, input := range []string{`{"x":1,"x":2}`, `{"x":1,"\u0078":2}`, `{"x":{"a":1,"a":2}}`, `[NaN]`, `[Infinity]`, `[1e999]`, `[9007199254740992]`, `[-9007199254740992]`, `"\ud800"`, `"\udead"`, `"\ud800\u0041"`, `"\udead\ud800"`, `{} {}`, `[1,]`, "\"bad\ntext\"", "\"\xff\"", strings.Repeat("[", MaxJSONDepth+2) + "0" + strings.Repeat("]", MaxJSONDepth+2)} {
		if _, err := ParseJSON([]byte(input)); err == nil {
			t.Errorf("invalid input accepted: %q", input)
		}
	}
	if _, err := ParseJSON(bytes.Repeat([]byte(" "), MaxJSONBytes+1)); err == nil {
		t.Fatal("byte limit not enforced")
	}
	for _, input := range []string{"[1e-999999999]", "[" + strings.Repeat("1", MaxJSONNumberBytes+1) + "]"} {
		if _, err := ParseJSON([]byte(input)); err == nil {
			t.Fatal("number resource limit not enforced")
		}
	}
	for _, input := range []string{`{"x":"\ud83d\ude00"}`, `{"\\ud800":"literal escape"}`, `[9007199254740991,-9007199254740991]`, `[]`, `null`} {
		if _, err := ParseJSON([]byte(input)); err != nil {
			t.Fatalf("valid input rejected: %v", err)
		}
	}
}

func TestDecodePreservesSchemaIntegerSemantics(t *testing.T) {
	var version PositiveVersion
	for _, input := range []string{"1", "1.0", "1e0", "9007199254740991.0"} {
		if err := Decode("PositiveVersion", []byte(input), &version); err != nil {
			t.Fatalf("valid integer %s rejected: %v", input, err)
		}
	}
	for _, input := range []string{"1.00000000000000000001", "9007199254740991.1", "9007199254740992.0", "9.007199254740992e15"} {
		if err := Decode("PositiveVersion", []byte(input), &version); err == nil {
			t.Fatalf("non-integer or unsafe exact number accepted: %s", input)
		}
	}
	if _, err := HashValue(map[string]any{"invalid": "\xff"}); err == nil {
		t.Fatal("invalid Go UTF-8 repaired before hashing")
	}
}

func TestIndependentJCSVectors(t *testing.T) {
	var vectors struct {
		Positive []struct {
			ID        string `json:"id"`
			Input     string `json:"input_json"`
			Canonical string `json:"expected_canonical"`
			Hash      string `json:"expected_sha256"`
		} `json:"positive"`
		Negative []struct {
			ID    string `json:"id"`
			Input string `json:"input_json"`
		} `json:"strict_json_negative"`
	}
	if err := json.Unmarshal(readTestFile(t, "jcs-vectors.json"), &vectors); err != nil {
		t.Fatal(err)
	}
	for _, v := range vectors.Positive {
		t.Run(v.ID, func(t *testing.T) {
			actual, err := Canonicalize([]byte(v.Input))
			if err != nil {
				t.Fatal(err)
			}
			if string(actual) != v.Canonical || BytesHash(actual) != v.Hash {
				t.Fatalf("independent RFC8785 vector mismatch: %s", actual)
			}
		})
	}
	for _, v := range vectors.Negative {
		t.Run(v.ID, func(t *testing.T) {
			if _, err := Canonicalize([]byte(v.Input)); err == nil {
				t.Fatal("strict JSON negative accepted")
			}
		})
	}
	for _, v := range []struct{ input, want string }{{`-0.0`, `0`}, {`"<>&"`, `"<>&"`}, {`true`, `true`}, {`null`, `null`}} {
		actual, err := Canonicalize([]byte(v.input))
		if err != nil || string(actual) != v.want {
			t.Fatalf("scalar JCS failed %s: %s %v", v.input, actual, err)
		}
	}
}

func TestHashPoliciesPreserveNestedContentAndLifecycleExclusions(t *testing.T) {
	base := map[string]any{"snapshot_hash": "first", "status": "pending", "nested": map[string]any{"hash": "child"}, "items": []any{"a", "b"}}
	hash, err := HashObject("Proposal", base)
	if err != nil {
		t.Fatal(err)
	}
	base["snapshot_hash"] = "other"
	base["status"] = "accepted"
	same, _ := HashObject("Proposal", base)
	if same != hash {
		t.Fatal("own hash or lifecycle field changed proposal identity")
	}
	base["nested"].(map[string]any)["hash"] = "changed"
	different, _ := HashObject("Proposal", base)
	if different == hash {
		t.Fatal("nested hash was incorrectly excluded")
	}
	base["nested"].(map[string]any)["hash"] = "child"
	base["items"] = []any{"b", "a"}
	different, _ = HashObject("Proposal", base)
	if different == hash {
		t.Fatal("array order was silently normalized")
	}
	if _, err := HashObject("Unpublished", base); err == nil {
		t.Fatal("arbitrary hash exclusion policy accepted")
	}
	if BytesHash([]byte("file\n")) == BytesHash([]byte("file")) {
		t.Fatal("raw byte hash normalized content")
	}
	without := map[string]any{"x": 1}
	withNull := map[string]any{"x": 1, "y": nil}
	h1, _ := HashValue(without)
	h2, _ := HashValue(withNull)
	if h1 == h2 {
		t.Fatal("null and absent conflated")
	}
}

func TestGeneratedReleaseMatchesSource(t *testing.T) {
	root := filepath.Join("..", "..", "..")
	for file, data := range SchemaFiles() {
		source, err := os.ReadFile(filepath.Join(root, "design", "contracts", "releases", ContractVersion, file))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(data, source) || BytesHash(data) != SchemaDigests()[file] {
			t.Fatalf("derived schema drift: %s", file)
		}
	}
	cmd := exec.Command("python3", filepath.Join(root, "scripts", "generate-contracts.py"), "--check")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("nonreproducible generation: %v\n%s", err, output)
	}
	files := SchemaFiles()
	files["agent.schema.json"][0] = 'x'
	if reflect.DeepEqual(files, SchemaFiles()) {
		t.Fatal("SchemaFiles exposes shared mutable bytes")
	}
}
