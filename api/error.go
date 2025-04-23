package api

import (
	"errors"
	
	"github.com/gin-gonic/gin"
)

var (
	ErrSellerIDMismatch = errors.New("seller OfferID in URL does not match authenticated user OfferID")
)

type FailedValidationResponse struct {
	Message         string            `json:"message"`
	FieldViolations []*FieldViolation `json:"field_violations"`
}

type FieldViolation struct {
	Field       string `json:"field"`
	Description string `json:"description"`
}

func fieldViolation(field string, err error) *FieldViolation {
	return &FieldViolation{
		Field:       field,
		Description: err.Error(),
	}
}

func errorResponse(err error) gin.H {
	return gin.H{"error": err.Error()}
}

func failedValidationError(violations []*FieldViolation) *FailedValidationResponse {
	return &FailedValidationResponse{
		Message:         "Invalid request parameters",
		FieldViolations: violations,
	}
}
