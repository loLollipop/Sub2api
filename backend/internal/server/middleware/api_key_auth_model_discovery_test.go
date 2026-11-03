//go:build unit

package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestModelDiscoveryAdmission(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, google := range []bool{false, true} {
		name := "standard"
		paths := []string{"/models", "/models/gpt-5", "/v1/models", "/v1/models/gpt-5", "/openai/v1/models", "/v1beta/models", "/v1beta/models/gemini-pro", "/backend-api/codex/models", "/antigravity/models", "/antigravity/v1/models", "/antigravity/v1beta/models/gemini-pro"}
		if google {
			name = "google"
			paths = []string{"/v1beta/models", "/v1beta/models/gemini-pro", "/antigravity/v1beta/models/gemini-pro"}
		}
		t.Run(name, func(t *testing.T) {
			cases := []struct {
				name   string
				edit   func(*service.APIKey)
				status int
			}{
				{"zero_balance", func(k *service.APIKey) { k.User.Balance = 0 }, 200},
				{"negative_balance", func(k *service.APIKey) { k.User.Balance = -1 }, 200},
				{"no_subscription", func(k *service.APIKey) { k.Group.SubscriptionType = service.SubscriptionTypeSubscription }, 200},
				{"disabled_key", func(k *service.APIKey) { k.Status = service.StatusDisabled }, 401},
				{"expired_status", func(k *service.APIKey) { k.Status = service.StatusAPIKeyExpired }, 403},
				{"expired_timestamp", func(k *service.APIKey) { past := time.Now().Add(-time.Hour); k.ExpiresAt = &past }, 403},
				{"exhausted_status", func(k *service.APIKey) { k.Status = service.StatusAPIKeyQuotaExhausted }, 429},
				{"exhausted_quota", func(k *service.APIKey) { k.Quota = 1; k.QuotaUsed = 1 }, 429},
				{"inactive_user", func(k *service.APIKey) { k.User.Status = service.StatusDisabled }, 401},
				{"disabled_group", func(k *service.APIKey) { k.Group.Status = service.StatusDisabled }, 403},
				{"exclusive_permission", func(k *service.APIKey) { k.Group.IsExclusive = true }, 403},
				{"ip_restriction", func(k *service.APIKey) { k.IPWhitelist = []string{"192.0.2.2"} }, 403},
			}
			for _, tc := range cases {
				for _, path := range paths {
					t.Run(tc.name+path, func(t *testing.T) {
						key := &service.APIKey{ID: 101, Key: "catalog-test-key", UserID: 7, Status: service.StatusActive,
							User:  &service.User{ID: 7, Status: service.StatusActive, Role: service.RoleUser, Balance: 10},
							Group: &service.Group{ID: 42, Status: service.StatusActive, Hydrated: true},
						}
						key.GroupID = &key.Group.ID
						tc.edit(key)
						cfg := &config.Config{RunMode: config.RunModeStandard}
						repo := &stubApiKeyRepo{getByKey: func(context.Context, string) (*service.APIKey, error) { return key, nil }}
						svc := service.NewAPIKeyService(repo, nil, nil, nil, nil, nil, cfg)
						subSvc := service.NewSubscriptionService(nil, &stubUserSubscriptionRepo{getActive: func(context.Context, int64, int64) (*service.UserSubscription, error) {
							t.Error("catalog discovery must not load or maintain a paid subscription")
							return nil, errors.New("no subscription")
						}}, nil, nil, cfg)
						t.Cleanup(subSvc.Stop)
						auth := gin.HandlerFunc(NewAPIKeyAuthMiddleware(svc, subSvc, cfg))
						if google {
							auth = APIKeyAuthWithSubscriptionGoogle(svc, subSvc, cfg)
						}
						r := gin.New()
						r.GET(path, auth, func(c *gin.Context) {
							got, ok := GetAPIKeyFromContext(c)
							require.True(t, ok)
							require.Equal(t, key.ID, got.ID)
							c.Status(http.StatusOK)
						})
						w := httptest.NewRecorder()
						req := httptest.NewRequest(http.MethodGet, path, nil)
						req.Header.Set("x-api-key", key.Key)
						r.ServeHTTP(w, req)
						require.Equal(t, tc.status, w.Code, w.Body.String())
					})
				}
			}
		})
	}
}

func TestModelDiscoveryNeverAdmitsGeneration(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, google := range []bool{false, true} {
		for _, path := range []string{"/v1/responses", "/v1/chat/completions", "/v1beta/models/gemini-pro:generateContent", "/v1beta/models/gemini-pro:streamGenerateContent", "/models"} {
			t.Run(path, func(t *testing.T) {
				cfg := &config.Config{RunMode: config.RunModeStandard}
				key := &service.APIKey{ID: 101, Key: "catalog-test-key", UserID: 7, Status: service.StatusActive, User: &service.User{ID: 7, Role: service.RoleUser, Status: service.StatusActive}}
				svc := service.NewAPIKeyService(&stubApiKeyRepo{getByKey: func(context.Context, string) (*service.APIKey, error) { return key, nil }}, nil, nil, nil, nil, nil, cfg)
				auth := gin.HandlerFunc(NewAPIKeyAuthMiddleware(svc, nil, cfg))
				if google {
					auth = APIKeyAuthWithSubscriptionGoogle(svc, nil, cfg)
				}
				r := gin.New()
				r.POST(path, auth, func(c *gin.Context) { t.Error("zero balance generation admitted"); c.Status(200) })
				w := httptest.NewRecorder()
				req := httptest.NewRequest(http.MethodPost, path, nil)
				req.Header.Set("x-api-key", key.Key)
				r.ServeHTTP(w, req)
				require.Equal(t, http.StatusTooManyRequests, w.Code, w.Body.String())
			})
		}
	}
	for _, path := range []string{"/v1beta/models/gemini:generateContent", "/v1/models/gpt/extra", "/v1/models-other", "/models//", "/v1/images/generations"} {
		require.False(t, isModelDiscoveryRequest(http.MethodGet, path), path)
	}
}
