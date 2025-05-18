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

// Store provides all functions to execute db queries or transactions.
type Store interface {
	Querier
	ExecTx(ctx context.Context, fn func(*Queries) error) error
	
	CreateUserTx(ctx context.Context, arg CreateUserParams) (User, error)
	
	CreateUserAddressTx(ctx context.Context, arg CreateUserAddressTxParams) (UserAddress, error)
	UpdateUserAddressTx(ctx context.Context, arg UpdateUserAddressParams) (UserAddress, error)
	DeleteUserAddressTx(ctx context.Context, arg DeleteUserAddressParams) error
	
	CreateGundamTx(ctx context.Context, arg CreateGundamTxParams) (GundamDetails, error)
	PublishGundamTx(ctx context.Context, arg PublishGundamTxParams) error
	UnpublishGundamTx(ctx context.Context, arg UnpublishGundamTxParams) error
	UpdateGundamAccessoriesTx(ctx context.Context, arg UpdateGundamAccessoriesParams) error
	AddGundamSecondaryImagesTx(ctx context.Context, arg AddGundamSecondaryImagesTxParams) (AddGundamSecondaryImagesTxResult, error)
	DeleteGundamSecondaryImageTx(ctx context.Context, arg DeleteGundamSecondaryImageTxParams) error
	
	BecomeSellerTx(ctx context.Context, userID string) (User, error)
	HandleZalopayCallbackTx(ctx context.Context, arg HandleZalopayCallbackTxParams) error
	
	CreateOrderTx(ctx context.Context, arg CreateOrderTxParams) (CreateOrderTxResult, error)
	CancelOrderBySellerTx(ctx context.Context, arg CancelOrderBySellerTxParams) (CancelOrderBySellerTxResult, error)
	CancelOrderByBuyerTx(ctx context.Context, arg CancelOrderByBuyerTxParams) (CancelOrderByBuyerTxResult, error)
	ConfirmOrderBySellerTx(ctx context.Context, arg ConfirmOrderTxParams) (ConfirmOrderTxResult, error)
	PackageOrderTx(ctx context.Context, arg PackageOrderTxParams) (PackageOrderTxResult, error)
	CompleteRegularOrderTx(ctx context.Context, arg CompleteRegularOrderTxParams) (CompleteRegularOrderTxResult, error)
	CompleteExchangeOrderTx(ctx context.Context, arg CompleteExchangeOrderTxParams) (CompleteExchangeOrderTxResult, error)
	
	CreateExchangePostTx(ctx context.Context, arg CreateExchangePostTxParams) (CreateExchangePostTxResult, error)
	DeleteExchangePostTx(ctx context.Context, arg DeleteExchangePostTxParams) (DeleteExchangePostTxResult, error)
	CreateExchangeOfferTx(ctx context.Context, arg CreateExchangeOfferTxParams) (CreateExchangeOfferTxResult, error)
	RequestNegotiationForOfferTx(ctx context.Context, arg RequestNegotiationForOfferTxParams) (RequestNegotiationForOfferTxResult, error)
	UpdateExchangeOfferTx(ctx context.Context, arg UpdateExchangeOfferTxParams) (UpdateExchangeOfferTxResult, error)
	AcceptExchangeOfferTx(ctx context.Context, arg AcceptExchangeOfferTxParams) (AcceptExchangeOfferTxResult, error)
	GetGundamDetailsByID(ctx context.Context, q *Queries, gundamID int64) (GundamDetails, error)
	ProvideDeliveryAddressesForExchangeTx(ctx context.Context, arg ProvideDeliveryAddressesForExchangeTxParams) (ProvideDeliveryAddressesForExchangeTxResult, error)
	PayExchangeDeliveryFeeTx(ctx context.Context, arg PayExchangeDeliveryFeeTxParams) (PayExchangeDeliveryFeeTxResult, error)
	CancelExchangeTx(ctx context.Context, arg CancelExchangeTxParams) (CancelExchangeTxResult, error)
	
	CreateAuctionRequestTx(ctx context.Context, arg CreateAuctionRequestTxParams) (AuctionRequest, error)
	DeleteAuctionRequestTx(ctx context.Context, request AuctionRequest) error
	RejectAuctionRequestTx(ctx context.Context, arg RejectAuctionRequestTxParams) (AuctionRequest, error)
	ApproveAuctionRequestTx(ctx context.Context, arg ApproveAuctionRequestTxParams) (ApproveAuctionRequestTxResult, error)
	ParticipateInAuctionTx(ctx context.Context, arg ParticipateInAuctionTxParams) (ParticipateInAuctionTxResult, error)
	PlaceBidTx(ctx context.Context, arg PlaceBidTxParams) (PlaceBidTxResult, error)
	EndAuctionTx(ctx context.Context, arg EndAuctionTxParams) (EndAuctionTxResult, error)
	HandleAuctionNonPaymentTx(ctx context.Context, arg HandleAuctionNonPaymentTxParams) (HandleAuctionNonPaymentTxResult, error)
	PayAuctionWinningBidTx(ctx context.Context, arg PayAuctionWinningBidTxParams) (PayAuctionWinningBidTxResult, error)
}

type SQLStore struct {
	*Queries               // Dùng cho các truy vấn SQL đơn lẻ
	ConnPool *pgxpool.Pool // Dùng để khởi tạo transaction
}

// NewStore creates a new Store.
func NewStore(db *pgxpool.Pool) Store {
	return &SQLStore{
		Queries:  New(db),
		ConnPool: db,
	}
}
