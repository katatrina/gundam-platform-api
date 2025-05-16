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
	"github.com/rs/zerolog/log"
)

type PayloadPaymentReminder struct {
	AuctionID        uuid.UUID `json:"auction_id"`
	WinnerID         string    `json:"winner_id"`
	GundamName       string    `json:"gundam_name"`
	RemainingHours   int       `json:"remaining_hours"`
	ReminderSequence int       `json:"reminder_sequence"`
}

// DistributeTaskPaymentReminder lên lịch task nhắc nhở thanh toán.
func (distributor *RedisTaskDistributor) DistributeTaskPaymentReminder(
	ctx context.Context,
	payload *PayloadPaymentReminder,
	opts ...asynq.Option,
) error {
	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal task payload: %w", err)
	}
	
	taskID := fmt.Sprintf("auction:payment_reminder:%s:%d", payload.AuctionID.String(), payload.ReminderSequence)
	task := asynq.NewTask(TaskPaymentReminder, jsonPayload, append(opts, asynq.TaskID(taskID))...)
	info, err := distributor.client.EnqueueContext(ctx, task)
	if err != nil {
		return fmt.Errorf("failed to enqueue task: %w", err)
	}
	
	log.Info().
		Str("type", task.Type()).
		Str("task_id", taskID).
		Str("auction_id", payload.AuctionID.String()).
		Str("winner_id", payload.WinnerID).
		Int("reminder_sequence", payload.ReminderSequence).
		Str("queue", info.Queue).
		Int("max_retry", info.MaxRetry).
		Time("process_at", info.NextProcessAt).
		Msg("payment reminder task scheduled")
	
	return nil
}

// ProcessTaskPaymentReminder xử lý task nhắc nhở thanh toán
func (processor *RedisTaskProcessor) ProcessTaskPaymentReminder(
	ctx context.Context,
	task *asynq.Task,
) error {
	var payload PayloadPaymentReminder
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return fmt.Errorf("failed to unmarshal payment reminder payload: %w", asynq.SkipRetry)
	}
	
	log.Info().
		Str("auction_id", payload.AuctionID.String()).
		Str("winner_id", payload.WinnerID).
		Int("reminder_sequence", payload.ReminderSequence).
		Int("remaining_hours", payload.RemainingHours).
		Msg("processing payment reminder")
	
	// Kiểm tra auction
	auction, err := processor.store.GetAuctionByID(ctx, payload.AuctionID)
	if err != nil {
		if errors.Is(err, db.ErrRecordNotFound) {
			log.Info().
				Str("auction_id", payload.AuctionID.String()).
				Msg("auction not found, skipping payment reminder")
			return nil // Skip silently
		}
		return fmt.Errorf("failed to get auction: %w", err)
	}
	
	// Kiểm tra nếu phiên đấu giá không còn ở trạng thái ended
	if auction.Status != db.AuctionStatusEnded {
		log.Info().
			Str("auction_id", payload.AuctionID.String()).
			Str("status", string(auction.Status)).
			Msg("auction not in 'ended' status, skipping payment reminder")
		return nil
	}
	
	// Kiểm tra đã có order hay chưa
	if auction.OrderID != nil {
		log.Info().
			Str("auction_id", payload.AuctionID.String()).
			Str("order_id", auction.OrderID.String()).
			Msg("payment already made, skipping reminder")
		return nil
	}
	
	// Kiểm tra xem còn deadline thanh toán không
	if auction.WinnerPaymentDeadline != nil {
		log.Warn().
			Str("auction_id", payload.AuctionID.String()).
			Msg("auction has no payment deadline, skipping reminder")
		return nil
	}
	
	// Kiểm tra xem đã quá deadline chưa
	if time.Now().After(*auction.WinnerPaymentDeadline) {
		log.Info().
			Str("auction_id", payload.AuctionID.String()).
			Time("deadline", *auction.WinnerPaymentDeadline).
			Msg("payment deadline passed, skipping reminder")
		return nil
	}
	
	gundamName := payload.GundamName
	var messageText string
	switch payload.ReminderSequence {
	case 1:
		messageText = fmt.Sprintf("Nhắc nhở: Bạn còn %d giờ để thanh toán phiên đấu giá %s. Vui lòng thanh toán để nhận sản phẩm.",
			payload.RemainingHours,
			gundamName)
	case 2:
		messageText = fmt.Sprintf("Nhắc nhở quan trọng: Bạn còn %d giờ để thanh toán phiên đấu giá %s. Tiền đặt cọc sẽ bị mất nếu không thanh toán đúng hạn.",
			payload.RemainingHours,
			gundamName)
	default:
		messageText = fmt.Sprintf("Nhắc nhở khẩn cấp: Chỉ còn %d giờ để thanh toán phiên đấu giá %s. Hãy thanh toán ngay để tránh mất tiền đặt cọc.",
			payload.RemainingHours,
			gundamName)
	}
	
	err = processor.distributor.DistributeTaskSendNotification(ctx, &PayloadSendNotification{
		RecipientID: payload.WinnerID,
		Title:       fmt.Sprintf("Nhắc nhở thanh toán đấu giá (%d/%d)", payload.ReminderSequence, 3),
		Message:     messageText,
		Type:        "auction_payment_reminder",
		ReferenceID: payload.AuctionID.String(),
	})
	if err != nil {
		log.Warn().
			Err(err).
			Str("winner_id", payload.WinnerID).
			Str("auction_id", payload.AuctionID.String()).
			Int("reminder_sequence", payload.ReminderSequence).
			Msg("failed to send payment reminder notification")
		// Không return error vì đây chỉ là nhắc nhở
	}
	
	log.Info().
		Str("auction_id", payload.AuctionID.String()).
		Str("winner_id", payload.WinnerID).
		Int("reminder_sequence", payload.ReminderSequence).
		Int("remaining_hours", payload.RemainingHours).
		Msg("payment reminder sent successfully")
	
	return nil
}
