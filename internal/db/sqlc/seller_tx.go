package db

import (
	"context"
	
	"github.com/jackc/pgx/v5/pgtype"
)

type PublishGundamTxParams struct {
	GundamID             int64
	SellerID             string
	ActiveSubscriptionID int64
	ListingsUsed         int64
}

func (store *SQLStore) PublishGundamTx(ctx context.Context, arg PublishGundamTxParams) error {
	err := store.ExecTx(ctx, func(qTx *Queries) error {
		// Update gundam status to published
		err := qTx.UpdateGundam(ctx, UpdateGundamParams{
			ID: arg.GundamID,
			Status: NullGundamStatus{
				GundamStatus: GundamStatusPublished,
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

type UnpublishGundamTxParams struct {
	GundamID int64
	SellerID string
}

func (store *SQLStore) UnpublishGundamTx(ctx context.Context, arg UnpublishGundamTxParams) error {
	err := store.ExecTx(ctx, func(qTx *Queries) error {
		// Update gundam status to "in store"
		err := qTx.UpdateGundam(ctx, UpdateGundamParams{
			ID: arg.GundamID,
			Status: NullGundamStatus{
				GundamStatus: GundamStatusInstore,
				Valid:        true,
			},
		})
		if err != nil {
			return err
		}
		
		subscription, err := qTx.GetCurrentActiveSubscriptionDetailsForSeller(ctx, arg.SellerID)
		if err != nil {
			return err
		}
		
		// Minus 1 to the seller's listings used
		err = qTx.UpdateCurrentActiveSubscriptionForSeller(ctx, UpdateCurrentActiveSubscriptionForSellerParams{
			ListingsUsed: pgtype.Int8{
				Int64: subscription.ListingsUsed - 1,
				Valid: true,
			},
			SubscriptionID: subscription.ID,
			SellerID:       arg.SellerID,
		})
		if err != nil {
			return err
		}
		
		return nil
	})
	
	return err
}
