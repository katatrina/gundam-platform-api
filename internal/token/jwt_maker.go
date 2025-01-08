package token

import (
	"fmt"
	"time"
	
	"github.com/golang-jwt/jwt/v5"
)

const (
	minSecretKeyLength int = 32
)

type JWTMaker struct {
	secretKey string
}

// NewJWTMaker creates a new JWTMaker with the specific secret key.
func NewJWTMaker(secretKey string) (Maker, error) {
	if len(secretKey) < minSecretKeyLength {
		return nil, fmt.Errorf("invalid secret key size: must be at least %d characters", minSecretKeyLength)
	}
	
	return &JWTMaker{secretKey}, nil
}

func (maker *JWTMaker) CreateToken(email string, role string, duration time.Duration) (string, *Payload, error) {
	payload, err := NewPayload(email, role, duration)
	if err != nil {
		return "", nil, fmt.Errorf("failed to create payload: %w", err)
	}
	
	unsignedToken := jwt.NewWithClaims(jwt.SigningMethodHS256, payload)
	
	signedToken, err := unsignedToken.SignedString([]byte(maker.secretKey))
	return signedToken, &payload, err
}

func (maker *JWTMaker) VerifyToken(tokenString string) (*Payload, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Payload{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		
		return []byte(maker.secretKey), nil
	})
	if err != nil {
		return nil, err
	}
	
	payload, ok := token.Claims.(*Payload)
	if !ok {
		return nil, fmt.Errorf("unknown payload type, cannot proceed")
	}
	
	return payload, nil
}
