package ordertracking

import (
	"context"
	"errors"
	"fmt"
	"time"
	
	"github.com/hibiken/asynq"
	db "github.com/katatrina/gundam-BE/internal/db/sqlc"
	"github.com/katatrina/gundam-BE/internal/util"
	"github.com/katatrina/gundam-BE/internal/worker"
	"github.com/rs/zerolog/log"
)

// updateExchangeStatusIfNeeded kiểm tra xem đơn hàng có phải là một phần của giao dịch trao đổi không
// và cập nhật trạng thái của giao dịch trao đổi dựa trên trạng thái của đơn hàng
func (t *OrderTracker) updateExchangeStatusIfNeeded(ctx context.Context, order db.Order) {
	if order.Type != db.OrderTypeExchange {
		return
	}
	
	// Tìm giao dịch trao đổi liên quan đến đơn hàng
	exchange, err := t.store.GetExchangeByOrderID(ctx, &order.ID)
	if err != nil {
		if !errors.Is(err, db.ErrRecordNotFound) {
			log.Error().Err(err).Str("order_id", order.ID.String()).Msg("failed to get exchange by order ID")
		}
		return
	}
	
	// Lấy cả hai đơn hàng liên quan đến giao dịch trao đổi
	var posterOrder, offererOrder db.Order
	var err1, err2 error
	
	if exchange.PosterOrderID != nil {
		posterOrder, err1 = t.store.GetOrderByID(ctx, *exchange.PosterOrderID)
	}
	
	if exchange.OffererOrderID != nil {
		offererOrder, err2 = t.store.GetOrderByID(ctx, *exchange.OffererOrderID)
	}
	
	if err1 != nil || err2 != nil {
		log.Error().
			Err(err1).
			AnErr("err2", err2).
			Str("exchange_id", exchange.ID.String()).
			Msg("failed to get exchange orders")
		return
	}
	
	// Xác định trạng thái thấp nhất giữa hai đơn hàng
	lowestStatus := db.GetLowestOrderStatus(posterOrder.Status, offererOrder.Status)
	
	// Ánh xạ từ trạng thái đơn hàng sang trạng thái exchange
	var exchangeStatus db.ExchangeStatus
	switch lowestStatus {
	case db.OrderStatusPending:
		exchangeStatus = db.ExchangeStatusPending
	case db.OrderStatusPackaging:
		exchangeStatus = db.ExchangeStatusPackaging
	case db.OrderStatusDelivering:
		exchangeStatus = db.ExchangeStatusDelivering
	case db.OrderStatusDelivered:
		exchangeStatus = db.ExchangeStatusDelivered
	case db.OrderStatusCompleted:
		exchangeStatus = db.ExchangeStatusCompleted
	case db.OrderStatusFailed:
		exchangeStatus = db.ExchangeStatusFailed
	case db.OrderStatusCanceled:
		exchangeStatus = db.ExchangeStatusCanceled
	default:
		exchangeStatus = exchange.Status
	}
	
	// Cập nhật trạng thái exchange nếu khác với trạng thái hiện tại
	if exchange.Status != exchangeStatus {
		log.Info().
			Str("exchange_id", exchange.ID.String()).
			Str("old_status", string(exchange.Status)).
			Str("new_status", string(exchangeStatus)).
			Msg("Updating exchange status")
		
		updateParams := db.UpdateExchangeParams{
			ID: exchange.ID,
			Status: db.NullExchangeStatus{
				ExchangeStatus: exchangeStatus,
				Valid:          true,
			},
		}
		
		// Nếu trạng thái là completed, cập nhật thêm completed_at
		if exchangeStatus == db.ExchangeStatusCompleted {
			now := time.Now()
			updateParams.CompletedAt = &now
		}
		
		_, err := t.store.UpdateExchange(ctx, updateParams)
		if err != nil {
			log.Error().Err(err).Str("exchange_id", exchange.ID.String()).Msg("failed to update exchange status")
			return
		}
	}
}

