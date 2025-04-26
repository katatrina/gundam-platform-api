package db

import (
	"context"
	"fmt"
	"strings"
	"time"
	
	"github.com/google/uuid"
	"github.com/katatrina/gundam-BE/internal/util"
)

type CreateExchangeOfferTxParams struct {
	PostID             uuid.UUID // OfferID bài đăng trao đổi
	OffererID          string    // OfferID người đề xuất
	PosterGundamID     int64     // OfferID Gundam của người đăng bài
	OffererGundamID    int64     // OfferID Gundam của người đề xuất
	PayerID            *string   // OfferID người bù tiền (có thể là người đề xuất hoặc người đăng bài, nếu không có thì là nil)
	CompensationAmount *int64    // Số tiền bồi thường (có thể là nil nếu không có bù tiền)
	Note               *string   // Ghi chú đề xuất
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
			return fmt.Errorf("failed to generate offer OfferID: %w", err)
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
			Note:                 arg.Note,
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
			return fmt.Errorf("failed to generate offerer item OfferID: %w", err)
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
			return fmt.Errorf("failed to generate poster item OfferID: %w", err)
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

type RequestNegotiationForOfferTxParams struct {
	OfferID uuid.UUID // OfferID đề xuất trao đổi
	UserID  string    // OfferID người yêu cầu thương lượng
	Note    *string   // Ghi chú yêu cầu thương lượng
}

type RequestNegotiationForOfferTxResult struct {
	Offer ExchangeOffer      `json:"offer"`
	Note  *ExchangeOfferNote `json:"note"` // Có thể là nil nếu không có ghi chú
}

func (store *SQLStore) RequestNegotiationForOfferTx(ctx context.Context, arg RequestNegotiationForOfferTxParams) (RequestNegotiationForOfferTxResult, error) {
	var result RequestNegotiationForOfferTxResult
	
	err := store.ExecTx(ctx, func(qTx *Queries) error {
		// 1. Cập nhật trạng thái đề xuất
		updateOfferParams := UpdateExchangeOfferParams{
			NegotiationRequested: util.BoolPointer(true),       // Đánh dấu đã yêu cầu thương lượng
			LastNegotiationAt:    util.TimePointer(time.Now()), // Thời gian gần nhất yêu cầu thương lượng
			ID:                   arg.OfferID,
		}
		
		updatedOffer, err := qTx.UpdateExchangeOffer(ctx, updateOfferParams)
		if err != nil {
			return err
		}
		
		result.Offer = updatedOffer
		
		// 2. Tạo ghi chú thương lượng nếu có
		if arg.Note != nil {
			noteID, _ := uuid.NewV7()
			note, err := qTx.CreateExchangeOfferNote(ctx, CreateExchangeOfferNoteParams{
				ID:      noteID,
				OfferID: arg.OfferID,
				UserID:  arg.UserID,
				Content: *arg.Note,
			})
			if err != nil {
				return err
			}
			
			result.Note = &note
		}
		
		return nil
	})
	
	return result, err
}

type UpdateExchangeOfferTxParams struct {
	OfferID              uuid.UUID
	UserID               string
	CompensationAmount   *int64
	PayerID              *string
	Note                 *string
	NegotiationRequested *bool
	NegotiationsCount    *int64
}

type UpdateExchangeOfferTxResult struct {
	Offer ExchangeOffer      `json:"offer"`
	Note  *ExchangeOfferNote `json:"note"` // Có thể là nil nếu không có ghi chú
}

func (store *SQLStore) UpdateExchangeOfferTx(ctx context.Context, arg UpdateExchangeOfferTxParams) (UpdateExchangeOfferTxResult, error) {
	var result UpdateExchangeOfferTxResult
	
	err := store.ExecTx(ctx, func(qTx *Queries) error {
		// 1. Cập nhật thông tin đề xuất
		updatedOffer, err := qTx.UpdateExchangeOffer(ctx, UpdateExchangeOfferParams{
			CompensationAmount:   arg.CompensationAmount,
			PayerID:              arg.PayerID,
			NegotiationRequested: arg.NegotiationRequested,
			NegotiationsCount:    arg.NegotiationsCount,
			LastNegotiationAt:    util.TimePointer(time.Now()),
			ID:                   arg.OfferID,
		})
		if err != nil {
			return err
		}
		
		result.Offer = updatedOffer
		
		// 2. Tạo ghi chú thương lượng nếu có
		if arg.Note != nil {
			noteID, _ := uuid.NewV7()
			note, err := qTx.CreateExchangeOfferNote(ctx, CreateExchangeOfferNoteParams{
				ID:      noteID,
				OfferID: arg.OfferID,
				UserID:  arg.UserID,
				Content: *arg.Note,
			})
			if err != nil {
				return err
			}
			
			result.Note = &note
		}
		
		return nil
	})
	
	return result, err
}

type AcceptExchangeOfferTxParams struct {
	PostID             uuid.UUID // ID của bài đăng trao đổi
	OfferID            uuid.UUID // ID của đề xuất trao đổi
	PosterID           string    // ID của người đăng bài
	OffererID          string    // ID của người đề xuất
	PayerID            *string   // ID của người bù tiền (có thể là nil)
	CompensationAmount *int64    // Số tiền bồi thường (có thể là nil)
}

type AcceptExchangeOfferTxResult struct {
	Exchange       Exchange        `json:"exchange"`
	RejectedOffers []ExchangeOffer `json:"-"` // Danh sách các đề xuất bị từ chối
}

func (store *SQLStore) AcceptExchangeOfferTx(ctx context.Context, arg AcceptExchangeOfferTxParams) (AcceptExchangeOfferTxResult, error) {
	var result AcceptExchangeOfferTxResult
	
	err := store.ExecTx(ctx, func(qTx *Queries) error {
		// 1. Tạo giao dịch trao đổi (exchange)
		exchangeID, err := uuid.NewV7()
		if err != nil {
			return fmt.Errorf("failed to generate exchange ID: %w", err)
		}
		
		exchange, err := qTx.CreateExchange(ctx, CreateExchangeParams{
			ID:                 exchangeID,
			PosterID:           arg.PosterID,
			OffererID:          arg.OffererID,
			PayerID:            arg.PayerID,
			CompensationAmount: arg.CompensationAmount,
			Status:             ExchangeStatusPending,
		})
		if err != nil {
			return fmt.Errorf("failed to create exchange: %w", err)
		}
		result.Exchange = exchange
		
		// 2. Xử lý thanh toán tiền bù (nếu có)
		if arg.PayerID != nil && arg.CompensationAmount != nil && *arg.CompensationAmount > 0 {
			// Trừ tiền từ người bù
			_, err = qTx.CreateWalletEntry(ctx, CreateWalletEntryParams{
				WalletID:      *arg.PayerID,
				ReferenceID:   util.StringPointer(exchange.ID.String()),
				ReferenceType: WalletReferenceTypeExchange,
				EntryType:     WalletEntryTypePayment,
				Amount:        -*arg.CompensationAmount, // Truyền âm số âm để trừ tiền
				Status:        WalletEntryStatusCompleted,
				CompletedAt:   util.TimePointer(time.Now()),
			})
			if err != nil {
				return fmt.Errorf("failed to create wallet entry for payer: %w", err)
			}
			
			// Cập nhật số dư ví của người bù
			_, err = qTx.AddWalletBalance(ctx, AddWalletBalanceParams{
				Amount: -*arg.CompensationAmount,
				UserID: *arg.PayerID,
			})
			
			// Cộng tiền vào non_withdrawable_amount của người nhận tiền bù
			receiverID := arg.PosterID
			if *arg.PayerID == arg.PosterID {
				receiverID = arg.OffererID
			}
			
			_, err = qTx.CreateWalletEntry(ctx, CreateWalletEntryParams{
				WalletID:      receiverID,
				ReferenceID:   util.StringPointer(exchange.ID.String()),
				ReferenceType: WalletReferenceTypeExchange,
				EntryType:     WalletEntryTypeNonWithdrawable,
				Amount:        *arg.CompensationAmount, // Truyền dương số dương để cộng tiền
				Status:        WalletEntryStatusCompleted,
				CompletedAt:   util.TimePointer(time.Now()),
			})
			if err != nil {
				return fmt.Errorf("failed to create wallet entry for receiver: %w", err)
			}
			
			err = qTx.AddWalletNonWithdrawableAmount(ctx, AddWalletNonWithdrawableAmountParams{
				Amount: *arg.CompensationAmount,
				UserID: receiverID,
			})
			if err != nil {
				return fmt.Errorf("failed to add non withdrawable amount: %w", err)
			}
			
			// Tạo wallet entry cho người nhận tiền bù với status pending
			_, err = qTx.CreateWalletEntry(ctx, CreateWalletEntryParams{
				WalletID:      receiverID,
				ReferenceID:   util.StringPointer(exchange.ID.String()),
				ReferenceType: WalletReferenceTypeExchange,
				EntryType:     WalletEntryTypePaymentreceived,
				Amount:        *arg.CompensationAmount,
				Status:        WalletEntryStatusPending,
			})
			if err != nil {
				return fmt.Errorf("failed to create wallet entry for receiver: %w", err)
			}
		}
		
		// 3. Lấy tất cả Gundam từ bài đăng trao đổi
		postItems, err := qTx.ListExchangePostItems(ctx, arg.PostID)
		if err != nil {
			return fmt.Errorf("failed to list all post items: %w", err)
		}
		
		// 4. Lấy các Gundam từ bài đăng được chọn trong đề xuất
		selectedPostItems, err := qTx.ListExchangeOfferItems(ctx, ListExchangeOfferItemsParams{
			OfferID:      arg.OfferID,
			IsFromPoster: util.BoolPointer(true),
		})
		if err != nil {
			return fmt.Errorf("failed to list selected post items: %w", err)
		}
		
		// 5. Lấy các Gundam từ người đề xuất
		offererItems, err := qTx.ListExchangeOfferItems(ctx, ListExchangeOfferItemsParams{
			OfferID:      arg.OfferID,
			IsFromPoster: util.BoolPointer(false),
		})
		if err != nil {
			return fmt.Errorf("failed to list offerer items: %w", err)
		}
		
		// 6. Chuẩn bị danh sách ID của các nhóm Gundam
		// - selectedPosterGundamIDs: Gundam từ bài đăng được chọn để trao đổi
		// - unselectedPosterGundamIDs: Gundam từ bài đăng không được chọn
		// - offererGundamIDs: Gundam từ người đề xuất
		
		selectedPosterGundamIDs := make([]int64, 0, len(selectedPostItems))
		selectedGundamIDMap := make(map[int64]bool, len(selectedPostItems))
		
		for _, item := range selectedPostItems {
			selectedPosterGundamIDs = append(selectedPosterGundamIDs, item.GundamID)
			selectedGundamIDMap[item.GundamID] = true
		}
		
		unselectedPosterGundamIDs := make([]int64, 0, len(postItems)-len(selectedPostItems))
		for _, item := range postItems {
			if !selectedGundamIDMap[item.GundamID] {
				unselectedPosterGundamIDs = append(unselectedPosterGundamIDs, item.GundamID)
			}
		}
		
		offererGundamIDs := make([]int64, 0, len(offererItems))
		for _, item := range offererItems {
			offererGundamIDs = append(offererGundamIDs, item.GundamID)
		}
		
		// 7. Bulk update: Chuyển các Gundam được chọn từ bài đăng sang "exchanging"
		if len(selectedPosterGundamIDs) > 0 {
			err = qTx.BulkUpdateGundamsExchanging(ctx, BulkUpdateGundamsExchangingParams{
				GundamIds: selectedPosterGundamIDs,
				OwnerID:   arg.PosterID,
			})
			if err != nil {
				return fmt.Errorf("failed to update selected post gundams to exchanging: %w", err)
			}
		}
		
		// 8. Bulk update: Chuyển các Gundam không được chọn từ bài đăng về "in store"
		if len(unselectedPosterGundamIDs) > 0 {
			err = qTx.BulkUpdateGundamsInStore(ctx, BulkUpdateGundamsInStoreParams{
				GundamIds: unselectedPosterGundamIDs,
				OwnerID:   arg.PosterID,
			})
			if err != nil {
				return fmt.Errorf("failed to update unselected post gundams to in store: %w", err)
			}
		}
		
		// 9. Bulk update: Chuyển các Gundam từ người đề xuất sang "exchanging"
		if len(offererGundamIDs) > 0 {
			err = qTx.BulkUpdateGundamsExchanging(ctx, BulkUpdateGundamsExchangingParams{
				GundamIds: offererGundamIDs,
				OwnerID:   arg.OffererID,
			})
			if err != nil {
				return fmt.Errorf("failed to update offerer gundams to exchanging: %w", err)
			}
		}
		
		// 10. Tạo exchange_items cho các Gundam được trao đổi
		
		// 10.1 Cho Gundam từ bài đăng được chọn
		for _, gundamID := range selectedPosterGundamIDs {
			// Lấy thông tin chi tiết của Gundam
			gundam, err := store.GetGundamDetailsByID(ctx, qTx, gundamID)
			if err != nil {
				return fmt.Errorf("failed to get gundam details: %w", err)
			}
			
			exchangeItemID, err := uuid.NewV7()
			if err != nil {
				return fmt.Errorf("failed to generate exchange item ID: %w", err)
			}
			
			_, err = qTx.CreateExchangeItem(ctx, CreateExchangeItemParams{
				ID:           exchangeItemID,
				ExchangeID:   exchange.ID,
				GundamID:     &gundamID,
				Name:         gundam.Name,
				Slug:         gundam.Slug,
				Grade:        gundam.Grade,
				Scale:        gundam.Scale,
				Quantity:     gundam.Quantity,
				Price:        gundam.Price,
				Weight:       gundam.Weight,
				ImageURL:     gundam.PrimaryImageURL,
				OwnerID:      &gundam.OwnerID,
				IsFromPoster: true,
			})
			if err != nil {
				return fmt.Errorf("failed to create exchange item: %w", err)
			}
		}
		
		// 10.2 Cho Gundam từ người đề xuất
		for _, gundamID := range offererGundamIDs {
			// Lấy thông tin chi tiết của Gundam
			gundam, err := store.GetGundamDetailsByID(ctx, qTx, gundamID)
			if err != nil {
				return fmt.Errorf("failed to get gundam details: %w", err)
			}
			
			exchangeItemID, err := uuid.NewV7()
			if err != nil {
				return fmt.Errorf("failed to generate exchange item ID: %w", err)
			}
			
			_, err = qTx.CreateExchangeItem(ctx, CreateExchangeItemParams{
				ID:           exchangeItemID,
				ExchangeID:   exchange.ID,
				GundamID:     &gundamID,
				Name:         gundam.Name,
				Slug:         gundam.Slug,
				Grade:        gundam.Grade,
				Scale:        gundam.Scale,
				Quantity:     gundam.Quantity,
				Price:        gundam.Price,
				Weight:       gundam.Weight,
				ImageURL:     gundam.PrimaryImageURL,
				OwnerID:      &gundam.OwnerID,
				IsFromPoster: false,
			})
			if err != nil {
				return fmt.Errorf("failed to create exchange item: %w", err)
			}
		}
		
		// 11. Trước khi xóa cứng bài đăng, lấy danh sách tất cả đề xuất không được chấp nhận.
		rejectedOffers, err := qTx.ListExchangeOffersByPostExcluding(ctx, ListExchangeOffersByPostExcludingParams{
			PostID:         arg.PostID,
			ExcludeOfferID: arg.OfferID,
		})
		if err != nil {
			return fmt.Errorf("failed to list other offers: %w", err)
		}
		result.RejectedOffers = rejectedOffers
		
		// 12. Xóa cứng bài đăng
		_, err = qTx.DeleteExchangePost(ctx, arg.PostID)
		if err != nil {
			return fmt.Errorf("failed to delete exchange post: %w", err)
		}
		
		return nil
	})
	
	return result, err
}
