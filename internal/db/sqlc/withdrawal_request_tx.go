package db

import (
	"context"
	"fmt"
	"time"
	
	"github.com/google/uuid"
	"github.com/katatrina/gundam-BE/internal/util"
)

type CreateWithdrawalRequestTxParams struct {
	UserID        string
	BankAccountID uuid.UUID
	Amount        int64
}

func (store *SQLStore) CreateWithdrawalRequestTx(ctx context.Context, arg CreateWithdrawalRequestTxParams) (WithdrawalRequest, error) {
	var request WithdrawalRequest
	
	err := store.ExecTx(ctx, func(qTx *Queries) error {
		// 1. Create wallet entry to deduct money
		deductEntry, err := qTx.CreateWalletEntry(ctx, CreateWalletEntryParams{
			WalletID:      arg.UserID,
			ReferenceID:   nil, // Will update after creating withdrawal request
			ReferenceType: WalletReferenceTypeWithdrawalRequest,
			EntryType:     WalletEntryTypeWithdrawal,
			AffectedField: WalletAffectedFieldBalance,
			Amount:        -arg.Amount, // Negative = deduction
			Status:        WalletEntryStatusPending,
		})
		if err != nil {
			return fmt.Errorf("failed to create wallet entry: %w", err)
		}
		
		// 2. Deduct the amount from the user's wallet
		_, err = qTx.AddWalletBalance(ctx, AddWalletBalanceParams{
			Amount: deductEntry.Amount,
			UserID: arg.UserID,
		})
		
		// 3. Create the withdrawal request
		requestID, err := uuid.NewV7()
		if err != nil {
			return fmt.Errorf("failed to generate request ID: %w", err)
		}
		
		request, err = qTx.CreateWithdrawalRequest(ctx, CreateWithdrawalRequestParams{
			ID:            requestID,
			UserID:        arg.UserID,
			BankAccountID: arg.BankAccountID,
			Amount:        arg.Amount,
			WalletEntryID: &deductEntry.ID,
		})
		if err != nil {
			return fmt.Errorf("failed to create withdrawal request: %w", err)
		}
		
		// 4: Cập nhật reference_id trong wallet_entry
		_, err = qTx.UpdateWalletEntryByID(ctx, UpdateWalletEntryByIDParams{
			ID:          deductEntry.ID,
			ReferenceID: util.StringPointer(request.ID.String()),
		})
		if err != nil {
			return fmt.Errorf("failed to update wallet entry reference: %w", err)
		}
		
		return nil
	})
	
	return request, err
}

type CancelWithdrawalRequestTxParams struct {
	WithdrawalRequest WithdrawalRequest // Yêu cầu rút tiền cần hủy
}

func (store *SQLStore) CancelWithdrawalRequestTx(ctx context.Context, arg CancelWithdrawalRequestTxParams) (WithdrawalRequest, error) {
	var updatedRequest WithdrawalRequest
	
	err := store.ExecTx(ctx, func(qTx *Queries) error {
		var err error
		// 1. Update the withdrawal request
		updatedRequest, err = qTx.UpdateWithdrawalRequest(ctx, UpdateWithdrawalRequestParams{
			ID: arg.WithdrawalRequest.ID,
			Status: NullWithdrawalRequestStatus{
				WithdrawalRequestStatus: WithdrawalRequestStatusCanceled,
				Valid:                   true,
			},
		})
		if err != nil {
			return fmt.Errorf("failed to update withdrawal request: %w", err)
		}
		
		// 2. Update the wallet entry
		if updatedRequest.WalletEntryID != nil {
			_, err = qTx.UpdateWalletEntryByID(ctx, UpdateWalletEntryByIDParams{
				Status: NullWalletEntryStatus{
					WalletEntryStatus: WalletEntryStatusCanceled,
					Valid:             true,
				},
				ID: *updatedRequest.WalletEntryID,
			})
			if err != nil {
				return fmt.Errorf("failed to update wallet entry: %w", err)
			}
		} else {
			return fmt.Errorf("withdrawal request %s does not have a wallet entry", arg.WithdrawalRequest.ID)
		}
		
		// 3. Refund the amount back to the user's wallet
		_, err = qTx.AddWalletBalance(ctx, AddWalletBalanceParams{
			Amount: arg.WithdrawalRequest.Amount,
			UserID: arg.WithdrawalRequest.UserID,
		})
		if err != nil {
			return fmt.Errorf("failed to refund amount to wallet: %w", err)
		}
		
		// 4. Create a new wallet entry for the refund
		_, err = qTx.CreateWalletEntry(ctx, CreateWalletEntryParams{
			WalletID:      arg.WithdrawalRequest.UserID,
			ReferenceID:   util.StringPointer(arg.WithdrawalRequest.ID.String()),
			ReferenceType: WalletReferenceTypeWithdrawalRequest,
			EntryType:     WalletEntryTypeRefund,
			AffectedField: WalletAffectedFieldBalance,
			Amount:        arg.WithdrawalRequest.Amount, // Positive = refund
			Status:        WalletEntryStatusCompleted,
			CompletedAt:   util.TimePointer(time.Now()),
		})
		if err != nil {
			return fmt.Errorf("failed to create wallet entry for refund: %w", err)
		}
		
		return nil
	})
	
	return updatedRequest, err
}

