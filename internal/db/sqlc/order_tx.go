package db

import (
	"context"
	"fmt"
	"mime/multipart"
	"time"
	
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
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
	Note                 pgtype.Text
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
			Note:          arg.Note,
		})
		if err != nil {
			return err
		}
		result.Order = order
		
		// Trừ tiền từ ví người mua
		_, err = qTx.AddWalletBalance(ctx, AddWalletBalanceParams{
			WalletID: buyerWallet.ID,
			Amount:   -arg.TotalAmount, // Truyền số âm để trừ
		})
		if err != nil {
			return fmt.Errorf("failed to deduct balance: %w", err)
		}
		
		// Tạo wallet entry cho người mua
		buyerEntry, err = qTx.CreateWalletEntry(ctx, CreateWalletEntryParams{
			WalletID: buyerWallet.ID,
			ReferenceID: pgtype.Text{
				String: order.Code, // Tham chiếu đến mã đơn hàng
				Valid:  true,
			},
			ReferenceType: WalletReferenceTypeOrder,
			EntryType:     WalletEntryTypePayment,
			Amount:        -arg.TotalAmount, // Số âm (-) vì đây là bút toán trừ tiền
			Status:        WalletEntryStatusCompleted,
			CompletedAt: pgtype.Timestamptz{
				Time:  time.Now(),
				Valid: true,
			},
		})
		if err != nil {
			return fmt.Errorf("failed to create buyer wallet entry: %w", err)
		}
		result.BuyerEntry = buyerEntry
		
		// 3. Tạo các order items
		for _, gundam := range arg.Gundams {
			var orderItem OrderItem
			orderItem, err = qTx.CreateOrderItem(ctx, CreateOrderItemParams{
				OrderID:  order.ID.String(),
				GundamID: gundam.ID,
				Price:    gundam.Price,
				Quantity: gundam.Quantity,
				Weight:   gundam.Weight,
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
		// Các cột status, overall_status, ghn_order_code sẽ được cập nhật sau
		// khi người bán xác nhận và đóng gói đơn hàng.
		orderDelivery, err := qTx.CreateOrderDelivery(ctx, CreateOrderDeliveryParams{
			OrderID:              order.ID.String(),
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
			OrderID:      order.ID.String(),
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

func ConvertToDeliveryCreateOrderRequest(order Order, orderItems []OrderItem, senderAddress, receiverAddress DeliveryInformation) delivery.CreateOrderRequest {
	ghnOrder := delivery.OrderInfo{
		ID:            order.ID.String(),
		Code:          order.Code,
		BuyerID:       order.BuyerID,
		SellerID:      order.SellerID,
		ItemsSubtotal: order.ItemsSubtotal,
		DeliveryFee:   order.DeliveryFee,
		TotalAmount:   order.TotalAmount,
		Status:        string(order.Status),
		PaymentMethod: string(order.PaymentMethod),
	}
	if order.Note.Valid {
		ghnOrder.Note = order.Note.String
	}
	
	ghnOrderItems := make([]delivery.OrderItemInfo, len(orderItems))
	for i, item := range orderItems {
		ghnOrderItems[i] = delivery.OrderItemInfo{
			OrderID:  item.OrderID,
			GundamID: item.GundamID,
			Price:    item.Price,
			Quantity: item.Quantity,
			Weight:   item.Weight,
		}
	}
	
	ghnSenderAddress := delivery.AddressInfo{
		UserID:        senderAddress.UserID,
		FullName:      senderAddress.FullName,
		PhoneNumber:   senderAddress.PhoneNumber,
		ProvinceName:  senderAddress.ProvinceName,
		DistrictName:  senderAddress.DistrictName,
		GhnDistrictID: senderAddress.GhnDistrictID,
		WardName:      senderAddress.WardName,
		GhnWardCode:   senderAddress.GhnWardCode,
		Detail:        senderAddress.Detail,
	}
	
	ghnReceiverAddress := delivery.AddressInfo{
		UserID:        receiverAddress.UserID,
		FullName:      receiverAddress.FullName,
		PhoneNumber:   receiverAddress.PhoneNumber,
		ProvinceName:  receiverAddress.ProvinceName,
		DistrictName:  receiverAddress.DistrictName,
		GhnDistrictID: receiverAddress.GhnDistrictID,
		WardName:      receiverAddress.WardName,
		GhnWardCode:   receiverAddress.GhnWardCode,
		Detail:        receiverAddress.Detail,
	}
	
	return delivery.CreateOrderRequest{
		Order:           ghnOrder,
		OrderItems:      ghnOrderItems,
		SenderAddress:   ghnSenderAddress,
		ReceiverAddress: ghnReceiverAddress,
	}
}

// ConfirmOrderTxParams chứa các tham số cần thiết để xác nhận đơn hàng từ người bán
type ConfirmOrderTxParams struct {
	Order    *Order // Đơn hàng cần xác nhận
	SellerID string // ID của người bán xác nhận đơn hàng
}

// ConfirmOrderTxResult chứa kết quả trả về sau khi xác nhận đơn hàng
type ConfirmOrderTxResult struct {
	Order            Order            `json:"order"`             // Đơn hàng đã được cập nhật
	OrderItems       []OrderItem      `json:"order_items"`       // Các mặt hàng trong đơn hàng
	SellerEntry      WalletEntry      `json:"seller_entry"`      // Bút toán cộng tiền cho người bán (pending)
	OrderTransaction OrderTransaction `json:"order_transaction"` // Giao dịch đơn hàng đã được cập nhật với seller_entry_id
}

// ConfirmOrderTx xử lý việc người bán xác nhận đơn hàng
func (store *SQLStore) ConfirmOrderTx(ctx context.Context, arg ConfirmOrderTxParams) (ConfirmOrderTxResult, error) {
	var result ConfirmOrderTxResult
	
	err := store.ExecTx(ctx, func(qTx *Queries) error {
		var err error
		
		// 1. Lấy thông tin các mặt hàng trong đơn hàng
		orderItems, err := qTx.GetOrderItems(ctx, arg.Order.ID.String())
		if err != nil {
			return fmt.Errorf("failed to get order items: %w", err)
		}
		result.OrderItems = orderItems
		
		// 2. Cập nhật trạng thái đơn hàng thành "packaging"
		updatedOrder, err := qTx.ConfirmOrderByID(ctx, ConfirmOrderByIDParams{
			OrderID:  arg.Order.ID,
			SellerID: arg.SellerID,
		})
		if err != nil {
			return fmt.Errorf("failed to confirm order: %w", err)
		}
		result.Order = updatedOrder
		
		// 3. Lấy ví của người bán để cập nhật số dư
		sellerWallet, err := qTx.GetWalletForUpdate(ctx, arg.SellerID)
		if err != nil {
			return fmt.Errorf("failed to get seller wallet: %w", err)
		}
		
		// 4. Cộng tiền hàng vào non_withdrawable_amount của người bán
		// Đây là số tiền người bán sẽ nhận được sau khi người mua xác nhận đã nhận hàng thành công
		err = qTx.AddWalletNonWithdrawableAmount(ctx, AddWalletNonWithdrawableAmountParams{
			Amount:   updatedOrder.ItemsSubtotal,
			WalletID: sellerWallet.ID,
		})
		if err != nil {
			return fmt.Errorf("failed to add non-withdrawable amount: %w", err)
		}
		
		// Tạo bút toán cho non_withdrawable_amount với status completed
		_, err = qTx.CreateWalletEntry(ctx, CreateWalletEntryParams{
			WalletID: sellerWallet.ID,
			ReferenceID: pgtype.Text{
				String: updatedOrder.Code,
				Valid:  true,
			},
			ReferenceType: WalletReferenceTypeOrder,
			EntryType:     WalletEntryTypeNonWithdrawable,
			Amount:        updatedOrder.ItemsSubtotal,
			Status:        WalletEntryStatusCompleted, // Completed vì đã cộng ngay
			CompletedAt: pgtype.Timestamptz{
				Time:  time.Now(),
				Valid: true,
			},
		})
		if err != nil {
			return fmt.Errorf("failed to create non-withdrawable wallet entry: %w", err)
		}
		
		// 5. Tạo bút toán (wallet entry) cho người bán với trạng thái pending
		// Amount là số dương (+) vì đây là bút toán cộng tiền
		sellerEntry, err := qTx.CreateWalletEntry(ctx, CreateWalletEntryParams{
			WalletID: sellerWallet.ID,
			ReferenceID: pgtype.Text{
				String: updatedOrder.Code,
				Valid:  true,
			},
			ReferenceType: WalletReferenceTypeOrder,
			EntryType:     WalletEntryTypePaymentReceived,
			Amount:        updatedOrder.ItemsSubtotal, // Số dương (+)
			Status:        WalletEntryStatusPending,   // Chờ người mua xác nhận nhận hàng thành công
		})
		if err != nil {
			return fmt.Errorf("failed to create seller wallet entry: %w", err)
		}
		result.SellerEntry = sellerEntry
		
		// 6. Cập nhật seller_entry_id trong order_transaction
		// Liên kết bút toán với giao dịch đơn hàng
		orderTransaction, err := qTx.UpdateOrderTransaction(ctx, UpdateOrderTransactionParams{
			SellerEntryID: pgtype.Int8{
				Int64: sellerEntry.ID,
				Valid: true,
			},
			OrderID: updatedOrder.ID.String(),
		})
		if err != nil {
			return fmt.Errorf("failed to update order transaction seller entry: %w", err)
		}
		result.OrderTransaction = orderTransaction
		
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
		packageImages, err := arg.UploadImagesFunc("packaging images", arg.Order.Code, util.FolderOrders, arg.PackageImages...)
		if err != nil {
			return err
		}
		
		updatedOrder, err := qTx.UpdateOrder(ctx, UpdateOrderParams{
			IsPackaged: pgtype.Bool{
				Bool:  true,
				Valid: true,
			},
			PackagingImages: packageImages,
			OrderID:         arg.Order.ID,
		})
		if err != nil {
			return fmt.Errorf("failed to update order: %w", err)
		}
		result.Order = updatedOrder
		
		// 2. Lấy thông tin các mặt hàng trong đơn hàng
		orderItems, err := qTx.GetOrderItems(ctx, updatedOrder.ID.String())
		if err != nil {
			return fmt.Errorf("failed to get order items: %w", err)
		}
		
		// 3. Lấy thông tin giao hàng
		orderDelivery, err := qTx.GetOrderDelivery(ctx, updatedOrder.ID.String())
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
			ID: orderDelivery.ID,
			DeliveryTrackingCode: pgtype.Text{
				String: ghnResponse.Data.OrderCode,
				Valid:  true,
			},
			ExpectedDeliveryTime: pgtype.Timestamptz{
				Time:  ghnResponse.Data.ExpectedDeliveryTime,
				Valid: true,
			},
			Status: pgtype.Text{String: "ready_to_pick", Valid: true}, // Hardcode status vì GHN không trả về
			OverallStatus: NullDeliveryOverralStatus{
				DeliveryOverralStatus: DeliveryOverralStatusPicking,
				Valid:                 true,
			},
		})
		if err != nil {
			return fmt.Errorf("failed to update order delivery: %w", err)
		}
		result.OrderDelivery = updatedDelivery
		
		return nil
	})
	
	return result, err
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

func (store *SQLStore) ConfirmOrderReceivedTx(ctx context.Context, arg ConfirmOrderReceivedTxParams) (ConfirmOrderReceivedTxResult, error) {
	var result ConfirmOrderReceivedTxResult
	
	err := store.ExecTx(ctx, func(qTx *Queries) error {
		// 1. Chuyển tiền từ non_withdrawable_amount sang balance của người bán
		var err error
		updatedWallet, err := qTx.TransferNonWithdrawableToBalance(ctx, TransferNonWithdrawableToBalanceParams{
			Amount:   arg.Order.ItemsSubtotal,
			WalletID: arg.SellerWallet.ID,
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
			CompletedAt: pgtype.Timestamptz{
				Time:  time.Now(),
				Valid: true,
			},
		})
		if err != nil {
			return fmt.Errorf("failed to update seller wallet entry status: %w", err)
		}
		result.SellerEntry = sellerEntry
		
		// 3. Cập nhật trạng thái giao dịch đơn hàng thành "completed"
		updatedOrderTransaction, err := qTx.UpdateOrderTransaction(ctx, UpdateOrderTransactionParams{
			Status: NullOrderTransactionStatus{
				OrderTransactionStatus: OrderTransactionStatusCompleted,
				Valid:                  true,
			},
			OrderID: arg.Order.ID.String(),
		})
		if err != nil {
			return fmt.Errorf("failed to update order transaction status: %w", err)
		}
		result.OrderTransaction = updatedOrderTransaction
		
		// 4. Chuyển quyền sở hữu các mặt hàng trong đơn hàng cho người mua,
		// cũng như cập nhật trạng thái của chúng thành "in store"
		for _, item := range arg.OrderItems {
			err = qTx.UpdateGundam(ctx, UpdateGundamParams{
				ID: item.GundamID,
				OwnerID: pgtype.Text{
					String: arg.Order.BuyerID,
					Valid:  true,
				},
				Status: NullGundamStatus{
					GundamStatus: GundamStatusInstore,
					Valid:        true,
				},
			})
			if err != nil {
				return fmt.Errorf("failed to update gundam owner: %w", err)
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
		
		return nil
	})
	
	return result, err
}
