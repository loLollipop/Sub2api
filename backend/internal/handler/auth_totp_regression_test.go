//go:build unit

package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/pquerna/otp/totp"
	"github.com/stretchr/testify/require"
)

func TestOAuthExistingAccountTOTPChallengesAcrossProviders(t *testing.T) {
	for _, provider := range []string{"github", "google", "oidc"} {
		for _, setting := range []string{"enabled", "read_error"} {
			t.Run(provider+"/"+setting, func(t *testing.T) {
				cache := &oauthPendingFlowTotpCacheStub{}
				h, client := newOAuthPendingFlowTestHandlerWithDependencies(t, oauthPendingFlowTestHandlerOptions{
					settingValues: map[string]string{service.SettingKeyTotpEnabled: "true"}, totpCache: cache,
				})
				user, err := client.User.Create().SetEmail("existing@example.com").SetUsername("existing").SetPasswordHash("hash").SetRole(service.RoleUser).SetStatus(service.StatusActive).SetTotpEnabled(true).Save(context.Background())
				require.NoError(t, err)
				if setting == "read_error" {
					// Rebuild the auth service with the same ent-backed repositories and failing settings.
					cfg := &config.Config{JWT: config.JWTConfig{Secret: "test-secret", AccessTokenExpireMinutes: 60}}
					h.settingSvc = service.NewSettingService(&loginTOTPSettingRepo{errKey: service.SettingKeyTotpEnabled}, cfg)
					h.authService = service.NewAuthService(client, &oauthPendingFlowUserRepo{client: client}, nil, &oauthPendingFlowRefreshTokenCacheStub{}, cfg, h.settingSvc, nil, nil, nil, nil, nil, nil, nil)
				}
				rec := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(rec)
				c.Request = httptest.NewRequest(http.MethodGet, "/callback", nil)
				if provider == "oidc" {
					handled := h.tryOIDCVerifiedEmailFastPath(c, "/auth/oidc/callback", "/profile", service.PendingAuthIdentityKey{ProviderType: provider, ProviderKey: provider, ProviderSubject: "subject"}, user.Email, "existing", nil)
					require.True(t, handled)
				} else {
					h.emailOAuthCallbackWithProfile(c, provider, config.EmailOAuthProviderConfig{}, "/auth/oauth/callback", "/profile", &emailOAuthProfile{Subject: "subject", Email: user.Email, EmailVerified: true})
				}
				require.Equal(t, http.StatusFound, rec.Code)
				location, err := url.Parse(rec.Header().Get("Location"))
				require.NoError(t, err)
				fragment, err := url.ParseQuery(location.Fragment)
				require.NoError(t, err)
				require.Equal(t, "1", fragment.Get("requires_2fa"))
				require.NotEmpty(t, fragment.Get("temp_token"))
				require.NotContains(t, location.Fragment, "access_token")
				require.NotContains(t, location.Fragment, "refresh_token")
				session, err := cache.GetLoginSession(context.Background(), fragment.Get("temp_token"))
				require.NoError(t, err)
				require.Equal(t, user.ID, session.UserID)
			})
		}
	}
}

func TestPendingOAuthExchangeTOTPConsumesSessionAcrossProviders(t *testing.T) {
	for _, provider := range []string{"linuxdo", "dingtalk", "wechat", "oidc"} {
		t.Run(provider, func(t *testing.T) {
			cache := &oauthPendingFlowTotpCacheStub{}
			h, client := newOAuthPendingFlowTestHandlerWithDependencies(t, oauthPendingFlowTestHandlerOptions{settingValues: map[string]string{service.SettingKeyTotpEnabled: "true"}, totpCache: cache})
			ctx := context.Background()
			user, err := client.User.Create().SetEmail("existing@example.com").SetUsername("existing").SetPasswordHash("hash").SetRole(service.RoleUser).SetStatus(service.StatusActive).SetTotpEnabled(true).Save(ctx)
			require.NoError(t, err)
			pending, err := client.PendingAuthSession.Create().SetSessionToken("pending").SetIntent("login").SetProviderType(provider).SetProviderKey(provider).SetProviderSubject("subject").SetTargetUserID(user.ID).SetResolvedEmail(user.Email).SetBrowserSessionKey("browser").SetLocalFlowState(map[string]any{oauthCompletionResponseKey: map[string]any{"redirect": "/profile", "access_token": "legacy-token", "refresh_token": "legacy-refresh"}}).SetExpiresAt(time.Now().Add(time.Minute)).Save(ctx)
			require.NoError(t, err)
			exchange := func() *httptest.ResponseRecorder {
				rec := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(rec)
				req := httptest.NewRequest(http.MethodPost, "/exchange", nil)
				req.AddCookie(&http.Cookie{Name: oauthPendingSessionCookieName, Value: encodeCookieValue("pending")})
				req.AddCookie(&http.Cookie{Name: oauthPendingBrowserCookieName, Value: encodeCookieValue("browser")})
				c.Request = req
				h.ExchangePendingOAuthCompletion(c)
				return rec
			}
			rec := exchange()
			require.Equal(t, http.StatusOK, rec.Code)
			data := decodeJSONResponseData(t, rec)
			require.Equal(t, true, data["requires_2fa"])
			require.NotEmpty(t, data["temp_token"])
			require.NotContains(t, data, "access_token")
			require.NotContains(t, data, "refresh_token")
			stored, err := client.PendingAuthSession.Get(ctx, pending.ID)
			require.NoError(t, err)
			require.NotNil(t, stored.ConsumedAt)
			rec = exchange()
			require.NotEqual(t, http.StatusOK, rec.Code)
			require.NotContains(t, rec.Body.String(), "access_token")
		})
	}
}

