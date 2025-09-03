package redis

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

const idemNS = "tixgo:v1:idem"

func KeyIdemHold(eventID int64, idemKey string) string {
	return fmt.Sprintf("%s:holds:%d:%s", idemNS, eventID, idemKey)
}

type IdempotencyStore struct {
	rdb *redis.Client
	ttl time.Duration
}

func NewIdempotencyStore(rdb *redis.Client, ttl time.Duration) *IdempotencyStore {
	return &IdempotencyStore{rdb: rdb, ttl: ttl}
}

func (s *IdempotencyStore) AcquireLock(ctx context.Context, key string, lockTTL time.Duration) (bool, error) {
	return s.rdb.SetNX(ctx, key, "LOCK", lockTTL).Result()
}

func (s *IdempotencyStore) SaveResult(ctx context.Context, key string, jsonPayload string) error {
	val := "RES:" + jsonPayload
	return s.rdb.Set(ctx, key, val, s.ttl).Err()
}

func (s *IdempotencyStore) GetResult(ctx context.Context, key string) (string, bool, error) {
	v, err := s.rdb.Get(ctx, key).Result()
	if err == redis.Nil {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	if strings.HasPrefix(v, "RES:") {
		return strings.TrimPrefix(v, "RES:"), true, nil
	}

	return "", false, nil
}

func (s *IdempotencyStore) IsLocked(ctx context.Context, key string) (bool, error) {
	v, err := s.rdb.Get(ctx, key).Result()
	if err == redis.Nil {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return v == "LOCK", nil
}

func (s *IdempotencyStore) Release(ctx context.Context, key string) error {
	return s.rdb.Del(ctx, key).Err()
}
