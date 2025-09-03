package reservation

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/kirinyoku/tix-go/internal/domain"
	"github.com/kirinyoku/tix-go/internal/repository"
	postgresrepo "github.com/kirinyoku/tix-go/internal/repository/postgres"
	redisrepo "github.com/kirinyoku/tix-go/internal/repository/redis"
	"github.com/kirinyoku/tix-go/internal/uow"
)

type Config struct {
	MinHoldTTL time.Duration
	MaxHoldTTL time.Duration
}

type Service struct {
	store   *postgresrepo.Store
	cache   *redisrepo.Cache
	pubsub  *redisrepo.EventsPubSub
	limiter *redisrepo.SlidingWindowLimiter
	uow     *uow.UoW
	cfg     Config
}

func New(
	store *postgresrepo.Store,
	cache *redisrepo.Cache,
	pubsub *redisrepo.EventsPubSub,
	limiter *redisrepo.SlidingWindowLimiter,
	cfg Config,
) *Service {
	if cfg.MinHoldTTL <= 0 {
		cfg.MinHoldTTL = 15 * time.Second
	}

	if cfg.MaxHoldTTL <= 0 || cfg.MaxHoldTTL < cfg.MinHoldTTL {
		cfg.MaxHoldTTL = 5 * time.Minute
	}

	return &Service{
		store:   store,
		cache:   cache,
		pubsub:  pubsub,
		limiter: limiter,
		uow:     uow.NewUoW(store),
		cfg:     cfg,
	}
}

// CreateHold creates a new hold for the specified seats.
//
// Parameters:
//   - ctx: request-scoped context.
//   - userID: ID of the user creating the hold.
//   - eventID: ID of the event the seats are for.
//   - seatIDs: IDs of the seats to hold.
//   - ttl: time-to-live for the hold.
//
// Returns:
//   - uuid.UUID: the ID of the created hold.
//   - error: reservation.ErrSeatsUnavailable if the seats are unavailable.
//   - error: reservation.ErrHoldConflict if the hold conflicts with an existing hold.
func (s *Service) CreateHold(
	ctx context.Context,
	userID, eventID int64,
	seatIDs []int64,
	ttl time.Duration,
	rlKey string,
) (uuid.UUID, error) {
	const op = "service.reservation.CreateHold"

	if len(seatIDs) == 0 {
		return uuid.Nil, fmt.Errorf("%s:%s", op, "no seats selected")
	}

	ttl = s.clampTTL(ttl)

	if s.limiter != nil && rlKey != "" {
		ok, _, retry, err := s.limiter.Allow(ctx, rlKey)
		if err != nil {
			return uuid.Nil, fmt.Errorf("%s:%w", op, err)
		}
		if !ok {
			return uuid.Nil, fmt.Errorf("%s: rate limited, retry in %s", op, retry)
		}
	}

	var holdID uuid.UUID

	err := s.uow.Do(ctx, func(
		ctx context.Context,
		tx postgresrepo.DB,
		after func(uow.AfterCommit),
	) error {
		rid, err := s.store.Reservations().
			With(tx).
			HoldSeats(ctx, eventID, userID, seatIDs, ttl)
		if err != nil {
			if errors.Is(err, repository.ErrSeatsUnavailable) {
				return fmt.Errorf("%s:%w", op, ErrSeatsUnavailable)
			}

			if errors.Is(err, repository.ErrConflict) {
				return fmt.Errorf("%s:%w", op, ErrHoldConflict)
			}

			return fmt.Errorf("%s:%w", op, err)
		}

		holdID = rid

		after(func(ctx context.Context) {
			_ = s.cache.InvalidateEvent(ctx, eventID)
			_ = s.pubsub.PublishEventChanged(ctx, eventID)
		})

		return nil
	})
	if err != nil {
		return uuid.Nil, fmt.Errorf("%s:%w", op, err)
	}

	return holdID, nil
}

