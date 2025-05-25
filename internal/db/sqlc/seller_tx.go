package db

import (
	"context"
	"fmt"
	"time"
	
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
		_, err = qTx.UpdateCurrentActiveSubscriptionForSeller(ctx, UpdateCurrentActiveSubscriptionForSellerParams{
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
		_, err = qTx.UpdateCurrentActiveSubscriptionForSeller(ctx, UpdateCurrentActiveSubscriptionForSellerParams{
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
		
		// Tạo bút toán cho non_withdrawable_amount với status completed ✅
		_, err = qTx.CreateWalletEntry(ctx, CreateWalletEntryParams{
			WalletID:      sellerWallet.UserID,
			ReferenceID:   &updatedOrder.Code,
			ReferenceType: WalletReferenceTypeOrder,
			EntryType:     WalletEntryTypeHoldFunds,
			AffectedField: WalletAffectedFieldNonWithdrawableAmount,
			Amount:        updatedOrder.ItemsSubtotal,
			Status:        WalletEntryStatusCompleted, // Completed vì đã cộng ngay
			CompletedAt:   util.TimePointer(time.Now()),
		})
		if err != nil {
			return fmt.Errorf("failed to create non-withdrawable wallet entry: %w", err)
		}
		
		// 5. Tạo bút toán (wallet entry) cho người bán với trạng thái pending
		// Amount là số dương (+) vì đây là bút toán cộng tiền ✅
		sellerEntry, err := qTx.CreateWalletEntry(ctx, CreateWalletEntryParams{
			WalletID:      sellerWallet.UserID,
			ReferenceID:   &updatedOrder.Code,
			ReferenceType: WalletReferenceTypeOrder,
			EntryType:     WalletEntryTypePaymentReceived,
			AffectedField: WalletAffectedFieldBalance,
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
		
		return nil
	})
	
	return result, err
}

type CancelOrderBySellerTxParams struct {
	Order  *Order
	Reason *string
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
				log.Warn().Msgf("Gundam ID not found in order item %d", item.ID)
			}
		}
		
		return nil
	})
	
	return result, err
}

type UpgradeSubscriptionTxParams struct {
	SellerID          string
	OldSubscriptionID int64
	NewPlanID         int64
	NewPlanPrice      int64
	NewPlanDuration   *int64 // duration in days
}

type UpgradeSubscriptionTxResult struct {
	OldSubscription SellerSubscription `json:"old_subscription"`
	NewSubscription SellerSubscription `json:"new_subscription"`
	PaymentEntry    *WalletEntry       `json:"payment_entry"`
}

func (store *SQLStore) UpgradeSubscriptionTx(ctx context.Context, arg UpgradeSubscriptionTxParams) (UpgradeSubscriptionTxResult, error) {
	var result UpgradeSubscriptionTxResult
	
	err := store.ExecTx(ctx, func(qTx *Queries) error {
		var err error
		
		// 1. Deactivate old subscription
		result.OldSubscription, err = qTx.UpdateCurrentActiveSubscriptionForSeller(ctx, UpdateCurrentActiveSubscriptionForSellerParams{
			IsActive:       util.BoolPointer(false), // Deactivate old subscription
			SubscriptionID: arg.OldSubscriptionID,
			SellerID:       arg.SellerID,
		})
		if err != nil {
			return fmt.Errorf("failed to deactivate old subscription: %w", err)
		}
		
		// 2. Calculate end date for new subscription
		var endDate *time.Time
		if arg.NewPlanDuration != nil {
			ed := time.Now().AddDate(0, 0, int(*arg.NewPlanDuration))
			endDate = &ed
		}
		
		// 3. Create new subscription (reset counters to 0)
		result.NewSubscription, err = qTx.CreateSellerSubscription(ctx, CreateSellerSubscriptionParams{
			SellerID:         arg.SellerID,
			PlanID:           arg.NewPlanID,
			EndDate:          endDate,
			ListingsUsed:     0, // Reset to 0
			OpenAuctionsUsed: 0, // Reset to 0
			IsActive:         true,
		})
		if err != nil {
			return fmt.Errorf("failed to create new subscription: %w", err)
		}
		
		// 4. Process payment (only if plan has cost)
		if arg.NewPlanPrice > 0 {
			// Create wallet entry
			paymentEntry, err := qTx.CreateWalletEntry(ctx, CreateWalletEntryParams{
				WalletID:      arg.SellerID,
				ReferenceID:   util.StringPointer(fmt.Sprintf("%d", result.NewSubscription.ID)),
				ReferenceType: WalletReferenceTypeSubscription,
				EntryType:     WalletEntryTypeSubscriptionPayment,
				AffectedField: WalletAffectedFieldBalance,
				Amount:        -arg.NewPlanPrice,
				Status:        WalletEntryStatusCompleted,
				CompletedAt:   util.TimePointer(time.Now()),
			})
			if err != nil {
				return fmt.Errorf("failed to create payment entry: %w", err)
			}
			result.PaymentEntry = &paymentEntry
			
			// Update wallet balance
			_, err = qTx.AddWalletBalance(ctx, AddWalletBalanceParams{
				UserID: arg.SellerID,
				Amount: -arg.NewPlanPrice,
			})
			if err != nil {
				return fmt.Errorf("failed to update wallet balance: %w", err)
			}
		}
		
		return nil
	})
	
	return result, err
}
