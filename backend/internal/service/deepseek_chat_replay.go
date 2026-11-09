package service

import (
	"net/url"
	"strconv"
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// Prepare only the selected DeepSeek Chat endpoint. Keep raw messages so vendor
// fields and exact numeric values survive tool-pair repair.
func prepareDeepSeekChatReplay(account *Account, targetURL string, body []byte) []byte {
	endpoint, err := url.Parse(targetURL)
	if account.Platform != PlatformDeepseek && (err != nil || !strings.EqualFold(endpoint.Hostname(), "api.deepseek.com")) {
		return body
	}
	body = normalizeDeepSeekChatMessages(body)
	if gjson.GetBytes(body, "thinking.type").String() == "disabled" {
		return body
	}
	for i, message := range gjson.GetBytes(body, "messages").Array() {
		if message.Get("role").String() != "assistant" {
			continue
		}
		reasoning := message.Get("reasoning_content")
		if reasoning.Exists() && reasoning.Type != gjson.Null && (reasoning.Type != gjson.String || reasoning.String() != "") {
			continue
		}
		if updated, err := sjson.SetBytes(body, "messages."+strconv.Itoa(i)+".reasoning_content", deepSeekReasoningPlaceholderText); err == nil {
			body = updated
		}
	}
	return body
}

// Replies belong to the nearest preceding announcement of their ID. In
// particular, a repeated call ID must not borrow a result from a later turn.
func normalizeDeepSeekChatMessages(body []byte) []byte {
	array := gjson.GetBytes(body, "messages")
	if !array.IsArray() {
		return body
	}
	messages := array.Array()
	owners := make(map[string]int)
	replies := make(map[int]map[string]string)
	for i, message := range messages {
		switch message.Get("role").String() {
		case "assistant":
			for _, call := range message.Get("tool_calls").Array() {
				if id := call.Get("id").String(); id != "" {
					owners[id] = i
				}
			}
		case "tool":
			id := message.Get("tool_call_id").String()
			if owner, ok := owners[id]; id != "" && ok {
				if replies[owner] == nil {
					replies[owner] = make(map[string]string)
				}
				if _, exists := replies[owner][id]; !exists {
					replies[owner][id] = message.Raw
				}
			}
		}
	}
	kept := make([]string, 0, len(messages))
	for i, message := range messages {
		if message.Get("role").String() == "tool" {
			continue
		}
		calls := message.Get("tool_calls")
		if message.Get("role").String() != "assistant" || !calls.IsArray() || len(calls.Array()) == 0 {
			kept = append(kept, message.Raw)
			continue
		}
		answered, outputs := make([]string, 0), make([]string, 0)
		for _, call := range calls.Array() {
			id := call.Get("id").String()
			if reply, ok := replies[i][id]; ok {
				answered = append(answered, call.Raw)
				outputs = append(outputs, reply)
				delete(replies[i], id)
			}
		}
		raw := []byte(message.Raw)
		var err error
		if len(answered) == 0 {
			content := message.Get("content")
			if content.String() == "" || content.Type == gjson.Null || content.Raw == "[]" {
				continue
			}
			raw, err = sjson.DeleteBytes(raw, "tool_calls")
		} else if len(answered) != len(calls.Array()) {
			raw, err = sjson.SetRawBytes(raw, "tool_calls", []byte("["+strings.Join(answered, ",")+"]"))
		}
		if err != nil {
			return body
		}
		kept = append(kept, string(raw))
		kept = append(kept, outputs...)
	}
	// Leave already valid requests byte-for-byte intact.
	if len(kept) == len(messages) {
		unchanged := true
		for i := range messages {
			unchanged = unchanged && kept[i] == messages[i].Raw
		}
		if unchanged {
			return body
		}
	}
	if updated, err := sjson.SetRawBytes(body, "messages", []byte("["+strings.Join(kept, ",")+"]")); err == nil {
		return updated
	}
	return body
}
