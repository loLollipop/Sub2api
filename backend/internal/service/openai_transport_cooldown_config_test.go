//go:build unit

package service

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestOpenAITransportCooldownConfigPreservesFailover(t *testing.T) {
	for _, disabled := range []bool{false, true} {
		for _, passthrough := range []bool{false, true} {
			t.Run(fmt.Sprintf("disabled=%t/passthrough=%t", disabled, passthrough), func(t *testing.T) {
				repo := &openaiTransportAccountRepoStub{}
				svc := &OpenAIGatewayService{accountRepo: repo, cfg: &config.Config{
					Gateway: config.GatewayConfig{DisableOpenAITransportErrorCooldown: disabled},
				}}
				account := &Account{ID: 8541, Platform: PlatformOpenAI}
				c, rec := newOpenAITransportErrTestContext()
				err := svc.handleOpenAIUpstreamTransportError(context.Background(), c, account,
					errors.New("dial tcp: no such host"), passthrough)
				var failover *UpstreamFailoverError
				require.ErrorAs(t, err, &failover)
				require.Equal(t, 502, failover.StatusCode)
				require.Empty(t, rec.Body.String())
				require.False(t, c.Writer.Written())
				require.Equal(t, !disabled, svc.isOpenAIAccountRuntimeBlocked(account))
				if disabled {
					require.Empty(t, repo.tempUnschedCalls)
				} else {
					require.Len(t, repo.tempUnschedCalls, 1)
				}
			})
		}
	}
}

func TestOpenAITransportCooldownDisabledPreservesPriorState(t *testing.T) {
	repo := &openaiTransportAccountRepoStub{}
	svc := &OpenAIGatewayService{accountRepo: repo, cfg: &config.Config{}}
	account := &Account{ID: 8541, Platform: PlatformOpenAI, Status: StatusActive, Schedulable: false}
	svc.tempUnscheduleOpenAITransportError(context.Background(), account, "old failure")
	require.Len(t, repo.tempUnschedCalls, 1)
	prior := repo.tempUnschedCalls[0]
	account.TempUnschedulableUntil = &prior.until
	account.TempUnschedulableReason = prior.reason
	block, _ := svc.openaiAccountRuntimeBlockUntil.Load(account.ID)
	svc.cfg.Gateway.DisableOpenAITransportErrorCooldown = true
	svc.tempUnscheduleOpenAITransportErrorFor(context.Background(), account, "new failure", time.Second)
	require.Equal(t, []tempUnschedCall{prior}, repo.tempUnschedCalls)
	require.False(t, account.Schedulable)
	require.Equal(t, prior.until, *account.TempUnschedulableUntil)
	require.Equal(t, prior.reason, account.TempUnschedulableReason)
	after, _ := svc.openaiAccountRuntimeBlockUntil.Load(account.ID)
	require.Equal(t, block, after)
}
