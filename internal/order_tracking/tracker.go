package ordertracking

import (
	"context"
	"fmt"
	"time"
	
	"github.com/go-co-op/gocron/v2"
	"github.com/hibiken/asynq"
	db "github.com/katatrina/gundam-BE/internal/db/sqlc"
	"github.com/katatrina/gundam-BE/internal/delivery"
	"github.com/katatrina/gundam-BE/internal/util"
	"github.com/katatrina/gundam-BE/internal/worker"
	"github.com/rs/zerolog/log"
)

// OrderTracker là một struct để theo dõi trạng thái đơn hàng trên GHN.
type OrderTracker struct {
	store           db.Store
	taskDistributor *worker.RedisTaskDistributor
	ghnService      delivery.IDeliveryProvider
	scheduler       gocron.Scheduler
}

// NewOrderTracker tạo một tracker mới để theo dõi trạng thái đơn hàng trên GHN.
func NewOrderTracker(store db.Store, deliveryService delivery.IDeliveryProvider, taskDistributor *worker.RedisTaskDistributor) (*OrderTracker, error) {
	scheduler, err := gocron.NewScheduler()
	if err != nil {
		return nil, err
	}
	
	return &OrderTracker{
		store:           store,
		taskDistributor: taskDistributor,
		ghnService:      deliveryService,
		scheduler:       scheduler,
	}, nil
}

// Start bắt đầu chạy cronjob theo dõi trạng thái đơn hàng.
func (t *OrderTracker) Start() error {
	// Tạo job chạy mỗi 10 giây
	_, err := t.scheduler.NewJob(
		gocron.DurationJob(10*time.Second),
		gocron.NewTask(
			func() {
				// log.Info().
				// 	Str("job", "order_status_tracking").
				// 	Time("start_time", time.Now()).
				// 	Msg("Starting order status check job")
				
				t.checkOrderStatus()
			},
		),
	)
	
	if err != nil {
		return err
	}
	
	// Bắt đầu scheduler
	t.scheduler.Start()
	return nil
}

// Stop dừng cronjob theo dõi trạng thái đơn hàng
func (t *OrderTracker) Stop() error {
	return t.scheduler.Shutdown()
}

// checkOrderStatus kiểm tra trạng thái đơn hàng trên GHN và cập nhật vào database.
func (t *OrderTracker) checkOrderStatus() {
	ctx := context.Background()
	
	// Lấy danh sách đơn hàng đang vận chuyển ("picking", "delivering", "return")
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
					Nếu người mua xác nhận đã nhận hàng, hủy task này.
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
