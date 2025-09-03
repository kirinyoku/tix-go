package service

import (
	postgres "github.com/kirinyoku/tix-go/internal/repository/postgres"
	redis "github.com/kirinyoku/tix-go/internal/repository/redis"
	"github.com/kirinyoku/tix-go/internal/service/admin"
	"github.com/kirinyoku/tix-go/internal/service/orders"
	"github.com/kirinyoku/tix-go/internal/service/query"
	"github.com/kirinyoku/tix-go/internal/service/reservation"
)

type Services struct {
	Reservation *reservation.Service
	Query       *query.Service
	Admin       *admin.Service
	Orders      *orders.Service
}

type Config struct {
	Reservation reservation.Config
	Query       query.Config
}

func NewServices(
	store *postgres.Store,
	cache *redis.Cache,
	pubsub *redis.EventsPubSub,
	limiter *redis.SlidingWindowLimiter,
	cfg Config,
) *Services {
	return &Services{
		Reservation: reservation.New(store, cache, pubsub, limiter, cfg.Reservation),
		Query:       query.New(store, cache, cfg.Query),
		Admin:       admin.New(store, cache, pubsub),
		Orders:      orders.New(store),
	}
}
