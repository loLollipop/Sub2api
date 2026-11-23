package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"go.uber.org/zap"
)

var codexTextContextNamesForTest = []string{"browser_info", "goal_objective", "untrusted_context", "untrusted_content", "ide_context", "proposed_plan"}

func TestNormalizeCodexAdditionalUntrustedContext(t *testing.T) {
	for _, name := range codexTextContextNamesForTest {
		t.Run(name, func(t *testing.T) {
			output := "external context\nline two"
			if name == "ide_context" {
				output = "# Context from my IDE setup:\n\n## Active file: main.go\n\n## Active selection of the file:\nfunc main() {}"
			}
			contextItem := map[string]any{"type": "function_call_output", "namespace": "agent", "name": name, "output": output}
			body := pageContextRequest(t, contextItem,
				map[string]any{"role": "user", "content": "continue"})
			h := &OpenAIGatewayHandler{}
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			require.False(t, h.validateFunctionCallOutputRequest(c, body, zap.NewNop()))
			require.Equal(t, 400, w.Code)
			require.Contains(t, w.Body.String(), "function_call_output requires call_id")

			got, changed := normalizeCodexUntrustedContext(body)
			require.True(t, changed)
			w = httptest.NewRecorder()
			c, _ = gin.CreateTestContext(w)
			require.True(t, h.validateFunctionCallOutputRequest(c, got, zap.NewNop()))
			require.False(t, c.Writer.Written())
			item := gjson.GetBytes(got, "input.0")
			require.Equal(t, "message", item.Get("type").String())
			require.Equal(t, "user", item.Get("role").String())
			require.Equal(t, "<external_"+name+">"+output+"</external_"+name+">", item.Get("content.0.text").String())
			require.False(t, item.Get("call_id").Exists())
			require.Equal(t, "continue", gjson.GetBytes(got, "input.1.content").String())
			again, changed := normalizeCodexUntrustedContext(got)
			require.False(t, changed)
			require.Equal(t, got, again)
		})
	}
}

func TestNormalizeCodexAdditionalUntrustedContextPreservesRealAndMalformedOutputs(t *testing.T) {
	for _, name := range codexTextContextNamesForTest {
		for _, override := range []map[string]any{
			{"call_id": "call_real"},
			{"call_id": nil},
			{"call_id": 42},
			{"namespace": "other"},
			{"namespace": nil},
			{"namespace": 42},
			{"output": []any{map[string]any{"type": "encrypted_content", "encrypted_content": "sealed"}}},
			{"output": nil},
		} {
			item := pageContextItem(name, "context")
			for key, value := range override {
				item[key] = value
			}
			body := pageContextRequest(t, item)
			got, changed := normalizeCodexUntrustedContext(body)
			require.False(t, changed, "name=%s override=%v", name, override)
			require.Equal(t, body, got)
		}
		for _, field := range []string{"namespace", "output"} {
			item := pageContextItem(name, "context")
			delete(item, field)
			body := pageContextRequest(t, item)
			got, changed := normalizeCodexUntrustedContext(body)
			require.False(t, changed)
			require.Equal(t, body, got)
		}
	}
	// 已识别的外部上下文不能让无关的工具结果变成合法条目。
	body := pageContextRequest(t, pageContextItem("browser_info", "tab"),
		pageContextItem("read_file", "result"))
	got, changed := normalizeCodexUntrustedContext(body)
	require.True(t, changed)
	h := &OpenAIGatewayHandler{}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	require.False(t, h.validateFunctionCallOutputRequest(c, got, zap.NewNop()))
	require.Equal(t, 400, w.Code)

	// 历史调用、续传字段与精确数字保持原样。
	body = []byte(`{"model":"gpt-5","previous_response_id":"resp_history","metadata":{"large":9007199254740993},"input":[{"type":"function_call","name":"read_file","call_id":"call_real","arguments":"{}"},{"type":"function_call_output","call_id":"call_real","output":"result"},{"type":"function_call_output","namespace":"agent","name":"goal_objective","output":"finish"}]}`)
	got, changed = normalizeCodexUntrustedContext(body)
	require.True(t, changed)
	require.Equal(t, "resp_history", gjson.GetBytes(got, "previous_response_id").String())
	require.Equal(t, "9007199254740993", gjson.GetBytes(got, "metadata.large").Raw)
	for _, index := range []string{"input.0", "input.1"} {
		require.JSONEq(t, gjson.GetBytes(body, index).Raw, gjson.GetBytes(got, index).Raw)
	}
}

