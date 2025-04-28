package db

import (
	"context"
	"time"
	
	"github.com/google/uuid"
	"github.com/katatrina/gundam-BE/internal/util"
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
	Exchange             Exchange
	Note                 *string
}

type PayExchangeDeliveryFeeTxResult struct {
	Exchange        Exchange   `json:"exchange"`
	BothPartiesPaid bool       `json:"both_parties_paid"`
	PartnerHasPaid  bool       `json:"partner_has_paid"`
	PosterOrderID   *uuid.UUID `json:"poster_order_id"`
	OffererOrderID  *uuid.UUID `json:"offerer_order_id"`
}

func (store *SQLStore) PayExchangeDeliveryFeeTx(ctx context.Context, arg PayExchangeDeliveryFeeTxParams) (PayExchangeDeliveryFeeTxResult, error) {
	var result PayExchangeDeliveryFeeTxResult
	
	err := store.ExecTx(ctx, func(qTx *Queries) error {
		var err error
		
		// 1. Lưu phí vận chuyển và ghi chú (nếu có) vào bảng exchanges
		updateExchangeParams := UpdateExchangeParams{
			ID: arg.ExchangeID,
		}
		
		if arg.IsPoster {
			updateExchangeParams.PosterDeliveryFee = &arg.DeliveryFee
			updateExchangeParams.PosterOrderNote = arg.Note
			updateExchangeParams.PosterOrderExpectedDeliveryTime = &arg.ExpectedDeliveryTime
		} else {
			updateExchangeParams.OffererDeliveryFee = &arg.DeliveryFee
			updateExchangeParams.OffererOrderNote = arg.Note
			updateExchangeParams.OffererOrderExpectedDeliveryTime = &arg.ExpectedDeliveryTime
		}
		
		updatedExchange, err := qTx.UpdateExchange(ctx, updateExchangeParams)
		if err != nil {
			return err
		}
		
		// 2. Trừ tiền từ ví người dùng
		_, err = qTx.AddWalletBalance(ctx, AddWalletBalanceParams{
			Amount: -arg.DeliveryFee,
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
		
		result.Exchange = updatedExchange
		
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
			// 6.1 Lấy thông tin địa chỉ vận chuyển
			_, err := qTx.GetDeliveryInformation(ctx, *updatedExchange.PosterFromDeliveryID)
			if err != nil {
				return err
			}
			
			_, err = qTx.GetDeliveryInformation(ctx, *updatedExchange.PosterToDeliveryID)
			if err != nil {
				return err
			}
			
			_, err = qTx.GetDeliveryInformation(ctx, *updatedExchange.OffererFromDeliveryID)
			if err != nil {
				return err
			}
			
			_, err = qTx.GetDeliveryInformation(ctx, *updatedExchange.OffererToDeliveryID)
			if err != nil {
				return err
			}
			
			// 6.2 Tạo đơn hàng cho poster (từ offerer đến poster)
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
			
			_, err = qTx.CreateOrder(ctx, posterOrderParams)
			if err != nil {
				return err
			}
			
			// 6.3 Tạo đơn hàng cho offerer (từ poster đến offerer)
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
			
			_, err = qTx.CreateOrder(ctx, offererOrderParams)
			if err != nil {
				return err
			}
			
			// 6.4 Tạo thông tin giao hàng cho đơn hàng poster
			// Các cột status, overall_status, delivery_tracking_code sẽ được cập nhật sau
			// khi người bán đóng gói đơn hàng.
			_, err = qTx.CreateOrderDelivery(ctx, CreateOrderDeliveryParams{
				OrderID:              posterOrderID,
				ExpectedDeliveryTime: *updatedExchange.PosterOrderExpectedDeliveryTime,
				FromDeliveryID:       *updatedExchange.OffererFromDeliveryID,
				ToDeliveryID:         *updatedExchange.PosterToDeliveryID,
			})
			if err != nil {
				return err
			}
			
			// 6.5 Tạo thông tin giao hàng cho đơn hàng offerer
			// Các cột status, overall_status, delivery_tracking_code sẽ được cập nhật sau
			// khi người bán đóng gói đơn hàng.
			_, err = qTx.CreateOrderDelivery(ctx, CreateOrderDeliveryParams{
				OrderID:              offererOrderID,
				ExpectedDeliveryTime: *updatedExchange.OffererOrderExpectedDeliveryTime,
				FromDeliveryID:       *updatedExchange.PosterFromDeliveryID,
				ToDeliveryID:         *updatedExchange.OffererToDeliveryID,
			})
			if err != nil {
				return err
			}
			
			// 6.6 Lấy danh sách sản phẩm trong giao dịch trao đổi
			exchangeItems, err := qTx.ListExchangeItems(ctx, ListExchangeItemsParams{
				ExchangeID:   updatedExchange.ID,
				IsFromPoster: nil,
			})
			if err != nil {
				return err
			}
			
			// 6.7 Tạo các order item cho mỗi đơn hàng
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
			
			// 6.8 Cập nhật exchange với ID đơn hàng và chuyển trạng thái
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
			result.PosterOrderID = &posterOrderID
			result.OffererOrderID = &offererOrderID
		}
		
		return nil
	})
	
	return result, err
}
