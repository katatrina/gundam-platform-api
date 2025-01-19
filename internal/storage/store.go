package storage

type FileStore interface {
	UploadFile(file []byte, filename string) (string, error)
}
