package db

import (
	"context"
	"time"
	
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/katatrina/gundam-BE/internal/util"
)

type CreateOrderTxParams struct {
	BuyerID              string
	BuyerAddress         UserAddress
	SellerID             string
	PickupAddress        UserAddress
	ItemsSubtotal        int64
	TotalAmount          int64
	DeliveryFee          int64
	ExpectedDeliveryTime time.Time
	PaymentMethod
	Note    pgtype.Text
	Gundams []Gundam
}

type CreateOrderTxResult struct {
	Order         `json:"order"`
	OrderItems    []OrderItem `json:"order_items"`
	OrderDelivery `json:"order_delivery"`
}

func (store *SQLStore) CreateOrderTx(ctx context.Context, arg CreateOrderTxParams) (CreateOrderTxResult, error) {
	var result CreateOrderTxResult
	
	err := store.ExecTx(ctx, func(qTx *Queries) error {
		// Tạo record order
		// Một order luôn có status ban đầu là "pending"
		order, err := qTx.CreateOrder(ctx, CreateOrderParams{
			ID:            util.GenerateOrderID(),
			BuyerID:       arg.BuyerID,
			SellerID:      arg.SellerID,
			ItemsSubtotal: arg.ItemsSubtotal,
			DeliveryFee:   arg.DeliveryFee,
			TotalAmount:   arg.TotalAmount,
			Status:        OrderStatusPending,
			PaymentMethod: arg.PaymentMethod,
			Note:          arg.Note,
		})
		if err != nil {
			return err
		}
		result.Order = order
		
		// Tạo record order item
		for _, gundam := range arg.Gundams {
			var orderItem OrderItem
			orderItem, err = qTx.CreateOrderItem(ctx, CreateOrderItemParams{
				OrderID:  order.ID,
				GundamID: gundam.ID,
				Price:    gundam.Price,
			})
			if err != nil {
				return err
			}
			result.OrderItems = append(result.OrderItems, orderItem)
			
			// Cập nhật trạng thái gundam thành "processing"
			err = qTx.UpdateGundam(ctx, UpdateGundamParams{
				ID: gundam.ID,
				Status: NullGundamStatus{
					GundamStatus: GundamStatusProcessing,
					Valid:        true,
				},
			})
			if err != nil {
				return err
			}
		}
		
		// Tạo record delivery information cho cả người bán và người mua.
		// Thực chất chỉ là clone lại địa chỉ đã tạo.
		buyerDeliveryInfo, err := qTx.CreateDeliveryInformation(ctx, CreateDeliveryInformationParams{
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
			return err
		}
		
		pickupDeliveryInfo, err := qTx.CreateDeliveryInformation(ctx, CreateDeliveryInformationParams{
			UserID:        arg.SellerID,
			FullName:      arg.PickupAddress.FullName,
			PhoneNumber:   arg.PickupAddress.PhoneNumber,
			ProvinceName:  arg.PickupAddress.ProvinceName,
			DistrictName:  arg.PickupAddress.DistrictName,
			GhnDistrictID: arg.PickupAddress.GhnDistrictID,
			WardName:      arg.PickupAddress.WardName,
			GhnWardCode:   arg.PickupAddress.GhnWardCode,
			Detail:        arg.PickupAddress.Detail,
		})
		if err != nil {
			return err
		}
		
		// Tạo record order delivery
		// Một số thông tin như GhnOrderCode, Status, OverallStatus
		// sẽ được cập nhật sau khi seller xác nhận đơn hàng.
		// Chỉ tạo order bên ghn khi seller xác nhận đơn hàng.
		orderDelivery, err := qTx.CreateOrderDelivery(ctx, CreateOrderDeliveryParams{
			OrderID:              order.ID,
			ExpectedDeliveryTime: arg.ExpectedDeliveryTime,
			FromID:               pickupDeliveryInfo.ID,
			ToID:                 buyerDeliveryInfo.ID,
		})
		if err != nil {
			return err
		}
		result.OrderDelivery = orderDelivery
		
		// TODO: Trừ tiền của người mua
		// if arg.PaymentMethod == PaymentMethodWallet {
		// 	// 1. Lấy ví người mua với lock để tránh race condition
		// 	buyerWallet, err := qTx.GetWalletForUpdate(ctx, arg.BuyerID)
		// 	if err != nil {
		// 		return fmt.Errorf("failed to get buyer wallet: %w", err)
		// 	}
		//
		// 	// 2. Kiểm tra số dư khả dụng
		// 	if buyerWallet.Balance < arg.TotalAmount {
		// 		return fmt.Errorf("insufficient balance: available %d, needed %d",
		// 			buyerWallet.Balance, arg.TotalAmount)
		// 	}
		//
		// 	// 3. Cập nhật số dư ví
		// 	updatedWallet, err := qTx.UpdateWalletBalance(ctx, UpdateWalletBalanceParams{
		// 		ID:      buyerWallet.ID,
		// 		Balance: buyerWallet.Balance - arg.TotalAmount,
		// 	})
		// 	if err != nil {
		// 		return fmt.Errorf("failed to deduct balance: %w", err)
		// 	}
		//
		// 	// 4. Tạo bút toán trừ tiền
		// 	walletEntry, err := qTx.CreateWalletEntry(ctx, CreateWalletEntryParams{
		// 		WalletID:      buyerWallet.ID,
		// 		ReferenceID:   order.ID,
		// 		ReferenceType: ReferenceTypeOrder,
		// 		EntryType:     EntryTypePayment,
		// 		Amount:        -arg.TotalAmount,
		// 		Status:        EntryStatusCompleted,
		// 	})
		// 	if err != nil {
		// 		return fmt.Errorf("failed to create wallet entry: %w", err)
		// 	}
		//
		// 	// 5. Tạo payment transaction
		// 	_, err = qTx.CreateOrderTransaction(ctx, CreateOrderTransactionParams{
		// 		OrderID:      order.ID,
		// 		Amount:       arg.TotalAmount,
		// 		Status:       OrderTransactionStatusPending,
		// 		BuyerEntryID: walletEntry.ID,
		// 	})
		// 	if err != nil {
		// 		return fmt.Errorf("failed to create payment transaction: %w", err)
		// 	}
		// }
		
		return nil
	})
	
	return result, err
}
