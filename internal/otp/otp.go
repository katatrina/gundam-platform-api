package otp

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"time"
	
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog/log"
)

// OTPService xử lý việc tạo và xác thực mã OTP
type OTPService struct {
	redis      *redis.Client // Kết nối đến Redis để lưu trữ OTP
	otpPrefix  string        // Prefix để phân biệt các loại OTP (email, phone, etc.)
	expiration time.Duration // Thời gian sống của OTP (10 phút)
}

// OTPServiceOption là function type để cấu hình OTPService
type OTPServiceOption func(*OTPService)

// NewOTPService tạo một instance mới của OTPService
func NewOTPService(redis *redis.Client, opts ...OTPServiceOption) *OTPService {
	s := &OTPService{
		redis:      redis,
		expiration: 10 * time.Minute, // Mặc định OTP có hiệu lực 10 phút
	}
	
	// Áp dụng các tùy chọn cấu hình
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// WithPrefix cấu hình prefix cho các key trong Redis
func WithPrefix(prefix string) OTPServiceOption {
	return func(s *OTPService) {
		s.otpPrefix = prefix // Ví dụ: "otp:phone_number" hoặc "otp:email"
	}
}

// GenerateOTP tạo mã OTP mới và lưu vào Redis
func (s *OTPService) GenerateOTP(ctx context.Context, identifier string) (code string, createdAt time.Time, expiresAt time.Time, err error) {
	// Tạo mã OTP ngẫu nhiên 6 chữ số
	otp, err := generateSixDigitOTP()
	if err != nil {
		return "", time.Time{}, time.Time{}, err
	}
	
	// Tạo key để lưu OTP trong Redis
	// Ví dụ: otp:phone_number:0987993316
	otpKey := fmt.Sprintf("%s:%s", s.otpPrefix, identifier)
	
	// Lấy múi giờ Việt Nam
	loc, err := time.LoadLocation("Asia/Ho_Chi_Minh")
	if err != nil {
		log.Error().Err(err).Msg("failed to load timezone")
	}
	
	// Tính thời gian tạo và hết hạn của OTP
	now := time.Now().In(loc)
	expiresAt = now.Add(s.expiration)
	
	// Lưu OTP vào Redis với thời gian sống s.expiration
	// Nếu đã có OTP cũ, nó sẽ bị ghi đè bởi OTP mới
	err = s.redis.Set(ctx, otpKey, otp, s.expiration).Err()
	if err != nil {
		return "", time.Time{}, time.Time{}, err
	}
	
	return otp, now, expiresAt, nil
}

// VerifyOTP xác thực mã OTP được cung cấp
func (s *OTPService) VerifyOTP(ctx context.Context, identifier, providedOTP string) (bool, error) {
	// Kiểm tra độ dài OTP phải là 6 chữ số
	if len(providedOTP) != 6 {
		return false, fmt.Errorf("invalid OTP format")
	}
	
	// Tạo key để lấy OTP từ Redis
	otpKey := fmt.Sprintf("%s:%s", s.otpPrefix, identifier)
	
	// Lấy OTP từ Redis
	storedOTP, err := s.redis.Get(ctx, otpKey).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return false, fmt.Errorf("no OTP found or expired")
		}
		return false, fmt.Errorf("failed to retrieve OTP: %w", err)
	}
	
	// So sánh OTP được cung cấp với OTP trong Redis
	if storedOTP == providedOTP {
		// Nếu OTP đúng, xóa nó khỏi Redis để không thể sử dụng lại
		s.redis.Del(ctx, otpKey)
		return true, nil
	}
	
	return false, fmt.Errorf("invalid OTP")
}

// generateSixDigitOTP tạo mã OTP ngẫu nhiên 6 chữ số
func generateSixDigitOTP() (string, error) {
	// Tạo số ngẫu nhiên từ 100000 đến 999999
	maxInt := big.NewInt(999999)
	minInt := big.NewInt(100000)
	diff := new(big.Int).Sub(maxInt, minInt)
	
	// Sử dụng crypto/rand để tạo số ngẫu nhiên an toàn
	n, err := rand.Int(rand.Reader, diff)
	if err != nil {
		return "", err
	}
	
	// Cộng với minInt để đảm bảo số luôn có 6 chữ số
	n.Add(n, minInt)
	return n.String(), nil
}
