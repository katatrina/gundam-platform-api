package api

import (
	"errors"
	"fmt"
	"net/http"
	
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	db "github.com/katatrina/gundam-BE/internal/db/sqlc"
	"github.com/katatrina/gundam-BE/internal/worker"
	"github.com/rs/zerolog/log"
)

//	@Summary		List all auction requests for moderator
//	@Description	Get a list of all auction requests with optional status filter.
//	@Tags			moderator
//	@Produce		json
//	@Security		accessToken
//	@Param			status	query	string				false	"Filter by auction request status"	Enums(pending,approved,rejected)
//	@Success		200		{array}	db.AuctionRequest	"List of auction requests"
//	@Router			/mod/auction-requests [get]
func (server *Server) listAuctionRequestsForModerator(c *gin.Context) {
	status := c.Query("status")
	if status != "" {
		if err := db.IsValidAuctionRequestStatus(status); err != nil {
			c.JSON(http.StatusBadRequest, errorResponse(err))
			return
		}
	}
	
	auctionRequests, err := server.dbStore.ListAuctionRequests(c.Request.Context(), db.NullAuctionRequestStatus{
		AuctionRequestStatus: db.AuctionRequestStatus(status),
		Valid:                true,
	})
	if err != nil {
		log.Error().Err(err).Msg("failed to list auction requests")
		c.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	c.JSON(http.StatusOK, auctionRequests)
}

type rejectAuctionRequestBody struct {
	Reason string `json:"reason" binding:"required"`
}

//	@Summary		Reject an auction request by moderator
//	@Description	Moderator rejects an auction request with a reason.
//	@Tags			moderator
//	@Accept			json
//	@Produce		json
//	@Security		accessToken
//	@Param			requestID	path		string						true	"Auction Request ID (UUID format)"
//	@Param			request		body		rejectAuctionRequestBody	true	"Rejection reason"
//	@Success		200			{object}	db.AuctionRequest			"Rejected auction request"
//	@Router			/mod/auction-requests/{requestID}/reject [patch]
func (server *Server) rejectAuctionRequest(c *gin.Context) {
	user := c.MustGet(moderatorPayloadKey).(*db.User)
	
	requestID, err := uuid.Parse(c.Param("requestID"))
	if err != nil {
		err = fmt.Errorf("invalid request ID format: %w", err)
		c.JSON(http.StatusBadRequest, errorResponse(err))
		return
	}
	
	var req rejectAuctionRequestBody
	if err = c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse(err))
		return
	}
	
	// Get auction request to check current status
	auctionRequest, err := server.dbStore.GetAuctionRequestByID(c.Request.Context(), requestID)
	if err != nil {
		if errors.Is(err, db.ErrRecordNotFound) {
			err = fmt.Errorf("auction request ID %s not found", requestID)
			c.JSON(http.StatusNotFound, errorResponse(err))
			return
		}
		
		c.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	// Check if request can be rejected (only pending requests)
	if auctionRequest.Status != db.AuctionRequestStatusPending {
		err = fmt.Errorf("cannot reject auction request with status '%s'. Only 'pending' requests can be rejected", auctionRequest.Status)
		c.JSON(http.StatusConflict, errorResponse(err))
		return
	}
	
	// Reject the auction request in transaction
	rejectedRequest, err := server.dbStore.RejectAuctionRequestTx(c.Request.Context(), db.RejectAuctionRequestTxParams{
		RequestID:      requestID,
		RejectedBy:     user.ID,
		RejectedReason: req.Reason,
	})
	if err != nil {
		log.Error().Err(err).Msg("failed to reject auction request")
		c.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	// Send notification to the seller about rejection
	opts := []asynq.Option{
		asynq.MaxRetry(3),
		asynq.Queue(worker.QueueCritical),
	}
	
	// Lấy thông tin gudam đấu giá
	gundam, err := server.dbStore.GetGundamByID(c.Request.Context(), *rejectedRequest.GundamID)
	if err != nil {
		if errors.Is(err, db.ErrRecordNotFound) {
			err = fmt.Errorf("gundam ID %d not found", *rejectedRequest.GundamID)
			c.JSON(http.StatusNotFound, errorResponse(err))
			return
		}
		
		c.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	err = server.taskDistributor.DistributeTaskSendNotification(c.Request.Context(), &worker.PayloadSendNotification{
		RecipientID: rejectedRequest.SellerID,
		Title:       "Yêu cầu đấu giá bị từ chối",
		Message:     fmt.Sprintf("Yêu cầu đấu giá của bạn cho gundam \"%s\" đã bị từ chối. Lý do: %s.", gundam.Name, req.Reason),
		Type:        "auction_request",
		ReferenceID: rejectedRequest.ID.String(),
	}, opts...)
	if err != nil {
		log.Err(err).Msgf("failed to send notification to user ID %s", rejectedRequest.SellerID)
	}
	
	c.JSON(http.StatusOK, rejectedRequest)
}
