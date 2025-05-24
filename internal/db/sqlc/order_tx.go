package db

import (
	"context"
	"errors"
	"fmt"
	"mime/multipart"
	"time"
	
	"github.com/google/uuid"
	"github.com/katatrina/gundam-BE/internal/delivery"
	"github.com/katatrina/gundam-BE/internal/util"
	"github.com/rs/zerolog/log"
)

type CreateOrderTxParams struct {
	BuyerID              string
	BuyerAddress         UserAddress
	SellerID             string
	SellerAddress        UserAddress
	ItemsSubtotal        int64
	TotalAmount          int64
	DeliveryFee          int64
	ExpectedDeliveryTime time.Time
	PaymentMethod        PaymentMethod
	Note                 *string
	Gundams              []Gundam
}

type CreateOrderTxResult struct {
	Order            Order            `json:"order"`
	OrderItems       []OrderItem      `json:"order_items"`
	OrderDelivery    OrderDelivery    `json:"order_delivery"`
	BuyerEntry       WalletEntry      `json:"buyer_entry"`
	OrderTransaction OrderTransaction `json:"order_transaction"`
}

func (store *SQLStore) CreateOrderTx(ctx context.Context, arg CreateOrderTxParams) (CreateOrderTxResult, error) {
	var result CreateOrderTxResult
	
	err := store.ExecTx(ctx, func(qTx *Queries) error {
		var err error
		var buyerWallet Wallet
		var buyerEntry WalletEntry
		
		// Kể từ đây, chúng ta sẽ giả sử đơn hàng được thanh toán bằng ví.
		
		// 1. Kiểm tra và cập nhật số dư ví của người mua
		buyerWallet, err = qTx.GetWalletForUpdate(ctx, arg.BuyerID)
		if err != nil {
			return fmt.Errorf("failed to get buyer wallet: %w", err)
		}
		
		if buyerWallet.Balance < arg.TotalAmount {
			return fmt.Errorf("insufficient balance: available %d, needed %d",
				buyerWallet.Balance, arg.TotalAmount)
		}
		
		orderID, _ := uuid.NewV7() // Xác suất xảy ra err gần như bằng 0
		
		// 2. Tạo order
		orderCode := util.GenerateOrderCode() // Bỏ qua kiểm tra unique cho đơn giản
		order, err := qTx.CreateOrder(ctx, CreateOrderParams{
			ID:            orderID, // Đã ràng buộc unique trong db
			Code:          orderCode,
			BuyerID:       arg.BuyerID,
			SellerID:      arg.SellerID,
			ItemsSubtotal: arg.ItemsSubtotal,
			DeliveryFee:   arg.DeliveryFee, // Phí vận chuyển có thể được cập nhật trong tương lai
			TotalAmount:   arg.TotalAmount,
			Status:        OrderStatusPending,
			PaymentMethod: arg.PaymentMethod,
			Type:          OrderTypeRegular,
			Note:          arg.Note,
		})
		if err != nil {
			return err
		}
		result.Order = order
		
		// Trừ tiền từ ví người mua
		_, err = qTx.AddWalletBalance(ctx, AddWalletBalanceParams{
			UserID: buyerWallet.UserID,
			Amount: -arg.TotalAmount, // Truyền số âm để trừ
		})
		if err != nil {
			return fmt.Errorf("failed to deduct balance: %w", err)
		}
		
		// Tạo wallet entry cho người mua ✅
		buyerEntry, err = qTx.CreateWalletEntry(ctx, CreateWalletEntryParams{
			WalletID:      buyerWallet.UserID,
			ReferenceID:   &order.Code,
			ReferenceType: WalletReferenceTypeOrder,
			EntryType:     WalletEntryTypePayment,
			AffectedField: WalletAffectedFieldBalance,
			Amount:        -arg.TotalAmount, // Trừ tiền từ ví người mua
			Status:        WalletEntryStatusCompleted,
			CompletedAt:   util.TimePointer(time.Now()),
		})
		if err != nil {
			return fmt.Errorf("failed to create buyer wallet entry: %w", err)
		}
		result.BuyerEntry = buyerEntry
		
		// 3. Tạo các order items
		for _, gundam := range arg.Gundams {
			var orderItem OrderItem
			
			grade, err := qTx.GetGradeByID(ctx, gundam.GradeID)
			if err != nil {
				return fmt.Errorf("failed to get grade by ID: %w", err)
			}
			
			primaryImageURL, err := qTx.GetGundamPrimaryImageURL(ctx, gundam.ID)
			if err != nil {
				return fmt.Errorf("failed to get primary image: %w", err)
			}
			
			orderItem, err = qTx.CreateOrderItem(ctx, CreateOrderItemParams{
				OrderID:  order.ID,
				GundamID: &gundam.ID,
				Name:     gundam.Name,
				Slug:     gundam.Slug,
				Grade:    grade.DisplayName,
				Scale:    string(gundam.Scale),
				Price:    *gundam.Price,
				Quantity: gundam.Quantity,
				Weight:   gundam.Weight,
				ImageURL: primaryImageURL,
			})
			if err != nil {
				return err
			}
			
			// 4. Cập nhật trạng thái Gundam thành "processing"
			// để tránh người khác mua sản phẩm nếu giao dịch chưa hoàn tất
			if err = qTx.UpdateGundam(ctx, UpdateGundamParams{
				ID: gundam.ID,
				Status: NullGundamStatus{
					GundamStatus: GundamStatusProcessing,
					Valid:        true,
				},
			}); err != nil {
				return err
			}
			
			result.OrderItems = append(result.OrderItems, orderItem)
		}
		
		// 5. Tạo thông tin vận chuyển
		buyerDelivery, sellerDelivery, err := createDeliveryInfo(qTx, ctx, arg)
		if err != nil {
			return err
		}
		
		// 6. Tạo order delivery
		// Các cột status, overall_status, delivery_tracking_code sẽ được cập nhật sau
		// khi người bán xác nhận và đóng gói đơn hàng.
		orderDelivery, err := qTx.CreateOrderDelivery(ctx, CreateOrderDeliveryParams{
			OrderID:              order.ID,
			ExpectedDeliveryTime: arg.ExpectedDeliveryTime,
			FromDeliveryID:       sellerDelivery.ID,
			ToDeliveryID:         buyerDelivery.ID,
		})
		if err != nil {
			return err
		}
		result.OrderDelivery = orderDelivery
		
		// 7. Tạo order transaction
		orderTrans, err := qTx.CreateOrderTransaction(ctx, CreateOrderTransactionParams{
			OrderID:      order.ID,
			Amount:       arg.TotalAmount,
			Status:       OrderTransactionStatusPending,
			BuyerEntryID: buyerEntry.ID,
			// seller_entry_id sẽ được cập nhật sau khi người bán xác nhận đơn hàng
		})
		if err != nil {
			return fmt.Errorf("failed to create order transaction: %w", err)
		}
		result.OrderTransaction = orderTrans
		
		return nil
	})
	
	return result, err
}

