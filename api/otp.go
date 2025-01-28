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

type GenerateOTPRequest struct {
	PhoneNumber string `json:"phone_number" binding:"required"`
}

type GenerateOTPResponse struct {
	OTPCode     string    `json:"otp_code"`
	ExpiresAt   time.Time `json:"expires_at"` // in seconds
	PhoneNumber string    `json:"phone_number"`
	CanResendIn time.Time `json:"can_resend_in,omitempty"` // seconds util next OTP is allowed
}

func (server *Server) generateOTP(c *gin.Context) {
	var req GenerateOTPRequest
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
	
	c.JSON(http.StatusOK, GenerateOTPResponse{
		OTPCode:     code,
		PhoneNumber: req.PhoneNumber,
		ExpiresAt:   time.Now().Add(10 * time.Minute),
		CanResendIn: canSendIn,
	})
}

type VerifyOTPRequest struct {
	UserID      string `json:"user_id"`
	PhoneNumber string `json:"phone_number" binding:"required"`
	OTPCode     string `json:"otp_code" binding:"required,len=6"`
}

func (server *Server) verifyOTP(c *gin.Context) {
	req := new(VerifyOTPRequest)
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
	}
	_, err = server.dbStore.UpdateUser(context.Background(), arg)
	if err != nil {
		log.Error().Err(err).Msg("failed to update user's phone number")
		c.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	c.JSON(http.StatusOK, gin.H{"message": "OTP code verified successfully!"})
}
