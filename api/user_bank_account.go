package api

import (
	"errors"
	"fmt"
	"net/http"
	"time"
	
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	db "github.com/katatrina/gundam-BE/internal/db/sqlc"
	"github.com/katatrina/gundam-BE/internal/token"
	"github.com/katatrina/gundam-BE/internal/util"
)

type addBankAccountRequest struct {
	AccountName   string `json:"account_name" binding:"required"`
	AccountNumber string `json:"account_number" binding:"required"`
	BankCode      string `json:"bank_code" binding:"required"`
	BankName      string `json:"bank_name" binding:"required"`
	BankShortName string `json:"bank_short_name" binding:"required"`
}

//	@Summary		Add bank account
//	@Description	Add a new bank account for the authenticated user (for withdrawals)
//	@Tags			wallet
//	@Accept			json
//	@Produce		json
//	@Security		accessToken
//	@Param			request	body		addBankAccountRequest	true	"Bank account information"
//	@Success		201		{object}	db.UserBankAccount		"Successfully added bank account"
//	@Router			/users/me/bank-accounts [post]
func (server *Server) addBankAccount(c *gin.Context) {
	authPayload := c.MustGet(authorizationPayloadKey).(*token.Payload)
	userID := authPayload.Subject
	
	var req addBankAccountRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse(err))
		return
	}
	
	accountID, err := uuid.NewV7()
	if err != nil {
		err = fmt.Errorf("failed to generate account ID: %w", err)
		c.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	arg := db.CreateUserBankAccountParams{
		ID:            accountID,
		UserID:        userID,
		AccountName:   req.AccountName,
		AccountNumber: req.AccountNumber,
		BankCode:      req.BankCode,
		BankName:      req.BankName,
		BankShortName: req.BankShortName,
	}
	
	account, err := server.dbStore.CreateUserBankAccount(c, arg)
	if err != nil {
		c.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	c.JSON(http.StatusOK, account)
}

//	@Summary		List user bank accounts
//	@Description	Get all bank accounts for the authenticated user (for withdrawals)
//	@Tags			wallet
//	@Produce		json
//	@Security		accessToken
//	@Success		200	{array}	db.UserBankAccount	"List of user bank accounts"
//	@Router			/users/me/bank-accounts [get]
func (server *Server) listUserBankAccounts(c *gin.Context) {
	authPayload := c.MustGet(authorizationPayloadKey).(*token.Payload)
	userID := authPayload.Subject
	
	accounts, err := server.dbStore.ListUserBankAccounts(c, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	c.JSON(http.StatusOK, accounts)
}

//	@Summary		Delete user bank account
//	@Description	Delete a bank account for the authenticated user (for withdrawals)
//	@Tags			wallet
//	@Produce		json
//	@Security		accessToken
//	@Param			accountID	path		string				true	"Bank account ID"
//	@Success		200			{object}	db.UserBankAccount	"Successfully deleted bank account"
//	@Router			/users/me/bank-accounts/{accountID} [delete]
func (server *Server) deleteUserBankAccount(c *gin.Context) {
	authPayload := c.MustGet(authorizationPayloadKey).(*token.Payload)
	userID := authPayload.Subject
	
	accountID, err := uuid.Parse(c.Param("accountID"))
	if err != nil {
		err = fmt.Errorf("invalid account ID: %w", err)
		c.JSON(http.StatusBadRequest, errorResponse(err))
		return
	}
	
	deletedAccount, err := server.dbStore.UpdateUserBankAccount(c.Request.Context(), db.UpdateUserBankAccountParams{
		ID:        accountID,
		UserID:    userID,
		DeletedAt: util.TimePointer(time.Now()),
	})
	if err != nil {
		if errors.Is(err, db.ErrRecordNotFound) {
			err = fmt.Errorf("bank account ID %s not found for user ID %s", accountID, userID)
			c.JSON(http.StatusNotFound, errorResponse(err))
			return
		}
		
		err = fmt.Errorf("failed to delete bank account ID %s for user ID %s: %w", accountID, userID, err)
		c.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	c.JSON(http.StatusOK, deletedAccount)
}
