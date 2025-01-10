package token

import (
	"time"
)

type Maker interface {
	CreateToken(userID int64, role string, duration time.Duration) (token string, payload *Payload, err error)
	VerifyToken(tokenString string) (payload *Payload, err error)
}
