package utils

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
)

var (
	imageDataURLPatternForLog     = regexp.MustCompile(`data:image\/[a-zA-Z0-9.+-]+;base64,[A-Za-z0-9+/=]+`)
	reasoningContentPatternForLog = regexp.MustCompile(`(?is)("reasoning_content"\s*:\s*")((?:\\.|[^"\\])*)(")`)
)

const (
	defaultMaxLogChars             = 12000
	defaultMaxDataURLPreview       = 96
	redactedReasoningContentMarker = "REDACTED_REASONING_CONTENT"
)

// CompactImageDataURLForLog shortens large image data URLs for log output.
func CompactImageDataURLForLog(raw string) string {
	masked := redactReasoningContentForLog(raw)
	masked = imageDataURLPatternForLog.ReplaceAllStringFunc(masked, func(match string) string {
		if len(match) <= defaultMaxDataURLPreview {
			return match
		}
		hidden := len(match) - defaultMaxDataURLPreview
		return match[:defaultMaxDataURLPreview] + "...<omitted " + strconv.Itoa(hidden) + " chars>"
	})

	if len(masked) <= defaultMaxLogChars {
		return masked
	}
	return masked[:defaultMaxLogChars] + "... (truncated, total " + strconv.Itoa(len(masked)) + " chars)"
}

func redactReasoningContentForLog(raw string) string {
	if strings.TrimSpace(raw) == "" {
		return raw
	}

	var payload any
	if err := json.Unmarshal([]byte(raw), &payload); err == nil {
		redactReasoningContentValue(payload)
		if encoded, err := json.MarshalIndent(payload, "", "  "); err == nil {
			return string(encoded)
		}
	}

	return reasoningContentPatternForLog.ReplaceAllString(raw, `${1}`+redactedReasoningContentMarker+`${3}`)
}

func redactReasoningContentValue(v any) {
	switch value := v.(type) {
	case map[string]any:
		for key, child := range value {
			if strings.EqualFold(key, "reasoning_content") {
				value[key] = redactedReasoningContentMarker
				continue
			}
			redactReasoningContentValue(child)
		}
	case []any:
		for _, child := range value {
			redactReasoningContentValue(child)
		}
	}
}
