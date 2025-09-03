package app

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/kirinyoku/tix-go/internal/config"
	"github.com/kirinyoku/tix-go/internal/postgres"
	"github.com/kirinyoku/tix-go/internal/redis"
	postgresrepo "github.com/kirinyoku/tix-go/internal/repository/postgres"
	redisrepo "github.com/kirinyoku/tix-go/internal/repository/redis"
	"github.com/kirinyoku/tix-go/internal/service"
	"github.com/kirinyoku/tix-go/internal/service/reservation"
	httpgin "github.com/kirinyoku/tix-go/internal/transport/http/gin"
	"golang.org/x/sync/errgroup"
)

type App struct {
	cfg        *config.Config
	logger     *slog.Logger
	httpServer *http.Server
}

func New(cfg *config.Config, logger *slog.Logger) (*App, error) {
	// Initialize dependencies
	dsn := fmt.Sprintf(
		"postgres://%s:%s@%s:%d/%s?sslmode=%s",
		cfg.Postgres.User,
		cfg.Postgres.Password,
		cfg.Postgres.Host,
		cfg.Postgres.Port,
		cfg.Postgres.Name,
		cfg.Postgres.SSLMode,
	)

	pgxPool, err := postgres.New(context.Background(), postgres.Config{DSN: dsn})
	if err != nil {
		return nil, fmt.Errorf("failed to initialize postgres: %w", err)
	}

	rdb, err := redis.New(context.Background(), redis.Config{Addr: cfg.Redis.Addr, Password: cfg.Redis.Password, DB: cfg.Redis.DB})
	if err != nil {
		return nil, fmt.Errorf("failed to initialize redis: %w", err)
	}

	// Initialize repositories
	store := postgresrepo.NewStore(pgxPool)
	cache := redisrepo.New(rdb)
	pubsub := redisrepo.NewEventsPubSub(rdb)
	limiter := redisrepo.NewSlidingWindowLimiter(rdb, "rl", 10, 1*time.Minute)
	idempotencyStore := redisrepo.NewIdempotencyStore(rdb, 2*time.Hour)

	// Initialize services
	services := service.NewServices(store, cache, pubsub, limiter, service.Config{
		Reservation: reservation.Config{},
	})

	// Initialize Gin router
	router := httpgin.NewRouter(services, idempotencyStore, logger)

	return &App{
		cfg:    cfg,
		logger: logger,
		httpServer: &http.Server{
			Addr:    fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port),
			Handler: router,
		},
	}, nil
}

func (a *App) Run(ctx context.Context) error {
	ctx, cancel := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer cancel()

	g, gCtx := errgroup.WithContext(ctx)

	// Start HTTP server
	g.Go(func() error {
		a.logger.Info("HTTP server listening", "host", a.cfg.Server.Host, "port", a.cfg.Server.Port)
		if err := a.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			return fmt.Errorf("failed to start HTTP server: %w", err)
		}
		return nil
	})

	// Graceful shutdown
	g.Go(func() error {
		<-gCtx.Done()
		a.logger.Info("shutting down HTTP server")
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return a.httpServer.Shutdown(ctx)
	})

	return g.Wait()
}
