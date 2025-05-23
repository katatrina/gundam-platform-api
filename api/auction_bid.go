package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
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
	
	// Kiểm tra buy_now trước khi validate increment
	isBuyNow := false
	if auction.BuyNowPrice != nil && req.Amount >= *auction.BuyNowPrice {
		isBuyNow = true
	}
	
	// Chỉ validate bid increment nếu không phải buy now
	if !isBuyNow {
		minRequiredBid := auction.CurrentPrice + auction.BidIncrement
		if req.Amount < minRequiredBid {
			err = fmt.Errorf("bid amount must be at least %s, current price: %s, bid increment: %s",
				util.FormatMoney(minRequiredBid),
				util.FormatMoney(auction.CurrentPrice),
				util.FormatMoney(auction.BidIncrement))
			c.JSON(http.StatusUnprocessableEntity, errorResponse(err))
			return
		}
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
	
	// 2. Thực hiện transaction đặt giá
	result, err := server.dbStore.PlaceBidTx(c, db.PlaceBidTxParams{
		UserID:      userID,
		Auction:     auction,
		Participant: participant,
		Amount:      req.Amount,
		OnEndNowFunc: func(auctionID uuid.UUID) error {
			// Lấy task ID từ key trong Redis
			taskID := fmt.Sprintf("auction:end:%s", auctionID.String())
			
			// Sử dụng taskInspector để xóa task
			return server.taskInspector.DeleteTask(c.Request.Context(), worker.QueueCritical, taskID)
		},
		CheckPaymentFunc: func(endedAuction db.Auction, winnerID string) error {
			opts := []asynq.Option{
				asynq.MaxRetry(3),
				asynq.Queue(worker.QueueCritical),
				asynq.ProcessAt(*endedAuction.WinnerPaymentDeadline),
			}
			
			return server.taskDistributor.DistributeTaskCheckAuctionPayment(c.Request.Context(), &worker.PayloadCheckAuctionPayment{
				AuctionID:  endedAuction.ID,
				WinnerID:   winnerID,
				SellerID:   endedAuction.SellerID,
				Deadline:   endedAuction.WinnerPaymentDeadline.Format(time.RFC3339),
				GundamName: endedAuction.GundamSnapshot.Name,
			}, opts...)
		},
		PaymentReminderFunc: func(endedAuction db.Auction, winnerID string) error {
			// TODO: Trong tương lai, có thể thay thế việc nhắc nhở thanh toán bằng cách gửi qua email, sđt, push notification,... thay vì gửi thông báo thông thường.
			// Bởi vì thanh toán đấu giá là một việc rất quan trọng, nên cần có nhiều cách nhắc nhở khác nhau để đảm bảo người thắng không bỏ lỡ.
			
			reminderTimes := []struct {
				Duration time.Duration
				Sequence int
			}{
				{6 * time.Hour, 1},
				{24 * time.Hour, 2},
				{36 * time.Hour, 3},
			}
			
			var wg sync.WaitGroup
			
			for _, reminder := range reminderTimes {
				wg.Add(1)
				go func() {
					defer wg.Done()
					
					reminderTime := endedAuction.ActualEndTime.Add(reminder.Duration)
					remainingHours := int((endedAuction.WinnerPaymentDeadline.Sub(reminderTime)).Hours())
					
					opts := []asynq.Option{
						asynq.MaxRetry(3),
						asynq.Queue(worker.QueueCritical),
						asynq.ProcessAt(reminderTime),
					}
					
					err := server.taskDistributor.DistributeTaskPaymentReminder(
						c.Request.Context(),
						&worker.PayloadPaymentReminder{
							AuctionID:        endedAuction.ID,
							WinnerID:         winnerID,
							GundamName:       endedAuction.GundamSnapshot.Name,
							RemainingHours:   remainingHours,
							ReminderSequence: reminder.Sequence,
						},
						opts...,
					)
					
					if err != nil {
						log.Err(err).
							Str("auction_id", endedAuction.ID.String()).
							Str("winner_id", winnerID).
							Int("reminder_sequence", reminder.Sequence).
							Msg("failed to schedule payment reminder task")
					}
				}()
			}
			
			wg.Wait()
			return err
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
	
	// Broadcast events và send notifications async
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		
		topic := fmt.Sprintf("auction:%s", auctionID.String())
		
		opts := []asynq.Option{
			asynq.MaxRetry(3),
			asynq.Queue(worker.QueueCritical),
		}
		
		// 3. Gửi sự kiện và thông báo dựa vào ngữ cảnh
		if result.CanEndNow { // Phiên đấu giá đã kết thúc
			auctionEndedEvent := event.Event{
				Topic: topic,
				Type:  event.EventTypeAuctionEnded,
				Data: map[string]interface{}{
					"auction_id":     auctionID.String(),            // ID phiên đấu giá
					"final_price":    req.Amount,                    // Giá cuối cùng
					"winning_bid_id": result.AuctionBid.ID.String(), // ID của bid thắng
					"winner":         result.Bidder,                 // Thông tin người thắng
					"reason":         "buy_now_price_reached",       // Lý do kết thúc
					"bid_details":    result.AuctionBid,             // Chi tiết bid thắng
					"total_bids":     result.Auction.TotalBids,      // Tổng số lượt đặt giá
					"timestamp":      result.Auction.ActualEndTime,  // Thời gian kết thúc
				},
			}
			server.eventSender.Broadcast(auctionEndedEvent)
			
			// Log thông tin của phiên đấu giá đã kết thúc
			log.Info().
				Str("auction_id", auctionID.String()).
				Str("bidder_id", userID).
				Int64("buy_now_price", req.Amount).
				Int("refunded_users", len(result.RefundedUserIDs)).
				Msg("auction ended by buy now")
			
			// Thông báo cho người thắng
			err = server.taskDistributor.DistributeTaskSendNotification(
				ctx,
				&worker.PayloadSendNotification{
					RecipientID: userID,
					Title:       "Bạn đã thắng phiên đấu giá!",
					Message: fmt.Sprintf("Chúc mừng! Bạn đã thắng đấu giá %s với giá %s. Vui lòng thanh toán trong vòng 48 giờ.",
						result.Auction.GundamSnapshot.Name,
						util.FormatVND(req.Amount)),
					Type:        "auction_win",
					ReferenceID: result.Auction.ID.String(),
				},
				opts...,
			)
			if err != nil {
				log.Err(err).
					Str("recipient_id", userID).
					Str("auction_id", result.Auction.ID.String()).
					Msg("failed to send win notification")
			}
			
			// Thông báo cho người bán
			err = server.taskDistributor.DistributeTaskSendNotification(
				ctx,
				&worker.PayloadSendNotification{
					RecipientID: result.Auction.SellerID,
					Title:       "Phiên đấu giá đã kết thúc",
					Message: fmt.Sprintf("Phiên đấu giá %s của bạn đã kết thúc với giá cuối cùng là %s.",
						result.Auction.GundamSnapshot.Name,
						util.FormatVND(req.Amount)),
					Type:        "auction_ended",
					ReferenceID: result.Auction.ID.String(),
				},
				opts...,
			)
			if err != nil {
				log.Err(err).
					Str("recipient_id", result.Auction.SellerID).
					Str("auction_id", result.Auction.ID.String()).
					Msg("failed to send seller notification")
			}
			
			// Thông báo hoàn tiền cho những người tham gia khác
			for _, refundedUserID := range result.RefundedUserIDs {
				err = server.taskDistributor.DistributeTaskSendNotification(
					ctx,
					&worker.PayloadSendNotification{
						RecipientID: refundedUserID,
						Title:       "Hoàn trả tiền đặt cọc",
						Message: fmt.Sprintf("Bạn đã không thắng phiên đấu giá %s. Số tiền đặt cọc %s đã được hoàn trả.",
							result.Auction.GundamSnapshot.Name,
							util.FormatVND(result.Auction.DepositAmount)),
						Type:        "auction_deposit_refund",
						ReferenceID: result.Auction.ID.String(),
					},
					opts...,
				)
				if err != nil {
					log.Err(err).
						Str("recipient_id", refundedUserID).
						Str("auction_id", result.Auction.ID.String()).
						Msg("failed to send refund notification")
				}
			}
		} else { // Phiên đấu giá vẫn tiếp tục diễn ra
			auctionNewBidEvent := event.Event{
				Topic: topic,
				Type:  event.EventTypeNewBid,
				Data: map[string]interface{}{
					"auction_id":    auctionID.String(),            // ID phiên đấu giá
					"current_price": result.Auction.CurrentPrice,   // Giá hiện tại
					"bid_id":        result.AuctionBid.ID.String(), // ID của bid mới
					"bid_amount":    result.AuctionBid.Amount,      // Số tiền đặt giá
					"bidder":        result.Bidder,                 // Thông tin người đặt giá
					"total_bids":    result.Auction.TotalBids,      // Tổng số lượt đặt giá mới nhất
					"timestamp":     result.AuctionBid.CreatedAt,   // Thời gian đặt giá
				},
			}
			server.eventSender.Broadcast(auctionNewBidEvent)
			
			// Log thông tin của phiên đấu giá đang diễn ra
			log.Info().
				Str("auction_id", auctionID.String()).
				Str("bidder_id", userID).
				Int64("amount", req.Amount).
				Int32("total_bids", result.Auction.TotalBids).
				Msg("bid placed successfully")
			
			// Thông báo cho người bán về lượt đặt giá mới
			err = server.taskDistributor.DistributeTaskSendNotification(
				ctx,
				&worker.PayloadSendNotification{
					RecipientID: result.Auction.SellerID,
					Title:       "Có lượt đặt giá mới",
					Message: fmt.Sprintf("Có người vừa đặt giá %s cho phiên đấu giá %s của bạn.",
						util.FormatVND(req.Amount),
						result.Auction.GundamSnapshot.Name),
					Type:        "auction_new_bid",
					ReferenceID: result.Auction.ID.String(),
				},
				opts...,
			)
			if err != nil {
				log.Err(err).
					Str("recipient_id", result.Auction.SellerID).
					Str("auction_id", result.Auction.ID.String()).
					Msg("failed to send notification to seller")
			}
			
			// Thông báo cho người bị vượt giá (nếu có)
			if result.PreviousBidder != nil && result.PreviousBidder.ID != userID {
				err = server.taskDistributor.DistributeTaskSendNotification(
					ctx,
					&worker.PayloadSendNotification{
						RecipientID: result.PreviousBidder.ID,
						Title:       "Giá của bạn đã bị vượt qua",
						Message: fmt.Sprintf("Lượt đặt giá của bạn cho %s đã bị vượt qua. Giá mới là %s.",
							result.Auction.GundamSnapshot.Name,
							util.FormatMoney(req.Amount)),
						Type:        "auction_outbid",
						ReferenceID: result.Auction.ID.String(),
					},
					opts...,
				)
				if err != nil {
					log.Err(err).
						Str("recipient_id", result.PreviousBidder.ID).
						Str("auction_id", result.Auction.ID.String()).
						Msg("failed to send notification to outbid user")
				}
			}
		}
	}()
	
	// Trả về kết quả cho client
	c.JSON(http.StatusOK, result)
}