// Confirm confirms a hold and creates an order.
//
// Parameters:
//   - ctx: request-scoped context.
//   - holdID: ID of the hold to confirm.
//   - totalCents: total amount for the order.
//
// Returns:
//   - uuid.UUID: the ID of the created order.
//   - int64: the ID of the event the order is for.
//   - error: reservation.ErrHoldConflict if the hold conflicts with an existing hold.
//   - error: reservation.ErrHoldNotFound if the hold is not found.
//   - error: reservation.ErrHoldExpired if the hold has expired.
func (s *Service) Confirm(
	ctx context.Context,
	holdID uuid.UUID,
	totalCents int,
) (uuid.UUID, int64, error) {
	const op = "service.reservation.Confirm"

	if totalCents <= 0 {
		return uuid.Nil, 0, fmt.Errorf("%s: total must be positive", op)
	}

	var orderID uuid.UUID
	var eventID int64

	err := s.uow.Do(ctx, func(
		ctx context.Context,
		tx postgresrepo.DB,
		after func(uow.AfterCommit),
	) error {
		eid, err := s.store.Query().With(tx).EventIDByHold(ctx, holdID)
		if err != nil {
			if errors.Is(err, repository.ErrNotFound) {
				return fmt.Errorf("%s:%w", op, ErrHoldNotFound)
			}

			return fmt.Errorf("%s:%w", op, err)
		}

		eventID = eid

		oid, err := s.store.Reservations().
			With(tx).
			ConfirmHold(ctx, holdID, totalCents)
		if err != nil {
			if errors.Is(err, repository.ErrConflict) {
				return fmt.Errorf("%s:%w", op, ErrHoldConflict)
			}

			if errors.Is(err, repository.ErrHoldExpired) {
				return fmt.Errorf("%s:%w", op, ErrHoldExpired)
			}

			return fmt.Errorf("%s:%w", op, err)
		}

		orderID = oid

		after(func(ctx context.Context) {
			_ = s.cache.InvalidateEvent(ctx, eventID)
			_ = s.pubsub.PublishEventChanged(ctx, eventID)
		})

		return nil
	})

	return orderID, eventID, err
}

// Cancel cancels a hold.
//
// Parameters:
//   - ctx: request-scoped context.
//   - holdID: ID of the hold to cancel.
//
// Returns:
//   - int64: the ID of the event the hold was for.
//   - error: reservation.ErrHoldNotFound if the hold is not found.
func (s *Service) Cancel(ctx context.Context, holdID uuid.UUID) (int64, error) {
	const op = "service.reservation.Cancel"

	var eventID int64

	err := s.uow.Do(ctx, func(
		ctx context.Context,
		tx postgresrepo.DB,
		after func(uow.AfterCommit),
	) error {
		eid, err := s.store.Query().With(tx).EventIDByHold(ctx, holdID)
		if err != nil {
			if errors.Is(err, repository.ErrNotFound) {
				return fmt.Errorf("%s:%w", op, ErrHoldNotFound)
			}

			return fmt.Errorf("%s:%w", op, err)
		}

		eventID = eid

		if err := s.store.Reservations().With(tx).CancelHold(ctx, holdID); err != nil {
			if errors.Is(err, repository.ErrNotFound) {
				return fmt.Errorf("%s:%w", op, ErrHoldNotFound)
			}

			return fmt.Errorf("%s:%w", op, err)
		}

		after(func(ctx context.Context) {
			_ = s.cache.InvalidateEvent(ctx, eventID)
			_ = s.pubsub.PublishEventChanged(ctx, eventID)
		})

		return nil
	})

	return eventID, err
}

// Expire expires all holds that have exceeded their TTL.
//
// Parameters:
//   - ctx: request-scoped context.
//
// Returns:
//   - int64: the number of expired holds.
//   - error: if the expiration fails.
func (s *Service) Expire(ctx context.Context) (int64, error) {
	const op = "service.reservation.Expire"

	released, err := s.store.Reservations().ExpireHolds(ctx)
	if err != nil {
		return 0, fmt.Errorf("%s:%w", op, err)
	}

	return released, nil
}

// Availability returns the availability of an event.
//
// Parameters:
//   - ctx: request-scoped context.
//   - eventID: ID of the event to check availability for.
//
// Returns:
//   - *domain.EventCounts: the availability counts for the event.
//   - error: if the availability check fails.
func (s *Service) Availability(ctx context.Context, eventID int64) (*domain.EventCounts, error) {
	const op = "service.reservation.Availability"

	eventCounts, err := s.store.Query().CountsByStatus(ctx, eventID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, fmt.Errorf("%s:%w", op, ErrEventNotFound)
		}

		return nil, fmt.Errorf("%s:%w", op, err)
	}

	return eventCounts, nil
}

func (s *Service) clampTTL(ttl time.Duration) time.Duration {
	if ttl < s.cfg.MinHoldTTL {
		return s.cfg.MinHoldTTL
	}

	if ttl > s.cfg.MaxHoldTTL {
		return s.cfg.MaxHoldTTL
	}

	return ttl
}
