package db

import (
	"errors"
	"fmt"
	
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// PostgreSQL error codes
const (
	UniqueViolationCode = "23505"
)

// Constraint names
const (
	UniqueEmailConstraint         = "users_email_key"
	UniqueSellerProfileConstraint = "seller_profiles_pkey"
)

// Common errors
var (
	ErrRecordNotFound            = pgx.ErrNoRows
	ErrPrimaryAddressDeletion    = errors.New("primary address cannot be deleted")
	ErrPickupAddressDeletion     = errors.New("pickup address cannot be deleted")
	ErrSubscriptionLimitExceeded = errors.New("subscription limit exceeded")
	ErrSubscriptionExpired       = errors.New("subscription has expired")
	ErrCartItemExists            = errors.New("item already exists in cart")
	ErrExchangeOfferUnique       = errors.New("user already has an offer for this exchange post")
	ErrDuplicateParticipation    = errors.New("already participated in this auction")
	ErrInsufficientBalance       = errors.New("insufficient balance")
	ErrAuctionEnded              = errors.New("auction has ended")
	ErrBidTooLow                 = errors.New("bid amount too low")
)

// PgError represents a PostgreSQL error with its code, message and constraint name
type PgError struct {
	Code           string
	Message        string
	ConstraintName string
	Detail         string
}

// ErrorDescription returns details about a PostgreSQL error if present
func ErrorDescription(err error) *PgError {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return &PgError{
			Code:           pgErr.Code,
			Message:        pgErr.Message,
			ConstraintName: pgErr.ConstraintName,
			Detail:         pgErr.Detail,
		}
	}
	return nil
}

func IsValidGundamStatus(status string) error {
	if !GundamStatus(status).Valid() {
		err := fmt.Errorf("invalid status: %s, must be one of %v", status, AllGundamStatusValues())
		return err
	}
	
	return nil
}

func IsValidOrderStatus(status string) error {
	if !OrderStatus(status).Valid() {
		err := fmt.Errorf("invalid status: %s, must be one of %v", status, AllOrderStatusValues())
		return err
	}
	
	return nil
}

func IsValidExchangePostStatus(status string) error {
	if !ExchangePostStatus(status).Valid() {
		err := fmt.Errorf("invalid status: %s, must be one of %v", status, AllExchangePostStatusValues())
		return err
	}
	
	return nil
}

func IsValidExchangeStatus(status string) error {
	if !ExchangeStatus(status).Valid() {
		err := fmt.Errorf("invalid status: %s, must be one of %v", status, AllExchangeStatusValues())
		return err
	}
	
	return nil
}

func IsValidAuctionRequestStatus(status string) error {
	if !AuctionRequestStatus(status).Valid() {
		err := fmt.Errorf("invalid status: %s, must be one of %v", status, AllAuctionRequestStatusValues())
		return err
	}
	
	return nil
}

func IsValidAuctionStatus(status string) error {
	if !AuctionStatus(status).Valid() {
		err := fmt.Errorf("invalid status: %s, must be one of %v", status, AllAuctionStatusValues())
		return err
	}
	
	return nil
}

func IsValidWalletEntryStatus(status string) error {
	if !WalletEntryStatus(status).Valid() {
		err := fmt.Errorf("invalid status: %s, must be one of %v", status, AllWalletEntryStatusValues())
		return err
	}
	
	return nil
}
