package pg

import (
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
)

func constraintViolation(err error, code string) (string, bool) {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == code {
		return pgErr.ConstraintName, true
	}
	return "", false
}
