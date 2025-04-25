package api

import (
	"fmt"
	"io"
	"mime/multipart"
	"time"
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
