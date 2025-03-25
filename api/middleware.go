package api

import (
	"errors"
	"net/http"
	"strings"
	
	"github.com/gin-gonic/gin"
	db "github.com/katatrina/gundam-BE/internal/db/sqlc"
	"github.com/katatrina/gundam-BE/internal/token"
)

const (
	authorizationHeaderKey  = "Authorization"
	authorizationTypeBearer = "Bearer"
	authorizationPayloadKey = "authPayload"
)

// authMiddleware authenticates the user.
func authMiddleware(tokenMaker token.Maker) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		authorizationHeader := ctx.GetHeader(authorizationHeaderKey)
		if authorizationHeader == "" {
			err := errors.New("authorization header is not provided")
			ctx.AbortWithStatusJSON(http.StatusUnauthorized, errorResponse(err))
			return
		}
		
		fields := strings.Fields(authorizationHeader)
		if len(fields) != 2 {
			err := errors.New("invalid authorization header format")
			ctx.AbortWithStatusJSON(http.StatusUnauthorized, errorResponse(err))
			return
		}
		
		authorizationHeaderType := fields[0]
		if authorizationHeaderType != authorizationTypeBearer {
			err := errors.New("unsupported authorization header type")
			ctx.AbortWithStatusJSON(http.StatusUnauthorized, errorResponse(err))
			return
		}
		
		accessToken := fields[1]
		payload, err := tokenMaker.VerifyToken(accessToken)
		if err != nil {
			ctx.AbortWithStatusJSON(http.StatusUnauthorized, errorResponse(err))
			return
		}
		
		ctx.Set(authorizationPayloadKey, payload)
		ctx.Next()
	}
}

func requiredSellerOrAdminRole(ctx *gin.Context) {
	payload := ctx.MustGet(authorizationPayloadKey).(*token.Payload)
	sellerID := ctx.Param("sellerID")
	if payload.Role != string(db.UserRoleAdmin) {
		if payload.Role == string(db.UserRoleSeller) && payload.Subject == sellerID {
			ctx.Next()
			return
		}
		
		err := errors.New("user is not authorized to access this resource")
		ctx.AbortWithStatusJSON(http.StatusForbidden, errorResponse(err))
		return
	}
	
	ctx.Next()
}
