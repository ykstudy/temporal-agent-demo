package contracts

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func s2Fixtures(t *testing.T) map[string]json.RawMessage {
	t.Helper()
	var fixtures map[string]json.RawMessage
	if err := json.Unmarshal(readTestFile(t, "s2-objects.valid.json"), &fixtures); err != nil {
		t.Fatal(err)
	}
	return fixtures
}

func TestSupportedReleaseIsolationAndOriginalBytes(t *testing.T) {
	if !reflect.DeepEqual(SupportedContractVersions(), []string{"1.1.0", "1.2.0", "1.3.0", "1.4.0"}) {
		t.Fatal("unexpected supported release identities")
	}
	for _, version := range SupportedContractVersions() {
		files, err := SchemaFilesFor(version)
		if err != nil {
			t.Fatal(err)
		}
		hashes, err := SchemaDigestsFor(version)
		if err != nil {
			t.Fatal(err)
		}
		for file, data := range files {
			source, err := os.ReadFile(filepath.Join("..", "..", "..", "design", "contracts", "releases", version, file))
			if err != nil || !bytes.Equal(data, source) || BytesHash(data) != hashes[file] {
				t.Fatalf("published %s/%s bytes changed: %v", version, file, err)
			}
		}
		files["agent.schema.json"][0] = 'x'
		hashes["agent.schema.json"] = "changed"
		otherFiles, _ := SchemaFilesFor(version)
		otherHashes, _ := SchemaDigestsFor(version)
		if reflect.DeepEqual(files, otherFiles) || reflect.DeepEqual(hashes, otherHashes) {
			t.Fatal("versioned source access exposes mutable internals")
		}
	}
	old := readTestFile(t, "definition-snapshot.valid.json")
	var snapshot DefinitionSnapshot
	if err := DecodeVersion("DefinitionSnapshot", "1.1.0", old, &snapshot); err != nil {
		t.Fatal(err)
	}
	if err := ValidateJSONVersion("DefinitionSnapshot", "1.2.0", old); err == nil {
		t.Fatal("old snapshot accepted as the new contract")
	}
	if err := ValidateJSON("DefinitionSnapshot", old); err != nil {
		t.Fatalf("declared old release was not respected: %v", err)
	}
	for _, version := range []string{"1.0.0", "1.5.0", "", "../../1.1.0"} {
		if _, err := SchemaFilesFor(version); err == nil {
			t.Fatalf("unpublished release accepted: %q", version)
		}
		if err := ValidateJSONVersion("DefinitionSnapshot", version, old); err == nil {
			t.Fatal("unavailable version silently fell back")
		}
	}
	snapshot.ContractVersion = "1.2.0"
	if err := Validate("DefinitionSnapshot", snapshot); err == nil {
		t.Fatal("mixed release closure accepted")
	}
	snapshot.ContractVersion = "unknown"
	if err := Validate("DefinitionSnapshot", snapshot); err == nil {
		t.Fatal("unknown declared release silently upgraded")
	}
	if _, err := SchemaRefFor("BudgetLedger", "1.1.0"); err == nil {
		t.Fatal("new object exposed under old release identity")
	}
	oldRef, err := SchemaRefFor("ProjectPlan", "1.1.0")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = SchemaRefFor(oldRef, "1.2.0"); err == nil {
		t.Fatal("cross-release Schema reference accepted")
	}
	if err = ValidateDocumentVersion("1.2.0", map[string]any{"$ref": oldRef}, map[string]any{}); err == nil {
		t.Fatal("parameter compiler resolved another release")
	}
}

func TestS2FullGeneratedObjectRoundTrips(t *testing.T) {
	fixtures := s2Fixtures(t)
	types := map[string]any{
		"BudgetReservation": BudgetReservation{}, "BudgetSettlement": BudgetSettlement{}, "BudgetWindow": BudgetWindow{},
		"BudgetCounters": BudgetCounters{}, "BudgetTotals": BudgetTotals{}, "BudgetLedger": BudgetLedger{},
		"ContextMessage": ContextMessage{}, "ContextRecord": ContextRecord{}, "ToolExecutionReceipt": ToolExecutionReceipt{},
		"WorkspaceReceipt": WorkspaceReceipt{}, "S2AuditBundle": S2AuditBundle{},
	}
	hashFields := map[string]string{
		"BudgetReservation": "reservation_hash", "BudgetSettlement": "settlement_hash", "BudgetLedger": "ledger_hash",
		"ContextRecord": "context_hash", "ToolExecutionReceipt": "receipt_hash", "WorkspaceReceipt": "receipt_hash", "S2AuditBundle": "audit_hash",
	}
	for name, value := range types {
		t.Run(name, func(t *testing.T) {
			data := fixtures[name]
			destination := reflect.New(reflect.TypeOf(value)).Interface()
			if err := DecodeVersion(name, "1.2.0", data, destination); err != nil {
				t.Fatal(err)
			}
			encoded, err := json.Marshal(destination)
			if err != nil {
				t.Fatal(err)
			}
			before, _ := Canonicalize(data)
			after, _ := Canonicalize(encoded)
			if !bytes.Equal(before, after) {
				t.Fatalf("generated %s lost a field or changed a nullable/union value", name)
			}
			if err = ValidateVersion(name, "1.2.0", destination); err != nil {
				t.Fatal(err)
			}
			parsed, _ := ParseJSON(data)
			object := parsed.(map[string]any)
			if field := hashFields[name]; field != "" {
				hash, err := HashObject(name, destination)
				if err != nil || hash != object[field] {
					t.Fatalf("independent Python %s hash differs: %v", name, err)
				}
				object[field] = strings.Repeat("f", 64)
				same, _ := HashObject(name, object)
				if same != hash {
					t.Fatal("own hash was included")
				}
				// These new objects have no excluded lifecycle fields.
				object["run_id"] = "different_run"
				changed, _ := HashObject(name, object)
				if changed == hash {
					t.Fatal("binding data was excluded from the hash")
				}
			}
			parsed, _ = ParseJSON(data)
			object = parsed.(map[string]any)
			for field := range object {
				copy, _ := ParseJSON(data)
				delete(copy.(map[string]any), field)
				if err = ValidateVersion(name, "1.2.0", copy); err == nil {
					t.Fatalf("missing required %s accepted", field)
				}
			}
			object["unpublished"] = true
			if err = ValidateVersion(name, "1.2.0", object); err == nil {
				t.Fatal("unknown field accepted")
			}
		})
	}
}

