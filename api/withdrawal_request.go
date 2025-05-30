package api

import (
	"fmt"
	"net/http"
	
	"github.com/gin-gonic/gin"
	db "github.com/katatrina/gundam-BE/internal/db/sqlc"
)

//	@Summary		List withdrawal requests
//	@Description	List all withdrawal requests for moderators
//	@Tags			moderator
//	@Produce		json
//	@Security		accessToken
//	@Param			status	query	string						false	"Filter by withdrawal request status"	Enums(pending, approved, rejected, completed, rejected, canceled)
//	@Success		200		{array}	db.WithdrawalRequestDetails	"List of withdrawal requests"
//	@Router			/mod/withdrawal-requests [get]
func (server *Server) listWithdrawalRequests(c *gin.Context) {
	_ = c.MustGet(moderatorPayloadKey).(*db.User)
	
	status := c.Query("status")
	if status != "" {
		if err := db.IsValidWithdrawalRequestStatus(status); err != nil {
			c.JSON(http.StatusBadRequest, errorResponse(err))
			return
		}
	}
	
	requests, err := server.dbStore.ListWithdrawalRequests(c, db.NullWithdrawalRequestStatus{
		WithdrawalRequestStatus: db.WithdrawalRequestStatus(status),
		Valid:                   status != "",
	})
	if err != nil {
		err = fmt.Errorf("failed to list withdrawal requests: %w", err)
		c.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	resp := make([]db.WithdrawalRequestDetails, 0, len(requests))
	for _, req := range requests {
		resp = append(resp, db.NewWithdrawalRequestDetails(req.WithdrawalRequest, req.UserBankAccount))
	}
	
	c.JSON(http.StatusOK, resp)
}

func (server *Server) completeWithdrawalRequest(c *gin.Context) {}
