package repository

import (
	"context"
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestUsageInflightCacheRoundTrip(t *testing.T) {
	server := miniredis.RunT(t)
	cache := NewUsageInflightCache(redis.NewClient(&redis.Options{Addr: server.Addr()}))
	ctx := context.Background()
	first := 40
	snap := service.UsageInflightSnapshot{
		RequestID: "req-1", UserID: 9, APIKeyID: 3, Model: "deepseek-v4",
		StartedAt: time.Now().Add(-time.Second), FirstTokenMs: &first,
		InputTokens: 12, OutputTokens: 4, ReservedAmount: 1.25, Email: "a@b.c",
	}
	if err := cache.Save(ctx, snap); err != nil {
		t.Fatal(err)
	}
	rows, err := cache.ListUser(ctx, 9)
	if err != nil || len(rows) != 1 || rows[0].ReservedAmount != 1.25 || rows[0].FirstTokenMs == nil || *rows[0].FirstTokenMs != 40 {
		t.Fatalf("rows=%+v err=%v", rows, err)
	}
	all, err := cache.ListAll(ctx)
	if err != nil || len(all) != 1 {
		t.Fatalf("all=%+v err=%v", all, err)
	}
	if err := cache.Delete(ctx, 9, "req-1"); err != nil {
		t.Fatal(err)
	}
	rows, err = cache.ListUser(ctx, 9)
	if err != nil || len(rows) != 0 {
		t.Fatalf("after delete rows=%+v err=%v", rows, err)
	}
	if err := cache.Save(ctx, snap); err != nil {
		t.Fatal(err)
	}
	rows, _ = cache.ListUser(ctx, 9)
	if len(rows) != 0 {
		t.Fatalf("tombstone should block rewrite, got %+v", rows)
	}
}

type inflightReadHook struct {
	once      sync.Once
	afterRead func()
}

func (h *inflightReadHook) DialHook(next redis.DialHook) redis.DialHook {
	return func(ctx context.Context, network, addr string) (net.Conn, error) { return next(ctx, network, addr) }
}
func (h *inflightReadHook) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		err := next(ctx, cmd)
		if err == nil && cmd.Name() == "hgetall" {
			h.once.Do(h.afterRead)
		}
		return err
	}
}
func (h *inflightReadHook) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return func(ctx context.Context, cmds []redis.Cmder) error {
		err := next(ctx, cmds)
		if err == nil {
			for _, cmd := range cmds {
				if cmd.Name() == "hgetall" {
					h.once.Do(h.afterRead)
					break
				}
			}
		}
		return err
	}
}

func TestUsageInflightCleanupPreservesConcurrentHeartbeatAndNewUserRows(t *testing.T) {
	for _, all := range []bool{false, true} {
		for _, empty := range []bool{false, true} {
			t.Run(fmt.Sprintf("all=%v/empty=%v", all, empty), func(t *testing.T) {
				server := miniredis.RunT(t)
				rdb := redis.NewClient(&redis.Options{Addr: server.Addr()})
				t.Cleanup(func() { _ = rdb.Close() })
				cache := NewUsageInflightCache(rdb)
				ctx := context.Background()
				snap := service.UsageInflightSnapshot{RequestID: "client:abc", UserID: 9, StartedAt: time.Now().Add(-2 * time.Minute)}
				if !empty {
					if err := cache.Save(ctx, snap); err != nil {
						t.Fatal(err)
					}
				}
				if err := rdb.SAdd(ctx, usageInflightUsersKey, "9").Err(); err != nil {
					t.Fatal(err)
				}
				rdb.AddHook(&inflightReadHook{afterRead: func() {
					snap.UpdatedAt = time.Now()
					if err := cache.Save(ctx, snap); err != nil {
						t.Error(err)
					}
				}})
				if all {
					_, _ = cache.ListAll(ctx)
				} else {
					_, _ = cache.ListUser(ctx, 9)
				}
				rows, err := cache.ListUser(ctx, 9)
				if err != nil || len(rows) != 1 {
					t.Fatalf("heartbeat erased: rows=%+v err=%v", rows, err)
				}
				member, err := rdb.SIsMember(ctx, usageInflightUsersKey, "9").Result()
				if err != nil || !member {
					t.Fatalf("concurrent user lost: member=%v err=%v", member, err)
				}
			})
		}
	}
}

func TestUsageInflightConcurrentSaveDeleteNeverResurrects(t *testing.T) {
	server := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	cache := NewUsageInflightCache(rdb)
	ctx := context.Background()
	for i := 0; i < 40; i++ {
		snap := service.UsageInflightSnapshot{RequestID: fmt.Sprintf("client:%d", i), UserID: 9, StartedAt: time.Now()}
		var workers sync.WaitGroup
		workers.Add(2)
		go func() {
			defer workers.Done()
			if err := cache.Save(ctx, snap); err != nil {
				t.Error(err)
			}
		}()
		go func() {
			defer workers.Done()
			if err := cache.Delete(ctx, snap.UserID, snap.RequestID); err != nil {
				t.Error(err)
			}
		}()
		workers.Wait()
	}
	rows, err := cache.ListAll(ctx)
	if err != nil || len(rows) != 0 {
		t.Fatalf("late save resurrected rows: %+v err=%v", rows, err)
	}
}
