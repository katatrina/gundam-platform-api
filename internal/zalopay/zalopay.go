package zalopay

import (
	db "github.com/katatrina/gundam-BE/internal/db/sqlc"
	"github.com/katatrina/gundam-BE/internal/util"
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
	config  *util.Config
}

func NewZalopayService(dbStore db.Store, config *util.Config) *ZalopayService {
	return &ZalopayService{
		appID:   sandboxTestAppID,
		key1:    sandboxTestKey1,
		key2:    sandboxTestKey2,
		dbStore: dbStore,
		config:  config,
	}
}
