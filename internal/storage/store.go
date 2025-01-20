package storage

type FileStore interface {
	UploadFile(file []byte, filename string, folder string) (string, error)
	DeleteFile(publicID string, folder string) error
}
