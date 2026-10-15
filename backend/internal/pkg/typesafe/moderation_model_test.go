package typesafe

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidModerationModelMetadata(t *testing.T) {
	for _, model := range []string{"jev-1.13.0", "omni-moderation-latest", "openai/omni-moderation-latest", "meta-llama/Llama-Guard-3-8B", "guard:latest"} {
		require.True(t, ValidModerationModelMetadata(model, "ordinary sample", "private-key"), model)
	}
	for _, model := range []string{"", "private-key", "jev-private-key-version", "private-input", "jev-private-input-version", "https://private.example/path", "/private/path", "openai//omni", "model with spaces", strings.Repeat("m", 129)} {
		require.False(t, ValidModerationModelMetadata(model, "private-input", "private-key"), model)
	}
	require.False(t, ValidModerationModelMetadata("customer-ssn-123456789", "customer-ssn-123456789 and account review"))
	require.Empty(t, SafeModerationModelMetadata("customer-ssn-123456789", "customer-ssn-123456789 and account review"))
	for _, input := range []string{"customer-ssn-123456789 and account review", "review: customer-ssn-123456789, please", "review customer-ssn-123456789.", "请审核customer-ssn-123456789谢谢"} {
		require.Empty(t, SafeModerationModelMetadata("jev-customer-ssn-123456789-version", input))
	}
	for _, model := range []string{"private-input", "jev-private-input-version", "PRIVATE-INPUT", "jev-PRIVATE-INPUT-version"} {
		require.Empty(t, SafeModerationModelMetadata(model, "Private-Input"))
		require.Empty(t, SafeModerationModelMetadata(model, "a review for Private-Input today"))
	}
	require.Empty(t, SafeModerationModelMetadata("jev-private-key-version", "ordinary sample", "Private-Key"))
	require.Empty(t, SafeModerationModelMetadata("jev-customer-ssn-123456789-version", "Customer-SSN-123456789 and account review"))
}
