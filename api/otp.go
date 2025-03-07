package api

import (
	"context"
	"net/http"
	"strings"
	"time"
	
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgtype"
	db "github.com/katatrina/gundam-BE/internal/db/sqlc"
	"github.com/rs/zerolog/log"
)

// GeneratePhoneOTPRequest represents the input structure for generating an OTP
type GeneratePhoneOTPRequest struct {
	PhoneNumber string `json:"phone_number" binding:"required"`
}

// GeneratePhoneOTPResponse represents the response structure after OTP generation
type GeneratePhoneOTPResponse struct {
	OTPCode     string    `json:"otp_code"`
	ExpiresAt   time.Time `json:"expires_at"`
	PhoneNumber string    `json:"phone_number"`
	CanResendIn time.Time `json:"can_resend_in"`
}

//	@Summary		Generate a One-Time Password (OTP) for phone number
//	@Description	Generates and sends an OTP to the specified phone number
//	@Tags			authentication
//	@Accept			json
//	@Produce		json
//	@Param			request	body		GeneratePhoneOTPRequest		true	"OTP Generation Request"
//	@Success		200		{object}	GeneratePhoneOTPResponse	"OTP generated successfully"
//	@Failure		400		"Bad Request - Invalid input"
//	@Failure		429		"Too Many Requests - OTP request rate limit exceeded"
//	@Failure		500		"Internal Server Error"
//	@Router			/otp/phone/generate [get]
func (server *Server) generatePhoneOTP(c *gin.Context) {
	var req GeneratePhoneOTPRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse(err))
		return
	}
	
	code, canSendIn, err := server.otpService.GenerateAndSendOTP(c, req.PhoneNumber)
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
		ExpiresAt:   time.Now().Add(10 * time.Minute),
		CanResendIn: canSendIn,
	})
}

type VerifyPhoneOTPRequest struct {
	UserID      string `json:"user_id" binding:"required"`
	PhoneNumber string `json:"phone_number" binding:"required"`
	OTPCode     string `json:"otp_code" binding:"required,len=6"`
}

//	@Summary		Verify One-Time Password (OTP) via phone number
//	@Description	Verifies the OTP sent to a user's phone number and updates the user's phone number if valid
//	@Tags			authentication
//	@Accept			json
//	@Produce		json
//	@Param			request	body	VerifyPhoneOTPRequest	true	"OTP Verification Request"
//	@Success		200		"OTP verified successfully"
//	@Failure		400		"Bad Request - Invalid input or OTP verification failed"
//	@Failure		401		"Unauthorized - Invalid OTP code"
//	@Failure		500		"Internal Server Error - Failed to update user information"
//	@Router			/otp/phone/verify [post]
func (server *Server) verifyPhoneOTP(c *gin.Context) {
	req := new(VerifyPhoneOTPRequest)
	if err := c.ShouldBindJSON(&req); err != nil {
		log.Error().Err(err).Msg("failed to bind JSON")
		c.JSON(http.StatusBadRequest, errorResponse(err))
		return
	}
	
	valid, err := server.otpService.VerifyOTP(c, req.PhoneNumber, req.OTPCode)
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
	
	// Update user's phone number
	arg := db.UpdateUserParams{
		UserID: req.UserID,
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
		log.Error().Err(err).Msg("failed to update user's phone number")
		c.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	c.JSON(http.StatusOK, gin.H{"message": "OTP verified successfully"})
}
