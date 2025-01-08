package token

import (
	"time"
)

type Maker interface {
	CreateToken(email string, role string, duration time.Duration) (token string, payload *Payload, err error)
	VerifyToken(tokenString string) (payload *Payload, err error)
}
