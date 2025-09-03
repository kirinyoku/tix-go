package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type DB interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	SendBatch(ctx context.Context, b *pgx.Batch) pgx.BatchResults
}

type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{
		pool: pool,
	}
}

func (s *Store) RunTx(
	ctx context.Context,
	opts *pgx.TxOptions,
	fn func(ctx context.Context, tx DB) error,
) error {
	txOpts := pgx.TxOptions{
		IsoLevel:   pgx.Serializable,
		AccessMode: pgx.ReadWrite,
	}

	if opts != nil {
		txOpts.IsoLevel = opts.IsoLevel
		txOpts.AccessMode = opts.AccessMode
		txOpts.DeferrableMode = opts.DeferrableMode
	}

	tx, err := s.pool.BeginTx(ctx, txOpts)
	if err != nil {
		return err
	}

	defer tx.Rollback(ctx)

	if err := fn(ctx, tx); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}

	return nil
}

func (s *Store) Query() *QueryRepo              { return &QueryRepo{pool: s.pool} }
func (s *Store) Admin() *AdminRepo              { return &AdminRepo{pool: s.pool} }
func (s *Store) Orders() *OrderRepo             { return &OrderRepo{pool: s.pool} }
func (s *Store) Reservations() *ReservationRepo { return &ReservationRepo{pool: s.pool} }
