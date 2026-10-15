package service

import (
	"github.com/Wei-Shaw/sub2api/internal/pkg/claude"
	"github.com/tidwall/gjson"
	"net/http"
	"strings"
)

// The legacy streaming beta is incompatible with the stable computer and
// browser toolsets. The per-tool eager_input_streaming field remains available.
// Applies to Sonnet 5.5 and Haiku 5.5, which share the computer_toolset_20260801
// / browser_toolset_20260801 toolsets (Haiku 5.5 migration guide).
func filterSonnet55ToolsetBeta(header string, body []byte, modelID string) string {
	if at := strings.IndexByte(modelID, '@'); at >= 0 {
		modelID = modelID[:at] // Vertex's dated model ID
	}
	if !claude.IsSonnet55(modelID) && !claude.IsHaiku55(modelID) {
		return header
	}
	for _, tool := range gjson.GetBytes(body, "tools").Array() {
		switch tool.Get("type").String() {
		case "computer_toolset_20260801", "browser_toolset_20260801":
			if !containsBetaToken(header, claude.BetaFineGrainedToolStreaming) {
				return header
			}
			return stripBetaTokensWithSet(header, map[string]struct{}{claude.BetaFineGrainedToolStreaming: {}})
		}
	}
	return header
}

func filterSonnet55ToolsetBetaHeader(headers http.Header, body []byte, modelID string) {
	header := getHeaderRaw(headers, "anthropic-beta")
	filtered := filterSonnet55ToolsetBeta(header, body, modelID)
	if filtered == header {
		return
	}
	deleteHeaderAllForms(headers, "anthropic-beta")
	if filtered != "" {
		setHeaderRaw(headers, "anthropic-beta", filtered)
	}
}
