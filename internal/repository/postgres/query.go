package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kirinyoku/tix-go/internal/domain"
)

type QueryRepo struct {
	pool *pgxpool.Pool
	db   DB
}

func (r *QueryRepo) With(db DB) *QueryRepo {
	cp := *r
	cp.db = db
	return &cp
}

func (r *QueryRepo) handle() DB {
	if r.db != nil {
		return r.db
	}
	return r.pool
}

// GetVenue retrieves a venue by its ID.
//
// Parameters:
//   - ctx: request-scoped context for cancellation and timeouts.
//   - id: unique identifier of the venue to retrieve.
//
// Returns:
//   - *domain.Venue: the venue when found.
//   - error: repository.ErrNotFound if the venue is not found.
func (r *QueryRepo) GetVenue(ctx context.Context, id int64) (*domain.Venue, error) {
	const op = "postgres.QueryRepo.GetVenue"

	db := r.handle()

	var v domain.Venue
	err := db.QueryRow(ctx,
		`SELECT id, name, seating_scheme
       	 FROM venues WHERE id = $1`,
		id,
	).Scan(&v.ID, &v.Name, &v.SeatingScheme)
	if err != nil {
		return nil, fmt.Errorf("%s:%w", op, translateDBErr(err))
	}

	return &v, nil
}

// GetEvent retrieves an event by its ID.
//
// Parameters:
//   - ctx: request-scoped context for cancellation and timeouts.
//   - id: unique identifier of the event to retrieve.
//
// Returns:
//   - *domain.Event: the event when found.
//   - error: repository.ErrNotFound if the event is not found.
func (r *QueryRepo) GetEvent(ctx context.Context, id int64) (*domain.Event, error) {
	const op = "postgres.QueryRepo.GetEvent"

	db := r.handle()

	var e domain.Event
	err := db.QueryRow(ctx,
		`SELECT id, venue_id, title, starts_at, ends_at
       	 FROM events WHERE id = $1`,
		id,
	).Scan(&e.ID, &e.VenueID, &e.Title, &e.Starts, &e.Ends)
	if err != nil {
		return nil, fmt.Errorf("%s:%w", op, translateDBErr(err))
	}

	return &e, nil
}

// ListEvents lists all events.
//
// Parameters:
//   - ctx: request-scoped context for cancellation and timeouts.
//   - limit, offset: pagination parameters.
//
// Returns:
//   - []domain.Event: list of events.
//   - error: repository.ErrNotFound if no events are found.
func (r *QueryRepo) ListEvents(ctx context.Context, limit, offset int) ([]domain.Event, error) {
	const op = "postgres.QueryRepo.ListEvents"

	db := r.handle()

	rows, err := db.Query(ctx,
		`SELECT id, venue_id, title, starts_at, ends_at
		 FROM evenets
		 ORDER BY starts_at
		 LIMIT $1 OFFSET $2`,
		limit, offset,
	)
	if err != nil {
		return nil, fmt.Errorf("%s:%w", op, translateDBErr(err))
	}

	defer rows.Close()

	var out []domain.Event
	for rows.Next() {
		var e domain.Event
		if err := rows.Scan(&e.ID, &e.VenueID, &e.Title, &e.Ends); err != nil {
			return nil, fmt.Errorf("%s:%w", op, translateDBErr(err))
		}

		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("%s:%w", op, err)
	}

	return out, nil
}

// CountsByStatus counts seats by status for an event.
//
// Parameters:
//   - ctx: request-scoped context for cancellation and timeouts.
//   - eventID: unique identifier of the event to retrieve.
//
// Returns:
//   - *domain.EventCounts: the event counts when found.
//   - error: repository.ErrNotFound if the event is not found.
func (r *QueryRepo) CountsByStatus(ctx context.Context, eventID int64) (*domain.EventCounts, error) {
	const op = "postgres.QueryRepo.CountsByStatus"

	db := r.handle()

	var ec domain.EventCounts
	err := db.QueryRow(ctx,
		`SELECT
       	 	COALESCE(SUM(CASE WHEN status = 'available' THEN 1 ELSE 0 END), 0),
    	 	COALESCE(SUM(CASE WHEN status = 'held' THEN 1 ELSE 0 END), 0),
       	 	COALESCE(SUM(CASE WHEN status = 'sold' THEN 1 ELSE 0 END), 0)
     	 FROM event_seats
     	 WHERE event_id = $1`,
		eventID,
	).Scan(&ec.Available, &ec.Held, &ec.Sold)
	if err != nil {
		return nil, fmt.Errorf("%s:%w", op, translateDBErr(err))
	}

	ec.Total = ec.Available + ec.Held + ec.Sold

	return &ec, nil
}

