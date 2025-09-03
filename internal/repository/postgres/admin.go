package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kirinyoku/tix-go/internal/domain"
)

type AdminRepo struct {
	pool *pgxpool.Pool
	db   DB
}

func (r *AdminRepo) With(db DB) *AdminRepo {
	cp := *r
	cp.db = db
	return &cp
}

func (r *AdminRepo) handle() DB {
	if r.db != nil {
		return r.db
	}
	return r.pool
}

// CreateVenue inserts a new venue record and returns its generated ID.
//
// The seatingSchemeJSON is stored in the venues.seating_scheme column
// and is expected to be a JSON representation of the venue layout.
//
// Parameters:
//   - ctx: request-scoped context for cancellation and deadlines.
//   - name: human-readable venue name.
//   - seatingSchemeJSON: raw JSON bytes representing the seating scheme.
//
// Returns:
//   - int64: newly created venue ID.
//   - error: repository.ErrConflict if a venue with the same name exists.
func (r *AdminRepo) CreateVenue(ctx context.Context, name string, seatingSchemeJSON []byte) (int64, error) {
	const op = "postgres.AdminRepo.CreateVenue"

	db := r.handle()

	var id int64
	if err := db.QueryRow(ctx,
		`INSERT INTO venues(name, seating_scheme)
			 VALUES ($1, $2)
			 RETURNING id`,
		name, seatingSchemeJSON,
	).Scan(&id); err != nil {
		return 0, fmt.Errorf("%s:%w", op, translateDBErr(err))
	}

	return id, nil
}

// BatchCreateSeats inserts multiple seat rows for the given venue.
//
// Parameters:
//   - ctx: request-scoped context.
//   - venueID: ID of the venue the seats belong to.
//   - seats: slice of domain.Seat values to be created.
//
// Returns:
//   - error: repository.ErrConflict if a seat with the same attributes exists.
func (r *AdminRepo) BatchCreateSeats(ctx context.Context, venueID int64, seats []domain.Seat) error {
	const op = "postgres.AdminRepo.BacthCreateSeats"

	db := r.handle()

	batch := &pgx.Batch{}
	for _, s := range seats {
		batch.Queue(
			`INSERT INTO seats(venue_id, section, row, number)
				 VALUES ($1, $2, $3, $4)
			 ON CONFLICT (venue_id, section, row, number) DO NOTHING`,
			venueID, s.Section, s.Row, s.Number,
		)
	}
	if err := db.SendBatch(ctx, batch).Close(); err != nil {
		return fmt.Errorf("%s:%w", op, translateDBErr(err))
	}

	return nil
}

// CreateEvent inserts a new event for a venue and returns the created
// event ID.
//
// Parameters:
//   - ctx: request-scoped context.
//   - venueID: ID of the venue the event is for.
//   - title: event title.
//   - starts, ends: start and end timestamps/values for the event.
//
// Returns:
//   - int64: created event ID.
//   - error: repository.ErrConflict if an event with the same attributes exists.
func (r *AdminRepo) CreateEvent(
	ctx context.Context,
	venueID int64,
	title string,
	starts, ends any,
) (int64, error) {
	const op = "postgres.AdminRepo.CreateEvent"

	db := r.handle()

	var id int64
	if err := db.QueryRow(ctx,
		`INSERT INTO events(venue_id, title, starts_at, ends_at)
			 VALUES ($1, $2, $3, $4)
			 RETURNING id`,
		venueID, title, starts, ends,
	).Scan(&id); err != nil {
		return 0, fmt.Errorf("%s:%w", op, translateDBErr(err))
	}

	return id, nil
}

// InitEventSeats materializes seats for a specific event by copying
// all seats from the venue into the event_seats table with an initial
// status of 'available'.
//
// Parameters:
//   - ctx: request-scoped context.
//   - eventID: ID of the event to initialize seats for.
//   - venueID: ID of the venue whose seats will be copied.
//
// Returns:
//   - int64: number of rows inserted into event_seats.
//   - error: repository.ErrConflict if an event seat with the same attributes exists.
func (r *AdminRepo) InitEventSeats(ctx context.Context, eventID int64, venueID int64) (int64, error) {
	const op = "postgres.AdminRepo.InitEventSeats"

	db := r.handle()

	tag, err := db.Exec(ctx,
		`INSERT INTO event_seats(event_id, seat_id, status)
			 SELECT $1, s.id, 'available'
		 FROM seats s
		 WHERE s.venue_id = $2
			 ON CONFLICT DO NOTHING`,
		eventID, venueID,
	)
	if err != nil {
		return 0, fmt.Errorf("%s:%w", op, translateDBErr(err))
	}

	return tag.RowsAffected(), nil
}
