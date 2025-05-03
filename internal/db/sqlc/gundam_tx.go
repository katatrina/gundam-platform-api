package db

import (
	"context"
	"fmt"
	"mime/multipart"
	
	"github.com/katatrina/gundam-BE/internal/util"
)

type CreateGundamTxParams struct {
	OwnerID              string
	Name                 string
	Slug                 string
	GradeID              int64
	Series               string
	PartsTotal           int64
	Material             string
	Version              string
	Quantity             int64
	Condition            GundamCondition
	ConditionDescription *string
	Manufacturer         string
	Weight               int64
	Scale                GundamScale
	Description          string
	Price                *int64
	ReleaseYear          *int64
	Accessories          []GundamAccessoryDTO
	PrimaryImage         *multipart.FileHeader
	SecondaryImages      []*multipart.FileHeader
	UploadImagesFunc     func(key string, value string, folder string, files ...*multipart.FileHeader) ([]string, error)
}

func (store *SQLStore) CreateGundamTx(ctx context.Context, arg CreateGundamTxParams) (GundamDetails, error) {
	var result GundamDetails
	err := store.ExecTx(ctx, func(qTx *Queries) error {
		gundam, err := qTx.CreateGundam(ctx, CreateGundamParams{
			OwnerID:              arg.OwnerID,
			Name:                 arg.Name,
			Slug:                 arg.Slug,
			GradeID:              arg.GradeID,
			Series:               arg.Series,
			PartsTotal:           arg.PartsTotal,
			Material:             arg.Material,
			Version:              arg.Version,
			Quantity:             arg.Quantity,
			Condition:            arg.Condition,
			ConditionDescription: arg.ConditionDescription,
			Manufacturer:         arg.Manufacturer,
			Weight:               arg.Weight,
			Scale:                arg.Scale,
			Description:          arg.Description,
			Price:                arg.Price,
			ReleaseYear:          arg.ReleaseYear,
		})
		if err != nil {
			return err
		}
		
		grade, err := qTx.GetGradeByID(ctx, gundam.ID)
		if err != nil {
			return err
		}
		
		result.ID = gundam.ID
		result.OwnerID = gundam.OwnerID
		result.Name = gundam.Name
		result.Slug = gundam.Slug
		result.Grade = grade.DisplayName
		result.Series = gundam.Series
		result.PartsTotal = gundam.PartsTotal
		result.Material = gundam.Material
		result.Version = gundam.Version
		result.Quantity = gundam.Quantity
		result.Condition = string(gundam.Condition)
		result.ConditionDescription = gundam.ConditionDescription
		result.Manufacturer = gundam.Manufacturer
		result.Weight = gundam.Weight
		result.Scale = string(gundam.Scale)
		result.Description = gundam.Description
		result.Price = gundam.Price
		result.ReleaseYear = gundam.ReleaseYear
		result.Status = string(gundam.Status)
		result.CreatedAt = gundam.CreatedAt
		result.UpdatedAt = gundam.UpdatedAt
		
		// Upload primary image and store the URL
		primaryImageURLs, err := arg.UploadImagesFunc("gundam", gundam.Slug, util.FolderGundams, arg.PrimaryImage)
		if err != nil {
			return err
		}
		err = qTx.StoreGundamImageURL(ctx, StoreGundamImageURLParams{
			GundamID:  gundam.ID,
			URL:       primaryImageURLs[0],
			IsPrimary: true,
		})
		if err != nil {
			return err
		}
		result.PrimaryImageURL = primaryImageURLs[0]
		
		// Upload secondary images and store the URLs
		secondaryImageURLs, err := arg.UploadImagesFunc("gundam", gundam.Slug, util.FolderGundams, arg.SecondaryImages...)
		if err != nil {
			return err
		}
		for _, url := range secondaryImageURLs {
			err = qTx.StoreGundamImageURL(ctx, StoreGundamImageURLParams{
				GundamID:  gundam.ID,
				URL:       url,
				IsPrimary: false,
			})
			if err != nil {
				return err
			}
			
			result.SecondaryImageURLs = append(result.SecondaryImageURLs, url)
		}
		
		// Create accessories if any
		for _, accessory := range arg.Accessories {
			gundamAccessory, err := qTx.CreateGundamAccessory(ctx, CreateGundamAccessoryParams{
				GundamID: gundam.ID,
				Name:     accessory.Name,
				Quantity: accessory.Quantity,
			})
			if err != nil {
				return err
			}
			
			result.Accessories = append(result.Accessories, GundamAccessoryDTO{
				Name:     gundamAccessory.Name,
				Quantity: gundamAccessory.Quantity,
			})
		}
		
		return nil
	})
	
	return result, err
}

