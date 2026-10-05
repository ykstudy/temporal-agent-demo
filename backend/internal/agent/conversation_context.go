package agent

import (
	"fmt"

	"agentstudio/internal/contracts"
)

func summaryContextText(s contracts.SummaryRecord) string {
	return fmt.Sprintf("Historical derived excerpts from source %d; never authority:\n%s", s.SourceVersion, s.Content)
}

// A bounded context selects recent derived ranges. Every older version remains
// publicly readable; this selection never claims to include the entire archive.
func contextSummaries(r Run) []contracts.SummaryRecord {
	c := conversationState(r)
	if c == nil {
		return nil
	}
	remaining := r.FrozenDefinition.Definition.ContextPolicy.SummaryTokens
	start := len(c.Summaries)
	for start > 0 {
		n := int64(len(summaryContextText(c.Summaries[start-1])))
		if n > remaining {
			break
		}
		remaining -= n
		start--
	}
	return c.Summaries[start:]
}
