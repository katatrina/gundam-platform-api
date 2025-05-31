package api

import (
	"errors"
	"fmt"
	"net/http"
	"time"
	
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	db "github.com/katatrina/gundam-BE/internal/db/sqlc"
	"github.com/katatrina/gundam-BE/internal/validator"
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
		Valid:                status != "",
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
//	@Description	ModeratorID rejects an auction request with a reason.
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
	
	err = server.taskDistributor.DistributeTaskSendNotification(c.Request.Context(), &worker.PayloadSendNotification{
		RecipientID: rejectedRequest.SellerID,
		Title:       "Yêu cầu đấu giá bị từ chối",
		Message:     fmt.Sprintf("Yêu cầu đấu giá của bạn cho gundam \"%s\" đã bị từ chối. Lý do: %s.", rejectedRequest.GundamSnapshot.Name, req.Reason),
		Type:        "auction_request",
		ReferenceID: rejectedRequest.ID.String(),
	}, opts...)
	if err != nil {
		log.Err(err).Msgf("failed to send notification to user ID %s", rejectedRequest.SellerID)
	}
	
	c.JSON(http.StatusOK, rejectedRequest)
}

//	@Summary		Approve an auction request by moderator
//	@Description	ModeratorID approves an auction request and schedules the auction.
//	@Tags			moderator
//	@Produce		json
//	@Security		accessToken
//	@Param			requestID	path		string								true	"Auction Request ID (UUID format)"
//	@Success		200			{object}	db.ApproveAuctionRequestTxResult	"Result of the approval transaction"
//	@Router			/mod/auction-requests/{requestID}/approve [patch]
func (server *Server) approveAuctionRequest(c *gin.Context) {
	user := c.MustGet(moderatorPayloadKey).(*db.User)
	
	requestID, err := uuid.Parse(c.Param("requestID"))
	if err != nil {
		err = fmt.Errorf("invalid request ID format: %w", err)
		c.JSON(http.StatusBadRequest, errorResponse(err))
		return
	}
	
	// Get auction request to validate
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
	
	// Business validation
	// 1. Check status
	if auctionRequest.Status != db.AuctionRequestStatusPending {
		err = fmt.Errorf("cannot approve auction request with status '%s'. Only 'pending' requests can be approved", auctionRequest.Status)
		c.JSON(http.StatusConflict, errorResponse(err))
		return
	}
	
	// 2. Validate timing
	if err = validator.ValidateAuctionTimesForApproval(auctionRequest.StartTime); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse(err))
		return
	}
	
	// 3. Validate gundam if needed
	if auctionRequest.GundamID != nil {
		gundam, err := server.dbStore.GetGundamByID(c.Request.Context(), *auctionRequest.GundamID)
		if err != nil && !errors.Is(err, db.ErrRecordNotFound) {
			c.JSON(http.StatusInternalServerError, errorResponse(err))
			return
		}
		
		if err == nil {
			if gundam.Status != db.GundamStatusPendingAuctionApproval {
				err = fmt.Errorf("gundam status is %s, expected %s",
					gundam.Status, db.GundamStatusPendingAuctionApproval)
				c.JSON(http.StatusConflict, errorResponse(err))
				return
			}
			
			if gundam.OwnerID != auctionRequest.SellerID {
				err = fmt.Errorf("gundam owner mismatch")
				c.JSON(http.StatusForbidden, errorResponse(err))
				return
			}
		}
	}
	
	// Now transaction only does data manipulation
	result, err := server.dbStore.ApproveAuctionRequestTx(c.Request.Context(), db.ApproveAuctionRequestTxParams{
		RequestID:  requestID,
		ApprovedBy: user.ID,
		AfterAuctionCreated: func(auction db.Auction) error {
			// 1. Schedule start auction task với custom task ID
			startTaskPayload := &worker.PayloadStartAuction{
				AuctionID: auction.ID,
			}
			
			startTaskID := fmt.Sprintf("auction:start:%s", auction.ID)
			startOpts := []asynq.Option{
				asynq.ProcessAt(auction.StartTime),
				asynq.TaskID(startTaskID),
				asynq.MaxRetry(3),
				asynq.Queue(worker.QueueCritical),
			}
			
			if err = server.taskDistributor.DistributeTaskStartAuction(c.Request.Context(), startTaskPayload, startOpts...); err != nil {
				return fmt.Errorf("failed to schedule start auction task: %w", err)
			}
			
			// 2. Schedule end auction task với custom task ID
			endTaskPayload := &worker.PayloadEndAuction{
				AuctionID: auction.ID,
			}
			
			endTaskID := fmt.Sprintf("auction:end:%s", auction.ID)
			endOpts := []asynq.Option{
				asynq.ProcessAt(auction.EndTime),
				asynq.TaskID(endTaskID),
				asynq.MaxRetry(3),
				asynq.Queue(worker.QueueCritical),
			}
			
			if err = server.taskDistributor.DistributeTaskEndAuction(c.Request.Context(), endTaskPayload, endOpts...); err != nil {
				return fmt.Errorf("failed to schedule end auction task: %w", err)
			}
			
			return nil
		},
	})
	if err != nil {
		log.Error().
			Err(err).
			Str("request_id", requestID.String()).
			Str("moderator_id", user.ID).
			Msg("failed to approve auction request")
		c.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	// Send notification to the seller about approval
	opts := []asynq.Option{
		asynq.MaxRetry(3),
		asynq.Queue(worker.QueueCritical),
	}
	
	// Nên handle error khi load timezone
	var message string
	gundamName := result.CreatedAuction.GundamSnapshot.Name
	loc, err := time.LoadLocation("Asia/Ho_Chi_Minh")
	if err != nil {
		// Fallback to UTC or default format
		message = fmt.Sprintf("Yêu cầu đấu giá cho gundam \"%s\" của bạn đã được chấp thuận. Đấu giá sẽ bắt đầu vào lúc %s.",
			gundamName, result.CreatedAuction.StartTime.Format("15:04 02/01/2006"))
	} else {
		auctionStartTimeVN := result.CreatedAuction.StartTime.In(loc)
		message = fmt.Sprintf("Yêu cầu đấu giá cho gundam \"%s\" của bạn đã được chấp thuận. Đấu giá sẽ bắt đầu vào lúc %s (giờ Việt Nam).",
			gundamName, auctionStartTimeVN.Format("15:04 02/01/2006"))
	}
	
	err = server.taskDistributor.DistributeTaskSendNotification(c.Request.Context(), &worker.PayloadSendNotification{
		RecipientID: result.CreatedAuction.SellerID,
		Title:       "Yêu cầu đấu giá đã được chấp thuận",
		Message:     message,
		Type:        "auction_request",
		ReferenceID: result.UpdatedRequest.ID.String(),
	}, opts...)
	if err != nil {
		log.Err(err).Msgf("failed to send notification to user ID %s", result.CreatedAuction.SellerID)
	}
	
	c.JSON(http.StatusOK, result)
}

