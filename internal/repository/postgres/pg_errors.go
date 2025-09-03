package postgres

import (
	"errors"

	"github.com/jackc/pgconn"
	"github.com/jackc/pgx/v5"
	"github.com/kirinyoku/tix-go/internal/repository"
)

func IsRetryable(err error) bool {
	var pgErr *pgconn.PgError

	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "40001", "40P01":
			return true
		}
	}

	return false
}

func translateDBErr(err error) error {
	if err == nil {
		return nil
	}

	if errors.Is(err, pgx.ErrNoRows) {
		return repository.ErrNotFound
	}

	var pge *pgconn.PgError
	if errors.As(err, &pge) {
		// unique_violation
		if pge.Code == "23505" {
			return repository.ErrConflict
		}
	}

	return err
}
