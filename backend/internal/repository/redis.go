package repository

import (
	"crypto/tls"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"

	"github.com/redis/go-redis/v9"
)

// InitRedis 初始化 Redis 客户端
//
// FIFO 轮转检查连接寿命；空闲连接超过上限时关闭，以便突发后收缩。
// 池耗尽的等待上限独立于读写超时，默认值见 config.go。
func InitRedis(cfg *config.Config) *redis.Client {
	client := redis.NewClient(buildRedisOptions(cfg))
	if cfg.Server.EnableServerTiming {
		client.AddHook(serverTimingRedisHook{})
	}
	return client
}

// ReasoningRedisClient owns a separate pool for best-effort reasoning replay.
// Its caller deadlines must bound socket I/O as well as pool acquisition,
// without changing timeout behavior for billing, locks, or other caches.
type ReasoningRedisClient struct {
	*redis.Client
}

func ProvideReasoningRedis(cfg *config.Config) *ReasoningRedisClient {
	opts := buildRedisOptions(cfg)
	opts.ContextTimeoutEnabled = true
	// Replay is optional; do not prewarm a second set of idle connections.
	opts.MinIdleConns = 0
	reasoningConfig := cfg.Redis
	reasoningConfig.MinIdleConns = 0
	opts.MaxIdleConns = redisMaxIdleConns(reasoningConfig)
	client := redis.NewClient(opts)
	if cfg.Server.EnableServerTiming {
		client.AddHook(serverTimingRedisHook{})
	}
	return &ReasoningRedisClient{Client: client}
}

// buildRedisOptions 构建 Redis 连接选项
// 从配置文件读取连接池和超时参数，支持生产环境调优
func buildRedisOptions(cfg *config.Config) *redis.Options {
	opts := &redis.Options{
		Addr:            cfg.Redis.Address(),
		Username:        cfg.Redis.Username,
		Password:        cfg.Redis.Password,
		DB:              cfg.Redis.DB,
		DialTimeout:     time.Duration(cfg.Redis.DialTimeoutSeconds) * time.Second,  // 建连超时
		ReadTimeout:     time.Duration(cfg.Redis.ReadTimeoutSeconds) * time.Second,  // 读取超时
		WriteTimeout:    time.Duration(cfg.Redis.WriteTimeoutSeconds) * time.Second, // 写入超时
		PoolSize:        cfg.Redis.PoolSize,                                         // 连接池大小
		MinIdleConns:    cfg.Redis.MinIdleConns,                                     // 最小空闲连接
		MaxIdleConns:    redisMaxIdleConns(cfg.Redis),
		PoolFIFO:        cfg.Redis.PoolFIFO,
		PoolTimeout:     time.Duration(cfg.Redis.PoolTimeoutMS) * time.Millisecond,
		ConnMaxIdleTime: redisConnMaxIdleTime(cfg.Redis.ConnMaxIdleTimeSeconds),
	}
	if lifetime := time.Duration(cfg.Redis.ConnMaxLifetimeSeconds) * time.Second; lifetime > 0 {
		opts.ConnMaxLifetime = lifetime
		opts.ConnMaxLifetimeJitter = lifetime / 10
	}

	if cfg.Redis.EnableTLS {
		opts.TLSConfig = &tls.Config{
			MinVersion: tls.VersionTLS12,
			ServerName: cfg.Redis.Host,
		}
	}

	return opts
}

// Normalize the new idle cap against existing explicit pool settings so older
// configurations do not churn their prewarmed connections or fail validation.
func redisMaxIdleConns(r config.RedisConfig) int {
	maxIdle := r.MaxIdleConns
	if maxIdle <= 0 {
		return 0
	}
	if maxIdle < r.MinIdleConns {
		maxIdle = r.MinIdleConns
	}
	if r.PoolSize > 0 && maxIdle > r.PoolSize {
		maxIdle = r.PoolSize
	}
	return maxIdle
}

func redisConnMaxIdleTime(seconds int) time.Duration {
	if seconds <= 0 {
		// go-redis replaces zero with 30 minutes; negative disables idle expiry.
		return -1
	}
	return time.Duration(seconds) * time.Second
}
