//go:build unit

package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	middleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type expiryAPIKeyRepo struct {
	service.APIKeyRepository
	created []*service.APIKey
}

func (r *expiryAPIKeyRepo) Create(_ context.Context, key *service.APIKey) error {
	key.ID = 1
	r.created = append(r.created, key)
	return nil
}

type expiryUserRepo struct{ service.UserRepository }

func (r *expiryUserRepo) GetByID(_ context.Context, id int64) (*service.User, error) {
	return &service.User{ID: id}, nil
}

func TestAPIKeyCreateAbsoluteExpiryHTTP(t *testing.T) {
	expiry := time.Now().UTC().Add(30 * time.Minute).Truncate(time.Millisecond)
	cfg := &config.Config{}
	repo := &expiryAPIKeyRepo{}
	svc := service.NewAPIKeyService(repo, &expiryUserRepo{}, nil, nil, nil, nil, cfg)
	handler := NewAPIKeyHandler(svc)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 7})
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/keys", strings.NewReader(`{"name":"half hour","expires_at":"`+expiry.Format(time.RFC3339Nano)+`"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	handler.Create(c)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	require.Len(t, repo.created, 1)
	require.Equal(t, expiry, *repo.created[0].ExpiresAt)
	var result struct {
		Data struct {
			ExpiresAt time.Time `json:"expires_at"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &result))
	require.True(t, expiry.Equal(result.Data.ExpiresAt), recorder.Body.String())
}

func TestAPIKeyCreateHTTPRejectsInvalidExpiryBeforeService(t *testing.T) {
	future := time.Now().Add(time.Hour).Format(time.RFC3339)
	for _, body := range []string{
		`{"name":"key","expires_at":"invalid"}`,
		`{"name":"key","expires_at":""}`,
		`{"name":"key","expires_at":"2000-01-01T00:00:00Z"}`,
		`{"name":"key","expires_at":"` + future + `","expires_in_days":1}`,
	} {
		t.Run(body, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 7})
			c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/keys", strings.NewReader(body))
			c.Request.Header.Set("Content-Type", "application/json")
			NewAPIKeyHandler(nil).Create(c)
			require.Equal(t, http.StatusBadRequest, recorder.Code, recorder.Body.String())
		})
	}
}
