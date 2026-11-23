package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/antigravity"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestImagesCompletedEventKeepsPrettyPrintedUsageOnOneLine(t *testing.T) {
	c, rec := newOpenAIImagesTestContext(t, nil)
	payload := buildOpenAIImagesStreamCompletedPayload("image_generation.completed", openAIResponsesImageResult{Result: "aGVsbG8="}, "b64_json", 1710000000, []byte("{\n  \"input_tokens\":10,\n  \"output_tokens\":20\n}"))
	require.NoError(t, (&OpenAIGatewayService{}).writeOpenAIImagesStreamEvent(c, rec, "image_generation.completed", payload))
	var data string
	for _, line := range strings.Split(strings.TrimRight(rec.Body.String(), "\n"), "\n") {
		require.True(t, line == "" || strings.HasPrefix(line, "event: ") || strings.HasPrefix(line, "data: "), "split SSE payload: %q", line)
		if strings.HasPrefix(line, "data: ") {
			data = strings.TrimPrefix(line, "data: ")
		}
	}
	require.True(t, gjson.Valid(data), data)
	require.Equal(t, "aGVsbG8=", gjson.Get(data, "b64_json").String())
	require.Equal(t, int64(20), gjson.Get(data, "usage.output_tokens").Int())
}

func TestCreateTestPayloadTemperaturePreservesCustomPrompt(t *testing.T) {
	for _, tc := range []struct {
		model string
		want  bool
	}{
		{"claude-sonnet-4-6", true},
		{"claude-opus-4-7", false},
		{"claude-opus-4-8", false},
		{"claude-opus-5-5", false},
		{"claude-fable-5-2", false},
		{"claude-mythos-5-2", false},
	} {
		t.Run(tc.model, func(t *testing.T) {
			payload, err := createTestPayload(tc.model, " custom prompt ")
			require.NoError(t, err)
			_, present := payload["temperature"]
			require.Equal(t, tc.want, present)
			require.Equal(t, 8192, payload["max_tokens"])
			messages, ok := payload["messages"].([]map[string]any)
			require.True(t, ok)
			require.Len(t, messages, 1)
			content, ok := messages[0]["content"].([]map[string]any)
			require.True(t, ok)
			require.Len(t, content, 1)
			require.Equal(t, "custom prompt", content[0]["text"])
		})
	}
}

func TestFetchQuotaUsesAccountForwardEndpoint(t *testing.T) {
	for _, plan := range []string{"Free", "Pro", "Ultra"} {
		t.Run(plan, func(t *testing.T) {
			t.Setenv(antigravityForwardBaseURLEnv, "")
			makeEndpoint := func(remaining string) *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					if r.URL.Path == "/v1internal:fetchAvailableModels" {
						_, _ = w.Write([]byte(`{"models":{"gemini-3-flash":{"quotaInfo":{"remainingFraction":` + remaining + `,"resetTime":"2026-11-23T09:00:00Z"}}}}`))
						return
					}
					_, _ = w.Write([]byte(`{"paidTier":{"id":"g1-ultra-tier"}}`))
				}))
			}
			prod, daily := makeEndpoint("0.75"), makeEndpoint("0.25")
			defer prod.Close()
			defer daily.Close()
			oldURLs, oldAvailability := antigravity.BaseURLs, antigravity.DefaultURLAvailability
			t.Cleanup(func() {
				antigravity.BaseURLs, antigravity.DefaultURLAvailability = oldURLs, oldAvailability
			})
			antigravity.BaseURLs = []string{prod.URL, daily.URL}
			antigravity.DefaultURLAvailability = antigravity.NewURLAvailability(time.Minute)
			result, err := NewAntigravityQuotaFetcher(nil, &config.Config{}).FetchQuota(context.Background(), &Account{
				Platform: PlatformAntigravity, Type: AccountTypeOAuth,
				Credentials: map[string]any{"access_token": "fixture", "project_id": "project", "plan_type": plan},
			}, "")
			require.NoError(t, err)
			want := 75
			if plan == "Free" {
				want = 25
			}
			require.Equal(t, want, result.UsageInfo.AntigravityQuota["gemini-3-flash"].Utilization)
		})
	}
}
