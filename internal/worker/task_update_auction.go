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

const (
	TaskStartAuction = "auction:start"
	TaskEndAuction   = "auction:end"
)

type PayloadStartAuction struct {
	AuctionID uuid.UUID `json:"auction_id"`
}

type PayloadEndAuction struct {
	AuctionID uuid.UUID `json:"auction_id"`
}

func (distributor *RedisTaskDistributor) DistributeTaskStartAuction(
	ctx context.Context,
	payload *PayloadStartAuction,
	opts ...asynq.Option,
) error {
	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal task payload: %w", err)
	}
	
	task := asynq.NewTask(TaskStartAuction, jsonPayload, opts...)
	info, err := distributor.client.EnqueueContext(ctx, task)
	if err != nil {
		return fmt.Errorf("failed to enqueue task: %w", err)
	}
	
	log.Info().Str("type", task.Type()).Bytes("payload", task.Payload()).
		Str("queue", info.Queue).Int("max_retry", info.MaxRetry).
		Msg("auction status update task enqueued")
	
	return nil
}

func (processor *RedisTaskProcessor) ProcessTaskStartAuction(
	ctx context.Context,
	task *asynq.Task,
) error {
	var payload PayloadStartAuction
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return fmt.Errorf("failed to unmarshal payload: %w", asynq.SkipRetry)
	}
	
	// Kiểm tra auction status trước khi xử lý
	auction, err := processor.store.GetAuctionByID(ctx, payload.AuctionID)
	if err != nil {
		if errors.Is(err, db.ErrRecordNotFound) {
			log.Info().Str("auction_id", payload.AuctionID.String()).
				Msg("auction not found, skipping task")
			return nil // Skip task nếu auction không tồn tại
		}
		return err
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
	_, err = processor.store.UpdateAuction(ctx, db.UpdateAuctionParams{
		ID: payload.AuctionID,
		Status: db.NullAuctionStatus{
			AuctionStatus: db.AuctionStatusActive,
			Valid:         true,
		},
	})
	if err != nil {
		log.Error().Err(err).Str("auction_id", payload.AuctionID.String()).
			Msg("failed to update auction status to active")
		return err
	}
	
	log.Info().Str("type", task.Type()).Str("auction_id", payload.AuctionID.String()).
		Msg("auction status updated from scheduled to active")
	
	return nil
}

func (distributor *RedisTaskDistributor) DistributeTaskEndAuction(
	ctx context.Context,
	payload *PayloadEndAuction,
	opts ...asynq.Option,
) error {
	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal task payload: %w", err)
	}
	
	task := asynq.NewTask(TaskEndAuction, jsonPayload, opts...)
	info, err := distributor.client.EnqueueContext(ctx, task)
	if err != nil {
		return fmt.Errorf("failed to enqueue task: %w", err)
	}
	
	log.Info().Str("type", task.Type()).Bytes("payload", task.Payload()).
		Str("queue", info.Queue).Int("max_retry", info.MaxRetry).
		Msg("auction status update task enqueued")
	
	return nil
}

func (processor *RedisTaskProcessor) ProcessTaskEndAuction(
	ctx context.Context,
	task *asynq.Task,
) error {
	var payload PayloadEndAuction
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return fmt.Errorf("failed to unmarshal payload: %w", asynq.SkipRetry)
	}
	
	// Kiểm tra auction status trước khi xử lý
	auction, err := processor.store.GetAuctionByID(ctx, payload.AuctionID)
	if err != nil {
		if errors.Is(err, db.ErrRecordNotFound) {
			log.Info().Str("auction_id", payload.AuctionID.String()).
				Msg("auction not found, skipping task")
			return nil
		}
		return err
	}
	
	// Chỉ xử lý nếu status đang là active
	if auction.Status != db.AuctionStatusActive {
		log.Info().
			Str("auction_id", payload.AuctionID.String()).
			Str("current_status", string(auction.Status)).
			Msg("auction status is not active, skipping task")
		return nil
	}
	
	// TODO: Xử lý kết thúc đấu giá (sẽ triển khai sau)
	// result, err := processor.store.EndAuctionTx(ctx, payload.AuctionID)
	// if err != nil {
	// 	log.Error().Err(err).Str("auction_id", payload.AuctionID.String()).
	// 		Msg("failed to end auction")
	// 	return err
	// }
	
	// ... handle result, send notifications, etc.
	
	return nil
}
