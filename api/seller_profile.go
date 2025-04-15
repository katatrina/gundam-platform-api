package api

import (
	"errors"
	"net/http"
	
	"github.com/gin-gonic/gin"
	db "github.com/katatrina/gundam-BE/internal/db/sqlc"
	"github.com/rs/zerolog/log"
)

type createSellerProfileRequest struct {
	UserID   string `json:"user_id" binding:"required"`
	ShopName string `json:"shop_name" binding:"required"`
}

//	@Summary		Create a seller profile
//	@Description	Creates a new seller profile
//	@Tags			seller profile
//	@Accept			json
//	@Produce		json
//	@Param			request	body		createSellerProfileRequest	true	"Seller profile creation request"
//	@Success		201		{object}	db.SellerProfile			"Seller profile created successfully"
//	@Failure		400		"Invalid request format"
//	@Failure		409		"Seller profile already exists"
//	@Failure		500		"Internal server error"
//	@Router			/seller/profile [post]
func (server *Server) createSellerProfile(c *gin.Context) {
	var req createSellerProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse(err))
		return
	}
	
	sellerProfile, err := server.dbStore.CreateSellerProfile(c, db.CreateSellerProfileParams{
		SellerID: req.UserID,
		ShopName: req.ShopName,
	})
	if err != nil {
		if pgErr := db.ErrorDescription(err); pgErr != nil {
			switch {
			case pgErr.Code == db.UniqueViolationCode && pgErr.ConstraintName == db.UniqueSellerProfileConstraint:
				err = errors.New("seller profile already exists")
				c.JSON(http.StatusConflict, errorResponse(err))
				return
			}
			
			log.Err(err).Msg("failed to create user")
			c.JSON(http.StatusInternalServerError, errorResponse(err))
			return
		}
	}
	
	c.JSON(http.StatusCreated, sellerProfile)
}
