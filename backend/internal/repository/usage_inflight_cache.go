package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
)

// 在途请求展示，和余额预留不是同一组 key。
//
//	usage:inflight:{uid}           HASH  field=requestID value=JSON
//	usage:inflight:users           SET   有在途请求的 userID，管理员列表只读这个集合
//	usage:inflight:done:{uid}:{id} STRING 结束墓碑，挡住迟到的刷新把行写回来
//
// ponytail: 管理员一次最多展开 500 个用户。超过再改成按用户分页，不要 SCAN。
const (
	usageInflightUsersKey = "usage:inflight:users"
	usageInflightTTL      = time.Hour
	usageInflightDoneTTL  = 2 * time.Minute
	usageInflightUserCap  = 500
)

type usageInflightCache struct {
	rdb *redis.Client
}

func NewUsageInflightCache(rdb *redis.Client) service.UsageInflightStore {
	return &usageInflightCache{rdb: rdb}
}

const usageInflightHeartbeatStale = 60 * time.Second

// usageInflightLive 超过 1 分钟没有心跳的行当结束。旧数据没有 updated_at，就用开始时间。
func usageInflightLive(snap service.UsageInflightSnapshot, now time.Time) bool {
	if snap.ExpiresAtMs > 0 && snap.ExpiresAtMs <= now.UnixMilli() {
		return false
	}
	stamp := snap.UpdatedAt
	if stamp.IsZero() {
		stamp = snap.StartedAt
	}
	return !stamp.IsZero() && now.Sub(stamp) <= usageInflightHeartbeatStale
}

func usageInflightHashKey(userID int64) string {
	return fmt.Sprintf("usage:inflight:{%d}", userID)
}

func usageInflightDoneKey(userID int64, requestID string) string {
	return fmt.Sprintf("usage:inflight:done:{%d}:%s", userID, requestID)
}

var usageInflightSaveScript = redis.NewScript(`
if redis.call('EXISTS', KEYS[3]) ~= 0 then return 0 end
redis.call('HSET', KEYS[1], ARGV[1], ARGV[2])
redis.call('PEXPIRE', KEYS[1], ARGV[3])
redis.call('SADD', KEYS[2], ARGV[4])
return 1
`)

var usageInflightDeleteScript = redis.NewScript(`
redis.call('SET', KEYS[3], '1', 'PX', ARGV[3])
redis.call('HDEL', KEYS[1], ARGV[1])
if redis.call('HLEN', KEYS[1]) == 0 then redis.call('SREM', KEYS[2], ARGV[2]) end
return 1
`)

// Compare the exact value read by the list query. A concurrent heartbeat may
// have refreshed that field, or added another field to this user's hash.
const usageInflightCleanupLua = `
for i = 2, #ARGV, 2 do
  if redis.call('HGET', KEYS[1], ARGV[i]) == ARGV[i+1] then
    redis.call('HDEL', KEYS[1], ARGV[i])
  end
end
if redis.call('HLEN', KEYS[1]) == 0 then redis.call('SREM', KEYS[2], ARGV[1]) end
return 1
`

func usageInflightCleanupArgs(userID int64, expired map[string]string) []any {
	args := []any{strconv.FormatInt(userID, 10)}
	for field, payload := range expired {
		args = append(args, field, payload)
	}
	return args
}

func (c *usageInflightCache) Save(ctx context.Context, snap service.UsageInflightSnapshot) error {
	if c == nil || c.rdb == nil || snap.UserID <= 0 || snap.RequestID == "" {
		return nil
	}
	snap.ExpiresAtMs = time.Now().Add(usageInflightTTL).UnixMilli()
	payload, err := json.Marshal(snap)
	if err != nil {
		return err
	}
	return usageInflightSaveScript.Run(ctx, c.rdb, []string{
		usageInflightHashKey(snap.UserID), usageInflightUsersKey, usageInflightDoneKey(snap.UserID, snap.RequestID),
	}, snap.RequestID, payload, usageInflightTTL.Milliseconds(), strconv.FormatInt(snap.UserID, 10)).Err()
}

