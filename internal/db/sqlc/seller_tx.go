package db

import (
	"context"
	
	"github.com/jackc/pgx/v5/pgtype"
)

type SellGundamTxParams struct {
	GundamID             int64
	SellerID             string
	ActiveSubscriptionID int64
	ListingsUsed         int64
}

func (store *SQLStore) SellGundamTx(ctx context.Context, arg SellGundamTxParams) error {
	err := store.ExecTx(ctx, func(qTx *Queries) error {
		// Update gundam status to selling
		err := qTx.UpdateGundam(ctx, UpdateGundamParams{
			ID: arg.GundamID,
			Status: NullGundamStatus{
				GundamStatus: GundamStatusSelling,
				Valid:        true,
			},
		})
		if err != nil {
			return err
		}
		
		// Plus 1 to the seller's listings used
		err = qTx.UpdateCurrentActiveSubscriptionForSeller(ctx, UpdateCurrentActiveSubscriptionForSellerParams{
			ListingsUsed: pgtype.Int8{
				Int64: arg.ListingsUsed,
				Valid: true,
			},
			SubscriptionID: arg.ActiveSubscriptionID,
			SellerID:       arg.SellerID,
		})
		if err != nil {
			return err
		}
		
		return nil
	})
	
	return err
}
