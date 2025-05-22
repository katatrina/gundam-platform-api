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
// và cập nhật trạng thái của cuộc trao đổi dựa trên trạng thái của đơn hàng.
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

// getDetailedReturnNotification tạo thông báo chi tiết dựa trên trạng thái trả hàng cụ thể
func (t *OrderTracker) getDetailedReturnNotification(orderCode, ghnStatus string) (title, message string) {
	switch ghnStatus {
	case "waiting_to_return":
		title = fmt.Sprintf("Đơn hàng %s đang chờ trả về", orderCode)
		message = fmt.Sprintf("Đơn hàng %s đã được đánh dấu là cần trả về cho bạn. Đơn vị vận chuyển sẽ sớm thu gom và gửi lại hàng.", orderCode)
	
	case "return":
		title = fmt.Sprintf("Đơn hàng %s đang được thu gom để trả về", orderCode)
		message = fmt.Sprintf("Đơn hàng %s đang được đơn vị vận chuyển thu gom để trả về cho bạn.", orderCode)
	
	case "return_transporting":
		title = fmt.Sprintf("Đơn hàng %s đang được vận chuyển để trả về", orderCode)
		message = fmt.Sprintf("Đơn hàng %s đang được vận chuyển trả về cho bạn. Quá trình này có thể mất vài ngày tùy thuộc vào khoảng cách.", orderCode)
	
	case "return_sorting":
		title = fmt.Sprintf("Đơn hàng %s đang phân loại để trả về", orderCode)
		message = fmt.Sprintf("Đơn hàng %s đang được phân loại tại kho của đơn vị vận chuyển và sẽ sớm được gửi trả về cho bạn.", orderCode)
	
	case "returning":
		title = fmt.Sprintf("Đơn hàng %s đang được giao trả", orderCode)
		message = fmt.Sprintf("Đơn hàng %s đang được shipper mang đến trả lại cho bạn. Vui lòng chuẩn bị nhận hàng trong ngày hôm nay.", orderCode)
	
	case "return_fail":
		title = fmt.Sprintf("Trả hàng thất bại cho đơn %s", orderCode)
		message = fmt.Sprintf("Đơn vị vận chuyển không thể trả hàng cho đơn %s. Vui lòng liên hệ với đơn vị vận chuyển GHN qua hotline 1900.2042 để biết thêm chi tiết và sắp xếp lại việc nhận hàng.", orderCode)
	
	case "returned":
		title = fmt.Sprintf("Đơn hàng %s đã được trả về thành công", orderCode)
		message = fmt.Sprintf("Đơn hàng %s đã được trả về thành công. Vui lòng kiểm tra lại tình trạng sản phẩm.", orderCode)
	
	default:
		title = fmt.Sprintf("Cập nhật về đơn hàng %s đang trả về", orderCode)
		message = fmt.Sprintf("Đơn hàng %s đang trong quá trình trả về cho bạn. Vui lòng theo dõi cập nhật tiếp theo.", orderCode)
	}
	
	return title, message
}

// getDetailedFailureNotification tạo thông báo chi tiết dựa trên trạng thái thất bại cụ thể
func (t *OrderTracker) getDetailedFailureNotification(orderCode, ghnStatus string) (title, message string) {
	switch ghnStatus {
	case "delivery_fail":
		title = fmt.Sprintf("Giao hàng thất bại cho đơn %s", orderCode)
		message = fmt.Sprintf("Đơn vị vận chuyển không thể giao hàng cho đơn %s. Hàng sẽ được trả về cho người bán.", orderCode)
	
	case "cancel":
		title = fmt.Sprintf("Đơn hàng %s đã bị hủy", orderCode)
		message = fmt.Sprintf("Đơn hàng %s đã bị hủy trong quá trình vận chuyển. Hàng sẽ được trả về cho người bán.", orderCode)
	
	case "exception":
		title = fmt.Sprintf("Đơn hàng %s gặp sự cố bất thường", orderCode)
		message = fmt.Sprintf("Đơn hàng %s gặp sự cố bất thường trong quá trình vận chuyển. Hàng sẽ được trả về cho người bán.", orderCode)
	
	case "damage":
		title = fmt.Sprintf("Đơn hàng %s bị hư hỏng", orderCode)
		message = fmt.Sprintf("Đơn hàng %s bị hư hỏng trong quá trình vận chuyển. Hàng sẽ được trả về cho người bán.", orderCode)
	
	case "lost":
		title = fmt.Sprintf("Đơn hàng %s bị mất", orderCode)
		message = fmt.Sprintf("Đơn hàng #%s bị mất trong quá trình vận chuyển. Vui lòng liên hệ với đơn vị vận chuyển để biết thêm chi tiết.", orderCode)
	
	default:
		title = fmt.Sprintf("Đơn hàng %s không thể hoàn thành", orderCode)
		message = fmt.Sprintf("Đơn hàng %s không thể hoàn thành giao dịch. Hàng sẽ được trả về cho người bán.", orderCode)
	}
	
	return title, message
}

