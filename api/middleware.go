package api

import (
	"errors"
	"fmt"
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
	sellerPayloadKey        = "sellerPayload"
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

func requiredSellerRole(dbStore db.Store) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		authPayload := ctx.MustGet(authorizationPayloadKey).(*token.Payload)
		authenticatedUserID := authPayload.Subject
		sellerID := ctx.Param("sellerID")
		
		seller, err := dbStore.GetSellerByID(ctx, sellerID)
		if err != nil {
			if errors.Is(err, db.ErrRecordNotFound) {
				err = fmt.Errorf("seller ID %s not found", sellerID)
				ctx.AbortWithStatusJSON(http.StatusNotFound, errorResponse(err))
				return
			}
			
			ctx.AbortWithStatusJSON(http.StatusInternalServerError, errorResponse(err))
			return
		}
		
		if authenticatedUserID != seller.ID {
			ctx.AbortWithStatusJSON(http.StatusForbidden, errorResponse(ErrSellerIDMismatch))
			return
		}
		
		ctx.Set(sellerPayloadKey, &seller)
		ctx.Next()
	}
}
