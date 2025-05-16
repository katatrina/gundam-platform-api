package db

import (
	"context"
	"errors"
	"fmt"
	"time"
	
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/katatrina/gundam-BE/internal/util"
	"github.com/shopspring/decimal"
)

type ParticipateInAuctionTxParams struct {
	UserID  string
	Auction Auction
	Wallet  Wallet
}

type ParticipateInAuctionTxResult struct {
	AuctionParticipant AuctionParticipant `json:"auction_participant"`
	Auction            Auction            `json:"updated_auction"`
	Wallet             Wallet             `json:"updated_wallet"`
}

func (store *SQLStore) ParticipateInAuctionTx(ctx context.Context, arg ParticipateInAuctionTxParams) (ParticipateInAuctionTxResult, error) {
	var result ParticipateInAuctionTxResult
	
	err := store.ExecTx(ctx, func(qTx *Queries) error {
		// 1. Xác nhận lại số dư trong transaction để tránh race condition
		wallet, err := qTx.GetWalletForUpdate(ctx, arg.UserID)
		if err != nil {
			return fmt.Errorf("failed to get wallet for update: %w", err)
		}
		
		if wallet.Balance < arg.Auction.DepositAmount {
			return ErrInsufficientBalance
		}
		
		// 2. Tạo bút toán trừ tiền đặt cọc
		depositEntry, err := qTx.CreateWalletEntry(ctx, CreateWalletEntryParams{
			WalletID:      arg.Wallet.UserID,
			ReferenceID:   util.StringPointer(arg.Auction.ID.String()),
			ReferenceType: WalletReferenceTypeAuction,
			EntryType:     WalletEntryTypeAuctionDeposit,
			Amount:        -arg.Auction.DepositAmount, // Tiền đặt cọc, âm để trừ
			Status:        WalletEntryStatusCompleted,
			CompletedAt:   util.TimePointer(time.Now()),
		})
		if err != nil {
			return fmt.Errorf("failed to create deposit entry: %w", err)
		}
		
		// 3. Cập nhật số dư ví
		updatedWallet, err := qTx.AddWalletBalance(ctx, AddWalletBalanceParams{
			UserID: arg.Wallet.UserID,
			Amount: -arg.Auction.DepositAmount, // Trừ tiền đặt cọc
		})
		if err != nil {
			return fmt.Errorf("failed to update wallet balance: %w", err)
		}
		result.Wallet = updatedWallet
		
		// 4. Tạo bản ghi participant - xử lý unique constraint violation
		participantID, err := uuid.NewV7()
		if err != nil {
			return fmt.Errorf("failed to generate participant ID: %w", err)
		}
		
		participant, err := qTx.CreateAuctionParticipant(ctx, CreateAuctionParticipantParams{
			ID:             participantID,
			AuctionID:      arg.Auction.ID,
			UserID:         arg.UserID,
			DepositAmount:  arg.Auction.DepositAmount,
			DepositEntryID: depositEntry.ID,
		})
		if err != nil {
			// Xử lý lỗi unique constraint violation
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == UniqueViolationCode {
				return ErrDuplicateParticipation
			}
			return fmt.Errorf("failed to create auction participant: %w", err)
		}
		result.AuctionParticipant = participant
		
		// 5. Cập nhật tổng số người tham gia auction - sử dụng increment atomic
		updatedAuction, err := qTx.IncrementAuctionParticipants(ctx, arg.Auction.ID)
		if err != nil {
			return fmt.Errorf("failed to update auction participants count: %w", err)
		}
		result.Auction = updatedAuction
		
		return nil
	})
	
	return result, err
}

type EndAuctionTxParams struct {
	AuctionID     uuid.UUID
	ActualEndTime time.Time
}

type EndAuctionTxResult struct {
	HasWinner             bool       `json:"has_winner"`
	WinnerID              *string    `json:"winner_id,omitempty"`
	FinalPrice            int64      `json:"final_price"`
	RefundedUserIDs       []string   `json:"refunded_user_ids"`
	WinnerPaymentDeadline *time.Time `json:"winner_payment_deadline,omitempty"`
}

