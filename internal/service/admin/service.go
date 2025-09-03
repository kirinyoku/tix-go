package admin

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/kirinyoku/tix-go/internal/domain"
	"github.com/kirinyoku/tix-go/internal/repository"
	postgresrepo "github.com/kirinyoku/tix-go/internal/repository/postgres"
	redisrepo "github.com/kirinyoku/tix-go/internal/repository/redis"
	"github.com/kirinyoku/tix-go/internal/uow"
)

type Service struct {
	store  *postgresrepo.Store
	cache  *redisrepo.Cache
	pubsub *redisrepo.EventsPubSub
	uow    *uow.UoW
}

func New(store *postgresrepo.Store, cache *redisrepo.Cache, pubsub *redisrepo.EventsPubSub) *Service {
	return &Service{
		store:  store,
		cache:  cache,
		pubsub: pubsub,
		uow:    uow.NewUoW(store),
	}
}

// CreateVenue creates a venue record and returns its ID.
//
// Parameters:
//   - ctx: request-scoped context.
//   - name: venue name.
//   - seatingSchemeJSON: raw JSON representing the seating layout.
//
// Returns:
//   - int64: the created venue ID on success.
//   - error: admin.ErrVenueConflict if a venue with the same name already exists.
func (s *Service) CreateVenue(ctx context.Context, name string, seatingSchemeJSON []byte) (int64, error) {
	const op = "service.admin.CreateVenue"

	var id int64
	err := s.uow.Do(ctx, func(ctx context.Context, tx postgresrepo.DB, after func(uow.AfterCommit)) error {
		var err error
		id, err = s.store.Admin().With(tx).CreateVenue(ctx, name, seatingSchemeJSON)
		if err != nil {
			if errors.Is(err, repository.ErrConflict) {
				return fmt.Errorf("%s: %w", op, ErrVenueConflict)
			}
			return fmt.Errorf("%s: %w", op, err)
		}
		return nil
	})

	return id, err
}

// BatchCreateSeats inserts multiple seats for a venue within a
// transactional Unit of Work.
//
// Parameters:
//   - ctx: request-scoped context.
//   - venueID: ID of the venue to add seats to.
//   - seats: list of domain.Seat objects to create.
//
// Returns:
//   - error: admin.ErrSeatsConflict if a seat with the same identifying data
//     already exists.
func (s *Service) BatchCreateSeats(ctx context.Context, venueID int64, seats []domain.Seat) error {
	const op = "service.admin.BatchCreateSeats"

	err := s.uow.Do(ctx, func(ctx context.Context, tx postgresrepo.DB, after func(uow.AfterCommit)) error {
		err := s.store.Admin().With(tx).BatchCreateSeats(ctx, venueID, seats)
		if err != nil {
			if errors.Is(err, repository.ErrConflict) {
				return fmt.Errorf("%s: %w", op, ErrSeatsConflict)
			}
			return fmt.Errorf("%s: %w", op, err)
		}
		return nil
	})

	return err
}

// CreateEventWithInit creates an event and initializes event seats by
// copying all seats from the venue into the event_seats table.
//
// Parameters:
//   - ctx: request-scoped context.
//   - venueID: the venue the event belongs to.
//   - title: event title.
//   - starts, ends: start and end times for the event.
//
// Returns:
//   - int64: the created event ID.
//   - error: admin.ErrEventConflict if the event creation violates a uniqueness
//     constraint.
//   - error: admin.ErrFailedToInitEventSeats if initializing event seats fails.
func (s *Service) CreateEventWithInit(
	ctx context.Context,
	venueID int64,
	title string,
	starts, ends time.Time,
) (int64, error) {
	const op = "service.admin.CreateEventWithInit"

	var eventID int64
	var err error

	err = s.uow.Do(ctx, func(
		ctx context.Context,
		tx postgresrepo.DB,
		after func(uow.AfterCommit),
	) error {
		eventID, err = s.store.Admin().
			With(tx).
			CreateEvent(ctx, venueID, title, starts, ends)
		if err != nil {
			if errors.Is(err, repository.ErrConflict) {
				return fmt.Errorf("%s: %w", op, ErrEventConflict)
			}
			return fmt.Errorf("%s: %w", op, err)
		}

		if _, err := s.store.Admin().
			With(tx).
			InitEventSeats(ctx, eventID, venueID); err != nil {
			if errors.Is(err, repository.ErrNotFound) {
				return fmt.Errorf("%s: %w", op, ErrFailedToInitEventSeats)
			}
			return fmt.Errorf("%s: %w", op, err)
		}

		after(func(ctx context.Context) {
			_ = s.cache.InvalidateEvent(ctx, eventID)
			_ = s.pubsub.PublishEventChanged(ctx, eventID)
		})
		return nil
	})
	return eventID, err
}
