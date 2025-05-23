package api

import (
	"context"
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
//	@Param			status	query	string				false	"Filter by status"	Enums(scheduled, active)
//	@Success		200		{array}	db.AuctionDetails	"List of auctions"
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
	
	// Lấy danh sách các phiên đấu giá, có lọc theo trạng thái, có sắp xếp hợp lý theo trạng thái
	auctions, err := server.dbStore.ListAuctions(c, db.NullAuctionStatus{
		AuctionStatus: status,
		Valid:         status != "",
	})
	if err != nil {
		err = fmt.Errorf("failed to list auctions: %w", err)
		c.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	// Danh sách thông tin đấu giá chi tiết
	var result []db.AuctionDetails
	for _, auction := range auctions {
		// Lấy danh sách người tham gia đấu giá (sắp xếp theo thời gian tham gia gần nhất)
		participants, err := server.dbStore.ListAuctionParticipants(c, auction.ID)
		if err != nil {
			err = fmt.Errorf("failed to list auction participants: %w", err)
			c.JSON(http.StatusInternalServerError, errorResponse(err))
			return
		}
		
		// Lấy danh sách giá đấu đã đặt (sắp xếp theo thời gian đặt giá gần nhất)
		bids, err := server.dbStore.ListAuctionBids(c, &auction.ID)
		if err != nil {
			err = fmt.Errorf("failed to list auction bids: %w", err)
			c.JSON(http.StatusInternalServerError, errorResponse(err))
			return
		}
		
		result = append(result, db.AuctionDetails{
			Auction:             auction,
			AuctionParticipants: participants,
			AuctionBids:         bids,
		})
	}
	
	c.JSON(http.StatusOK, result)
}

//	@Summary		Get auction details
//	@Description	Retrieves details of a specific auction by its ID.
//	@Tags			auctions
//	@Produce		json
//	@Param			auctionID	path		string				true	"ID of the auction"
//	@Success		200			{object}	db.AuctionDetails	"Details of the auction"
//	@Router			/auctions/{auctionID} [get]
func (server *Server) getAuctionDetails(c *gin.Context) {
	auctionID, err := uuid.Parse(c.Param("auctionID"))
	if err != nil {
		err = fmt.Errorf("invalid auction ID: %w", err)
		c.JSON(http.StatusBadRequest, errorResponse(err))
		return
	}
	
	// Lấy thông tin phiên đấu giá
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
	
	// Thông tin chi tiết phiên đấu giá
	var auctionDetails db.AuctionDetails
	auctionDetails.Auction = auction
	
	// Lấy danh sách người tham gia đấu giá (sắp xếp theo thời gian tham gia gần nhất)
	participants, err := server.dbStore.ListAuctionParticipants(c, auction.ID)
	if err != nil {
		err = fmt.Errorf("failed to list auction participants: %w", err)
		c.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	auctionDetails.AuctionParticipants = participants
	
	// Lấy danh sách giá đấu đã đặt (sắp xếp theo thời gian đặt giá gần nhất)
	bids, err := server.dbStore.ListAuctionBids(c, &auction.ID)
	if err != nil {
		err = fmt.Errorf("failed to list auction bids: %w", err)
		c.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	auctionDetails.AuctionBids = bids
	
	c.JSON(http.StatusOK, auctionDetails)
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
			c.JSON(http.StatusUnprocessableEntity, errorResponse(fmt.Errorf("insufficient balance to participate in the auction")))
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

//	@Summary		List user participated auctions
//	@Description	Retrieves a list of auctions the user has participated in.
//	@Tags			auctions
//	@Produce		json
//	@Success		200	{array}	db.ListUserParticipatedAuctionsRow	"List of participated auctions"
//	@Security		accessToken
//	@Router			/users/me/auctions [get]
func (server *Server) listUserParticipatedAuctions(c *gin.Context) {
	authPayload := c.MustGet(authorizationPayloadKey).(*token.Payload)
	userID := authPayload.Subject
	
	// Lấy danh sách các phiên đấu giá mà người dùng đã tham gia
	rows, err := server.dbStore.ListUserParticipatedAuctions(c, userID)
	if err != nil {
		err = fmt.Errorf("failed to list participated rows: %w", err)
		c.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	c.JSON(http.StatusOK, rows)
}

//	@Summary		List user bids
//	@Description	Retrieves a list of bids made by the user in a specific auction.
//	@Tags			auctions
//	@Produce		json
//	@Param			auctionID	query	string			true	"Auction ID"
//	@Success		200			{array}	db.AuctionBid	"List of user bids"
//	@Security		accessToken
//	@Router			/users/me/auctions/:auctionID/bids [get]
func (server *Server) listUserBids(c *gin.Context) {
	authPayload := c.MustGet(authorizationPayloadKey).(*token.Payload)
	userID := authPayload.Subject
	
	auctionIDStr := c.Query("auctionID")
	auctionID, err := uuid.Parse(auctionIDStr)
	if err != nil {
		err = fmt.Errorf("invalid auction ID format: %w", err)
		c.JSON(http.StatusBadRequest, errorResponse(err))
		return
	}
	
	auctionBids, err := server.dbStore.ListUserAuctionBids(c.Request.Context(), db.ListUserAuctionBidsParams{
		BidderID:  &userID,
		AuctionID: &auctionID,
	})
	if err != nil {
		err = fmt.Errorf("failed to list user auction bids: %w", err)
		c.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	c.JSON(http.StatusOK, auctionBids)
}

type payAuctionWinningBidRequest struct {
	UserAddressID        int64     `json:"user_address_id" binding:"required"`
	DeliveryFee          int64     `json:"delivery_fee" binding:"required"`
	ExpectedDeliveryTime time.Time `json:"expected_delivery_time" binding:"required"`
	Note                 *string   `json:"note"`
}

//	@Summary		Pay for winning auction bid
//	@Description	Pay the remaining amount after deposit for a winning auction.
//	@Tags			auctions
//	@Accept			json
//	@Produce		json
//	@Param			auctionID	path		string							true	"Auction ID"
//	@Param			request		body		payAuctionWinningBidRequest		true	"Payment request"
//	@Success		200			{object}	db.PayAuctionWinningBidTxResult	"Payment result"
//	@Security		accessToken
//	@Router			/users/me/auctions/{auctionID}/payment [post]
func (server *Server) payAuctionWinningBid(c *gin.Context) {
	// 1. Lấy thông tin người dùng từ token
	authPayload := c.MustGet(authorizationPayloadKey).(*token.Payload)
	userID := authPayload.Subject
	
	// 2. Parse auction ID
	auctionID, err := uuid.Parse(c.Param("auctionID"))
	if err != nil {
		err = fmt.Errorf("invalid auction ID format: %w", err)
		c.JSON(http.StatusBadRequest, errorResponse(err))
		return
	}
	
	// 3. Parse request body
	var req payAuctionWinningBidRequest
	if err = c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse(err))
		return
	}
	
	// Parse expected delivery time
	expectedDeliveryTime, err := time.Parse(time.RFC3339, req.ExpectedDeliveryTime.String())
	if err != nil {
		err = fmt.Errorf("invalid expected_delivery_time format: %w", err)
		c.JSON(http.StatusBadRequest, errorResponse(err))
		return
	}
	
	// 4. Kiểm tra auction
	auction, err := server.dbStore.GetAuctionByID(c.Request.Context(), auctionID)
	if err != nil {
		if errors.Is(err, db.ErrRecordNotFound) {
			err = fmt.Errorf("auction ID %s not found", auctionID)
			c.JSON(http.StatusNotFound, errorResponse(err))
			return
		}
		
		c.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	// 5. Kiểm tra auction status
	if auction.Status != db.AuctionStatusEnded {
		err = fmt.Errorf("auction ID %s is not in ended status, current status: %s", auction.ID, auction.Status)
		c.JSON(http.StatusUnprocessableEntity, errorResponse(err))
		return
	}
	
	// 6. Kiểm tra phiên đấu giá đã có người thắng chưa
	if auction.WinningBidID == nil {
		err = fmt.Errorf("auction ID %s has no winner", auction.ID)
		c.JSON(http.StatusUnprocessableEntity, errorResponse(err))
		return
	}
	
	// 7. Lấy thông tin lượt đấu giá thắng
	winningBid, err := server.dbStore.GetAuctionBidByID(c.Request.Context(), *auction.WinningBidID)
	if err != nil {
		if errors.Is(err, db.ErrRecordNotFound) {
			err = fmt.Errorf("winning bid ID %s not found for auction ID %s", *auction.WinningBidID, auction.ID)
			c.JSON(http.StatusNotFound, errorResponse(err))
			return
		}
		
		c.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	// 8. Kiểm tra người dùng có phải là người thắng không
	if winningBid.BidderID == nil || *winningBid.BidderID != userID {
		err = fmt.Errorf("user ID %s is not the winner of auction ID %s", userID, auction.ID)
		c.JSON(http.StatusForbidden, errorResponse(err))
		return
	}
	
	// 9. Kiểm tra thời hạn thanh toán
	if auction.WinnerPaymentDeadline != nil && time.Now().After(*auction.WinnerPaymentDeadline) {
		err = fmt.Errorf("payment deadline has passed for auction ID %s", auction.ID)
		c.JSON(http.StatusForbidden, errorResponse(err))
		return
	}
	
	// 10. Kiểm tra đã thanh toán chưa
	if auction.OrderID != nil {
		err = fmt.Errorf("auction ID %s has already been paid for", auction.ID)
		c.JSON(http.StatusUnprocessableEntity, errorResponse(err))
		return
	}
	
	// 11. Kiểm tra địa chỉ giao hàng
	toAddress, err := server.dbStore.GetUserAddressByID(c.Request.Context(), db.GetUserAddressByIDParams{
		ID:     req.UserAddressID,
		UserID: userID,
	})
	if err != nil {
		if errors.Is(err, db.ErrRecordNotFound) {
			err = fmt.Errorf("user address ID %d not found", req.UserAddressID)
			c.JSON(http.StatusNotFound, errorResponse(err))
			return
		}
		
		c.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	// Kiểm tra địa chỉ giao hàng có thuộc về người dùng không
	if toAddress.UserID != userID {
		err = fmt.Errorf("address ID does not belong to user ID %s", userID)
		c.JSON(http.StatusForbidden, errorResponse(err))
		return
	}
	
	// 12. Lấy thông tin tham gia đấu giá
	participant, err := server.dbStore.GetAuctionParticipantByUserID(c.Request.Context(), db.GetAuctionParticipantByUserIDParams{
		AuctionID: auction.ID,
		UserID:    userID,
	})
	if err != nil {
		if errors.Is(err, db.ErrRecordNotFound) {
			err = fmt.Errorf("user ID %s has not participated in auction ID %s", userID, auction.ID)
			c.JSON(http.StatusNotFound, errorResponse(err))
			return
		}
		
		c.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	// 13. Lấy thông tin người dùng
	user, err := server.dbStore.GetUserByID(c.Request.Context(), userID)
	if err != nil {
		if errors.Is(err, db.ErrRecordNotFound) {
			err = fmt.Errorf("user ID %s not found", userID)
			c.JSON(http.StatusNotFound, errorResponse(err))
			return
		}
		
		c.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	// 14. Thực hiện transaction thanh toán
	result, err := server.dbStore.PayAuctionWinningBidTx(c.Request.Context(), db.PayAuctionWinningBidTxParams{
		Auction:              auction,
		User:                 user,
		WinningBid:           winningBid,
		Participant:          participant,
		ToAddress:            toAddress,
		DeliveryFee:          req.DeliveryFee,
		ExpectedDeliveryTime: expectedDeliveryTime,
		Note:                 req.Note,
	})
	if err != nil {
		if errors.Is(err, db.ErrInsufficientBalance) {
			c.JSON(http.StatusUnprocessableEntity, errorResponse(err))
			return
		}
		
		c.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	// 15. Tạo thông báo cho người bán
	go func() {
		// Xử lý thông báo (không trả về lỗi)
		err = server.taskDistributor.DistributeTaskSendNotification(
			context.Background(),
			&worker.PayloadSendNotification{
				RecipientID: auction.SellerID,
				Title:       "Đã nhận thanh toán cho phiên đấu giá",
				Message:     fmt.Sprintf("Người thắng đã thanh toán %s cho phiên đấu giá. Vui lòng chuẩn bị đóng gói sản phẩm.", util.FormatVND(winningBid.Amount+req.DeliveryFee)),
				Type:        "auction_payment",
				ReferenceID: auction.ID.String(),
			},
		)
		if err != nil {
			log.Error().
				Err(err).
				Str("auction_id", auction.ID.String()).
				Str("seller_id", auction.SellerID).
				Msg("Failed to send notification to seller")
		}
	}()
	
	// 16. Trả về kết quả
	c.JSON(http.StatusOK, result)
}
