package db

import (
	"context"
	"fmt"
	"time"
	
	"github.com/google/uuid"
	"github.com/katatrina/gundam-BE/internal/util"
	"github.com/rs/zerolog/log"
)

type ProvideDeliveryAddressesForExchangeTxParams struct {
	ExchangeID  uuid.UUID
	UserID      string
	IsPoster    bool
	FromAddress UserAddress
	ToAddress   UserAddress
}

type ProvideDeliveryAddressesForExchangeTxResult struct {
	Exchange Exchange `json:"exchange"`
}

func (store *SQLStore) ProvideDeliveryAddressesForExchangeTx(ctx context.Context, arg ProvideDeliveryAddressesForExchangeTxParams) (ProvideDeliveryAddressesForExchangeTxResult, error) {
	var result ProvideDeliveryAddressesForExchangeTxResult
	
	err := store.ExecTx(ctx, func(qTx *Queries) error {
		// 1. Tạo bản ghi delivery_information cho địa chỉ gửi
		fromDeliveryInfoParams := CreateDeliveryInformationParams{
			UserID:        arg.UserID,
			FullName:      arg.FromAddress.FullName,
			PhoneNumber:   arg.FromAddress.PhoneNumber,
			ProvinceName:  arg.FromAddress.ProvinceName,
			DistrictName:  arg.FromAddress.DistrictName,
			GhnDistrictID: arg.FromAddress.GhnDistrictID,
			WardName:      arg.FromAddress.WardName,
			GhnWardCode:   arg.FromAddress.GhnWardCode,
			Detail:        arg.FromAddress.Detail,
		}
		
		fromDeliveryInfo, err := qTx.CreateDeliveryInformation(ctx, fromDeliveryInfoParams)
		if err != nil {
			return err
		}
		
		// 2. Tạo bản ghi delivery_information cho địa chỉ nhận
		toDeliveryInfoParams := CreateDeliveryInformationParams{
			UserID:        arg.UserID,
			FullName:      arg.ToAddress.FullName,
			PhoneNumber:   arg.ToAddress.PhoneNumber,
			ProvinceName:  arg.ToAddress.ProvinceName,
			DistrictName:  arg.ToAddress.DistrictName,
			GhnDistrictID: arg.ToAddress.GhnDistrictID,
			WardName:      arg.ToAddress.WardName,
			GhnWardCode:   arg.ToAddress.GhnWardCode,
			Detail:        arg.ToAddress.Detail,
		}
		
		toDeliveryInfo, err := qTx.CreateDeliveryInformation(ctx, toDeliveryInfoParams)
		if err != nil {
			return err
		}
		
		// 3. Cập nhật thông tin vận chuyển cho exchange
		var updateParams UpdateExchangeParams
		updateParams.ID = arg.ExchangeID
		
		if arg.IsPoster {
			updateParams.PosterFromDeliveryID = &fromDeliveryInfo.ID
			updateParams.PosterToDeliveryID = &toDeliveryInfo.ID
		} else {
			updateParams.OffererFromDeliveryID = &fromDeliveryInfo.ID
			updateParams.OffererToDeliveryID = &toDeliveryInfo.ID
		}
		
		// Chỉ cập nhật các trường thông tin vận chuyển tương ứng với vai trò của người dùng,
		// các trường khác giữ nguyên giá trị.
		updatedExchange, err := qTx.UpdateExchange(ctx, updateParams)
		if err != nil {
			return err
		}
		result.Exchange = updatedExchange
		
		return nil
	})
	
	return result, err
}

type PayExchangeDeliveryFeeTxParams struct {
	ExchangeID           uuid.UUID
	UserID               string
	IsPoster             bool
	DeliveryFee          int64
	ExpectedDeliveryTime time.Time
	Note                 *string
}

type PayExchangeDeliveryFeeTxResult struct {
	Exchange        Exchange `json:"exchange"`
	BothPartiesPaid bool     `json:"both_parties_paid"`
	PartnerHasPaid  bool     `json:"partner_has_paid"`
	PosterOrder     *Order   `json:"poster_order"`
	OffererOrder    *Order   `json:"offerer_order"`
}

