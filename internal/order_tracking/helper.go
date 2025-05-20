package ordertracking

import (
	"context"
	"fmt"
	
	"github.com/hibiken/asynq"
	db "github.com/katatrina/gundam-BE/internal/db/sqlc"
	"github.com/katatrina/gundam-BE/internal/worker"
	"github.com/rs/zerolog/log"
)

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
		message = fmt.Sprintf("Đơn vị vận chuyển không thể giao hàng cho đơn #%s. Hàng sẽ được trả về cho người bán.", orderCode)
	
	case "cancel":
		title = fmt.Sprintf("Đơn hàng %s đã bị hủy", orderCode)
		message = fmt.Sprintf("Đơn hàng %s đã bị hủy trong quá trình vận chuyển. Hàng sẽ được trả về cho người bán.", orderCode)
	
	case "exception":
		title = fmt.Sprintf("Đơn hàng %s gặp sự cố bất thường", orderCode)
		message = fmt.Sprintf("Đơn hàng %s gặp sự cố bất thường trong quá trình vận chuyển. Hàng sẽ được trả về cho người bán.", orderCode)
	
	case "damage":
		title = fmt.Sprintf("Đơn hàng #%s bị hư hỏng", orderCode)
		message = fmt.Sprintf("Đơn hàng #%s bị hư hỏng trong quá trình vận chuyển. Hàng sẽ được trả về cho người bán.", orderCode)
	
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
func (t *OrderTracker) sendReturnNotifications(ctx context.Context, orderDelivery db.GetOrderDetailsRow, ghnStatus string, orderAlreadyFailed bool) {
	opts := []asynq.Option{
		asynq.MaxRetry(3),
		asynq.Queue(worker.QueueCritical),
	}
	
	title, message := t.getDetailedReturnNotification(orderDelivery.OrderCode, ghnStatus)
	
	// Nếu đơn hàng chưa failed, gửi thông báo cho cả người mua và người bán
	if !orderAlreadyFailed {
		// Gửi thông báo cho người mua
		buyerMessage := fmt.Sprintf("%s. Số tiền đã thanh toán đã được hoàn trả vào ví của bạn.", message)
		err := t.taskDistributor.DistributeTaskSendNotification(ctx, &worker.PayloadSendNotification{
			RecipientID: orderDelivery.BuyerID,
			Title:       title,
			Message:     buyerMessage,
			Type:        "order",
			ReferenceID: orderDelivery.OrderCode,
		}, opts...)
		if err != nil {
			log.Err(err).Msg("failed to send return notification to buyer")
		} else {
			log.Info().Str("buyer_id", orderDelivery.BuyerID).Str("order_code", orderDelivery.OrderCode).Msg("Sent return notification to buyer")
		}
	}
	
	// Luôn gửi thông báo chi tiết cho người bán
	err := t.taskDistributor.DistributeTaskSendNotification(ctx, &worker.PayloadSendNotification{
		RecipientID: orderDelivery.SellerID,
		Title:       title,
		Message:     message,
		Type:        "order",
		ReferenceID: orderDelivery.OrderCode,
	}, opts...)
	if err != nil {
		log.Err(err).Msg("failed to send return notification to seller")
	} else {
		log.Info().
			Str("seller_id", orderDelivery.SellerID).
			Str("order_code", orderDelivery.OrderCode).
			Str("status", ghnStatus).
			Bool("order_already_failed", orderAlreadyFailed).
			Msg("Sent return notification to seller")
	}
}

// Xử lý gửi thông báo cho trường hợp failed
func (t *OrderTracker) sendFailureNotifications(ctx context.Context, orderDelivery db.GetOrderDetailsRow, ghnStatus string) {
	opts := []asynq.Option{
		asynq.MaxRetry(3),
		asynq.Queue(worker.QueueCritical),
	}
	
	title, message := t.getDetailedFailureNotification(orderDelivery.OrderCode, ghnStatus)
	
	// Gửi thông báo cho người mua
	buyerMessage := fmt.Sprintf("%s. Số tiền đã thanh toán đã được hoàn trả vào ví của bạn.", message)
	err := t.taskDistributor.DistributeTaskSendNotification(ctx, &worker.PayloadSendNotification{
		RecipientID: orderDelivery.BuyerID,
		Title:       title,
		Message:     buyerMessage,
		Type:        "order",
		ReferenceID: orderDelivery.OrderCode,
	}, opts...)
	if err != nil {
		log.Err(err).Msg("failed to send failure notification to buyer")
	} else {
		log.Info().Str("buyer_id", orderDelivery.BuyerID).Str("order_code", orderDelivery.OrderCode).Msg("Sent failure notification to buyer")
	}
	
	// Gửi thông báo chi tiết cho người bán
	sellerMessage := fmt.Sprintf("%s. Vui lòng chuẩn bị nhận lại sản phẩm.", message)
	err = t.taskDistributor.DistributeTaskSendNotification(ctx, &worker.PayloadSendNotification{
		RecipientID: orderDelivery.SellerID,
		Title:       title,
		Message:     sellerMessage,
		Type:        "order",
		ReferenceID: orderDelivery.OrderCode,
	}, opts...)
	if err != nil {
		log.Err(err).Msg("failed to send failure notification to seller")
	} else {
		log.Info().Str("seller_id", orderDelivery.SellerID).Str("order_code", orderDelivery.OrderCode).Msg("Sent failure notification to seller")
	}
}
