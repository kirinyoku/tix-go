package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Config struct {
	DSN      string
	MaxConns int32
}

func New(ctx context.Context, cfg Config) (*pgxpool.Pool, error) {
	const op = "postgres.New"

	poolCfg, err := pgxpool.ParseConfig(cfg.DSN)
	if err != nil {
		return nil, fmt.Errorf("%s:%w", op, err)
	}

	if cfg.MaxConns > 0 {
		poolCfg.MaxConns = cfg.MaxConns
	}

	poolCfg.MaxConnIdleTime = 5 * time.Minute
	poolCfg.HealthCheckPeriod = 30 * time.Second

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("%s:%w", op, err)
	}

	ctxPing, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	if err := pool.Ping(ctxPing); err != nil {
		pool.Close()
		return nil, fmt.Errorf("%s:%w", op, err)
	}

	return pool, err
}
