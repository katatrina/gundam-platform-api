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

// handleOrderFailure xử lý khi đơn hàng giao thất bại
func (t *OrderTracker) handleOrderFailure(ctx context.Context, order db.Order) error {
	switch order.Type {
	case db.OrderTypeRegular:
		return t.handleRegularOrderFailure(ctx, order)
	case db.OrderTypeExchange:
		return t.handleExchangeOrderFailure(ctx, order)
	case db.OrderTypeAuction:
		return t.handleAuctionOrderFailure(ctx, order)
	default:
		return fmt.Errorf("unsupported order type: %s", order.Type)
	}
}

// handleRegularOrderFailure xử lý khi đơn hàng thông thường giao thất bại
func (t *OrderTracker) handleRegularOrderFailure(ctx context.Context, order db.Order) error {
	// 1. Lấy thông tin giao dịch đơn hàng
	orderTransaction, err := t.store.GetOrderTransactionByOrderID(ctx, order.ID)
	if err != nil {
		return fmt.Errorf("failed to get transaction: %w", err)
	}
	
	// 2. Lấy bút toán của người mua (để hoàn tiền vào số dư ví)
	buyerEntry, err := t.store.GetWalletEntryByID(ctx, orderTransaction.BuyerEntryID)
	if err != nil {
		return fmt.Errorf("failed to get buyer entry: %w", err)
	}
	
	// Kiểm tra xem bút toán của người bán có tồn tại không
	if orderTransaction.SellerEntryID == nil {
		return fmt.Errorf("seller entry not found for order %s", order.Code)
	}
	
	// 3. Lấy bút toán của người bán (để cập nhật trạng thái bút toán và trừ vào non-withdrawable)
	sellerEntry, err := t.store.GetWalletEntryByID(ctx, *orderTransaction.SellerEntryID)
	if err != nil {
		return fmt.Errorf("failed to get seller entry: %w", err)
	}
	
	// 4. Lấy thông tin order items
	orderItems, err := t.store.ListOrderItems(ctx, order.ID)
	if err != nil {
		return fmt.Errorf("failed to get order items: %w", err)
	}
	
	// 5. Thực hiện transaction hoàn tiền và cập nhật trạng thái
	_, err = t.store.FailRegularOrderTx(ctx, db.FailRegularOrderTxParams{
		FailedOrder:  &order,
		BuyerEntry:   &buyerEntry,
		SellerEntry:  &sellerEntry,
		Transaction:  &orderTransaction,
		OrderItems:   orderItems,
		RefundAmount: buyerEntry.Amount, // Hoàn trả toàn bộ số tiền người mua đã thanh toán
	})
	if err != nil {
		return fmt.Errorf("failed to process regular order failure: %w", err)
	}
	
	log.Info().
		Str("order_id", order.ID.String()).
		Str("buyer_id", order.BuyerID).
		Str("seller_id", order.SellerID).
		Int64("refund_amount", -buyerEntry.Amount).
		Msg("Regular order failure processed successfully")
	
	return nil
}

// handleExchangeOrderFailure xử lý khi đơn hàng trao đổi giao thất bại
func (t *OrderTracker) handleExchangeOrderFailure(ctx context.Context, order db.Order) error {
	// 1. Lấy thông tin exchange từ order
	exchange, err := t.store.GetExchangeByOrderID(ctx, &order.ID)
	if err != nil {
		if errors.Is(err, db.ErrRecordNotFound) {
			return fmt.Errorf("exchange for order ID %s not found", order.ID)
		}
		return err
	}
	
	// 2. Cập nhật exchange status dựa trên updateExchangeStatusIfNeeded
	t.updateExchangeStatusIfNeeded(ctx, order)
	
	// TODO: Xử lý quy trình khi đơn hàng trao đổi thất bại
	
	// Đơn hàng trao đổi sẽ được xử lý qua updateExchangeStatusIfNeeded
	// không cần xử lý riêng về tiền bạc (vì chỉ thanh toán phí vận chuyển)
	log.Info().
		Str("order_id", order.ID.String()).
		Str("exchange_id", exchange.ID.String()).
		Msg("Exchange order failure handled through updateExchangeStatusIfNeeded")
	
	return nil
}