// checkOrderStatus kiểm tra trạng thái đơn hàng trên GHN và cập nhật vào database.
func (t *OrderTracker) checkOrderStatus() {
	ctx := context.Background()
	
	// Lấy danh sách đơn hàng đang vận chuyển ("picking", "delivering")
	orderDeliveries, err := t.store.GetActiveOrderDeliveries(ctx)
	if err != nil {
		log.Error().Err(err).Msg("failed to get orderDeliveries for tracking")
		return
	}
	
	for _, orderDelivery := range orderDeliveries {
		// Kiểm tra xem đơn hàng có mã theo dõi không
		if orderDelivery.DeliveryTrackingCode == nil {
			log.Warn().Str("order_code", orderDelivery.OrderCode).Msg("order-delivery status changed but no tracking code found")
			continue
		}
		
		// Kiểm tra trạng thái đơn hàng trên GHN
		response, err := t.ghnService.GetOrderDetails(ctx, *orderDelivery.DeliveryTrackingCode)
		if err != nil {
			log.Error().Err(err).Str("order_code", orderDelivery.OrderCode).Str("delivery_tracking_code", *orderDelivery.DeliveryTrackingCode).Msg("failed to get order details from GHN")
			continue
		}
		
		// Nếu trạng thái đã thay đổi, cập nhật vào db
		ghnStatus := response.Data.Status
		if ghnStatus != *orderDelivery.Status {
			log.Info().
				Str("order_code", orderDelivery.OrderCode).
				Str("delivery_tracking_code", *orderDelivery.DeliveryTrackingCode).
				Str("old_status", *orderDelivery.Status).
				Str("new_status", response.Data.Status).
				Msg("order-delivery status changed, updating database...")
			
			// Tính toán overall status mới
			oldOverallStatus := orderDelivery.OverallStatus.DeliveryOverralStatus
			newOverallStatus := mapGHNStatusToOverallStatus(ghnStatus)
			isOverallStatusChanged := oldOverallStatus != newOverallStatus
			
			// Chuẩn bị tham số cho câu query UPDATE
			updateParams := db.UpdateOrderDeliveryParams{
				ID:     orderDelivery.ID,
				Status: &ghnStatus,
			}
			
			// Chỉ cập nhật overall_status nếu nó thay đổi
			if isOverallStatusChanged {
				updateParams.OverallStatus = db.NullDeliveryOverralStatus{
					DeliveryOverralStatus: newOverallStatus,
					Valid:                 true,
				}
			}
			
			updatedOrderDelivery, err := t.store.UpdateOrderDelivery(ctx, updateParams)
			if err != nil {
				log.Error().Err(err).Str("delivery_tracking_code", *orderDelivery.DeliveryTrackingCode).Msg("failed to update delivery status")
				continue
			}
			
			log.Info().Str("order_code", orderDelivery.OrderCode).Msgf("order-delivery status has been updated to \"%s\"", *updatedOrderDelivery.Status)
			if isOverallStatusChanged {
				log.Info().Str("order_code", orderDelivery.OrderCode).Msgf("order-delivery overall status has been updated to \"%s\"", updatedOrderDelivery.OverallStatus.DeliveryOverralStatus)
			}
			
			// Xử lý business logic theo quy trình từng bước
			// Chỉ xử lý khi có sự chuyển đổi giữa các trạng thái tổng quát
			if isOverallStatusChanged {
				switch {
				// Đơn hàng của người bán đã được shipper đến lấy và chuẩn bị giao cho người mua
				// picking -> delivering
				case oldOverallStatus == db.DeliveryOverralStatusPicking && newOverallStatus == db.DeliveryOverralStatusDelivering:
					// Cập nhật trạng thái đơn hàng thành "delivering"
					updatedOrder, err := t.store.UpdateOrder(ctx, db.UpdateOrderParams{
						OrderID: orderDelivery.OrderID,
						Status: db.NullOrderStatus{
							OrderStatus: db.OrderStatusDelivering,
							Valid:       true,
						},
					})
					if err != nil {
						log.Error().Err(err).Str("order_code", orderDelivery.OrderCode).Msg("failed to update order status to delivered")
					}
					log.Info().Str("order_code", orderDelivery.OrderCode).Msgf("order status has been updated to \"%s\"", updatedOrder.Status)
					
					// Kiểm tra và cập nhật giao dịch trao đổi nếu có
					t.updateExchangeStatusIfNeeded(ctx, updatedOrder)
					
					opts := []asynq.Option{
						asynq.MaxRetry(3),
						asynq.Queue(worker.QueueCritical),
					}
					
					// Gửi thông báo cho người mua
					err = t.taskDistributor.DistributeTaskSendNotification(ctx, &worker.PayloadSendNotification{
						RecipientID: orderDelivery.BuyerID,
						Title:       fmt.Sprintf("Đơn hàng #%s đã được bàn giao cho đơn vị vận chuyển", orderDelivery.OrderCode),
						Message:     fmt.Sprintf("Đơn hàng #%s đã được bàn giao cho đơn vị vận chuyển và chuẩn bị giao đến cho bạn. Bạn có thể theo dõi trạng thái đơn hàng trong mục Đơn mua.", orderDelivery.OrderCode),
						Type:        "order",
						ReferenceID: orderDelivery.OrderCode,
					}, opts...)
					if err != nil {
						log.Err(err).Msg("failed to send notification to buyer")
					}
					log.Info().Msgf("Notification sent to buyer: %s", orderDelivery.BuyerID)
					
					// Gửi thông báo cho người bán
					err = t.taskDistributor.DistributeTaskSendNotification(ctx, &worker.PayloadSendNotification{
						RecipientID: orderDelivery.SellerID,
						Title:       fmt.Sprintf("Đơn hàng #%s đã được bàn giao cho đơn vị vận chuyển", orderDelivery.OrderCode),
						Message:     fmt.Sprintf("Đơn hàng #%s đã được bàn giao cho đơn vị vận chuyển và chuẩn bị giao đến người mua.", orderDelivery.OrderCode),
						Type:        "order",
						ReferenceID: orderDelivery.OrderCode,
					}, opts...)
					if err != nil {
						log.Err(err).Msg("failed to send notification to seller")
					}
					log.Info().Msgf("Notification sent to seller: %s", orderDelivery.SellerID)
				
				// Đơn hàng đã được giao thành công cho người mua
				// delivering -> delivered
				case oldOverallStatus == db.DeliveryOverralStatusDelivering && newOverallStatus == db.DeliveryOverralStatusDelivered:
					// Cập nhật trạng thái đơn hàng thành "delivered"
					updatedOrder, err := t.store.UpdateOrder(ctx, db.UpdateOrderParams{
						OrderID: orderDelivery.OrderID,
						Status: db.NullOrderStatus{
							OrderStatus: db.OrderStatusDelivered,
							Valid:       true,
						},
					})
					if err != nil {
						log.Error().Err(err).Str("order_code", orderDelivery.OrderCode).Msg("failed to update order status to delivered")
					}
					log.Info().Str("order_code", orderDelivery.OrderCode).Msgf("order status has been updated to \"%s\"", updatedOrder.Status)
					
					// Kiểm tra và cập nhật giao dịch trao đổi nếu có
					t.updateExchangeStatusIfNeeded(ctx, updatedOrder)
					
					opts := []asynq.Option{
						asynq.MaxRetry(3),
						asynq.Queue(worker.QueueCritical),
					}
					
					// Gửi thông báo cho người mua
					err = t.taskDistributor.DistributeTaskSendNotification(ctx, &worker.PayloadSendNotification{
						RecipientID: orderDelivery.BuyerID,
						Title:       fmt.Sprintf("Đơn hàng #%s đã được giao thành công", orderDelivery.OrderCode),
						Message:     fmt.Sprintf("Đơn hàng #%s đã được giao thành công. Vui lòng kiểm tra và xác nhận đã nhận được hàng trong mục Đơn mua.", orderDelivery.OrderCode),
						Type:        "order",
						ReferenceID: orderDelivery.OrderCode,
					}, opts...)
					if err != nil {
						log.Err(err).Msg("failed to send notification to buyer")
					}
					log.Info().Msgf("Notification sent to buyer: %s", orderDelivery.BuyerID)
					
					/* TODO: Thêm task xử lý hoàn tất đơn hàng (cộng tiền cho người bán, đánh dấu đơn hàng là hoàn tất, v.v.)
					với deadline là 7 ngày sau khi đơn hàng được giao thành công.
					Nếu người mua không xác nhận trong 7 ngày, tự động đánh dấu đơn hàng là hoàn tất.
					Nếu người mua xác nhận đã nhận hàng, hủy task này. (Tạm thời bỏ qua)
					*/
					
					// Gửi thông báo cho người bán
					err = t.taskDistributor.DistributeTaskSendNotification(ctx, &worker.PayloadSendNotification{
						RecipientID: orderDelivery.SellerID,
						Title:       fmt.Sprintf("Đơn hàng #%s đã được giao thành công", orderDelivery.OrderCode),
						Message:     fmt.Sprintf("Đơn hàng #%s đã được giao thành công cho người mua. Số tiền %s sẽ được cộng vào số dư khả dụng của bạn sau khi người mua xác nhận đã nhận được hàng.", orderDelivery.OrderCode, util.FormatVND(orderDelivery.ItemsSubtotal)),
						Type:        "order",
						ReferenceID: orderDelivery.OrderCode,
					}, opts...)
					if err != nil {
						log.Err(err).Msg("failed to send notification to seller")
					}
					log.Info().Msgf("Notification sent to seller: %s", orderDelivery.SellerID)
				
				default:
					// TODO: Xử lý các trường hợp khác
					log.Info().Msgf("Unhandled status change: \"%s\" -> \"%s\"", oldOverallStatus, newOverallStatus)
				}
			}
		}
	}
}
