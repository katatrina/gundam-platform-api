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
	// Create upload parameters
	uploadParams := uploader.UploadParams{
		Folder:         folder,
		PublicID:       strings.TrimSuffix(filename, filepath.Ext(filename)),
		UniqueFilename: api.Bool(false),
		Overwrite:      api.Bool(true),
	}
	
	reader := bytes.NewReader(file)
	result, err := cld.Upload.Upload(context.Background(), reader, uploadParams)
	if err != nil {
		err = fmt.Errorf("failed to upload file to cloudinary: %w", err)
		return "", err
	}
	
	return result.SecureURL, nil
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
