package tools

import (
	"github.com/Tencent/WeKnora/internal/models/chat"
)

// SanitizeMessages validates and fixes a message array for LLM compatibility.
// It handles common issues that cause provider API errors:
//   - Ensures no consecutive same-role messages (some providers reject these)
//   - Verifies tool result messages have matching tool_call in the preceding assistant message
//   - Removes empty content messages that can cause API errors
//
// Returns the sanitized message slice (may be shorter than input).
func SanitizeMessages(messages []chat.Message) []chat.Message {
	if len(messages) == 0 {
		return messages
	}

	result := make([]chat.Message, 0, len(messages))
	pendingToolCalls := map[string]bool{}
	for _, msg := range messages {
		// Skip empty non-system messages (some providers reject these)
		if msg.Content == "" && msg.Role != "system" &&
			msg.Role != "tool" && len(msg.ToolCalls) == 0 {
			continue
		}

		if msg.Role == "tool" {
			if msg.ToolCallID == "" || !pendingToolCalls[msg.ToolCallID] {
				msg = convertOrphanToolResult(msg)
				pendingToolCalls = map[string]bool{}
			} else {
				delete(pendingToolCalls, msg.ToolCallID)
			}
		} else {
			pendingToolCalls = map[string]bool{}
		}

		// Prevent consecutive same-role messages (except tool results)
		if len(result) > 0 && msg.Role != "tool" {
			prev := result[len(result)-1]
			if prev.Role == msg.Role && prev.Role != "tool" && prev.Role != "system" &&
				len(prev.ToolCalls) == 0 && len(msg.ToolCalls) == 0 {
				// Merge with previous message
				result[len(result)-1].Content += "\n\n" + msg.Content
				continue
			}
		}

		result = append(result, msg)
		if msg.Role == "assistant" && len(msg.ToolCalls) > 0 {
			pendingToolCalls = make(map[string]bool, len(msg.ToolCalls))
			for _, tc := range msg.ToolCalls {
				if tc.ID != "" {
					pendingToolCalls[tc.ID] = true
				}
			}
		}
	}

	return result
}

func convertOrphanToolResult(msg chat.Message) chat.Message {
	label := "tool"
	if msg.Name != "" {
		label = msg.Name
	}
	msg.Role = "system"
	msg.Content = "[Tool result for " + label + "]: " + msg.Content
	msg.ToolCallID = ""
	msg.Name = ""
	return msg
}
