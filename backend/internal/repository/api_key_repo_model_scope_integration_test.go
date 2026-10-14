//go:build integration

package repository

import "github.com/Wei-Shaw/sub2api/internal/service"

func (s *APIKeyRepoSuite) TestModelScopeRoundtripAndConcurrentUsage() {
	user := s.mustCreateUser("key-scope@example.com")
	key := &service.APIKey{UserID: user.ID, Key: "sk-model-scope", Name: "scope", Status: service.StatusActive, ModelScope: "cn", Quota: 100}
	s.Require().NoError(s.repo.Create(s.ctx, key))
	stale, err := s.repo.GetByID(s.ctx, key.ID)
	s.Require().NoError(err)
	s.Require().Equal("cn", stale.ModelScope)
	auth, err := s.repo.GetByKeyForAuth(s.ctx, key.Key)
	s.Require().NoError(err)
	s.Require().Equal("cn", auth.ModelScope)
	_, err = s.repo.IncrementQuotaUsed(s.ctx, key.ID, 30)
	s.Require().NoError(err)
	s.Require().NoError(s.repo.IncrementRateLimitUsage(s.ctx, key.ID, 42))
	stale.ModelScope = "openai"
	s.Require().NoError(s.repo.Update(s.ctx, stale, service.APIKeyUpdateFields{ModelScope: true}))
	got, err := s.repo.GetByID(s.ctx, key.ID)
	s.Require().NoError(err)
	s.Require().Equal("openai", got.ModelScope)
	s.Require().InDelta(30, got.QuotaUsed, 1e-9)
	s.Require().InDelta(42, got.Usage5h, 1e-9)
	stale.ModelScope = "other"
	stale.Name = "rename"
	s.Require().NoError(s.repo.Update(s.ctx, stale, service.APIKeyUpdateFields{Name: true}))
	got, err = s.repo.GetByKey(s.ctx, key.Key)
	s.Require().NoError(err)
	s.Require().Equal("openai", got.ModelScope)
}
