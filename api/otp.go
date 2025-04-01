package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"
	
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgtype"
	db "github.com/katatrina/gundam-BE/internal/db/sqlc"
	"github.com/katatrina/gundam-BE/internal/mailer"
	"github.com/katatrina/gundam-BE/internal/util"
	"github.com/rs/zerolog/log"
)

// GeneratePhoneOTPRequest represents the input structure for generating an OTP
type GeneratePhoneOTPRequest struct {
	PhoneNumber string `json:"phone_number" binding:"required"` // Số điện thoại cần gửi OTP
}

// GeneratePhoneOTPResponse represents the response structure after OTP generation
type GeneratePhoneOTPResponse struct {
	OTPCode     string    `json:"otp_code"`     // Mã OTP được tạo
	PhoneNumber string    `json:"phone_number"` // Số điện thoại đã gửi OTP
	ExpiresAt   time.Time `json:"expires_at"`   // Thời điểm OTP hết hạn
	CreatedAt   time.Time `json:"created_at"`   // Thời điểm OTP được tạo
}

// @Summary		Generate a One-Time Password (OTP) for phone number
// @Description	Generates and sends an OTP to the specified phone number. The OTP will be valid for 10 minutes.
// Phone number must be a valid Vietnamese phone number (10-11 digits, starting with 03, 05, 07, 08, 09, or 84).
// @Tags			authentication
// @Accept			json
// @Produce		json
// @Param			request	body		GeneratePhoneOTPRequest		true	"OTP Generation Request"
// @Success		200		{object}	GeneratePhoneOTPResponse	"OTP generated successfully"
// @Failure		400		"Bad Request - Invalid phone number format"
// @Failure		500		"Internal Server Error - Failed to generate or send OTP"
// @Router			/otp/phone_number/generate [post]
func (server *Server) generatePhoneNumberOTP(c *gin.Context) {
	var req GeneratePhoneOTPRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		log.Error().Err(err).Msg("failed to bind JSON request")
		c.JSON(http.StatusBadRequest, errorResponse(err))
		return
	}
	
	// Validate phone number format (Vietnam)
	if !util.IsValidVietnamesePhoneNumber(req.PhoneNumber) {
		log.Error().Str("phone", req.PhoneNumber).Msg("invalid phone number format")
		c.JSON(http.StatusBadRequest, errorResponse(fmt.Errorf("invalid phone number format")))
		return
	}
	
	// Tạo và gửi OTP
	code, expiresAt, createdAt, err := server.phoneNumberService.SendOTP(c, req.PhoneNumber)
	if err != nil {
		log.Error().Err(err).Msg("failed to send OTP")
		c.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	c.JSON(http.StatusOK, GeneratePhoneOTPResponse{
		OTPCode:     code,
		PhoneNumber: req.PhoneNumber,
		ExpiresAt:   expiresAt,
		CreatedAt:   createdAt,
	})
}

type VerifyPhoneOTPRequest struct {
	UserID      string `json:"user_id" binding:"required"`        // ID của user cần cập nhật số điện thoại
	PhoneNumber string `json:"phone_number" binding:"required"`   // Số điện thoại mới
	OTPCode     string `json:"otp_code" binding:"required,len=6"` // Mã OTP
}

