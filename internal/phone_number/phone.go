package phone_number

import (
	"context"
	"fmt"
	
	"github.com/bwmarrin/discordgo"
	"github.com/katatrina/gundam-BE/internal/otp"
	"github.com/katatrina/gundam-BE/internal/util"
	"github.com/redis/go-redis/v9"
)

type PhoneService struct {
	otpService *otp.OTPService
	discord    *discordgo.Session
	config     util.Config
}

func NewPhoneService(config util.Config, redis *redis.Client) (*PhoneService, error) {
	// Initialize Discord
	discord, err := discordgo.New("Bot " + config.DiscordBotToken)
	if err != nil {
		return nil, fmt.Errorf("failed to create Discord session: %w", err)
	}
	
	return &PhoneService{
		discord: discord,
		otpService: otp.NewOTPService(redis,
			otp.WithPrefixes("ratelimit:phone_number", "otp:phone_number", "attempts:phone_number"),
		),
		config: config,
	}, nil
}

func (s *PhoneService) SendOTP(ctx context.Context, phone string) (code string, err error) {
	code, createdAt, expiresAt, err := s.otpService.GenerateOTP(ctx, phone)
	if err != nil {
		return code, err
	}
	
	// Format message
	message := fmt.Sprintf("Số điện thoại: %s | OTP: %s | Hiệu lực từ %s đến %s",
		phone,
		code,
		// Format time in HH:MM:SS dd/mm/yyyy
		createdAt.Format("15:04:05 02/01/2006"),
		expiresAt.Format("15:04:05 02/01/2006"),
	)
	
	// Tạm thời: gửi OTP qua Discord
	_, err = s.discord.ChannelMessageSend(s.config.DiscordChannelID, message)
	return code, err
}

func (s *PhoneService) VerifyOTP(ctx context.Context, phone string, code string) (bool, error) {
	// Verify OTP
	ok, err := s.otpService.VerifyOTP(ctx, phone, code)
	if err != nil {
		return false, err
	}
	
	if !ok {
		return false, fmt.Errorf("invalid OTP")
	}
	
	return true, nil
}