// Xử lý gửi thông báo cho trường hợp return
func (t *OrderTracker) sendReturnNotifications(ctx context.Context, orderDetails db.GetOrderDetailsRow, ghnStatus string, orderAlreadyFailed bool) {
	opts := []asynq.Option{
		asynq.MaxRetry(3),
		asynq.Queue(worker.QueueCritical),
	}
	
	title, message := t.getDetailedReturnNotification(orderDetails.OrderCode, ghnStatus)
	
	// Nếu đơn hàng chưa failed, gửi thông báo cho cả người mua và người bán
	if !orderAlreadyFailed {
		// Gửi thông báo cho người mua
		buyerMessage := fmt.Sprintf("%s. Số tiền đã thanh toán đã được hoàn trả vào ví của bạn.", message)
		err := t.taskDistributor.DistributeTaskSendNotification(ctx, &worker.PayloadSendNotification{
			RecipientID: orderDetails.BuyerID,
			Title:       title,
			Message:     buyerMessage,
			Type:        "order",
			ReferenceID: orderDetails.OrderCode,
		}, opts...)
		if err != nil {
			log.Err(err).Msg("failed to send return notification to buyer")
		} else {
			log.Info().Str("buyer_id", orderDetails.BuyerID).Str("order_code", orderDetails.OrderCode).Msg("Sent return notification to buyer")
		}
	}
	
	// Luôn gửi thông báo chi tiết cho người bán
	err := t.taskDistributor.DistributeTaskSendNotification(ctx, &worker.PayloadSendNotification{
		RecipientID: orderDetails.SellerID,
		Title:       title,
		Message:     message,
		Type:        "order",
		ReferenceID: orderDetails.OrderCode,
	}, opts...)
	if err != nil {
		log.Err(err).Msg("failed to send return notification to seller")
	} else {
		log.Info().
			Str("seller_id", orderDetails.SellerID).
			Str("order_code", orderDetails.OrderCode).
			Str("status", ghnStatus).
			Bool("order_already_failed", orderAlreadyFailed).
			Msg("Sent return notification to seller")
	}
}

// Xử lý gửi thông báo cho trường hợp failed chỉ cho đơn thông thường và đơn đấu giá
func (t *OrderTracker) sendFailureNotifications(ctx context.Context, orderDetails db.GetOrderDetailsRow, ghnStatus string) {
	opts := []asynq.Option{
		asynq.MaxRetry(3),
		asynq.Queue(worker.QueueCritical),
	}
	
	title, message := t.getDetailedFailureNotification(orderDetails.OrderCode, ghnStatus)
	
	// Gửi thông báo cho người mua
	buyerMessage := fmt.Sprintf("%s. Số tiền đã thanh toán đã được hoàn trả vào ví của bạn.", message)
	err := t.taskDistributor.DistributeTaskSendNotification(ctx, &worker.PayloadSendNotification{
		RecipientID: orderDetails.BuyerID,
		Title:       title,
		Message:     buyerMessage,
		Type:        "order",
		ReferenceID: orderDetails.OrderCode,
	}, opts...)
	if err != nil {
		log.Err(err).Msg("failed to send failure notification to buyer")
	} else {
		log.Info().Str("buyer_id", orderDetails.BuyerID).Str("order_code", orderDetails.OrderCode).Msg("Sent failure notification to buyer")
	}
	
	// Gửi thông báo chi tiết cho người bán
	sellerMessage := fmt.Sprintf("%s. Vui lòng chuẩn bị nhận lại sản phẩm.", message)
	err = t.taskDistributor.DistributeTaskSendNotification(ctx, &worker.PayloadSendNotification{
		RecipientID: orderDetails.SellerID,
		Title:       title,
		Message:     sellerMessage,
		Type:        "order",
		ReferenceID: orderDetails.OrderCode,
	}, opts...)
	if err != nil {
		log.Err(err).Msg("failed to send failure notification to seller")
	} else {
		log.Info().Str("seller_id", orderDetails.SellerID).Str("order_code", orderDetails.OrderCode).Msg("Sent failure notification to seller")
	}
}

