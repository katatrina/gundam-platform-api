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
		// 1. Create exchange post
		exchangePostID, _ := uuid.NewV7()
		exchangePost, err := qTx.CreateExchangePost(ctx, CreateExchangePostParams{
			ID:      exchangePostID,
			UserID:  arg.UserID,
			Content: arg.Content,
			// PostImageUrls: nil,
		})
		if err != nil {
			return err
		}
		
		// 2. Create exchange post items
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
		
		// 3. Update gundam status
		err = qTx.BulkUpdateGundamsForExchange(ctx, BulkUpdateGundamsForExchangeParams{
			OwnerID:   arg.UserID,
			GundamIds: arg.PostItemIDs,
		})
		if err != nil {
			return fmt.Errorf("failed to update gundam status to 'for exchange': %w", err)
		}
		
		// 4. Upload images
		imageUrls, err := arg.UploadImagesFunc("exchange_post", exchangePost.ID.String()[:10], util.FolderExchanges, arg.PostImages...)
		if err != nil {
			return err
		}
		
		// 5. Update exchange post with image URLs
		updatedExchangePost, err := qTx.UpdateExchangePost(ctx, UpdateExchangePostParams{
			ID:            exchangePost.ID,
			PostImageUrls: imageUrls,
		})
		if err != nil {
			return err
		}
		result.ExchangePost = updatedExchangePost
		
		return nil
	})
	
	return result, err
}
