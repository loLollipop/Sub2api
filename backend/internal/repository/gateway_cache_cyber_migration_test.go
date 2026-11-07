package repository

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestCyberIndexLegacyScopeAndTTL(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	store := NewGatewayCache(client).(service.CyberSessionBlockStore)
	ctx := context.Background()
	require.NoError(t, client.Set(ctx, cyberSessionScopePrefix+"legacy", "1", time.Hour).Err())
	require.NoError(t, client.Set(ctx, cyberSessionBlockPrefix+"old", "1", time.Hour).Err())
	for _, scope := range []string{"new", "legacy"} {
		require.NoError(t, store.SetCyberSessionBlocked(ctx, service.CyberSessionBlock{APIKeyID: 7, ScopeKey: scope, Keys: []string{"new"}, TranscriptLengths: []int{3}}, time.Hour))
	}
	lengths, err := store.CyberSessionTranscriptLengths(ctx, 7, "legacy")
	require.NoError(t, err)
	require.Equal(t, []int{service.LegacyCyberTranscriptLengths}, lengths, "adding an index must not hide old unindexed blocks")
	lengths, err = store.CyberSessionTranscriptLengths(ctx, 7, "new")
	require.NoError(t, err)
	require.Equal(t, []int{3}, lengths)
	server.FastForward(time.Minute)
	exactTTL := server.TTL(cyberSessionBlockPrefix + "new")
	require.NoError(t, store.SetCyberSessionBlocked(ctx, service.CyberSessionBlock{APIKeyID: 7, ScopeKey: "new", Keys: []string{"new"}, TranscriptLengths: []int{3}}, time.Minute))
	require.Equal(t, exactTTL, server.TTL(cyberSessionBlockPrefix+"new"))
	require.NoError(t, store.SetCyberSessionBlocked(ctx, service.CyberSessionBlock{APIKeyID: 7, ScopeKey: "new", Keys: []string{"short"}, TranscriptLengths: []int{5}}, time.Minute))
	require.GreaterOrEqual(t, server.TTL(cyberSessionTranscriptLengthsKey(7)), server.TTL(cyberSessionBlockPrefix+"new"))
	require.GreaterOrEqual(t, server.TTL(cyberSessionScopePrefix+"new"), server.TTL(cyberSessionBlockPrefix+"new"))
	server.FastForward(time.Hour)
	lengths, err = store.CyberSessionTranscriptLengths(ctx, 7, "legacy")
	require.NoError(t, err)
	require.Empty(t, lengths)
}

func TestCyberIndexWriteFailurePublishesNothing(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	store := NewGatewayCache(client).(service.CyberSessionBlockStore)
	ctx := context.Background()
	require.NoError(t, client.Set(ctx, cyberSessionTranscriptLengthsKey(7), "wrong-type", time.Hour).Err())
	err := store.SetCyberSessionBlocked(ctx, service.CyberSessionBlock{APIKeyID: 7, ScopeKey: "s", Keys: []string{"b"}, TranscriptLengths: []int{3}}, time.Minute)
	require.Error(t, err)
	require.False(t, server.Exists(cyberSessionBlockPrefix+"b"))
	require.False(t, server.Exists(cyberSessionScopePrefix+"s"))
}

func TestCyberIndexConcurrentWritesRetainEveryLength(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	store := NewGatewayCache(client).(service.CyberSessionBlockStore)
	ctx := context.Background()
	var wg sync.WaitGroup
	errs := make(chan error, 24)
	for i := 1; i <= 24; i++ {
		wg.Go(func() {
			errs <- store.SetCyberSessionBlocked(ctx, service.CyberSessionBlock{APIKeyID: 7, ScopeKey: "s", Keys: []string{fmt.Sprintf("b-%d", i)}, TranscriptLengths: []int{i}}, time.Minute)
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	lengths, err := store.CyberSessionTranscriptLengths(ctx, 7, "s")
	require.NoError(t, err)
	require.Len(t, lengths, 24)
	for i := 1; i <= 24; i++ {
		require.Contains(t, lengths, i)
		require.True(t, server.Exists(cyberSessionBlockPrefix+fmt.Sprintf("b-%d", i)))
	}
}
