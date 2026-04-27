package utils

import (
	"strings"
	"testing"
)

func TestCompactImageDataURLForLog_RedactsReasoningContent(t *testing.T) {
	raw := `{
		"messages": [
			{
				"role": "assistant",
				"content": "visible",
				"reasoning_content": "private chain of thought"
			}
		]
	}`

	got := CompactImageDataURLForLog(raw)
	if strings.Contains(got, "private chain of thought") {
		t.Fatalf("reasoning_content leaked into log output: %s", got)
	}
	if !strings.Contains(got, redactedReasoningContentMarker) {
		t.Fatalf("redaction marker missing: %s", got)
	}
}
