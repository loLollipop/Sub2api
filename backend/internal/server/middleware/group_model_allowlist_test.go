package middleware

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/httputil"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

func newGroupModelAllowlistTestRouter(apiKey *service.APIKey, pathPrefix string) (*gin.Engine, *[]string) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	var calls []string
	router.Use(func(c *gin.Context) {
		if apiKey != nil {
			c.Set(string(ContextKeyAPIKey), apiKey)
		}
		c.Next()
	})
	router.Use(GroupModelAllowlist())
	register := func(method, path string) {
		router.Handle(method, path, func(c *gin.Context) {
			calls = append(calls, "handler:"+path)
			c.Status(http.StatusOK)
		})
	}
	register(http.MethodPost, pathPrefix+"/messages")
	register(http.MethodPost, pathPrefix+"/responses")
	register(http.MethodPost, pathPrefix+"/chat/completions")
	register(http.MethodPost, pathPrefix+"/embeddings")
	register(http.MethodGet, pathPrefix+"/models/:model")
	register(http.MethodPost, pathPrefix+"/models/*modelAction")
	register(http.MethodGet, pathPrefix+"/realtime")
	register(http.MethodPost, pathPrefix+"/images/edits")
	register(http.MethodPost, pathPrefix+"/live")
	return router, &calls
}

func allowlistAPIKey(enabled bool, models ...string) *service.APIKey {
	return &service.APIKey{
		Group: &service.Group{
			Platform: service.PlatformAnthropic,
			ModelAllowlist: service.GroupModelAllowlist{
				Enabled: enabled,
				Models:  models,
			},
		},
	}
}

func doJSON(t *testing.T, router *gin.Engine, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

// readTrackingBody 记录请求体是否被读取，用于断言快速路径零读取。
type readTrackingBody struct {
	io.Reader
	read bool
}

func (b *readTrackingBody) Read(p []byte) (int, error) {
	b.read = true
	return b.Reader.Read(p)
}

func (b *readTrackingBody) Close() error { return nil }

func TestGroupModelAllowlistDisabledDoesNotReadBody(t *testing.T) {
	router, calls := newGroupModelAllowlistTestRouter(allowlistAPIKey(false, "claude-sonnet-4.5"), "/v1")

	body := &readTrackingBody{Reader: strings.NewReader(`{"model":"claude-opus-4.6"}`)}
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if len(*calls) != 1 {
		t.Fatalf("expected handler to run once, got %v", *calls)
	}
	if body.read {
		t.Fatal("allowlist disabled: middleware must not read the request body")
	}
}

// 白名单开启但请求不携带模型时同样不应读体之外产生副作用：读体是必要开销，
// 这里验证读取后请求体被完整回填（handler 可零拷贝重读）。
func TestGroupModelAllowlistEnabledModelFreeRequestRestoresBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(ContextKeyAPIKey), allowlistAPIKey(true, "claude-sonnet-4.5"))
		c.Next()
	})
	router.Use(GroupModelAllowlist())
	router.POST("/v1/messages", func(c *gin.Context) {
		body, err := httputil.ReadRequestBodyWithPrealloc(c.Request)
		if err != nil {
			t.Errorf("handler reread body: %v", err)
			c.Status(http.StatusInternalServerError)
			return
		}
		if string(body) != `{"input":"no model field"}` {
			t.Errorf("handler saw wrong body: %s", body)
		}
		c.Status(http.StatusOK)
	})

	w := doJSON(t, router, http.MethodPost, "/v1/messages", `{"input":"no model field"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

// 413 传递：请求体超过 MaxBytesReader 上限时按现有合成中间件方式返回 413。
func TestGroupModelAllowlistRequestBodyTooLargePassesThrough413(t *testing.T) {
	router, calls := newGroupModelAllowlistTestRouter(allowlistAPIKey(true, "claude-sonnet-4.5"), "/v1")

	bigBody := `{"model":"claude-opus-4.6","padding":"` + strings.Repeat("x", 512) + `"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(bigBody))
	req.Header.Set("Content-Type", "application/json")
	req.Body = http.MaxBytesReader(httptest.NewRecorder(), req.Body, 64)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "Request body is too large") {
		t.Fatalf("expected too-large message, got %s", w.Body.String())
	}
	if len(*calls) != 0 {
		t.Fatalf("expected handler not to run, got %v", *calls)
	}
}