type CompleteWithdrawalRequestTxParams struct {
	WithdrawalRequest    WithdrawalRequest // Yêu cầu rút tiền cần hoàn thành
	ModeratorID          string            // ID của moderator hoàn tất yêu cầu
	TransactionReference string            // Mã tham chiếu giao dịch từ ngân hàng
}

func (store *SQLStore) CompleteWithdrawalRequestTx(ctx context.Context, arg CompleteWithdrawalRequestTxParams) (WithdrawalRequest, error) {
	var updatedRequest WithdrawalRequest
	
	err := store.ExecTx(ctx, func(qTx *Queries) error {
		var err error
		// 1. Update the withdrawal updatedRequest
		updatedRequest, err = qTx.UpdateWithdrawalRequest(ctx, UpdateWithdrawalRequestParams{
			ID: arg.WithdrawalRequest.ID,
			Status: NullWithdrawalRequestStatus{
				WithdrawalRequestStatus: WithdrawalRequestStatusCompleted,
				Valid:                   true,
			},
			ProcessedBy:          &arg.ModeratorID,
			ProcessedAt:          util.TimePointer(time.Now()),
			TransactionReference: &arg.TransactionReference,
			CompletedAt:          util.TimePointer(time.Now()),
		})
		if err != nil {
			return fmt.Errorf("failed to update withdrawal updatedRequest: %w", err)
		}
		
		// 2. Update the wallet entry
		if arg.WithdrawalRequest.WalletEntryID != nil {
			_, err = qTx.UpdateWalletEntryByID(ctx, UpdateWalletEntryByIDParams{
				Status: NullWalletEntryStatus{
					WalletEntryStatus: WalletEntryStatusCompleted,
					Valid:             true,
				},
				CompletedAt: util.TimePointer(time.Now()),
				ID:          *arg.WithdrawalRequest.WalletEntryID,
			})
			if err != nil {
				return fmt.Errorf("failed to update wallet entry: %w", err)
			}
		} else {
			return fmt.Errorf("withdrawal updatedRequest %s does not have a wallet entry", arg.WithdrawalRequest.ID)
		}
		
		return nil
	})
	
	return updatedRequest, err
}

type RejectWithdrawalRequestTxParams struct {
	WithdrawalRequest WithdrawalRequest // Yêu cầu rút tiền bị từ chối
	ModeratorID       string            // ID của moderator từ chối yêu cầu
	Reason            string            // Lý do từ chối
}

func (store *SQLStore) RejectWithdrawalRequestTx(ctx context.Context, arg RejectWithdrawalRequestTxParams) (WithdrawalRequest, error) {
	var updatedRequest WithdrawalRequest
	
	err := store.ExecTx(ctx, func(qTx *Queries) error {
		var err error
		// 1. Update the withdrawal request
		updatedRequest, err = qTx.UpdateWithdrawalRequest(ctx, UpdateWithdrawalRequestParams{
			ID: arg.WithdrawalRequest.ID,
			Status: NullWithdrawalRequestStatus{
				WithdrawalRequestStatus: WithdrawalRequestStatusRejected,
				Valid:                   true,
			},
			RejectedReason: &arg.Reason,
			ProcessedBy:    &arg.ModeratorID,
			ProcessedAt:    util.TimePointer(time.Now()),
		})
		if err != nil {
			return fmt.Errorf("failed to update withdrawal request: %w", err)
		}
		
		// 2. Update the wallet entry
		if arg.WithdrawalRequest.WalletEntryID != nil {
			_, err = qTx.UpdateWalletEntryByID(ctx, UpdateWalletEntryByIDParams{
				Status: NullWalletEntryStatus{
					WalletEntryStatus: WalletEntryStatusFailed,
					Valid:             true,
				},
				ID: *arg.WithdrawalRequest.WalletEntryID,
			})
			if err != nil {
				return fmt.Errorf("failed to update wallet entry: %w", err)
			}
		} else {
			return fmt.Errorf("withdrawal updatedRequest %s does not have a wallet entry", arg.WithdrawalRequest.ID)
		}
		
		// 3. Refund the amount back to the user's wallet
		_, err = qTx.AddWalletBalance(ctx, AddWalletBalanceParams{
			Amount: arg.WithdrawalRequest.Amount,
			UserID: arg.WithdrawalRequest.UserID,
		})
		if err != nil {
			return fmt.Errorf("failed to refund amount to wallet: %w", err)
		}
		
		// 4. Create a new wallet entry for the refund
		_, err = qTx.CreateWalletEntry(ctx, CreateWalletEntryParams{
			WalletID:      arg.WithdrawalRequest.UserID,
			ReferenceID:   util.StringPointer(arg.WithdrawalRequest.ID.String()),
			ReferenceType: WalletReferenceTypeWithdrawalRequest,
			EntryType:     WalletEntryTypeRefund,
			AffectedField: WalletAffectedFieldBalance,
			Amount:        arg.WithdrawalRequest.Amount, // Positive = refund
			Status:        WalletEntryStatusCompleted,
			CompletedAt:   util.TimePointer(time.Now()),
		})
		if err != nil {
			return fmt.Errorf("failed to create wallet entry for refund: %w", err)
		}
		
		return nil
	})
	
	return updatedRequest, err
}
