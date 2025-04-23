package db

import (
	"context"
	"fmt"
	"mime/multipart"
	"time"
	
	"github.com/katatrina/gundam-BE/internal/delivery"
	"github.com/katatrina/gundam-BE/internal/util"
	"github.com/rs/zerolog/log"
)

type PublishGundamTxParams struct {
	GundamID             int64
	SellerID             string
	ActiveSubscriptionID int64
	ListingsUsed         int64
}

func (store *SQLStore) PublishGundamTx(ctx context.Context, arg PublishGundamTxParams) error {
	err := store.ExecTx(ctx, func(qTx *Queries) error {
		// Update gundam status to published
		err := qTx.UpdateGundam(ctx, UpdateGundamParams{
			ID: arg.GundamID,
			Status: NullGundamStatus{
				GundamStatus: GundamStatusPublished,
				Valid:        true,
			},
		})
		if err != nil {
			return err
		}
		
		// Plus 1 to the seller's listings used
		err = qTx.UpdateCurrentActiveSubscriptionForSeller(ctx, UpdateCurrentActiveSubscriptionForSellerParams{
			ListingsUsed:   &arg.ListingsUsed,
			SubscriptionID: arg.ActiveSubscriptionID,
			SellerID:       arg.SellerID,
		})
		if err != nil {
			return err
		}
		
		return nil
	})
	
	return err
}

type UnpublishGundamTxParams struct {
	GundamID int64
	SellerID string
}

func (store *SQLStore) UnpublishGundamTx(ctx context.Context, arg UnpublishGundamTxParams) error {
	err := store.ExecTx(ctx, func(qTx *Queries) error {
		// Update gundam status to "in store"
		err := qTx.UpdateGundam(ctx, UpdateGundamParams{
			ID: arg.GundamID,
			Status: NullGundamStatus{
				GundamStatus: GundamStatusInstore,
				Valid:        true,
			},
		})
		if err != nil {
			return err
		}
		
		subscription, err := qTx.GetCurrentActiveSubscriptionDetailsForSeller(ctx, arg.SellerID)
		if err != nil {
			return err
		}
		
		// Minus 1 to the seller's listings used
		err = qTx.UpdateCurrentActiveSubscriptionForSeller(ctx, UpdateCurrentActiveSubscriptionForSellerParams{
			ListingsUsed:   util.Int64Pointer(subscription.ListingsUsed - 1),
			SubscriptionID: subscription.ID,
			SellerID:       arg.SellerID,
		})
		if err != nil {
			return err
		}
		
		return nil
	})
	
	return err
}

// ConfirmOrderTxParams chứa các tham số cần thiết để xác nhận đơn hàng từ người bán
type ConfirmOrderTxParams struct {
	Order    *Order // Đơn hàng cần xác nhận
	SellerID string // OfferID của người bán xác nhận đơn hàng
}

// ConfirmOrderTxResult chứa kết quả trả về sau khi xác nhận đơn hàng
type ConfirmOrderTxResult struct {
	Order            Order            `json:"order"`             // Đơn hàng đã được cập nhật
	OrderItems       []OrderItem      `json:"order_items"`       // Các mặt hàng trong đơn hàng
	SellerEntry      WalletEntry      `json:"seller_entry"`      // Bút toán cộng tiền cho người bán (pending)
	OrderTransaction OrderTransaction `json:"order_transaction"` // Giao dịch đơn hàng đã được cập nhật với seller_entry_id
}

// ConfirmOrderTx xử lý việc người bán xác nhận đơn hàng
func (store *SQLStore) ConfirmOrderBySellerTx(ctx context.Context, arg ConfirmOrderTxParams) (ConfirmOrderTxResult, error) {
	var result ConfirmOrderTxResult
	
	err := store.ExecTx(ctx, func(qTx *Queries) error {
		var err error
		
		// 1. Lấy thông tin các mặt hàng trong đơn hàng
		orderItems, err := qTx.ListOrderItems(ctx, arg.Order.ID)
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
			Amount: updatedOrder.ItemsSubtotal,
			UserID: sellerWallet.UserID,
		})
		if err != nil {
			return fmt.Errorf("failed to add non-withdrawable amount: %w", err)
		}
		
		// Tạo bút toán cho non_withdrawable_amount với status completed
		_, err = qTx.CreateWalletEntry(ctx, CreateWalletEntryParams{
			WalletID:      sellerWallet.UserID,
			ReferenceID:   &updatedOrder.Code,
			ReferenceType: WalletReferenceTypeOrder,
			EntryType:     WalletEntryTypeNonWithdrawable,
			Amount:        updatedOrder.ItemsSubtotal,
			Status:        WalletEntryStatusCompleted, // Completed vì đã cộng ngay
			CompletedAt:   util.TimePointer(time.Now()),
		})
		if err != nil {
			return fmt.Errorf("failed to create non-withdrawable wallet entry: %w", err)
		}
		
		// 5. Tạo bút toán (wallet entry) cho người bán với trạng thái pending
		// Amount là số dương (+) vì đây là bút toán cộng tiền
		sellerEntry, err := qTx.CreateWalletEntry(ctx, CreateWalletEntryParams{
			WalletID:      sellerWallet.UserID,
			ReferenceID:   &updatedOrder.Code,
			ReferenceType: WalletReferenceTypeOrder,
			EntryType:     WalletEntryTypePaymentreceived,
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
			SellerEntryID: &sellerEntry.ID,
			OrderID:       updatedOrder.ID,
		})
		if err != nil {
			return fmt.Errorf("failed to update order transaction seller entry: %w", err)
		}
		result.OrderTransaction = orderTransaction
		
		// TODO: Cần xử lý thêm nếu đơn hàng là
		
		return nil
	})
	
	return result, err
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
	if order.Note != nil {
		ghnOrder.Note = *order.Note
	}
	
	ghnOrderItems := make([]delivery.OrderItemInfo, len(orderItems))
	for i, item := range orderItems {
		ghnOrderItems[i] = delivery.OrderItemInfo{
			OrderID:  item.OrderID.String(),
			Name:     item.Name,
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

func (store *SQLStore) PackageOrderBySellerTx(ctx context.Context, arg PackageOrderTxParams) (PackageOrderTxResult, error) {
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

type CancelOrderBySellerTxParams struct {
	Order          *Order
	CanceledReason string
}

type CancelOrderBySellerTxResult struct {
	Order            Order            `json:"order"`
	OrderTransaction OrderTransaction `json:"order_transaction"`
	RefundEntry      WalletEntry      `json:"refund_entry"`
	BuyerWallet      Wallet           `json:"buyer_wallet"`
}

func (store *SQLStore) CancelOrderBySellerTx(ctx context.Context, arg CancelOrderBySellerTxParams) (CancelOrderBySellerTxResult, error) {
	var result CancelOrderBySellerTxResult
	
	err := store.ExecTx(ctx, func(qTx *Queries) error {
		// 1. Cập nhật trạng thái đơn hàng thành "canceled"
		updatedOrder, err := qTx.UpdateOrder(ctx, UpdateOrderParams{
			OrderID: arg.Order.ID,
			Status: NullOrderStatus{
				OrderStatus: OrderStatusCanceled,
				Valid:       true,
			},
			CanceledBy:     util.StringPointer(arg.Order.SellerID),
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
				log.Warn().Msgf("Gundam OfferID not found in order item %d", item.ID)
			}
		}
		
		return nil
	})
	
	return result, err
}
