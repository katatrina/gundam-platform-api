package db

import (
	"errors"
	"fmt"
	
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

const (
	UniqueViolationCode = "23505"
)

const (
	UniqueEmailConstraint = "users_email_key"
)

var ErrRecordNotFound = pgx.ErrNoRows

// ErrorDescription returns the error code and constraint name from a Postgres error.
func ErrorDescription(err error) (errCode string, constraintName string) {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		fmt.Println("pgErr: ", pgErr)
		return pgErr.Code, pgErr.ConstraintName
	}
	
	return
}