type PackageOrderTxParams struct {
	Order               *Order
	PackageImages       []*multipart.FileHeader
	UploadImagesFunc    func(key string, value string, folder string, files ...*multipart.FileHeader) ([]string, error)
	CreateDeliveryOrder func(ctx context.Context, request delivery.CreateOrderRequest) (*delivery.CreateOrderResponse, error)
}

type PackageOrderTxResult struct {
	Order         Order         `json:"order"`
	OrderDelivery OrderDelivery `json:"order_delivery"`
}

func (store *SQLStore) PackageOrderTx(ctx context.Context, arg PackageOrderTxParams) (PackageOrderTxResult, error) {
	var result PackageOrderTxResult
	
	err := store.ExecTx(ctx, func(qTx *Queries) error {
		// 1. Upload packaging images and store the URLs
		packagingImageURLs, err := arg.UploadImagesFunc("packaging_image", arg.Order.Code, util.FolderOrders, arg.PackageImages...)
		if err != nil {
			return err
		}
		
		// Cập nhật đơn hàng - chỉ đánh dấu đã đóng gói, không thay đổi trạng thái
		updatedOrder, err := qTx.UpdateOrder(ctx, UpdateOrderParams{
			IsPackaged:         util.BoolPointer(true),
			PackagingImageURLs: packagingImageURLs,
			OrderID:            arg.Order.ID,
		})
		if err != nil {
			return fmt.Errorf("failed to update order: %w", err)
		}
		result.Order = updatedOrder
		
		// 2. Lấy thông tin các mặt hàng trong đơn hàng
		orderItems, err := qTx.ListOrderItems(ctx, updatedOrder.ID)
		if err != nil {
			return fmt.Errorf("failed to get order items: %w", err)
		}
		
		// 3. Lấy thông tin giao hàng
		orderDelivery, err := qTx.GetOrderDelivery(ctx, updatedOrder.ID)
		if err != nil {
			return fmt.Errorf("failed to get order delivery: %w", err)
		}
		
		// 4. Lấy địa chỉ người gửi
		senderAddress, err := qTx.GetDeliveryInformation(ctx, orderDelivery.FromDeliveryID)
		if err != nil {
			return fmt.Errorf("failed to get sender address: %w", err)
		}
		
		// 5. Lấy địa chỉ người nhận
		receiverAddress, err := qTx.GetDeliveryInformation(ctx, orderDelivery.ToDeliveryID)
		if err != nil {
			return fmt.Errorf("failed to get receiver address: %w", err)
		}
		
		// Chuyển đổi dữ liệu từ db sang ghn
		createOrderRequest := ConvertToDeliveryCreateOrderRequest(updatedOrder, orderItems, senderAddress, receiverAddress)
		
		// 6. Gọi hàm tạo đơn hàng GHN
		ghnResponse, err := arg.CreateDeliveryOrder(ctx, createOrderRequest)
		if err != nil {
			return fmt.Errorf("failed to create GHN order: %w", err)
		}
		
		// So sánh phí vận chuyển từ GHN với phí đã thanh toán
		if updatedOrder.DeliveryFee != ghnResponse.Data.TotalFee {
			log.Warn().Msgf("Delivery fee mismatch: %d != %d", updatedOrder.DeliveryFee, ghnResponse.Data.TotalFee)
		}
		// So sánh thời gian giao hàng dự kiến với giá trị đã lưu
		if orderDelivery.ExpectedDeliveryTime != ghnResponse.Data.ExpectedDeliveryTime {
			log.Warn().Msgf("Expected delivery time mismatch: %v != %v", orderDelivery.ExpectedDeliveryTime, ghnResponse.Data.ExpectedDeliveryTime)
		}
		
		// 7. Cập nhật thông tin giao hàng với mã đơn GHN
		updatedDelivery, err := qTx.UpdateOrderDelivery(ctx, UpdateOrderDeliveryParams{
			ID:                   orderDelivery.ID,
			DeliveryTrackingCode: &ghnResponse.Data.OrderCode,
			ExpectedDeliveryTime: util.TimePointer(ghnResponse.Data.ExpectedDeliveryTime),
			Status:               util.StringPointer("ready_to_pick"), // Hardcode status vì GHN không trả về trong response sau khi tạo đơn hàng
			OverallStatus: NullDeliveryOverralStatus{
				DeliveryOverralStatus: DeliveryOverralStatusPicking,
				Valid:                 true,
			},
		})
		if err != nil {
			return fmt.Errorf("failed to update order delivery: %w", err)
		}
		result.OrderDelivery = updatedDelivery
		
		// Xử lý các loại đơn hàng đặc biệt (trao đổi, đấu giá)
		switch updatedOrder.Type {
		case OrderTypeRegular:
			// Đơn hàng thông thường - không có xử lý đặc biệt
			log.Info().Msgf("Regular order %s has been packaged", updatedOrder.ID)
		
		case OrderTypeExchange:
			// Xử lý đơn hàng trao đổi
			exchange, err := qTx.GetExchangeByOrderID(ctx, &updatedOrder.ID)
			if err != nil {
				if errors.Is(err, ErrRecordNotFound) {
					return fmt.Errorf("cannot find exchange for order ID %s", updatedOrder.ID)
				}
				return fmt.Errorf("failed to get exchange by order ID: %w", err)
			}
			
			// Lấy cả hai đơn hàng liên quan đến exchange
			var posterOrder, offererOrder Order
			var err1, err2 error
			
			if exchange.PosterOrderID != nil {
				posterOrder, err1 = qTx.GetOrderByID(ctx, *exchange.PosterOrderID)
			}
			
			if exchange.OffererOrderID != nil {
				offererOrder, err2 = qTx.GetOrderByID(ctx, *exchange.OffererOrderID)
			}
			
			if err1 != nil || err2 != nil {
				return fmt.Errorf("failed to get exchange orders: %v, %v", err1, err2)
			}
			
			// Xác định trạng thái thấp nhất giữa hai đơn hàng
			lowestStatus := GetLowestOrderStatus(posterOrder.Status, offererOrder.Status)
			
			// Ánh xạ từ trạng thái đơn hàng sang trạng thái exchange
			var exchangeStatus ExchangeStatus
			switch lowestStatus {
			case OrderStatusPending:
				exchangeStatus = ExchangeStatusPending
			case OrderStatusPackaging:
				exchangeStatus = ExchangeStatusPackaging
			case OrderStatusDelivering:
				exchangeStatus = ExchangeStatusDelivering
			case OrderStatusDelivered:
				exchangeStatus = ExchangeStatusDelivered
			case OrderStatusCompleted:
				exchangeStatus = ExchangeStatusCompleted
			case OrderStatusFailed:
				exchangeStatus = ExchangeStatusFailed
			case OrderStatusCanceled:
				exchangeStatus = ExchangeStatusCanceled
			default:
				exchangeStatus = exchange.Status
			}
			
			// Cập nhật trạng thái exchange nếu khác với trạng thái hiện tại
			if exchange.Status != exchangeStatus {
				_, err = qTx.UpdateExchange(ctx, UpdateExchangeParams{
					ID: exchange.ID,
					Status: NullExchangeStatus{
						ExchangeStatus: exchangeStatus,
						Valid:          true,
					},
				})
				if err != nil {
					return fmt.Errorf("failed to update exchange status: %w", err)
				}
			}
			
			log.Info().Msgf("Exchange order %s has been packaged, exchange status: %s", updatedOrder.ID, exchangeStatus)
		
		case OrderTypeAuction:
			// Đơn hàng đấu giá cũng như đơn hàng thông thường
			log.Info().Msgf("Auction order %s has been packaged", updatedOrder.ID)
		
		default:
			log.Warn().Msgf("Unknown order type %s for order ID %s", updatedOrder.Type, updatedOrder.ID)
		}
		
		return nil
	})
	
	return result, err
}

