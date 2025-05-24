package api

import (
	"errors"
	"fmt"
	"net/http"
	
	"github.com/gin-gonic/gin"
	db "github.com/katatrina/gundam-BE/internal/db/sqlc"
	"github.com/katatrina/gundam-BE/internal/token"
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
//	@Router			/users/:id/wallet [get]
func (server *Server) getUserWallet(c *gin.Context) {
	userID := c.Param("id")
	if userID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "user id is required"})
		return
	}
	
	wallet, err := server.dbStore.GetWalletByUserID(c, userID)
	if err != nil {
		if errors.Is(err, db.ErrRecordNotFound) {
			err = fmt.Errorf("user ID %s not found", userID)
			c.JSON(http.StatusNotFound, errorResponse(err))
			return
		}
		
		c.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	c.JSON(http.StatusOK, wallet)
}

//	@Summary		List user wallet entries
//	@Description	List user wallet entries
//	@Tags			wallet
//	@Accept			json
//	@Produce		json
//	@Security		accessToken
//	@Param			status	query	string			false	"Filter by wallet entry status"
//	@Success		200		{array}	db.WalletEntry	"List of wallet entries"
//	@Router			/user/me/wallet/entries [get]
func (server *Server) listUserWalletEntries(c *gin.Context) {
	// Lấy thông tin người dùng từ token
	authPayload := c.MustGet(authorizationPayloadKey).(*token.Payload)
	userID := authPayload.Subject
	
	status := c.Query("status")
	if status != "" {
		if err := db.IsValidWalletEntryStatus(status); err != nil {
			c.JSON(http.StatusBadRequest, errorResponse(err))
			return
		}
	}
	
	entries, err := server.dbStore.ListUserWalletEntries(c, db.ListUserWalletEntriesParams{
		WalletID: userID,
		Status: db.NullWalletEntryStatus{
			WalletEntryStatus: db.WalletEntryStatus(status),
			Valid:             status != "",
		},
	})
	if err != nil {
		err = fmt.Errorf("failed to list wallet entries for user ID %s: %w", userID, err)
		c.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	c.JSON(http.StatusOK, entries)
}
