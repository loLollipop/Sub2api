package service

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRepairChatCompletionMessageRole(t *testing.T) {
	for _, tc := range []struct {
		name, body, want string
	}{
		{"empty", ` {"object":"chat.completion","choices":[{"message":{"role":"","content":"ok"}}]} `, ` {"object":"chat.completion","choices":[{"message":{"role":"assistant","content":"ok"}}]} `},
		{"missing", `{"object":"chat.completion","choices":[{"message":{"content":"ok"}}]}`, `{"object":"chat.completion","choices":[{"message":{"content":"ok","role":"assistant"}}]}`},
		{"multiple choices", `{"object":"chat.completion","choices":[{"message":{"role":""}},{"message":{"role":"user"}},{"message":{}}]}`, `{"object":"chat.completion","choices":[{"message":{"role":"assistant"}},{"message":{"role":"user"}},{"message":{"role":"assistant"}}]}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, string(repairChatCompletionMessageRole([]byte(tc.body))))
		})
	}
	for _, body := range []string{
		`{"object":"chat.completion","choices":[{"message":{"role":"assistant"}}]}`,
		`{"object":"chat.completion","choices":[{"message":{"role":"tool"}}]}`,
		`{"object":"chat.completion","choices":[{"message":{"role":null}}]}`,
		`{"object":"chat.completion","choices":[{"message":{"role":false}}]}`,
		`{"object":"chat.completion","choices":[{"message":{"role":0}}]}`,
		`{"object":"chat.completion","choices":[{"message":{"role":{}}}]}`,
		`{"object":"chat.completion","choices":[{"message":{"role":[]}}]}`,
		`{"object":"chat.completion","choices":[{"message":null},{"message":"text"}]}`,
		`{"object":"response","text":"chat.completion","choices":[{"message":{"role":""}}]}`,
		`{"text":"chat.completion","choices":[{"message":{"role":""}}]}`,
		`[{"object":"chat.completion","choices":[{"message":{"role":""}}]}]`,
		`{"object":"chat.completion.chunk","choices":[{"message":{"role":""}}]}`,
		`{"object":"chat.completion","choices":{}}`,
		`{"object":"chat.completion"`, "", "null",
	} {
		t.Run(body, func(t *testing.T) {
			require.Equal(t, body, string(repairChatCompletionMessageRole([]byte(body))))
		})
	}
}

func TestRepairChatCompletionChunkRole(t *testing.T) {
	for _, finish := range []string{"", `,"finish_reason":null`, `,"finish_reason":""`} {
		body := `{"object":"chat.completion.chunk","choices":[{"delta":{"role":""}` + finish + `}]}`
		require.Equal(t, strings.Replace(body, `"role":""`, `"role":"assistant"`, 1), string(repairChatCompletionChunkRole([]byte(body))))
	}
	for _, delta := range []string{
		`{}`, `null`, `"text"`, `{"role":"assistant"}`, `{"role":null}`, `{"role":false}`, `{"role":0}`, `{"role":{}}`,
		`{"role":"","content":""}`, `{"role":"","content":null}`, `{"role":"","content":"ok"}`,
		`{"role":"","reasoning_content":"thinking"}`, `{"role":"","reasoning":"thinking"}`,
		`{"role":"","tool_calls":[]}`, `{"role":"","refusal":null}`, `{"role":"","function_call":{}}`, `{"role":"","future":true}`,
	} {
		body := `{"object":"chat.completion.chunk","choices":[{"delta":` + delta + `}]}`
		t.Run(delta, func(t *testing.T) {
			require.Equal(t, body, string(repairChatCompletionChunkRole([]byte(body))))
		})
	}
	for _, body := range []string{
		`{"object":"chat.completion.chunk","choices":[{"delta":{"role":""},"finish_reason":"stop"}]}`,
		`{"object":"chat.completion.chunk","choices":[{"delta":{"role":""},"finish_reason":false}]}`,
		`{"object":"response","text":"chat.completion.chunk","choices":[{"delta":{"role":""}}]}`,
		`{"text":"chat.completion.chunk","choices":[{"delta":{"role":""}}]}`,
		`{"object":"chat.completion.chunk","choices":[],"usage":{"prompt_tokens":1}}`,
		`{"object":"chat.completion.chunk","choices":{}}`,
		`[{"object":"chat.completion.chunk","choices":[{"delta":{"role":""}}]}]`,
		`{"object":"chat.completion.chunk"`, "[DONE]", "", "null",
	} {
		require.Equal(t, body, string(repairChatCompletionChunkRole([]byte(body))))
	}
}

func TestRepairChatCompletionChunkSSELine(t *testing.T) {
	payload := `{"object":"chat.completion.chunk","choices":[{"delta":{"role":""}}]}`
	for _, prefix := range []string{"data:", "data: ", "data:  \t"} {
		line := prefix + payload + " \r\n"
		got, changed := repairChatCompletionChunkSSELine(line)
		require.True(t, changed)
		require.Equal(t, strings.Replace(line, `"role":""`, `"role":"assistant"`, 1), got)
	}
	for _, line := range []string{
		"data: [DONE]\r\n", "data:", "data: ", "event: " + payload,
		": data: " + payload, " data: " + payload,
		`data: {"object":"response","text":"chat.completion.chunk","choices":[{"delta":{"role":""}}]}`,
		`data: {"object":"chat.completion.chunk","choices":[{"delta":{"role":"","content":"data: example"}}]}`,
	} {
		got, changed := repairChatCompletionChunkSSELine(line)
		require.False(t, changed)
		require.Equal(t, line, got)
	}
}
