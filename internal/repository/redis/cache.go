package redis

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
	"golang.org/x/sync/singleflight"
)

type Cache struct {
	rdb *redis.Client
	sf  singleflight.Group
}

func New(client *redis.Client) *Cache {
	return &Cache{rdb: client}
}

func (c *Cache) GetString(ctx context.Context, key string) (string, bool, error) {
	s, err := c.rdb.Get(ctx, key).Result()
	if err == redis.Nil {
		return "", false, nil
	}

	if err != nil {
		return "", false, err
	}

	return s, true, nil
}

func (c *Cache) SetString(
	ctx context.Context,
	key string,
	val string,
	ttl time.Duration,
) error {
	return c.rdb.Set(ctx, key, val, ttl).Err()
}

func (c *Cache) Del(ctx context.Context, keys ...string) error {
	if len(keys) == 0 {
		return nil
	}

	return c.rdb.Del(ctx, keys...).Err()
}

func GetJSON[T any](ctx context.Context, c *Cache, key string) (T, bool, error) {
	var zero T

	s, ok, err := c.GetString(ctx, key)
	if err != nil || !ok {
		return zero, ok, err
	}

	var out T
	if err := json.Unmarshal([]byte(s), &out); err != nil {
		return zero, false, err
	}

	return out, true, nil
}

func SetJSON(
	ctx context.Context,
	c *Cache,
	key string,
	val any,
	ttl time.Duration,
) error {
	b, err := json.Marshal(val)
	if err != nil {
		return err
	}

	return c.SetString(ctx, key, string(b), ttl)
}

func GetOrSetJSON[T any](
	ctx context.Context,
	c *Cache,
	key string,
	ttl time.Duration,
	loader func(ctx context.Context) (T, error),
) (T, error) {
	if v, ok, err := GetJSON[T](ctx, c, key); err != nil || ok {
		return v, err
	}

	vAny, err, _ := c.sf.Do(key, func() (any, error) {
		if v2, ok2, err2 := GetJSON[T](ctx, c, key); err2 != nil || ok2 {
			return v2, err2
		}
		v3, err3 := loader(ctx)
		if err3 != nil {
			return nil, err3
		}
		_ = SetJSON(ctx, c, key, v3, ttl)
		return v3, nil
	})
	if err != nil {
		var zero T
		return zero, err
	}

	v, ok := vAny.(T)
	if !ok {
		var zero T
		return zero, errors.New("type assertion failed")
	}

	return v, nil
}

func (c *Cache) InvalidateEvent(ctx context.Context, eventID int64) error {
	return c.Del(
		ctx,
		KeyEventSummary(eventID),
		KeyEventAvailability(eventID),
		KeyEventSeatMap(eventID),
	)
}
