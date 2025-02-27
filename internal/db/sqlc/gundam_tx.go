package db

import (
	"context"
	"mime/multipart"
	
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/katatrina/gundam-BE/internal/util"
)

type CreateGundamTxParams struct {
	OwnerID              string
	Name                 string
	Slug                 string
	GradeID              int64
	Condition            GundamCondition
	ConditionDescription pgtype.Text
	Manufacturer         string
	Weight               int64
	Scale                GundamScale
	Description          string
	Price                int64
	Accessories          []GundamAccessory
	PrimaryImage         *multipart.FileHeader
	SecondaryImages      []*multipart.FileHeader
	UploadImagesFunc     func(key string, value string, folder string, files ...*multipart.FileHeader) ([]string, error)
}

func (store *SQLStore) CreateGundamTx(ctx context.Context, arg CreateGundamTxParams) error {
	err := store.ExecTx(ctx, func(qTx *Queries) error {
		gundam, err := qTx.CreateGundam(ctx, CreateGundamParams{
			OwnerID:              arg.OwnerID,
			Name:                 arg.Name,
			Slug:                 arg.Slug,
			GradeID:              arg.GradeID,
			Condition:            arg.Condition,
			ConditionDescription: arg.ConditionDescription,
			Manufacturer:         arg.Manufacturer,
			Weight:               arg.Weight,
			Scale:                arg.Scale,
			Description:          arg.Description,
			Price:                arg.Price,
		})
		if err != nil {
			return err
		}
		
		// Upload primary image and store the URL
		primaryImageURLs, err := arg.UploadImagesFunc("gundam", gundam.Slug, util.FolderGundams, arg.PrimaryImage)
		if err != nil {
			return err
		}
		err = qTx.StoreGundamImageURL(ctx, StoreGundamImageURLParams{
			GundamID:  gundam.ID,
			Url:       primaryImageURLs[0],
			IsPrimary: true,
		})
		
		// Upload secondary images and store the URLs
		secondaryImageURLs, err := arg.UploadImagesFunc("gundam", gundam.Slug, util.FolderGundams, arg.SecondaryImages...)
		if err != nil {
			return err
		}
		for _, url := range secondaryImageURLs {
			err = qTx.StoreGundamImageURL(ctx, StoreGundamImageURLParams{
				GundamID:  gundam.ID,
				Url:       url,
				IsPrimary: false,
			})
			if err != nil {
				return err
			}
		}
		
		// Create accessories if any
		for _, accessory := range arg.Accessories {
			_, err = qTx.CreateGundamAccessory(ctx, CreateGundamAccessoryParams{
				GundamID: gundam.ID,
				Name:     accessory.Name,
				Quantity: accessory.Quantity,
			})
			if err != nil {
				return err
			}
		}
		
		return nil
	})
	
	return err
}
