package api

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"
	
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgtype"
	db "github.com/katatrina/gundam-BE/internal/db/sqlc"
	"github.com/katatrina/gundam-BE/internal/mailer"
	"github.com/rs/zerolog/log"
)

// GeneratePhoneOTPRequest represents the input structure for generating an OTP
type GeneratePhoneOTPRequest struct {
	PhoneNumber string `json:"phone_number" binding:"required"`
}

// GeneratePhoneOTPResponse represents the response structure after OTP generation
type GeneratePhoneOTPResponse struct {
	OTPCode     string    `json:"otp_code"`
	PhoneNumber string    `json:"phone_number"`
	ExpiresAt   time.Time `json:"expires_at"`
	CreatedAt   time.Time `json:"created_at"`
}

// @Summary		Generate a One-Time Password (OTP) for phone_number number
// @Description	Generates and sends an OTP to the specified phone_number number
// @Tags			authentication
// @Accept			json
// @Produce		json
// @Param			request	body		GeneratePhoneOTPRequest		true	"OTP Generation Request"
// @Success		200		{object}	GeneratePhoneOTPResponse	"OTP generated successfully"
// @Failure		400		"Bad Request - Invalid input"
// @Failure		429		"Too Many Requests - OTP request rate limit exceeded"
// @Failure		500		"Internal Server Error"
// @Router			/otp/phone_number/generate [post]
func (server *Server) generatePhoneOTP(c *gin.Context) {
	var req GeneratePhoneOTPRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse(err))
		return
	}
	
	code, expiresAt, createdAt, err := server.phoneNumberService.SendOTP(c, req.PhoneNumber)
	if err != nil {
		if strings.Contains(err.Error(), "wait") {
			c.JSON(http.StatusTooManyRequests, errorResponse(err))
			return
		}
		
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
	PhoneNumber string `json:"phone_number" binding:"required"`
	OTPCode     string `json:"otp_code" binding:"required,len=6"`
}

// @Summary		Verify One-Time Password (OTP) via phone_number number
// @Description	Verifies the OTP sent to a user's phone_number number and updates the user's phone_number number if valid
// @Tags			authentication
// @Accept			json
// @Produce		json
// @Param			request	body	VerifyPhoneOTPRequest	true	"OTP Verification Request"
// @Success		200		"OTP verified successfully"
// @Failure		400		"Bad Request - Invalid input or OTP verification failed"
// @Failure		401		"Unauthorized - Invalid OTP code"
// @Failure		500		"Internal Server Error - Failed to update user information"
// @Router			/otp/phone/verify [post]
func (server *Server) verifyPhoneOTP(c *gin.Context) {
	req := new(VerifyPhoneOTPRequest)
	if err := c.ShouldBindJSON(&req); err != nil {
		log.Error().Err(err).Msg("failed to bind JSON")
		c.JSON(http.StatusBadRequest, errorResponse(err))
		return
	}
	
	// Kiểm tra xem số điện thoại đã được sử dụng chưa
	user, err := server.dbStore.GetUserByPhoneNumber(c.Request.Context(), pgtype.Text{
		String: req.PhoneNumber,
		Valid:  true,
	})
	if err != nil {
		if !errors.Is(err, db.ErrRecordNotFound) {
			// Lỗi khác khi truy vấn cơ sở dữ liệu
			log.Error().Err(err).Msg("failed to check phone number")
			c.JSON(http.StatusInternalServerError, errorResponse(err))
			return
		}
		
		// Số điện thoại chưa được sử dụng, có thể tiếp tục
	}
	
	valid, err := server.phoneNumberService.VerifyOTP(c, req.PhoneNumber, req.OTPCode)
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
	
	c.JSON(http.StatusOK, gin.H{"message": "OTP verified successfully"})
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
	code, createdAt, expiresAt, err := server.mailer.SendOTP(mailHeader)
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
	
	valid, err := server.mailer.VerifyOTP(c.Request.Context(), req.Email, req.OTPCode)
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
