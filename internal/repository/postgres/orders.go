package postgres

import (
	"context"
	"fmt"

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

// Get retrieves an order by its ID.
//
// Parameters:
//   - ctx: request-scoped context for cancellation and timeouts.
//   - id: string identifier of the order to retrieve.
//
// Returns:
//   - *domain.Order: the order when found.
//   - error: repository.ErrNotFound if the order does not exist.
func (r *OrderRepo) Get(ctx context.Context, id string) (*domain.Order, error) {
	const op = "postgres.OrderRepo.Get"

	db := r.handle()

	var o domain.Order
	err := db.QueryRow(ctx,
		`SELECT id, event_id, user_id, total_cents, created_at
			 FROM orders WHERE id = $1`,
		id,
	).Scan(&o.ID, &o.EventID, &o.UserID, &o.TotalCents, &o.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("%s:%w", op, translateDBErr(err))
	}

	return &o, nil
}
