package redis

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// Lua script for a “sliding window” on an ordered set.
// KEYS[1] = key
// ARGV[1] = now_ms
// ARGV[2] = window_ms
// ARGV[3] = limit
// ARGV[4] = member (unique)
const luaSlidingWindow = `
local key = KEYS[1]
local now = tonumber(ARGV[1])
local window = tonumber(ARGV[2])
local limit = tonumber(ARGV[3])
local member = ARGV[4]

-- remove expired
redis.call('ZREMRANGEBYSCORE', key, 0, now - window)
-- add current hit
redis.call('ZADD', key, 'NX', now, member)
local count = redis.call('ZCARD', key)
-- keep TTL ~ window
redis.call('PEXPIRE', key, window)

if count > limit then
  local earliest = redis.call('ZRANGE', key, 0, 0, 'WITHSCORES')
  local earliestScore = tonumber(earliest[2]) or (now - window)
  local retry_ms = window - (now - earliestScore)
  if retry_ms < 0 then retry_ms = 0 end
  return {0, count, retry_ms}
end
return {1, count, 0}
`

type SlidingWindowLimiter struct {
	rdb    *redis.Client
	prefix string
	limit  int
	window time.Duration
	script *redis.Script
}

func NewSlidingWindowLimiter(
	rdb *redis.Client,
	prefix string,
	limit int,
	window time.Duration,
) *SlidingWindowLimiter {
	return &SlidingWindowLimiter{
		rdb:    rdb,
		prefix: prefix,
		limit:  limit,
		window: window,
		script: redis.NewScript(luaSlidingWindow),
	}
}

func (l *SlidingWindowLimiter) key(suffix string) string {
	return fmt.Sprintf("%s:%s", l.prefix, suffix)
}

func (l *SlidingWindowLimiter) Allow(ctx context.Context, suffix string) (allowed bool, current int64, retryAfter time.Duration, err error) {
	key := l.key(suffix)
	nowMs := time.Now().UnixNano() / 1e6
	winMs := l.window.Milliseconds()
	member := randomHex(12)

	res, err := l.script.Run(
		ctx,
		l.rdb,
		[]string{key},
		nowMs, winMs, l.limit, member,
	).Result()
	if err != nil {
		return false, 0, 0, err
	}

	arr, ok := res.([]any)
	if !ok || len(arr) != 3 {
		return false, 0, 0, fmt.Errorf("bad script result: %v", res)
	}

	allowed = toInt(arr[0]) == 1
	current = toInt(arr[1])
	retryAfter = time.Duration(toInt(arr[2])) * time.Millisecond

	return
}

func toInt(v any) int64 {
	switch t := v.(type) {
	case int64:
		return t
	case int:
		return int64(t)
	case float64:
		return int64(t)
	case string:
		var x int64
		fmt.Sscan(t, &x)
		return x
	default:
		return 0
	}
}

func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
