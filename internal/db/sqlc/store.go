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
	CreateGundamTx(ctx context.Context, arg CreateGundamTxParams) (GundamDetails, error)
	BecomeSellerTx(ctx context.Context, userID string) (User, error)
	PublishGundamTx(ctx context.Context, arg PublishGundamTxParams) error
	UnpublishGundamTx(ctx context.Context, arg UnpublishGundamTxParams) error
	CreateOrderTx(ctx context.Context, arg CreateOrderTxParams) (CreateOrderTxResult, error)
	HandleZalopayCallbackTx(ctx context.Context, arg HandleZalopayCallbackTxParams) error
	ConfirmOrderBySellerTx(ctx context.Context, arg ConfirmOrderTxParams) (ConfirmOrderTxResult, error)
	PackageOrderBySellerTx(ctx context.Context, arg PackageOrderTxParams) (PackageOrderTxResult, error)
	ConfirmOrderReceivedByBuyerTx(ctx context.Context, arg ConfirmOrderReceivedTxParams) (ConfirmOrderReceivedTxResult, error)
	CancelOrderByBuyerTx(ctx context.Context, arg CancelOrderByBuyerTxParams) (CancelOrderByBuyerTxResult, error)
	CancelOrderBySellerTx(ctx context.Context, arg CancelOrderBySellerTxParams) (CancelOrderBySellerTxResult, error)
	CreateExchangePostTx(ctx context.Context, arg CreateExchangePostTxParams) (CreateExchangePostTxResult, error)
	DeleteExchangePostTx(ctx context.Context, arg DeleteExchangePostTxParams) (DeleteExchangePostTxResult, error)
	CreateExchangeOfferTx(ctx context.Context, arg CreateExchangeOfferTxParams) (CreateExchangeOfferTxResult, error)
	RequestNegotiationForOfferTx(ctx context.Context, arg RequestNegotiationForOfferTxParams) (RequestNegotiationForOfferTxResult, error)
	UpdateExchangeOfferTx(ctx context.Context, arg UpdateExchangeOfferTxParams) (UpdateExchangeOfferTxResult, error)
	AcceptExchangeOfferTx(ctx context.Context, arg AcceptExchangeOfferTxParams) (AcceptExchangeOfferTxResult, error)
	GetGundamDetailsByID(ctx context.Context, gundamID int64) (GundamDetails, error)
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
