package redis

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

type Config struct {
	Addr     string
	Password string
	DB       int
}

func New(ctx context.Context, cfg Config) (*redis.Client, error) {
	const op = "redis.New"

	opts := &redis.Options{
		Addr:     cfg.Addr,
		Password: cfg.Password,
		DB:       cfg.DB,
	}

	client := redis.NewClient(opts)

	ctxPing, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	if _, err := client.Ping(ctxPing).Result(); err != nil {
		return nil, fmt.Errorf("%s:%w", op, err)
	}

	return client, nil
}