type CompleteRegularOrderTxParams struct {
	Order        *Order
	OrderItems   []OrderItem
	SellerEntry  *WalletEntry
	SellerWallet *Wallet
}

type CompleteRegularOrderTxResult struct {
	Order            Order            `json:"order"`
	OrderTransaction OrderTransaction `json:"order_transaction"`
	SellerWallet     Wallet           `json:"seller_wallet"`
	SellerEntry      WalletEntry      `json:"seller_entry"`
}

// CompleteRegularOrderTx xử lý việc hoàn tất đơn hàng thông thường khi người nhận xác nhận nhận hàng thành công.
func (store *SQLStore) CompleteRegularOrderTx(ctx context.Context, arg CompleteRegularOrderTxParams) (CompleteRegularOrderTxResult, error) {
	var result CompleteRegularOrderTxResult
	
	err := store.ExecTx(ctx, func(qTx *Queries) error {
		// 1. Chuyển tiền từ non_withdrawable_amount sang balance của người bán
		var err error
		updatedWallet, err := qTx.TransferNonWithdrawableToBalance(ctx, TransferNonWithdrawableToBalanceParams{
			Amount: arg.Order.ItemsSubtotal,
			UserID: arg.SellerWallet.UserID,
		})
		if err != nil {
			return fmt.Errorf("failed to transfer non-withdrawable amount to balance: %w", err)
		}
		result.SellerWallet = updatedWallet
		
		// 2. Cập nhật trạng thái bút toán của người bán thành "completed"
		sellerEntry, err := qTx.UpdateWalletEntryByID(ctx, UpdateWalletEntryByIDParams{
			ID: arg.SellerEntry.ID,
			Status: NullWalletEntryStatus{
				WalletEntryStatus: WalletEntryStatusCompleted,
				Valid:             true,
			},
			CompletedAt: util.TimePointer(time.Now()),
		})
		if err != nil {
			return fmt.Errorf("failed to update seller wallet entry status: %w", err)
		}
		result.SellerEntry = sellerEntry
		
		// TODO: Cập nhật trạng thái các bút toán khác nếu giao dịch bao gồm nhiều loại bút toán
		
		// 3. Cập nhật trạng thái giao dịch đơn hàng thành "completed"
		updatedOrderTransaction, err := qTx.UpdateOrderTransaction(ctx, UpdateOrderTransactionParams{
			Status: NullOrderTransactionStatus{
				OrderTransactionStatus: OrderTransactionStatusCompleted,
				Valid:                  true,
			},
			CompletedAt: util.TimePointer(time.Now()),
			OrderID:     arg.Order.ID,
		})
		if err != nil {
			return fmt.Errorf("failed to update order transaction status: %w", err)
		}
		result.OrderTransaction = updatedOrderTransaction
		
		// 4. Chuyển quyền sở hữu các mặt hàng trong đơn hàng cho người nhận hàng,
		// cũng như cập nhật trạng thái của chúng thành "in store"
		for _, item := range arg.OrderItems {
			if item.GundamID != nil {
				err = qTx.UpdateGundam(ctx, UpdateGundamParams{
					ID:      *item.GundamID,
					OwnerID: &arg.Order.BuyerID,
					Status: NullGundamStatus{
						GundamStatus: GundamStatusInstore,
						Valid:        true,
					},
				})
				if err != nil {
					return fmt.Errorf("failed to update gundam owner: %w", err)
				}
			} else {
				log.Warn().Msgf("gundam ID %d not found in order item %d", item.GundamID, item.ID)
			}
		}
		
		// 5. Cập nhật trạng thái đơn hàng thành "completed"
		updatedOrder, err := qTx.UpdateOrder(ctx, UpdateOrderParams{
			OrderID: arg.Order.ID,
			Status: NullOrderStatus{
				OrderStatus: OrderStatusCompleted,
				Valid:       true,
			},
			CompletedAt: util.TimePointer(time.Now()),
		})
		if err != nil {
			return fmt.Errorf("failed to update order status: %w", err)
		}
		result.Order = updatedOrder
		
		return nil
	})
	
	return result, err
}

