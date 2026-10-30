package service

import (
	"bytes"
	"encoding/json"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const responsesMissingToolOutput = `{"error":"tool_output_missing","message":"The client did not provide an output for this tool call; treat it as interrupted and continue."}`

// repairOpenAIResponsesMissingToolOutputs repairs interrupted client tool history
// without executing tools or changing any client item. Call only after native
// Responses routing, outside compact requests. OpenAI stored continuations may
// resolve history server-side; native DeepSeek drops previous_response_id and
// therefore must be repaired as a self-contained request even if it contains one.
func repairOpenAIResponsesMissingToolOutputs(body []byte, allowStoredContinuation bool) ([]byte, bool, error) {
	if !gjson.ValidBytes(body) {
		return body, false, nil
	}
	if allowStoredContinuation && gjson.GetBytes(body, "previous_response_id").String() != "" {
		return body, false, nil
	}
	input := gjson.GetBytes(body, "input")
	if !input.IsArray() {
		return body, false, nil
	}
	items := input.Array()
	type outputKey struct{ kind, callID string }
	// Match each output only to a preceding call of the same type and string ID.
	// An old output or one from another tool kind cannot answer a later call.
	unanswered := make(map[outputKey][]int)
	missing := make(map[int]outputKey)
	for i, item := range items {
		kind, callID := item.Get("type"), item.Get("call_id")
		if kind.Str == "local_shell_call_output" {
			// Native local shell outputs use id to reference the call_id.
			callID = item.Get("id")
		}
		if kind.Type != gjson.String || callID.Type != gjson.String || callID.Str == "" {
			continue
		}
		if outputType := responsesToolOutputType(kind.Str); outputType != "" {
			key := outputKey{outputType, callID.Str}
			unanswered[key] = append(unanswered[key], i)
			missing[i] = key
		} else {
			key := outputKey{kind.Str, callID.Str}
			if queue := unanswered[key]; len(queue) != 0 {
				delete(missing, queue[0])
				unanswered[key] = queue[1:]
			}
		}
	}
	if len(missing) == 0 {
		return body, false, nil
	}

	var repaired bytes.Buffer
	repaired.Grow(len(input.Raw))
	_ = repaired.WriteByte('[')
	first := true
	appendItem := func(raw string) {
		if !first {
			_ = repaired.WriteByte(',')
		}
		first = false
		_, _ = repaired.WriteString(raw)
	}
	pending := make([]outputKey, 0, len(missing))
	flush := func() error {
		for _, key := range pending {
			output := map[string]string{"type": key.kind, "output": responsesMissingToolOutput}
			if key.kind == "local_shell_call_output" {
				output["id"] = key.callID
			} else {
				output["call_id"] = key.callID
			}
			raw, err := json.Marshal(output)
			if err != nil {
				return err
			}
			appendItem(string(raw))
		}
		pending = pending[:0]
		return nil
	}
	for i, item := range items {
		kind := item.Get("type").String()
		if responsesToolOutputType(kind) == "" && !isResponsesClientToolOutput(kind) {
			if err := flush(); err != nil {
				return body, false, err
			}
		}
		// Preserve original item bytes, including unknown fields, namespaces,
		// nested media outputs, large integers and argument formatting.
		appendItem(item.Raw)
		if key, ok := missing[i]; ok {
			pending = append(pending, key)
		}
	}
	if err := flush(); err != nil {
		return body, false, err
	}
	_ = repaired.WriteByte(']')
	next, err := sjson.SetRawBytes(body, "input", repaired.Bytes())
	if err != nil {
		return body, false, err
	}
	return next, true, nil
}

func responsesToolOutputType(callType string) string {
	switch callType {
	case "function_call":
		return "function_call_output"
	case "custom_tool_call":
		return "custom_tool_call_output"
	case "local_shell_call":
		return "local_shell_call_output"
	default:
		// Tool search output requires a tools array, not an arbitrary output
		// string. Preserve an interrupted search's explicit upstream error
		// rather than silently reporting a successful empty search.
		return ""
	}
}

func isResponsesClientToolOutput(kind string) bool {
	switch kind {
	case "function_call_output", "custom_tool_call_output", "local_shell_call_output", "tool_search_output":
		return true
	default:
		return false
	}
}