// ListEventSeats lists seats for an event.
//
// Parameters:
//   - ctx: request-scoped context for cancellation and timeouts.
//   - eventID: unique identifier of the event to retrieve.
//   - onlyAvailable: flag to filter only available seats.
//
// Returns:
//   - []domain.SeatWithStatus: list of seats with their status.
//   - error: repository.ErrNotFound if the event is not found.
func (r *QueryRepo) ListEventSeats(
	ctx context.Context,
	eventID int64,
	onlyAvailable bool,
	limit, offset int,
) ([]domain.SeatWithStatus, error) {
	const op = "postgres.QueryRepo.ListEventSeats"

	db := r.handle()

	var rows pgx.Rows
	var err error

	if onlyAvailable {
		rows, err = db.Query(ctx,
			`SELECT s.id, s.venue_id, s.section, s.row, s.number, es.status
			 FROM events_seats es
			 JOIN seats s ON s.id = es.seat_id
			 WHERE es.event_id = $1 AND es.status = 'available'
			 ORDER BY s.section, s.row, s.number
        	 LIMIT $2 OFFSET $3`,
			eventID, limit, offset,
		)
	} else {
		rows, err = db.Query(ctx,
			`SELECT s.id, s.venue_id, s.section, s.row, s.number, es.status
         	 FROM event_seats es
          	 JOIN seats s ON s.id = es.seat_id
        	 WHERE es.event_id = $1
        	 ORDER BY s.section, s.row, s.number
        	 LIMIT $2 OFFSET $3`,
			eventID, limit, offset,
		)
	}
	if err != nil {
		return nil, fmt.Errorf("%s:%w", op, translateDBErr(err))
	}

	defer rows.Close()

	var out []domain.SeatWithStatus
	for rows.Next() {
		var sws domain.SeatWithStatus
		var status string

		if err := rows.Scan(
			&sws.ID,
			&sws.VenueID,
			&sws.Section,
			&sws.Row,
			&sws.Number,
			&status,
		); err != nil {
			return nil, fmt.Errorf("%s:%w", op, translateDBErr(err))
		}

		sws.Status = domain.SeatStatus(status)
		out = append(out, sws)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("%s:%w", op, err)
	}

	return out, nil
}

// GetOrderWithTickets retrieves an order with its tickets.
//
// Parameters:
//   - ctx: request-scoped context for cancellation and timeouts.
//   - orderID: unique identifier of the order to retrieve.
//
// Returns:
//   - *domain.OrderWithTickets: the order with its tickets when found.
//   - error: repository.ErrNotFound if the order is not found.
func (r *QueryRepo) GetOrderWithTickets(ctx context.Context, orderID string) (*domain.OrderWithTickets, error) {
	const op = "postgres.QueryRepo.GetOrderWithTickets"

	db := r.handle()

	var out domain.OrderWithTickets

	err := db.QueryRow(ctx,
		`SELECT id, event_id, user_id, total_cents, created_at
         FROM orders
         WHERE id = $1`,
		orderID,
	).Scan(
		&out.Order.ID,
		&out.Order.EventID,
		&out.Order.UserID,
		&out.Order.TotalCents,
		&out.Order.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("%s:%w", op, translateDBErr(err))
	}

	rows, err := db.Query(ctx,
		`SELECT id, order_id, event_id, seat_id, created_at
         FROM tickets
      	 WHERE order_id = $1
       	 ORDER BY created_at`,
		orderID,
	)
	if err != nil {
		return nil, fmt.Errorf("%s:%w", op, translateDBErr(err))
	}

	defer rows.Close()

	for rows.Next() {
		var t domain.Ticket

		if err := rows.Scan(
			&t.ID,
			&t.OrderID,
			&t.EventID,
			&t.SeatID,
			&t.Created,
		); err != nil {
			return nil, fmt.Errorf("%s:%w", op, translateDBErr(err))
		}

		out.Tickets = append(out.Tickets, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("%s:%w", op, err)
	}

	return &out, nil
}

// EventIDByHold retrieves an event ID by its hold ID.
//
// Returns:
//   - int64: the event ID when found.
//   - error: repository.ErrNotFound if the hold is not found.
func (r *QueryRepo) EventIDByHold(ctx context.Context, holdID uuid.UUID) (int64, error) {
	const op = "postgres.QueryRepo.EventIDByHold"

	db := r.handle()

	var eventID int64

	err := db.QueryRow(ctx, `SELECT event_id FROM holds WHERE id = $1`, holdID).Scan(&eventID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, fmt.Errorf("%s:%w", op, translateDBErr(err))
		}

		return 0, fmt.Errorf("%s:%w", op, err)
	}

	return eventID, nil
}
