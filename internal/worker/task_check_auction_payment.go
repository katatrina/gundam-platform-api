package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	db "github.com/katatrina/gundam-BE/internal/db/sqlc"
	"github.com/katatrina/gundam-BE/internal/util"
	"github.com/rs/zerolog/log"
)

type PayloadCheckAuctionPayment struct {
	AuctionID  uuid.UUID `json:"auction_id"`
	WinnerID   string    `json:"winner_id"`
	SellerID   string    `json:"seller_id"`
	Deadline   string    `json:"deadline"`
	GundamName string    `json:"gundam_name"`
}

// DistributeTaskCheckAuctionPayment lên lịch task kiểm tra thanh toán sau khi phiên đấu giá kết thúc
func (distributor *RedisTaskDistributor) DistributeTaskCheckAuctionPayment(
	ctx context.Context,
	payload *PayloadCheckAuctionPayment,
	opts ...asynq.Option,
) error {
	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal task payload: %w", err)
	}
	
	taskID := fmt.Sprintf("auction:check_payment:%s", payload.AuctionID.String())
	task := asynq.NewTask(TaskCheckAuctionPayment, jsonPayload, append(opts, asynq.TaskID(taskID))...)
	info, err := distributor.client.EnqueueContext(ctx, task)
	if err != nil {
		return fmt.Errorf("failed to enqueue task: %w", err)
	}
	
	log.Info().
		Str("type", task.Type()).
		Str("task_id", taskID).
		Str("auction_id", payload.AuctionID.String()).
		Str("winner_id", payload.WinnerID).
		Str("queue", info.Queue).
		Int("max_retry", info.MaxRetry).
		Time("process_at", info.NextProcessAt).
		Msg("auction payment check task scheduled")
	
	return nil
}

// ProcessTaskCheckAuctionPayment kiểm tra thanh toán sau khi phiên đấu giá kết thúc
func (processor *RedisTaskProcessor) ProcessTaskCheckAuctionPayment(
	ctx context.Context,
	task *asynq.Task,
) error {
	var payload PayloadCheckAuctionPayment
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return fmt.Errorf("failed to unmarshal check auction payment payload: %w", asynq.SkipRetry)
	}
	
	log.Info().
		Str("auction_id", payload.AuctionID.String()).
		Str("winner_id", payload.WinnerID).
		Str("deadline", payload.Deadline).
		Msg("checking auction payment status")
	
	// 1. Lấy thông tin phiên đấu giá
	auction, err := processor.store.GetAuctionByID(ctx, payload.AuctionID)
	if err != nil {
		if errors.Is(err, db.ErrRecordNotFound) {
			log.Warn().
				Str("auction_id", payload.AuctionID.String()).
				Msg("auction not found, skipping payment check")
			return nil // Skip silently, auction might have been deleted
		}
		return fmt.Errorf("failed to get auction details: %w", err)
	}
	
	// 2. Kiểm tra trạng thái hiện tại của phiên đấu giá
	// Nếu đã ở trạng thái completed, có nghĩa là người thắng đã thanh toán
	if auction.Status == db.AuctionStatusCompleted {
		log.Info().
			Str("auction_id", payload.AuctionID.String()).
			Msg("auction payment already completed, no action needed")
		return nil
	}
	
	// Nếu ở trạng thái khác "ended", có thể đã có xử lý trước đó (failed, canceled...)
	if auction.Status != db.AuctionStatusEnded {
		log.Info().
			Str("auction_id", payload.AuctionID.String()).
			Str("status", string(auction.Status)).
			Msg("auction not in 'ended' status, skipping payment check")
		return nil
	}
	
	// 3. Kiểm tra xem đã có order được tạo chưa
	if auction.OrderID != nil {
		// Đã có order - thanh toán đã được xử lý
		log.Info().
			Str("auction_id", payload.AuctionID.String()).
			Str("order_id", auction.OrderID.String()).
			Msg("auction already has an order, payment is processing")
		return nil
	}
	
	// 4. Chưa thanh toán trong thời hạn - xử lý theo quy trình
	log.Info().
		Str("auction_id", payload.AuctionID.String()).
		Str("winner_id", payload.WinnerID).
		Msg("payment deadline passed, handling non-payment")
	
	// Xử lý trong transaction
	result, err := processor.store.HandleAuctionNonPaymentTx(ctx, db.HandleAuctionNonPaymentTxParams{
		AuctionID: payload.AuctionID,
		WinnerID:  payload.WinnerID,
		SellerID:  payload.SellerID,
	})
	if err != nil {
		return fmt.Errorf("failed to handle auction non-payment: %w", err)
	}
	
	// 5. Gửi thông báo cho người bán về việc được bồi thường
	err = processor.distributor.DistributeTaskSendNotification(ctx, &PayloadSendNotification{
		RecipientID: payload.SellerID,
		Title:       "Bồi thường từ phiên đấu giá",
		Message: fmt.Sprintf("Người thắng đã không thanh toán trong thời hạn. Bạn đã nhận được %s tiền bồi thường từ phiên đấu giá %s.",
			util.FormatMoney(result.CompensationAmount),
			result.GundamName),
		Type:        "auction_compensation",
		ReferenceID: payload.AuctionID.String(),
	})
	if err != nil {
		log.Err(err).
			Str("seller_id", payload.SellerID).
			Str("auction_id", payload.AuctionID.String()).
			Msg("failed to send compensation notification")
	}
	
	// 6. Gửi thông báo cho người thắng về việc mất tiền đặt cọc
	err = processor.distributor.DistributeTaskSendNotification(ctx, &PayloadSendNotification{
		RecipientID: payload.WinnerID,
		Title:       "Tiền đặt cọc đã bị mất",
		Message: fmt.Sprintf("Bạn đã không thanh toán phiên đấu giá %s trong thời hạn 48 giờ. 70%% tiền đặt cọc đã bị mất theo quy định.",
			result.GundamName),
		Type:        "auction_deposit_forfeit",
		ReferenceID: payload.AuctionID.String(),
	})
	if err != nil {
		log.Err(err).
			Str("winner_id", payload.WinnerID).
			Str("auction_id", payload.AuctionID.String()).
			Msg("failed to send deposit forfeit notification")
	}
	
	log.Info().
		Str("auction_id", payload.AuctionID.String()).
		Str("winner_id", payload.WinnerID).
		Str("seller_id", payload.SellerID).
		Int64("compensation_amount", result.CompensationAmount).
		Msg("successfully handled auction non-payment")
	
	return nil
}
