package postgresrepo

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kirinyoku/tix-go/internal/domain"
)

type OrderRepo struct {
	pool *pgxpool.Pool
	db   DB
}

func (r *OrderRepo) With(db DB) *OrderRepo {
	cp := *r
	cp.db = db
	return &cp
}

func (r *OrderRepo) handle() DB {
	if r.db != nil {
		return r.db
	}
	return r.pool
}

func (r *OrderRepo) Get(
	ctx context.Context,
	id string,
) (*domain.Order, error) {
	const op = "postgresrepo.OrderRepo.Get"

	db := r.handle()

	var o domain.Order
	err := db.QueryRow(ctx,
		`SELECT id, event_id, user_id, total_cents, created_at
       	 FROM orders WHERE id = $1`,
		id,
	).Scan(&o.ID, &o.EventID, &o.UserID, &o.TotalCents, &o.CreatedAt)
	if err != nil {
		return nil, wrapDBErr(op, err)
	}

	return &o, nil
}