type CancelOrderByBuyerTxParams struct {
	Order  *Order
	Reason *string
}

type CancelOrderByBuyerTxResult struct {
	Order            Order            `json:"order"`
	OrderTransaction OrderTransaction `json:"order_transaction"`
	RefundEntry      WalletEntry      `json:"refund_entry"`
	BuyerWallet      Wallet           `json:"buyer_wallet"`
}

func (store *SQLStore) CancelOrderByBuyerTx(ctx context.Context, arg CancelOrderByBuyerTxParams) (CancelOrderByBuyerTxResult, error) {
	var result CancelOrderByBuyerTxResult
	
	err := store.ExecTx(ctx, func(qTx *Queries) error {
		// 1. Cập nhật trạng thái đơn hàng thành "canceled"
		updatedOrder, err := qTx.UpdateOrder(ctx, UpdateOrderParams{
			OrderID: arg.Order.ID,
			Status: NullOrderStatus{
				OrderStatus: OrderStatusCanceled,
				Valid:       true,
			},
			CanceledBy:     util.StringPointer(arg.Order.BuyerID),
			CanceledReason: arg.Reason,
		})
		if err != nil {
			return fmt.Errorf("failed to update order status to cancel: %w", err)
		}
		result.Order = updatedOrder
		
		// 2. Cập nhật trạng thái giao dịch đơn hàng thành "refunded"
		orderTrans, err := qTx.UpdateOrderTransaction(ctx, UpdateOrderTransactionParams{
			OrderID: arg.Order.ID,
			Status: NullOrderTransactionStatus{
				OrderTransactionStatus: OrderTransactionStatusRefunded,
				Valid:                  true,
			},
		})
		if err != nil {
			return fmt.Errorf("failed to update order transaction status to refunded: %w", err)
		}
		result.OrderTransaction = orderTrans
		
		// 3. Lấy thông tin ví người mua
		buyerWallet, err := qTx.GetWalletForUpdate(ctx, updatedOrder.BuyerID)
		if err != nil {
			return fmt.Errorf("failed to get buyer wallet: %w", err)
		}
		
		// 4. Tạo bút toán hoàn tiền cho người mua ✅
		refundEntry, err := qTx.CreateWalletEntry(ctx, CreateWalletEntryParams{
			WalletID:      buyerWallet.UserID,
			ReferenceID:   &updatedOrder.Code,
			ReferenceType: WalletReferenceTypeOrder,
			EntryType:     WalletEntryTypeRefund,
			AffectedField: WalletAffectedFieldBalance,
			Amount:        orderTrans.Amount, // Số dương (+) vì đây là bút toán hoàn tiền
			Status:        WalletEntryStatusCompleted,
			CompletedAt:   util.TimePointer(time.Now()),
		})
		if err != nil {
			return fmt.Errorf("failed to create refund wallet entry for buyer: %w", err)
		}
		result.RefundEntry = refundEntry
		
		// 5. Hoàn tiền cho người mua (cập nhật số dư ví)
		updatedBuyerWallet, err := qTx.AddWalletBalance(ctx, AddWalletBalanceParams{
			Amount: orderTrans.Amount,
			UserID: updatedOrder.BuyerID,
		})
		if err != nil {
			return fmt.Errorf("failed to add balance to buyer wallet: %w", err)
		}
		result.BuyerWallet = updatedBuyerWallet
		
		// 6. Khôi phục trạng thái gundam về "published"
		orderItems, err := qTx.ListOrderItems(ctx, updatedOrder.ID)
		if err != nil {
			return fmt.Errorf("failed to get order items: %w", err)
		}
		
		for _, item := range orderItems {
			if item.GundamID != nil {
				if err = qTx.UpdateGundam(ctx, UpdateGundamParams{
					ID: *item.GundamID,
					Status: NullGundamStatus{
						GundamStatus: GundamStatusPublished,
						Valid:        true,
					},
				}); err != nil {
					return fmt.Errorf("failed to restore gundam status: %w", err)
				}
			} else {
				log.Warn().Msgf("Gundam ID %d not found in order item %d", item.GundamID, item.ID)
			}
			
		}
		
		return nil
	})
	
	return result, err
}