func TestGroupModelAllowlistJSONBodyAllowed(t *testing.T) {
	router, calls := newGroupModelAllowlistTestRouter(allowlistAPIKey(true, "claude-sonnet-4.5"), "/v1")

	w := doJSON(t, router, http.MethodPost, "/v1/messages", `{"model":"claude-sonnet-4.5"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if len(*calls) != 1 {
		t.Fatalf("expected handler to run once, got %v", *calls)
	}
}

func TestGroupModelAllowlistJSONBodyDeniedOpenAIFormat(t *testing.T) {
	router, calls := newGroupModelAllowlistTestRouter(allowlistAPIKey(true, "claude-sonnet-4.5"), "/v1")

	w := doJSON(t, router, http.MethodPost, "/v1/responses", `{"model":"gpt-5.4"}`)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"model_not_found"`) {
		t.Fatalf("expected OpenAI error format, got %s", w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `gpt-5.4\" is not available for this group`) {
		t.Fatalf("expected model in message, got %s", w.Body.String())
	}
	if len(*calls) != 0 {
		t.Fatalf("expected handler not to run, got %v", *calls)
	}
}

func TestGroupModelAllowlistMessagesDeniedAnthropicFormat(t *testing.T) {
	router, _ := newGroupModelAllowlistTestRouter(allowlistAPIKey(true, "claude-sonnet-4.5"), "/v1")

	w := doJSON(t, router, http.MethodPost, "/v1/messages", `{"model":"claude-opus-4.6"}`)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"not_found_error"`) {
		t.Fatalf("expected Anthropic not_found_error format, got %s", w.Body.String())
	}
}

func TestGroupModelAllowlistGeminiDeniedGoogleFormat(t *testing.T) {
	router, _ := newGroupModelAllowlistTestRouter(allowlistAPIKey(true, "gemini-2.5-pro"), "/v1beta")

	w := doJSON(t, router, http.MethodPost, "/v1beta/models/gemini-2.5-flash:generateContent", `{}`)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"NOT_FOUND"`) {
		t.Fatalf("expected Google NOT_FOUND format, got %s", w.Body.String())
	}
}

func TestGroupModelAllowlistGeminiWildcardAndPrefixForms(t *testing.T) {
	router, _ := newGroupModelAllowlistTestRouter(allowlistAPIKey(true, "gemini-2.5-*"), "/v1beta")

	w := doJSON(t, router, http.MethodPost, "/v1beta/models/models/gemini-2.5-flash:generateContent", `{}`)
	if w.Code != http.StatusOK {
		t.Fatalf("models/ prefixed model should match wildcard entry, got %d: %s", w.Code, w.Body.String())
	}
}

func TestGroupModelAllowlistGeminiGetModelRouteParam(t *testing.T) {
	router, _ := newGroupModelAllowlistTestRouter(allowlistAPIKey(true, "gemini-2.5-pro"), "/v1beta")

	req := httptest.NewRequest(http.MethodGet, "/v1beta/models/gemini-2.5-flash", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for unlisted GET model, got %d", w.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/v1beta/models/gemini-2.5-pro", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for listed GET model, got %d", w.Code)
	}
}

func TestGroupModelAllowlistRealtimeQueryParameter(t *testing.T) {
	router, _ := newGroupModelAllowlistTestRouter(allowlistAPIKey(true, "grok-4.6"), "/v1")

	req := httptest.NewRequest(http.MethodGet, "/v1/realtime?model=grok-4.6", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for listed realtime model, got %d: %s", w.Code, w.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/v1/realtime?model=grok-4.20", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for unlisted realtime model, got %d", w.Code)
	}
}