func (c *usageInflightCache) Delete(ctx context.Context, userID int64, requestID string) error {
	if c == nil || c.rdb == nil || userID <= 0 || requestID == "" {
		return nil
	}
	return usageInflightDeleteScript.Run(ctx, c.rdb, []string{
		usageInflightHashKey(userID), usageInflightUsersKey, usageInflightDoneKey(userID, requestID),
	}, requestID, strconv.FormatInt(userID, 10), usageInflightDoneTTL.Milliseconds()).Err()
}

func (c *usageInflightCache) ListUser(ctx context.Context, userID int64) ([]service.UsageInflightSnapshot, error) {
	if c == nil || c.rdb == nil || userID <= 0 {
		return nil, nil
	}
	hashKey := usageInflightHashKey(userID)
	raw, err := c.rdb.HGetAll(ctx, hashKey).Result()
	if err != nil || len(raw) == 0 {
		if err == nil {
			_ = c.rdb.Eval(ctx, usageInflightCleanupLua, []string{hashKey, usageInflightUsersKey}, usageInflightCleanupArgs(userID, nil)...).Err()
		}
		return nil, err
	}
	now := time.Now()
	out := make([]service.UsageInflightSnapshot, 0, len(raw))
	expired := make(map[string]string)
	for field, payload := range raw {
		var snap service.UsageInflightSnapshot
		if json.Unmarshal([]byte(payload), &snap) != nil || !usageInflightLive(snap, now) {
			expired[field] = payload
			continue
		}
		out = append(out, snap)
	}
	if len(expired) > 0 {
		_ = c.rdb.Eval(ctx, usageInflightCleanupLua, []string{hashKey, usageInflightUsersKey}, usageInflightCleanupArgs(userID, expired)...).Err()
	}
	return out, nil
}

func (c *usageInflightCache) ListAll(ctx context.Context) ([]service.UsageInflightSnapshot, error) {
	if c == nil || c.rdb == nil {
		return nil, nil
	}
	ids, err := c.rdb.SMembers(ctx, usageInflightUsersKey).Result()
	if err != nil || len(ids) == 0 {
		return nil, err
	}
	if len(ids) > usageInflightUserCap {
		ids = ids[:usageInflightUserCap]
	}
	pipe := c.rdb.Pipeline()
	type inflightHashCmd struct {
		userID int64
		cmd    *redis.MapStringStringCmd
	}
	cmds := make([]inflightHashCmd, 0, len(ids))
	for _, id := range ids {
		userID, convErr := strconv.ParseInt(id, 10, 64)
		if convErr != nil {
			pipe.SRem(ctx, usageInflightUsersKey, id)
			continue
		}
		cmds = append(cmds, inflightHashCmd{userID: userID, cmd: pipe.HGetAll(ctx, usageInflightHashKey(userID))})
	}
	if _, err = pipe.Exec(ctx); err != nil && err != redis.Nil {
		return nil, err
	}
	now := time.Now()
	out := make([]service.UsageInflightSnapshot, 0, len(cmds))
	clean := c.rdb.Pipeline()
	cleanupCount := 0
	for _, item := range cmds {
		raw, cmdErr := item.cmd.Result()
		if cmdErr != nil || len(raw) == 0 {
			if cmdErr == nil || cmdErr == redis.Nil {
				clean.Eval(ctx, usageInflightCleanupLua, []string{usageInflightHashKey(item.userID), usageInflightUsersKey}, usageInflightCleanupArgs(item.userID, nil)...)
				cleanupCount++
			}
			continue
		}
		expired := make(map[string]string)
		for field, payload := range raw {
			var snap service.UsageInflightSnapshot
			if json.Unmarshal([]byte(payload), &snap) != nil || !usageInflightLive(snap, now) {
				expired[field] = payload
				continue
			}
			out = append(out, snap)
		}
		if len(expired) > 0 {
			clean.Eval(ctx, usageInflightCleanupLua, []string{usageInflightHashKey(item.userID), usageInflightUsersKey}, usageInflightCleanupArgs(item.userID, expired)...)
			cleanupCount++
		}
	}
	if cleanupCount > 0 {
		_, _ = clean.Exec(ctx)
	}
	return out, nil
}