func TestPasswordLoginTOTPUnavailableDoesNotIssueTokens(t *testing.T) {
	user := newLoginTOTPUser("totp@example.com")
	// Build the same entrypoint without a TOTP service.
	cfg := &config.Config{}
	cfg.JWT.Secret = "test-secret"
	settings := service.NewSettingService(&loginTOTPSettingRepo{values: map[string]string{service.SettingKeyTotpEnabled: "true"}}, cfg)
	auth := service.NewAuthService(nil, &loginTOTPUserRepo{user: user}, nil, nil, cfg, settings, nil, nil, nil, nil, nil, nil, nil)
	h := NewAuthHandler(cfg, auth, nil, settings, nil, nil, nil, nil)
	router := gin.New()
	router.POST("/login", h.Login)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(`{"email":"totp@example.com","password":"correct-horse"}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusServiceUnavailable, rec.Code)
	require.NotContains(t, rec.Body.String(), "access_token")
}

func TestLogin2FAReplayAcrossChallengesDoesNotIssueTokens(t *testing.T) {
	cache := &oauthPendingFlowTotpCacheStub{}
	h, client := newOAuthPendingFlowTestHandlerWithDependencies(t, oauthPendingFlowTestHandlerOptions{settingValues: map[string]string{service.SettingKeyTotpEnabled: "true"}, totpCache: cache})
	secret := "JBSWY3DPEHPK3PXP"
	user, err := client.User.Create().SetEmail("owner@example.com").SetUsername("owner").SetPasswordHash("hash").SetRole(service.RoleUser).SetStatus(service.StatusActive).SetTotpEnabled(true).SetTotpSecretEncrypted(secret).Save(context.Background())
	require.NoError(t, err)
	code, err := totp.GenerateCode(secret, time.Now())
	require.NoError(t, err)
	login := func(token string) *httptest.ResponseRecorder {
		body, _ := json.Marshal(Login2FARequest{TempToken: token, TotpCode: code})
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodPost, "/2fa", strings.NewReader(string(body)))
		c.Request.Header.Set("Content-Type", "application/json")
		h.Login2FA(c)
		return rec
	}
	first, err := h.totpService.CreateLoginSession(context.Background(), user.ID, user.Email)
	require.NoError(t, err)
	second, err := h.totpService.CreateLoginSession(context.Background(), user.ID, user.Email)
	require.NoError(t, err)
	rec := login(first)
	require.Equal(t, http.StatusOK, rec.Code)
	require.NotEmpty(t, decodeJSONResponseData(t, rec)["access_token"])
	rec = login(second)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "TOTP_CODE_REPLAYED")
	require.NotContains(t, rec.Body.String(), "access_token")
	rec = login(first)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.NotContains(t, rec.Body.String(), "access_token")
	third, err := h.totpService.CreateLoginSession(context.Background(), user.ID, user.Email)
	require.NoError(t, err)
	cache.loginMarkerErr = errors.New("redis marker storage failed")
	rec = login(third)
	require.Equal(t, http.StatusServiceUnavailable, rec.Code)
	require.NotContains(t, rec.Body.String(), "access_token")
	cache.loginMarkerErr = nil
	cache.usedLoginCodes = nil
	cache.consumeLoginErr = errors.New("redis challenge consumption failed")
	rec = login(third)
	require.Equal(t, http.StatusServiceUnavailable, rec.Code)
	require.NotContains(t, rec.Body.String(), "access_token")

}
