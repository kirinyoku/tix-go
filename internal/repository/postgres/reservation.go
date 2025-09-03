package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kirinyoku/tix-go/internal/repository"
)

type ReservationRepo struct {
	pool *pgxpool.Pool
	db   DB
}

func (r *ReservationRepo) With(db DB) *ReservationRepo {
	cp := *r
	cp.db = db
	return &cp
}

func (r *ReservationRepo) handle() DB {
	if r.db != nil {
		return r.db
	}
	return r.pool
}

// HoldSeats holds seats for a user.
//
// Parameters:
//   - ctx: request-scoped context for cancellation and timeouts.
//   - eventID: unique identifier of the event to retrieve.
//   - userID: unique identifier of the user holding the seats.
//   - seatIDs: list of seat IDs to hold.
//   - ttl: time-to-live for the hold.
//
// Returns:
//   - uuid.UUID: the hold ID when successful.
//   - error: repository.ErrSeatsUnavailable if some seats are not available.
//   - error: repository.ErrConflict if there is a conflict creating the hold.
func (r *ReservationRepo) HoldSeats(
	ctx context.Context,
	eventID int64,
	userID int64,
	seatIDs []int64,
	ttl time.Duration,
) (uuid.UUID, error) {
	const op = "postgres.ReservationRepo.HoldSeats"

	if r.db != nil {
		id, err := r.holdSeatsCore(ctx, r.db, eventID, userID, seatIDs, ttl)
		if err != nil {
			return uuid.Nil, fmt.Errorf("%s:%w", op, translateDBErr(err))
		}
		return id, nil
	}

	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{
		IsoLevel:   pgx.Serializable,
		AccessMode: pgx.ReadWrite,
	})
	if err != nil {
		return uuid.Nil, fmt.Errorf("%s:%w", op, translateDBErr(err))
	}

	defer tx.Rollback(ctx)

	holdID, err := r.holdSeatsCore(ctx, tx, eventID, userID, seatIDs, ttl)
	if err != nil {
		return uuid.Nil, fmt.Errorf("%s:%w", op, translateDBErr(err))
	}

	if err := tx.Commit(ctx); err != nil {
		return uuid.Nil, fmt.Errorf("%s:%w", op, translateDBErr(err))
	}

	return holdID, nil
}

// ConfirmHold confirms a hold and creates an order.
//
// Parameters:
//   - ctx: request-scoped context for cancellation and timeouts.
//   - holdID: unique identifier of the hold to confirm.
//   - totalCents: total amount in cents to charge for the order.
//
// Returns:
//   - uuid.UUID: the order ID when successful.
//   - error: repository.ErrHoldExpired if the hold is expired.
//   - error: repository.ErrNothingToConfirm if there are no seats to confirm.
//   - error: repository.ConflictError if there is a conflict creating the order or tickets.
func (r *ReservationRepo) ConfirmHold(ctx context.Context, holdID uuid.UUID, totalCents int) (uuid.UUID, error) {
	const op = "postgres.ReservationRepo.ConfirmHold"

	if r.db != nil {
		id, err := r.confirmHoldCore(ctx, r.db, holdID, totalCents)
		if err != nil {
			return uuid.Nil, fmt.Errorf("%s:%w", op, translateDBErr(err))
		}
		return id, nil
	}

	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{
		IsoLevel:   pgx.Serializable,
		AccessMode: pgx.ReadWrite,
	})
	if err != nil {
		return uuid.Nil, fmt.Errorf("%s:%w", op, translateDBErr(err))
	}

	defer tx.Rollback(ctx)

	orderID, err := r.confirmHoldCore(ctx, tx, holdID, totalCents)
	if err != nil {
		return uuid.Nil, fmt.Errorf("%s:%w", op, translateDBErr(err))
	}

	if err := tx.Commit(ctx); err != nil {
		return uuid.Nil, fmt.Errorf("%s:%w", op, translateDBErr(err))
	}

	return orderID, nil
}

// CancelHold cancels a hold.
//
// Parameters:
//   - ctx: request-scoped context for cancellation and timeouts.
//   - holdID: unique identifier of the hold to cancel.
//
// Returns:
//   - error: repository.ErrNotFound if the hold is not found.
func (r *ReservationRepo) CancelHold(ctx context.Context, holdID uuid.UUID) error {
	const op = "postgres.ReservationRepo.CancelHold"

	if r.db != nil {
		if err := r.cancelHoldCore(ctx, r.db, holdID); err != nil {
			return fmt.Errorf("%s:%w", op, translateDBErr(err))
		}
		return nil
	}

	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{
		IsoLevel:   pgx.Serializable,
		AccessMode: pgx.ReadWrite,
	})
	if err != nil {
		return fmt.Errorf("%s:%w", op, translateDBErr(err))
	}

	defer tx.Rollback(ctx)

	if err := r.cancelHoldCore(ctx, tx, holdID); err != nil {
		return fmt.Errorf("%s:%w", op, translateDBErr(err))
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("%s:%w", op, translateDBErr(err))
	}

	return nil
}

// ExpireHolds expires old holds.
//
// Parameters:
//   - ctx: request-scoped context for cancellation and timeouts.
//
// Returns:
//   - int64: the number of expired holds.
//   - error: if any error occurs while expiring holds.
func (r *ReservationRepo) ExpireHolds(ctx context.Context) (int64, error) {
	const op = "postgres.ReservationRepo.ExpireHolds"

	db := r.handle()

	var released int64
	tag, err := db.Exec(ctx,
		`UPDATE event_seats
         SET status = 'available', hold_id = NULL, hold_expires_at = NULL
      	 WHERE status = 'held' AND hold_expires_at <= now()`,
	)
	if err != nil {
		return 0, fmt.Errorf("%s:%w", op, translateDBErr(err))
	}

	released += tag.RowsAffected()

	_, err = db.Exec(ctx, `DELETE FROM holds WHERE expires_at <= now()`)
	if err != nil {
		return released, fmt.Errorf("%s:%w", op, translateDBErr(err))
	}

	return released, nil
}

