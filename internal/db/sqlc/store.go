package db

import (
	"context"
	
	"github.com/jackc/pgx/v5/pgxpool"
)

// Store provides all functions to execute db queries and transactions.
type Store interface {
	Querier
	CreateUserAddressTx(ctx context.Context, arg CreateUserAddressTxParams) error
}

type SQLStore struct {
	*Queries
	connPool *pgxpool.Pool
}

// NewStore creates a new Store.
func NewStore(db *pgxpool.Pool) Store {
	return &SQLStore{
		Queries:  New(db),
		connPool: db,
	}
}
