package db

import (
	"context"
	"errors"
	"fmt"
	"time"
	
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/katatrina/gundam-BE/internal/util"
)

type ParticipateInAuctionTxParams struct {
	UserID  string
	Auction Auction
	Wallet  Wallet
}

type ParticipateInAuctionTxResult struct {
	AuctionParticipant AuctionParticipant `json:"auction_participant"`
	Auction            Auction            `json:"updated_auction"`
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
		_, err = qTx.AddWalletBalance(ctx, AddWalletBalanceParams{
			UserID: arg.Wallet.UserID,
			Amount: -arg.Auction.DepositAmount, // Trừ tiền đặt cọc
		})
		if err != nil {
			return fmt.Errorf("failed to update wallet balance: %w", err)
		}
		
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
