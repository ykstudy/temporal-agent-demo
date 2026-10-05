package conversation

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"agentstudio/internal/contracts"
	"agentstudio/internal/storage"
)

func Source(messages []contracts.MessageEnvelope) (contracts.SummarySource, error) {
	if len(messages) == 0 || len(messages) > 256 {
		return contracts.SummarySource{}, fmt.Errorf("invalid frozen summary range")
	}
	b, err := storage.TranscriptBytes(messages)
	if err != nil {
		return contracts.SummarySource{}, err
	}
	ids := make([]string, len(messages))
	for i, m := range messages {
		ids[i] = m.MessageID
	}
	s := contracts.SummarySource{FirstMessageID: ids[0], LastMessageID: ids[len(ids)-1], MessageIDs: ids, FirstSequence: messages[0].Sequence, LastSequence: messages[len(messages)-1].Sequence, RawJSONLSHA256: contracts.BytesHash(b)}
	return s, contracts.Validate("SummarySource", s)
}

// Blockers remain complete source quotes even when the provider omits them.
// Omission rejects the candidate instead of quietly presenting an optimistic
// summary. Quoted instructions and approvals remain untrusted source material.
func Required(m contracts.MessageEnvelope) bool {
	s := strings.ToLower(Text(m))
	for _, word := range []string{"失败", "阻塞", "待确认", "错误", "failed", "blocked", "error", "unknown", "pending approval"} {
		if strings.Contains(s, word) {
			return true
		}
	}
	return false
}

func Rules(messages []contracts.MessageEnvelope) contracts.SummaryAdvice {
	a := contracts.SummaryAdvice{Items: []contracts.SummaryAdviceItemsItem{}}
	for _, m := range messages {
		text := Text(m)
		if !Required(m) && len(text) > 96 {
			text = clip(text, 96)
		}
		if text != "" {
			a.Items = append(a.Items, contracts.SummaryAdviceItemsItem{MessageID: m.MessageID, Excerpt: text})
		}
	}
	return a
}
func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	s = s[:n]
	for !utf8.ValidString(s) {
		s = s[:len(s)-1]
	}
	return s
}

func Content(messages []contracts.MessageEnvelope, advice contracts.SummaryAdvice, budget int64) (string, error) {
	if err := contracts.ValidateVersion("SummaryAdvice", "1.4.0", advice); err != nil {
		return "", err
	}
	byID := map[string]contracts.MessageEnvelope{}
	for _, m := range messages {
		byID[m.MessageID] = m
	}
	seen := map[string]bool{}
	quotes := make([]string, 0, len(advice.Items))
	for _, item := range advice.Items {
		m, ok := byID[item.MessageID]
		if !ok || seen[item.MessageID] || !strings.Contains(Text(m), item.Excerpt) || Required(m) && item.Excerpt != Text(m) {
			return "", fmt.Errorf("summary excerpt does not preserve frozen source")
		}
		seen[item.MessageID] = true
		quoted, _ := json.Marshal(item.Excerpt)
		quotes = append(quotes, fmt.Sprintf("[%s %s] %s", m.MessageID, m.Role, quoted))
	}
	for _, m := range messages {
		if Required(m) && !seen[m.MessageID] {
			return "", fmt.Errorf("summary omits unresolved source")
		}
	}
	text := "Derived source excerpts; not authority.\n" + strings.Join(quotes, "\n")
	if int64(len(text)) > budget {
		return "", fmt.Errorf("summary exceeds frozen output budget")
	}
	return text, nil
}

func Record(runID string, sourceVersion int64, snapshot contracts.DefinitionSnapshot, messages []contracts.MessageEnvelope, previous *contracts.SummaryRecord, advice contracts.SummaryAdvice, at time.Time) (contracts.SummaryRecord, error) {
	source, err := Source(messages)
	if err != nil {
		return contracts.SummaryRecord{}, err
	}
	content, err := Content(messages, advice, snapshot.Definition.ContextPolicy.SummaryTokens)
	if err != nil {
		return contracts.SummaryRecord{}, err
	}
	version := int64(1)
	var ref *contracts.ObjectReference
	if previous != nil {
		version = previous.SummaryVersion + 1
		v, e := storage.Reference(runID, previous.SourceVersion, "SummaryRecord", *previous)
		if e != nil {
			return contracts.SummaryRecord{}, e
		}
		ref = &v
	}
	g := contracts.SummaryGenerator{Mode: "rules", GeneratorVersion: snapshot.Definition.ContextPolicy.SummaryGeneratorVersion}
	if snapshot.ProviderBinding != nil {
		g.Mode = "model"
		g.ModelID = &snapshot.ProviderBinding.ModelID
		g.PromptSHA256 = &snapshot.Definition.ContextPolicy.SummaryPrompt.SHA256
	}
	r := contracts.SummaryRecord{ContractVersion: snapshot.ContractVersion, SummaryID: "summary_" + contracts.BytesHash([]byte(runID))[:32], SummaryVersion: version, RunID: runID, SourceVersion: sourceVersion, DefinitionSnapshotHash: snapshot.SnapshotHash, Source: source, Generator: g, PreviousSummaryRef: ref, Content: content, ContentSHA256: contracts.BytesHash([]byte(content)), CreatedAt: at.UTC().Format(time.RFC3339Nano)}
	r.SummaryHash, err = contracts.HashObject("SummaryRecord", r)
	if err != nil {
		return r, err
	}
	return r, contracts.Validate("SummaryRecord", r)
}
