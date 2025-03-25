package mailer

import (
	"context"
	"fmt"
	"time"
	
	"github.com/katatrina/gundam-BE/internal/otp"
	"github.com/katatrina/gundam-BE/internal/util"
	"github.com/redis/go-redis/v9"
	"github.com/wneessen/go-mail"
)

const (
	smtpGmailHost = "smtp.gmail.com"
	smtpGmailPort = 587
)

type GmailSender struct {
	client     *mail.Client
	redis      *redis.Client
	config     util.Config
	otpService *otp.OTPService
}

func NewGmailSender(username, password string, config util.Config, redisDb *redis.Client) (*GmailSender, error) {
	client, err := mail.NewClient(smtpGmailHost, mail.WithPort(smtpGmailPort), mail.WithSMTPAuth(mail.SMTPAuthPlain),
		mail.WithUsername(username), mail.WithPassword(password))
	if err != nil {
		return nil, err
	}
	if err = client.DialWithContext(context.Background()); err != nil {
		return nil, fmt.Errorf("failed to connect to SMTP server: %w", err)
	}
	
	return &GmailSender{
		client: client,
		redis:  redisDb,
		config: config,
		otpService: otp.NewOTPService(redisDb,
			otp.WithPrefixes("ratelimit:email", "otp:email", "attempts:email"),
		),
	}, nil
}

func (sender *GmailSender) SendOTP(
	header EmailHeader,
) (code string, createdAt time.Time, expiresAt time.Time, err error) {
	// Initialize a new email message
	msg := mail.NewMsg()
	
	// Set "From: Mecha World <mechaworldcapstone@gmail.com>"
	err = msg.FromFormat(senderEmailName, senderEmailAddress)
	if err != nil {
		return "", time.Time{}, time.Time{}, fmt.Errorf("failed to set From address: %w", err)
	}
	
	// Set the subject title
	msg.Subject(header.Subject)
	
	// Set the recipient email addresses
	if err = msg.To(header.To...); err != nil {
		return "", time.Time{}, time.Time{}, fmt.Errorf("failed to set To address: %w", err)
	}
	
	code, createdAt, expiresAt, err = sender.otpService.GenerateOTP(context.Background(), header.To[0])
	body := fmt.Sprintf("Your OTP code is: %s", code)
	msg.SetBodyString(mail.TypeTextHTML, body)
	
	// Send email
	if err = sender.client.DialAndSend(msg); err != nil {
		return "", time.Time{}, time.Time{}, fmt.Errorf("failed to send email: %w", err)
	}
	
	return code, createdAt, expiresAt, nil
}

func (sender *GmailSender) VerifyOTP(
	ctx context.Context,
	email string,
	code string,
) (bool, error) {
	ok, err := sender.otpService.VerifyOTP(ctx, email, code)
	if err != nil {
		return false, err
	}
	
	if !ok {
		return false, fmt.Errorf("invalid OTP")
	}
	
	return true, nil
}
