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

type placeBidRequest struct {
	Amount int64 `json:"amount" binding:"required"`
}

//	@Summary		Place a bid in an auction
//	@Description	User places a bid in an active auction they have participated in.
//	@Tags			auctions
//	@Produce		json
//	@Param			auctionID	path						string			true	"Auction ID"
//	@Param			request		body						placeBidRequest	true	"Request body containing bid amount"
//	@Success		200			"Successful bid placement"	db.PlaceBidTxResult
//	@Security		accessToken
//	@Router			/users/me/auctions/{auctionID}/bids [post]
func (server *Server) placeBid(c *gin.Context) {
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
	
	// Parse body request
	var req placeBidRequest
	if err = c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse(fmt.Errorf("invalid request body: %w", err)))
		return
	}
	
	// Validate bid amount
	if req.Amount <= 0 {
		err = fmt.Errorf("bid amount must be greater than 0, provided: %d", req.Amount)
		c.JSON(http.StatusBadRequest, errorResponse(err))
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
	
	// Kiểm tra trạng thái hợp lệ của phiên đấu giá
	if auction.Status != db.AuctionStatusActive {
		err = fmt.Errorf("auction ID %s status is not active, current status: %s", auction.ID, auction.Status)
		c.JSON(http.StatusUnprocessableEntity, errorResponse(err))
		return
	}
	
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
	
	// Kiểm tra người đặt giá đã tham gia phiên đấu giá hay chưa
	participant, err := server.dbStore.GetAuctionParticipantByUserID(c, db.GetAuctionParticipantByUserIDParams{
		AuctionID: auction.ID,
		UserID:    userID,
	})
	if err != nil {
		if errors.Is(err, db.ErrRecordNotFound) {
			err = fmt.Errorf("user has not participated in this auction yet")
			c.JSON(http.StatusUnprocessableEntity, errorResponse(err))
			return
		}
		
		err = fmt.Errorf("failed to check participation: %w", err)
		c.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	// Kiểm tra số tiền đặt giá phải lớn hơn (giá hiện tại + bước giá)
	minRequiredBid := auction.CurrentPrice + auction.BidIncrement
	if req.Amount < minRequiredBid {
		err = fmt.Errorf("bid amount must be at least %s, current price: %s, bid increment: %s",
			util.FormatMoney(minRequiredBid),
			util.FormatMoney(auction.CurrentPrice),
			util.FormatMoney(auction.BidIncrement))
		c.JSON(http.StatusUnprocessableEntity, errorResponse(err))
		return
	}
	
	// Kiểm tra người bán không thể đặt giá (thêm check dự phòng - dù đã kiểm tra ở participateInAuction)
	if auction.SellerID == userID {
		err = fmt.Errorf("seller cannot bid on their own auction")
		c.JSON(http.StatusUnprocessableEntity, errorResponse(err))
		return
	}
	
	// Kiểm tra số dư của người dùng
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
	
	// Kiểm tra người dùng có đủ số dư để đặt giá
	if wallet.Balance < req.Amount-auction.DepositAmount {
		err = fmt.Errorf("insufficient balance to place bid: required %s (excluding deposit), available %s",
			util.FormatMoney(req.Amount-auction.DepositAmount),
			util.FormatMoney(wallet.Balance))
		c.JSON(http.StatusUnprocessableEntity, errorResponse(err))
		return
	}
	
	// 2. Thực hiện transaction đặt giá
	result, err := server.dbStore.PlaceBidTx(c, db.PlaceBidTxParams{
		UserID:        userID,
		Auction:       auction,
		ParticipantID: participant.ID,
		Amount:        req.Amount,
		OnBuyNowFunc: func(auctionID uuid.UUID) error {
			// Lấy task ID từ key trong Redis
			taskID := fmt.Sprintf("auction:end:%s", auctionID.String())
			
			// Sử dụng taskInspector để xóa task
			return server.taskInspector.DeleteTask(c.Request.Context(), worker.QueueCritical, taskID)
		},
	})
	if err != nil {
		switch {
		case errors.Is(err, db.ErrAuctionEnded):
			c.JSON(http.StatusUnprocessableEntity, errorResponse(fmt.Errorf("auction has already ended")))
		case errors.Is(err, db.ErrBidTooLow):
			c.JSON(http.StatusUnprocessableEntity, errorResponse(fmt.Errorf("bid amount too low, someone may have placed a higher bid")))
		case errors.Is(err, db.ErrInsufficientBalance):
			c.JSON(http.StatusUnprocessableEntity, errorResponse(fmt.Errorf("insufficient balance to place bid")))
		default:
			c.JSON(http.StatusInternalServerError, errorResponse(fmt.Errorf("failed to place bid: %w", err)))
		}
		return
	}
	
	// Phân loại và xử lý transaction.
	// 3. Lấy thông tin người đặt giá
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
	
	// 4. Gửi thông báo realtime qua SSE
	topic := fmt.Sprintf("auction:%s", auctionID.String())
	
	// Gửi thông báo new_bid trước
	server.eventSender.Broadcast(event.Event{
		Topic: topic,
		Type:  event.EventTypeNewBid,
		Data: map[string]interface{}{
			"auction_id":    auctionID.String(),
			"current_price": result.Auction.CurrentPrice,
			"bid_id":        result.AuctionBid.ID.String(),
			"bid_amount":    result.AuctionBid.Amount,
			"bidder":        user,
			"timestamp":     result.AuctionBid.CreatedAt,
			"total_bids":    result.Auction.TotalBids,
		},
	})
	
	// Log thông tin đặt giá
	log.Info().
		Str("auction_id", auctionID.String()).
		Str("bidder_id", userID).
		Int64("amount", req.Amount).
		Bool("is_buy_now", result.IsBuyNow).
		Int32("total_bids", result.Auction.TotalBids).
		Msg("bid placed successfully")
	
	// 5. Gửi thông báo cho người bán và người vượt giá
	go func() {
		opts := []asynq.Option{
			asynq.MaxRetry(3),
			asynq.Queue(worker.QueueCritical),
		}
		
		// Thông báo cho người bán về lượt đặt giá mới (nếu không phải mua ngay)
		if !result.IsBuyNow {
			err = server.taskDistributor.DistributeTaskSendNotification(c.Request.Context(), &worker.PayloadSendNotification{
				RecipientID: result.Auction.SellerID,
				Title:       "Có lượt đặt giá mới",
				Message: fmt.Sprintf("Có người vừa đặt giá %s cho phiên đấu giá %s của bạn.",
					util.FormatMoney(req.Amount),
					result.Auction.GundamSnapshot.Name),
				Type:        "auction_new_bid",
				ReferenceID: result.Auction.ID.String(),
			}, opts...)
			if err != nil {
				log.Err(err).
					Str("recipient_id", result.Auction.SellerID).
					Str("auction_id", result.Auction.ID.String()).
					Msg("failed to send notification to seller")
			}
		}
		
		// Thông báo cho người bị vượt giá (nếu có và không phải mua ngay)
		if result.PreviousBidder != nil && result.PreviousBidder.ID != userID && !result.IsBuyNow {
			err := server.taskDistributor.DistributeTaskSendNotification(c.Request.Context(), &worker.PayloadSendNotification{
				RecipientID: result.PreviousBidder.ID,
				Title:       "Giá của bạn đã bị vượt qua",
				Message: fmt.Sprintf("Lượt đặt giá của bạn cho %s đã bị vượt qua. Giá mới là %s.",
					result.Auction.GundamSnapshot.Name,
					util.FormatMoney(req.Amount)),
				Type:        "auction_outbid",
				ReferenceID: result.Auction.ID.String(),
			}, opts...)
			if err != nil {
				log.Err(err).
					Str("recipient_id", result.PreviousBidder.ID).
					Str("auction_id", result.Auction.ID.String()).
					Msg("failed to send notification to outbid user")
			}
		}
	}()
	
	// 6. Xử lý trường hợp mua ngay
	if result.IsBuyNow {
		// Gửi thông báo auction ended qua SSE
		server.eventSender.Broadcast(event.Event{
			Topic: topic,
			Type:  event.EventTypeAuctionEnded,
			Data: map[string]interface{}{
				"auction_id":     auctionID.String(),
				"final_price":    req.Amount,
				"winning_bid_id": result.AuctionBid.ID.String(),
				"winner":         user,
				"reason":         "buy_now_price_reached",
				"timestamp":      time.Now(),
			},
		})
		
		// Log thông tin mua ngay
		log.Info().
			Str("auction_id", auctionID.String()).
			Str("bidder_id", userID).
			Int64("buy_now_price", req.Amount).
			Int("refunded_users", len(result.RefundedUserIDs)).
			Msg("auction ended by buy now")
		
		// Lên lịch thông báo cho người thắng và người bán
		go func() {
			opts := []asynq.Option{
				asynq.MaxRetry(3),
				asynq.Queue(worker.QueueCritical),
			}
			
			// Thông báo cho người thắng
			err = server.taskDistributor.DistributeTaskSendNotification(c.Request.Context(), &worker.PayloadSendNotification{
				RecipientID: userID,
				Title:       "Bạn đã thắng phiên đấu giá!",
				Message: fmt.Sprintf("Chúc mừng! Bạn đã thắng đấu giá %s với giá %s. Vui lòng thanh toán trong vòng 48 giờ.",
					result.Auction.GundamSnapshot.Name,
					util.FormatVND(req.Amount)),
				Type:        "auction_win",
				ReferenceID: result.Auction.ID.String(),
			}, opts...)
			if err != nil {
				log.Err(err).
					Str("recipient_id", userID).
					Str("auction_id", result.Auction.ID.String()).
					Msg("failed to send win notification")
			}
			
			// Thông báo cho người bán
			err = server.taskDistributor.DistributeTaskSendNotification(c.Request.Context(), &worker.PayloadSendNotification{
				RecipientID: result.Auction.SellerID,
				Title:       "Phiên đấu giá đã kết thúc",
				Message: fmt.Sprintf("Phiên đấu giá %s của bạn đã kết thúc với giá cuối cùng là %s.",
					result.Auction.GundamSnapshot.Name,
					util.FormatVND(req.Amount)),
				Type:        "auction_ended",
				ReferenceID: result.Auction.ID.String(),
			}, opts...)
			if err != nil {
				log.Err(err).
					Str("recipient_id", result.Auction.SellerID).
					Str("auction_id", result.Auction.ID.String()).
					Msg("failed to send seller notification")
			}
			
			// Thông báo hoàn tiền cho những người tham gia khác
			for _, refundedUserID := range result.RefundedUserIDs {
				err = server.taskDistributor.DistributeTaskSendNotification(c.Request.Context(), &worker.PayloadSendNotification{
					RecipientID: refundedUserID,
					Title:       "Hoàn trả tiền đặt cọc",
					Message: fmt.Sprintf("Bạn đã không thắng phiên đấu giá %s. Số tiền đặt cọc %s đã được hoàn trả.",
						result.Auction.GundamSnapshot.Name,
						util.FormatVND(result.Auction.DepositAmount)),
					Type:        "auction_deposit_refund",
					ReferenceID: result.Auction.ID.String(),
				}, opts...)
				if err != nil {
					log.Err(err).
						Str("recipient_id", refundedUserID).
						Str("auction_id", result.Auction.ID.String()).
						Msg("failed to send refund notification")
				}
			}
		}()
	}
	
	c.JSON(http.StatusOK, result)
}
