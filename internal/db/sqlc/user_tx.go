package db

import (
	"context"
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

func (store *SQLStore) CreateUserAddressTx(ctx context.Context, arg CreateUserAddressTxParams) (UserAddress, error) {
	var result UserAddress
	
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
		var err error
		result, err = qTx.CreateUserAddress(ctx, CreateUserAddressParams{
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
		
		return err
	})
	
	return result, err
}

func (store *SQLStore) UpdateUserAddressTx(ctx context.Context, arg UpdateUserAddressParams) (UserAddress, error) {
	var result UserAddress
	
	err := store.ExecTx(ctx, func(qTx *Queries) error {
		addressUpdated, err := qTx.GetUserAddressForUpdate(ctx, GetUserAddressForUpdateParams{
			AddressID: arg.AddressID,
			UserID:    arg.UserID,
		})
		if err != nil {
			return err
		}
		
		// First unset the existing primary addressUpdated if the new addressUpdated is primary
		if arg.IsPrimary.Bool {
			err = qTx.UnsetPrimaryAddress(ctx, addressUpdated.UserID)
			if err != nil {
				return err
			}
		}
		
		// Second unset the existing pickup addressUpdated if the new addressUpdated is a pickup addressUpdated
		if arg.IsPickupAddress.Bool {
			err = qTx.UnsetPickupAddress(ctx, addressUpdated.UserID)
			if err != nil {
				return err
			}
		}
		
		// Finally, update the address
		addressUpdated, err = qTx.UpdateUserAddress(ctx, arg)
		if err != nil {
			return err
		}
		result = addressUpdated
		
		return nil
	})
	
	return result, err
}

func (store *SQLStore) DeleteUserAddressTx(ctx context.Context, arg DeleteUserAddressParams) error {
	err := store.ExecTx(ctx, func(qTx *Queries) error {
		// Check address exists and belongs to user
		address, err := qTx.GetUserAddressForUpdate(ctx, GetUserAddressForUpdateParams{
			AddressID: arg.AddressID,
			UserID:    arg.UserID,
		})
		if err != nil {
			return err
		}
		
		if address.IsPrimary {
			return ErrPrimaryAddressDeletion
		}
		
		if address.IsPickupAddress {
			return ErrPickupAddressDeletion
		}
		
		return qTx.DeleteUserAddress(ctx, arg)
	})
	
	return err
}

func (store *SQLStore) BecomeSellerTx(ctx context.Context, userID string) (User, error) {
	var seller User
	
	err := store.ExecTx(ctx, func(qTx *Queries) error {
		userUpdated, err := qTx.UpdateUser(ctx, UpdateUserParams{
			UserID: userID,
			Role: NullUserRole{
				UserRole: UserRoleSeller,
				Valid:    true,
			},
		})
		if err != nil {
			return err
		}
		seller = userUpdated
		
		err = qTx.CreateTrialSubscriptionForSeller(ctx, userUpdated.ID)
		if err != nil {
			return err
		}
		
		return nil
	})
	
	return seller, err
}
