//go:build unit

package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func imagesRegressionResponse(body string, status int) *http.Response {
	return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}
}

func TestImagesRegressionChannelZeroImageCache(t *testing.T) {
	pricing := &ModelPricing{InputPricePerToken: 5e-6, ImageInputPricePerToken: 8e-6, CacheReadPricePerToken: 1.25e-6, ImageCacheReadPricePerToken: 2e-6}
	zero := 0.0
	r := &ModelPricingResolver{}
	resolved := &ResolvedPricing{BasePricing: pricing}
	r.applyTokenOverrides(&ChannelModelPricing{CacheReadPrice: &zero}, resolved)
	cost := (&BillingService{}).computeTokenBreakdown(resolved.BasePricing, UsageTokens{CacheReadTokens: 100, ImageCacheReadTokens: 100}, 1, "", false)
	require.Zero(t, cost.CacheReadCost, "channel cache_read_price=0 must make all cache reads free")
}

func TestImagesRegressionAPIKeyImageCachedUsage(t *testing.T) {
	raw := []byte(`{"model":"gpt-image-2.6-flare","prompt":"draw"}`)
	c, _ := newOpenAIImagesTestContext(t, raw)
	u := &httpUpstreamRecorder{resp: imagesRegressionResponse(`{"data":[{"b64_json":"aA=="}],"usage":{"input_tokens":100,"output_tokens":10,"input_tokens_details":{"image_tokens":60,"cached_tokens":40,"cached_tokens_details":{"image_tokens":30,"text_tokens":10}}}}`, 200)}
	s := newOpenAIImagesTestService(u)
	p, e := s.ParseOpenAIImagesRequest(c, raw)
	require.NoError(t, e)
	out, e := s.ForwardImages(context.Background(), c, newOpenAIImagesAPIKeyAccount(), raw, p, "")
	require.NoError(t, e)
	require.Equal(t, 30, out.Usage.ImageCacheReadTokens)
}

func TestImagesRegressionDirectTerminalClearsImageCache(t *testing.T) {
	raw := []byte(`{"model":"gpt-image-2.6-flare","prompt":"draw","stream":true}`)
	c, _ := newOpenAIImagesTestContext(t, raw)
	events := "data: {\"type\":\"image_generation.partial_image\",\"b64_json\":\"cA==\",\"usage\":{\"input_tokens\":100,\"output_tokens\":4,\"input_tokens_details\":{\"image_tokens\":60,\"cached_tokens\":40,\"cached_tokens_details\":{\"image_tokens\":30,\"text_tokens\":10}}}}\n\n" +
		"data: {\"type\":\"image_generation.completed\",\"b64_json\":\"aA==\",\"usage\":{\"input_tokens\":100,\"output_tokens\":10,\"input_tokens_details\":{\"image_tokens\":60,\"cached_tokens\":0,\"cached_tokens_details\":{\"image_tokens\":0,\"text_tokens\":0}}}}\n\n"
	resp := imagesRegressionResponse(events, 200)
	resp.Header.Set("Content-Type", "text/event-stream")
	s := newOpenAIImagesTestService(&httpUpstreamRecorder{resp: resp})
	p, e := s.ParseOpenAIImagesRequest(c, raw)
	require.NoError(t, e)
	out, e := s.ForwardImages(context.Background(), c, newDirectImagesOAuthAccount(), raw, p, "")
	require.NoError(t, e)
	require.Zero(t, out.Usage.CacheReadInputTokens)
	require.Zero(t, out.Usage.ImageCacheReadTokens)
}

