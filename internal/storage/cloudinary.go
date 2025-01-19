package storage

import (
	"github.com/cloudinary/cloudinary-go/v2"
	"github.com/rs/zerolog/log"
)

type CloudinaryStore struct {
	*cloudinary.Cloudinary
}

func (cld *CloudinaryStore) UploadFile(file []byte, filename string) (string, error) {
	panic("implement me")
}

func NewCloudinaryStore() FileStore {
	cld, err := cloudinary.New()
	if err != nil {
		log.Fatal().Err(err).Msg("failed to create cloudinary store 😣")
	}
	
	cld.Config.URL.Secure = true
	
	return &CloudinaryStore{cld}
}
