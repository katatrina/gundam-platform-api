package api

import (
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"time"
	
	db "github.com/katatrina/gundam-BE/internal/db/sqlc"
)

func (server *Server) uploadFileToCloudinary(key string, value string, folder string, files ...*multipart.FileHeader) (uploadedFileURLs []string, err error) {
	if len(files) == 0 {
		return nil, fmt.Errorf("no files provided")
	}
	
	for _, file := range files {
		// Open and read file
		currentFile, err := file.Open()
		if err != nil {
			return nil, fmt.Errorf("failed to open file: %w", err)
		}
		defer currentFile.Close()
		
		fileBytes, err := io.ReadAll(currentFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read file: %w", err)
		}
		
		fileName := fmt.Sprintf("%s_%s_%d", key, value, time.Now().Unix())
		
		// Upload new avatar to cloudinary
		uploadedFileURL, err := server.fileStore.UploadFile(fileBytes, fileName, folder)
		if err != nil {
			return nil, fmt.Errorf("failed to upload file: %w", err)
		}
		
		uploadedFileURLs = append(uploadedFileURLs, uploadedFileURL)
	}
	
	return uploadedFileURLs, nil
}

// Helper function để lấy thông tin chi tiết Gundam
func (server *Server) getGundamDetailsByID(ctx context.Context, gundamID int64) (db.GundamDetails, error) {
	var detail db.GundamDetails
	
	gundam, err := server.dbStore.GetGundamByID(ctx, gundamID)
	if err != nil {
		return detail, err
	}
	
	grade, err := server.dbStore.GetGradeByID(ctx, gundam.GradeID)
	if err != nil {
		return detail, err
	}
	
	primaryImageURL, err := server.dbStore.GetGundamPrimaryImageURL(ctx, gundam.ID)
	if err != nil {
		return detail, err
	}
	
	secondaryImageURLs, err := server.dbStore.GetGundamSecondaryImageURLs(ctx, gundam.ID)
	if err != nil {
		return detail, err
	}
	
	accessories, err := server.dbStore.GetGundamAccessories(ctx, gundam.ID)
	if err != nil {
		return detail, err
	}
	
	accessoryDTOs := make([]db.GundamAccessoryDTO, len(accessories))
	for i, accessory := range accessories {
		accessoryDTOs[i] = db.ConvertGundamAccessoryToDTO(accessory)
	}
	
	detail = db.GundamDetails{
		ID:                   gundam.ID,
		OwnerID:              gundam.OwnerID,
		Name:                 gundam.Name,
		Slug:                 gundam.Slug,
		Grade:                grade.DisplayName,
		Series:               gundam.Series,
		PartsTotal:           gundam.PartsTotal,
		Material:             gundam.Material,
		Version:              gundam.Version,
		Quantity:             gundam.Quantity,
		Condition:            string(gundam.Condition),
		ConditionDescription: gundam.ConditionDescription,
		Manufacturer:         gundam.Manufacturer,
		Weight:               gundam.Weight,
		Scale:                string(gundam.Scale),
		Description:          gundam.Description,
		Price:                gundam.Price,
		ReleaseYear:          gundam.ReleaseYear,
		Status:               string(gundam.Status),
		Accessories:          accessoryDTOs,
		PrimaryImageURL:      primaryImageURL,
		SecondaryImageURLs:   secondaryImageURLs,
		CreatedAt:            gundam.CreatedAt,
		UpdatedAt:            gundam.UpdatedAt,
	}
	
	return detail, nil
}
