package ordertracking

import (
	"context"
	"errors"
	"fmt"
	"time"
	
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	db "github.com/katatrina/gundam-BE/internal/db/sqlc"
	"github.com/katatrina/gundam-BE/internal/util"
	"github.com/katatrina/gundam-BE/internal/worker"
	"github.com/rs/zerolog/log"
)

// autoCompleteDeliveredOrders tự động hoàn tất các đơn hàng đã giao nhưng chưa được xác nhận sau 7 ngày.
func (t *OrderTracker) autoCompleteDeliveredOrders() {
	ctx := context.Background()
	
	// Lấy danh sách đơn hàng đã giao (delivered) hơn 7 ngày (so với updated_at) và chưa hoàn tất
	sevenDaysAgo := time.Now().AddDate(0, 0, -7)
	
	log.Info().
		Str("job", "auto_complete_orders").
		Time("cutoff_date", sevenDaysAgo).
		Msg("Scanning for orders to auto-complete")
	
	orders, err := t.store.GetDeliveredOrdersToAutoComplete(ctx, sevenDaysAgo)
	if err != nil {
		log.Error().Err(err).Msg("failed to get orders for auto-completion")
		return
	}
	
	log.Info().Int("count", len(orders)).Msg("Found orders to auto-complete")
	
	for _, order := range orders {
		log.Info().
			Str("order_id", order.ID.String()).
			Str("order_code", order.Code).
			Time("delivered_since", order.UpdatedAt).
			Msg("Auto-completing order after 7 days")
		
		// Xử lý tự động hoàn tất đơn hàng dựa vào loại đơn hàng
		switch order.Type {
		case db.OrderTypeRegular:
			// Xử lý đơn hàng thông thường
			err := t.autoCompleteRegularOrder(ctx, order)
			if err != nil {
				log.Error().
					Err(err).
					Str("order_id", order.ID.String()).
					Str("order_code", order.Code).
					Msg("Failed to auto-complete regular order")
				continue
			}
		
		case db.OrderTypeExchange:
			// Xử lý đơn hàng trao đổi
			err := t.autoCompleteExchangeOrder(ctx, order)
			if err != nil {
				log.Error().
					Err(err).
					Str("order_id", order.ID.String()).
					Str("order_code", order.Code).
					Msg("Failed to auto-complete exchange order")
				continue
			}
		
		case db.OrderTypeAuction:
			// Xử lý như đơn hàng thông thường
			// Bởi vì đơn hàng đấu giá không ảnh hưởng dến phiên đấu giá đã hoàn thành
			err := t.autoCompleteRegularOrder(ctx, order)
			if err != nil {
				log.Error().
					Err(err).
					Str("order_id", order.ID.String()).
					Str("order_code", order.Code).
					Msg("Failed to auto-complete auction order")
				continue
			}
		
		default:
			log.Error().
				Str("order_id", order.ID.String()).
				Str("order_code", order.Code).
				Str("order_type", string(order.Type)).
				Msg("Unsupported order type for auto-completion")
			continue
		}
		
		// Gửi thông báo cho người dùng
		t.sendAutoCompleteNotifications(ctx, order)
		
		log.Info().
			Str("order_id", order.ID.String()).
			Str("order_code", order.Code).
			Msg("Successfully auto-completed order")
	}
}

