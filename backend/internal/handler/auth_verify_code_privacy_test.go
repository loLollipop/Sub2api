package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type verifyCodePrivacyUserRepo struct {
	service.UserRepository
	exists, alias bool
	err           error
	calls         int
}

func (r *verifyCodePrivacyUserRepo) ExistsByEmail(context.Context, string) (bool, error) {
	r.calls++
	return r.exists, r.err
}

func (r *verifyCodePrivacyUserRepo) ExistsByEmailAlias(context.Context, string) (bool, error) {
	return r.alias, r.err
}

func TestSendVerifyCodeDoesNotDiscloseRegisteredEmail(t *testing.T) {
	for _, tc := range []struct {
		name          string
		exists, alias bool
	}{
		{name: "exact email", exists: true},
		{name: "email alias", alias: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &verifyCodePrivacyUserRepo{exists: tc.exists, alias: tc.alias}
			rec := sendVerifyCodePrivacyRequest(t, repo, nil)
			require.Equal(t, http.StatusOK, rec.Code)
			require.JSONEq(t, `{"code":0,"message":"success","data":{"message":"Verification code sent successfully","countdown":60}}`, rec.Body.String())
			// No email queue is configured: reaching the send path would fail.
			require.Equal(t, 1, repo.calls)
		})
	}
}

func TestSendVerifyCodePreservesOtherFailures(t *testing.T) {
	for _, tc := range []struct {
		name     string
		repo     *verifyCodePrivacyUserRepo
		settings map[string]string
		status   int
		lookups  int
	}{
		{name: "registration disabled", repo: &verifyCodePrivacyUserRepo{exists: true}, settings: map[string]string{service.SettingKeyRegistrationEnabled: "false"}, status: http.StatusForbidden},
		{name: "captcha required", repo: &verifyCodePrivacyUserRepo{exists: true}, settings: map[string]string{service.SettingKeyTurnstileEnabled: "true"}, status: http.StatusServiceUnavailable},
		{name: "database failure", repo: &verifyCodePrivacyUserRepo{err: errors.New("database unavailable")}, status: http.StatusServiceUnavailable, lookups: 1},
		{name: "unconfigured queue", repo: &verifyCodePrivacyUserRepo{}, status: http.StatusInternalServerError, lookups: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := sendVerifyCodePrivacyRequest(t, tc.repo, tc.settings)
			require.Equal(t, tc.status, rec.Code, rec.Body.String())
			require.Equal(t, tc.lookups, tc.repo.calls)
			require.NotContains(t, rec.Body.String(), "Verification code sent successfully")
		})
	}
}

func sendVerifyCodePrivacyRequest(t *testing.T, repo *verifyCodePrivacyUserRepo, overrides map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{}
	settings := map[string]string{service.SettingKeyRegistrationEnabled: "true"}
	for key, value := range overrides {
		settings[key] = value
	}
	settingSvc := service.NewSettingService(&oauthPendingFlowSettingRepoStub{values: settings}, cfg)
	authSvc := service.NewAuthService(nil, repo, nil, nil, cfg, settingSvc, nil, nil, nil, nil, nil, nil, nil)
	h := &AuthHandler{authService: authSvc}
	router := gin.New()
	router.POST("/send-verify-code", h.SendVerifyCode)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/send-verify-code", strings.NewReader(`{"email":"member@example.test"}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)
	return rec
}