type FailRegularOrderTxParams struct {
	FailedOrder  *Order            // Đơn hàng failed cần xử lý
	BuyerEntry   *WalletEntry      // Bút toán người mua để hoàn tiền về số dư ví
	SellerEntry  *WalletEntry      // Bút toán người bán để cập nhật trạng thái cũng như trừ non_withdrawable_amount
	Transaction  *OrderTransaction // Quản lý giao dịch đơn hàng
	OrderItems   []OrderItem       // Danh sách các mặt hàng trong đơn hàng để chuyển trạng thái
	RefundAmount int64             // Số tiền cần hoàn lại cho người mua
}

type FailRegularOrderTxResult struct {
	BuyerRefundEntry WalletEntry
	Order            Order
	OrderTransaction OrderTransaction
}

// FailRegularOrderTx xử lý việc hoàn tiền và cập nhật trạng thái khi đơn hàng thông thường thất bại
func (store *SQLStore) FailRegularOrderTx(ctx context.Context, arg FailRegularOrderTxParams) (FailRegularOrderTxResult, error) {
	var result FailRegularOrderTxResult
	
	err := store.ExecTx(ctx, func(qTx *Queries) error {
		// Số tiền âm sẽ được đổi thành dương, do lấy từ bút toán trừ số dư ví của người mua,
		// không phải từ order transaction.
		refundAmount := -arg.RefundAmount
		
		// 1. Hoàn tiền vào số dư của người mua
		_, err := qTx.AddWalletBalance(ctx, AddWalletBalanceParams{
			UserID: arg.FailedOrder.BuyerID,
			Amount: refundAmount,
		})
		if err != nil {
			return err
		}
		
		// 2. Tạo bút toán hoàn tiền cho người mua ✅
		buyerRefundEntry, err := qTx.CreateWalletEntry(ctx, CreateWalletEntryParams{
			WalletID:      arg.FailedOrder.BuyerID,
			ReferenceID:   util.StringPointer(arg.FailedOrder.ID.String()),
			ReferenceType: WalletReferenceTypeOrder,
			EntryType:     WalletEntryTypeRefund,
			AffectedField: WalletAffectedFieldBalance,
			Amount:        refundAmount,
			Status:        WalletEntryStatusCompleted,
			CompletedAt:   util.TimePointer(time.Now()),
		})
		if err != nil {
			return err
		}
		result.BuyerRefundEntry = buyerRefundEntry
		
		if arg.SellerEntry.Status == WalletEntryStatusCompleted {
			log.Warn().Msgf("seller entry ID %d is already completed", arg.SellerEntry.ID)
		}
		
		// Cập nhật trạng thái bút toán của người bán thành "failed"
		_, err = qTx.UpdateWalletEntryByID(ctx, UpdateWalletEntryByIDParams{
			ID: arg.SellerEntry.ID,
			Status: NullWalletEntryStatus{
				WalletEntryStatus: WalletEntryStatusFailed,
				Valid:             true,
			},
		})
		if err != nil {
			return fmt.Errorf("failed to update seller wallet entry status: %w", err)
		}
		
		// 3. Giảm non_withdrawable_amount của người bán vì đã cộng khi người bán xác nhận đơn hàng.
		err = qTx.AddWalletNonWithdrawableAmount(ctx, AddWalletNonWithdrawableAmountParams{
			UserID: arg.FailedOrder.SellerID,
			Amount: -arg.SellerEntry.Amount,
		})
		if err != nil {
			return err
		}
		
		// 4. Tạo bút toán cập nhật non_withdrawable_amount của người bán ✅
		_, err = qTx.CreateWalletEntry(ctx, CreateWalletEntryParams{
			WalletID:      arg.FailedOrder.SellerID,
			ReferenceID:   util.StringPointer(arg.FailedOrder.ID.String()),
			ReferenceType: WalletReferenceTypeOrder,
			EntryType:     WalletEntryTypeReleaseFunds,
			AffectedField: WalletAffectedFieldNonWithdrawableAmount,
			Amount:        -arg.SellerEntry.Amount,
			Status:        WalletEntryStatusCompleted,
			CompletedAt:   util.TimePointer(time.Now()),
		})
		
		// 5. Cập nhật trạng thái giao dịch đơn hàng
		updatedTransaction, err := qTx.UpdateOrderTransaction(ctx, UpdateOrderTransactionParams{
			OrderID: arg.FailedOrder.ID,
			Status: NullOrderTransactionStatus{
				OrderTransactionStatus: OrderTransactionStatusRefunded,
				Valid:                  true,
			},
		})
		if err != nil {
			return err
		}
		result.OrderTransaction = updatedTransaction
		
		// 6. Cập nhật trạng thái các sản phẩm trong đơn hàng về "in store" cho người bán
		for _, item := range arg.OrderItems {
			if item.GundamID != nil {
				// Đảm bảo Gundam vẫn thuộc về người bán
				err = qTx.UpdateGundam(ctx, UpdateGundamParams{
					ID: *item.GundamID,
					Status: NullGundamStatus{
						GundamStatus: GundamStatusInstore,
						Valid:        true,
					},
				})
				if err != nil {
					return err
				}
			}
		}
		
		// 7. Cập nhật trạng thái đơn hàng
		updatedOrder, err := qTx.UpdateOrder(ctx, UpdateOrderParams{
			OrderID: arg.FailedOrder.ID,
			Status: NullOrderStatus{
				OrderStatus: OrderStatusFailed,
				Valid:       true,
			},
		})
		if err != nil {
			return err
		}
		result.Order = updatedOrder
		
		return nil
	})
	
	return result, err
}