func (store *SQLStore) PayExchangeDeliveryFeeTx(ctx context.Context, arg PayExchangeDeliveryFeeTxParams) (PayExchangeDeliveryFeeTxResult, error) {
	var result PayExchangeDeliveryFeeTxResult
	
	err := store.ExecTx(ctx, func(qTx *Queries) error {
		var err error
		
		// 1. Cập nhật phí vận chuyển, thời gian giao hàng dự kiến, và ghi chú (nếu có) vào bảng exchanges
		updateExchangeParams := UpdateExchangeParams{
			ID: arg.ExchangeID,
		}
		
		if arg.IsPoster {
			updateExchangeParams.PosterDeliveryFee = &arg.DeliveryFee
			updateExchangeParams.PosterOrderExpectedDeliveryTime = &arg.ExpectedDeliveryTime
			updateExchangeParams.PosterOrderNote = arg.Note
		} else {
			updateExchangeParams.OffererDeliveryFee = &arg.DeliveryFee
			updateExchangeParams.OffererOrderExpectedDeliveryTime = &arg.ExpectedDeliveryTime
			updateExchangeParams.OffererOrderNote = arg.Note
		}
		
		updatedExchange, err := qTx.UpdateExchange(ctx, updateExchangeParams)
		if err != nil {
			return err
		}
		
		// 2. Trừ tiền từ ví người dùng
		_, err = qTx.AddWalletBalance(ctx, AddWalletBalanceParams{
			Amount: -arg.DeliveryFee, // Truyền số âm để trừ
			UserID: arg.UserID,
		})
		if err != nil {
			return err
		}
		
		// 3. Tạo wallet entry để ghi lại giao dịch
		_, err = qTx.CreateWalletEntry(ctx, CreateWalletEntryParams{
			WalletID:      arg.UserID,
			ReferenceID:   util.StringPointer(arg.ExchangeID.String()),
			ReferenceType: WalletReferenceTypeExchange,
			EntryType:     WalletEntryTypePayment,
			Amount:        -arg.DeliveryFee, // Số âm vì đây là giao dịch trừ tiền
			Status:        WalletEntryStatusCompleted,
			CompletedAt:   util.TimePointer(time.Now()),
		})
		if err != nil {
			return err
		}
		
		// 4. Cập nhật trạng thái thanh toán phí vận chuyển trong exchange
		var updateStatusParams UpdateExchangeParams
		updateStatusParams.ID = arg.ExchangeID
		
		if arg.IsPoster {
			updateStatusParams.PosterDeliveryFeePaid = util.BoolPointer(true)
		} else {
			updateStatusParams.OffererDeliveryFeePaid = util.BoolPointer(true)
		}
		
		updatedExchange, err = qTx.UpdateExchange(ctx, updateStatusParams)
		if err != nil {
			return err
		}
		
		// 5. Kiểm tra xem cả hai bên đã thanh toán chưa
		partnerHasPaid := false
		if arg.IsPoster {
			partnerHasPaid = updatedExchange.OffererDeliveryFeePaid
		} else {
			partnerHasPaid = updatedExchange.PosterDeliveryFeePaid
		}
		
		result.PartnerHasPaid = partnerHasPaid
		bothPartiesPaid := updatedExchange.PosterDeliveryFeePaid && updatedExchange.OffererDeliveryFeePaid
		result.BothPartiesPaid = bothPartiesPaid
		
		// 6. Nếu cả hai đã thanh toán, tạo hai đơn hàng
		if bothPartiesPaid {
			// 6.1 Tạo đơn hàng cho poster
			posterOrderID, err := uuid.NewV7()
			if err != nil {
				return err
			}
			
			// Tạo mã đơn hàng
			posterOrderCode := util.GenerateOrderCode()
			
			// Lấy posterDeliveryFee từ exchange
			posterDeliveryFee := *updatedExchange.PosterDeliveryFee
			
			posterOrderParams := CreateOrderParams{
				ID:            posterOrderID,
				Code:          posterOrderCode,
				BuyerID:       updatedExchange.PosterID,
				SellerID:      updatedExchange.OffererID,
				ItemsSubtotal: 0, // Không có giá trị sản phẩm trong đơn hàng trao đổi
				DeliveryFee:   posterDeliveryFee,
				TotalAmount:   posterDeliveryFee, // Chỉ tính phí vận chuyển
				Status:        OrderStatusPackaging,
				PaymentMethod: PaymentMethodWallet, // Đã thanh toán qua ví
				Type:          OrderTypeExchange,
				Note:          updatedExchange.PosterOrderNote,
			}
			
			posterOrder, err := qTx.CreateOrder(ctx, posterOrderParams)
			if err != nil {
				return err
			}
			
			// 6.2 Tạo đơn hàng cho offerer
			offererOrderID, err := uuid.NewV7()
			if err != nil {
				return err
			}
			
			// Tạo mã đơn hàng
			offererOrderCode := util.GenerateOrderCode()
			
			// Lấy offererDeliveryFee từ exchange
			offererDeliveryFee := *updatedExchange.OffererDeliveryFee
			
			offererOrderParams := CreateOrderParams{
				ID:            offererOrderID,
				Code:          offererOrderCode,
				BuyerID:       updatedExchange.OffererID,
				SellerID:      updatedExchange.PosterID,
				ItemsSubtotal: 0, // Không có giá trị sản phẩm trong đơn hàng trao đổi
				DeliveryFee:   offererDeliveryFee,
				TotalAmount:   offererDeliveryFee, // Chỉ tính phí vận chuyển
				Status:        OrderStatusPackaging,
				PaymentMethod: PaymentMethodWallet, // Đã thanh toán qua ví
				Type:          OrderTypeExchange,
				Note:          updatedExchange.OffererOrderNote,
			}
			
			offererOrder, err := qTx.CreateOrder(ctx, offererOrderParams)
			if err != nil {
				return err
			}
			
			// 6.3 Tạo thông tin giao hàng cho đơn hàng của poster
			// Các cột status, overall_status, delivery_tracking_code sẽ được cập nhật sau
			// khi người kia đóng gói đơn hàng.
			_, err = qTx.CreateOrderDelivery(ctx, CreateOrderDeliveryParams{
				OrderID:              posterOrderID,
				ExpectedDeliveryTime: *updatedExchange.PosterOrderExpectedDeliveryTime,
				FromDeliveryID:       *updatedExchange.OffererFromDeliveryID,
				ToDeliveryID:         *updatedExchange.PosterToDeliveryID,
			})
			if err != nil {
				return err
			}
			
			// 6.4 Tạo thông tin giao hàng cho đơn hàng của offerer
			// Các cột status, overall_status, delivery_tracking_code sẽ được cập nhật sau
			// khi người kia đóng gói đơn hàng.
			_, err = qTx.CreateOrderDelivery(ctx, CreateOrderDeliveryParams{
				OrderID:              offererOrderID,
				ExpectedDeliveryTime: *updatedExchange.OffererOrderExpectedDeliveryTime,
				FromDeliveryID:       *updatedExchange.PosterFromDeliveryID,
				ToDeliveryID:         *updatedExchange.OffererToDeliveryID,
			})
			if err != nil {
				return err
			}
			
			// 6.5 Lấy danh sách sản phẩm trong giao dịch trao đổi
			exchangeItems, err := qTx.ListExchangeItems(ctx, ListExchangeItemsParams{
				ExchangeID: updatedExchange.ID,
			})
			if err != nil {
				return err
			}
			
			// 6.6 Tạo các order item cho mỗi đơn hàng
			for _, item := range exchangeItems {
				// Xác định đơn hàng cần thêm item này
				orderID := posterOrderID
				if item.IsFromPoster {
					orderID = offererOrderID
				}
				
				// Tạo order item
				_, err = qTx.CreateOrderItem(ctx, CreateOrderItemParams{
					OrderID:  orderID,
					GundamID: item.GundamID,
					Name:     item.Name,
					Slug:     item.Slug,
					Grade:    item.Grade,
					Scale:    item.Scale,
					Quantity: item.Quantity,
					Price:    item.Price,
					Weight:   item.Weight,
					ImageURL: item.ImageURL,
				})
				if err != nil {
					return err
				}
				
				// Cập nhật trạng thái của Gundam
				if item.GundamID != nil {
					err = qTx.UpdateGundam(ctx, UpdateGundamParams{
						OwnerID: item.OwnerID,
						Status: NullGundamStatus{
							GundamStatus: GundamStatusExchanging,
							Valid:        true,
						},
						ID: *item.GundamID,
					})
					if err != nil {
						return err
					}
				}
			}
			
			// 6.7 Cập nhật exchange với ID đơn hàng và chuyển trạng thái
			updateExchangeParams = UpdateExchangeParams{
				ID:             arg.ExchangeID,
				PosterOrderID:  &posterOrderID,
				OffererOrderID: &offererOrderID,
				Status: NullExchangeStatus{
					ExchangeStatus: ExchangeStatusPackaging,
					Valid:          true,
				},
			}
			
			updatedExchange, err = qTx.UpdateExchange(ctx, updateExchangeParams)
			if err != nil {
				return err
			}
			
			result.Exchange = updatedExchange
			result.PosterOrder = &posterOrder
			result.OffererOrder = &offererOrder
		}
		
		return nil
	})
	
	return result, err
}

