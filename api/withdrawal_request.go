package api

import (
	"errors"
	"fmt"
	"net/http"
	
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	db "github.com/katatrina/gundam-BE/internal/db/sqlc"
	"github.com/katatrina/gundam-BE/internal/worker"
	"github.com/rs/zerolog/log"
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

type completeWithdrawalRequestRequest struct {
	TransactionReference string `json:"transaction_reference" binding:"required"`
}

func (server *Server) completeWithdrawalRequest(c *gin.Context) {
	user := c.MustGet(moderatorPayloadKey).(*db.User)
	
	var req completeWithdrawalRequestRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse(err))
		return
	}
	
	requestID, err := uuid.Parse(c.Param("requestID"))
	if err != nil {
		err = fmt.Errorf("invalid request ID: %w", err)
		c.JSON(http.StatusBadRequest, errorResponse(err))
		return
	}
	
	request, err := server.dbStore.GetWithdrawalRequest(c.Request.Context(), requestID)
	if err != nil {
		if errors.Is(err, db.ErrRecordNotFound) {
			err = fmt.Errorf("withdrawal request ID %s not found: %w", requestID, err)
			c.JSON(http.StatusNotFound, errorResponse(err))
			return
		}
		
		err = fmt.Errorf("failed to get withdrawal request ID %s: %w", requestID, err)
		c.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	// Validate business logic
	if request.WithdrawalRequest.Status != db.WithdrawalRequestStatusPending && request.WithdrawalRequest.Status != db.WithdrawalRequestStatusApproved {
		err = fmt.Errorf("can only complete request with status pending or approved, current request status is %s", request.WithdrawalRequest.Status)
		c.JSON(http.StatusUnprocessableEntity, errorResponse(err))
		return
	}
	
	// Execute transaction to complete withdrawal request
	arg := db.CompleteWithdrawalRequestTxParams{
		WithdrawalRequest:    request.WithdrawalRequest,
		ModeratorID:          user.ID,
		TransactionReference: req.TransactionReference,
	}
	updatedRequest, err := server.dbStore.CompleteWithdrawalRequestTx(c, arg)
	if err != nil {
		err = fmt.Errorf("failed to complete withdrawal request ID %s: %w", requestID, err)
		c.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	// Thông báo đến người dùng về việc yêu cầu rút tiền đã hoàn tất
	go func() {
		// Gửi thông báo trên nền tảng
		err = server.taskDistributor.DistributeTaskSendNotification(c.Request.Context(), &worker.PayloadSendNotification{
			RecipientID: updatedRequest.UserID,
			Title:       "Yêu cầu rút tiền đã hoàn tất",
			Message:     fmt.Sprintf("Nền tảng đã hoàn tất yêu cầu rút tiền của bạn với mã giao dịch: %s", req.TransactionReference),
			Type:        "withdrawal_request",
			ReferenceID: updatedRequest.ID.String(),
		})
		if err != nil {
			log.Err(err).Msgf("failed to distribute task notification for withdrawal request ID %s", request.WithdrawalRequest.ID.String())
		}
		
		// TODO: Gửi qua email
	}()
	
	resp := db.NewWithdrawalRequestDetails(updatedRequest, request.UserBankAccount)
	
	c.JSON(http.StatusOK, resp)
}
