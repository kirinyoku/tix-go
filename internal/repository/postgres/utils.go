package postgresrepo

import (
	"errors"
	"fmt"

	"github.com/jackc/pgconn"
	"github.com/jackc/pgx/v5"
	repository "github.com/kirinyoku/tix-go/internal/repository"
)

// wrapDBErr maps common DB errors to repository-level errors and wraps them with
// the provided operation name.
func wrapDBErr(op string, err error) error {
	if err == nil {
		return nil
	}

	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("%s:%w", op, repository.ErrNotFound)
	}

	var pge *pgconn.PgError
	if errors.As(err, &pge) {
		// unique_violation
		if pge.Code == "23505" {
			return fmt.Errorf("%s:%w", op, repository.ErrConflict)
		}
	}

	return fmt.Errorf("%s:%w", op, err)
}