func (store *SQLStore) EndAuctionTx(ctx context.Context, arg EndAuctionTxParams) (EndAuctionTxResult, error) {
	var result EndAuctionTxResult
	
	err := store.ExecTx(ctx, func(qTx *Queries) error {
		var err error
		
		// 1. Lấy thông tin auction với lock để tránh race condition
		auction, err := qTx.GetAuctionByIDForUpdate(ctx, arg.AuctionID)
		if err != nil {
			return fmt.Errorf("failed to get auction: %w", err)
		}
		
		// 2. Kiểm tra trạng thái hiện tại
		if auction.Status != AuctionStatusActive {
			err = fmt.Errorf("auction ID %s status is not active, current status: %s", auction.ID, auction.Status)
			return err
		}
		
		// 3. Kiểm tra xem đã có ActualEndTime (mua ngay đã xảy ra)
		if auction.ActualEndTime != nil {
			err = fmt.Errorf("auction ID %s already has actual end time at %v", auction.ID, *auction.ActualEndTime)
			return err
		}
		
		// 4. Cập nhật trạng thái của auction sang ended và lưu thời gian kết thúc thực tế
		updatedAuction, err := qTx.UpdateAuction(ctx, UpdateAuctionParams{
			ID:            arg.AuctionID,
			Status:        NullAuctionStatus{AuctionStatus: AuctionStatusEnded, Valid: true},
			ActualEndTime: util.TimePointer(arg.ActualEndTime),
		})
		if err != nil {
			return fmt.Errorf("failed to update auction status: %w", err)
		}
		
		// 5. Kiểm tra xem có người thắng cuộc hay không (có winning_bid_id)
		if updatedAuction.WinningBidID == nil {
			// Không có người thắng cuộc, đấu giá thất bại
			failedAuction, err := qTx.UpdateAuction(ctx, UpdateAuctionParams{
				ID:     arg.AuctionID,
				Status: NullAuctionStatus{AuctionStatus: AuctionStatusFailed, Valid: true},
			})
			if err != nil {
				return fmt.Errorf("failed to update auction to failed status: %w", err)
			}
			
			// Nếu có gundam, chuyển về in store
			if failedAuction.GundamID != nil {
				err = qTx.UpdateGundam(ctx, UpdateGundamParams{
					ID: *failedAuction.GundamID,
					Status: NullGundamStatus{
						GundamStatus: GundamStatusInstore,
						Valid:        true,
					},
				})
				if err != nil {
					return fmt.Errorf("failed to update gundam status: %w", err)
				}
			}
			
			result.HasWinner = false
			result.FinalPrice = auction.CurrentPrice // Giá cuối cùng vẫn là giá hiện tại
			
			// Hoàn tiền đặt cọc cho tất cả người tham gia
			participants, err := qTx.ListAuctionParticipants(ctx, arg.AuctionID)
			if err != nil {
				return fmt.Errorf("failed to list auction participants: %w", err)
			}
			
			result.RefundedUserIDs = make([]string, 0, len(participants))
			for _, p := range participants {
				// Skip nếu đã hoàn tiền rồi
				if p.IsRefunded {
					continue
				}
				
				// Hoàn tiền đặt cọc
				_, err = qTx.CreateWalletEntry(ctx, CreateWalletEntryParams{
					WalletID:      p.UserID,
					ReferenceID:   util.StringPointer(auction.ID.String()),
					ReferenceType: WalletReferenceTypeAuction,
					EntryType:     WalletEntryTypeAuctionDepositRefund,
					Amount:        p.DepositAmount, // Số tiền dương để cộng
					Status:        WalletEntryStatusCompleted,
					CompletedAt:   util.TimePointer(time.Now()),
				})
				if err != nil {
					return fmt.Errorf("failed to create refund entry for user %s: %w", p.UserID, err)
				}
				
				// Cập nhật số dư ví
				_, err = qTx.AddWalletBalance(ctx, AddWalletBalanceParams{
					UserID: p.UserID,
					Amount: p.DepositAmount,
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
			
			return nil
		}
		
		// 6. Có người thắng cuộc
		winningBid, err := qTx.GetAuctionBidByID(ctx, *updatedAuction.WinningBidID)
		if err != nil {
			return fmt.Errorf("failed to get winning bid: %w", err)
		}
		
		if winningBid.BidderID == nil {
			return fmt.Errorf("winning bid doesn't have a bidder ID")
		}
		
		result.HasWinner = true
		result.WinnerID = winningBid.BidderID
		result.FinalPrice = winningBid.Amount
		
		// 7. Thiết lập thời hạn thanh toán (48 giờ từ thời điểm kết thúc thực tế)
		paymentDeadline := arg.ActualEndTime.Add(48 * time.Hour)
		_, err = qTx.UpdateAuction(ctx, UpdateAuctionParams{
			ID:                    arg.AuctionID,
			WinnerPaymentDeadline: util.TimePointer(paymentDeadline),
		})
		if err != nil {
			return fmt.Errorf("failed to set winner payment deadline: %w", err)
		}
		result.WinnerPaymentDeadline = util.TimePointer(paymentDeadline)
		
		// 8. Hoàn tiền đặt cọc cho tất cả người tham gia khác
		participants, err := qTx.ListAuctionParticipantsExcept(ctx, ListAuctionParticipantsExceptParams{
			AuctionID: arg.AuctionID,
			UserID:    *winningBid.BidderID,
		})
		if err != nil {
			return fmt.Errorf("failed to list auction participants: %w", err)
		}
		
		result.RefundedUserIDs = make([]string, 0, len(participants))
		for _, p := range participants {
			// Skip nếu đã hoàn tiền rồi
			if p.IsRefunded {
				continue
			}
			
			// Hoàn tiền đặt cọc
			_, err = qTx.CreateWalletEntry(ctx, CreateWalletEntryParams{
				WalletID:      p.UserID,
				ReferenceID:   util.StringPointer(auction.ID.String()),
				ReferenceType: WalletReferenceTypeAuction,
				EntryType:     WalletEntryTypeAuctionDepositRefund,
				Amount:        p.DepositAmount, // Số tiền dương để cộng
				Status:        WalletEntryStatusCompleted,
				CompletedAt:   util.TimePointer(time.Now()),
			})
			if err != nil {
				return fmt.Errorf("failed to create refund entry for user %s: %w", p.UserID, err)
			}
			
			// Cập nhật số dư ví
			_, err = qTx.AddWalletBalance(ctx, AddWalletBalanceParams{
				UserID: p.UserID,
				Amount: p.DepositAmount,
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
		
		return nil
	})
	
	if err != nil {
		return EndAuctionTxResult{}, fmt.Errorf("EndAuctionTx failed: %w", err)
	}
	
	return result, nil
}

type HandleAuctionNonPaymentTxParams struct {
	AuctionID uuid.UUID
	WinnerID  string
	SellerID  string
}

type HandleAuctionNonPaymentTxResult struct {
	CompensationAmount int64  `json:"compensation_amount"`
	GundamName         string `json:"gundam_name"`
}

func (store *SQLStore) HandleAuctionNonPaymentTx(ctx context.Context, arg HandleAuctionNonPaymentTxParams) (HandleAuctionNonPaymentTxResult, error) {
	var result HandleAuctionNonPaymentTxResult
	
	err := store.ExecTx(ctx, func(q *Queries) error {
		var err error
		
		// 1. Lấy thông tin auction
		auction, err := q.GetAuctionByIDForUpdate(ctx, arg.AuctionID)
		if err != nil {
			return fmt.Errorf("failed to get auction: %w", err)
		}
		
		result.GundamName = auction.GundamSnapshot.Name
		
		// 2. Kiểm tra lại trạng thái auction
		if auction.Status != AuctionStatusEnded {
			err = fmt.Errorf("auction ID %s is not in ended status, current status: %s", auction.ID, auction.Status)
			return err
		}
		
		// 3. Kiểm tra xem đã có order chưa
		if auction.OrderID != nil {
			err = fmt.Errorf("auction ID %s already has an order, cannot handle non-payment", auction.ID)
			return err
		}
		
		// 4. Tìm participant record của người thắng
		participant, err := q.GetAuctionParticipantByUserID(ctx, GetAuctionParticipantByUserIDParams{
			AuctionID: auction.ID,
			UserID:    arg.WinnerID,
		})
		if err != nil {
			return fmt.Errorf("failed to get winner participant record: %w", err)
		}
		
		// 5. Tính toán tiền bồi thường (70% tiền đặt cọc)
		depositAmount := participant.DepositAmount
		
		// Sử dụng decimal để tính toán chính xác
		compensationRate := decimal.NewFromFloat(0.7)
		compensationDecimal := decimal.NewFromInt(depositAmount).Mul(compensationRate).Round(0)
		compensationAmount := compensationDecimal.IntPart()
		
		// Gán giá trị cho result để có thể sử dụng sau transaction
		result.CompensationAmount = compensationAmount
		
		// 6. Tạo bút toán bồi thường cho người bán (cộng tiền)
		_, err = q.CreateWalletEntry(ctx, CreateWalletEntryParams{
			WalletID:      arg.SellerID,
			ReferenceID:   util.StringPointer(auction.ID.String()),
			ReferenceType: WalletReferenceTypeAuction,
			EntryType:     WalletEntryTypeAuctionCompensation,
			Amount:        compensationAmount, // Số tiền dương để cộng
			Status:        WalletEntryStatusCompleted,
			CompletedAt:   util.TimePointer(time.Now()),
		})
		if err != nil {
			return fmt.Errorf("failed to create compensation entry: %w", err)
		}
		
		// 7. Cập nhật số dư ví người bán
		_, err = q.AddWalletBalance(ctx, AddWalletBalanceParams{
			UserID: arg.SellerID,
			Amount: compensationAmount,
		})
		if err != nil {
			return fmt.Errorf("failed to update seller balance: %w", err)
		}
		
		// 8. Còn lại 30% tiền đặt cọc là phí của nền tảng, nên không cần thao tác gì thêm
		
		// 9. Cập nhật trạng thái auction thành failed
		_, err = q.UpdateAuction(ctx, UpdateAuctionParams{
			ID: auction.ID,
			Status: NullAuctionStatus{
				AuctionStatus: AuctionStatusFailed,
				Valid:         true,
			},
		})
		if err != nil {
			return fmt.Errorf("failed to update auction status: %w", err)
		}
		
		// 10. Cập nhật status của Gundam về "in store"
		if auction.GundamID != nil {
			err = q.UpdateGundam(ctx, UpdateGundamParams{
				ID: *auction.GundamID,
				Status: NullGundamStatus{
					GundamStatus: GundamStatusInstore,
					Valid:        true,
				},
			})
			if err != nil {
				return fmt.Errorf("failed to update gundam status: %w", err)
			}
		}
		
		return nil
	})
	
	if err != nil {
		return HandleAuctionNonPaymentTxResult{}, fmt.Errorf("HandleAuctionNonPaymentTx failed: %w", err)
	}
	
	return result, nil
}
