package db

import (
	"context"
	
	"github.com/google/uuid"
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