// @Summary		Verify One-Time Password (OTP) via phone number
// @Description	Verifies the OTP sent to a user's phone number and updates the user's phone number if valid
// @Tags			authentication
// @Accept			json
// @Produce		json
// @Param			request	body	VerifyPhoneOTPRequest	true	"OTP Verification Request"
// @Success		200		"OTP verified successfully"
// @Failure		400		"Bad Request - Invalid input or OTP verification failed"
// @Failure		401		"Unauthorized - Invalid OTP code"
// @Failure		404		"Not Found - User not found"
// @Failure		500		"Internal Server Error - Failed to update user information"
// @Router			/otp/phone_number/verify [post]
func (server *Server) verifyPhoneNumberOTP(c *gin.Context) {
	var req VerifyPhoneOTPRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		log.Error().Err(err).Msg("failed to bind JSON")
		c.JSON(http.StatusBadRequest, errorResponse(err))
		return
	}
	
	// Validate phone number format (Vietnam)
	if !util.IsValidVietnamesePhoneNumber(req.PhoneNumber) {
		log.Error().Str("phone", req.PhoneNumber).Msg("invalid phone number format")
		c.JSON(http.StatusBadRequest, errorResponse(fmt.Errorf("invalid phone number format")))
		return
	}
	
	// Kiểm tra user có tồn tại không
	user, err := server.dbStore.GetUserByID(c.Request.Context(), req.UserID)
	if err != nil {
		if errors.Is(err, db.ErrRecordNotFound) {
			log.Error().Err(err).Msg("user not found")
			c.JSON(http.StatusNotFound, errorResponse(fmt.Errorf("user not found")))
			return
		}
		log.Error().Err(err).Msg("failed to get user")
		c.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	// So sánh số điện thoại mới với số điện thoại hiện tại
	if user.PhoneNumber.String == req.PhoneNumber {
		c.JSON(http.StatusBadRequest, errorResponse(fmt.Errorf("cannot update to the same phone number")))
		return
	}
	
	// Xác thực OTP
	valid, err := server.phoneNumberService.VerifyOTP(c, req.PhoneNumber, req.OTPCode)
	if err != nil {
		log.Error().Err(err).Msg("failed to verify OTP")
		c.JSON(http.StatusBadRequest, errorResponse(err))
		return
	}
	
	if !valid {
		c.JSON(http.StatusUnauthorized, errorResponse(err))
		return
	}
	
	// Kiểm tra xem số điện thoại đã được sử dụng bởi user khác chưa
	_, err = server.dbStore.GetUserByPhoneNumber(c.Request.Context(), pgtype.Text{
		String: req.PhoneNumber,
		Valid:  true,
	})
	if err != nil {
		if !errors.Is(err, db.ErrRecordNotFound) {
			log.Error().Err(err).Msg("failed to get user by phone number")
			c.JSON(http.StatusInternalServerError, errorResponse(err))
			return
		}
	}
	// Số điện thoại vẫn chưa được sử dụng
	
	// Update user's phone_number column
	arg := db.UpdateUserParams{
		UserID: user.ID,
		PhoneNumber: pgtype.Text{
			String: req.PhoneNumber,
			Valid:  true,
		},
		PhoneNumberVerified: pgtype.Bool{
			Bool:  true,
			Valid: true,
		},
	}
	_, err = server.dbStore.UpdateUser(context.Background(), arg)
	if err != nil {
		log.Error().Err(err).Msg("failed to update user's phone_number number")
		c.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	c.JSON(http.StatusOK, gin.H{"message": "OTP verified successfully and phone number updated"})
}

type GenerateEmailOTPRequest struct {
	Email string `json:"email" binding:"required,email"`
}

type GenerateEmailOTPResponse struct {
	OTPCode   string    `json:"otp_code"`
	Email     string    `json:"email"`
	ExpiresAt time.Time `json:"expires_at"`
	CreatedAt time.Time `json:"created_at"`
}

// @Summary		Generate a One-Time Password (OTP) for email
// @Description	Generates and sends an OTP to the specified email address
// @Tags			authentication
// @Accept			json
// @Produce		json
// @Param			request	body		GenerateEmailOTPRequest		true	"OTP Generation Request"
// @Success		200		{object}	GenerateEmailOTPResponse	"OTP generated successfully"
// @Failure		400		"Bad Request - Invalid input"
// @Failure		429		"Too Many Requests - OTP request rate limit exceeded"
// @Failure		500		"Internal Server Error"
// @Router			/otp/email/generate [post]
func (server *Server) generateEmailOTP(c *gin.Context) {
	var req GenerateEmailOTPRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse(err))
		return
	}
	
	mailHeader := mailer.EmailHeader{
		Subject: "Your OTP Code",
		To:      []string{req.Email},
	}
	code, createdAt, expiresAt, err := server.mailService.SendOTP(mailHeader)
	if err != nil {
		log.Error().Err(err).Msg("failed to send OTP email")
		c.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	c.JSON(http.StatusOK, GenerateEmailOTPResponse{
		OTPCode:   code,
		Email:     req.Email,
		ExpiresAt: expiresAt,
		CreatedAt: createdAt,
	})
}

type VerifyEmailOTPRequest struct {
	Email   string `json:"email" binding:"required,email"`
	OTPCode string `json:"otp_code" binding:"required,len=6"`
}

// @Summary		Verify One-Time Password (OTP) via email
// @Description	Verifies the OTP sent to a user's email address and updates the user's email if valid
// @Tags			authentication
// @Accept			json
// @Produce		json
// @Param			request	body	VerifyEmailOTPRequest	true	"OTP Verification Request"
// @Success		200		"OTP verified successfully"
// @Failure		400		"Bad Request - Invalid input or OTP verification failed"
// @Failure		401		"Unauthorized - Invalid OTP code"
// @Failure		500		"Internal Server Error - Failed to update user information"
// @Router			/otp/email/verify [post]
func (server *Server) verifyEmailOTP(c *gin.Context) {
	req := new(VerifyEmailOTPRequest)
	if err := c.ShouldBindJSON(&req); err != nil {
		log.Error().Err(err).Msg("failed to bind JSON")
		c.JSON(http.StatusBadRequest, errorResponse(err))
		return
	}
	
	valid, err := server.mailService.VerifyOTP(c.Request.Context(), req.Email, req.OTPCode)
	if err != nil {
		log.Error().Err(err).Msg("failed to verify OTP")
		c.JSON(http.StatusBadRequest, errorResponse(err))
		return
	}
	
	if !valid {
		log.Error().Msg("invalid OTP code")
		c.JSON(http.StatusUnauthorized, gin.H{"message": "Invalid OTP code"})
		return
	}
	
	c.JSON(http.StatusOK, gin.H{"message": "OTP verified successfully"})
}