func TestNormalizeCodexAdditionalUntrustedContextBlankCallIDsAndMixedInput(t *testing.T) {
	names := codexTextContextNamesForTest
	for _, callID := range []string{"", " \t\n"} {
		items := make([]any, 0, len(names))
		for _, name := range names {
			item := pageContextItem(name, "context for "+name)
			item["call_id"] = callID
			delete(item, "id")
			items = append(items, item)
		}
		got, changed := normalizeCodexUntrustedContext(pageContextRequest(t, items...))
		require.True(t, changed)
		require.Len(t, gjson.GetBytes(got, "input").Array(), len(names))
		for i, item := range gjson.GetBytes(got, "input").Array() {
			require.Equal(t, "user", item.Get("role").String())
			require.Equal(t, "<external_"+names[i]+">context for "+names[i]+"</external_"+names[i]+">", item.Get("content.0.text").String())
		}
	}
}

func TestNormalizeCodexAdditionalUntrustedContextRejectsDuplicateMembers(t *testing.T) {
	for _, name := range codexTextContextNamesForTest {
		for _, body := range []string{
			`{"input":[{"type":"function_call_output","namespace":"agent","name":"` + name + `","call_id":"real","call_id":"","output":"context"}]}`,
			`{"input":[{"type":"function_call_output","namespace":"agent","name":"` + name + `","output":"first","output":"second"}]}`,
		} {
			got, changed := normalizeCodexUntrustedContext([]byte(body))
			require.False(t, changed)
			require.Equal(t, body, string(got))
		}
	}
}

func TestNormalizeCodexProposedPlanChunks(t *testing.T) {
	chunks := []string{"# Plan\n", " \n\t", "1. Run tests."}
	var items []any
	for _, chunk := range chunks {
		items = append(items, pageContextItem("proposed_plan", chunk))
	}
	got, changed := normalizeCodexUntrustedContext(pageContextRequest(t, items...))
	require.True(t, changed)
	messages := gjson.GetBytes(got, "input").Array()
	require.Len(t, messages, len(chunks))
	for i, message := range messages {
		require.Equal(t, "user", message.Get("role").String())
		require.Equal(t, "<external_proposed_plan>"+chunks[i]+"</external_proposed_plan>", message.Get("content.0.text").String())
	}
}

func TestNormalizeCodexAdditionalUntrustedContextBlankOutput(t *testing.T) {
	for _, name := range codexTextContextNamesForTest {
		for _, output := range []string{"", " \t"} {
			body := pageContextRequest(t, pageContextItem(name, output))
			got, changed := normalizeCodexUntrustedContext(body)
			wantChange := name == "untrusted_content" || (name == "proposed_plan" && output != "")
			require.Equal(t, wantChange, changed, "name=%s output=%q", name, output)
			if wantChange {
				require.Equal(t, "<external_"+name+">"+output+"</external_"+name+">", gjson.GetBytes(got, "input.0.content.0.text").String())
			} else {
				require.Equal(t, body, got)
			}
		}
	}
}

// TestOpenAIResponses_FunctionCallOutputMissingCallIDErrorBody 覆盖 HTTP 预校验的
// 错误体格式与可操作提示：绝不能透传上游 500，也不能给出“复用
// previous_response_id”这种无效指引。
func TestOpenAIResponses_FunctionCallOutputMissingCallIDErrorBody(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := []byte(`{"model":"gpt-5.1","stream":false,"input":[{"type":"function_call_output","output":"{}"}]}`)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	require.False(t, (&OpenAIGatewayHandler{}).validateFunctionCallOutputRequest(c, body, zap.NewNop()))
	require.Equal(t, http.StatusBadRequest, w.Code)

	var parsed map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &parsed))
	errorObj, ok := parsed["error"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "invalid_request_error", errorObj["type"])
	message, _ := errorObj["message"].(string)
	require.Contains(t, message, "function_call_output requires call_id on HTTP requests")
	require.Contains(t, message, "untrusted_as_tool_output")
	require.NotContains(t, message, "reuse previous_response_id")
}