func TestImagesRegressionDirectNonStreamUserErrorWritesResponse(t *testing.T) {
	raw := []byte(`{"model":"gpt-image-2.6-flare","prompt":"draw"}`)
	c, rec := newOpenAIImagesTestContext(t, raw)
	s := newOpenAIImagesTestService(&httpUpstreamRecorder{resp: imagesRegressionResponse(`{"error":{"type":"invalid_request_error","code":"content_policy_violation","message":"blocked by policy"}}`, 200)})
	p, e := s.ParseOpenAIImagesRequest(c, raw)
	require.NoError(t, e)
	_, e = s.ForwardImages(context.Background(), c, newDirectImagesOAuthAccount(), raw, p, "")
	var up *OpenAIImagesUpstreamError
	require.True(t, errors.As(e, &up))
	require.False(t, IsOpenAIImagesRetryableUpstreamError(up))
	require.Equal(t, 400, rec.Code)
	require.Contains(t, rec.Body.String(), "blocked by policy")
}

func TestImagesRegressionGIFFractionNotSilentlyTruncated(t *testing.T) {
	for _, field := range []string{"frame_count", "frame_duration_ms"} {
		for _, value := range []string{"2.9", "120.5", "1e100", "9223372036854775808", "-1", `"2.9"`, `"120.5"`, `"1e100"`, `"9223372036854775808"`, `"-1"`} {
			raw := []byte(fmt.Sprintf(`{"model":"gpt-image-2.6-flare","prompt":"draw","output_format":"gif",%q:%s}`, field, value))
			c, _ := newOpenAIImagesTestContext(t, raw)
			_, err := (&OpenAIGatewayService{}).ParseOpenAIImagesRequest(c, raw)
			require.Error(t, err, "%s=%s must not be truncated", field, value)
		}
	}
	for _, fields := range []string{
		`"frame_count":2e1,"frame_duration_ms":1.2e2`,
		`"frame_count":"20","frame_duration_ms":"120"`,
	} {
		raw := []byte(`{"model":"gpt-image-2.6-flare","prompt":"draw",` + fields + `}`)
		c, _ := newOpenAIImagesTestContext(t, raw)
		parsed, err := (&OpenAIGatewayService{}).ParseOpenAIImagesRequest(c, raw)
		require.NoError(t, err)
		encoded, _, err := buildOpenAIImagesOAuthPayload(parsed, parsed.Model)
		require.NoError(t, err)
		require.Equal(t, gjson.Number, gjson.GetBytes(encoded, "frame_count").Type)
		require.Equal(t, int64(20), gjson.GetBytes(encoded, "frame_count").Int())
		require.Equal(t, gjson.Number, gjson.GetBytes(encoded, "frame_duration_ms").Type)
		require.Equal(t, int64(120), gjson.GetBytes(encoded, "frame_duration_ms").Int())
	}
}

func TestImagesRegressionAccountTestFallbackParity(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/1/test", nil)
	sse := "data: {\"type\":\"response.output_item.done\",\"item\":{\"id\":\"ig_1\",\"type\":\"image_generation_call\",\"result\":\"aA==\",\"output_format\":\"png\"}}\n\n" + "data: {\"type\":\"response.completed\",\"response\":{\"tool_usage\":{\"image_gen\":{\"images\":1}},\"output\":[]}}\n\n"
	u := &httpUpstreamRecorder{responses: []*http.Response{imagesRegressionResponse(`{"error":{"message":"not found"}}`, 404), imagesRegressionResponse(sse, 200)}}
	s := &AccountTestService{httpUpstream: u}
	e := s.testOpenAIImageOAuth(c, context.Background(), newDirectImagesOAuthAccount(), "gpt-image-2.6-flare", "draw")
	require.NoError(t, e)
	require.Len(t, u.requests, 2)
	for _, req := range u.requests {
		require.Equal(t, HTTPUpstreamProfileOpenAIImages, HTTPUpstreamProfileFromContext(req.Context()))
	}
}
func TestImagesRegressionDirectReadErrorBeforeOutputFailsOver(t *testing.T) {
	raw := []byte(`{"model":"gpt-image-2.6-flare","prompt":"draw","stream":true}`)
	c, _ := newOpenAIImagesTestContext(t, raw)
	resp := &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: passthroughErrReadCloser{err: io.ErrUnexpectedEOF}}
	s := newOpenAIImagesTestService(&httpUpstreamRecorder{resp: resp})
	p, e := s.ParseOpenAIImagesRequest(c, raw)
	require.NoError(t, e)
	_, e = s.ForwardImages(context.Background(), c, newDirectImagesOAuthAccount(), raw, p, "")
	var failover *UpstreamFailoverError
	require.ErrorAs(t, e, &failover)
}

