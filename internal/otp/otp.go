package otp

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"errors"
	"fmt"
	"math/big"
	"time"
	
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog/log"
)

type OTPService struct {
	redis           *redis.Client
	rateLimitPrefix string
	otpPrefix       string
	attemptPrefix   string
	expiration      time.Duration
	maxAttempts     int
}

type OTPServiceOption func(*OTPService)

func NewOTPService(redis *redis.Client, opts ...OTPServiceOption) *OTPService {
	s := &OTPService{
		redis:       redis,
		expiration:  10 * time.Minute,
		maxAttempts: 3,
	}
	
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func WithPrefixes(rateLimit, otp, attempt string) OTPServiceOption {
	return func(s *OTPService) {
		s.rateLimitPrefix = rateLimit
		s.otpPrefix = otp
		s.attemptPrefix = attempt
	}
}

func (s *OTPService) GenerateOTP(ctx context.Context, identifier string) (code string, createdAt time.Time, expiresAt time.Time, err error) {
	rateLimitKey := fmt.Sprintf("%s:%s", s.rateLimitPrefix, identifier)
	ttl, err := s.redis.TTL(ctx, rateLimitKey).Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		return "", time.Time{}, time.Time{}, err
	}
	
	if ttl.Seconds() > 0 {
		return "", time.Now(), time.Now().Add(ttl), fmt.Errorf("please wait before requesting new OTP")
	}
	
	otp, err := generateSixDigitOTP()
	if err != nil {
		return "", time.Time{}, time.Time{}, err
	}
	
	pipe := s.redis.Pipeline()
	otpKey := fmt.Sprintf("%s:%s", s.otpPrefix, identifier)
	
	// Load specific timezone
	loc, err := time.LoadLocation("Asia/Ho_Chi_Minh")
	if err != nil {
		// Handle error
		log.Error().Err(err).Msg("failed to load timezone")
	}
	
	now := time.Now().In(loc)
	expiresAt = now.Add(s.expiration)
	pipe.Set(ctx, otpKey, otp, s.expiration)
	
	pipe.Set(ctx, rateLimitKey, "1", 1*time.Minute)
	
	attemptKey := fmt.Sprintf("%s:%s", s.attemptPrefix, identifier)
	pipe.Set(ctx, attemptKey, 0, s.expiration)
	
	if _, err := pipe.Exec(ctx); err != nil {
		return "", time.Time{}, time.Time{}, err
	}
	
	return otp, now, expiresAt, nil
}

func (s *OTPService) VerifyOTP(ctx context.Context, identifier, providedOTP string) (bool, error) {
	if len(providedOTP) != 6 {
		return false, fmt.Errorf("invalid OTP format")
	}
	
	pipe := s.redis.Pipeline()
	otpKey := fmt.Sprintf("%s:%s", s.otpPrefix, identifier)
	attemptKey := fmt.Sprintf("%s:%s", s.attemptPrefix, identifier)
	
	storedOTPCmd := pipe.Get(ctx, otpKey)
	attemptsCmd := pipe.Incr(ctx, attemptKey)
	
	_, err := pipe.Exec(ctx)
	if err != nil && !errors.Is(err, redis.Nil) {
		return false, fmt.Errorf("failed to get OTP data: %w", err)
	}
	
	storedOTP, err := storedOTPCmd.Result()
	if errors.Is(err, redis.Nil) {
		return false, fmt.Errorf("no OTP found or expired")
	}
	if err != nil {
		return false, fmt.Errorf("failed to retrieve OTP: %w", err)
	}
	
	attempts, err := attemptsCmd.Result()
	if err != nil {
		return false, fmt.Errorf("failed to track attempts: %w", err)
	}
	
	if attempts > int64(s.maxAttempts) {
		pipe.Del(ctx, otpKey, attemptKey)
		if _, err := pipe.Exec(ctx); err != nil {
			log.Error().Err(err).Msg("failed to clean up after max attempts")
		}
		return false, fmt.Errorf("max attempts (%d) exceeded", s.maxAttempts)
	}
	
	if subtle.ConstantTimeCompare([]byte(storedOTP), []byte(providedOTP)) == 1 {
		pipe.Del(ctx, otpKey, attemptKey)
		pipe.Del(ctx, fmt.Sprintf("%s:%s", s.rateLimitPrefix, identifier))
		
		if _, err := pipe.Exec(ctx); err != nil {
			log.Error().Err(err).Msg("failed to clean up after successful verification")
		}
		return true, nil
	}
	
	remainingAttempts := s.maxAttempts - int(attempts)
	return false, fmt.Errorf("invalid OTP, %d attempts remaining", remainingAttempts)
}

func generateSixDigitOTP() (string, error) {
	// Generate number between 100000 and 999999
	maxInt := big.NewInt(999999)
	minInt := big.NewInt(100000)
	diff := new(big.Int).Sub(maxInt, minInt)
	
	n, err := rand.Int(rand.Reader, diff)
	if err != nil {
		return "", err
	}
	
	n.Add(n, minInt)
	return n.String(), nil
}
