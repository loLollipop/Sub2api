package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoadRedisPoolDefaultsAndExplicitOverrides(t *testing.T) {
	resetViperWithJWTSecret(t)
	cfg, err := Load()
	require.NoError(t, err)
	require.Equal(t, 512, cfg.Redis.PoolSize)
	require.Equal(t, 8, cfg.Redis.MinIdleConns)
	require.Equal(t, 64, cfg.Redis.MaxIdleConns)
	require.True(t, cfg.Redis.PoolFIFO)
	require.Equal(t, 1000, cfg.Redis.PoolTimeoutMS)
	require.Equal(t, 300, cfg.Redis.ConnMaxIdleTimeSeconds)
	require.Equal(t, 1800, cfg.Redis.ConnMaxLifetimeSeconds)

	t.Setenv("REDIS_POOL_SIZE", "4096")
	t.Setenv("REDIS_MIN_IDLE_CONNS", "256")
	t.Setenv("REDIS_POOL_FIFO", "false")
	t.Setenv("REDIS_MAX_IDLE_CONNS", "0")
	t.Setenv("REDIS_POOL_TIMEOUT_MS", "250")
	t.Setenv("REDIS_CONN_MAX_IDLE_TIME_SECONDS", "0")
	t.Setenv("REDIS_CONN_MAX_LIFETIME_SECONDS", "0")
	cfg, err = Load()
	require.NoError(t, err)
	require.Equal(t, 4096, cfg.Redis.PoolSize)
	require.Equal(t, 256, cfg.Redis.MinIdleConns)
	require.Zero(t, cfg.Redis.MaxIdleConns)
	require.False(t, cfg.Redis.PoolFIFO)
	require.Equal(t, 250, cfg.Redis.PoolTimeoutMS)
	require.Zero(t, cfg.Redis.ConnMaxIdleTimeSeconds)
	require.Zero(t, cfg.Redis.ConnMaxLifetimeSeconds)
}

func TestLoadRedisPoolRejectsInvalidSettings(t *testing.T) {
	for _, key := range []string{"REDIS_MAX_IDLE_CONNS", "REDIS_POOL_TIMEOUT_MS", "REDIS_CONN_MAX_IDLE_TIME_SECONDS", "REDIS_CONN_MAX_LIFETIME_SECONDS"} {
		t.Run(key, func(t *testing.T) {
			resetViperWithJWTSecret(t)
			t.Setenv(key, "-1")
			_, err := Load()
			require.Error(t, err)
		})
	}
}