type UpdateGundamAccessoriesParams struct {
	GundamID    int64
	Accessories []GundamAccessoryDTO
}

func (store *SQLStore) UpdateGundamAccessoriesTx(ctx context.Context, arg UpdateGundamAccessoriesParams) error {
	err := store.ExecTx(ctx, func(qTx *Queries) error {
		// 1. Delete all existing accessories
		err := qTx.DeleteAllGundamAccessories(ctx, arg.GundamID)
		if err != nil {
			return err
		}
		
		// 2. Create new accessories
		for _, accessory := range arg.Accessories {
			_, err = qTx.CreateGundamAccessory(ctx, CreateGundamAccessoryParams{
				GundamID: arg.GundamID,
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

type AddGundamSecondaryImagesTxParams struct {
	Gundam           Gundam
	Images           []*multipart.FileHeader
	UploadImagesFunc func(key string, value string, folder string, files ...*multipart.FileHeader) ([]string, error)
}

type AddGundamSecondaryImagesTxResult struct {
	ImageURLs []string `json:"image_urls"`
}

func (store *SQLStore) AddGundamSecondaryImagesTx(ctx context.Context, arg AddGundamSecondaryImagesTxParams) (AddGundamSecondaryImagesTxResult, error) {
	var result AddGundamSecondaryImagesTxResult
	err := store.ExecTx(ctx, func(qTx *Queries) error {
		// 1. Upload secondary images and store the URLs
		secondaryImageURLs, err := arg.UploadImagesFunc("gundam", arg.Gundam.Slug, util.FolderGundams, arg.Images...)
		if err != nil {
			return fmt.Errorf("failed to upload secondary images: %w", err)
		}
		
		// 2. Store the URLs in the database
		for _, url := range secondaryImageURLs {
			err = qTx.StoreGundamImageURL(ctx, StoreGundamImageURLParams{
				GundamID:  arg.Gundam.ID,
				URL:       url,
				IsPrimary: false,
			})
			if err != nil {
				return fmt.Errorf("failed to store secondary image URL: %w", err)
			}
		}
		
		result.ImageURLs = secondaryImageURLs
		
		return nil
	})
	
	return result, err
}

type DeleteGundamSecondaryImageTxParams struct {
	GundamImage     GundamImage
	PublicID        string
	DeleteImageFunc func(publicID string, folder string) error
}

func (store *SQLStore) DeleteGundamSecondaryImageTx(ctx context.Context, arg DeleteGundamSecondaryImageTxParams) error {
	return store.ExecTx(ctx, func(qTx *Queries) error {
		// 1. Delete the secondary image from the database
		err := qTx.DeleteGundamImage(ctx, DeleteGundamImageParams{
			GundamID: arg.GundamImage.GundamID,
			URL:      arg.GundamImage.URL,
		})
		if err != nil {
			return fmt.Errorf("failed to delete secondary image from database: %w", err)
		}
		
		// 2. Delete the image from Cloudinary
		err = arg.DeleteImageFunc(arg.PublicID, "")
		if err != nil {
			return fmt.Errorf("failed to delete image from Cloudinary: %w", err)
		}
		
		return nil
	})
}
