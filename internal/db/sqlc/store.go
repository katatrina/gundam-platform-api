package db

import (
	"context"
	
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	TrialSellerSubscriptionName     = "GÓI DÙNG THỬ"
	PremiumSellerSubscriptionName   = "GÓI NÂNG CẤP"
	UnlimitedSellerSubscriptionName = "GÓI KHÔNG GIỚI HẠN"
)

// Store provides all functions to execute db queries and transactions.
type Store interface {
	Querier
	CreateUserTx(ctx context.Context, arg CreateUserParams) (User, error)
	CreateUserAddressTx(ctx context.Context, arg CreateUserAddressTxParams) (UserAddress, error)
	UpdateUserAddressTx(ctx context.Context, arg UpdateUserAddressParams) (UserAddress, error)
	DeleteUserAddressTx(ctx context.Context, arg DeleteUserAddressParams) error
	CreateGundamTx(ctx context.Context, arg CreateGundamTxParams) error
	BecomeSellerTx(ctx context.Context, userID string) (User, error)
	PublishGundamTx(ctx context.Context, arg PublishGundamTxParams) error
	UnpublishGundamTx(ctx context.Context, arg UnpublishGundamTxParams) error
	CreateOrderTx(ctx context.Context, arg CreateOrderTxParams) (CreateOrderTxResult, error)
	HandleZalopayCallbackTx(ctx context.Context, arg HandleZalopayCallbackTxParams) error
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
