package db

import (
	"context"
)

type CreateUserAddressTxParams struct {
	UserID              string `json:"user_id"`
	ReceiverName        string `json:"receiver_name"`
	ReceiverPhoneNumber string `json:"receiver_phone_number"`
	ProvinceName        string `json:"province_name"`
	DistrictName        string `json:"district_name"`
	WardName            string `json:"ward_name"`
	Detail              string `json:"detail"`
	IsPrimary           bool   `json:"is_primary"`
}

type CreateUserAddressTxResult struct {
	UserAddress
}

func (store *SQLStore) CreateUserAddressTx(ctx context.Context, arg CreateUserAddressTxParams) (CreateUserAddressTxResult, error) {
	var result CreateUserAddressTxResult
	
	err := store.ExecTx(ctx, func(qTx *Queries) error {
		// First, unset the existing primary address if the new address is primary
		if arg.IsPrimary {
			err := qTx.UnsetPrimaryAddress(ctx, arg.UserID)
			if err != nil {
				return err
			}
		}
		
		// Second, insert new address
		address, err := qTx.CreateUserAddress(ctx, CreateUserAddressParams{
			UserID:              arg.UserID,
			ReceiverName:        arg.ReceiverName,
			ReceiverPhoneNumber: arg.ReceiverPhoneNumber,
			ProvinceName:        arg.ProvinceName,
			DistrictName:        arg.DistrictName,
			WardName:            arg.WardName,
			Detail:              arg.Detail,
			IsPrimary:           arg.IsPrimary,
		})
		if err != nil {
			return err
		}
		result.UserAddress = address
		
		return nil
	})
	
	return result, err
}
