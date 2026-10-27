//go:build unit

package service

import (
	"context"
	"errors"
	"net/http"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestDeepSeekDirectTransportCooldownRecoversAfterBriefRefusedConnection(t *testing.T) {
	for _, cause := range []error{syscall.ECONNREFUSED, errors.New("dial tcp 192.0.2.42:443: connect: connection refused")} {
		repo := &openaiTransportAccountRepoStub{}
		svc := &OpenAIGatewayService{accountRepo: repo}
		c, rec := newOpenAITransportErrTestContext()
		account := &Account{ID: 1, Platform: PlatformDeepseek, Type: AccountTypeAPIKey}
		before := time.Now()
		err := svc.handleOpenAIUpstreamTransportError(context.Background(), c, account, cause, false)
		var failover *UpstreamFailoverError
		require.ErrorAs(t, err, &failover)
		require.Equal(t, http.StatusBadGateway, failover.StatusCode)
		require.Len(t, repo.tempUnschedCalls, 1)
		require.WithinDuration(t, before.Add(deepSeekDirectConnectionRefusedCooldown), repo.tempUnschedCalls[0].until, time.Second)
		require.True(t, repo.tempUnschedCalls[0].until.After(before), "the persisted cooldown must retain a nonzero backoff")
		require.Empty(t, rec.Body.String(), "the handler still owns the final protocol error")
	}
}

func TestDeepSeekDirectTransportCooldownKeepsDurableFaultProtection(t *testing.T) {
	proxyID := int64(9)
	for _, tc := range []struct {
		account *Account
		cause   error
	}{
		{&Account{ID: 1, Platform: PlatformDeepseek, Type: AccountTypeAPIKey, ProxyID: &proxyID}, syscall.ECONNREFUSED},
		{&Account{ID: 2, Platform: PlatformDeepseek, Type: AccountTypeAPIKey}, errors.New("username/password authentication failed")},
		{&Account{ID: 3, Platform: PlatformDeepseek, Type: AccountTypeAPIKey}, errors.New("no such host")},
		{&Account{ID: 4, Platform: PlatformOpenAI, Type: AccountTypeAPIKey}, syscall.ECONNREFUSED},
		{&Account{ID: 5, Platform: PlatformDeepseek, Type: AccountTypeAPIKey, Proxy: &Proxy{}}, syscall.ECONNREFUSED},
		{&Account{ID: 6, Platform: PlatformDeepseek, Type: AccountTypeOAuth}, syscall.ECONNREFUSED},
		{&Account{ID: 7, Platform: PlatformDeepseek, Type: AccountTypeAPIKey}, syscall.ENETUNREACH},
	} {
		repo := &openaiTransportAccountRepoStub{}
		svc := &OpenAIGatewayService{accountRepo: repo}
		c, _ := newOpenAITransportErrTestContext()
		before := time.Now()
		_ = svc.handleOpenAIUpstreamTransportError(context.Background(), c, tc.account, tc.cause, false)
		require.Len(t, repo.tempUnschedCalls, 1)
		require.WithinDuration(t, before.Add(openAITransportErrorTempUnschedDuration), repo.tempUnschedCalls[0].until, time.Second)
	}
}

func TestDeepSeekDirectTransportCooldownNeverReplaysAcceptedPluginWork(t *testing.T) {
	repo := &openaiTransportAccountRepoStub{}
	svc := &OpenAIGatewayService{accountRepo: repo}
	c, _ := newOpenAITransportErrTestContext()
	cause := &PluginTransportError{Code: "REFUSED", Message: "connection refused", RequestSent: true}
	err := svc.handleOpenAIUpstreamTransportError(context.Background(), c, &Account{ID: 1, Platform: PlatformDeepseek, Type: AccountTypeAPIKey}, cause, false)
	require.Same(t, cause, err)
	require.Empty(t, repo.tempUnschedCalls)
	var replay *UpstreamFailoverError
	require.False(t, errors.As(err, &replay))
}
