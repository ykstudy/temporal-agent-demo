package contracts

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/cyberphone/json-canonicalization/go/src/webpki.org/jsoncanonicalizer"
)

// Canonicalize implements RFC 8785 after strict input checks. The upstream
// implementation accepts object/array roots; wrapping scalars extends that exact
// algorithm without creating a second numeric/string serializer.
func Canonicalize(data []byte) ([]byte, error) {
	if _, err := ParseJSON(data); err != nil {
		return nil, err
	}
	trimmed := bytes.TrimSpace(data)
	if trimmed[0] == '{' || trimmed[0] == '[' {
		return jsoncanonicalizer.Transform(trimmed)
	}
	wrapped := append(append([]byte{'['}, trimmed...), ']')
	canonical, err := jsoncanonicalizer.Transform(wrapped)
	if err != nil {
		return nil, err
	}
	return append([]byte(nil), canonical[1:len(canonical)-1]...), nil
}

func BytesHash(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// These are the exact top-level exclusions in AGENT-CONTRACTS.md. Callers cannot
// supply their own list or strip nested hashes. Collection ordering belongs to
// the owning object's normalization step and is never silently changed here.
var objectHashFields = map[string][]string{
	"RuntimeAuditEvent":       {"event_hash"},
	"RuntimeAuditTrail":       {"audit_hash"},
	"RuntimeView":             {"view_hash"},
	"RuntimeHealth":           {"health_hash"},
	"RuntimeServiceMetrics":   {"metrics_hash"},
	"AgentDefinition":         {"definition_hash"},
	"SkillDefinition":         {"skill_hash"},
	"DefinitionSnapshot":      {"snapshot_hash"},
	"ProjectPlan":             {"plan_hash"},
	"DecisionProposal":        {"decision_hash"},
	"ActionValidation":        {"validation_hash", "execution_request"},
	"ExecutionRequest":        {"request_hash"},
	"Proposal":                {"snapshot_hash", "status"},
	"Approval":                {"approval_hash", "status", "started_at", "revoked_at"},
	"PermissionSnapshot":      {"permission_hash"},
	"ArtifactManifest":        {"hash"},
	"VerificationReport":      {"report_hash"},
	"SummaryRecord":           {"summary_hash"},
	"RunSnapshot":             {"snapshot_hash"},
	"TemplateDependency":      {"manifest_hash"},
	"MessageEnvelope":         {},
	"BudgetReservation":       {"reservation_hash"},
	"BudgetSettlement":        {"settlement_hash"},
	"BudgetLedger":            {"ledger_hash"},
	"ContextRecord":           {"context_hash"},
	"ToolExecutionReceipt":    {"receipt_hash"},
	"WorkspaceReceipt":        {"receipt_hash"},
	"S2AuditBundle":           {"audit_hash"},
	"RemoteExecutionReceipt":  {"receipt_hash"},
	"PackageReceipt":          {"receipt_hash"},
	"VerificationCandidate":   {"candidate_hash"},
	"SummaryAttempt":          {"attempt_hash"},
	"ConversationAuditBundle": {"audit_hash"},
}

// HashObject computes the prescribed logical identity; it does not imply Schema
// validity. This permits building an object's own hash before final validation.
func HashObject(name string, value any) (string, error) {
	exclusions, ok := objectHashFields[name]
	if !ok {
		return "", fmt.Errorf("no published hash policy for object type")
	}
	data, err := jsonBytes(value)
	if err != nil {
		return "", err
	}
	decoded, err := ParseJSON(data)
	if err != nil {
		return "", err
	}
	object, ok := decoded.(map[string]any)
	if !ok {
		return "", fmt.Errorf("object hash requires a JSON object")
	}
	for _, field := range exclusions {
		delete(object, field)
	}
	payload, err := json.Marshal(object)
	if err != nil {
		return "", fmt.Errorf("invalid object hash input")
	}
	canonical, err := Canonicalize(payload)
	if err != nil {
		return "", err
	}
	return BytesHash(canonical), nil
}

// HashValue covers an entire ordered context/target/receipt value with no field
// exclusions. It cannot be used as a substitute for HashObject's published policy.
func HashValue(value any) (string, error) {
	data, err := jsonBytes(value)
	if err != nil {
		return "", err
	}
	canonical, err := Canonicalize(data)
	if err != nil {
		return "", err
	}
	return BytesHash(canonical), nil
}
