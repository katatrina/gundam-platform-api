package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"
	
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	db "github.com/katatrina/gundam-BE/internal/db/sqlc"
	"github.com/katatrina/gundam-BE/internal/event"
	"github.com/katatrina/gundam-BE/internal/util"
	"github.com/rs/zerolog/log"
)

type PayloadEndAuction struct {
	AuctionID uuid.UUID `json:"auction_id"`
}

// DistributeTaskEndAuction lên lịch task kết thúc phiên đấu giá
func (distributor *RedisTaskDistributor) DistributeTaskEndAuction(
	ctx context.Context,
	payload *PayloadEndAuction,
	opts ...asynq.Option,
) error {
	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal task payload: %w", err)
	}
	
	taskID := fmt.Sprintf("auction:end:%s", payload.AuctionID.String())
	task := asynq.NewTask(TaskEndAuction, jsonPayload, append(opts, asynq.TaskID(taskID))...)
	info, err := distributor.client.EnqueueContext(ctx, task)
	if err != nil {
		return fmt.Errorf("failed to enqueue task: %w", err)
	}
	
	log.Info().
		Str("type", task.Type()).
		Str("task_id", taskID).
		Str("auction_id", payload.AuctionID.String()).
		Str("queue", info.Queue).
		Int("max_retry", info.MaxRetry).
		Time("process_at", info.NextProcessAt).
		Msg("auction end task scheduled")
	
	return nil
}

