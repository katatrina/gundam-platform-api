package api

import (
	"errors"
	"net/http"
	
	"github.com/gin-gonic/gin"
	db "github.com/katatrina/gundam-BE/internal/db/sqlc"
)

//	@Summary		Get user wallet information details
//	@Description	Get user wallet information details
//	@Tags			wallet
//	@Accept			json
//	@Produce		json
//	@Security		accessToken
//	@Param			id	path		string		true	"User ID"
//	@Success		200	{object}	db.Wallet	"User wallet information"
//	@Failure		400	"Bad request"
//	@Failure		404	"User not found"
//	@Failure		500	"Internal server error"
//	@Router			/users/:id/wallet/ [get]
func (server *Server) getUserWallet(c *gin.Context) {
	userID := c.Param("id")
	if userID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "user id is required"})
		return
	}
	
	wallet, err := server.dbStore.GetWalletByUserID(c, userID)
	if err != nil {
		if errors.Is(err, db.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, errorResponse(db.ErrRecordNotFound))
			return
		}
		
		c.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	c.JSON(http.StatusOK, wallet)
}
