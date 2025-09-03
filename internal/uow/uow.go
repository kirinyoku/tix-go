package uow

import (
	"context"

	"github.com/jackc/pgx/v5"

	postgres "github.com/kirinyoku/tix-go/internal/repository/postgres"
)

// AfterCommit is a function that runs after a successful transaction commit.
type AfterCommit func(ctx context.Context)

// UoW represents a unit of work.
type UoW struct {
	store *postgres.Store
}

func NewUoW(store *postgres.Store) *UoW {
	return &UoW{store: store}
}

// Do runs fn inside the transaction. After a successful commit,
// it executes all after-commit hooks.
func (u *UoW) Do(
	ctx context.Context,
	fn func(ctx context.Context, tx postgres.DB, after func(AfterCommit)) error,
) error {
	return u.DoWithOpts(ctx, nil, fn)
}

// DoWithOpts runs fn inside the transaction with the given options. After a successful commit,
// it executes all after-commit hooks.
func (u *UoW) DoWithOpts(
	ctx context.Context,
	opts *pgx.TxOptions,
	fn func(ctx context.Context, tx postgres.DB, after func(AfterCommit)) error,
) error {
	var hooks []AfterCommit

	err := u.store.RunTx(ctx, opts, func(ctx context.Context, tx postgres.DB) error {
		return fn(ctx, tx, func(h AfterCommit) {
			hooks = append(hooks, h)
		})
	})
	if err != nil {
		return err
	}

	for _, h := range hooks {
		h(ctx)
	}

	return nil
}