// ProcessTaskEndAuction xử lý task kết thúc phiên đấu giá
func (processor *RedisTaskProcessor) ProcessTaskEndAuction(
	ctx context.Context,
	task *asynq.Task,
) error {
	var payload PayloadEndAuction
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return fmt.Errorf("failed to unmarshal payload: %w", asynq.SkipRetry)
	}
	
	log.Info().
		Str("auction_id", payload.AuctionID.String()).
		Msg("processing auction end task")
	
	// Kiểm tra auction status trước khi xử lý
	auction, err := processor.store.GetAuctionByID(ctx, payload.AuctionID)
	if err != nil {
		if errors.Is(err, db.ErrRecordNotFound) {
			log.Info().
				Str("auction_id", payload.AuctionID.String()).
				Msg("auction not found, skipping task")
			return nil
		}
		return fmt.Errorf("failed to get auction: %w", err)
	}
	
	// Chỉ xử lý nếu status đang là active
	if auction.Status != db.AuctionStatusActive {
		log.Info().
			Str("auction_id", payload.AuctionID.String()).
			Str("current_status", string(auction.Status)).
			Msg("auction status is not active, skipping task")
		return nil
	}
	
	// Kiểm tra xem đã có actualEndTime chưa (phòng trường hợp đã kết thúc qua mua ngay)
	if auction.ActualEndTime != nil {
		log.Info().
			Str("auction_id", payload.AuctionID.String()).
			Time("actual_end_time", *auction.ActualEndTime).
			Msg("auction already ended (buy now), skipping scheduled end")
		return nil
	}
	
	// Xử lý kết thúc đấu giá qua transaction
	actualEndTime := time.Now()
	result, err := processor.store.EndAuctionTx(ctx, db.EndAuctionTxParams{
		AuctionID:     payload.AuctionID,
		ActualEndTime: actualEndTime,
	})
	if err != nil {
		log.Error().
			Err(err).
			Str("auction_id", payload.AuctionID.String()).
			Msg("failed to end auction")
		return err
	}
	
	// Gửi event SSE về việc phiên đấu giá kết thúc
	topic := fmt.Sprintf("auction:%s", payload.AuctionID.String())
	
	// Phiên đấu giá đã kết thúc, và có người thắng
	if result.HasWinner && result.WinnerID != nil && result.Winner != nil && auction.WinningBidID != nil {
		// Event đấu giá kết thúc và có người thắng
		auctionEndedEvent := event.Event{
			Topic: topic,
			Type:  event.EventTypeAuctionEnded,
			Data: map[string]interface{}{
				"auction_id":     payload.AuctionID.String(),    // ID của phiên đấu giá
				"final_price":    result.FinalPrice,             // Giá cuối cùng của phiên đấu giá
				"winning_bid_id": auction.WinningBidID.String(), // ID của bid thắng cuộc
				"winner":         *result.Winner,                // Thông tin người thắng cuộc
				"reason":         "time_expired_has_winner",     // Kết thúc do hết thời gian
				"total_bids":     auction.TotalBids,             // Tổng số bid đã đặt
				"timestamp":      actualEndTime,                 // Thời gian kết thúc thực tế
				"has_winner":     true,                          // Có người thắng
			},
		}
		processor.eventSender.Broadcast(auctionEndedEvent)
		
	} else {
		// Event đấu giá kết thúc nhưng không có người thắng
		auctionEndedEvent := event.Event{
			Topic: topic,
			Type:  event.EventTypeAuctionEnded,
			Data: map[string]interface{}{
				"auction_id":  payload.AuctionID.String(), // ID của phiên đấu giá
				"final_price": result.FinalPrice,          // Giá cuối cùng của phiên đấu giá
				"reason":      "time_expired_no_winner",   // Hết thời gian, không có người thắng
				"total_bids":  auction.TotalBids,          // Tổng số bid đã đặt
				"timestamp":   actualEndTime,              // Thời gian kết thúc thực tế
				"has_winner":  false,                      // Không có người thắng
			},
		}
		processor.eventSender.Broadcast(auctionEndedEvent)
	}
	
	// Xử lý các thông báo và tác vụ tiếp theo tùy theo kết quả
	if result.HasWinner && result.WinnerID != nil && result.WinnerPaymentDeadline != nil {
		// Lên lịch kiểm tra thanh toán sau 48 giờ
		paymentDeadline := result.WinnerPaymentDeadline
		err = processor.distributor.DistributeTaskCheckAuctionPayment(ctx, &PayloadCheckAuctionPayment{
			AuctionID:  payload.AuctionID,
			WinnerID:   *result.WinnerID,
			SellerID:   auction.SellerID,
			Deadline:   paymentDeadline.Format(time.RFC3339),
			GundamName: auction.GundamSnapshot.Name,
		}, asynq.ProcessAt(*paymentDeadline))
		if err != nil {
			log.Error().
				Err(err).
				Str("auction_id", payload.AuctionID.String()).
				Str("winner_id", *result.WinnerID).
				Msg("failed to schedule payment check task")
			// Không return error vì payload check là tác vụ phụ
		}
		
		// Lên lịch các thông báo nhắc nhở thanh toán
		reminderTimes := []struct {
			Duration time.Duration
			Sequence int
		}{
			{6 * time.Hour, 1},
			{24 * time.Hour, 2},
			{36 * time.Hour, 3},
		}
		
		for _, reminder := range reminderTimes {
			reminderTime := actualEndTime.Add(reminder.Duration)
			remainingHours := int((paymentDeadline.Sub(reminderTime)).Hours())
			
			err = processor.distributor.DistributeTaskPaymentReminder(ctx, &PayloadPaymentReminder{
				AuctionID:        payload.AuctionID,
				WinnerID:         *result.WinnerID,
				GundamName:       auction.GundamSnapshot.Name,
				RemainingHours:   remainingHours,
				ReminderSequence: reminder.Sequence,
			}, asynq.ProcessAt(reminderTime))
			if err != nil {
				log.Warn().
					Err(err).
					Str("auction_id", payload.AuctionID.String()).
					Str("winner_id", *result.WinnerID).
					Int("reminder_sequence", reminder.Sequence).
					Msg("failed to schedule payment reminder task")
				// Không return error vì nhắc nhở không quan trọng bằng task chính
			}
		}
		
		// Gửi thông báo cho người thắng
		err = processor.distributor.DistributeTaskSendNotification(ctx, &PayloadSendNotification{
			RecipientID: *result.WinnerID,
			Title:       "Bạn đã thắng phiên đấu giá!",
			Message: fmt.Sprintf("Chúc mừng! Bạn đã thắng phiên đấu giá %s với giá %s. Vui lòng thanh toán số tiền còn lại trong vòng 48 giờ.",
				auction.GundamSnapshot.Name,
				util.FormatMoney(result.FinalPrice)),
			Type:        "auction_win",
			ReferenceID: payload.AuctionID.String(),
		})
		if err != nil {
			log.Warn().
				Err(err).
				Str("winner_id", *result.WinnerID).
				Str("auction_id", payload.AuctionID.String()).
				Msg("failed to send win notification")
		}
		
		// Gửi thông báo cho người bán
		err = processor.distributor.DistributeTaskSendNotification(ctx, &PayloadSendNotification{
			RecipientID: auction.SellerID,
			Title:       "Phiên đấu giá đã kết thúc",
			Message: fmt.Sprintf("Phiên đấu giá %s của bạn đã kết thúc với giá cuối cùng là %s, thuộc về người thắng cuộc %s. Vui lòng đợi người thắng thanh toán trong vòng 48 giờ.",
				auction.GundamSnapshot.Name,
				util.FormatMoney(result.FinalPrice),
				result.Winner.FullName),
			Type:        "auction_end",
			ReferenceID: payload.AuctionID.String(),
		})
		if err != nil {
			log.Warn().
				Err(err).
				Str("seller_id", auction.SellerID).
				Str("auction_id", payload.AuctionID.String()).
				Msg("failed to send seller notification")
		}
		
	} else {
		// Không có người thắng cuộc, gửi thông báo cho người bán
		err = processor.distributor.DistributeTaskSendNotification(ctx, &PayloadSendNotification{
			RecipientID: auction.SellerID,
			Title:       "Phiên đấu giá đã kết thúc",
			Message: fmt.Sprintf("Phiên đấu giá %s đã kết thúc mà không có người trả giá.",
				auction.GundamSnapshot.Name),
			Type:        "auction_failed",
			ReferenceID: payload.AuctionID.String(),
		})
		if err != nil {
			log.Warn().
				Err(err).
				Str("seller_id", auction.SellerID).
				Str("auction_id", payload.AuctionID.String()).
				Msg("failed to send auction failed notification")
		}
	}
	
	// Hoàn tiền đặt cọc cho tất cả người tham gia (nếu có)
	for _, refundedUserID := range result.RefundedUserIDs {
		err = processor.distributor.DistributeTaskSendNotification(ctx, &PayloadSendNotification{
			RecipientID: refundedUserID,
			Title:       "Hoàn trả tiền đặt cọc",
			Message: fmt.Sprintf("Bạn đã không thắng trong phiên đấu giá %s. Số tiền đặt cọc %s đã được hoàn trả.",
				auction.GundamSnapshot.Name,
				util.FormatVND(auction.DepositAmount)),
			Type:        "auction_deposit_refund",
			ReferenceID: payload.AuctionID.String(),
		})
		if err != nil {
			log.Warn().
				Err(err).
				Str("user_id", refundedUserID).
				Str("auction_id", payload.AuctionID.String()).
				Msg("failed to send refund notification")
		}
	}
	
	log.Info().
		Str("auction_id", payload.AuctionID.String()).
		Bool("has_winner", result.HasWinner).
		Int("refunded_users", len(result.RefundedUserIDs)).
		Msg("auction ended successfully")
	
	return nil
}
