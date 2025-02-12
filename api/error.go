package api

import (
	"github.com/gin-gonic/gin"
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
