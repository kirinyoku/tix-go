package postgresrepo

import (
	"context"
	"fmt"

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

func (r *QueryRepo) GetVenue(
	ctx context.Context,
	id int64,
) (*domain.Venue, error) {
	const op = "postgresrepo.QueryRepo.GetVenue"

	db := r.handle()

	var v domain.Venue
	err := db.QueryRow(ctx,
		`SELECT id, name, seating_scheme
       	 FROM venues WHERE id = $1`,
		id,
	).Scan(&v.ID, &v.Name, &v.SeatingScheme)
	if err != nil {
		return nil, wrapDBErr(op, err)
	}

	return &v, nil
}

func (r *QueryRepo) GetEvent(
	ctx context.Context,
	id int64,
) (*domain.Event, error) {
	const op = "postgresrepo.QueryRepo.GetEvent"

	db := r.handle()

	var e domain.Event
	err := db.QueryRow(ctx,
		`SELECT id, venue_id, title, starts_at, ends_at
       	 FROM events WHERE id = $1`,
		id,
	).Scan(&e.ID, &e.VenueID, &e.Title, &e.Starts, &e.Ends)
	if err != nil {
		return nil, wrapDBErr(op, err)
	}

	return &e, nil
}

func (r *QueryRepo) ListEvents(
	ctx context.Context,
	limit, offset int,
) ([]domain.Event, error) {
	const op = "postgresrepo.QueryRepo.ListEvents"

	db := r.handle()

	rows, err := db.Query(ctx,
		`SELECT id, venue_id, title, starts_at, ends_at
		 FROM evenets
		 ORDER BY starts_at
		 LIMIT $1 OFFSET $2`,
		limit, offset,
	)
	if err != nil {
		return nil, wrapDBErr(op, err)
	}

	defer rows.Close()

	var out []domain.Event
	for rows.Next() {
		var e domain.Event
		if err := rows.Scan(&e.ID, &e.VenueID, &e.Title, &e.Ends); err != nil {
			return nil, wrapDBErr(op, err)
		}

		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("%s:%w", op, err)
	}

	return out, nil
}

func (r *QueryRepo) CountsByStatus(
	ctx context.Context,
	eventID int64,
) (*domain.EventCounts, error) {
	const op = "postgresrepo.QueryRepo.CountsByStatus"

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
		return nil, wrapDBErr(op, err)
	}

	ec.Total = ec.Available + ec.Held + ec.Sold

	return &ec, nil
}

func (r *QueryRepo) ListEventsSeats(
	ctx context.Context,
	eventID int64,
	onlyAvailable bool,
	limit, offset int,
) ([]domain.SeatWithStatus, error) {
	const op = "postgresrepo.QueryRepo.ListEventsSeats"

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
		return nil, wrapDBErr(op, err)
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
			return nil, wrapDBErr(op, err)
		}

		sws.Status = domain.SeatStatus(status)
		out = append(out, sws)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("%s:%w", op, err)
	}

	return out, nil
}

func (r *QueryRepo) GetOrderWithTickets(
	ctx context.Context,
	orderID string,
) (*domain.OrderWithTickets, error) {
	const op = "postgresrepo.QueryRepo.GetOrderWithTickets"

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
		return nil, wrapDBErr(op, err)
	}

	rows, err := db.Query(ctx,
		`SELECT id, order_id, event_id, seat_id, created_at
         FROM tickets
      	 WHERE order_id = $1
       	 ORDER BY created_at`,
		orderID,
	)
	if err != nil {
		return nil, wrapDBErr(op, err)
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
			return nil, wrapDBErr(op, err)
		}

		out.Tickets = append(out.Tickets, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("%s:%w", op, err)
	}

	return &out, nil
}

func (r *QueryRepo) DebugCountSeatsByStatus(
	ctx context.Context,
	eventID int64,
) (map[string]int64, error) {
	const op = "postgresrepo.QueryRepo.DebugCountSeatsByStatus"

	db := r.handle()

	rows, err := db.Query(ctx,
		`SELECT status, COUNT(*)
       	 FROM event_seats
      	 WHERE event_id = $1
      	 GROUP BY status`,
		eventID,
	)
	if err != nil {
		return nil, wrapDBErr(op, err)
	}

	defer rows.Close()

	res := make(map[string]int64, 3)

	for rows.Next() {
		var st string
		var c int64

		if err := rows.Scan(&st, &c); err != nil {
			return nil, wrapDBErr(op, err)
		}

		res[st] = c
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("%s:%w", op, err)
	}

	return res, nil
}

func (r *QueryRepo) EnsureEventExists(
	ctx context.Context,
	eventID int64,
) error {
	const op = "postgresrepo.QueryRepo.EnsureEventExists"

	_, err := r.GetEvent(ctx, eventID)
	if err != nil {
		return fmt.Errorf("%s:%w", op, err)
	}

	return nil
}
