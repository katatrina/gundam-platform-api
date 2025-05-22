package ordertracking

import (
	"context"
	"errors"
	"fmt"
	
	"github.com/hibiken/asynq"
	db "github.com/katatrina/gundam-BE/internal/db/sqlc"
	"github.com/katatrina/gundam-BE/internal/worker"
	"github.com/rs/zerolog/log"
)

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
// Đơn hàng trao đổi sẽ được giao lại cho đến khi thành công thay vì hoàn tiền ngay lập tức
func (t *OrderTracker) handleExchangeOrderFailure(ctx context.Context, order db.Order) error {
	// 1. Lấy thông tin exchange từ order
	exchange, err := t.store.GetExchangeByOrderID(ctx, &order.ID)
	if err != nil {
		if errors.Is(err, db.ErrRecordNotFound) {
			return fmt.Errorf("exchange for order ID %s not found", order.ID)
		}
		return err
	}
	
	// 2. Cập nhật exchange status dựa trên trạng thái đơn hàng
	t.updateExchangeStatusIfNeeded(ctx, order)
	
	// 3. Gửi thông báo cho người nhận về việc sẽ giao lại
	opts := []asynq.Option{
		asynq.MaxRetry(3),
		asynq.Queue(worker.QueueDefault),
	}
	
	title := fmt.Sprintf("Đơn hàng trao đổi %s giao không thành công", order.Code)
	message := fmt.Sprintf("Đơn hàng trao đổi %s giao không thành côngg. Đơn vị vận chuyển sẽ giao lại cho bạn sớm nhất có thể. Vui lòng chú ý đến thời gian giao hàng và nhận cuộc gọi từ nhân viên vận chuyển.",
		order.Code)
	
	err = t.taskDistributor.DistributeTaskSendNotification(ctx, &worker.PayloadSendNotification{
		RecipientID: order.BuyerID,
		Title:       title,
		Message:     message,
		Type:        "exchange",
		ReferenceID: exchange.ID.String(),
	}, opts...)
	if err != nil {
		log.Error().Err(err).Str("order_id", order.ID.String()).Msg("failed to send retry notification for exchange order")
	}
	
	log.Info().
		Str("order_id", order.ID.String()).
		Str("exchange_id", exchange.ID.String()).
		Msg("Exchange order failure handled")
	
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
		
		ghnStatus := response.Data.Status // status vận chuyển mới nhất từ GHN
		// So sánh với status vận chuyển hiện tại trong db
		if ghnStatus != *orderDelivery.Status {
			log.Info(). // status đã thay đổi, chuẩn bị cập nhật lại thông tin
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
				ID:                   orderDelivery.ID,
				Status:               &ghnStatus,
				ExpectedDeliveryTime: &response.Data.LeadTime, // Luôn cập nhật lại thời gian dự kiến giao hàng mới nhất
			}
			
			// Chỉ cập nhật overall_status nếu có thay đổi
			if isOverallStatusChanged {
				updateParams.OverallStatus = db.NullDeliveryOverralStatus{
					DeliveryOverralStatus: newOverallStatus,
					Valid:                 true,
				}
			}
			
			// Thực hiện cập nhật thông tin cho bảng order_deliveries
			updatedOrderDelivery, err := t.store.UpdateOrderDelivery(ctx, updateParams)
			if err != nil {
				log.Error().Err(err).Str("delivery_tracking_code", *orderDelivery.DeliveryTrackingCode).Msg("failed to update delivery status")
				continue
			}
			log.Info().Str("order_code", orderDelivery.OrderCode).Msgf("order-delivery status has been updated to \"%s\"", *updatedOrderDelivery.Status)
			
			if isOverallStatusChanged {
				log.Info().Str("order_code", orderDelivery.OrderCode).Msgf("order-delivery overall status has been updated to \"%s\"", updatedOrderDelivery.OverallStatus.DeliveryOverralStatus)
			}
			
			// Lấy thông tin chi tiết của đơn hàng trong db
			currentOrder, err := t.store.GetOrderDetails(ctx, orderDelivery.OrderID)
			if err != nil {
				log.Error().Err(err).Str("order_id", orderDelivery.OrderID.String()).Msg("failed to get order")
				continue
			}
			
			// Xử lý business logic theo quy trình từng bước
			// Chỉ xử lý khi có sự chuyển đổi giữa các trạng thái tổng quát
			if isOverallStatusChanged {
				switch {
				// picking -> delivering: Đơn hàng của người gửi đã được shipper đến lấy và chuẩn bị giao cho người nhận ✅.
				case oldOverallStatus == db.DeliveryOverralStatusPicking && newOverallStatus == db.DeliveryOverralStatusDelivering:
					t.handlePickingToDelivering(ctx, currentOrder)
				
				// delivering -> delivered: Đơn hàng đã được giao thành công cho người nhận ✅.
				case oldOverallStatus == db.DeliveryOverralStatusDelivering && newOverallStatus == db.DeliveryOverralStatusDelivered:
					t.handleDeliveringToDelivered(ctx, currentOrder)
				
				// Đơn hàng giao thất bại (failed group) ✅.
				// Đối với đơn thông thường và đơn đấu giá:
				// Hệ thống chỉ xử lý kết thúc đơn hàng (cập nhật trạng thái đơn hàng, hoàn tiền người mua,...) và gửi thông báo
				// khi order_deliveries.status rơi vào nhóm trạng thái failed ở lần đầu tiên.
				// Các lần sau đó sẽ không xử lý nữa.
				// Đối với đơn trao đổi:
				// Đơn vị vận chuyển sẽ lưu kho và giao lại cho tới khi giao thành công cho người nhận (delivered).
				case newOverallStatus == db.DeliveryOverralStatusFailed:
					t.handleOrderStatusFailed(ctx, currentOrder, ghnStatus)
				
				// failed -> delivering (chỉ cho đơn trao đổi): Đơn hàng đang được giao lại cho người nhận sau khi giao thất bại ✅.
				case oldOverallStatus == db.DeliveryOverralStatusFailed && newOverallStatus == db.DeliveryOverralStatusDelivering:
					if currentOrder.OrderType == db.OrderTypeExchange {
						t.handleFailedToDelivering(ctx, currentOrder)
					} else {
						log.Info().Msgf("Order %s is not an exchange order, skipping failed to delivering", currentOrder.OrderCode)
					}
				
				// Đơn hàng được trả về cho người bán (return group) ✅.
				// Đối với đơn thông thường và đơn đấu giá:
				// Hệ thống cũng chỉ xử lý một lần duy nhất giống như trường hợp failed,
				// nhưng có thông báo cho người bán mỗi khi trạng thái vận chuyển (ghnStatus) thay đổi
				// để giúp người bán dễ dàng nhận lại hàng.
				// Đối với đơn trao đổi:
				// Không nên switch status vào nhóm này.
				// Nếu vẫn switch thì hệ thống sẽ không xử lý gì cả, và đơn vận chuyển có khả năng không thế giao lại cho người nhận.
				case newOverallStatus == db.DeliveryOverralStatusReturn:
					if currentOrder.OrderType == db.OrderTypeExchange {
						log.Warn().Str("order_code", currentOrder.OrderCode).Msgf("Exchange order in return group - new status %s, this should not happen", ghnStatus)
						continue
					}
					
					t.handleOrderStatusReturn(ctx, currentOrder, ghnStatus)
				
				default:
					log.Info().Msgf("Unhandled status change: \"%s\" -> \"%s\"", oldOverallStatus, newOverallStatus)
				}
			}
		}
	}
}