// handlePickingToDelivering xử lý khi đơn hàng chuyển từ picking sang delivering.
func (t *OrderTracker) handlePickingToDelivering(ctx context.Context, currentOrder db.GetOrderDetailsRow) {
	updatedOrder, err := t.store.UpdateOrder(ctx, db.UpdateOrderParams{
		OrderID: currentOrder.OrderID,
		Status: db.NullOrderStatus{
			OrderStatus: db.OrderStatusDelivering,
			Valid:       true,
		},
	})
	if err != nil {
		log.Error().Err(err).Str("order_code", currentOrder.OrderCode).Msg("failed to update order status to delivering")
		return
	}
	log.Info().Str("order_code", currentOrder.OrderCode).Msgf("order status has been updated to \"%s\"", updatedOrder.Status)
	
	// Kiểm tra và cập nhật trạng thái cuộc trao đổi nếu cần
	t.updateExchangeStatusIfNeeded(ctx, updatedOrder)
	
	opts := []asynq.Option{
		asynq.MaxRetry(3),
		asynq.Queue(worker.QueueCritical),
	}
	
	// Gửi thông báo cho người nhận
	err = t.taskDistributor.DistributeTaskSendNotification(ctx, &worker.PayloadSendNotification{
		RecipientID: currentOrder.BuyerID,
		Title:       fmt.Sprintf("Đơn hàng %s đã được bàn giao cho đơn vị vận chuyển", currentOrder.OrderCode),
		// TODO: Chỉnh sửa lại message ở câu cuối để phù hợp với page trên FE
		Message:     fmt.Sprintf("Đơn hàng %s đã được bàn giao cho đơn vị vận chuyển và chuẩn bị giao đến cho bạn. Vui lòng theo dõi chi tiết đơn hàng trong trang Quản lý đơn hàng.", currentOrder.OrderCode),
		Type:        "order",
		ReferenceID: currentOrder.OrderCode,
	}, opts...)
	if err != nil {
		log.Err(err).Msg("failed to send notification to buyer")
	}
	
	// Gửi thông báo cho người gửi
	err = t.taskDistributor.DistributeTaskSendNotification(ctx, &worker.PayloadSendNotification{
		RecipientID: currentOrder.SellerID,
		Title:       fmt.Sprintf("Đơn hàng %s đã được bàn giao cho đơn vị vận chuyển", currentOrder.OrderCode),
		Message:     fmt.Sprintf("Đơn hàng %s đã được bàn giao cho đơn vị vận chuyển và chuẩn bị giao đến người nhận.", currentOrder.OrderCode),
		Type:        "order",
		ReferenceID: currentOrder.OrderCode,
	}, opts...)
	if err != nil {
		log.Err(err).Msg("failed to send notification to seller")
	}
}

