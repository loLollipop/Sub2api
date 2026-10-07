//go:build unit

package admin_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	publichandler "github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/handler/admin"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type purchaseSettingsRepo struct {
	service.SettingRepository
	values map[string]string
	writes int
}

func (r *purchaseSettingsRepo) GetValue(_ context.Context, key string) (string, error) {
	return r.values[key], nil
}

func (r *purchaseSettingsRepo) SetMultiple(_ context.Context, values map[string]string) error {
	r.writes++
	for key, value := range values {
		r.values[key] = value
	}
	return nil
}

func (r *purchaseSettingsRepo) GetMultiple(_ context.Context, keys []string) (map[string]string, error) {
	out := map[string]string{}
	for _, key := range keys {
		if value, ok := r.values[key]; ok {
			out[key] = value
		}
	}
	return out, nil
}

func (r *purchaseSettingsRepo) GetAll(_ context.Context) (map[string]string, error) {
	out := map[string]string{}
	for key, value := range r.values {
		out[key] = value
	}
	return out, nil
}

func purchaseSettingsRequest(t *testing.T, handle gin.HandlerFunc, method string, body map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	raw, err := json.Marshal(body)
	require.NoError(t, err)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(method, "/settings", bytes.NewReader(raw))
	c.Request.Header.Set("Content-Type", "application/json")
	handle(c)
	return rec
}

func TestPurchaseProductsSettingsPartialUpdates(t *testing.T) {
	ctx := context.Background()
	repo := &purchaseSettingsRepo{values: map[string]string{
		service.SettingKeyPurchaseSubscriptionProducts: `{"10":"https://shop.example.test/ten"}`,
		service.SettingKeyPurchaseSubscriptionURL:      " https://shop.example.test/legacy ",
		service.SettingKeySiteName:                     "Keep site",
	}}
	svc := service.NewSettingService(repo, &config.Config{Default: config.DefaultConfig{UserConcurrency: 5}})
	h := admin.NewSettingHandler(svc, nil, nil, nil, nil, nil, nil)
	rec := purchaseSettingsRequest(t, h.UpdateSettings, http.MethodPut, map[string]any{"purchase_subscription_enabled": false})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Equal(t, `{"10":"https://shop.example.test/ten"}`, repo.values[service.SettingKeyPurchaseSubscriptionProducts])
	require.Equal(t, " https://shop.example.test/legacy ", repo.values[service.SettingKeyPurchaseSubscriptionURL])
	require.Equal(t, "Keep site", repo.values[service.SettingKeySiteName])

	rec = purchaseSettingsRequest(t, h.UpdateSettings, http.MethodPut, map[string]any{"purchase_subscription_products": map[string]string{}})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Equal(t, "{}", repo.values[service.SettingKeyPurchaseSubscriptionProducts])
	writes := repo.writes
	rec = purchaseSettingsRequest(t, h.UpdateSettings, http.MethodPut, map[string]any{"purchase_subscription_enabled": true})
	require.Equal(t, http.StatusBadRequest, rec.Code, "legacy scalar must not enable any product")
	require.Equal(t, writes, repo.writes)

	products := map[string]string{
		"10": " https://shop.example.test/ten?source=site#details ", "20": "http://shop.example.test/twenty",
		"30": "https://shop.example.test/thirty", "50": "https://shop.example.test/fifty", "100": "https://shop.example.test/hundred",
	}
	rec = purchaseSettingsRequest(t, h.UpdateSettings, http.MethodPut, map[string]any{"purchase_subscription_enabled": true, "purchase_subscription_products": products})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	products["10"] = "https://shop.example.test/ten?source=site#details"
	for _, endpoint := range []gin.HandlerFunc{h.GetSettings, publichandler.NewSettingHandler(svc, "test").GetPublicSettings} {
		rec = purchaseSettingsRequest(t, endpoint, http.MethodGet, nil)
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		var body struct {
			Data struct {
				Products map[string]string `json:"purchase_subscription_products"`
			} `json:"data"`
		}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
		require.Equal(t, products, body.Data.Products)
	}
	injected, err := svc.GetPublicSettingsForInjection(ctx)
	require.NoError(t, err)
	encoded, err := json.Marshal(injected)
	require.NoError(t, err)
	var payload map[string]any
	require.NoError(t, json.Unmarshal(encoded, &payload))
	require.Contains(t, payload, "purchase_subscription_products")
	require.Equal(t, products, injected.(*service.PublicSettingsInjectionPayload).PurchaseSubscriptionProducts)

	writes = repo.writes
	before := repo.values[service.SettingKeyPurchaseSubscriptionProducts]
	rec = purchaseSettingsRequest(t, h.UpdateSettings, http.MethodPut, map[string]any{"purchase_subscription_products": map[string]string{}})
	require.Equal(t, http.StatusBadRequest, rec.Code, "cannot clear all products while enabled")
	require.Equal(t, writes, repo.writes)
	require.Equal(t, before, repo.values[service.SettingKeyPurchaseSubscriptionProducts])
}

func TestPurchaseProductsSettingsRejectInvalidWithoutWriting(t *testing.T) {
	for _, products := range []map[string]string{
		{"": ""}, {"200": "https://shop.example.test/item"}, {"10": "javascript:alert(1)"},
		{"10": "https://user:secret@shop.example.test/item"}, {"10": "https://bad..host/item"},
		{"10": "https://shop.example.test:99999/item"}, {"10": "https://shop.example.test/\\item"},
	} {
		repo := &purchaseSettingsRepo{values: map[string]string{service.SettingKeyPurchaseSubscriptionProducts: "{}"}}
		svc := service.NewSettingService(repo, &config.Config{})
		h := admin.NewSettingHandler(svc, nil, nil, nil, nil, nil, nil)
		rec := purchaseSettingsRequest(t, h.UpdateSettings, http.MethodPut, map[string]any{"purchase_subscription_products": products})
		require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
		require.Zero(t, repo.writes)
		require.Equal(t, "{}", repo.values[service.SettingKeyPurchaseSubscriptionProducts])
	}
}

func TestPurchaseProductsSettingsDisabledPartialAndCorruptReads(t *testing.T) {
	repo := &purchaseSettingsRepo{values: map[string]string{}}
	svc := service.NewSettingService(repo, &config.Config{})
	h := admin.NewSettingHandler(svc, nil, nil, nil, nil, nil, nil)
	rec := purchaseSettingsRequest(t, h.UpdateSettings, http.MethodPut, map[string]any{"purchase_subscription_products": map[string]string{"20": "https://shop.example.test/twenty", "10": " "}})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Equal(t, `{"20":"https://shop.example.test/twenty"}`, repo.values[service.SettingKeyPurchaseSubscriptionProducts])
	rec = purchaseSettingsRequest(t, h.UpdateSettings, http.MethodPut, map[string]any{"purchase_subscription_enabled": true})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	for _, bad := range []string{"{", `{"10":"javascript:alert(1)"}`} {
		repo.values[service.SettingKeyPurchaseSubscriptionProducts] = bad
		rec = purchaseSettingsRequest(t, publichandler.NewSettingHandler(svc, "test").GetPublicSettings, http.MethodGet, nil)
		require.Equal(t, http.StatusOK, rec.Code)
		require.Contains(t, rec.Body.String(), `"purchase_subscription_products":{}`)
		require.NotContains(t, rec.Body.String(), "javascript:")
		require.Equal(t, bad, repo.values[service.SettingKeyPurchaseSubscriptionProducts])
	}
}
