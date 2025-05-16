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
	
	// Giao dịch đặt giá đã thành công, xử lý các thông báo và cập nhật
	
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
	
	// 4. Gửi thông báo realtime qua SSE dựa vào kết quả đặt giá
	topic := fmt.Sprintf("auction:%s", auctionID.String())
	
	if result.CanEndNow {
		// Nếu là mua ngay, chỉ gửi thông báo kết thúc đấu giá
		server.eventSender.Broadcast(event.Event{
			Topic: topic,
			Type:  event.EventTypeAuctionEnded,
			Data: map[string]interface{}{
				"auction_id":     auctionID.String(),
				"final_price":    req.Amount,
				"winning_bid_id": result.AuctionBid.ID.String(),
				"winner":         user,
				"reason":         "buy_now_price_reached",
				"timestamp":      result.Auction.ActualEndTime,
				"bid_details":    result.AuctionBid,
				"total_bids":     result.Auction.TotalBids,
			},
		})
		
		// Log thông tin mua ngay
		log.Info().
			Str("auction_id", auctionID.String()).
			Str("bidder_id", userID).
			Int64("buy_now_price", req.Amount).
			Int("refunded_users", len(result.RefundedUserIDs)).
			Msg("auction ended by buy now")
	} else {
		// Nếu là đặt giá thông thường, gửi thông báo new_bid
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
		
		// Log thông tin đặt giá thông thường
		log.Info().
			Str("auction_id", auctionID.String()).
			Str("bidder_id", userID).
			Int64("amount", req.Amount).
			Int32("total_bids", result.Auction.TotalBids).
			Msg("bid placed successfully")
	}
	
	// 5. Xử lý thông báo hệ thống cho tất cả người liên quan (trong goroutine riêng)
	go func() {
		opts := []asynq.Option{
			asynq.MaxRetry(3),
			asynq.Queue(worker.QueueCritical),
		}
		
		// Xử lý thông báo dựa trên loại đặt giá
		if result.CanEndNow {
			// A. Mua ngay - Thông báo cho người thắng/người bán/người tham gia khác
			
			// Thông báo cho người thắng
			err := server.taskDistributor.DistributeTaskSendNotification(
				context.Background(), // Sử dụng context mới để tránh cancel
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
				context.Background(),
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
					context.Background(),
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
		} else {
			// B. Đặt giá thông thường - Thông báo cho người bán và người vượt giá
			
			// Thông báo cho người bán về lượt đặt giá mới
			err := server.taskDistributor.DistributeTaskSendNotification(
				context.Background(),
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
				err := server.taskDistributor.DistributeTaskSendNotification(
					context.Background(),
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