// TestOpenAIResponses_FunctionCallOutputWithCallIDPassesValidation 覆盖带 call_id
// 的正常工具续链不受影响。
func TestOpenAIResponses_FunctionCallOutputWithCallIDPassesValidation(t *testing.T) {
	body := []byte(`{"model":"gpt-5.1","stream":false,"input":[{"type":"function_call","call_id":"call_1","name":"read_file","arguments":"{}"},{"type":"function_call_output","call_id":"call_1","output":"result"}]}`)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	require.True(t, (&OpenAIGatewayHandler{}).validateFunctionCallOutputRequest(c, body, zap.NewNop()))
	require.False(t, c.Writer.Written())
}

// TestOpenAIResponses_HTTPContinuationWithPageContextKeepsPreviousResponseID 覆盖
// HTTP + previous_response_id + 页面上下文：续传字段不被删除，归一化后校验通过。
func TestOpenAIResponses_HTTPContinuationWithPageContextKeepsPreviousResponseID(t *testing.T) {
	body := pageContextRequest(t, pageContextItem("codex_apps_open_page", `{"page_id":null}`))
	body = []byte(strings.Replace(string(body), `{"input":`, `{"previous_response_id":"resp_123456","input":`, 1))

	got, changed := normalizeCodexUntrustedContext(body)
	require.True(t, changed)
	require.Equal(t, "resp_123456", gjson.GetBytes(got, "previous_response_id").String())

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	require.True(t, (&OpenAIGatewayHandler{}).validateFunctionCallOutputRequest(c, got, zap.NewNop()))
	require.False(t, c.Writer.Written())

	// 即使客户端直接带着 previous_response_id 发未归一化的页面上下文，HTTP 预校验
	// 也允许透传（上游按 response.id 解析历史），不会本地 400。
	c, _ = gin.CreateTestContext(httptest.NewRecorder())
	require.True(t, (&OpenAIGatewayHandler{}).validateFunctionCallOutputRequest(c, body, zap.NewNop()))
}

// TestOpenAIResponses_HTTPContinuationErrorIsActionable 验证 OAuth/SetupToken
// 账号不允许 HTTP previous_response_id 时的错误可操作、且不是 500。
func TestOpenAIResponses_HTTPContinuationErrorIsActionable(t *testing.T) {
	require.Contains(t, previousResponseIDRequiresAPIKeyMessage, "previous_response_id requires an OpenAI API-key account on HTTP requests")
	require.Contains(t, previousResponseIDRequiresAPIKeyMessage, "WebSocket v2")
	require.Contains(t, previousResponseIDRequiresAPIKeyMessage, "resend the full input without previous_response_id")

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	(&OpenAIGatewayHandler{}).handleFailoverExhausted(c, &service.UpstreamFailoverError{
		StatusCode: http.StatusBadRequest,
		Stage:      service.GatewayFailureStageInference,
		Scope:      service.GatewayFailureScopeRequest,
		Reason:     service.OpenAIHTTPContinuationUnsupportedReason,
	}, false)
	require.Equal(t, http.StatusBadRequest, w.Code)

	var parsed map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &parsed))
	errorObj, ok := parsed["error"].(map[string]any)
	require.True(t, ok)
	message, _ := errorObj["message"].(string)
	require.Contains(t, message, "previous_response_id requires an OpenAI API-key account on HTTP requests")
	require.Contains(t, message, "resend the full input without previous_response_id")
}

type codexContextUpstream struct {
	service.HTTPUpstream
	bodies [][]byte
}

