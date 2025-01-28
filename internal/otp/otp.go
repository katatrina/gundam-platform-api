package otp

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"errors"
	"fmt"
	"math/big"
	"time"
	
	"github.com/bwmarrin/discordgo"
	"github.com/katatrina/gundam-BE/internal/util"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog/log"
)

//goland:noinspection ALL
type OTPService struct {
	redis   *redis.Client
	discord *discordgo.Session
	config  util.Config
}

func NewOTPService(config util.Config, redisDb *redis.Client) (*OTPService, error) {
	// Initialize Discord
	discord, err := discordgo.New("Bot " + config.DiscordBotToken)
	if err != nil {
		return nil, fmt.Errorf("failed to create Discord session: %w", err)
	}
	
	return &OTPService{
		redis:   redisDb,
		discord: discord,
		config:  config,
	}, nil
}

func (s *OTPService) GenerateAndSendOTP(ctx context.Context, phoneNumber string) (code string, canSendIn time.Time, err error) {
	// Check rate limiting
	rateLimitKey := fmt.Sprintf("ratelimit:%s", phoneNumber)
	ttl, err := s.redis.TTL(ctx, rateLimitKey).Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		return "", time.Time{}, err
	}
	
	// If rate limited, return the time when they can send next
	if ttl.Seconds() > 0 {
		nextAllowed := time.Now().Add(ttl)
		return "", nextAllowed, fmt.Errorf("please wait before requesting new OTP")
	}
	
	// Generate 6-digit OTP
	otp, err := generateSixDigitOTP()
	if err != nil {
		return "", time.Time{}, err
	}
	
	// Use Redis pipeline for atomic operations
	pipe := s.redis.Pipeline()
	
	// Store OTP with 10-minute expiry
	otpKey := fmt.Sprintf("otp:%s", phoneNumber)
	pipe.Set(ctx, otpKey, otp, 10*time.Minute)
	
	// Set rate limit (1 minute)
	pipe.Set(ctx, rateLimitKey, "1", 1*time.Minute)
	
	// Store attempt count
	attemptKey := fmt.Sprintf("attempts:%s", phoneNumber)
	pipe.Set(ctx, attemptKey, 0, 10*time.Minute)
	
	// Execute pipeline
	if _, err := pipe.Exec(ctx); err != nil {
		return "", time.Time{}, err
	}
	
	// Send to Discord Channel
	message := fmt.Sprintf("Số điện thoại: %s | OTP: %s | Hiệu lực từ: %s đến %s",
		phoneNumber,
		otp,
		time.Now().Format("15:04:05"),
		time.Now().Add(10*time.Minute).Format("15:04:05"),
	)
	
	if _, err = s.discord.ChannelMessageSend(s.config.DiscordChannelID, message); err != nil {
		return "", time.Time{}, err
	}
	
	// Return OTP and when they can send next (1 minute from now)
	nextAllowed := time.Now().Add(1 * time.Minute)
	return otp, nextAllowed, nil
}

func (s *OTPService) VerifyOTP(ctx context.Context, phoneNumber, providedOTP string) (bool, error) {
	// Validate inputs
	if len(providedOTP) != 6 {
		return false, fmt.Errorf("invalid OTP format")
	}
	
	// Use Redis transaction to ensure atomicity
	pipe := s.redis.Pipeline()
	
	otpKey := fmt.Sprintf("otp:%s", phoneNumber)
	attemptKey := fmt.Sprintf("attempts:%s", phoneNumber)
	
	// Get stored OTP and current attempts in one round trip
	storedOTPCmd := pipe.Get(ctx, otpKey)
	attemptsCmd := pipe.Incr(ctx, attemptKey)
	
	_, err := pipe.Exec(ctx)
	if err != nil && !errors.Is(err, redis.Nil) {
		return false, fmt.Errorf("failed to get OTP data: %w", err)
	}
	
	// Check if OTP exists
	storedOTP, err := storedOTPCmd.Result()
	if errors.Is(err, redis.Nil) {
		return false, fmt.Errorf("no OTP found or expired")
	}
	if err != nil {
		return false, fmt.Errorf("failed to retrieve OTP: %w", err)
	}
	
	// Get attempt count
	attempts, err := attemptsCmd.Result()
	if err != nil {
		return false, fmt.Errorf("failed to track attempts: %w", err)
	}
	
	// Check max attempts (3 tries)
	if attempts > 3 {
		// Clean up on max attempts
		pipe.Del(ctx, otpKey, attemptKey)
		if _, err := pipe.Exec(ctx); err != nil {
			// Log error but don't return it since we want to return max attempts error
			log.Printf("failed to clean up after max attempts: %v", err)
		}
		return false, fmt.Errorf("max attempts (3) exceeded, please request a new OTP")
	}
	
	// Time-constant comparison to prevent timing attacks
	if subtle.ConstantTimeCompare([]byte(storedOTP), []byte(providedOTP)) == 1 {
		// Clean up on success
		pipe.Del(ctx, otpKey, attemptKey)
		// Also clean up rate limit to allow immediate new OTP request after successful verification
		pipe.Del(ctx, fmt.Sprintf("ratelimit:%s", phoneNumber))
		
		if _, err := pipe.Exec(ctx); err != nil {
			// Log error but don't return it since verification was successful
			log.Printf("failed to clean up after successful verification: %v", err)
		}
		
		return true, nil
	}
	
	// Calculate remaining attempts
	remainingAttempts := 3 - attempts
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
