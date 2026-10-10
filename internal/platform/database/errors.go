package database

import (
	"errors"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
)

// IsUniqueViolation reports whether err is a unique or primary-key
// constraint violation, on either dialect.
func IsUniqueViolation(err error) bool {
	var pg *pgconn.PgError
	if errors.As(err, &pg) {
		return pg.Code == "23505"
	}
	return err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed")
}

// IsForeignKeyViolation reports whether err is a foreign-key violation, on
// either dialect.
func IsForeignKeyViolation(err error) bool {
	var pg *pgconn.PgError
	if errors.As(err, &pg) {
		return pg.Code == "23503"
	}
	return err != nil && strings.Contains(err.Error(), "FOREIGN KEY constraint failed")
}