func (u *codexContextUpstream) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	body, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}
	u.bodies = append(u.bodies, body)
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"id":"resp_context","object":"response","model":"gpt-5","status":"completed","output":[],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}`)),
	}, nil
}

func newCodexContextHandler(t *testing.T, accountType string) (*OpenAIGatewayHandler, *codexContextUpstream) {
	t.Helper()
	cfg := &config.Config{RunMode: config.RunModeSimple}
	cfg.Default.RateMultiplier = 1
	cfg.Security.URLAllowlist.Enabled = false
	accountRepo := &openAIWSUsageHandlerAccountRepoStub{account: service.Account{
		ID: 9911, Name: "context-test", Platform: service.PlatformOpenAI,
		Type: accountType, Status: service.StatusActive, Schedulable: true,
		Credentials: map[string]any{"api_key": "sk-test", "access_token": "test", "base_url": "https://api.example.test"},
		Extra:       map[string]any{"openai_passthrough": true},
	}}
	upstream := &codexContextUpstream{}
	billing := service.NewBillingCacheService(nil, nil, nil, nil, nil, nil, cfg, nil)
	t.Cleanup(billing.Stop)
	gateway := service.NewOpenAIGatewayService(
		accountRepo, nil, nil, nil, nil, nil, nil, cfg, nil, nil,
		service.NewBillingService(cfg, nil), nil, billing, upstream, &service.DeferredService{},
		nil, nil, nil, nil, nil, nil, nil,
	)
	h := NewOpenAIGatewayHandler(
		gateway, service.NewConcurrencyService(nil), billing,
		service.NewAPIKeyService(nil, nil, nil, nil, nil, nil, cfg),
		nil, nil, nil, nil, cfg,
	)
	return h, upstream
}

func runCodexContextResponses(t *testing.T, h *OpenAIGatewayHandler, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	groupID := int64(4203)
	c.Set(string(middleware.ContextKeyAPIKey), &service.APIKey{
		ID: 101, UserID: 1, GroupID: &groupID,
		User:  &service.User{ID: 1, Status: service.StatusActive},
		Group: &service.Group{ID: groupID, Platform: service.PlatformOpenAI, Status: service.StatusActive, Hydrated: true},
	})
	c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 1, Concurrency: 0})
	h.Responses(c)
	return w
}

func TestOpenAIResponses_CodexUntrustedContextForwardsAllTextKinds(t *testing.T) {
	for _, name := range codexTextContextNamesForTest {
		t.Run(name, func(t *testing.T) {
			h, upstream := newCodexContextHandler(t, service.AccountTypeAPIKey)
			item := pageContextItem(name, "context\n正文")
			for _, callID := range []any{nil, "", " \t\n"} {
				delete(item, "call_id")
				if callID != nil {
					item["call_id"] = callID
				}
				w := runCodexContextResponses(t, h, pageContextRequest(t, item, map[string]any{"role": "user", "content": "continue"}))
				require.Equal(t, http.StatusOK, w.Code, w.Body.String())
				got := upstream.bodies[len(upstream.bodies)-1]
				require.Equal(t, "user", gjson.GetBytes(got, "input.0.role").String())
				require.Equal(t, "<external_"+name+">context\n正文</external_"+name+">", gjson.GetBytes(got, "input.0.content.0.text").String())
				require.Equal(t, "continue", gjson.GetBytes(got, "input.1.content").String())
				require.False(t, gjson.GetBytes(got, "input.0.call_id").Exists())
			}
			require.Len(t, upstream.bodies, 3)
		})
	}
}

func TestOpenAIResponses_CodexUntrustedContextPreservesCompactedHistoryAndChunks(t *testing.T) {
	h, upstream := newCodexContextHandler(t, service.AccountTypeAPIKey)
	first := "Companion Page snapshot part 1 of 2:\n" + `{"page_id":"page1","document_view":"hello`
	second := "Companion Page snapshot part 2 of 2:\n" + ` world"}`
	items := []any{
		json.RawMessage(`{"type":"compaction","encrypted_content":"sealed-history","metadata":{"large":9007199254740993}}`),
		json.RawMessage(`{"type":"function_call","name":"read_file","call_id":"call_real","arguments":"{}"}`),
		json.RawMessage(`{"type":"function_call_output","call_id":"call_real","output":"result"}`),
		pageContextItem("codex_apps_selected_page_part_1_of_2", first),
		pageContextItem("codex_apps_selected_page_part_2_of_2", second),
		pageContextItem("untrusted_content", ""),
		pageContextItem("proposed_plan", "# Plan\n"),
		pageContextItem("proposed_plan", " \n\t"),
		pageContextItem("proposed_plan", "1. Run tests."),
		map[string]any{"type": "function_call_output", "namespace": "codex_app", "name": "create_thread", "output": delegationEnvelope},
	}
	body := pageContextRequest(t, items...)
	w := runCodexContextResponses(t, h, body)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.Len(t, upstream.bodies, 1)
	got := upstream.bodies[0]
	require.Len(t, gjson.GetBytes(got, "input").Array(), len(items))
	for _, index := range []string{"input.0", "input.1", "input.2"} {
		require.JSONEq(t, gjson.GetBytes(body, index).Raw, gjson.GetBytes(got, index).Raw)
	}
	require.Equal(t, "9007199254740993", gjson.GetBytes(got, "input.0.metadata.large").Raw)
	require.Equal(t, "<external_codex_apps_selected_page_part_1_of_2>\n"+first+"\n</external_codex_apps_selected_page_part_1_of_2>", gjson.GetBytes(got, "input.3.content.0.text").String())
	require.Equal(t, "<external_codex_apps_selected_page_part_2_of_2>\n"+second+"\n</external_codex_apps_selected_page_part_2_of_2>", gjson.GetBytes(got, "input.4.content.0.text").String())
	require.Equal(t, "<external_untrusted_content></external_untrusted_content>", gjson.GetBytes(got, "input.5.content.0.text").String())
	for i, chunk := range []string{"# Plan\n", " \n\t", "1. Run tests."} {
		require.Equal(t, "<external_proposed_plan>"+chunk+"</external_proposed_plan>", gjson.GetBytes(got, "input").Array()[i+6].Get("content.0.text").String())
	}
	require.Equal(t, delegationEnvelope, gjson.GetBytes(got, "input.9.content.0.text").String())
}

