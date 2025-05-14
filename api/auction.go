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
	"github.com/katatrina/gundam-BE/internal/event"
	"github.com/katatrina/gundam-BE/internal/token"
	"github.com/katatrina/gundam-BE/internal/util"
	"github.com/katatrina/gundam-BE/internal/worker"
	"github.com/rs/zerolog/log"
)

//	@Summary		Get platform auctions
//	@Description	Retrieves upcoming and ongoing auctions from the platform.
//	@Tags			auctions
//	@Produce		json
//	@Param			status	query	string		false	"Filter by status"	Enums(scheduled, active)
//	@Success		200		{array}	db.Auction	"List of auctions"
//	@Router			/auctions [get]
func (server *Server) listAuctions(c *gin.Context) {
	status := db.AuctionStatus(c.Query("status"))
	if status != "" {
		if status != db.AuctionStatusScheduled && status != db.AuctionStatusActive {
			err := fmt.Errorf("invalid status: %s, allowed statuses: [scheduled, active]", status)
			c.JSON(http.StatusBadRequest, errorResponse(err))
			return
		}
	}
	
	auctions, err := server.dbStore.ListAuctions(c, db.NullAuctionStatus{
		AuctionStatus: status,
		Valid:         status != "",
	})
	if err != nil {
		err = fmt.Errorf("failed to list auctions: %w", err)
		c.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	c.JSON(http.StatusOK, auctions)
}

//	@Summary		Get auction details
//	@Description	Retrieves details of a specific auction by its ID.
//	@Tags			auctions
//	@Produce		json
//	@Param			auctionID	path		string		true	"ID of the auction"
//	@Success		200			{object}	db.Auction	"Details of the auction"
//	@Router			/auctions/{auctionID} [get]
func (server *Server) getAuctionDetails(c *gin.Context) {
	auctionID, err := uuid.Parse(c.Param("auctionID"))
	if err != nil {
		err = fmt.Errorf("invalid auction ID: %w", err)
		c.JSON(http.StatusBadRequest, errorResponse(err))
		return
	}
	
	auction, err := server.dbStore.GetAuctionByID(c, auctionID)
	if err != nil {
		if errors.Is(err, db.ErrRecordNotFound) {
			err = fmt.Errorf("auction ID %s not found", auctionID)
			c.JSON(http.StatusNotFound, errorResponse(err))
			return
		}
		
		err = fmt.Errorf("failed to get auction details: %w", err)
		c.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	c.JSON(http.StatusOK, auction)
}

//	@Summary		Participate in an auction
//	@Description	User deposits money to participate in an active auction.
//	@Tags			auctions
//	@Produce		json
//	@Param			auctionID	path		string							true	"Auction ID"
//	@Success		200			{object}	db.ParticipateInAuctionTxResult	"Participation result"
//	@Security		accessToken
//	@Router			/users/me/auctions/{auctionID}/participate [post]
func (server *Server) participateInAuction(c *gin.Context) {
	// Lấy thông tin người dùng từ token
	authPayload := c.MustGet(authorizationPayloadKey).(*token.Payload)
	userID := authPayload.Subject
	
	// Parse auction ID từ URL
	auctionIDStr := c.Param("auctionID")
	auctionID, err := uuid.Parse(auctionIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, errorResponse(fmt.Errorf("invalid auction ID format")))
		return
	}
	
	// 1. Kiểm tra business logic
	// Lấy auction để kiểm tra các điều kiện
	auction, err := server.dbStore.GetAuctionByID(c, auctionID)
	if err != nil {
		if errors.Is(err, db.ErrRecordNotFound) {
			err = fmt.Errorf("auction ID %s not found", auctionIDStr)
			c.JSON(http.StatusNotFound, errorResponse(err))
			return
		}
		
		err = fmt.Errorf("failed to get auction details: %w", err)
		c.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	// Kiểm tra trạng thái hợp lệ
	if auction.Status != db.AuctionStatusActive {
		err = fmt.Errorf("auction ID %s is not open for participation, current status: %s", auction.ID, auction.Status)
		c.JSON(http.StatusUnprocessableEntity, errorResponse(err))
		return
	}
	
	// Kiểm tra người bán không thể tham gia đấu giá của chính mình
	if auction.SellerID == userID {
		err = fmt.Errorf("user cannot participate in their own auction")
		c.JSON(http.StatusUnprocessableEntity, errorResponse(err))
		return
	}
	
	// Kiểm tra thời gian đấu giá
	now := time.Now()
	if now.Before(auction.StartTime) {
		err = fmt.Errorf("auction ID %s has not yet started, starts at %s", auction.ID, auction.StartTime)
		c.JSON(http.StatusUnprocessableEntity, errorResponse(err))
		return
	}
	
	if now.After(auction.EndTime) {
		err = fmt.Errorf("auction ID %s has already ended at %s", auction.ID, auction.EndTime)
		c.JSON(http.StatusUnprocessableEntity, errorResponse(err))
		return
	}
	
	// Kiểm tra người dùng đã tham gia hay chưa
	hasParticipated, err := server.dbStore.CheckUserParticipation(c, db.CheckUserParticipationParams{
		AuctionID: auction.ID,
		UserID:    userID,
	})
	if err != nil {
		err = fmt.Errorf("failed to check user participation: %w", err)
		c.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	if hasParticipated {
		err = fmt.Errorf("user ID %s has already participated in auction ID %s", userID, auction.ID)
		c.JSON(http.StatusConflict, errorResponse(err))
		return
	}
	
	// Kiểm tra số dư ví
	wallet, err := server.dbStore.GetWalletByUserID(c, userID)
	if err != nil {
		if errors.Is(err, db.ErrRecordNotFound) {
			err = fmt.Errorf("wallet not found for user ID %s", userID)
			c.JSON(http.StatusNotFound, errorResponse(err))
			return
		}
		
		err = fmt.Errorf("failed to get wallet: %w", err)
		c.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	if wallet.Balance < auction.DepositAmount {
		err = fmt.Errorf("insufficient balance: required %s, available %s", util.FormatMoney(auction.DepositAmount), util.FormatMoney(wallet.Balance))
		c.JSON(http.StatusUnprocessableEntity, errorResponse(err))
		return
	}
	
	// 2. Thực hiện transaction
	result, err := server.dbStore.ParticipateInAuctionTx(c, db.ParticipateInAuctionTxParams{
		UserID:  userID,
		Auction: auction,
		Wallet:  wallet,
	})
	if err != nil {
		// Phân loại lỗi để trả về status code phù hợp
		switch {
		case errors.Is(err, db.ErrDuplicateParticipation):
			c.JSON(http.StatusConflict, errorResponse(fmt.Errorf("user has already participated in the auction")))
		case errors.Is(err, db.ErrInsufficientBalance):
			c.JSON(http.StatusUnprocessableEntity, errorResponse(fmt.Errorf("insufficient balance")))
		default:
			c.JSON(http.StatusInternalServerError, errorResponse(fmt.Errorf("failed to participate in auction: %w", err)))
		}
		
		return
	}
	
	// Lấy thông tin người dùng tham gia đấu giá
	user, err := server.dbStore.GetUserByID(c, userID)
	if err != nil {
		if errors.Is(err, db.ErrRecordNotFound) {
			err = fmt.Errorf("user ID %s not found", userID)
			c.JSON(http.StatusNotFound, errorResponse(err))
			return
		}
		
		err = fmt.Errorf("failed to get user details: %w", err)
		c.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	// Gửi thông tin sự kiện tới client qua SSE
	topic := fmt.Sprintf("auction:%s", auctionID.String())
	server.eventSender.Broadcast(event.Event{
		Topic: topic,
		Type:  event.EventTypeNewParticipant,
		Data: map[string]interface{}{
			"auction_id":         auctionID.String(),
			"total_participants": result.Auction.TotalParticipants,
			"user":               user,
			"timestamp":          result.AuctionParticipant.CreatedAt,
		},
	})
	
	opts := []asynq.Option{
		asynq.MaxRetry(3),
		asynq.Queue(worker.QueueCritical),
	}
	
	// Gửi thông báo cho người bán về việc có người tham gia mới (hibiken/asynq + Firestore database)
	err = server.taskDistributor.DistributeTaskSendNotification(c.Request.Context(), &worker.PayloadSendNotification{
		RecipientID: auction.SellerID,
		Title:       "Có người tham gia đấu giá",
		Message:     fmt.Sprintf("Có người đã xác nhận tham gia phiên đấu giá %s của bạn.", auction.GundamSnapshot.Name),
		Type:        "auction_participant",
		ReferenceID: auction.ID.String(),
	}, opts...)
	if err != nil {
		log.Err(err).
			Str("recipient_id", auction.SellerID).
			Str("auction_id", auction.ID.String()).
			Msg("failed to distribute task to send notification")
	}
	
	c.JSON(http.StatusOK, result)
}
