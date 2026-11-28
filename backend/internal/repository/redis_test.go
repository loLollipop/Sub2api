package repository

import (
	"context"
	"net"
	"strconv"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestBuildRedisOptions(t *testing.T) {
	cfg := &config.Config{
		Redis: config.RedisConfig{
			Host:                "localhost",
			Port:                6379,
			Username:            "app-user",
			Password:            "secret",
			DB:                  2,
			DialTimeoutSeconds:  5,
			ReadTimeoutSeconds:  3,
			WriteTimeoutSeconds: 4,
			PoolSize:            100,
			MinIdleConns:        10,
		},
	}

	opts := buildRedisOptions(cfg)
	require.Equal(t, "localhost:6379", opts.Addr)
	require.Equal(t, "app-user", opts.Username)
	require.Equal(t, "secret", opts.Password)
	require.Equal(t, 2, opts.DB)
	require.Equal(t, 5*time.Second, opts.DialTimeout)
	require.Equal(t, 3*time.Second, opts.ReadTimeout)
	require.Equal(t, 4*time.Second, opts.WriteTimeout)
	require.Equal(t, 100, opts.PoolSize)
	require.Equal(t, 10, opts.MinIdleConns)
	require.Nil(t, opts.TLSConfig)

	// Test case with TLS enabled
	cfgTLS := &config.Config{
		Redis: config.RedisConfig{
			Host:      "localhost",
			EnableTLS: true,
		},
	}
	optsTLS := buildRedisOptions(cfgTLS)
	require.NotNil(t, optsTLS.TLSConfig)
	require.Equal(t, "localhost", optsTLS.TLSConfig.ServerName)
}

func TestBuildRedisPoolOptions(t *testing.T) {
	r := config.RedisConfig{PoolSize: 512, MinIdleConns: 8, MaxIdleConns: 64,
		PoolFIFO: true, PoolTimeoutMS: 1000, ConnMaxIdleTimeSeconds: 300, ConnMaxLifetimeSeconds: 1800}
	opts := buildRedisOptions(&config.Config{Redis: r})
	require.Equal(t, 64, opts.MaxIdleConns)
	require.True(t, opts.PoolFIFO)
	require.Equal(t, time.Second, opts.PoolTimeout)
	require.Equal(t, 5*time.Minute, opts.ConnMaxIdleTime)
	require.Equal(t, 30*time.Minute, opts.ConnMaxLifetime)
	require.Equal(t, 3*time.Minute, opts.ConnMaxLifetimeJitter)
	r.MinIdleConns, r.PoolSize = 256, 4096
	require.Equal(t, 256, redisMaxIdleConns(r))
	r.MinIdleConns, r.PoolSize = 0, 32
	require.Equal(t, 32, redisMaxIdleConns(r))
	r.MaxIdleConns = 0
	require.Zero(t, redisMaxIdleConns(r))
	client := redis.NewClient(buildRedisOptions(&config.Config{}))
	t.Cleanup(func() { _ = client.Close() })
	require.Equal(t, time.Duration(-1), client.Options().ConnMaxIdleTime)
}

func TestProvideReasoningRedisPreservesOrdinaryOptions(t *testing.T) {
	cfg := &config.Config{Redis: config.RedisConfig{
		Host: "localhost", Port: 6379, Username: "app-user", Password: "secret", DB: 2,
		DialTimeoutSeconds: 5, ReadTimeoutSeconds: 3, WriteTimeoutSeconds: 4,
		PoolSize: 100, MinIdleConns: 10, MaxIdleConns: 6, EnableTLS: true,
	}}
	ordinaryOptions := buildRedisOptions(cfg)
	reasoning := ProvideReasoningRedis(cfg)
	t.Cleanup(func() { _ = reasoning.Close() })
	opts := reasoning.Options()
	require.False(t, ordinaryOptions.ContextTimeoutEnabled)
	require.Equal(t, 10, ordinaryOptions.MinIdleConns)
	require.Equal(t, 10, ordinaryOptions.MaxIdleConns)
	require.True(t, opts.ContextTimeoutEnabled)
	require.Zero(t, opts.MinIdleConns)
	require.Equal(t, 6, opts.MaxIdleConns, "reasoning pool has no prewarm floor")
	require.Equal(t, ordinaryOptions.Addr, opts.Addr)
	require.Equal(t, ordinaryOptions.Username, opts.Username)
	require.Equal(t, ordinaryOptions.Password, opts.Password)
	require.Equal(t, ordinaryOptions.DB, opts.DB)
	require.Equal(t, ordinaryOptions.DialTimeout, opts.DialTimeout)
	require.Equal(t, ordinaryOptions.ReadTimeout, opts.ReadTimeout)
	require.Equal(t, ordinaryOptions.WriteTimeout, opts.WriteTimeout)
	require.Equal(t, ordinaryOptions.PoolSize, opts.PoolSize)
	require.Equal(t, ordinaryOptions.TLSConfig, opts.TLSConfig)
	require.Zero(t, reasoning.PoolStats().TotalConns)
}

func TestProvideGatewayCacheIsolatesReasoningClientAndLifecycle(t *testing.T) {
	ordinaryServer := miniredis.RunT(t)
	reasoningServer := miniredis.RunT(t)
	ordinary := redis.NewClient(&redis.Options{Addr: ordinaryServer.Addr()})
	t.Cleanup(func() { _ = ordinary.Close() })
	host, portText, err := net.SplitHostPort(reasoningServer.Addr())
	require.NoError(t, err)
	port, err := strconv.Atoi(portText)
	require.NoError(t, err)
	reasoning := ProvideReasoningRedis(&config.Config{Redis: config.RedisConfig{Host: host, Port: port}})
	t.Cleanup(func() { _ = reasoning.Close() })
	cache := ProvideGatewayCache(ordinary, reasoning)
	ctx := context.Background()

	require.NoError(t, ordinaryServer.Set(buildSessionKey(1, "session"), "42"))
	id, err := cache.GetSessionAccountID(ctx, 1, "session")
	require.NoError(t, err)
	require.Equal(t, int64(42), id)
	require.NoError(t, cache.SetReasoningContent(ctx, "item", "reasoning", time.Minute))
	content, err := cache.GetReasoningContent(ctx, "item")
	require.NoError(t, err)
	require.Equal(t, "reasoning", content)
	require.True(t, reasoningServer.Exists(reasoningContentPrefix+"item"))
	require.False(t, ordinaryServer.Exists(reasoningContentPrefix+"item"))
	require.Equal(t, time.Minute, reasoningServer.TTL(reasoningContentPrefix+"item"))

	// Closing the dedicated pool must not close the ordinary cache client.
	require.NoError(t, reasoning.Close())
	_, err = cache.GetReasoningContent(ctx, "item")
	require.ErrorIs(t, err, redis.ErrClosed)
	id, err = cache.GetSessionAccountID(ctx, 1, "session")
	require.NoError(t, err)
	require.Equal(t, int64(42), id)
	require.False(t, ordinary.Options().ContextTimeoutEnabled)
}