type FailAuctionOrderTxParams struct {
	Order        *Order
	BuyerEntry   *WalletEntry
	SellerEntry  *WalletEntry
	Auction      *Auction
	Transaction  *OrderTransaction
	RefundAmount int64
}

type FailAuctionOrderTxResult struct {
	Order            Order
	OrderTransaction OrderTransaction
	BuyerRefundEntry WalletEntry
}

// FailAuctionOrderTx xử lý việc hoàn tiền và cập nhật trạng thái khi đơn hàng đấu giá thất bại
func (store *SQLStore) FailAuctionOrderTx(ctx context.Context, arg FailAuctionOrderTxParams) (FailAuctionOrderTxResult, error) {
	var result FailAuctionOrderTxResult
	
	err := store.ExecTx(ctx, func(qTx *Queries) error {
		refundAmount := -arg.RefundAmount // Số tiền âm sẽ được đổi thành dương
		
		// 1. Hoàn tiền cho người mua
		_, err := qTx.AddWalletBalance(ctx, AddWalletBalanceParams{
			UserID: arg.Order.BuyerID,
			Amount: refundAmount,
		})
		if err != nil {
			return err
		}
		
		// 2. Tạo bút toán hoàn tiền cho người mua ✅
		buyerRefundEntry, err := qTx.CreateWalletEntry(ctx, CreateWalletEntryParams{
			WalletID:      arg.Order.BuyerID,
			ReferenceID:   util.StringPointer(arg.Order.ID.String()),
			ReferenceType: WalletReferenceTypeOrder,
			EntryType:     WalletEntryTypeRefund,
			AffectedField: WalletAffectedFieldBalance,
			Amount:        refundAmount,
			Status:        WalletEntryStatusCompleted,
			CompletedAt:   util.TimePointer(time.Now()),
		})
		if err != nil {
			return err
		}
		result.BuyerRefundEntry = buyerRefundEntry
		
		if arg.SellerEntry.Status == WalletEntryStatusCompleted {
			log.Warn().Msgf("seller entry ID %d is already completed", arg.SellerEntry.ID)
		}
		
		// Cập nhật trạng thái bút toán của người bán thành "failed"
		_, err = qTx.UpdateWalletEntryByID(ctx, UpdateWalletEntryByIDParams{
			ID: arg.SellerEntry.ID,
			Status: NullWalletEntryStatus{
				WalletEntryStatus: WalletEntryStatusFailed,
				Valid:             true,
			},
		})
		if err != nil {
			return fmt.Errorf("failed to update seller wallet entry status: %w", err)
		}
		
		// 3. Giảm non_withdrawable_amount của người bán
		err = qTx.AddWalletNonWithdrawableAmount(ctx, AddWalletNonWithdrawableAmountParams{
			UserID: arg.Order.SellerID,
			Amount: -arg.SellerEntry.Amount,
		})
		if err != nil {
			return err
		}
		
		// 3. Tạo bút toán cập nhật non_withdrawable_amount của người bán ✅
		_, err = qTx.CreateWalletEntry(ctx, CreateWalletEntryParams{
			WalletID:      arg.Order.SellerID,
			ReferenceID:   util.StringPointer(arg.Order.ID.String()),
			ReferenceType: WalletReferenceTypeOrder,
			EntryType:     WalletEntryTypeReleaseFunds,
			AffectedField: WalletAffectedFieldNonWithdrawableAmount,
			Amount:        -arg.SellerEntry.Amount,
			Status:        WalletEntryStatusCompleted,
			CompletedAt:   util.TimePointer(time.Now()),
		})
		if err != nil {
			return err
		}
		
		// 4. Cập nhật trạng thái giao dịch đơn hàng
		updatedTransaction, err := qTx.UpdateOrderTransaction(ctx, UpdateOrderTransactionParams{
			OrderID: arg.Order.ID,
			Status: NullOrderTransactionStatus{
				OrderTransactionStatus: OrderTransactionStatusRefunded,
				Valid:                  true,
			},
		})
		if err != nil {
			return err
		}
		result.OrderTransaction = updatedTransaction
		
		// 5. Cập nhật Gundam status
		if arg.Auction.GundamID != nil {
			err = qTx.UpdateGundam(ctx, UpdateGundamParams{
				ID: *arg.Auction.GundamID,
				Status: NullGundamStatus{
					GundamStatus: GundamStatusInstore,
					Valid:        true,
				},
			})
			if err != nil {
				return err
			}
		}
		
		// 6. Cập nhật đơn hàng
		updatedOrder, err := qTx.UpdateOrder(ctx, UpdateOrderParams{
			OrderID: arg.Order.ID,
			Status: NullOrderStatus{
				OrderStatus: OrderStatusFailed,
				Valid:       true,
			},
		})
		if err != nil {
			return err
		}
		result.Order = updatedOrder
		
		return nil
	})
	
	return result, err
}
