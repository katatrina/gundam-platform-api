package zalopay

import (
	db "github.com/katatrina/gundam-BE/internal/db/sqlc"
)

const (
	sandboxTestAppID = "2553"
	sandboxTestKey1  = "PcY4iZIKFCIdgZvA6ueMcMHHUbRLYjPL"
	sandboxTestKey2  = "kLtgPl8HHhfvMuDHPwKfgfsY4Ydm9eIz"
)

type ZalopayService struct {
	appID   string
	key1    string
	key2    string
	dbStore db.Store
}

func NewZalopayService(dbStore db.Store) *ZalopayService {
	return &ZalopayService{
		appID:   sandboxTestAppID,
		key1:    sandboxTestKey1,
		key2:    sandboxTestKey2,
		dbStore: dbStore,
	}
}