// handleAuctionOrderFailure xử lý khi đơn hàng đấu giá giao thất bại
func (t *OrderTracker) handleAuctionOrderFailure(ctx context.Context, order db.Order) error {
	// 1. Lấy thông tin giao dịch đơn hàng
	orderTransaction, err := t.store.GetOrderTransactionByOrderID(ctx, order.ID)
	if err != nil {
		return fmt.Errorf("failed to get transaction: %w", err)
	}
	
	// 2. Lấy bút toán của người bán (để trừ vào non-withdrawable và cập nhật trạng thái bút toán)
	if orderTransaction.SellerEntryID == nil {
		return fmt.Errorf("seller entry not found for order %s", order.Code)
	}
	sellerEntry, err := t.store.GetWalletEntryByID(ctx, *orderTransaction.SellerEntryID)
	if err != nil {
		return fmt.Errorf("failed to get seller entry: %w", err)
	}
	
	// 3. Lấy bút toán của người mua (để hoàn tiền vào số dư ví)
	buyerEntry, err := t.store.GetWalletEntryByID(ctx, orderTransaction.BuyerEntryID)
	if err != nil {
		return fmt.Errorf("failed to get buyer entry: %w", err)
	}
	
	// 4. Lấy thông tin auction
	auction, err := t.store.GetAuctionByOrderID(ctx, &order.ID)
	if err != nil {
		if errors.Is(err, db.ErrRecordNotFound) {
			return fmt.Errorf("auction for order ID %s not found", order.ID)
		}
		
		return fmt.Errorf("failed to get auction: %w", err)
	}
	
	// 5. Thực hiện transaction hoàn tiền và cập nhật trạng thái
	_, err = t.store.FailAuctionOrderTx(ctx, db.FailAuctionOrderTxParams{
		Order:        &order,
		BuyerEntry:   &buyerEntry,
		SellerEntry:  &sellerEntry,
		Auction:      &auction,
		Transaction:  &orderTransaction,
		RefundAmount: buyerEntry.Amount, // Hoàn trả toàn bộ số tiền người mua đã thanh toán
	})
	if err != nil {
		return fmt.Errorf("failed to process auction order failure: %w", err)
	}
	
	log.Info().
		Str("order_id", order.ID.String()).
		Str("buyer_id", order.BuyerID).
		Str("seller_id", order.SellerID).
		Int64("refund_amount", -buyerEntry.Amount).
		Msg("Auction order failure processed successfully")
	
	return nil
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
		
		ghnStatus := response.Data.Status
		// Nếu trạng thái đã thay đổi, cập nhật vào db
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
			
			// Kiểm tra trạng thái hiện tại của đơn hàng trước khi xử lý
			currentOrder, err := t.store.GetOrderDetails(ctx, orderDelivery.OrderID)
			if err != nil {
				log.Error().Err(err).Str("order_id", orderDelivery.OrderID.String()).Msg("failed to get order")
				continue
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
					
					// Gửi thông báo cho người bán
					err = t.taskDistributor.DistributeTaskSendNotification(ctx, &worker.PayloadSendNotification{
						RecipientID: orderDelivery.SellerID,
						Title:       fmt.Sprintf("Đơn hàng #%s đã được giao thành công", orderDelivery.OrderCode),
						Message: fmt.Sprintf("Đơn hàng #%s đã được giao thành công cho người mua. Số tiền %s sẽ được cộng vào số dư khả dụng của bạn sau khi người mua xác nhận đã nhận được hàng.",
							orderDelivery.OrderCode,
							util.FormatVND(orderDelivery.ItemsSubtotal)),
						Type:        "order",
						ReferenceID: orderDelivery.OrderCode,
					}, opts...)
					if err != nil {
						log.Err(err).Msg("failed to send notification to seller")
					}
					log.Info().Msgf("Notification sent to seller: %s", orderDelivery.SellerID)
				
				// Đơn hàng giao thất bại (failed).
				// Hệ thống chỉ xử lý (cập nhật đơn hàng, hoàn tiền người mua,...) và thông báo duy nhất một lần.
				// Hệ thống không xử lý trường hợp giao lại hàng cho người mua.
				case newOverallStatus == db.DeliveryOverralStatusFailed:
					// Chỉ xử lý nếu đơn hàng chưa ở trạng thái failed
					if currentOrder.OrderStatus != db.OrderStatusFailed {
						updatedOrder, err := t.store.UpdateOrder(ctx, db.UpdateOrderParams{
							OrderID: orderDelivery.OrderID,
							Status: db.NullOrderStatus{
								OrderStatus: db.OrderStatusFailed,
								Valid:       true,
							},
						})
						if err != nil {
							log.Error().Err(err).Str("order_code", orderDelivery.OrderCode).Msg("failed to update order status to failed")
							continue
						}
						log.Info().Str("order_code", orderDelivery.OrderCode).Msgf("order status has been updated to \"%s\"", updatedOrder.Status)
						
						// Kiểm tra và cập nhật giao dịch trao đổi nếu có
						t.updateExchangeStatusIfNeeded(ctx, updatedOrder)
						
						// Xử lý hoàn tiền và cập nhật trạng thái Gundam dựa trên loại đơn hàng
						err = t.handleOrderFailure(ctx, updatedOrder)
						if err != nil {
							log.Error().Err(err).Str("order_code", orderDelivery.OrderCode).Msg("failed to handle order failure")
						}
						
						// Gửi thông báo chi tiết
						t.sendFailureNotifications(ctx, currentOrder, ghnStatus)
					} else {
						log.Info().Str("order_code", orderDelivery.OrderCode).Msg("Order already failed, skipping failed handling")
					}
				
				// Đơn hàng được trả về cho người bán.
				// Hệ thống cũng chỉ xử lý một lần duy nhất giống như trường hợp failed,
				// nhưng có thông báo cho người bán mỗi khi trạng thái vận chuyển thay đổi
				// để giúp người bán dễ dàng nhận lại hàng.
				case newOverallStatus == db.DeliveryOverralStatusReturn:
					// Kiểm tra trạng thái hiện tại của đơn hàng
					orderAlreadyFailed := currentOrder.OrderStatus == db.OrderStatusFailed
					
					// Nếu đơn hàng chưa ở trạng thái failed, cập nhật trạng thái và xử lý hoàn tiền
					if !orderAlreadyFailed {
						// Cập nhật trạng thái đơn hàng thành "failed"
						updatedOrder, err := t.store.UpdateOrder(ctx, db.UpdateOrderParams{
							OrderID: orderDelivery.OrderID,
							Status: db.NullOrderStatus{
								OrderStatus: db.OrderStatusFailed,
								Valid:       true,
							},
						})
						if err != nil {
							log.Error().Err(err).Str("order_code", orderDelivery.OrderCode).Msg("failed to update order status to failed")
							continue
						}
						log.Info().Str("order_code", orderDelivery.OrderCode).Msgf("order status has been updated to \"%s\" due to return", updatedOrder.Status)
						
						// Kiểm tra và cập nhật giao dịch trao đổi nếu có
						t.updateExchangeStatusIfNeeded(ctx, updatedOrder)
						
						// Xử lý hoàn tiền và cập nhật trạng thái Gundam dựa trên loại đơn hàng (giống như xử lý thất bại)
						err = t.handleOrderFailure(ctx, updatedOrder)
						if err != nil {
							log.Error().Err(err).Str("order_code", orderDelivery.OrderCode).Msg("failed to handle order failure due to return")
						}
					}
					
					// Gửi thông báo chi tiết (luôn gửi cho người bán, nhưng chỉ gửi cho người mua nếu đơn hàng mới failed)
					t.sendReturnNotifications(ctx, currentOrder, ghnStatus, orderAlreadyFailed)
				
				default:
					// TODO: Xử lý các trường hợp khác
					log.Info().Msgf("Unhandled status change: \"%s\" -> \"%s\"", oldOverallStatus, newOverallStatus)
				}
			}
		}
	}
}
