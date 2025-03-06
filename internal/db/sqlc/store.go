package db

import (
	"context"
	
	"github.com/jackc/pgx/v5/pgxpool"
)

// Store provides all functions to execute db queries and transactions.
type Store interface {
	Querier
	CreateUserAddressTx(ctx context.Context, arg CreateUserAddressTxParams) (UserAddress, error)
	UpdateUserAddressTx(ctx context.Context, arg UpdateUserAddressParams) (UserAddress, error)
	DeleteUserAddressTx(ctx context.Context, arg DeleteUserAddressParams) error
	CreateGundamTx(ctx context.Context, arg CreateGundamTxParams) error
	BecomeSellerTx(ctx context.Context, userID string) (User, error)
}

type SQLStore struct {
	*Queries
	ConnPool *pgxpool.Pool
}

// NewStore creates a new Store.
func NewStore(db *pgxpool.Pool) Store {
	return &SQLStore{
		Queries:  New(db),
		ConnPool: db,
	}
}
