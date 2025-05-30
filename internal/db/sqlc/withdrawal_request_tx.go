package db

import (
	"context"
	"fmt"
	
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
