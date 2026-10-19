package service

import (
	"bytes"
	"strconv"
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// repairChatCompletionMessageRole restores the assistant role omitted by some
// relays. Other role types and non-Chat response objects remain untouched.
func repairChatCompletionMessageRole(body []byte) []byte {
	if !gjson.ValidBytes(body) {
		return body
	}
	root := gjson.ParseBytes(body)
	if !root.IsObject() || root.Get("object").Type != gjson.String || root.Get("object").Str != "chat.completion" {
		return body
	}
	choices := root.Get("choices")
	if !choices.IsArray() {
		return body
	}
	out := body
	for i, choice := range choices.Array() {
		message := choice.Get("message")
		if !message.IsObject() {
			continue
		}
		role := message.Get("role")
		if role.Exists() && (role.Type != gjson.String || role.Str != "") {
			continue
		}
		if repaired, err := sjson.SetBytes(out, "choices."+strconv.Itoa(i)+".message.role", "assistant"); err == nil {
			out = repaired
		}
	}
	return out
}

// repairChatCompletionChunkRole only repairs an explicit empty role announcement.
// Empty terminal deltas and deltas carrying any other field are not announcements.
func repairChatCompletionChunkRole(payload []byte) []byte {
	if !gjson.ValidBytes(payload) {
		return payload
	}
	root := gjson.ParseBytes(payload)
	if !root.IsObject() || root.Get("object").Type != gjson.String || root.Get("object").Str != "chat.completion.chunk" {
		return payload
	}
	choices := root.Get("choices")
	if !choices.IsArray() {
		return payload
	}
	out := payload
	for i, choice := range choices.Array() {
		finish := choice.Get("finish_reason")
		if finish.Exists() && finish.Type != gjson.Null && (finish.Type != gjson.String || finish.Str != "") {
			continue
		}
		delta := choice.Get("delta")
		if !delta.IsObject() {
			continue
		}
		role := delta.Get("role")
		if role.Type != gjson.String || role.Str != "" {
			continue
		}
		onlyRole := true
		delta.ForEach(func(key, _ gjson.Result) bool {
			if key.Str != "role" {
				onlyRole = false
			}
			return onlyRole
		})
		if !onlyRole {
			continue
		}
		if repaired, err := sjson.SetBytes(out, "choices."+strconv.Itoa(i)+".delta.role", "assistant"); err == nil {
			out = repaired
		}
	}
	return out
}

// repairChatCompletionChunkSSELine preserves SSE whitespace and framing, including
// data fields without a space after the colon. It never searches inside text.
func repairChatCompletionChunkSSELine(line string) (string, bool) {
	const prefix = "data:"
	if !strings.HasPrefix(line, prefix) {
		return line, false
	}
	payload := []byte(line[len(prefix):])
	repaired := repairChatCompletionChunkRole(payload)
	if bytes.Equal(repaired, payload) {
		return line, false
	}
	return prefix + string(repaired), true
}
