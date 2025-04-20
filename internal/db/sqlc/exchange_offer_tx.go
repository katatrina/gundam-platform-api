package db

import (
	"context"
	
	"github.com/google/uuid"
)

type CreateExchangeOfferTxParams struct {
	PostID             uuid.UUID
	OffererID          string
	PosterGundamID     int64
	OffererGundamID    int64
	PayerID            *string
	CompensationAmount *int64
}

type CreateExchangeOfferTxResult struct {
	Offer     ExchangeOffer
	OfferItem ExchangeOfferItem
}

func (store *SQLStore) CreateExchangeOfferTx(ctx context.Context, arg CreateExchangeOfferTxParams) (CreateExchangeOfferTxResult, error) {
	var result CreateExchangeOfferTxResult
	err := store.ExecTx(ctx, func(qTx *Queries) error {
		var err error
		
		// 1. Tạo đề xuất trao đổi mới
		offerID, _ := uuid.NewV7()
		offer, err := qTx.CreateExchangeOffer(ctx, CreateExchangeOfferParams{
			ID:                 offerID,
			PostID:             arg.PostID,
			OffererID:          arg.OffererID,
			PayerID:            arg.PayerID,
			CompensationAmount: arg.CompensationAmount,
		})
		if err != nil {
			return err
		}
		result.Offer = offer
		
		// 2. Thêm Gundam của người đề xuất vào đề xuất
		offerItemID, _ := uuid.NewV7()
		offerItem, err := qTx.CreateExchangeOfferItem(ctx, CreateExchangeOfferItemParams{
			ID:       offerItemID,
			OfferID:  offerID,
			GundamID: arg.OffererGundamID,
		})
		if err != nil {
			return err
		}
		result.OfferItem = offerItem
		
		// 3. Cập nhật trạng thái Gundam của người đề xuất thành "for exchange"
		err = qTx.UpdateGundam(ctx, UpdateGundamParams{
			ID: arg.OffererGundamID,
			Status: NullGundamStatus{
				GundamStatus: GundamStatusForexchange,
				Valid:        true,
			},
		})
		if err != nil {
			return err
		}
		
		return nil
	})
	return result, err
}