func (r *ReservationRepo) holdSeatsCore(
	ctx context.Context,
	db DB,
	eventID int64,
	userID int64,
	seatIDs []int64,
	ttl time.Duration,
) (uuid.UUID, error) {
	const op = "postgres.ReservationRepo.holdSeatsCore"

	holdID := uuid.New()
	expires := time.Now().Add(ttl)

	if _, err := db.Exec(ctx,
		`UPDATE event_seats
        	SET status = 'available', hold_id = NULL, hold_expires_at = NULL
      	 WHERE event_id = $1
        	AND status = 'held'
        	AND hold_expires_at <= now()`,
		eventID,
	); err != nil {
		return uuid.Nil, fmt.Errorf("%s:%w", op, translateDBErr(err))
	}

	if _, err := db.Exec(ctx,
		`INSERT INTO holds(id, event_id, user_id, expires_at)
       	 VALUES ($1, $2, $3, $4)`,
		holdID, eventID, userID, expires,
	); err != nil {
		return uuid.Nil, fmt.Errorf("%s:%w", op, translateDBErr(err))
	}

	tag, err := db.Exec(ctx,
		`UPDATE event_seats
        	SET status = 'held', hold_id = $3, hold_expires_at = $4
      	 WHERE event_id = $1
        	AND seat_id = ANY($2)
        	AND status = 'available'`,
		eventID, seatIDs, holdID, expires,
	)
	if err != nil {
		return uuid.Nil, fmt.Errorf("%s:%w", op, translateDBErr(err))
	}

	if int(tag.RowsAffected()) != len(seatIDs) {
		return uuid.Nil, fmt.Errorf("%s:%w", op, repository.ErrSeatsUnavailable)
	}

	return holdID, nil
}

func (r *ReservationRepo) confirmHoldCore(
	ctx context.Context,
	db DB,
	holdID uuid.UUID,
	totalCents int,
) (uuid.UUID, error) {
	const op = "postgres.ReservationRepo.confirmHoldCore"

	var eventID int64
	var userID int64

	if err := db.QueryRow(ctx,
		`SELECT event_id, user_id
       	 FROM holds
      	 WHERE id = $1 AND expires_at > now()`,
		holdID,
	).Scan(&eventID, &userID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return uuid.Nil, fmt.Errorf("%s:%w", op, repository.ErrHoldExpired)
		}
		return uuid.Nil, fmt.Errorf("%s:%w", op, translateDBErr(err))
	}

	rows, err := db.Query(ctx,
		`UPDATE event_seats
         SET status = 'sold', hold_id = NULL, hold_expires_at = NULL
      	 WHERE hold_id = $1
      	 RETURNING seat_id`,
		holdID,
	)
	if err != nil {
		return uuid.Nil, fmt.Errorf("%s:%w", op, translateDBErr(err))
	}

	defer rows.Close()

	var seatIDs []int64
	for rows.Next() {
		var sid int64
		if err := rows.Scan(&sid); err != nil {
			return uuid.Nil, fmt.Errorf("%s:%w", op, translateDBErr(err))
		}
		seatIDs = append(seatIDs, sid)
	}
	if err := rows.Err(); err != nil {
		return uuid.Nil, fmt.Errorf("%s:%w", op, translateDBErr(err))
	}

	if len(seatIDs) == 0 {
		return uuid.Nil, fmt.Errorf("%s:%w", op, repository.ErrNothingToConfirm)
	}

	orderID := uuid.New()
	if _, err := db.Exec(ctx,
		`INSERT INTO orders(id, event_id, user_id, total_cents)
       	 VALUES ($1, $2, $3, $4)`,
		orderID, eventID, userID, totalCents,
	); err != nil {
		return uuid.Nil, fmt.Errorf("%s:%w", op, translateDBErr(err))
	}

	batch := &pgx.Batch{}
	for _, sid := range seatIDs {
		batch.Queue(
			`INSERT INTO tickets(id, order_id, event_id, seat_id)
         	 VALUES ($1, $2, $3, $4)`,
			uuid.New(), orderID, eventID, sid,
		)
	}
	if err := db.SendBatch(ctx, batch).Close(); err != nil {
		return uuid.Nil, fmt.Errorf("%s:%w", op, translateDBErr(err))
	}

	_, _ = db.Exec(ctx, `DELETE FROM holds WHERE id = $1`, holdID)

	return orderID, nil
}

func (r *ReservationRepo) cancelHoldCore(ctx context.Context, db DB, holdID uuid.UUID) error {
	const op = "postgres.ReservationRepo.cancelHoldCore"

	_, err := db.Exec(ctx,
		`UPDATE event_seats
         SET status = 'available', hold_id = NULL, hold_expires_at = NULL
      	 WHERE hold_id = $1`,
		holdID,
	)
	if err != nil {
		return fmt.Errorf("%s:%w", op, translateDBErr(err))
	}

	ct, err := db.Exec(ctx, `DELETE FROM holds WHERE id = $1`, holdID)
	if err != nil {
		return fmt.Errorf("%s:%w", op, translateDBErr(err))
	}

	if ct.RowsAffected() == 0 {
		return fmt.Errorf("%s:%w", op, repository.ErrNotFound)
	}

	return nil
}
