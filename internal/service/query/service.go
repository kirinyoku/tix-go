package query

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/kirinyoku/tix-go/internal/domain"
	"github.com/kirinyoku/tix-go/internal/repository"
	postgresrepo "github.com/kirinyoku/tix-go/internal/repository/postgres"
	redisrepo "github.com/kirinyoku/tix-go/internal/repository/redis"
)

type Config struct {
	EventSummaryTTL   time.Duration
	AvailabilityTTL   time.Duration
	DefaultSeatsPage  int
	MaxSeatsPage      int
	CacheEventSeatMap bool
	EventSeatMapTTL   time.Duration
}

type Service struct {
	store *postgresrepo.Store
	cache *redisrepo.Cache
	cfg   Config
}

func New(store *postgresrepo.Store, cache *redisrepo.Cache, cfg Config) *Service {
	if cfg.EventSummaryTTL <= 0 {
		cfg.EventSummaryTTL = 60 * time.Second
	}

	if cfg.AvailabilityTTL <= 0 {
		cfg.AvailabilityTTL = 15 * time.Second
	}

	if cfg.DefaultSeatsPage <= 0 {
		cfg.DefaultSeatsPage = 100
	}

	if cfg.MaxSeatsPage <= 0 {
		cfg.MaxSeatsPage = 500
	}

	if cfg.EventSeatMapTTL <= 0 {
		cfg.EventSeatMapTTL = 60 * time.Second
	}

	return &Service{
		store: store,
		cache: cache,
		cfg:   cfg,
	}
}

// GetEvent retrieves an event by its ID, utilizing a caching layer to improve performance.
//
// Parameters:
//   - ctx: request-scoped context.
//   - id: ID of the event to retrieve.
//
// Returns:
//   - *domain.Event: the retrieved event, or nil if not found.
//   - error: query.ErrEventNotFound if the event is not found.
func (s *Service) GetEvent(ctx context.Context, id int64) (*domain.Event, error) {
	const op = "service.query.GetEvent"

	key := redisrepo.KeyEventSummary(id)

	event, err := redisrepo.GetOrSetJSON(
		ctx,
		s.cache,
		key,
		s.cfg.EventSummaryTTL,
		func(ctx context.Context) (domain.Event, error) {
			e, err := s.store.Query().GetEvent(ctx, id)
			if err != nil {
				if errors.Is(err, repository.ErrNotFound) {
					return domain.Event{}, ErrEventNotFound
				}

				return domain.Event{}, err
			}

			return *e, nil
		},
	)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	return &event, nil
}

// CountByStatus retrieves the count of seats by their status for a specific event.
//
// Parameters:
//   - ctx: request-scoped context.
//   - eventID: ID of the event to retrieve seat counts for.
//
// Returns:
//   - *domain.EventCounts: the retrieved seat counts, or nil if not found.
//   - error: query.ErrEventNotFound if the event is not found.
func (s *Service) CountsByStatus(ctx context.Context, eventID int64) (*domain.EventCounts, error) {
	const op = "service.query.CountsByStatus"

	key := redisrepo.KeyEventAvailability(eventID)

	eventCounts, err := redisrepo.GetOrSetJSON(
		ctx,
		s.cache,
		key,
		s.cfg.AvailabilityTTL,
		func(ctx context.Context) (domain.EventCounts, error) {
			ec, err := s.store.Query().CountsByStatus(ctx, eventID)
			if err != nil {
				if errors.Is(err, repository.ErrNotFound) {
					return domain.EventCounts{}, ErrEventNotFound
				}

				return domain.EventCounts{}, err
			}

			return *ec, nil
		},
	)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	return &eventCounts, nil
}

// ListEventSeats retrieves a list of seats for a specific event, with optional filtering
// for only available seats. Pagination is supported via limit and offset parameters.
//
// Parameters:
//   - ctx: request-scoped context.
//   - eventID: ID of the event to list seats for.
//   - onlyAvailable: if true, only seats with 'available' status are returned.
//   - limit: maximum number of seats to return (default and max limits are enforced).
//   - offset: number of seats to skip for pagination.
//
// Returns:
//   - []domain.SeatWithStatus: list of seats with their status.
//   - error: query.ErrEventNotFound if the event is not found.
func (s *Service) ListEventSeats(
	ctx context.Context,
	eventID int64,
	onlyAvailable bool,
	limit, offset int,
) ([]domain.SeatWithStatus, error) {
	const op = "service.query.ListEventSeats"

	if limit <= 0 {
		limit = s.cfg.DefaultSeatsPage
	}

	if limit > s.cfg.MaxSeatsPage {
		limit = s.cfg.MaxSeatsPage
	}

	seats, err := s.store.Query().ListEventSeats(ctx, eventID, onlyAvailable, limit, offset)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, fmt.Errorf("%s: %w", op, ErrEventNotFound)
		}

		return nil, fmt.Errorf("%s: %w", op, err)
	}

	return seats, nil
}

// GetOrderWithTickets retrieves an order along with its associated tickets.
//
// Parameters:
//   - ctx: request-scoped context.
//   - orderID: ID of the order to retrieve.
//
// Returns:
//   - *domain.OrderWithTickets: the retrieved order with its tickets, or nil if not found.
//   - error: query.ErrOrderNotFound if the order is not found.
func (s *Service) GetOrderWithTickets(ctx context.Context, orderID string) (*domain.OrderWithTickets, error) {
	const op = "service.query.GetOrderWithTickets"

	order, err := s.store.Query().GetOrderWithTickets(ctx, orderID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, fmt.Errorf("%s:%w", op, ErrOrderNotFound)
		}

		return nil, fmt.Errorf("%s: %w", op, err)
	}

	return order, nil
}
