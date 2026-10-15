package typesafe

import (
	"regexp"
	"strings"
)

// Audit metadata is not a provider response preview. Restrict returned model
// IDs to the format used by moderation engines, and never record an echoed
// credential or input even when it happens to be shaped like a model ID.
var moderationModelMetadataID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]*(/[A-Za-z0-9][A-Za-z0-9._:-]*)*$`)

func ValidModerationModelMetadata(model, input string, credentials ...string) bool {
	if len(model) > 128 || !moderationModelMetadataID.MatchString(model) {
		return false
	}
	// Providers conventionally lowercase IDs. Ordinary case normalization must
	// not hide an otherwise recognizable credential or private input echo.
	comparisonModel := strings.ToLower(model)
	for _, key := range credentials {
		comparisonKey := strings.ToLower(key)
		if comparisonKey != "" && (strings.Contains(comparisonModel, comparisonKey) || (len(model) >= 4 && strings.Contains(comparisonKey, comparisonModel))) {
			return false
		}
	}
	text := strings.ToLower(strings.TrimSpace(input))
	if text != "" && (comparisonModel == text || (len(text) >= 4 && strings.Contains(comparisonModel, text)) || (len(model) >= 4 && strings.Contains(text, comparisonModel))) {
		return false
	}
	// A returned ID may wrap one private identifier from a longer sentence,
	// rather than echoing the whole sentence. Compare identifier-shaped words
	// too; clearing ambiguous metadata must never discard valid audit scores.
	start := -1
	for i := 0; i <= len(text); i++ {
		identifierByte := i < len(text) && ((text[i] >= 'A' && text[i] <= 'Z') ||
			(text[i] >= 'a' && text[i] <= 'z') || (text[i] >= '0' && text[i] <= '9') ||
			strings.ContainsRune("._:-/", rune(text[i])))
		if identifierByte {
			if start < 0 {
				start = i
			}
			continue
		}
		if start >= 0 {
			fragment := strings.Trim(text[start:i], "._:-/")
			if len(fragment) >= 4 && strings.Contains(comparisonModel, fragment) {
				return false
			}
			start = -1
		}
	}
	return true
}

// Bad metadata must not turn a valid audit score into a failed-open request.
// An empty model honestly means the returned identifier could not be recorded.
func SafeModerationModelMetadata(model, input string, credentials ...string) string {
	if !ValidModerationModelMetadata(model, input, credentials...) {
		return ""
	}
	return model
}
