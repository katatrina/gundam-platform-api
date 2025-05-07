package storage

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"
	
	"github.com/rs/zerolog/log"
	
	"github.com/cloudinary/cloudinary-go/v2"
	"github.com/cloudinary/cloudinary-go/v2/api"
	"github.com/cloudinary/cloudinary-go/v2/api/uploader"
)

type CloudinaryStore struct {
	*cloudinary.Cloudinary
}

func (cld *CloudinaryStore) UploadFile(file []byte, filename string, folder string) (string, error) {
	// Kiểm tra extension của file
	fileExt := filepath.Ext(filename)
	fileBase := strings.TrimSuffix(filename, filepath.Ext(filename))
	
	// Tạo upload parameters mặc định
	uploadParams := uploader.UploadParams{
		Folder:         folder,
		PublicID:       fileBase,
		UniqueFilename: api.Bool(false),
		Overwrite:      api.Bool(true),
	}
	
	// Xử lý đặc biệt cho file SVG
	if strings.ToLower(fileExt) == ".svg" {
		// Đặt resource_type là image và format là svg cho file SVG
		uploadParams.ResourceType = "image"
		uploadParams.Format = "svg"
	}
	
	// Tiến hành upload
	reader := bytes.NewReader(file)
	result, err := cld.Upload.Upload(context.Background(), reader, uploadParams)
	if err != nil {
		err = fmt.Errorf("failed to upload file to cloudinary: %w", err)
		return "", err
	}
	
	// Đảm bảo URL cho SVG có extension đúng
	secureURL := result.SecureURL
	if strings.ToLower(fileExt) == ".svg" && !strings.HasSuffix(secureURL, ".svg") {
		secureURL = secureURL + ".svg"
	}
	
	return secureURL, nil
}

func (cld *CloudinaryStore) DeleteFile(publicID string, folder string) error {
	if publicID == "" {
		return fmt.Errorf("publicID cannot be empty")
	}
	
	fullPublicID := publicID
	if folder != "" {
		fullPublicID = fmt.Sprintf("%s/%s", folder, publicID)
	}
	
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	
	_, err := cld.Upload.Destroy(ctx, uploader.DestroyParams{
		PublicID: fullPublicID,
	})
	if err != nil {
		return fmt.Errorf("failed to delete file from cloudinary (ID: %s): %w", fullPublicID, err)
	}
	
	return nil
}

func NewCloudinaryStore(url string) FileStore {
	cld, err := cloudinary.NewFromURL(url)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to create cloudinary store 😣")
	}
	
	cld.Config.URL.Secure = true
	
	return &CloudinaryStore{cld}
}
