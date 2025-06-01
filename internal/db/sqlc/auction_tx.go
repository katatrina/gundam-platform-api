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
	Participant        User               `json:"participant"`
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
		
		// 2. Tạo bút toán trừ tiền đặt cọc ✅
		depositEntry, err := qTx.CreateWalletEntry(ctx, CreateWalletEntryParams{
			WalletID:      arg.Wallet.UserID,
			ReferenceID:   util.StringPointer(arg.Auction.ID.String()),
			ReferenceType: WalletReferenceTypeAuction,
			EntryType:     WalletEntryTypeAuctionDeposit,
			AffectedField: WalletAffectedFieldBalance,
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
		
		// 4. Tạo bản ghi auction participant - xử lý unique constraint violation
		auctionParticipantID, err := uuid.NewV7()
		if err != nil {
			return fmt.Errorf("failed to generate auctionParticipant ID: %w", err)
		}
		
		auctionParticipant, err := qTx.CreateAuctionParticipant(ctx, CreateAuctionParticipantParams{
			ID:             auctionParticipantID,
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
		result.AuctionParticipant = auctionParticipant
		
		// 5. Cập nhật tổng số người tham gia auction - sử dụng increment atomic
		updatedAuction, err := qTx.IncrementAuctionParticipants(ctx, arg.Auction.ID)
		if err != nil {
			return fmt.Errorf("failed to update auction participants count: %w", err)
		}
		result.Auction = updatedAuction
		
		// 6. Lấy thông tin người tham gia
		participant, err := qTx.GetUserByID(ctx, arg.UserID)
		if err != nil {
			return fmt.Errorf("failed to get participated user info: %w", err)
		}
		result.Participant = participant
		
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
	Winner                *User      `json:"winner,omitempty"`
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
				
				// Hoàn tiền đặt cọc ✅
				_, err = qTx.CreateWalletEntry(ctx, CreateWalletEntryParams{
					WalletID:      p.UserID,
					ReferenceID:   util.StringPointer(auction.ID.String()),
					ReferenceType: WalletReferenceTypeAuction,
					EntryType:     WalletEntryTypeAuctionDepositRefund,
					AffectedField: WalletAffectedFieldBalance,
					Amount:        p.DepositAmount, // Cộng tiền vào số dư ví
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
			
			// Hoàn tiền đặt cọc ✅
			_, err = qTx.CreateWalletEntry(ctx, CreateWalletEntryParams{
				WalletID:      p.UserID,
				ReferenceID:   util.StringPointer(auction.ID.String()),
				ReferenceType: WalletReferenceTypeAuction,
				EntryType:     WalletEntryTypeAuctionDepositRefund,
				AffectedField: WalletAffectedFieldBalance,
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
		
		// 9. Lấy thông tin người thắng cuộc
		winner, err := qTx.GetUserByID(ctx, *winningBid.BidderID)
		if err != nil {
			return fmt.Errorf("failed to get winner info: %w", err)
		}
		result.Winner = &winner
		
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
		
		// 6. Tạo bút toán bồi thường cho người bán (cộng tiền balance) ✅
		_, err = q.CreateWalletEntry(ctx, CreateWalletEntryParams{
			WalletID:      arg.SellerID,
			ReferenceID:   util.StringPointer(auction.ID.String()),
			ReferenceType: WalletReferenceTypeAuction,
			EntryType:     WalletEntryTypeAuctionCompensation,
			AffectedField: WalletAffectedFieldBalance,
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

type PayAuctionWinningBidTxParams struct {
	Auction              Auction            // Thông tin phiên đấu giá cần thanh toán
	User                 User               // ID của người thắng đấu giá
	WinningBid           AuctionBid         // Lượt đấu giá thắng
	Participant          AuctionParticipant // Thông tin lượt tham gia đấu giá (đặt cọc)
	ToAddress            UserAddress        // Địa chỉ nhận hàng của người thắng đấu giá
	DeliveryFee          int64              // Phí vận chuyển
	ExpectedDeliveryTime time.Time          // Thời gian dự kiến giao hàng
	Note                 *string            // Ghi chú của người thắng đấu giá gửi cho người bán
}

type PayAuctionWinningBidTxResult struct {
	Order           Order       `json:"order"`
	Auction         Auction     `json:"auction"`
	WalletEntry     WalletEntry `json:"wallet_entry"`
	RemainingAmount int64       `json:"remaining_amount"`
}

func (store *SQLStore) PayAuctionWinningBidTx(ctx context.Context, arg PayAuctionWinningBidTxParams) (PayAuctionWinningBidTxResult, error) {
	var result PayAuctionWinningBidTxResult
	
	err := store.ExecTx(ctx, func(qTx *Queries) error {
		// 1. Tính số tiền còn lại cần thanh toán (tiền thắng - tiền cọc + phí vận chuyển)
		remainingAmount := arg.WinningBid.Amount - arg.Participant.DepositAmount
		totalPayment := remainingAmount + arg.DeliveryFee
		result.RemainingAmount = remainingAmount
		
		// 2. Kiểm tra số dư ví của người thắng đấu giá
		winnerWallet, err := qTx.GetWalletForUpdate(ctx, arg.User.ID)
		if err != nil {
			return fmt.Errorf("failed to get wallet: %w", err)
		}
		
		if winnerWallet.Balance < totalPayment {
			return fmt.Errorf("insufficient balance: %d, required: %d, %w", winnerWallet.Balance, totalPayment, ErrInsufficientBalance)
		}
		
		// 3. Trừ tiền còn lại từ ví người thắng
		_, err = qTx.AddWalletBalance(ctx, AddWalletBalanceParams{
			UserID: arg.User.ID,
			Amount: -totalPayment,
		})
		if err != nil {
			return fmt.Errorf("failed to deduct balance: %w", err)
		}
		
		// 4. Tạo bút toán trừ tiền số dư cho người thắng đấu giá ✅
		walletEntry, err := qTx.CreateWalletEntry(ctx, CreateWalletEntryParams{
			WalletID:      arg.User.ID,
			ReferenceID:   util.StringPointer(arg.Auction.ID.String()),
			ReferenceType: WalletReferenceTypeAuction,
			EntryType:     WalletEntryTypeAuctionWinnerPayment,
			AffectedField: WalletAffectedFieldBalance,
			Amount:        -totalPayment, // Số âm vì đây là bút toán trừ tiền
			Status:        WalletEntryStatusCompleted,
			CompletedAt:   util.TimePointer(time.Now()),
		})
		if err != nil {
			return fmt.Errorf("failed to create wallet entry: %w", err)
		}
		result.WalletEntry = walletEntry
		
		// 5. Cộng tiền vào non_withdrawable của người bán
		sellerWallet, err := qTx.GetWalletForUpdate(ctx, arg.Auction.SellerID)
		if err != nil {
			return fmt.Errorf("failed to get seller wallet: %w", err)
		}
		
		// Đây là số tiền người bán sẽ nhận được sau khi người thắng đấu giá xác nhận đã nhận hàng thành công.
		err = qTx.AddWalletNonWithdrawableAmount(ctx, AddWalletNonWithdrawableAmountParams{
			UserID: arg.Auction.SellerID,
			Amount: arg.WinningBid.Amount, // Tổng số tiền đấu giá (không bao gồm phí vận chuyển)
		})
		if err != nil {
			return fmt.Errorf("failed to add non-withdrawable amount: %w", err)
		}
		
		// 6. Tạo bút toán cộng tiền vào non_withdrawable cho người bán ✅
		_, err = qTx.CreateWalletEntry(ctx, CreateWalletEntryParams{
			WalletID:      sellerWallet.UserID,
			ReferenceID:   util.StringPointer(arg.Auction.ID.String()),
			ReferenceType: WalletReferenceTypeAuction,
			EntryType:     WalletEntryTypeHoldFunds,
			AffectedField: WalletAffectedFieldNonWithdrawableAmount,
			Amount:        arg.WinningBid.Amount, // Số dương vì đây là bút toán cộng tiền
			Status:        WalletEntryStatusCompleted,
			CompletedAt:   util.TimePointer(time.Now()),
		})
		if err != nil {
			return fmt.Errorf("failed to create non-withdrawable winnerWallet entry: %w", err)
		}
		
		// 7. Tạo bút toán cộng tiền vào số dư cho người bán (trạng thái pending, sẽ hoàn tất khi đơn hàng hoàn thành) ✅
		sellerEntry, err := qTx.CreateWalletEntry(ctx, CreateWalletEntryParams{
			WalletID:      arg.Auction.SellerID,
			ReferenceID:   util.StringPointer(arg.Auction.ID.String()),
			ReferenceType: WalletReferenceTypeAuction,
			EntryType:     WalletEntryTypeAuctionSellerPayment,
			AffectedField: WalletAffectedFieldBalance,
			Amount:        arg.WinningBid.Amount, // Tổng số tiền đấu giá (không bao gồm phí vận chuyển)
			Status:        WalletEntryStatusPending,
		})
		if err != nil {
			return fmt.Errorf("failed to create seller wallet entry: %w", err)
		}
		
		// 8. Tạo địa chỉ giao hàng
		fromDeliveryID, toDeliveryID, err := createDeliveryAddresses(qTx, ctx, arg.User.ID, arg.Auction.SellerID, arg.ToAddress)
		if err != nil {
			return fmt.Errorf("failed to create delivery addresses: %w", err)
		}
		
		// 9. Tạo đơn hàng mới cho người thắng đấu giá
		orderID, err := uuid.NewV7()
		if err != nil {
			return fmt.Errorf("failed to generate order ID: %w", err)
		}
		
		orderCode := util.GenerateOrderCode()
		order, err := qTx.CreateOrder(ctx, CreateOrderParams{
			ID:            orderID,
			Code:          orderCode,
			BuyerID:       arg.User.ID,
			SellerID:      arg.Auction.SellerID,
			ItemsSubtotal: arg.WinningBid.Amount,
			DeliveryFee:   arg.DeliveryFee,
			TotalAmount:   arg.WinningBid.Amount + arg.DeliveryFee,
			Status:        OrderStatusPackaging,
			PaymentMethod: PaymentMethodWallet,
			Type:          OrderTypeAuction,
			Note:          arg.Note,
		})
		if err != nil {
			return fmt.Errorf("failed to create order: %w", err)
		}
		result.Order = order
		
		// 10. Tạo order item từ thông tin Gundam trong auction
		_, err = qTx.CreateOrderItem(ctx, CreateOrderItemParams{
			OrderID:  orderID,
			GundamID: arg.Auction.GundamID,
			Name:     arg.Auction.GundamSnapshot.Name,
			Slug:     arg.Auction.GundamSnapshot.Slug,
			Grade:    arg.Auction.GundamSnapshot.Grade,
			Scale:    arg.Auction.GundamSnapshot.Scale,
			Price:    arg.WinningBid.Amount,
			Quantity: arg.Auction.GundamSnapshot.Quantity,
			Weight:   arg.Auction.GundamSnapshot.Weight,
			ImageURL: arg.Auction.GundamSnapshot.ImageURL,
		})
		if err != nil {
			return fmt.Errorf("failed to create order item: %w", err)
		}
		
		// 11. Tạo thông tin vận chuyển
		// Các cột status, overall_status, delivery_tracking_code sẽ được cập nhật
		// sau khi người bán đóng gói đơn hàng.
		_, err = qTx.CreateOrderDelivery(ctx, CreateOrderDeliveryParams{
			OrderID:              order.ID,
			ExpectedDeliveryTime: arg.ExpectedDeliveryTime,
			FromDeliveryID:       fromDeliveryID,
			ToDeliveryID:         toDeliveryID,
		})
		if err != nil {
			return err
		}
		
		// 12. Tạo order transaction
		_, err = qTx.CreateOrderTransaction(ctx, CreateOrderTransactionParams{
			OrderID:       order.ID,
			Amount:        arg.WinningBid.Amount + arg.DeliveryFee, // Bao gồm cả phí vận chuyển
			Status:        OrderTransactionStatusPending,
			BuyerEntryID:  walletEntry.ID,
			SellerEntryID: &sellerEntry.ID,
		})
		if err != nil {
			return fmt.Errorf("failed to create order transaction: %w", err)
		}
		
		// 13. Cập nhật auction
		updatedAuction, err := qTx.UpdateAuction(ctx, UpdateAuctionParams{
			ID:      arg.Auction.ID,
			OrderID: &orderID,
			Status:  NullAuctionStatus{AuctionStatus: AuctionStatusCompleted, Valid: true},
		})
		if err != nil {
			return fmt.Errorf("failed to update auction: %w", err)
		}
		result.Auction = updatedAuction
		
		// 14. Cập nhật trạng thái của gundam trong auction
		if arg.Auction.GundamID != nil {
			err = qTx.UpdateGundam(ctx, UpdateGundamParams{
				ID: *arg.Auction.GundamID,
				Status: NullGundamStatus{
					GundamStatus: GundamStatusProcessing,
					Valid:        true,
				},
			})
			if err != nil {
				return fmt.Errorf("failed to update gundam status: %w", err)
			}
		}
		
		return nil
	})
	
	return result, err
}

type UpdateAuctionByModeratorTxParams struct {
	AuctionID uuid.UUID
	StartTime *time.Time
	EndTime   *time.Time
}

type UpdateAuctionByModeratorTxResult struct {
	UpdatedAuction Auction `json:"updated_auction"`
}

func (store *SQLStore) UpdateAuctionByModeratorTx(ctx context.Context, arg UpdateAuctionByModeratorTxParams) (UpdateAuctionByModeratorTxResult, error) {
	var result UpdateAuctionByModeratorTxResult
	
	err := store.ExecTx(ctx, func(qTx *Queries) error {
		params := UpdateAuctionParams{
			ID:        arg.AuctionID,
			StartTime: arg.StartTime,
			EndTime:   arg.EndTime,
		}
		
		now := time.Now()
		if arg.StartTime != nil && arg.StartTime.Before(now) {
			params.Status = NullAuctionStatus{
				AuctionStatus: AuctionStatusActive,
				Valid:         true,
			}
		}
		
		updatedAuction, err := qTx.UpdateAuction(ctx, params)
		if err != nil {
			return fmt.Errorf("failed to update auction: %w", err)
		}
		result.UpdatedAuction = updatedAuction
		
		return nil
	})
	
	return result, err
}

type CancelAuctionTxParams struct {
	Auction    Auction // Phiên đấu giá cần hủy
	CanceledBy string  // ID của người hủy (có thể là người bán hoặc admin)
	Reason     *string // Lý do hủy (nếu có)
}

func (store *SQLStore) CancelAuctionTx(ctx context.Context, arg CancelAuctionTxParams) (Auction, error) {
	var updatedAuction Auction
	
	err := store.ExecTx(ctx, func(qTx *Queries) error {
		var err error
		
		// 1. Cập nhật phiên đấu giá
		updatedAuction, err = qTx.UpdateAuction(ctx, UpdateAuctionParams{
			ID: arg.Auction.ID,
			Status: NullAuctionStatus{
				AuctionStatus: AuctionStatusCanceled,
				Valid:         true,
			},
			CanceledBy:     util.StringPointer(arg.CanceledBy),
			CanceledReason: arg.Reason,
		})
		if err != nil {
			return fmt.Errorf("failed to update auction status: %w", err)
		}
		
		// 2. Chuyển gundam về trạng thái "in store"
		if arg.Auction.GundamID != nil {
			err = qTx.UpdateGundam(ctx, UpdateGundamParams{
				ID: *arg.Auction.GundamID,
				Status: NullGundamStatus{
					GundamStatus: GundamStatusInstore,
					Valid:        true,
				},
			})
			if err != nil {
				return fmt.Errorf("failed to update gundam status: %w", err)
			}
		}
		
		// 3. Lấy gói đăng ký hiện tại của người bán
		sellerSubscription, err := qTx.GetCurrentActiveSubscriptionDetailsForSeller(ctx, arg.Auction.SellerID)
		if err != nil {
			return fmt.Errorf("failed to get seller's active subscription: %w", err)
		}
		
		// 3. Hoàn trả lượt đăng đấu giá trong subscription
		_, err = qTx.UpdateCurrentActiveSubscriptionForSeller(ctx, UpdateCurrentActiveSubscriptionForSellerParams{
			OpenAuctionsUsed: util.Int64Pointer(sellerSubscription.OpenAuctionsUsed - 1),
			SubscriptionID:   sellerSubscription.ID,
			SellerID:         arg.Auction.SellerID,
		})
		if err != nil {
			return fmt.Errorf("failed to update seller's subscription: %w", err)
		}
		
		return nil
	})
	
	return updatedAuction, err
}
