package db

import (
	"context"
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
		
		// Tạo wallet entry cho người mua
		buyerEntry, err = qTx.CreateWalletEntry(ctx, CreateWalletEntryParams{
			WalletID:      buyerWallet.UserID,
			ReferenceID:   &order.Code,
			ReferenceType: WalletReferenceTypeOrder,
			EntryType:     WalletEntryTypePayment,
			Amount:        -arg.TotalAmount, // Số âm (-) vì đây là bút toán trừ tiền
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
				Price:    gundam.Price,
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
		
		// 5. Gọi hàm tạo đơn hàng GHN
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
		
		// 6. Cập nhật thông tin giao hàng với mã đơn GHN
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
		
		// TODO: Cần xử lý thêm nếu đơn hàng là đơn trao đổi hoặc đấu giá.
		
		return nil
	})
	
	return result, err
}

// Helper function để tạo delivery information
func createDeliveryInfo(qTx *Queries, ctx context.Context, arg CreateOrderTxParams) (buyerDelivery, sellerDelivery DeliveryInformation, err error) {
	buyerDelivery, err = qTx.CreateDeliveryInformation(ctx, CreateDeliveryInformationParams{
		UserID:        arg.BuyerID,
		FullName:      arg.BuyerAddress.FullName,
		PhoneNumber:   arg.BuyerAddress.PhoneNumber,
		ProvinceName:  arg.BuyerAddress.ProvinceName,
		DistrictName:  arg.BuyerAddress.DistrictName,
		GhnDistrictID: arg.BuyerAddress.GhnDistrictID,
		WardName:      arg.BuyerAddress.WardName,
		GhnWardCode:   arg.BuyerAddress.GhnWardCode,
		Detail:        arg.BuyerAddress.Detail,
	})
	if err != nil {
		return
	}
	
	sellerDelivery, err = qTx.CreateDeliveryInformation(ctx, CreateDeliveryInformationParams{
		UserID:        arg.SellerID,
		FullName:      arg.SellerAddress.FullName,
		PhoneNumber:   arg.SellerAddress.PhoneNumber,
		ProvinceName:  arg.SellerAddress.ProvinceName,
		DistrictName:  arg.SellerAddress.DistrictName,
		GhnDistrictID: arg.SellerAddress.GhnDistrictID,
		WardName:      arg.SellerAddress.WardName,
		GhnWardCode:   arg.SellerAddress.GhnWardCode,
		Detail:        arg.SellerAddress.Detail,
	})
	return
}

type ConfirmOrderReceivedTxParams struct {
	Order        *Order
	OrderItems   []OrderItem
	SellerEntry  *WalletEntry
	SellerWallet *Wallet
}

type ConfirmOrderReceivedTxResult struct {
	Order            Order            `json:"order"`
	OrderTransaction OrderTransaction `json:"order_transaction"`
	SellerWallet     Wallet           `json:"seller_wallet"`
	SellerEntry      WalletEntry      `json:"seller_entry"`
}

func (store *SQLStore) ConfirmOrderReceivedByBuyerTx(ctx context.Context, arg ConfirmOrderReceivedTxParams) (ConfirmOrderReceivedTxResult, error) {
	var result ConfirmOrderReceivedTxResult
	
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
			OrderID: arg.Order.ID,
		})
		if err != nil {
			return fmt.Errorf("failed to update order transaction status: %w", err)
		}
		result.OrderTransaction = updatedOrderTransaction
		
		// 4. Chuyển quyền sở hữu các mặt hàng trong đơn hàng cho người mua,
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
				log.Warn().Msgf("Gundam ID %d not found in order item %d", item.GundamID, item.ID)
			}
		}
		
		// 5. Cập nhật trạng thái đơn hàng thành "completed"
		updatedOrder, err := qTx.UpdateOrder(ctx, UpdateOrderParams{
			OrderID: arg.Order.ID,
			Status: NullOrderStatus{
				OrderStatus: OrderStatusCompleted,
				Valid:       true,
			},
		})
		if err != nil {
			return fmt.Errorf("failed to update order status: %w", err)
		}
		result.Order = updatedOrder
		
		// TODO: Nếu đây là đơn hàng liên quan đến trao đổi hay đấu giá, cập nhật trạng thái tương ứng
		
		return nil
	})
	
	return result, err
}

type CancelOrderByBuyerTxParams struct {
	Order          *Order
	CanceledReason string
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
		// TODO: Tùy vào đơn hàng này là đơn hàng thông thường, trao đổi hay đấu giá, mà cần quy trình xử lý khác nhau
		
		// 1. Cập nhật trạng thái đơn hàng thành "canceled"
		updatedOrder, err := qTx.UpdateOrder(ctx, UpdateOrderParams{
			OrderID: arg.Order.ID,
			Status: NullOrderStatus{
				OrderStatus: OrderStatusCanceled,
				Valid:       true,
			},
			CanceledBy:     util.StringPointer(arg.Order.BuyerID),
			CanceledReason: util.StringPointer(arg.CanceledReason),
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
		
		// 4. Tạo bút toán hoàn tiền cho người mua
		refundEntry, err := qTx.CreateWalletEntry(ctx, CreateWalletEntryParams{
			WalletID:      buyerWallet.UserID,
			ReferenceID:   &updatedOrder.Code,
			ReferenceType: WalletReferenceTypeOrder,
			EntryType:     WalletEntryTypeRefund,
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