func TestS2BudgetAndReceiptConditions(t *testing.T) {
	fixtures := s2Fixtures(t)
	for _, tc := range []struct {
		name, object string
		mutate       func(map[string]any)
	}{
		{"reserve_zero", "BudgetReservation", func(v map[string]any) { v["units"] = 0 }},
		{"action_missing_request", "BudgetReservation", func(v map[string]any) { v["kind"] = "action" }},
		{"command_unrelated_request", "BudgetReservation", func(v map[string]any) { v["request_hash"] = strings.Repeat("0", 64) }},
		{"unknown_cannot_release", "BudgetSettlement", func(v map[string]any) { v["outcome"], v["consumed_units"] = "unknown", 0 }},
		{"release_cannot_consume", "BudgetSettlement", func(v map[string]any) { v["outcome"] = "released" }},
		{"actual_missing_measurement", "BudgetSettlement", func(v map[string]any) { v["elapsed_ms"] = nil }},
		{"unknown_fabricated_zero", "BudgetSettlement", func(v map[string]any) { v["measurement_source"], v["elapsed_ms"] = "unknown", 0 }},
		{"unstarted_deadline", "BudgetLedger", func(v map[string]any) { v["execution_deadline_at"] = "2026-10-02T12:00:00Z" }},
		{"wrong_window_kind", "BudgetLedger", func(v map[string]any) {
			window, _ := ParseJSON(fixtures["BudgetWindow"])
			v["reconciliation_windows"] = []any{window}
		}},
		{"context_non_message_source", "ContextRecord", func(v map[string]any) { v["input_message_refs"].([]any)[0].(map[string]any)["object_type"] = "plan" }},
		{"wrong_fixed_arguments", "ToolExecutionReceipt", func(v map[string]any) { v["arguments"] = []string{"build", "./..."} }},
		{"success_nonzero_exit", "ToolExecutionReceipt", func(v map[string]any) { v["exit_code"] = 1 }},
		{"failed_zero_exit", "ToolExecutionReceipt", func(v map[string]any) { v["status"] = "failed" }},
		{"unknown_zero_exit", "ToolExecutionReceipt", func(v map[string]any) { v["status"] = "unknown" }},
		{"manifest_wrong_type", "ToolExecutionReceipt", func(v map[string]any) { v["input_manifest_ref"].(map[string]any)["object_type"] = "plan" }},
		{"oversized_output", "ToolExecutionReceipt", func(v map[string]any) { v["output"] = strings.Repeat("x", 65537) }},
		{"workspace_success_missing_manifest", "WorkspaceReceipt", func(v map[string]any) { v["output_manifest_ref"] = nil }},
		{"plan_without_permissions", "S2AuditBundle", func(v map[string]any) { v["state"] = "PLAN" }},
		{"mixed_snapshot_release", "S2AuditBundle", func(v map[string]any) { v["definition_snapshot"].(map[string]any)["contract_version"] = "1.1.0" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			value, _ := ParseJSON(fixtures[tc.object])
			tc.mutate(value.(map[string]any))
			if err := ValidateVersion(tc.object, "1.2.0", value); err == nil {
				t.Fatal("invalid contract condition accepted")
			}
		})
	}
}

func TestAliasInheritedObjectReferencesPreserveAllFiveFields(t *testing.T) {
	// The published allOf uses ObjectRef aliases across files. Resolving only
	// its local object_type discriminator silently loses the other four fields.
	data := []byte(`{"command_id":"accept_1","action":"accept","target":{"object_type":"run","object_id":"run_1","object_version":2,"source_version":1,"hash":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},"required_proposal_ref":{"object_type":"proposal","object_id":"proposal_1","object_version":1,"source_version":1,"hash":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}}`)
	for _, version := range SupportedContractVersions() {
		var command ActionIntent
		if err := DecodeVersion("ActionIntent", version, data, &command); err != nil {
			t.Fatal(err)
		}
		encoded, err := json.Marshal(command)
		if err != nil {
			t.Fatal(err)
		}
		want, _ := Canonicalize(data)
		got, _ := Canonicalize(encoded)
		if !bytes.Equal(want, got) {
			t.Fatal("aliased reference fields were lost")
		}
	}
}
