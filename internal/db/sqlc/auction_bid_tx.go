package db

import (
	"context"
	"errors"
	"fmt"
	"time"
	
	"github.com/google/uuid"
	"github.com/katatrina/gundam-BE/internal/util"
)

type PlaceBidTxParams struct {
	UserID        string
	Auction       Auction
	ParticipantID uuid.UUID
	Amount        int64
	OnBuyNowFunc  func(auctionID uuid.UUID) error // Callback để xóa task khi người dùng đặt giá bằng hoặc lớn hơn giá mua ngay
}

type PlaceBidTxResult struct {
	AuctionBid      AuctionBid `json:"auction_bid"`
	Auction         Auction    `json:"updated_auction"`
	PreviousBidder  *User      `json:"previous_bidder"`
	IsBuyNow        bool       `json:"is_buy_now"`
	RefundedUserIDs []string   `json:"refunded_user_ids"`
}

func (store *SQLStore) PlaceBidTx(ctx context.Context, arg PlaceBidTxParams) (PlaceBidTxResult, error) {
	var result PlaceBidTxResult
	
	err := store.ExecTx(ctx, func(qTx *Queries) error {
		var err error
		
		// 1. Lấy thông tin auction mới nhất trong transaction để tránh race condition
		auction, err := qTx.GetAuctionByIDForUpdate(ctx, arg.Auction.ID)
		if err != nil {
			return err
		}
		
		// Kiểm tra trạng thái auction
		if auction.Status != AuctionStatusActive {
			return ErrAuctionEnded
		}
		
		// Kiểm tra lại thời gian kết thúc
		if time.Now().After(auction.EndTime) {
			return ErrAuctionEnded
		}
		
		// Kiểm tra lại giá đặt có đủ cao không
		if arg.Amount < auction.CurrentPrice+auction.BidIncrement {
			return ErrBidTooLow
		}
		
		// 2. Tạo bản ghi đặt giá mới
		auctionBidID, err := uuid.NewV7()
		if err != nil {
			return fmt.Errorf("failed to generate auctionBid ID: %w", err)
		}
		
		auctionBid, err := qTx.CreateAuctionBid(ctx, CreateAuctionBidParams{
			ID:            auctionBidID,
			AuctionID:     &arg.Auction.ID,
			BidderID:      &arg.UserID,
			ParticipantID: arg.ParticipantID,
			Amount:        arg.Amount,
		})
		if err != nil {
			return fmt.Errorf("failed to create auction auctionBid: %w", err)
		}
		result.AuctionBid = auctionBid
		
		// 3. Lấy thông tin người đặt giá trước đó (nếu có)
		if auction.WinningBidID != nil {
			previousBid, err := qTx.GetAuctionBidByID(ctx, *auction.WinningBidID)
			if err != nil && !errors.Is(err, ErrRecordNotFound) {
				return fmt.Errorf("failed to get previous auctionBid: %w", err)
			}
			
			if err == nil && previousBid.BidderID != nil {
				prevBidder, err := qTx.GetUserByID(ctx, *previousBid.BidderID)
				if err != nil && !errors.Is(err, ErrRecordNotFound) {
					return fmt.Errorf("failed to get previous bidder: %w", err)
				}
				
				if err == nil {
					result.PreviousBidder = &prevBidder
				}
			}
		}
		
		// 4. Cập nhật thông tin auction
		updatedAuction, err := qTx.UpdateAuction(ctx, UpdateAuctionParams{
			ID:           arg.Auction.ID,
			CurrentPrice: &arg.Amount,
			WinningBidID: &auctionBid.ID,
		})
		if err != nil {
			return fmt.Errorf("failed to update auction with new auctionBid: %w", err)
		}
		result.Auction = updatedAuction
		
		// 5. Tăng số lượng bids
		updatedAuction, err = qTx.IncrementAuctionTotalBids(ctx, arg.Auction.ID)
		if err != nil {
			return fmt.Errorf("failed to increment total bids: %w", err)
		}
		result.Auction = updatedAuction
		
		// 6. Kiểm tra nếu đạt giá mua ngay
		isBuyNow := auction.BuyNowPrice != nil && arg.Amount >= *auction.BuyNowPrice
		if isBuyNow {
			result.IsBuyNow = true
			
			// Cập nhật trạng thái auction sang "ended"
			actualEndTime := time.Now()
			updatedAuction, err = qTx.UpdateAuction(ctx, UpdateAuctionParams{
				ID: arg.Auction.ID,
				Status: NullAuctionStatus{
					AuctionStatus: AuctionStatusEnded,
					Valid:         true,
				},
				ActualEndTime: &actualEndTime,
			})
			if err != nil {
				return fmt.Errorf("failed to update auction status: %w", err)
			}
			result.Auction = updatedAuction
			
			// Thiết lập deadline thanh toán (48 giờ từ thời điểm kết thúc)
			paymentDeadline := actualEndTime.Add(48 * time.Hour)
			updatedAuction, err = qTx.UpdateAuction(ctx, UpdateAuctionParams{
				ID:                    arg.Auction.ID,
				WinnerPaymentDeadline: util.TimePointer(paymentDeadline),
			})
			result.Auction = updatedAuction
			
			// Hoàn tiền đặt cọc cho tất cả người tham gia khác
			participants, err := qTx.ListAuctionParticipantsExcept(ctx, ListAuctionParticipantsExceptParams{
				AuctionID: arg.Auction.ID,
				UserID:    arg.UserID,
			})
			if err != nil {
				return fmt.Errorf("failed to list auction participants: %w", err)
			}
			
			result.RefundedUserIDs = make([]string, 0, len(participants))
			for _, p := range participants {
				// Hoàn tiền đặt cọc
				_, err = qTx.CreateWalletEntry(ctx, CreateWalletEntryParams{
					WalletID:      p.UserID,
					ReferenceID:   util.StringPointer(auction.ID.String()),
					ReferenceType: WalletReferenceTypeAuction,
					EntryType:     WalletEntryTypeAuctionDepositRefund,
					Amount:        auction.DepositAmount, // Số tiền dương để cộng
					Status:        WalletEntryStatusCompleted,
					CompletedAt:   util.TimePointer(time.Now()),
				})
				if err != nil {
					return fmt.Errorf("failed to create refund entry for user %s: %w", p.UserID, err)
				}
				
				// Cập nhật số dư ví
				_, err = qTx.AddWalletBalance(ctx, AddWalletBalanceParams{
					UserID: p.UserID,
					Amount: auction.DepositAmount,
				})
				if err != nil {
					return fmt.Errorf("failed to update wallet balance for user %s: %w", p.UserID, err)
				}
				// Cập nhật trạng thái đã hoàn tiền
				_, err = qTx.UpdateAuctionParticipant(ctx, UpdateAuctionParticipantParams{
					ID:         p.ID,
					IsRefunded: util.BoolPointer(true),
				})
				if err != nil {
					return fmt.Errorf("failed to update participant refund status: %w", err)
				}
				
				result.RefundedUserIDs = append(result.RefundedUserIDs, p.UserID)
			}
			
			// Xóa task kết thúc phiên đấu giá nếu có
			if arg.OnBuyNowFunc != nil {
				if err := arg.OnBuyNowFunc(arg.Auction.ID); err != nil {
					return fmt.Errorf("failed to delete end auction task: %w", err)
				}
			}
		}
		
		return nil
	})
	
	return result, err
}
