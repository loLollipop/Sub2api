package service

import (
	"strconv"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const deepSeekReasoningPlaceholderText = " "

// Only absent content or explicitly empty reasoning_text belongs to the replay
// compatibility path. Unknown vendor content must survive unchanged.
func emptyDeepSeekNativeReasoningContent(content gjson.Result) bool {
	if !content.Exists() || content.Type == gjson.Null {
		return true
	}
	if !content.IsArray() {
		return false
	}
	for _, part := range content.Array() {
		text := part.Get("text")
		if part.Get("type").String() != "reasoning_text" || (text.Type != gjson.Null && (text.Type != gjson.String || text.String() != "")) {
			return false
		}
	}
	return true
}

// Real scoped reasoning is restored first. A cache miss supplies only the
// single space accepted by DeepSeek; summaries never become reasoning text.
func ensureDeepSeekNativeReasoningPlaceholders(account *Account, body []byte) []byte {
	if account == nil || account.Platform != PlatformDeepseek || !account.UsesNativeCNResponses() {
		return body
	}
	input := gjson.GetBytes(body, "input")
	if !input.IsArray() {
		return body
	}
	updated := body
	for i, item := range input.Array() {
		content := item.Get("content")
		if item.Get("type").String() != "reasoning" || !emptyDeepSeekNativeReasoningContent(content) {
			continue
		}
		path := "input." + strconv.Itoa(i) + ".content"
		var next []byte
		var err error
		if parts := content.Array(); len(parts) > 0 {
			next = updated
			for j := range parts {
				next, err = sjson.SetBytes(next, path+"."+strconv.Itoa(j)+".text", deepSeekReasoningPlaceholderText)
				if err != nil {
					return body
				}
			}
		} else {
			next, err = sjson.SetRawBytes(updated, path, []byte(`[{"type":"reasoning_text","text":" "}]`))
		}
		if err != nil {
			return body
		}
		updated = next
	}
	return updated
}
