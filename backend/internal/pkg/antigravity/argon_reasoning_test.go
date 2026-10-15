package antigravity

import "testing"

// Source: Wei-Shaw/sub2api 373d43533.
func TestIsGeminiReasoningModel_IncludesProAgent(t *testing.T) {
	t.Parallel()
	for _, model := range []string{"gemini-pro-agent", "gemini-3.8-flash-high", "gemini-4-argon", "gemini-4-argon-high"} {
		if !IsGeminiReasoningModel(model) {
			t.Errorf("expected %q to be recognized as a Gemini reasoning model", model)
		}
	}
	if IsGeminiReasoningModel("gemini-2.5-flash") {
		t.Fatal("did not expect gemini-2.5-flash to be recognized as a Gemini reasoning model")
	}
}
