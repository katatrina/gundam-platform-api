package db

import (
	"context"
	"fmt"
	"strings"
	
	"github.com/google/uuid"
)

type CreateExchangeOfferTxParams struct {
	PostID             uuid.UUID // ID bài đăng trao đổi
	OffererID          string    // ID người đề xuất
	PosterGundamID     int64     // ID Gundam của người đăng bài
	OffererGundamID    int64     // ID Gundam của người đề xuất
	PayerID            *string   // ID người bù tiền (có thể là người đề xuất hoặc người đăng bài, nếu không có thì là nil)
	CompensationAmount *int64    // Số tiền bồi thường (có thể là nil nếu không có bù tiền)
}

type CreateExchangeOfferTxResult struct {
	Offer      ExchangeOffer       `json:"offer"`
	OfferItems []ExchangeOfferItem `json:"offer_items"`
}

func (store *SQLStore) CreateExchangeOfferTx(ctx context.Context, arg CreateExchangeOfferTxParams) (CreateExchangeOfferTxResult, error) {
	var result CreateExchangeOfferTxResult
	err := store.ExecTx(ctx, func(qTx *Queries) error {
		// 1. Tạo đề xuất trao đổi mới
		offerID, err := uuid.NewV7()
		if err != nil {
			return fmt.Errorf("failed to generate offer ID: %w", err)
		}
		
		offer, err := qTx.CreateExchangeOffer(ctx, CreateExchangeOfferParams{
			ID:                   offerID,
			PostID:               arg.PostID,
			OffererID:            arg.OffererID,
			PayerID:              arg.PayerID,
			CompensationAmount:   arg.CompensationAmount,
			NegotiationsCount:    0,
			MaxNegotiations:      3,
			NegotiationRequested: false,
		})
		if err != nil {
			if pgErr := ErrorDescription(err); pgErr != nil {
				if pgErr.Code == UniqueViolationCode && strings.Contains(pgErr.Detail, "post_id") && strings.Contains(pgErr.Detail, "offerer_id") {
					return ErrExchangeOfferUnique
				}
			}
			
			return fmt.Errorf("failed to create exchange offer: %w", err)
		}
		result.Offer = offer
		
		// 2. Thêm Gundam của người đề xuất vào đề xuất
		offererItemID, err := uuid.NewV7()
		if err != nil {
			return fmt.Errorf("failed to generate offerer item ID: %w", err)
		}
		
		offererItem, err := qTx.CreateExchangeOfferItem(ctx, CreateExchangeOfferItemParams{
			ID:           offererItemID,
			OfferID:      offerID,
			GundamID:     arg.OffererGundamID,
			IsFromPoster: false,
		})
		if err != nil {
			return fmt.Errorf("failed to create offerer exchange item: %w", err)
		}
		
		// 3. Thêm Gundam của người đăng bài (mà người đề xuất muốn trao đổi) vào đề xuất
		posterItemID, err := uuid.NewV7()
		if err != nil {
			return fmt.Errorf("failed to generate poster item ID: %w", err)
		}
		
		posterItem, err := qTx.CreateExchangeOfferItem(ctx, CreateExchangeOfferItemParams{
			ID:           posterItemID,
			OfferID:      offerID,
			GundamID:     arg.PosterGundamID,
			IsFromPoster: true,
		})
		if err != nil {
			return fmt.Errorf("failed to create poster exchange item: %w", err)
		}
		
		// Thêm cả hai item vào kết quả
		result.OfferItems = []ExchangeOfferItem{offererItem, posterItem}
		
		// 3. Cập nhật trạng thái Gundam của người đề xuất thành "for exchange"
		err = qTx.UpdateGundam(ctx, UpdateGundamParams{
			ID: arg.OffererGundamID,
			Status: NullGundamStatus{
				GundamStatus: GundamStatusForexchange,
				Valid:        true,
			},
		})
		if err != nil {
			return fmt.Errorf("failed to update gundam status to \"for exchange\": %w", err)
		}
		
		// Việc trừ tiền bù sẽ được thực hiện khi đề xuất được chấp nhận, không trừ ngay tại đây.
		
		// TODO: Có thể thực hiện việc trừ tiền bù nếu người đề xuất là người bù tiền ngay tại đây nếu có thay đổi trong tương lai.
		
		return nil
	})
	
	return result, err
}
