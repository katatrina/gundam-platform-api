package token

import (
	"fmt"
	"strconv"
	"time"
	
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

type Payload struct {
	jwt.RegisteredClaims
	Role string `json:"role"`
}

func NewPayload(userID int64, role string, duration time.Duration) (payload Payload, err error) {
	tokenID, err := uuid.NewRandom()
	if err != nil {
		return payload, fmt.Errorf("failed to generate tokenID: %w", err)
	}
	
	payload = Payload{
		Role: role,
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        tokenID.String(),
			Issuer:    "cvp",
			Subject:   strconv.FormatInt(userID, 10),
			Audience:  jwt.ClaimStrings{"client"},
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			NotBefore: jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(duration)),
		},
	}
	
	return payload, nil
}