// handleDeliveringToDelivered xử lý khi đơn hàng chuyển từ delivering sang delivered.
func (t *OrderTracker) handleDeliveringToDelivered(ctx context.Context, currentOrder db.GetOrderDetailsRow) {
	updatedOrder, err := t.store.UpdateOrder(ctx, db.UpdateOrderParams{
		OrderID: currentOrder.OrderID,
		Status: db.NullOrderStatus{
			OrderStatus: db.OrderStatusDelivered,
			Valid:       true,
		},
	})
	if err != nil {
		log.Error().Err(err).Str("order_code", currentOrder.OrderCode).Msg("failed to update order status to delivered")
		return
	}
	log.Info().Str("order_code", updatedOrder.Code).Msgf("order status has been updated to \"%s\"", updatedOrder.Status)
	
	// Kiểm tra và cập nhật giao dịch trao đổi nếu có
	t.updateExchangeStatusIfNeeded(ctx, updatedOrder)
	
	opts := []asynq.Option{
		asynq.MaxRetry(3),
		asynq.Queue(worker.QueueCritical),
	}
	
	// Gửi thông báo cho người nhận hàng
	err = t.taskDistributor.DistributeTaskSendNotification(ctx, &worker.PayloadSendNotification{
		RecipientID: updatedOrder.BuyerID,
		Title:       fmt.Sprintf("Đơn hàng %s đã được giao thành công", updatedOrder.Code),
		Message:     fmt.Sprintf("Đơn hàng %s đã được giao thành công. Vui lòng kiểm tra và xác nhận đã nhận được hàng trong trang Quản lý đơn hàng.", updatedOrder.Code),
		Type:        "order",
		ReferenceID: updatedOrder.Code,
	}, opts...)
	if err != nil {
		log.Err(err).Msg("failed to send notification to buyer")
	}
	
	// Gửi thông báo cho người gửi hàng
	err = t.taskDistributor.DistributeTaskSendNotification(ctx, &worker.PayloadSendNotification{
		RecipientID: updatedOrder.SellerID,
		Title:       fmt.Sprintf("Đơn hàng %s đã được giao thành công", updatedOrder.Code),
		Message: fmt.Sprintf("Đơn hàng %s đã được giao thành công cho người nhận. Số tiền %s sẽ được cộng vào số dư khả dụng của bạn sau khi người nhận xác nhận đã nhận được hàng.",
			updatedOrder.Code,
			util.FormatVND(updatedOrder.ItemsSubtotal)),
		Type:        "order",
		ReferenceID: updatedOrder.Code,
	}, opts...)
	if err != nil {
		log.Err(err).Msg("failed to send notification to seller")
	}
}

// handleFailedToDelivering xử lý khi đơn hàng chuyển từ failed sang delivering.
func (t *OrderTracker) handleFailedToDelivering(ctx context.Context, currentOrder db.GetOrderDetailsRow) {
	updatedOrder, err := t.store.UpdateOrder(ctx, db.UpdateOrderParams{
		OrderID: currentOrder.OrderID,
		Status: db.NullOrderStatus{
			OrderStatus: db.OrderStatusDelivering,
			Valid:       true,
		},
	})
	if err != nil {
		log.Error().Err(err).Str("order_code", currentOrder.OrderCode).Msg("failed to update order status to delivering")
		return
	}
	log.Info().Str("order_code", updatedOrder.Code).Msgf("order status has been updated to \"%s\"", updatedOrder.Status)
	
	// Kiểm tra và cập nhật giao dịch trao đổi nếu có
	t.updateExchangeStatusIfNeeded(ctx, updatedOrder)
	
	opts := []asynq.Option{
		asynq.MaxRetry(3),
		asynq.Queue(worker.QueueCritical),
	}
	
	// Gửi thông báo cho người nhận hàng
	err = t.taskDistributor.DistributeTaskSendNotification(ctx, &worker.PayloadSendNotification{
		RecipientID: updatedOrder.BuyerID,
		Title:       fmt.Sprintf("Đơn hàng %s chuẩn bị giao lại cho bạn", updatedOrder.Code),
		Message: fmt.Sprintf("Đơn hàng %s đã được bàn giao lại cho đơn vị vận chuyển và chuẩn bị giao lại đến bạn. Ngày giờ dự kiến giao hàng lại: %s",
			updatedOrder.Code,
			currentOrder.ExpectedDeliveryTime.Format("15:04 02/01/2006")),
		Type:        "order",
		ReferenceID: updatedOrder.Code,
	}, opts...)
	if err != nil {
		log.Err(err).Msg("failed to send notification to buyer")
	}
	
	// Gửi thông báo cho người gửi hàng
	err = t.taskDistributor.DistributeTaskSendNotification(ctx, &worker.PayloadSendNotification{
		RecipientID: updatedOrder.SellerID,
		Title:       fmt.Sprintf("Đơn hàng %s đã được bàn giao lại cho đơn vị vận chuyển", updatedOrder.Code),
		Message:     fmt.Sprintf("Đơn hàng %s đã được bàn giao lại cho đơn vị vận chuyển và chuẩn bị giao lại cho người nhận.", updatedOrder.Code),
		Type:        "order",
		ReferenceID: updatedOrder.Code,
	}, opts...)
	if err != nil {
		log.Err(err).Msg("failed to send notification to seller")
	}
}

