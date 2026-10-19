package service

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const openAIResponsesWebSearchPreviewType = "web_search_preview"

// normalizeOpenAIResponsesLegacyWebSearchToolType folds the retired Responses
// tool name into web_search without rebuilding input, options or numeric values.
func normalizeOpenAIResponsesLegacyWebSearchToolType(body []byte) ([]byte, bool, error) {
	// Unicode escapes can also encode the legacy name or its field names.
	if len(body) == 0 || (!bytes.Contains(body, []byte(openAIResponsesWebSearchPreviewType)) && !bytes.Contains(body, []byte(`\u`))) {
		return body, false, nil
	}
	if !gjson.ValidBytes(body) {
		return body, false, fmt.Errorf("normalize legacy web search tool: invalid JSON")
	}
	out := body
	changed := false
	if tools := gjson.GetBytes(body, "tools"); tools.IsArray() {
		for i, tool := range tools.Array() {
			if !tool.IsObject() || !isLegacyWebSearchPreviewType(tool.Get("type")) {
				continue
			}
			next, err := sjson.SetBytes(out, "tools."+strconv.Itoa(i)+".type", "web_search")
			if err != nil {
				return body, false, err
			}
			out, changed = next, true
		}
	}
	if choice := gjson.GetBytes(body, "tool_choice"); choice.IsObject() && isLegacyWebSearchPreviewType(choice.Get("type")) {
		next, err := sjson.SetBytes(out, "tool_choice.type", "web_search")
		if err != nil {
			return body, false, err
		}
		out, changed = next, true
	}
	return out, changed, nil
}

func isLegacyWebSearchPreviewType(value gjson.Result) bool {
	if value.Type != gjson.String {
		return false
	}
	toolType := value.String()
	if toolType == openAIResponsesWebSearchPreviewType {
		return true
	}
	date, dated := strings.CutPrefix(toolType, openAIResponsesWebSearchPreviewType+"_")
	if !dated || len(date) != len("2006_01_02") {
		return false
	}
	_, err := time.Parse("2006_01_02", date)
	return err == nil
}