func TestImagesRegressionExplicitZeroDynamicImageCache(t *testing.T) {
	ps := &PricingService{}
	data, e := ps.parsePricingData([]byte(`{"gpt-image-2.6-flare":{"input_cost_per_token":0.000005,"cache_read_input_token_cost":0.00000125,"cache_read_input_image_token_cost":0}}`))
	require.NoError(t, e)
	ps.pricingData = data
	bs := &BillingService{pricingService: ps}
	pricing, e := bs.GetModelPricing("gpt-image-2.6-flare")
	require.NoError(t, e)
	bd := bs.computeTokenBreakdown(pricing, UsageTokens{CacheReadTokens: 100, ImageCacheReadTokens: 100}, 1, "", false)
	require.Zero(t, bd.CacheReadCost, "explicit image cache price 0 must remain free")
}

func TestImagesRegressionDirectDrainAfterDisconnect(t *testing.T) {
	for _, interval := range []int{0, 1} {
		t.Run(string(rune('0'+interval)), func(t *testing.T) {
			raw := []byte(`{"model":"gpt-image-2.6-flare","prompt":"draw","stream":true}`)
			c, _ := newOpenAIImagesTestContext(t, raw)
			c.Writer = &failingOpenAIImageWriter{ResponseWriter: c.Writer, failAfter: 1}
			stream := "data: {\"type\":\"image_generation.partial_image\",\"b64_json\":\"cA==\"}\n\n" + "data: {\"type\":\"image_generation.completed\",\"b64_json\":\"aA==\",\"usage\":{\"input_tokens\":100,\"output_tokens\":10,\"input_tokens_details\":{\"image_tokens\":60,\"cached_tokens\":40,\"cached_tokens_details\":{\"image_tokens\":30,\"text_tokens\":10}}}}\n\n"
			resp := imagesRegressionResponse(stream, 200)
			resp.Header.Set("Content-Type", "text/event-stream")
			s := newOpenAIImagesTestService(&httpUpstreamRecorder{resp: resp})
			s.cfg.Gateway.ImageStreamDataIntervalTimeout = interval
			p, e := s.ParseOpenAIImagesRequest(c, raw)
			require.NoError(t, e)
			out, e := s.ForwardImages(context.Background(), c, newDirectImagesOAuthAccount(), raw, p, "")
			require.NoError(t, e)
			require.NotNil(t, out)
			require.Equal(t, 1, out.ImageCount)
			require.Equal(t, 10, out.Usage.OutputTokens)
			require.Equal(t, 30, out.Usage.ImageCacheReadTokens)
		})
	}
}

func TestImagesRegressionDirectPartialReadErrorCannotRetry(t *testing.T) {
	raw := []byte(`{"model":"gpt-image-2.6-flare","prompt":"draw","stream":true}`)
	c, _ := newOpenAIImagesTestContext(t, raw)
	partial := "data: {\"type\":\"image_generation.partial_image\",\"b64_json\":\"cA==\"}\n\n"
	resp := &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(io.MultiReader(strings.NewReader(partial), passthroughErrReadCloser{err: io.ErrUnexpectedEOF}))}
	u := &httpUpstreamRecorder{resp: resp}
	s := newOpenAIImagesTestService(u)
	p, e := s.ParseOpenAIImagesRequest(c, raw)
	require.NoError(t, e)
	_, e = s.ForwardImages(context.Background(), c, newDirectImagesOAuthAccount(), raw, p, "")
	require.Error(t, e)
	var failover *UpstreamFailoverError
	require.False(t, errors.As(e, &failover))
	require.Len(t, u.requests, 1)
}

