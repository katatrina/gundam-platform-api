package db

import (
	"context"
	"fmt"
	"time"
	
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/katatrina/gundam-BE/internal/ghn"
	"github.com/katatrina/gundam-BE/internal/util"
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
		// khi người bán xác nhận và đóng gói đơn hàng
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

func ConvertToGHNOrderRequest(order Order, orderItems []OrderItem, senderAddress, receiverAddress DeliveryInformation) ghn.CreateGHNOrderRequest {
	ghnOrder := ghn.OrderInfo{
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
	
	ghnOrderItems := make([]ghn.OrderItemInfo, len(orderItems))
	for i, item := range orderItems {
		ghnOrderItems[i] = ghn.OrderItemInfo{
			OrderID:  item.OrderID,
			GundamID: item.GundamID,
			Price:    item.Price,
			Quantity: item.Quantity,
			Weight:   item.Weight,
		}
	}
	
	ghnSenderAddress := ghn.AddressInfo{
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
	
	ghnReceiverAddress := ghn.AddressInfo{
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
	
	return ghn.CreateGHNOrderRequest{
		Order:           ghnOrder,
		OrderItems:      ghnOrderItems,
		SenderAddress:   ghnSenderAddress,
		ReceiverAddress: ghnReceiverAddress,
	}
}

// ConfirmOrderTxParams chứa các tham số cần thiết để xác nhận đơn hàng từ người bán
type ConfirmOrderTxParams struct {
	OrderID  uuid.UUID // ID của đơn hàng cần xác nhận
	SellerID string    // ID của người bán xác nhận đơn hàng
}

// ConfirmOrderTxResult chứa kết quả trả về sau khi xác nhận đơn hàng
type ConfirmOrderTxResult struct {
	Order            Order            `json:"order"`             // Đơn hàng đã được cập nhật
	OrderItems       []OrderItem      `json:"order_items"`       // Các mặt hàng trong đơn hàng
	SellerEntry      WalletEntry      `json:"seller_entry"`      // Bút toán cộng tiền cho người bán (pending)
	OrderTransaction OrderTransaction `json:"order_transaction"` // Giao dịch đơn hàng đã được cập nhật với seller_entry_id
}

// ConfirmOrderTx xử lý việc người bán xác nhận đơn hàng
// Quy trình bao gồm: cập nhật trạng thái đơn hàng, tạo đơn hàng trên GHN,
// cộng tiền vào non_withdrawable_amount của người bán và tạo bút toán tương ứng
func (store *SQLStore) ConfirmOrderTx(ctx context.Context, arg ConfirmOrderTxParams) (ConfirmOrderTxResult, error) {
	var result ConfirmOrderTxResult
	
	err := store.ExecTx(ctx, func(qTx *Queries) error {
		var err error
		
		// 1. Lấy thông tin đơn hàng và kiểm tra trạng thái
		order, err := qTx.GetSalesOrderForUpdate(ctx, GetSalesOrderForUpdateParams{
			OrderID:  arg.OrderID,
			SellerID: arg.SellerID,
		})
		if err != nil {
			return fmt.Errorf("failed to get order: %w", err)
		}
		
		// Kiểm tra xem đơn hàng có thuộc về người bán không
		if order.SellerID != arg.SellerID {
			return ErrOrderNotBelongToUser
		}
		
		// Chỉ xác nhận đơn hàng ở trạng thái pending
		if order.Status != OrderStatusPending {
			return ErrOrderNotPendingStatus
		}
		
		// TODO: Có thể kiểm tra chi tiết hơn nếu muốn
		
		// 2. Lấy thông tin các mặt hàng trong đơn hàng
		orderItems, err := qTx.GetOrderItems(ctx, arg.OrderID.String())
		if err != nil {
			return fmt.Errorf("failed to get order items: %w", err)
		}
		result.OrderItems = orderItems
		
		// 3. Cập nhật trạng thái đơn hàng thành "packaging"
		updatedOrder, err := qTx.ConfirmOrder(ctx, ConfirmOrderParams{
			OrderID:  arg.OrderID,
			SellerID: arg.SellerID,
		})
		if err != nil {
			return fmt.Errorf("failed to confirm order: %w", err)
		}
		result.Order = updatedOrder
		
		// 4. Lấy ví của người bán để cập nhật số dư
		sellerWallet, err := qTx.GetWalletForUpdate(ctx, arg.SellerID)
		if err != nil {
			return fmt.Errorf("failed to get seller wallet: %w", err)
		}
		
		// 5. Cộng tiền vào non_withdrawable_amount của người bán
		// Đây là số tiền người bán sẽ nhận được sau khi người mua xác nhận đã nhận hàng thành công
		err = qTx.AddWalletNonWithdrawableAmount(ctx, AddWalletNonWithdrawableAmountParams{
			Amount:   order.ItemsSubtotal,
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
			Amount:        order.ItemsSubtotal,
			Status:        WalletEntryStatusCompleted, // Completed vì đã cộng ngay
		})
		if err != nil {
			return fmt.Errorf("failed to create non-withdrawable wallet entry: %w", err)
		}
		
		// 6. Tạo bút toán (wallet entry) cho người bán với trạng thái pending
		// Amount là số dương (+) vì đây là bút toán cộng tiền
		sellerEntry, err := qTx.CreateWalletEntry(ctx, CreateWalletEntryParams{
			WalletID: sellerWallet.ID,
			ReferenceID: pgtype.Text{
				String: updatedOrder.Code,
				Valid:  true,
			},
			ReferenceType: WalletReferenceTypeOrder,
			EntryType:     WalletEntryTypePaymentReceived,
			Amount:        order.ItemsSubtotal,      // Số dương (+)
			Status:        WalletEntryStatusPending, // Chờ người mua xác nhận nhận hàng thành công
		})
		if err != nil {
			return fmt.Errorf("failed to create seller wallet entry: %w", err)
		}
		result.SellerEntry = sellerEntry
		
		// 7. Cập nhật seller_entry_id trong order_transaction
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