// autoCompleteRegularOrder xử lý tự động hoàn tất đơn hàng thông thường
func (t *OrderTracker) autoCompleteRegularOrder(ctx context.Context, order db.Order) error {
	// 1. Lấy thông tin giao dịch đơn hàng
	orderTransaction, err := t.store.GetOrderTransactionByOrderID(ctx, order.ID)
	if err != nil {
		if errors.Is(err, db.ErrRecordNotFound) {
			return fmt.Errorf("transaction for order ID %s not found", order.ID)
		}
		return err
	}
	
	// 2. Kiểm tra xem giao dịch đã có seller_entry_id chưa
	if orderTransaction.SellerEntryID == nil {
		return fmt.Errorf("seller entry not found for order %s", order.Code)
	}
	
	// 3. Lấy bút toán của người bán
	sellerEntry, err := t.store.GetWalletEntryByID(ctx, *orderTransaction.SellerEntryID)
	if err != nil {
		if errors.Is(err, db.ErrRecordNotFound) {
			return fmt.Errorf("seller entry not found for order %s", order.Code)
		}
		return err
	}
	
	// 4. Lấy ví của người bán
	sellerWallet, err := t.store.GetWalletForUpdate(ctx, order.SellerID)
	if err != nil {
		if errors.Is(err, db.ErrRecordNotFound) {
			return fmt.Errorf("wallet not found for seller %s", order.SellerID)
		}
		return err
	}
	
	// 5. Lấy thông tin order items
	orderItems, err := t.store.ListOrderItems(ctx, order.ID)
	if err != nil {
		return err
	}
	
	// 6. Kiểm tra trạng thái của các Gundam liên quan
	for _, item := range orderItems {
		if item.GundamID != nil {
			gundam, err := t.store.GetGundamByID(ctx, *item.GundamID)
			if err != nil {
				if errors.Is(err, db.ErrRecordNotFound) {
					return fmt.Errorf("gundam ID %d not found", *item.GundamID)
				}
				return err
			}
			
			if gundam.Status != db.GundamStatusProcessing {
				return fmt.Errorf("gundam ID %d is not in processing status", *item.GundamID)
			}
		}
	}
	
	// 7. Thực hiện transaction xác nhận đơn hàng đã nhận
	_, err = t.store.CompleteRegularOrderTx(ctx, db.CompleteRegularOrderTxParams{
		Order:        &order,
		OrderItems:   orderItems,
		SellerEntry:  &sellerEntry,
		SellerWallet: &sellerWallet,
	})
	
	return err
}

// autoCompleteExchangeOrder xử lý tự động hoàn tất đơn hàng trao đổi
func (t *OrderTracker) autoCompleteExchangeOrder(ctx context.Context, order db.Order) error {
	// 1. Lấy thông tin exchange từ order
	exchange, err := t.store.GetExchangeByOrderID(ctx, &order.ID)
	if err != nil {
		if errors.Is(err, db.ErrRecordNotFound) {
			return fmt.Errorf("exchange for order ID %s not found", order.ID)
		}
		return err
	}
	
	// 2. Xác định đơn hàng đối tác
	var partnerOrderID uuid.UUID
	isPosterOrder := exchange.PosterOrderID != nil && *exchange.PosterOrderID == order.ID
	isOffererOrder := exchange.OffererOrderID != nil && *exchange.OffererOrderID == order.ID
	
	if isPosterOrder && exchange.OffererOrderID != nil {
		partnerOrderID = *exchange.OffererOrderID
	} else if isOffererOrder && exchange.PosterOrderID != nil {
		partnerOrderID = *exchange.PosterOrderID
	} else {
		return fmt.Errorf("invalid exchange configuration")
	}
	
	// 3. Lấy thông tin order items
	orderItems, err := t.store.ListOrderItems(ctx, order.ID)
	if err != nil {
		return err
	}
	
	// 4. Kiểm tra trạng thái của các gundam liên quan
	for _, item := range orderItems {
		if item.GundamID != nil {
			gundam, err := t.store.GetGundamByID(ctx, *item.GundamID)
			if err != nil {
				if errors.Is(err, db.ErrRecordNotFound) {
					return fmt.Errorf("gundam ID %d not found", *item.GundamID)
				}
				return err
			}
			
			if gundam.Status != db.GundamStatusExchanging {
				return fmt.Errorf("gundam ID %d is not in exchanging status", *item.GundamID)
			}
		}
	}
	
	// 5. Lấy thông tin exchange items (tất cả các items trong exchange)
	exchangeItems, err := t.store.ListExchangeItems(ctx, db.ListExchangeItemsParams{
		ExchangeID: exchange.ID,
	})
	if err != nil {
		return err
	}
	
	// 6. Thực hiện transaction xác nhận đơn hàng trao đổi đã nhận
	_, err = t.store.CompleteExchangeOrderTx(ctx, db.CompleteExchangeOrderTxParams{
		Order:          &order,
		Exchange:       &exchange,
		ExchangeItems:  exchangeItems,
		PartnerOrderID: partnerOrderID,
	})
	
	return err
}

