package postgresrepo

import (
	"context"
	"errors"
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

func (r *ReservationRepo) HoldSeats(
	ctx context.Context,
	eventID int64,
	userID int64,
	seatIDs []int64,
	ttl time.Duration,
) (uuid.UUID, error) {
	const op = "postgresrepo.ReservationRepo.HoldSeats"

	if len(seatIDs) == 0 {
		return uuid.Nil, repository.ErrSeatsUnavailable
	}

	if r.db != nil {
		id, err := r.holdSeatsCore(ctx, r.db, eventID, userID, seatIDs, ttl)
		if err != nil {
			return uuid.Nil, wrapDBErr(op, err)
		}
		return id, nil
	}

	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{
		IsoLevel:   pgx.Serializable,
		AccessMode: pgx.ReadWrite,
	})
	if err != nil {
		return uuid.Nil, wrapDBErr(op, err)
	}

	defer tx.Rollback(ctx)

	holdID, err := r.holdSeatsCore(ctx, tx, eventID, userID, seatIDs, ttl)
	if err != nil {
		return uuid.Nil, wrapDBErr(op, err)
	}

	if err := tx.Commit(ctx); err != nil {
		return uuid.Nil, wrapDBErr(op, err)
	}

	return holdID, nil
}

func (r *ReservationRepo) ConfirmHold(
	ctx context.Context,
	holdID uuid.UUID,
	totalCents int,
) (uuid.UUID, error) {
	const op = "postgresrepo.ReservationRepo.ConfirmHold"

	if r.db != nil {
		id, err := r.confirmHoldCore(ctx, r.db, holdID, totalCents)
		if err != nil {
			return uuid.Nil, wrapDBErr(op, err)
		}
		return id, nil
	}

	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{
		IsoLevel:   pgx.Serializable,
		AccessMode: pgx.ReadWrite,
	})
	if err != nil {
		return uuid.Nil, wrapDBErr(op, err)
	}

	defer tx.Rollback(ctx)

	orderID, err := r.confirmHoldCore(ctx, tx, holdID, totalCents)
	if err != nil {
		return uuid.Nil, wrapDBErr(op, err)
	}

	if err := tx.Commit(ctx); err != nil {
		return uuid.Nil, wrapDBErr(op, err)
	}

	return orderID, nil
}

func (r *ReservationRepo) CancelHold(
	ctx context.Context,
	holdID uuid.UUID,
) error {
	const op = "postgresrepo.ReservationRepo.CancelHold"

	if r.db != nil {
		if err := r.cancelHoldCore(ctx, r.db, holdID); err != nil {
			return wrapDBErr(op, err)
		}
		return nil
	}

	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{
		IsoLevel:   pgx.Serializable,
		AccessMode: pgx.ReadWrite,
	})
	if err != nil {
		return wrapDBErr(op, err)
	}

	defer tx.Rollback(ctx)

	if err := r.cancelHoldCore(ctx, tx, holdID); err != nil {
		return wrapDBErr(op, err)
	}

	if err := tx.Commit(ctx); err != nil {
		return wrapDBErr(op, err)
	}

	return nil
}

func (r *ReservationRepo) ExpireHolds(ctx context.Context) (int64, error) {
	const op = "postgresrepo.ReservedRepo.ExpireHolds"

	db := r.handle()

	var released int64
	tag, err := db.Exec(ctx,
		`UPDATE event_seats
         SET status = 'available', hold_id = NULL, hold_expires_at = NULL
      	 WHERE status = 'held' AND hold_expires_at <= now()`,
	)
	if err != nil {
		return 0, wrapDBErr(op, err)
	}

	released += tag.RowsAffected()

	_, err = db.Exec(ctx, `DELETE FROM holds WHERE expires_at <= now()`)
	if err != nil {
		return released, wrapDBErr(op, err)
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
	const op = "postgresrepo.ReservationRepo.holdSeatsCore"
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
		return uuid.Nil, wrapDBErr(op, err)
	}

	if _, err := db.Exec(ctx,
		`INSERT INTO holds(id, event_id, user_id, expires_at)
       	 VALUES ($1, $2, $3, $4)`,
		holdID, eventID, userID, expires,
	); err != nil {
		return uuid.Nil, wrapDBErr(op, err)
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
		return uuid.Nil, wrapDBErr(op, err)
	}

	if int(tag.RowsAffected()) != len(seatIDs) {
		return uuid.Nil, wrapDBErr(op, repository.ErrSeatsUnavailable)
	}

	return holdID, nil
}

func (r *ReservationRepo) confirmHoldCore(
	ctx context.Context,
	db DB,
	holdID uuid.UUID,
	totalCents int,
) (uuid.UUID, error) {
	const op = "postgresrepo.ReservationRepo.confirmHoldCore"
	var eventID int64
	var userID int64

	if err := db.QueryRow(ctx,
		`SELECT event_id, user_id
       	 FROM holds
      	 WHERE id = $1 AND expires_at > now()`,
		holdID,
	).Scan(&eventID, &userID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return uuid.Nil, wrapDBErr(op, repository.ErrHoldExpired)
		}
		return uuid.Nil, wrapDBErr(op, err)
	}

	rows, err := db.Query(ctx,
		`UPDATE event_seats
         SET status = 'sold', hold_id = NULL, hold_expires_at = NULL
      	 WHERE hold_id = $1
      	 RETURNING seat_id`,
		holdID,
	)
	if err != nil {
		return uuid.Nil, wrapDBErr(op, err)
	}

	defer rows.Close()

	var seatIDs []int64
	for rows.Next() {
		var sid int64
		if err := rows.Scan(&sid); err != nil {
			return uuid.Nil, wrapDBErr(op, err)
		}
		seatIDs = append(seatIDs, sid)
	}
	if err := rows.Err(); err != nil {
		return uuid.Nil, wrapDBErr(op, err)
	}

	if len(seatIDs) == 0 {
		return uuid.Nil, wrapDBErr(op, repository.ErrNothingToConfirm)
	}

	orderID := uuid.New()
	if _, err := db.Exec(ctx,
		`INSERT INTO orders(id, event_id, user_id, total_cents)
       	 VALUES ($1, $2, $3, $4)`,
		orderID, eventID, userID, totalCents,
	); err != nil {
		return uuid.Nil, wrapDBErr(op, err)
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
		return uuid.Nil, wrapDBErr(op, err)
	}

	_, _ = db.Exec(ctx, `DELETE FROM holds WHERE id = $1`, holdID)

	return orderID, nil
}

func (r *ReservationRepo) cancelHoldCore(
	ctx context.Context,
	db DB,
	holdID uuid.UUID,
) error {
	const op = "postgresrepo.ReservationRepo.cancelHoldCore"
	_, err := db.Exec(ctx,
		`UPDATE event_seats
         SET status = 'available', hold_id = NULL, hold_expires_at = NULL
      	 WHERE hold_id = $1`,
		holdID,
	)
	if err != nil {
		return wrapDBErr(op, err)
	}

	ct, err := db.Exec(ctx, `DELETE FROM holds WHERE id = $1`, holdID)
	if err != nil {
		return wrapDBErr(op, err)
	}

	if ct.RowsAffected() == 0 {
		return wrapDBErr(op, repository.ErrHoldNotFound)
	}

	return nil
}