// handleOrderStatusFailed xử lý khi đơn hàng chuyển sang trạng thái failed
func (t *OrderTracker) handleOrderStatusFailed(ctx context.Context, currentOrder db.GetOrderDetailsRow, ghnStatus string) {
	// Chỉ xử lý nếu đơn hàng chưa ở trạng thái failed
	if currentOrder.OrderStatus != db.OrderStatusFailed {
		updatedOrder, err := t.store.UpdateOrder(ctx, db.UpdateOrderParams{
			OrderID: currentOrder.OrderID,
			Status: db.NullOrderStatus{
				OrderStatus: db.OrderStatusFailed,
				Valid:       true,
			},
		})
		if err != nil {
			log.Error().Err(err).Str("order_code", currentOrder.OrderCode).Msg("failed to update order status to failed")
			return
		}
		log.Info().Str("order_code", currentOrder.OrderCode).Msgf("order status has been updated to \"%s\"", updatedOrder.Status)
		
		// Xử lý hoàn tiền và cập nhật trạng thái Gundam dựa trên loại đơn hàng
		err = t.handleOrderFailure(ctx, updatedOrder)
		if err != nil {
			log.Error().Err(err).Str("order_code", currentOrder.OrderCode).Msg("failed to handle order failure")
		}
		
		// Gửi thông báo chi tiết chỉ cho đơn thông thường và đơn đấu giá
		// Vì đơn trao đổi đã được xử lý trong hàm handleOrderFailure rồi
		if currentOrder.OrderType == db.OrderTypeRegular || currentOrder.OrderType == db.OrderTypeExchange {
			t.sendFailureNotifications(ctx, currentOrder, ghnStatus)
		}
		
	} else {
		log.Info().Str("order_code", currentOrder.OrderCode).Msg("Order already failed, skipping failed handling")
	}
}

// handleOrderStatusReturn xử lý khi đơn hàng chuyển sang trạng thái return
func (t *OrderTracker) handleOrderStatusReturn(ctx context.Context, currentOrder db.GetOrderDetailsRow, ghnStatus string) {
	// Không xử lý đơn hàng trao đổi trong trạng thái return (sẽ được giao lại cho người nhận tới khi thành công, không trả lại hàng cho nguời gửi)
	if currentOrder.OrderType == db.OrderTypeExchange {
		log.Info().Str("order_code", currentOrder.OrderCode).Msg("Exchange order in return status - will be retried, no processing needed")
		return
	}
	
	// Kiểm tra trạng thái hiện tại của đơn hàng
	orderAlreadyFailed := currentOrder.OrderStatus == db.OrderStatusFailed
	
	// Nếu đơn hàng chưa ở trạng thái failed, cập nhật trạng thái đơn hàng và xử lý hoàn tiền
	if !orderAlreadyFailed {
		updatedOrder, err := t.store.UpdateOrder(ctx, db.UpdateOrderParams{
			OrderID: currentOrder.OrderID,
			Status: db.NullOrderStatus{
				OrderStatus: db.OrderStatusFailed,
				Valid:       true,
			},
		})
		if err != nil {
			log.Error().Err(err).Str("order_code", currentOrder.OrderCode).Msg("failed to update order status to failed")
			return
		}
		log.Info().Str("order_code", currentOrder.OrderCode).Msgf("order status has been updated to \"%s\" due to return", updatedOrder.Status)
		
		// Kiểm tra và cập nhật giao dịch trao đổi nếu có
		t.updateExchangeStatusIfNeeded(ctx, updatedOrder)
		
		// Xử lý hoàn tiền và cập nhật trạng thái Gundam dựa trên loại đơn hàng (giống như xử lý thất bại)
		err = t.handleOrderFailure(ctx, updatedOrder)
		if err != nil {
			log.Error().Err(err).Str("order_code", currentOrder.OrderCode).Msg("failed to handle order failure due to return")
		}
		
		// Gửi thông báo chi tiết (luôn gửi cho người bán, nhưng chỉ gửi cho người mua nếu đơn hàng mới failed)
		t.sendReturnNotifications(ctx, currentOrder, ghnStatus, orderAlreadyFailed)
	}
}
