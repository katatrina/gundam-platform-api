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
	ErrSubscriptionLimitExceeded = errors.New("subscription limit exceeded")
	ErrSubscriptionExpired       = errors.New("subscription has expired")
	ErrCartItemExists            = errors.New("item already exists in cart")
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
