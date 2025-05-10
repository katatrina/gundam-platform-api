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

func (store *SQLStore) DeleteAuctionRequestTx(ctx context.Context, request AuctionRequest) error {
	return store.ExecTx(ctx, func(qTx *Queries) error {
		// 1. Xóa yêu cầu đấu giá
		err := qTx.DeleteAuctionRequest(ctx, request.ID)
		if err != nil {
			return err
		}
		
		// 2. Kiểm tra thông tin và cập nhật trạng thái Gundam
		if request.GundamID != nil {
			gundam, err := qTx.GetGundamByID(ctx, *request.GundamID)
			if err != nil {
				return fmt.Errorf("failed to get gundam by ID: %w", err)
			}
			
			// Chỉ cập nhật status nếu gundam đang pending_auction_approval
			if gundam.Status == GundamStatusPendingAuctionApproval {
				err = qTx.UpdateGundam(ctx, UpdateGundamParams{
					ID: *request.GundamID,
					Status: NullGundamStatus{
						GundamStatus: GundamStatusInstore,
						Valid:        true,
					},
				})
				if err != nil {
					return err
				}
			}
		}
		
		// 3. Chỉ giảm open_auctions_used nếu request có status "pending"
		// Nếu request có status "rejected" thì đã hoàn trả từ lúc reject rồi
		if request.Status == AuctionRequestStatusPending {
			subscription, err := qTx.GetCurrentActiveSubscriptionDetailsForSeller(ctx, request.SellerID)
			if err != nil {
				return err
			}
			
			if subscription.OpenAuctionsUsed > 0 {
				err = qTx.UpdateCurrentActiveSubscriptionForSeller(ctx, UpdateCurrentActiveSubscriptionForSellerParams{
					SubscriptionID:   subscription.ID,
					SellerID:         request.SellerID,
					OpenAuctionsUsed: util.Int64Pointer(subscription.OpenAuctionsUsed - 1),
				})
				if err != nil {
					return err
				}
			}
		}
		
		return nil
	})
}

type RejectAuctionRequestTxParams struct {
	RequestID      uuid.UUID
	RejectedBy     string
	RejectedReason string
}

func (store *SQLStore) RejectAuctionRequestTx(ctx context.Context, arg RejectAuctionRequestTxParams) (AuctionRequest, error) {
	var request AuctionRequest
	err := store.ExecTx(ctx, func(qTx *Queries) error {
		// 1. Cập nhật trạng thái yêu cầu đấu giá thành "rejected"
		rejectedRequest, err := qTx.UpdateAuctionRequest(ctx, UpdateAuctionRequestParams{
			ID: arg.RequestID,
			Status: NullAuctionRequestStatus{
				AuctionRequestStatus: AuctionRequestStatusRejected,
				Valid:                true,
			},
			RejectedBy:     &arg.RejectedBy,
			RejectedReason: &arg.RejectedReason,
		})
		if err != nil {
			return fmt.Errorf("failed to update auction request status: %w", err)
		}
		request = rejectedRequest
		
		// 2. Cập nhật trạng thái Gundam về "in store"
		if rejectedRequest.GundamID != nil {
			err = qTx.UpdateGundam(ctx, UpdateGundamParams{
				ID: *rejectedRequest.GundamID,
				Status: NullGundamStatus{
					GundamStatus: GundamStatusInstore,
					Valid:        true,
				},
			})
			if err != nil {
				return fmt.Errorf("failed to update gundam status: %w", err)
			}
		}
		
		// 3. Lấy thông tin subscription của người bán
		subscription, err := qTx.GetCurrentActiveSubscriptionDetailsForSeller(ctx, rejectedRequest.SellerID)
		if err != nil {
			return fmt.Errorf("failed to get subscription details: %w", err)
		}
		
		// 4. Giảm open_auctions_used trong subscription
		if subscription.OpenAuctionsUsed > 0 {
			err = qTx.UpdateCurrentActiveSubscriptionForSeller(ctx, UpdateCurrentActiveSubscriptionForSellerParams{
				SubscriptionID:   subscription.ID,
				SellerID:         rejectedRequest.SellerID,
				OpenAuctionsUsed: util.Int64Pointer(subscription.OpenAuctionsUsed - 1),
			})
			if err != nil {
				return fmt.Errorf("failed to update open auctions used: %w", err)
			}
		}
		
		return nil
	})
	
	return request, err
}