type updateAuctionDetailsByModeratorBody struct {
	StartTime *time.Time `json:"start_time"`
	EndTime   *time.Time `json:"end_time"`
}

//	@Summary		Update auction details by moderator
//	@Description	ModeratorID can update auction start and end times.
//	@Tags			moderator
//	@Accept			json
//	@Produce		json
//	@Security		accessToken
//	@Param			auctionID	path		string								true	"Auction ID (UUID format)"
//	@Param			request		body		updateAuctionDetailsByModeratorBody	true	"Updated auction details"
//	@Success		200			{object}	db.UpdateAuctionByModeratorTxResult	"Updated auction details"
//	@Router			/mod/auctions/{auctionID} [patch]
func (server *Server) updateAuctionDetailsByModerator(c *gin.Context) {
	_ = c.MustGet(moderatorPayloadKey).(*db.User)
	
	var req updateAuctionDetailsByModeratorBody
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse(err))
		return
	}
	
	auctionID, err := uuid.Parse(c.Param("auctionID"))
	if err != nil {
		err = fmt.Errorf("invalid auction ID format: %w", err)
		c.JSON(http.StatusBadRequest, errorResponse(err))
		return
	}
	
	_, err = server.dbStore.GetAuctionByID(c.Request.Context(), auctionID)
	if err != nil {
		if errors.Is(err, db.ErrRecordNotFound) {
			err = fmt.Errorf("auction ID %s not found", auctionID)
			c.JSON(http.StatusNotFound, errorResponse(err))
			return
		}
		
		c.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	result, err := server.dbStore.UpdateAuctionByModeratorTx(c.Request.Context(), db.UpdateAuctionByModeratorTxParams{
		AuctionID: auctionID,
		StartTime: req.StartTime,
		EndTime:   req.EndTime,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	c.JSON(http.StatusOK, result)
}

type completeWithdrawalRequestRequest struct {
	// Mã giao dịch từ ngân hàng để hoàn tất yêu cầu rút tiền
	TransactionReference string `json:"transaction_reference" binding:"required"`
}

//	@Summary		Complete withdrawal request
//	@Description	Complete a withdrawal request with transaction reference from bank.
//	@Description	The request must be in pending or approved status.
//	@Tags			moderator
//	@Accept			json
//	@Produce		json
//	@Security		accessToken
//	@Param			requestID	path		string								true	"Withdrawal Request ID"
//	@Param			body		body		completeWithdrawalRequestRequest	true	"Request body"
//	@Success		200			{object}	db.WithdrawalRequestDetails			"Updated withdrawal request details"
//	@Router			/mod/withdrawal-requests/{requestID}/complete [patch]
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

type rejectWithdrawalRequestRequest struct {
	// Lý do từ chối yêu cầu rút tiền của moderator
	Reason string `json:"reason" binding:"required"`
}

//	@Summary		Reject withdrawal request
//	@Description	Reject a withdrawal request with reason from moderator. The request must be in pending status.
//	@Tags			moderator 
//	@Accept			json
//	@Produce		json
//	@Security		accessToken
//	@Param			requestID	path		string							true	"Withdrawal Request ID"
//	@Param			body		body		rejectWithdrawalRequestRequest	true	"Request body"
//	@Success		200			{object}	db.WithdrawalRequestDetails		"Updated withdrawal request details"
//	@Router			/mod/withdrawal-requests/{requestID}/reject [patch]
func (server *Server) rejectWithdrawalRequest(c *gin.Context) {
	user := c.MustGet(moderatorPayloadKey).(*db.User)
	
	var req rejectWithdrawalRequestRequest
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
	if request.WithdrawalRequest.Status != db.WithdrawalRequestStatusPending {
		err = fmt.Errorf("can only reject request with status pending, current request status is %s", request.WithdrawalRequest.Status)
		c.JSON(http.StatusUnprocessableEntity, errorResponse(err))
		return
	}
	
	// Execute transaction to reject withdrawal request
	arg := db.RejectWithdrawalRequestTxParams{
		WithdrawalRequest: request.WithdrawalRequest,
		ModeratorID:       user.ID,
		Reason:            req.Reason,
	}
	updatedRequest, err := server.dbStore.RejectWithdrawalRequestTx(c, arg)
	if err != nil {
		err = fmt.Errorf("failed to reject withdrawal request ID %s: %w", requestID, err)
		c.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	// Thông báo đến người dùng về việc yêu cầu rút tiền đã bị từ chối
	go func() {
		// Gửi thông báo trên nền tảng
		err = server.taskDistributor.DistributeTaskSendNotification(c.Request.Context(), &worker.PayloadSendNotification{
			RecipientID: updatedRequest.UserID,
			Title:       "Yêu cầu rút tiền bị từ chối",
			Message:     fmt.Sprintf("Yêu cầu rút tiền của bạn đã bị từ chối. Lý do: %s", req.Reason),
			Type:        "withdrawal_request",
			ReferenceID: updatedRequest.ID.String(),
		})
		if err != nil {
			log.Err(err).Msgf("failed to distribute task notification for reject request ID %s", request.WithdrawalRequest.ID.String())
		}
		
		// TODO: Gửi qua email
	}()
	
	resp := db.NewWithdrawalRequestDetails(updatedRequest, request.UserBankAccount)
	
	c.JSON(http.StatusOK, resp)
}