func TestGroupModelAllowlistMultipartModelField(t *testing.T) {
	router, _ := newGroupModelAllowlistTestRouter(allowlistAPIKey(true, "gpt-image-1"), "/v1")

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	_ = writer.WriteField("model", "gpt-image-1")
	_ = writer.WriteField("prompt", "cat")
	_ = writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/v1/images/edits", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestGroupModelAllowlistMultipartDenied(t *testing.T) {
	router, _ := newGroupModelAllowlistTestRouter(allowlistAPIKey(true, "gpt-image-1"), "/v1")

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	_ = writer.WriteField("model", "dall-e-3")
	_ = writer.WriteField("prompt", "cat")
	_ = writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/v1/images/edits", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestGroupModelAllowlistNoModelPassesThrough(t *testing.T) {
	router, _ := newGroupModelAllowlistTestRouter(allowlistAPIKey(true, "claude-sonnet-4.5"), "/v1")

	w := doJSON(t, router, http.MethodPost, "/v1/messages", `{"input":"no model field"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 when no model extractable, got %d: %s", w.Code, w.Body.String())
	}
}

func TestGroupModelAllowlistBodyRestoredForHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(ContextKeyAPIKey), allowlistAPIKey(true, "claude-sonnet-4.5"))
		c.Next()
	})
	router.Use(GroupModelAllowlist())
	router.POST("/v1/messages", func(c *gin.Context) {
		body, err := io.ReadAll(c.Request.Body)
		if err != nil {
			t.Errorf("handler reread body: %v", err)
			c.Status(http.StatusInternalServerError)
			return
		}
		if !strings.Contains(string(body), `"claude-sonnet-4.5"`) {
			t.Errorf("handler saw wrong body: %s", body)
		}
		c.Status(http.StatusOK)
	})

	w := doJSON(t, router, http.MethodPost, "/v1/messages", `{"model":"claude-sonnet-4.5","input":"hi"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestGroupModelAllowlistWebSocketUpgradeSkipsBodyExtraction(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(ContextKeyAPIKey), allowlistAPIKey(true, "claude-sonnet-4.5"))
		c.Next()
	})
	router.Use(GroupModelAllowlist())
	router.GET("/v1/responses", func(c *gin.Context) {
		c.Status(http.StatusSwitchingProtocols)
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/responses", nil)
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Connection", "Upgrade")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusSwitchingProtocols {
		t.Fatalf("expected WS upgrade to reach handler, got %d: %s", w.Code, w.Body.String())
	}
}

// 伪造 WebSocket Upgrade 头的普通请求不得绕过白名单校验。
func TestGroupModelAllowlistForgedUpgradeHeaderDoesNotBypass(t *testing.T) {
	router, calls := newGroupModelAllowlistTestRouter(allowlistAPIKey(true, "claude-sonnet-4.5"), "/v1")

	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{"model":"claude-opus-4.6"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Connection", "Upgrade")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 despite forged upgrade headers, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "claude-opus-4.6") {
		t.Fatalf("expected blocked model in message, got %s", w.Body.String())
	}
	if len(*calls) != 0 {
		t.Fatalf("expected handler not to run, got %v", *calls)
	}
}

// Grok Realtime 升级请求的模型在查询参数里，携带 Upgrade 头也必须校验。
func TestGroupModelAllowlistRealtimeUpgradeStillChecksQueryModel(t *testing.T) {
	router, _ := newGroupModelAllowlistTestRouter(allowlistAPIKey(true, "grok-4.6"), "/v1")

	req := httptest.NewRequest(http.MethodGet, "/v1/realtime?model=grok-4.20", nil)
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Connection", "Upgrade")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for unlisted realtime model with upgrade headers, got %d: %s", w.Code, w.Body.String())
	}
}

