package antigravity

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMalformedFunctionCallTransform(t *testing.T) {
	for _, body := range []string{
		`{"candidates":[{"finishReason":"MALFORMED_FUNCTION_CALL","content":{"parts":[{"thoughtSignature":"test-signature"}]}}]}`,
		`{"response":{"candidates":[{"finishReason":"MALFORMED_FUNCTION_CALL"}]}}`,
	} {
		response, _, err := TransformGeminiToClaude([]byte(body), "test-model")
		if !errors.Is(err, ErrMalformedFunctionCall) || response != nil {
			t.Fatalf("expected malformed call error without successful response: %s %v", response, err)
		}
	}
}

func TestMalformedFunctionCallTransformRetainsUsage(t *testing.T) {
	body := []byte(`{"candidates":[{"finishReason":"MALFORMED_FUNCTION_CALL"}],"usageMetadata":{"promptTokenCount":17,"candidatesTokenCount":3,"thoughtsTokenCount":2,"cachedContentTokenCount":4}}`)
	response, usage, err := TransformGeminiToClaude(body, "test-model")
	require.ErrorIs(t, err, ErrMalformedFunctionCall)
	require.Nil(t, response)
	require.NotNil(t, usage)
	require.Equal(t, 13, usage.InputTokens)
	require.Equal(t, 5, usage.OutputTokens)
	require.Equal(t, 4, usage.CacheReadInputTokens)
}
