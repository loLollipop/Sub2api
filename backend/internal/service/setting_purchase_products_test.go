//go:build unit

package service

import (
	"context"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type purchaseProductsRepo struct {
	SettingRepository
	values map[string]string
}

func (r *purchaseProductsRepo) GetValue(_ context.Context, key string) (string, error) {
	value, ok := r.values[key]
	if !ok {
		return "", ErrSettingNotFound
	}
	return value, nil
}

func (r *purchaseProductsRepo) SetMultiple(_ context.Context, values map[string]string) error {
	for key, value := range values {
		r.values[key] = value
	}
	return nil
}

func (r *purchaseProductsRepo) GetMultiple(_ context.Context, keys []string) (map[string]string, error) {
	out := map[string]string{}
	for _, key := range keys {
		if value, ok := r.values[key]; ok {
			out[key] = value
		}
	}
	return out, nil
}

func (r *purchaseProductsRepo) GetAll(_ context.Context) (map[string]string, error) {
	return r.values, nil
}

func TestPurchaseSubscriptionProductsNormalization(t *testing.T) {
	input := map[string]string{"10": " https://shop.example.test/ten?a=1#details ", "20": "", "30": "http://shop.example.test:8080/thirty", "50": "https://[::1]:8443/fifty", "100": "https://shop.example.test/hundred"}
	out, err := NormalizePurchaseSubscriptionProducts(input)
	require.NoError(t, err)
	require.Len(t, out, 4)
	require.Equal(t, "https://shop.example.test/ten?a=1#details", out["10"])
	require.NotContains(t, out, "20")
	out["10"] = "changed"
	require.Equal(t, " https://shop.example.test/ten?a=1#details ", input["10"])
	out, err = NormalizePurchaseSubscriptionProducts(nil)
	require.NoError(t, err)
	require.NotNil(t, out)
	require.Empty(t, out)
}

func TestPurchaseSubscriptionProductsRejectUnsafeConfiguration(t *testing.T) {
	for _, raw := range []string{
		"javascript:alert(1)", "data:text/html,hi", "/relative", "//shop.example.test/ten", "https:///ten",
		"https://user:password@shop.example.test/ten", "https://user@shop.example.test/ten",
		"https://shop.example.test/\\ten", "https://shop.example.test/\nten", "https://shop.example.test/\tten",
		"https://bad..host/ten", "https://-bad.host/ten", "https://bad_.host/ten", "https://999.1.1.1/ten", "https://127.1/ten",
		"https://shop.example.test:/ten", "https://shop.example.test:0/ten", "https://shop.example.test:65536/ten", "https://shop.example.test:bad/ten",
		"https://[bad]:80/ten", "https://[127.0.0.1]/ten", "https://shop.example.test/" + strings.Repeat("x", 2048),
	} {
		t.Run(raw, func(t *testing.T) {
			_, err := NormalizePurchaseSubscriptionProducts(map[string]string{"10": raw})
			require.Error(t, err)
		})
	}
	for _, key := range []string{"", "1", "010", "10.0", "200", " 10"} {
		_, err := NormalizePurchaseSubscriptionProducts(map[string]string{key: ""})
		require.Error(t, err)
	}
	_, err := NormalizePurchaseSubscriptionProducts(map[string]string{"10": "", "20": "", "30": "", "50": "", "100": "", "200": ""})
	require.Error(t, err)
}

func TestPurchaseSubscriptionProductsDefaultParsePublicAndInjection(t *testing.T) {
	ctx := context.Background()
	repo := &purchaseProductsRepo{values: map[string]string{}}
	svc := NewSettingService(repo, &config.Config{})
	require.NoError(t, svc.InitializeDefaultSettings(ctx))
	require.Equal(t, "{}", repo.values[SettingKeyPurchaseSubscriptionProducts])
	for _, raw := range []string{"", "null", "{", `[]`, `{"10":42}`, `{"200":"https://shop.example.test/item"}`, `{"10":"javascript:alert(1)"}`} {
		repo.values[SettingKeyPurchaseSubscriptionProducts] = raw
		repo.values[SettingKeyPurchaseSubscriptionURL] = "https://shop.example.test/legacy"
		settings, err := svc.GetAllSettings(ctx)
		require.NoError(t, err)
		require.NotNil(t, settings.PurchaseSubscriptionProducts)
		require.Empty(t, settings.PurchaseSubscriptionProducts)
		public, err := svc.GetPublicSettings(ctx)
		require.NoError(t, err)
		require.Empty(t, public.PurchaseSubscriptionProducts)
		injected, err := svc.GetPublicSettingsForInjection(ctx)
		require.NoError(t, err)
		require.NotNil(t, injected.(*PublicSettingsInjectionPayload).PurchaseSubscriptionProducts)
		require.Empty(t, injected.(*PublicSettingsInjectionPayload).PurchaseSubscriptionProducts)
		require.Equal(t, raw, repo.values[SettingKeyPurchaseSubscriptionProducts], "reads must not rewrite existing data")
	}
}