// Live 入口的 handler 只读 session.model：准入必须以 session.model 为准，
// 与顶层 model 不一致时按实际执行模型校验。
func TestGroupModelAllowlistLiveUsesSessionModel(t *testing.T) {
	router, _ := newGroupModelAllowlistTestRouter(allowlistAPIKey(true, "gpt-realtime-model"), "/v1")

	// session.model 不在白名单：即使顶层 model 命中也必须拒绝。
	w := doJSON(t, router, http.MethodPost, "/v1/live", `{"model":"gpt-realtime-model","session":{"model":"blocked-model"},"sdp":"v=0"}`)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 when session.model is blocked, got %d: %s", w.Code, w.Body.String())
	}

	// session.model 命中：顶层 model 不在白名单也不影响（handler 不读它）。
	w = doJSON(t, router, http.MethodPost, "/v1/live", `{"model":"blocked-model","session":{"model":"gpt-realtime-model"},"sdp":"v=0"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 when session.model is allowlisted, got %d: %s", w.Code, w.Body.String())
	}
}

func TestGroupModelAllowlistMarksRejectionReasons(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(ContextKeyAPIKey), allowlistAPIKey(true, "claude-sonnet-4.5"))
		c.Next()
	})
	router.Use(GroupModelAllowlist())
	router.POST("/v1/responses", func(c *gin.Context) { c.Status(http.StatusOK) })

	w := doJSON(t, router, http.MethodPost, "/v1/responses", `{"model":"gpt-5.4"}`)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
	// IngressRejectModelNotAllowed 与业务限流原因由 ops 中间件在真实链路落日志，
	// 此处至少验证拒绝原因可从 context 读取（MarkIngressRejected 生效）。
}

func TestGroupModelAllowlistNilGroupPasses(t *testing.T) {
	router, calls := newGroupModelAllowlistTestRouter(&service.APIKey{}, "/v1")
	w := doJSON(t, router, http.MethodPost, "/v1/messages", `{"model":"whatever"}`)
	if w.Code != http.StatusOK || len(*calls) != 1 {
		t.Fatalf("expected passthrough for key without group, got %d", w.Code)
	}
}

// encoding/json 绑定系 handler（如批量生图 ShouldBindJSON）对键名大小写不敏感：
// {"Model":"blocked"} 会绑定为 blocked，准入必须拒绝。
func TestGroupModelAllowlistCaseVariantModelKeyRejected(t *testing.T) {
	router, calls := newGroupModelAllowlistTestRouter(allowlistAPIKey(true, "claude-sonnet-4.5"), "/v1")

	w := doJSON(t, router, http.MethodPost, "/v1/responses", `{"Model":"claude-opus-4.6"}`)
	if w.Code != http.StatusNotFound {
		t.Fatalf("case-variant model key must be validated, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "claude-opus-4.6") {
		t.Fatalf("expected blocked model in message, got %s", w.Body.String())
	}
	if len(*calls) != 0 {
		t.Fatalf("expected handler not to run, got %v", *calls)
	}
}

// encoding/json 绑定对重复键取末值、gjson 取首个：两个值都必须通过校验。
func TestGroupModelAllowlistDuplicateModelKeysRejected(t *testing.T) {
	router, _ := newGroupModelAllowlistTestRouter(allowlistAPIKey(true, "claude-sonnet-4.5"), "/v1")

	w := doJSON(t, router, http.MethodPost, "/v1/responses", `{"model":"claude-sonnet-4.5","model":"claude-opus-4.6"}`)
	if w.Code != http.StatusNotFound {
		t.Fatalf("duplicate model keys must all be validated, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "claude-opus-4.6") {
		t.Fatalf("expected the disallowed duplicate to be reported, got %s", w.Body.String())
	}
}

// 图片编辑的 multipart 解析器对重复 model 字段取末值：全部字段值都必须校验。
func TestGroupModelAllowlistMultipartDuplicateModelFieldsRejected(t *testing.T) {
	router, _ := newGroupModelAllowlistTestRouter(allowlistAPIKey(true, "gpt-image-1"), "/v1")

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	_ = writer.WriteField("model", "gpt-image-1")
	_ = writer.WriteField("model", "gpt-image-1.5")
	_ = writer.WriteField("prompt", "cat")
	_ = writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/v1/images/edits", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("duplicate multipart model fields must all be validated, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "gpt-image-1.5") {
		t.Fatalf("expected the disallowed field value to be reported, got %s", w.Body.String())
	}
}

// 全部候选都在白名单内时放行（重复但同值的字段不误伤）。
func TestGroupModelAllowlistDuplicateIdenticalModelsAllowed(t *testing.T) {
	router, calls := newGroupModelAllowlistTestRouter(allowlistAPIKey(true, "gpt-image-1"), "/v1")

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	_ = writer.WriteField("model", "gpt-image-1")
	_ = writer.WriteField("model", "gpt-image-1")
	_ = writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/v1/images/edits", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("identical duplicate fields should pass, got %d: %s", w.Code, w.Body.String())
	}
	if len(*calls) != 1 {
		t.Fatalf("expected handler to run once, got %v", *calls)
	}
}

func TestGroupModelAllowlistJSONWithMultipartContentType(t *testing.T) {
	tests := []struct {
		name   string
		body   string
		status int
	}{
		{"BOM blocked model", "\xef\xbb\xbf" + `{"model":"blocked"}`, http.StatusNotFound},
		{"single blocked model", `{"model":"blocked"}`, http.StatusNotFound},
		{"blocked last duplicate", `{"model":"allowed","model":"blocked"}`, http.StatusNotFound},
		{"blocked first duplicate", `{"model":"blocked","model":"allowed"}`, http.StatusNotFound},
		{"blocked case variant", `{"model":"allowed","Model":"blocked"}`, http.StatusNotFound},
		{"single allowed model", `{"model":"allowed"}`, http.StatusOK},
		{"identical allowed models", `{"model":"allowed","model":"allowed"}`, http.StatusOK},
		{"unrelated nested model", `{"model":"allowed","input":{"model":"blocked"}}`, http.StatusOK},
	}
	for _, path := range []string{"/v1/responses", "/v1/chat/completions"} {
		t.Run(path, func(t *testing.T) {
			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					router, calls := newGroupModelAllowlistTestRouter(allowlistAPIKey(true, "allowed"), "/v1")
					req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(tt.body))
					req.Header.Set("Content-Type", "multipart/form-data; boundary=x")
					w := httptest.NewRecorder()
					router.ServeHTTP(w, req)

					if w.Code != tt.status {
						t.Fatalf("expected %d, got %d: %s", tt.status, w.Code, w.Body.String())
					}
					if tt.status == http.StatusNotFound {
						if len(*calls) != 0 {
							t.Fatalf("blocked JSON must not reach the handler, got %v", *calls)
						}
						if !strings.Contains(w.Body.String(), "blocked") {
							t.Fatalf("expected blocked model in error, got %s", w.Body.String())
						}
					} else if len(*calls) != 1 {
						t.Fatalf("allowed JSON must reach the handler once, got %v", *calls)
					}
				})
			}
		})
	}
}

func TestGroupModelAllowlistNormalizesBeforeAdmission(t *testing.T) {
	for _, path := range []string{"/v1/messages", "/v1/responses", "/v1/chat/completions"} {
		t.Run(path, func(t *testing.T) {
			router, calls := newGroupModelAllowlistTestRouter(allowlistAPIKey(true, "allowed"), "/v1")
			blocked := doJSON(t, router, http.MethodPost, path, "\xef\xbb\xbf"+`{"model":"blocked"}`)
			if blocked.Code != http.StatusNotFound || len(*calls) != 0 {
				t.Fatalf("BOM bypass: %d %v", blocked.Code, *calls)
			}
			admitted := doJSON(t, router, http.MethodPost, path, "\xef\xbb\xbf"+`{"model":"allowed","input":"raw`+"\x01"+`control"}`)
			if admitted.Code != http.StatusOK || len(*calls) != 1 {
				t.Fatalf("valid normalized request rejected: %d %s", admitted.Code, admitted.Body.String())
			}
		})
	}
}

func TestGroupModelAllowlistJSONProfileCannotBypassNormalization(t *testing.T) {
	router, calls := newGroupModelAllowlistTestRouter(allowlistAPIKey(true, "allowed"), "/v1")
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader("\xef\xbb\xbf"+`{"model":"blocked"}`))
	req.Header.Set("Content-Type", `application/json; profile="multipart/foo"`)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)
	if response.Code != http.StatusNotFound || len(*calls) != 0 {
		t.Fatalf("JSON media-type parameter bypass: %d %v", response.Code, *calls)
	}
}

func TestGroupModelAllowlistNormalizedBodyRespectsConfiguredBudget(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, limit := range []int64{48, 128} {
		router := gin.New()
		router.Use(func(c *gin.Context) { c.Set(string(ContextKeyAPIKey), allowlistAPIKey(true, "allowed")); c.Next() })
		router.Use(GroupModelAllowlist(limit))
		router.POST("/v1/responses", func(c *gin.Context) {
			body, err := httputil.ReadRequestBodyWithPrealloc(c.Request)
			if err != nil {
				t.Fatal(err)
			}
			if bytes.Contains(body, []byte{1}) || !bytes.Contains(body, []byte(`\u0001`)) {
				t.Fatalf("downstream body not normalized: %q", body)
			}
			c.Status(http.StatusOK)
		})
		response := doJSON(t, router, http.MethodPost, "/v1/responses", `{"model":"allowed","input":"`+strings.Repeat("\x01", 8)+`"}`)
		want := http.StatusOK
		if limit == 48 {
			want = http.StatusRequestEntityTooLarge
		}
		if response.Code != want {
			t.Fatalf("limit=%d: got %d want %d: %s", limit, response.Code, want, response.Body.String())
		}
	}
}

func TestGroupModelAllowlistPreservesMultipartBinary(t *testing.T) {
	for _, preamble := range []string{"", "{}\r\n", "{\"model\":\"blocked\"}\r\n"} {
		t.Run(preamble, func(t *testing.T) {
			var body bytes.Buffer
			body.WriteString(preamble)
			writer := multipart.NewWriter(&body)
			_ = writer.WriteField("model", "allowed")
			part, err := writer.CreateFormFile("image", "fixture.png")
			if err != nil {
				t.Fatal(err)
			}
			payload := []byte{0xef, 0xbb, 0xbf, '"', 0, 1, 2, '"', 13, 10, 255}
			_, _ = part.Write(payload)
			_ = writer.Close()
			original := append([]byte(nil), body.Bytes()...)
			for _, limit := range []int64{4096, int64(len(original))} {
				router := gin.New()
				router.Use(func(c *gin.Context) { c.Set(string(ContextKeyAPIKey), allowlistAPIKey(true, "allowed")); c.Next() })
				router.Use(GroupModelAllowlist(limit))
				router.POST("/v1/images/edits", func(c *gin.Context) {
					forwarded, err := httputil.ReadRequestBodyWithPrealloc(c.Request)
					if err != nil {
						t.Fatal(err)
					}
					if !bytes.Equal(original, forwarded) {
						t.Fatal("multipart binary changed during admission")
					}
					reader := multipart.NewReader(bytes.NewReader(forwarded), writer.Boundary())
					model, err := reader.NextPart()
					if err != nil {
						t.Fatal(err)
					}
					value, err := io.ReadAll(model)
					if err != nil || string(value) != "allowed" {
						t.Fatalf("multipart model changed: %q %v", value, err)
					}
					file, err := reader.NextPart()
					if err != nil {
						t.Fatal(err)
					}
					uploaded, err := io.ReadAll(file)
					if err != nil || !bytes.Equal(payload, uploaded) {
						t.Fatalf("multipart payload changed: %x %v", uploaded, err)
					}
					c.Status(http.StatusOK)
				})
				req := httptest.NewRequest(http.MethodPost, "/v1/images/edits", bytes.NewReader(original))
				req.Header.Set("Content-Type", writer.FormDataContentType())
				response := httptest.NewRecorder()
				router.ServeHTTP(response, req)
				if response.Code != http.StatusOK {
					t.Fatalf("multipart denied at limit %d: %d %s", limit, response.Code, response.Body.String())
				}
			}
		})
	}
}

func TestGroupModelAllowlistRepeatedBOMMatchesHandler(t *testing.T) {
	for _, contentType := range []string{"application/json", "multipart/form-data; boundary=x"} {
		for _, path := range []string{"/v1/messages", "/v1/responses", "/v1/chat/completions"} {
			for _, model := range []string{"allowed", "blocked"} {
				t.Run(contentType+path+model, func(t *testing.T) {
					called := false
					router := gin.New()
					router.Use(func(c *gin.Context) { c.Set(string(ContextKeyAPIKey), allowlistAPIKey(true, "allowed")); c.Next() })
					router.Use(GroupModelAllowlist(1024))
					router.POST(path, func(c *gin.Context) {
						called = true
						body, err := httputil.ReadRequestBodyWithPrealloc(c.Request)
						if err != nil {
							t.Fatal(err)
						}
						normalized, err := httputil.NormalizeLenientJSONRequestBody(body, 1024)
						if err != nil {
							t.Fatal(err)
						}
						var parsed struct {
							Model string `json:"model"`
						}
						if err := json.Unmarshal(normalized, &parsed); err != nil {
							t.Fatal(err)
						}
						if parsed.Model != "allowed" {
							t.Fatalf("blocked model reached handler: %s", parsed.Model)
						}
						c.Status(http.StatusOK)
					})
					req := httptest.NewRequest(http.MethodPost, path, strings.NewReader("\xef\xbb\xbf\xef\xbb\xbf"+`{"model":"`+model+`"}`))
					req.Header.Set("Content-Type", contentType)
					response := httptest.NewRecorder()
					router.ServeHTTP(response, req)
					want := http.StatusOK
					if model == "blocked" {
						want = http.StatusNotFound
					}
					if response.Code != want || called != (model == "allowed") {
						t.Fatalf("status %d, called=%v, want %d: %s", response.Code, called, want, response.Body.String())
					}
				})
			}
		}
	}
}
