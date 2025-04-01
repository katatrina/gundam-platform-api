package phone_number

import (
	"context"
	"fmt"
	"time"
	
	"github.com/bwmarrin/discordgo"
	"github.com/katatrina/gundam-BE/internal/otp"
	"github.com/katatrina/gundam-BE/internal/util"
	"github.com/redis/go-redis/v9"
)

// PhoneNumberService xử lý các thao tác liên quan đến số điện thoại
// Bao gồm việc gửi và xác thực OTP qua số điện thoại
type PhoneNumberService struct {
	otpService *otp.OTPService    // Service để tạo và xác thực OTP
	discord    *discordgo.Session // Kết nối Discord để gửi OTP (tạm thời)
	config     *util.Config       // Cấu hình của ứng dụng
}

// NewPhoneService tạo một instance mới của PhoneNumberService
// Khởi tạo kết nối Discord và cấu hình OTP service
func NewPhoneService(config *util.Config, redis *redis.Client) (*PhoneNumberService, error) {
	// Khởi tạo kết nối Discord với token bot
	discord, err := discordgo.New("Bot " + config.DiscordBotToken)
	if err != nil {
		return nil, fmt.Errorf("failed to create Discord session: %w", err)
	}
	
	return &PhoneNumberService{
		discord: discord,
		// Khởi tạo OTP service với prefix cho số điện thoại
		otpService: otp.NewOTPService(redis,
			otp.WithPrefix("otp:phone_number"),
		),
		config: config,
	}, nil
}

// SendOTP tạo và gửi mã OTP đến số điện thoại
// Hiện tại đang gửi qua Discord (tạm thời)
func (s *PhoneNumberService) SendOTP(ctx context.Context, phoneNumber string) (code string, expiresAt time.Time, createdAt time.Time, err error) {
	// Tạo mã OTP mới
	code, createdAt, expiresAt, err = s.otpService.GenerateOTP(ctx, phoneNumber)
	if err != nil {
		return "", time.Time{}, time.Time{}, err
	}
	
	// Định dạng nội dung tin nhắn
	message := fmt.Sprintf("Số điện thoại: %s | OTP: %s | Hiệu lực từ %s đến %s",
		phoneNumber,
		code,
		createdAt.Format("15:04:05 02/01/2006"),
		expiresAt.Format("15:04:05 02/01/2006"),
	)
	
	// Gửi OTP qua Discord
	// TODO: Thay thế bằng SMS service thực tế
	_, err = s.discord.ChannelMessageSend(s.config.DiscordChannelID, message)
	return code, expiresAt, createdAt, err
}

// VerifyOTP xác thực mã OTP được cung cấp
// Trả về true nếu OTP hợp lệ, false nếu không
func (s *PhoneNumberService) VerifyOTP(ctx context.Context, phone string, code string) (bool, error) {
	// Xác thực OTP
	ok, err := s.otpService.VerifyOTP(ctx, phone, code)
	if err != nil {
		return false, err
	}
	
	return ok, nil
}
