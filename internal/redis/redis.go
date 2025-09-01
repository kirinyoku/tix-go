package redisx

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

func New(ctx context.Context, addr, pass string, db int) (*redis.Client, error) {
	rdb := redis.NewClient(&redis.Options{
		Addr:        addr,
		Password:    pass,
		DB:          db,
		DialTimeout: 2 * time.Second,
	})

	ctxPing, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	if err := rdb.Ping(ctxPing).Err(); err != nil {
		return nil, err
	}

	return rdb, nil
}
