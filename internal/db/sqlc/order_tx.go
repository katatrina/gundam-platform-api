package db

import (
	"context"
	"fmt"
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
	PaymentMethod        PaymentMethod
	Note                 pgtype.Text
	Gundams              []Gundam
}

type GundamOrderItem struct {
	ID       int64
	Price    int64
	Quantity int32
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
		
		// 1. Kiểm tra và cập nhật số dư ví của người mua (nếu thanh toán bằng ví)
		buyerWallet, err = qTx.GetWalletForUpdate(ctx, arg.BuyerID)
		if err != nil {
			return fmt.Errorf("failed to get buyer wallet: %w", err)
		}
		
		if buyerWallet.Balance < arg.TotalAmount {
			return fmt.Errorf("insufficient balance: available %d, needed %d",
				buyerWallet.Balance, arg.TotalAmount)
		}
		
		// 2. Tạo order
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
		
		// Trừ tiền từ ví người mua
		_, err = qTx.AddWalletBalance(ctx, AddWalletBalanceParams{
			ID:      buyerWallet.ID,
			Balance: -arg.TotalAmount, // Truyền số âm để trừ
		})
		if err != nil {
			return fmt.Errorf("failed to deduct balance: %w", err)
		}
		
		// Tạo wallet entry cho người mua
		buyerEntry, err = qTx.CreateWalletEntry(ctx, CreateWalletEntryParams{
			WalletID: buyerWallet.ID,
			ReferenceID: pgtype.Text{
				String: order.ID,
				Valid:  true,
			},
			ReferenceType: NullWalletReferenceType{
				WalletReferenceType: WalletReferenceTypeOrder,
				Valid:               true,
			},
			EntryType: WalletEntryTypePayment,
			Amount:    -arg.TotalAmount,
			Status:    WalletEntryStatusCompleted,
		})
		if err != nil {
			return fmt.Errorf("failed to create buyer wallet entry: %w", err)
		}
		
		// 3. Tạo các order items
		for _, gundam := range arg.Gundams {
			var orderItem OrderItem
			orderItem, err = qTx.CreateOrderItem(ctx, CreateOrderItemParams{
				OrderID:  order.ID,
				GundamID: gundam.ID,
				Price:    gundam.Price,
				Quantity: gundam.Quantity,
			})
			if err != nil {
				return err
			}
			
			// 4. Cập nhật trạng thái Gundam
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
		// Một số trường có dữ liệu ban đầu là null, và sẽ được cập nhật sau.
		orderDelivery, err := qTx.CreateOrderDelivery(ctx, CreateOrderDeliveryParams{
			OrderID:              order.ID,
			ExpectedDeliveryTime: arg.ExpectedDeliveryTime,
			FromID:               sellerDelivery.ID,
			ToID:                 buyerDelivery.ID,
		})
		if err != nil {
			return err
		}
		result.OrderDelivery = orderDelivery
		
		// 7. Tạo order transaction
		if _, err = qTx.CreateOrderTransaction(ctx, CreateOrderTransactionParams{
			OrderID:      order.ID,
			Amount:       arg.TotalAmount,
			Status:       OrderTransactionStatusPending,
			BuyerEntryID: buyerEntry.ID,
		}); err != nil {
			return fmt.Errorf("failed to create order transaction: %w", err)
		}
		
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
		FullName:      arg.PickupAddress.FullName,
		PhoneNumber:   arg.PickupAddress.PhoneNumber,
		ProvinceName:  arg.PickupAddress.ProvinceName,
		DistrictName:  arg.PickupAddress.DistrictName,
		GhnDistrictID: arg.PickupAddress.GhnDistrictID,
		WardName:      arg.PickupAddress.WardName,
		GhnWardCode:   arg.PickupAddress.GhnWardCode,
		Detail:        arg.PickupAddress.Detail,
	})
	return
}