func TestImagesRegressionConfiguredCachePriceAcrossResolvers(t *testing.T) {
	for _, price := range []float64{0, 7e-6} {
		for _, path := range []string{"channel", "resolver", "interval"} {
			t.Run(fmt.Sprintf("%s/%g", path, price), func(t *testing.T) {
				pricing := &ModelPricing{CacheReadPricePerToken: 1.25e-6, ImageCacheReadPricePerToken: 2e-6, ImageCacheReadPriceExplicit: true}
				switch path {
				case "channel":
					applyChannelTokenPriceOverrides(pricing, &ChannelModelPricing{CacheReadPrice: &price})
				case "resolver":
					resolved := &ResolvedPricing{BasePricing: pricing}
					(&ModelPricingResolver{}).applyTokenOverrides(&ChannelModelPricing{CacheReadPrice: &price}, resolved)
					pricing = resolved.BasePricing
				case "interval":
					pricing = intervalToModelPricing(&PricingInterval{CacheReadPrice: &price}, pricing, nil)
				}
				cost := (&BillingService{}).computeTokenBreakdown(pricing, UsageTokens{CacheReadTokens: 100, ImageCacheReadTokens: 60}, 1, "", false)
				require.InDelta(t, 100*price, cost.CacheReadCost, 1e-15)
			})
		}
	}
	multiplier := 0.5
	pricing := intervalToModelPricing(&PricingInterval{CacheReadMultiplier: &multiplier}, &ModelPricing{CacheReadPricePerToken: 1.25e-6, ImageCacheReadPricePerToken: 2e-6}, nil)
	cost := (&BillingService{}).computeTokenBreakdown(pricing, UsageTokens{CacheReadTokens: 100, ImageCacheReadTokens: 60}, 1, "", false)
	require.InDelta(t, (40*1.25e-6+60*2e-6)*0.5, cost.CacheReadCost, 1e-15)
}

func TestImagesRegressionAbsentCachePriceUsesTextRate(t *testing.T) {
	ps := &PricingService{}
	data, err := ps.parsePricingData([]byte(`{"gpt-image-2.6-flare":{"input_cost_per_token":0.000005,"cache_read_input_token_cost":0.00000125}}`))
	require.NoError(t, err)
	ps.pricingData = data
	bs := &BillingService{pricingService: ps}
	price, err := bs.GetModelPricing("gpt-image-2.6-flare")
	require.NoError(t, err)
	cost := bs.computeTokenBreakdown(price, UsageTokens{CacheReadTokens: 100, ImageCacheReadTokens: 100}, 1, "", false)
	require.InDelta(t, 0.000125, cost.CacheReadCost, 1e-15)
}