// sendAutoCompleteNotifications gửi thông báo cho người mua và người bán khi đơn hàng được tự động hoàn tất
func (t *OrderTracker) sendAutoCompleteNotifications(ctx context.Context, order db.Order) {
	opts := []asynq.Option{
		asynq.MaxRetry(3),
		asynq.Queue(worker.QueueDefault),
	}
	
	// Tạo thông báo dựa vào loại đơn hàng
	switch order.Type {
	case db.OrderTypeRegular:
		// Thông báo cho người mua
		err := t.taskDistributor.DistributeTaskSendNotification(ctx, &worker.PayloadSendNotification{
			RecipientID: order.BuyerID,
			Title:       fmt.Sprintf("Đơn hàng %s đã được tự động hoàn tất", order.Code),
			Message:     fmt.Sprintf("Đơn hàng %s đã được tự động hoàn tất sau 7 ngày kể từ khi giao hàng thành công. Mô hình Gundam đã được thêm vào bộ sưu tập của bạn.", order.Code),
			Type:        "order",
			ReferenceID: order.Code,
		}, opts...)
		
		if err != nil {
			log.Error().
				Err(err).
				Str("buyer_id", order.BuyerID).
				Str("order_code", order.Code).
				Msg("failed to send auto-complete notification to buyer")
		} else {
			log.Info().
				Str("buyer_id", order.BuyerID).
				Str("order_code", order.Code).
				Msg("auto-complete notification sent to buyer")
		}
		
		// Thông báo cho người bán
		err = t.taskDistributor.DistributeTaskSendNotification(ctx, &worker.PayloadSendNotification{
			RecipientID: order.SellerID,
			Title:       fmt.Sprintf("Đơn hàng %s đã được tự động hoàn tất", order.Code),
			Message: fmt.Sprintf("Đơn hàng %s đã được tự động hoàn tất sau 7 ngày kể từ khi giao hàng thành công. Số tiền %s đã được chuyển vào số dư khả dụng của bạn.",
				order.Code, util.FormatVND(order.ItemsSubtotal)),
			Type:        "order",
			ReferenceID: order.Code,
		}, opts...)
		
		if err != nil {
			log.Error().
				Err(err).
				Str("seller_id", order.SellerID).
				Str("order_code", order.Code).
				Msg("failed to send auto-complete notification to seller")
		} else {
			log.Info().
				Str("seller_id", order.SellerID).
				Str("order_code", order.Code).
				Msg("auto-complete notification sent to seller")
		}
	
	case db.OrderTypeExchange:
		// Xác định bối cảnh của cuộc trao đổi dựa trên đơn hàng
		exchange, err := t.store.GetExchangeByOrderID(ctx, &order.ID)
		if err != nil {
			log.Error().
				Err(err).
				Str("order_id", order.ID.String()).
				Msg("failed to get exchange for notification")
			return
		}
		
		// Thông báo cho người nhận hàng (buyer của đơn hàng hiện tại)
		err = t.taskDistributor.DistributeTaskSendNotification(ctx, &worker.PayloadSendNotification{
			RecipientID: order.BuyerID,
			Title:       fmt.Sprintf("Đơn hàng trao đổi %s đã được tự động hoàn tất", order.Code),
			Message:     fmt.Sprintf("Đơn hàng trao đổi %s đã được tự động hoàn tất sau 7 ngày kể từ khi giao hàng thành công. Các mô hình Gundam đã được cập nhật trong bộ sưu tập của bạn.", order.Code),
			Type:        "order",
			ReferenceID: order.Code,
		}, opts...)
		
		if err != nil {
			log.Error().
				Err(err).
				Str("buyer_id", order.BuyerID).
				Str("order_code", order.Code).
				Msg("failed to send auto-complete notification to exchange buyer")
		}
		
		// Xác định bên đối tác trong giao dịch trao đổi
		var partnerID string
		isPosterOrder := exchange.PosterOrderID != nil && *exchange.PosterOrderID == order.ID
		if isPosterOrder {
			partnerID = exchange.OffererID
		} else {
			partnerID = exchange.PosterID
		}
		
		// Thông báo cho đối tác
		message := fmt.Sprintf("Đơn hàng trao đổi %s đã được tự động hoàn tất sau 7 ngày. ", order.Code)
		
		// Kiểm tra xem cả hai đơn hàng đã hoàn tất chưa
		if exchange.Status == db.ExchangeStatusCompleted {
			message = fmt.Sprintf("Giao dịch trao đổi đã hoàn tất. Các mô hình Gundam đã được chuyển quyền sở hữu. ")
			
			// Nếu có tiền bù, thêm thông tin
			if exchange.PayerID != nil && exchange.CompensationAmount != nil {
				if partnerID != *exchange.PayerID {
					message += fmt.Sprintf("Bạn đã nhận được %s tiền bù cho giao dịch này.", util.FormatVND(*exchange.CompensationAmount))
				}
			}
		}
		
		err = t.taskDistributor.DistributeTaskSendNotification(ctx, &worker.PayloadSendNotification{
			RecipientID: partnerID,
			Title:       "Cập nhật giao dịch trao đổi",
			Message:     message,
			Type:        "exchange",
			ReferenceID: exchange.ID.String(),
		}, opts...)
		
		if err != nil {
			log.Error().
				Err(err).
				Str("partner_id", partnerID).
				Str("exchange_id", exchange.ID.String()).
				Msg("failed to send auto-complete notification to exchange partner")
		}
	
	case db.OrderTypeAuction:
		// Thông báo cho người thắng đấu giá
		err := t.taskDistributor.DistributeTaskSendNotification(ctx, &worker.PayloadSendNotification{
			RecipientID: order.BuyerID,
			Title:       fmt.Sprintf("Đơn hàng đấu giá %s đã được tự động hoàn tất", order.Code),
			Message:     fmt.Sprintf("Đơn hàng đấu giá %s đã được tự động hoàn tất sau 7 ngày kể từ khi giao hàng thành công. Mô hình Gundam đã được thêm vào bộ sưu tập của bạn.", order.Code),
			Type:        "order",
			ReferenceID: order.Code,
		}, opts...)
		
		if err != nil {
			log.Error().
				Err(err).
				Str("buyer_id", order.BuyerID).
				Str("order_code", order.Code).
				Msg("failed to send auto-complete notification to buyer")
		} else {
			log.Info().
				Str("buyer_id", order.BuyerID).
				Str("order_code", order.Code).
				Msg("auto-complete notification sent to buyer")
		}
		
		// Thông báo cho người bán
		err = t.taskDistributor.DistributeTaskSendNotification(ctx, &worker.PayloadSendNotification{
			RecipientID: order.SellerID,
			Title:       fmt.Sprintf("Đơn hàng đấu giá %s đã được tự động hoàn tất", order.Code),
			Message: fmt.Sprintf("Đơn hàng đấu giá %s đã được tự động hoàn tất sau 7 ngày kể từ khi giao hàng thành công. Số tiền %s đã được chuyển vào số dư khả dụng của bạn.",
				order.Code, util.FormatVND(order.ItemsSubtotal)),
			Type:        "order",
			ReferenceID: order.Code,
		}, opts...)
		
		if err != nil {
			log.Error().
				Err(err).
				Str("seller_id", order.SellerID).
				Str("order_code", order.Code).
				Msg("failed to send auto-complete notification to seller")
		} else {
			log.Info().
				Str("seller_id", order.SellerID).
				Str("order_code", order.Code).
				Msg("auto-complete notification sent to seller")
		}
	
	default:
		log.Warn().
			Str("order_id", order.ID.String()).
			Str("order_type", string(order.Type)).
			Msg("Unsupported order type for notifications")
	}
}