type ConfirmExchangeOrderReceivedTxParams struct {
	Order          *Order
	Exchange       *Exchange
	ExchangeItems  []ExchangeItem
	PartnerOrderID uuid.UUID
}

type ConfirmExchangeOrderReceivedTxResult struct {
	Order         Order     `json:"order"`
	Exchange      *Exchange `json:"exchange"`
	BothConfirmed bool      `json:"both_confirmed"`
	PartnerOrder  *Order    `json:"partner_order"`
}

// ConfirmExchangeOrderReceivedTx xử lý xác nhận đơn hàng trao đổi đã nhận
func (store *SQLStore) ConfirmExchangeOrderReceivedTx(ctx context.Context, arg ConfirmExchangeOrderReceivedTxParams) (ConfirmExchangeOrderReceivedTxResult, error) {
	var result ConfirmExchangeOrderReceivedTxResult
	
	err := store.ExecTx(ctx, func(qTx *Queries) error {
		var err error
		
		// 1. Cập nhật trạng thái đơn hàng thành "completed"
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
		
		// 2. Lấy thông tin đơn hàng đối tác
		partnerOrder, err := qTx.GetOrderByID(ctx, arg.PartnerOrderID)
		if err != nil {
			return fmt.Errorf("failed to get partner order: %w", err)
		}
		result.PartnerOrder = &partnerOrder
		
		// 3. Kiểm tra xem cả hai đơn hàng đã hoàn thành chưa
		bothCompleted := updatedOrder.Status == OrderStatusCompleted &&
			partnerOrder.Status == OrderStatusCompleted
		result.BothConfirmed = bothCompleted
		
		// 4. Nếu cả hai đơn hàng đã hoàn thành, hoàn tất giao dịch trao đổi
		if bothCompleted {
			// 4.1. Cập nhật trạng thái exchange thành "completed"
			updatedExchange, err := qTx.UpdateExchange(ctx, UpdateExchangeParams{
				ID: arg.Exchange.ID,
				Status: NullExchangeStatus{
					ExchangeStatus: ExchangeStatusCompleted,
					Valid:          true,
				},
				CompletedAt: util.TimePointer(time.Now()),
			})
			if err != nil {
				return fmt.Errorf("failed to update exchange status: %w", err)
			}
			result.Exchange = &updatedExchange
			
			// 4.2. Xử lý tiền bù (nếu có)
			if arg.Exchange.PayerID != nil && arg.Exchange.CompensationAmount != nil {
				// Xác định người nhận tiền bù
				var receiverID string
				if *arg.Exchange.PayerID == arg.Exchange.PosterID {
					receiverID = arg.Exchange.OffererID
				} else {
					receiverID = arg.Exchange.PosterID
				}
				
				// Chuyển tiền từ non_withdrawable_amount sang balance
				_, err := qTx.TransferNonWithdrawableToBalance(ctx, TransferNonWithdrawableToBalanceParams{
					UserID: receiverID,
					Amount: *arg.Exchange.CompensationAmount,
				})
				if err != nil {
					return fmt.Errorf("failed to transfer compensation amount: %w", err)
				}
				
				// Ghi lại giao dịch
				_, err = qTx.CreateWalletEntry(ctx, CreateWalletEntryParams{
					WalletID:      receiverID,
					ReferenceID:   util.StringPointer(arg.Exchange.ID.String()),
					ReferenceType: WalletReferenceTypeExchange,
					EntryType:     WalletEntryTypePaymentreceived,
					Amount:        *arg.Exchange.CompensationAmount,
					Status:        WalletEntryStatusCompleted,
					CompletedAt:   util.TimePointer(time.Now()),
				})
				if err != nil {
					return fmt.Errorf("failed to create wallet entry for compensation: %w", err)
				}
				
				log.Info().
					Str("exchange_id", arg.Exchange.ID.String()).
					Str("receiver_id", receiverID).
					Int64("amount", *arg.Exchange.CompensationAmount).
					Msg("Compensation amount transferred to balance")
			}
			
			// 4.3. Chuyển quyền sở hữu các Gundam trong giao dịch
			for _, item := range arg.ExchangeItems {
				if item.GundamID == nil {
					log.Warn().
						Str("exchange_id", arg.Exchange.ID.String()).
						Str("item_name", item.Name).
						Msg("Exchange item has no gundam_id, skipping ownership transfer")
					continue
				}
				
				// Xác định chủ sở hữu mới
				var newOwnerID string
				if item.IsFromPoster {
					newOwnerID = arg.Exchange.OffererID
				} else {
					newOwnerID = arg.Exchange.PosterID
				}
				
				// Cập nhật chủ sở hữu của Gundam
				err = qTx.UpdateGundam(ctx, UpdateGundamParams{
					ID:      *item.GundamID,
					OwnerID: &newOwnerID,
					Status: NullGundamStatus{
						GundamStatus: GundamStatusInstore,
						Valid:        true,
					},
				})
				if err != nil {
					return fmt.Errorf("failed to update gundam ownership: %w", err)
				}
				
				log.Info().
					Str("exchange_id", arg.Exchange.ID.String()).
					Int64("gundam_id", *item.GundamID).
					Str("new_owner_id", newOwnerID).
					Msg("Gundam ownership transferred")
			}
		} else {
			// 5. Nếu chưa hoàn thành, cập nhật trạng thái exchange dựa trên trạng thái đơn hàng
			lowestStatus := GetLowestOrderStatus(updatedOrder.Status, partnerOrder.Status)
			
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
				exchangeStatus = arg.Exchange.Status
			}
			
			// Cập nhật trạng thái exchange nếu cần
			if arg.Exchange.Status != exchangeStatus {
				updatedExchange, err := qTx.UpdateExchange(ctx, UpdateExchangeParams{
					ID: arg.Exchange.ID,
					Status: NullExchangeStatus{
						ExchangeStatus: exchangeStatus,
						Valid:          true,
					},
				})
				if err != nil {
					return fmt.Errorf("failed to update exchange status: %w", err)
				}
				result.Exchange = &updatedExchange
			} else {
				result.Exchange = arg.Exchange
			}
		}
		
		return nil
	})
	
	return result, err
}
