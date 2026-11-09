package service

import (
	"strconv"
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// DeepSeek requires thinking on replayed assistant tool turns. Preserve real
// and redacted thinking, and use a single space only for missing/empty slots.
func ensureDeepSeekThinkingReplayPlaceholders(body []byte, model string) ([]byte, bool) {
	if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(model)), "deepseek-") ||
		strings.EqualFold(strings.TrimSpace(gjson.GetBytes(body, "thinking.type").String()), "disabled") {
		return body, false
	}
	messages := gjson.GetBytes(body, "messages")
	if !messages.IsArray() {
		return body, false
	}
	updated := body
	changed := false
	for i, message := range messages.Array() {
		if message.Get("role").String() != "assistant" {
			continue
		}
		content := message.Get("content")
		if !content.IsArray() {
			continue
		}
		hasTool, hasThinking, preserveThinking := false, false, false
		for _, block := range content.Array() {
			switch block.Get("type").String() {
			case "tool_use":
				hasTool = true
			case "redacted_thinking":
				preserveThinking = true
			case "thinking":
				hasThinking = true
				text := block.Get("thinking")
				if text.Type != gjson.Null && (text.Type != gjson.String || text.String() != "") {
					preserveThinking = true
				}
			}
		}
		if !hasTool || preserveThinking {
			continue
		}
		path := "messages." + strconv.Itoa(i) + ".content"
		if hasThinking {
			for j, block := range content.Array() {
				if block.Get("type").String() != "thinking" {
					continue
				}
				next, err := sjson.SetBytes(updated, path+"."+strconv.Itoa(j)+".thinking", deepSeekReasoningPlaceholderText)
				if err != nil {
					return body, false
				}
				updated = next
			}
		} else {
			rewritten := `[{"type":"thinking","thinking":" "},` + strings.TrimPrefix(content.Raw, "[")
			next, err := sjson.SetRawBytes(updated, path, []byte(rewritten))
			if err != nil {
				return body, false
			}
			updated = next
		}
		changed = true
	}
	return updated, changed
}