func TestImagesRegressionAPIKeyStreamingCacheAccounting(t *testing.T) {
	raw := []byte(`{"model":"gpt-image-2.6-flare","prompt":"draw","stream":true}`)
	c, _ := newOpenAIImagesTestContext(t, raw)
	stream := `data: {"type":"image_generation.completed","b64_json":"aA==","usage":{"input_tokens":100,"output_tokens":10,"output_tokens_details":{"image_tokens":10},"input_tokens_details":{"image_tokens":60,"cached_tokens":40,"cached_tokens_details":{"image_tokens":30,"text_tokens":10}}}}` + "\n\n"
	resp := imagesRegressionResponse(stream, 200)
	resp.Header.Set("Content-Type", "text/event-stream")
	s := newOpenAIImagesTestService(&httpUpstreamRecorder{resp: resp})
	parsed, err := s.ParseOpenAIImagesRequest(c, raw)
	require.NoError(t, err)
	result, err := s.ForwardImages(context.Background(), c, newOpenAIImagesAPIKeyAccount(), raw, parsed, "")
	require.NoError(t, err)
	require.Equal(t, 100, result.Usage.InputTokens)
	require.Equal(t, 60, result.Usage.ImageInputTokens)
	require.Equal(t, 40, result.Usage.CacheReadInputTokens)
	require.Equal(t, 30, result.Usage.ImageCacheReadTokens)
	require.Equal(t, 10, result.Usage.ImageOutputTokens)
	// Uncached input consists of 30 text + 30 image tokens; cache is 10 text + 30 image.
	cost := (&BillingService{}).computeTokenBreakdown(&ModelPricing{InputPricePerToken: 5e-6, ImageInputPricePerToken: 8e-6, CacheReadPricePerToken: 1.25e-6, ImageCacheReadPricePerToken: 2e-6, ImageOutputPricePerToken: 3e-5}, UsageTokens{
		InputTokens:      result.Usage.InputTokens - result.Usage.CacheReadInputTokens,
		ImageInputTokens: result.Usage.ImageInputTokens - result.Usage.ImageCacheReadTokens,
		CacheReadTokens:  result.Usage.CacheReadInputTokens, ImageCacheReadTokens: result.Usage.ImageCacheReadTokens,
		OutputTokens: result.Usage.OutputTokens, ImageOutputTokens: result.Usage.ImageOutputTokens,
	}, 1, "", false)
	require.InDelta(t, 0.0007625, cost.TotalCost, 1e-15)
}

func TestImagesRegressionDirectStreamingErrorClassification(t *testing.T) {
	for _, interval := range []int{0, 1} {
		for _, partial := range []bool{false, true} {
			for _, header := range []bool{false, true} {
				for _, kind := range []string{"content_policy_violation", "server_error"} {
					t.Run(fmt.Sprintf("interval_%d/partial_%t/header_%t/%s", interval, partial, header, kind), func(t *testing.T) {
						raw := []byte(`{"model":"gpt-image-2.6-flare","prompt":"draw","stream":true}`)
						c, rec := newOpenAIImagesTestContext(t, raw)
						errorType := "invalid_request_error"
						if kind == "server_error" {
							errorType = "server_error"
						}
						event := fmt.Sprintf("data: {\"type\":\"error\",\"error\":{\"type\":%q,\"code\":%q,\"message\":\"original actionable error\"}}\n\n", errorType, kind)
						if header {
							event = "event: error\n" + event
						}
						if partial {
							event = "data: {\"type\":\"image_generation.partial_image\",\"b64_json\":\"cA==\"}\n\n" + event
						}
						resp := imagesRegressionResponse(event, 200)
						resp.Header.Set("Content-Type", "text/event-stream")
						// A transport close after a terminal error must not replace
						// the upstream's actionable error with a generic read error.
						resp.Body = io.NopCloser(io.MultiReader(strings.NewReader(event), passthroughErrReadCloser{err: io.ErrUnexpectedEOF}))
						svc := newOpenAIImagesTestService(&httpUpstreamRecorder{resp: resp})
						svc.cfg.Gateway.ImageStreamDataIntervalTimeout = interval
						parsed, err := svc.ParseOpenAIImagesRequest(c, raw)
						require.NoError(t, err)
						_, err = svc.ForwardImages(context.Background(), c, newDirectImagesOAuthAccount(), raw, parsed, "")
						if kind == "server_error" && !partial {
							var failover *UpstreamFailoverError
							require.ErrorAs(t, err, &failover)
							require.Empty(t, rec.Body.String(), "retryable error must not commit downstream")
							require.Contains(t, string(failover.ResponseBody), "original actionable error")
						} else {
							var up *OpenAIImagesUpstreamError
							require.ErrorAs(t, err, &up)
							require.Equal(t, kind, up.Code)
							require.Contains(t, rec.Body.String(), "original actionable error")
							var failover *UpstreamFailoverError
							require.False(t, errors.As(err, &failover))
							if partial {
								require.Contains(t, rec.Body.String(), "image_generation.partial_image")
							} else {
								require.Equal(t, http.StatusBadRequest, rec.Code)
							}
						}
					})
				}
			}
		}
	}
}
