package db

import (
	"context"
	
	"github.com/jackc/pgx/v5/pgtype"
)

type CreateUserAddressTxParams struct {
	UserID          string
	FullName        string
	PhoneNumber     string
	ProvinceName    string
	DistrictName    string
	GHNDistrictID   int64
	WardName        string
	GHNWardCode     string
	Detail          string
	IsPrimary       bool
	IsPickupAddress bool
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
		
		// Then, unset the existing pickup address if the new address is a pickup address
		if arg.IsPickupAddress {
			err := qTx.UnsetPickupAddress(ctx, arg.UserID)
			if err != nil {
				return err
			}
		}
		
		// Finally, create the new address
		address, err := qTx.CreateUserAddress(ctx, CreateUserAddressParams{
			UserID:          arg.UserID,
			FullName:        arg.FullName,
			PhoneNumber:     arg.PhoneNumber,
			ProvinceName:    arg.ProvinceName,
			DistrictName:    arg.DistrictName,
			GhnDistrictID:   arg.GHNDistrictID,
			WardName:        arg.WardName,
			GhnWardCode:     arg.GHNWardCode,
			Detail:          arg.Detail,
			IsPrimary:       arg.IsPrimary,
			IsPickupAddress: arg.IsPickupAddress,
		})
		if err != nil {
			return err
		}
		result.UserAddress = address
		
		return nil
	})
	
	return result, err
}

type UpdateUserAddressTxParams struct {
	UserID    string
	AddressID int64
	IsPrimary pgtype.Bool
}

func (store *SQLStore) UpdateUserAddressTx(ctx context.Context, arg UpdateUserAddressTxParams) error {
	err := store.ExecTx(ctx, func(qTx *Queries) error {
		// First, unset the existing primary address if the new address is primary
		if arg.IsPrimary.Bool {
			err := qTx.UnsetPrimaryAddress(ctx, arg.UserID)
			if err != nil {
				return err
			}
		}
		
		err := store.UpdateUserAddress(ctx, UpdateUserAddressParams{
			IsPrimary: arg.IsPrimary,
			AddressID: arg.AddressID,
		})
		
		return err
	})
	
	return err
}
