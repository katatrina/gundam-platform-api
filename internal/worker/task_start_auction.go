package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	db "github.com/katatrina/gundam-BE/internal/db/sqlc"
	"github.com/rs/zerolog/log"
)

type PayloadStartAuction struct {
	AuctionID uuid.UUID `json:"auction_id"`
}

// DistributeTaskStartAuction lên lịch task bắt đầu phiên đấu giá
func (distributor *RedisTaskDistributor) DistributeTaskStartAuction(
	ctx context.Context,
	payload *PayloadStartAuction,
	opts ...asynq.Option,
) error {
	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal task payload: %w", err)
	}
	
	taskID := fmt.Sprintf("auction:start:%s", payload.AuctionID.String())
	task := asynq.NewTask(TaskStartAuction, jsonPayload, append(opts, asynq.TaskID(taskID))...)
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
		Msg("auction start task scheduled")
	
	return nil
}

// ProcessTaskStartAuction xử lý task bắt đầu phiên đấu giá
func (processor *RedisTaskProcessor) ProcessTaskStartAuction(
	ctx context.Context,
	task *asynq.Task,
) error {
	var payload PayloadStartAuction
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return fmt.Errorf("failed to unmarshal payload: %w", asynq.SkipRetry)
	}
	
	log.Info().
		Str("auction_id", payload.AuctionID.String()).
		Msg("processing auction start task")
	
	// Kiểm tra auction status trước khi xử lý
	auction, err := processor.store.GetAuctionByID(ctx, payload.AuctionID)
	if err != nil {
		if errors.Is(err, db.ErrRecordNotFound) {
			log.Info().
				Str("auction_id", payload.AuctionID.String()).
				Msg("auction not found, skipping task")
			return nil // Skip task nếu auction không tồn tại
		}
		return fmt.Errorf("failed to get auction: %w", err)
	}
	
	// Chỉ xử lý nếu status đang là scheduled
	if auction.Status != db.AuctionStatusScheduled {
		log.Info().
			Str("auction_id", payload.AuctionID.String()).
			Str("current_status", string(auction.Status)).
			Msg("auction status is not scheduled, skipping task")
		return nil
	}
	
	// Cập nhật trạng thái của auction sang active
	updatedAuction, err := processor.store.UpdateAuction(ctx, db.UpdateAuctionParams{
		ID: payload.AuctionID,
		Status: db.NullAuctionStatus{
			AuctionStatus: db.AuctionStatusActive,
			Valid:         true,
		},
	})
	if err != nil {
		log.Error().
			Err(err).
			Str("auction_id", payload.AuctionID.String()).
			Msg("failed to update auction status to active")
		return err
	}
	
	log.Info().
		Str("auction_id", payload.AuctionID.String()).
		Str("old_status", string(auction.Status)).
		Str("new_status", string(updatedAuction.Status)).
		Msg("auction status updated from scheduled to active")
	
	// Gửi thông báo cho người bán rằng phiên đấu giá đã bắt đầu
	err = processor.distributor.DistributeTaskSendNotification(ctx, &PayloadSendNotification{
		RecipientID: updatedAuction.SellerID,
		Title:       "Phiên đấu giá đã bắt đầu",
		Message: fmt.Sprintf("Phiên đấu giá %s của bạn đã bắt đầu và sẽ kết thúc vào %s.",
			auction.GundamSnapshot.Name,
			updatedAuction.EndTime.Format("15:04 ngày 02/01/2006")),
		Type:        "auction_started",
		ReferenceID: updatedAuction.ID.String(),
	})
	if err != nil {
		log.Warn().
			Err(err).
			Str("seller_id", updatedAuction.SellerID).
			Str("auction_id", updatedAuction.ID.String()).
			Msg("failed to send notification to seller")
		// Không return lỗi vì thông báo không quan trọng bằng việc cập nhật status
	}
	
	return nil
}
