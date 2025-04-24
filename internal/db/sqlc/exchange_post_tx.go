package db

import (
	"context"
	"fmt"
	"mime/multipart"
	
	"github.com/google/uuid"
	"github.com/katatrina/gundam-BE/internal/util"
)

type CreateExchangePostTxParams struct {
	UserID           string
	Content          string
	PostImages       []*multipart.FileHeader
	PostItemIDs      []int64
	UploadImagesFunc func(key string, value string, folder string, files ...*multipart.FileHeader) ([]string, error)
}

type CreateExchangePostTxResult struct {
	ExchangePost      ExchangePost       `json:"exchange_post"`
	ExchangePostItems []ExchangePostItem `json:"exchange_post_items"`
}

func (store *SQLStore) CreateExchangePostTx(ctx context.Context, arg CreateExchangePostTxParams) (CreateExchangePostTxResult, error) {
	var result CreateExchangePostTxResult
	err := store.ExecTx(ctx, func(qTx *Queries) error {
		exchangePostID, _ := uuid.NewV7()
		
		// 1. Upload images
		imageUrls, err := arg.UploadImagesFunc("exchange_post", exchangePostID.String()[:8], util.FolderExchanges, arg.PostImages...)
		if err != nil {
			return err
		}
		
		// 2. Create exchange post
		exchangePost, err := qTx.CreateExchangePost(ctx, CreateExchangePostParams{
			ID:            exchangePostID,
			UserID:        arg.UserID,
			Content:       arg.Content,
			PostImageUrls: imageUrls,
		})
		if err != nil {
			return err
		}
		result.ExchangePost = exchangePost
		
		// 3. Create exchange post items
		// Create a new slice of UUID for the exchange post items
		exchangePostItemIDs := make([]uuid.UUID, len(arg.PostItemIDs))
		for i, _ := range arg.PostItemIDs {
			exchangePostItemIDs[i], _ = uuid.NewV7()
		}
		exchangePostItems, err := qTx.CreateExchangePostItems(ctx, CreateExchangePostItemsParams{
			ItemIds:   exchangePostItemIDs,
			PostID:    exchangePost.ID,
			GundamIds: arg.PostItemIDs,
		})
		if err != nil {
			return err
		}
		result.ExchangePostItems = exchangePostItems
		
		// 4. Update gundam status
		err = qTx.BulkUpdateGundamsForExchange(ctx, BulkUpdateGundamsForExchangeParams{
			OwnerID:   arg.UserID,
			GundamIds: arg.PostItemIDs,
		})
		if err != nil {
			return fmt.Errorf("failed to update gundam status to 'for exchange': %w", err)
		}
		
		return nil
	})
	
	return result, err
}

type DeleteExchangePostTxParams struct {
	PostID uuid.UUID
	UserID string
}

type DeleteExchangePostTxResult struct {
	DeletedExchangePost       ExchangePost    `json:"deleted_exchange_post"`
	DeletedExchangePostOffers []ExchangeOffer `json:"deleted_exchange_post_offers"`
}

func (store *SQLStore) DeleteExchangePostTx(ctx context.Context, arg DeleteExchangePostTxParams) (DeleteExchangePostTxResult, error) {
	var result DeleteExchangePostTxResult
	
	err := store.ExecTx(ctx, func(qTx *Queries) error {
		// 1. Lấy danh sách các item của chủ bài đăng
		items, err := qTx.ListExchangePostItems(ctx, arg.PostID)
		if err != nil {
			return fmt.Errorf("failed to list exchange post items: %w", err)
		}
		
		// 2. Cập nhật trạng thái của các Gundam trong bài đăng về trạng thái "in store"
		if len(items) > 0 {
			postGundamIDs := make([]int64, len(items))
			for i, item := range items {
				postGundamIDs[i] = item.GundamID
			}
			
			err = qTx.BulkUpdateGundamsInStore(ctx, BulkUpdateGundamsInStoreParams{
				OwnerID:   arg.UserID,
				GundamIds: postGundamIDs,
			})
			if err != nil {
				return fmt.Errorf("failed to update gundam status to 'in store': %w", err)
			}
		}
		
		// 3. Lấy danh sách các đề xuất trao đổi
		offers, err := qTx.ListExchangeOffers(ctx, arg.PostID)
		if err != nil {
			return fmt.Errorf("failed to list exchange offers: %w", err)
		}
		result.DeletedExchangePostOffers = offers
		
		// 4. Cập nhật trạng thái của các Gundam trong đề xuất
		for _, offer := range offers {
			// Chỉ lấy danh sách các item của người đề xuất
			offerItems, err := qTx.ListExchangeOfferItems(ctx, ListExchangeOfferItemsParams{
				OfferID:      offer.ID,
				IsFromPoster: util.BoolPointer(false),
			})
			if err != nil {
				return fmt.Errorf("failed to list exchange offer items: %w", err)
			}
			
			if len(offerItems) > 0 {
				offerGundamIDs := make([]int64, len(offerItems))
				for i, item := range offerItems {
					offerGundamIDs[i] = item.GundamID
				}
				
				err = qTx.BulkUpdateGundamsInStore(ctx, BulkUpdateGundamsInStoreParams{
					OwnerID:   offer.OffererID,
					GundamIds: offerGundamIDs,
				})
				if err != nil {
					return fmt.Errorf("failed to update gundam status to 'in store': %w", err)
				}
			}
		}
		
		// 5. Xóa bài đăng trao đổi
		deletedPost, err := qTx.DeleteExchangePost(ctx, arg.PostID)
		if err != nil {
			return fmt.Errorf("failed to delete exchange post: %w", err)
		}
		result.DeletedExchangePost = deletedPost
		
		return nil
	})
	
	return result, err
}
