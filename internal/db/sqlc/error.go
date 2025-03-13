package db

import (
	"errors"
	
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// PostgreSQL error codes
const (
	UniqueViolationCode = "23505"
)

// Constraint names
const (
	UniqueEmailConstraint = "users_email_key"
)

// Common errors
var (
	ErrRecordNotFound            = pgx.ErrNoRows
	ErrPrimaryAddressDeletion    = errors.New("primary address cannot be deleted")
	ErrPickupAddressDeletion     = errors.New("pickup address cannot be deleted")
	ErrGundamNotAvailableForSale = errors.New("gundam is not available for sale")
	ErrSubscriptionLimitExceeded = errors.New("subscription limit exceeded")
)

// PgError represents a PostgreSQL error with its code, message and constraint name
type PgError struct {
	Code           string
	Message        string
	ConstraintName string
}

// ErrorDescription returns details about a PostgreSQL error if present
func ErrorDescription(err error) *PgError {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return &PgError{
			Code:           pgErr.Code,
			Message:        pgErr.Message,
			ConstraintName: pgErr.ConstraintName,
		}
	}
	return nil
}