func TestOpenAIResponses_CodexUntrustedContextKeepsInvalidToolsRejected(t *testing.T) {
	for _, tc := range []struct {
		name string
		item any
	}{
		{"unknown", pageContextItem("read_file", "result")},
		{"real call without reference", map[string]any{"type": "function_call_output", "namespace": "agent", "name": "browser_info", "call_id": "call_real", "output": "result"}},
		{"null call ID", map[string]any{"type": "function_call_output", "namespace": "agent", "name": "untrusted_content", "call_id": nil, "output": ""}},
		{"null output", map[string]any{"type": "function_call_output", "namespace": "agent", "name": "untrusted_content", "output": nil}},
		{"missing output", map[string]any{"type": "function_call_output", "namespace": "agent", "name": "untrusted_content"}},
		{"blank context", pageContextItem("browser_info", " \t")},
		{"empty plan", pageContextItem("proposed_plan", "")},
		{"duplicate output", json.RawMessage(`{"type":"function_call_output","namespace":"agent","name":"browser_info","output":"one","output":"two"}`)},
		{"duplicate call ID", json.RawMessage(`{"type":"function_call_output","namespace":"agent","name":"browser_info","call_id":"","call_id":"call_real","output":"context"}`)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h, upstream := newCodexContextHandler(t, service.AccountTypeAPIKey)
			w := runCodexContextResponses(t, h, pageContextRequest(t, pageContextItem("goal_objective", "finish"), tc.item))
			require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
			require.Equal(t, "invalid_request_error", gjson.GetBytes(w.Body.Bytes(), "error.type").String())
			require.Empty(t, upstream.bodies)
		})
	}
}

func TestOpenAIResponses_CodexUntrustedContextHTTPContinuationAccounts(t *testing.T) {
	for _, accountType := range []string{service.AccountTypeAPIKey, service.AccountTypeOAuth, service.AccountTypeSetupToken} {
		t.Run(accountType, func(t *testing.T) {
			h, upstream := newCodexContextHandler(t, accountType)
			require.NoError(t, h.gatewayService.BindOpenAIHTTPResponseOwner(context.Background(), 4203, "resp_history", 1, 101))
			body := []byte(`{"model":"gpt-5","previous_response_id":"resp_history","input":[{"type":"function_call_output","namespace":"agent","name":"browser_info","output":"tab"},{"type":"function_call_output","call_id":"call_history","output":"result"}]}`)
			w := runCodexContextResponses(t, h, body)
			if accountType == service.AccountTypeAPIKey {
				require.Equal(t, http.StatusOK, w.Code, w.Body.String())
				require.Len(t, upstream.bodies, 1)
				got := upstream.bodies[0]
				require.Equal(t, "resp_history", gjson.GetBytes(got, "previous_response_id").String())
				require.Equal(t, "<external_browser_info>tab</external_browser_info>", gjson.GetBytes(got, "input.0.content.0.text").String())
				require.Equal(t, gjson.GetBytes(body, "input.1").Raw, gjson.GetBytes(got, "input.1").Raw)
			} else {
				require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
				require.Equal(t, previousResponseIDRequiresAPIKeyMessage, gjson.GetBytes(w.Body.Bytes(), "error.message").String())
				require.Empty(t, upstream.bodies)
			}
		})
	}
}
