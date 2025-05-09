package db

import (
	"context"
	"fmt"
	"time"
	
	"github.com/google/uuid"
	"github.com/katatrina/gundam-BE/internal/util"
	"github.com/shopspring/decimal"
)

type CreateAuctionRequestTxParams struct {
	Gundam         Gundam
	GundamSnapshot GundamSnapshot
	StartingPrice  int64
	BidIncrement   int64
	BuyNowPrice    *int64
	StartTime      time.Time
	EndTime        time.Time
	Subscription   GetCurrentActiveSubscriptionDetailsForSellerRow
}

func (store *SQLStore) CreateAuctionRequestTx(ctx context.Context, arg CreateAuctionRequestTxParams) (AuctionRequest, error) {
	var request AuctionRequest
	
	err := store.ExecTx(ctx, func(qTx *Queries) error {
		// 1. Tính toán deposite_rate (15%) và deposit_amount
		depositRate := decimal.NewFromFloat(0.15) // 15% cố định
		
		startingPriceDecimal := decimal.NewFromInt(arg.StartingPrice)
		depositAmountDecimal := startingPriceDecimal.Mul(depositRate)
		depositAmount := depositAmountDecimal.IntPart()
		
		auctionRequestID, err := uuid.NewV7()
		if err != nil {
			return fmt.Errorf("failed to generate auction request ID: %w", err)
		}
		
		// 2. Tạo yêu cầu đấu giá
		auctionRequest, err := qTx.CreateAuctionRequest(ctx, CreateAuctionRequestParams{
			ID:             auctionRequestID,
			GundamID:       &arg.Gundam.ID,
			SellerID:       arg.Gundam.OwnerID,
			GundamSnapshot: arg.GundamSnapshot,
			StartingPrice:  arg.StartingPrice,
			BidIncrement:   arg.BidIncrement,
			BuyNowPrice:    arg.BuyNowPrice,
			DepositRate:    depositRate,
			DepositAmount:  depositAmount,
			StartTime:      arg.StartTime,
			EndTime:        arg.EndTime,
		})
		if err != nil {
			return fmt.Errorf("failed to create auction request: %w", err)
		}
		request = auctionRequest
		
		// 3. Cập nhật trạng thái Gundam thành "pending_auction_approval"
		err = qTx.UpdateGundam(ctx, UpdateGundamParams{
			ID: arg.Gundam.ID,
			Status: NullGundamStatus{
				GundamStatus: GundamStatusPendingAuctionApproval,
				Valid:        true,
			},
		})
		if err != nil {
			return fmt.Errorf("failed to update gundam status: %w", err)
		}
		
		// 4. Cập nhật open_auctions_used của người bán
		err = qTx.UpdateCurrentActiveSubscriptionForSeller(ctx, UpdateCurrentActiveSubscriptionForSellerParams{
			SubscriptionID:   arg.Subscription.ID,
			SellerID:         arg.Gundam.OwnerID,
			OpenAuctionsUsed: util.Int64Pointer(arg.Subscription.OpenAuctionsUsed + 1),
		})
		if err != nil {
			return fmt.Errorf("failed to update open auctions used: %w", err)
		}
		
		return nil
	})
	
	return request, err
}
