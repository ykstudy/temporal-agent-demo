// Package conversation validates immutable messages and derived summaries.
// It never supplies execution authority or reads the design model.
package conversation

import (
	"encoding/json"
	"fmt"
	"strings"

	"agentstudio/internal/contracts"
)

func Text(m contracts.MessageEnvelope) string {
	parts := make([]string, 0, len(m.Parts))
	seen := map[string]bool{}
	for _, raw := range m.Parts {
		var p struct {
			Type     string `json:"type"`
			Text     string `json:"text"`
			Fallback string `json:"fallback_text"`
		}
		if json.Unmarshal(raw, &p) != nil {
			continue
		}
		text := p.Fallback
		if p.Type == "text" {
			text = p.Text
		}
		if !seen[text] {
			parts = append(parts, text)
			seen[text] = true
		}
	}
	return strings.Join(parts, "\n")
}

// Resolve must check the full reference in this message's own Run namespace.
func ValidateMessage(m contracts.MessageEnvelope, resolve func(contracts.ObjectReference) bool) error {
	if err := contracts.Validate("MessageEnvelope", m); err != nil {
		return err
	}
	body, err := json.Marshal(m)
	if err != nil || len(body) > 256<<10 {
		return fmt.Errorf("message exceeds resource bound")
	}
	ids := map[string]bool{}
	for _, raw := range m.Parts {
		v, err := contracts.ParseJSON(raw)
		if err != nil {
			return err
		}
		p := v.(map[string]any)
		id := p["part_id"].(string)
		if ids[id] {
			return fmt.Errorf("duplicate part identity")
		}
		ids[id] = true
		if p["type"] == "component" {
			kind := p["component_type"].(string)
			if !knownComponent(kind) {
				b, _ := json.Marshal(p["payload"])
				if len(b) > 64<<10 || depth(p["payload"]) > 8 {
					return fmt.Errorf("unknown component exceeds read-only bound")
				}
			}
		}
		if err := walkRefs(v, func(ref contracts.ObjectReference) bool {
			return ref.SourceVersion <= m.SourceVersion && (ref.ObjectType != "run" || ref.ObjectVersion <= m.StateSequence) && resolve != nil && resolve(ref)
		}); err != nil {
			return err
		}
	}
	return nil
}

func knownComponent(s string) bool {
	switch s {
	case "clarification", "proposal_diff", "progress", "artifact_manifest", "verification_report":
		return true
	}
	return false
}
func depth(v any) int {
	max := 0
	switch x := v.(type) {
	case map[string]any:
		for _, child := range x {
			if n := depth(child); n > max {
				max = n
			}
		}
	case []any:
		for _, child := range x {
			if n := depth(child); n > max {
				max = n
			}
		}
	default:
		return 0
	}
	return max + 1
}
func walkRefs(v any, resolve func(contracts.ObjectReference) bool) error {
	switch x := v.(type) {
	case map[string]any:
		_, kind := x["object_type"]
		_, identity := x["object_id"]
		_, version := x["object_version"]
		_, source := x["source_version"]
		_, hash := x["hash"]
		if kind && identity && version && source && hash {
			b, _ := json.Marshal(x)
			var ref contracts.ObjectReference
			if err := contracts.Decode("ObjectReference", b, &ref); err != nil {
				return err
			}
			if resolve == nil || !resolve(ref) {
				return fmt.Errorf("message references unavailable exact historical object")
			}
			return nil
		}
		for _, child := range x {
			if err := walkRefs(child, resolve); err != nil {
				return err
			}
		}
	case []any:
		for _, child := range x {
			if err := walkRefs(child, resolve); err != nil {
				return err
			}
		}
	}
	return nil
}
