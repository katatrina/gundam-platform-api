package api

import (
	"context"
	"errors"
	"net/http"
	
	"github.com/gin-gonic/gin"
	db "github.com/katatrina/gundam-BE/internal/db/sqlc"
	"github.com/rs/zerolog/log"
)

// CreateUserAddressRequest represents the input for creating a user address
type createUserAddressRequest struct {
	FullName        string `json:"full_name" binding:"required"`
	PhoneNumber     string `json:"phone_number" binding:"required"`
	ProvinceName    string `json:"province_name" binding:"required"`
	DistrictName    string `json:"district_name" binding:"required"`
	GHNDistrictID   int64  `json:"ghn_district_id" binding:"required"`
	WardName        string `json:"ward_name" binding:"required"`
	GHNWardCode     string `json:"ghn_ward_code" binding:"required"`
	Detail          string `json:"detail" binding:"required"`
	IsPrimary       bool   `json:"is_primary"`
	IsPickupAddress bool   `json:"is_pickup_address"`
}

//	@Summary		Create a new user address
//	@Description	Add a new address for a specific user
//	@Tags			users
//	@Accept			json
//	@Produce		json
//	@Param			id		path		string						true	"User ID"
//	@Param			request	body		createUserAddressRequest	false	"Address creation request"
//	@Success		201		{object}	db.UserAddress				"Address created successfully"
//	@Failure		400		"Invalid request body"
//	@Failure		404		"User not found"
//	@Failure		500		"Internal server error"
//	@Router			/users/{id}/addresses [post]
func (server *Server) createUserAddress(ctx *gin.Context) {
	userID := ctx.Param("id")
	
	req := new(createUserAddressRequest)
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.Status(http.StatusBadRequest)
		return
	}
	
	// Check if the user exists
	_, err := server.dbStore.GetUserByID(context.Background(), userID)
	if err != nil {
		if errors.Is(err, db.ErrRecordNotFound) {
			ctx.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
			return
		}
		
		log.Err(err).Msg("failed to get user")
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	arg := db.CreateUserAddressTxParams{
		UserID:          userID,
		FullName:        req.FullName,
		PhoneNumber:     req.PhoneNumber,
		ProvinceName:    req.ProvinceName,
		DistrictName:    req.DistrictName,
		GHNDistrictID:   req.GHNDistrictID,
		WardName:        req.WardName,
		GHNWardCode:     req.GHNWardCode,
		Detail:          req.Detail,
		IsPrimary:       req.IsPrimary,
		IsPickupAddress: req.IsPickupAddress,
	}
	
	result, err := server.dbStore.CreateUserAddressTx(context.Background(), arg)
	if err != nil {
		log.Err(err).Msg("failed to create address")
		ctx.Status(http.StatusInternalServerError)
		return
	}
	
	ctx.JSON(http.StatusCreated, result)
}

//	@Summary		Retrieve user addresses
//	@Description	Get all addresses for a specific user
//	@Tags			users
//	@Produce		json
//	@Param			id	path	string			true	"User ID"
//	@Success		200	{array}	db.UserAddress	"Successfully retrieved user addresses"
//	@Failure		500	"Internal server error"
//	@Router			/users/{id}/addresses [get]
func (server *Server) listUserAddresses(ctx *gin.Context) {
	userID := ctx.Param("id")
	
	addresses, err := server.dbStore.ListUserAddresses(context.Background(), userID)
	if err != nil {
		log.Err(err).Msg("failed to get user addresses")
		ctx.Status(http.StatusInternalServerError)
		return
	}
	
	ctx.JSON(http.StatusOK, addresses)
}

type updateUserAddressRequest struct {
	FullName        *string `json:"full_name"`
	PhoneNumber     *string `json:"phone_number"`
	ProvinceName    *string `json:"province_name"`
	DistrictName    *string `json:"district_name"`
	GHNDistrictID   *int64  `json:"ghn_district_id"`
	WardName        *string `json:"ward_name"`
	GHNWardCode     *string `json:"ghn_ward_code"`
	Detail          *string `json:"detail"`
	IsPrimary       *bool   `json:"is_primary"`
	IsPickupAddress *bool   `json:"is_pickup_address"`
}

type updateUserAddressPathParams struct {
	UserID    string `uri:"id" binding:"required"`
	AddressID int64  `uri:"address_id" binding:"required"`
}

func (req *updateUserAddressRequest) validate() (arg db.UpdateUserAddressParams) {
	if req.FullName != nil {
		arg.FullName = req.FullName
	}
	if req.PhoneNumber != nil {
		arg.PhoneNumber = req.PhoneNumber
	}
	if req.ProvinceName != nil {
		arg.ProvinceName = req.ProvinceName
	}
	if req.DistrictName != nil {
		arg.DistrictName = req.DistrictName
	}
	if req.GHNDistrictID != nil {
		arg.GhnDistrictID = req.GHNDistrictID
	}
	if req.WardName != nil {
		arg.WardName = req.WardName
	}
	if req.GHNWardCode != nil {
		arg.GhnWardCode = req.GHNWardCode
	}
	if req.Detail != nil {
		arg.Detail = req.Detail
	}
	if req.IsPrimary != nil {
		arg.IsPrimary = req.IsPrimary
	}
	if req.IsPickupAddress != nil {
		arg.IsPickupAddress = req.IsPickupAddress
	}
	
	return arg
}

//	@Summary		Update user address
//	@Description	Update an existing address information for a specific user
//	@Tags			users
//	@Accept			json
//	@Produce		json
//	@Param			id			path		string						true	"User ID"
//	@Param			address_id	path		integer						true	"Address ID"
//	@Param			request		body		updateUserAddressRequest	true	"Address information to update"
//	@Success		200			{object}	db.UserAddress				"Address updated successfully"
//	@Router			/users/{id}/addresses/{address_id} [put]
func (server *Server) updateUserAddress(ctx *gin.Context) {
	params := new(updateUserAddressPathParams)
	if err := ctx.ShouldBindUri(params); err != nil {
		log.Err(err).Msg("failed to bind uri")
		ctx.JSON(http.StatusBadRequest, errorResponse(err))
		return
	}
	
	req := new(updateUserAddressRequest)
	if err := ctx.ShouldBindJSON(req); err != nil {
		log.Err(err).Msg("failed to bind json")
		ctx.Status(http.StatusBadRequest)
		return
	}
	
	arg := req.validate()
	arg.UserID = params.UserID
	arg.AddressID = params.AddressID
	
	addressUpdated, err := server.dbStore.UpdateUserAddressTx(context.Background(), arg)
	if err != nil {
		if errors.Is(err, db.ErrRecordNotFound) {
			ctx.JSON(http.StatusNotFound, gin.H{"error": "address not found"})
			return
		}
		
		log.Err(err).Msg("failed to update address")
		ctx.Status(http.StatusInternalServerError)
		return
	}
	
	ctx.JSON(http.StatusOK, addressUpdated)
}

type deleteUserAddressPathParams struct {
	UserID    string `uri:"id" binding:"required"`
	AddressID int64  `uri:"address_id" binding:"required"`
}

//	@Summary		Delete user address
//	@Description	Delete an address of a user
//	@Tags			users
//	@Param			id			path	string	true	"User ID"
//	@Param			address_id	path	integer	true	"Address ID"
//	@Success		204			"Address deleted successfully"
//	@Failure		400			"Invalid request"
//	@Failure		404			"Address not found"
//	@Failure		409			"Cannot delete primary or pickup address"
//	@Failure		500			"Internal server error"
//	@Router			/users/{id}/addresses/{address_id} [delete]
func (server *Server) deleteUserAddress(ctx *gin.Context) {
	params := new(deleteUserAddressPathParams)
	
	if err := ctx.ShouldBindUri(params); err != nil {
		log.Err(err).Msg("failed to bind uri")
		ctx.Status(http.StatusBadRequest)
		return
	}
	
	err := server.dbStore.DeleteUserAddressTx(ctx, db.DeleteUserAddressParams{
		AddressID: params.AddressID,
		UserID:    params.UserID,
	})
	if err != nil {
		switch {
		case errors.Is(err, db.ErrRecordNotFound):
			ctx.JSON(http.StatusNotFound, gin.H{"error": "address not found"})
		case errors.Is(err, db.ErrPrimaryAddressDeletion):
			ctx.JSON(http.StatusConflict, gin.H{"error": "primary address cannot be deleted"})
		case errors.Is(err, db.ErrPickupAddressDeletion):
			ctx.JSON(http.StatusConflict, gin.H{"error": "pickup address cannot be deleted"})
		default:
			log.Err(err).Msg("failed to delete address")
			ctx.Status(http.StatusInternalServerError)
		}
		
		return
	}
	
	ctx.Status(http.StatusNoContent)
}

//	@Summary		Get user pickup address
//	@Description	Get the pickup address of a specific user
//	@Tags			users
//	@Produce		json
//	@Param			id	path		string			true	"User ID"
//	@Success		200	{object}	db.UserAddress	"Successfully retrieved user pickup address"
//	@Failure		404	"Pickup address not found"
//	@Failure		500	"Internal server error"
//	@Router			/users/{id}/addresses/pickup [get]
func (server *Server) getUserPickupAddress(ctx *gin.Context) {
	userID := ctx.Param("id")
	
	address, err := server.dbStore.GetUserPickupAddress(context.Background(), userID)
	if err != nil {
		if errors.Is(err, db.ErrRecordNotFound) {
			ctx.JSON(http.StatusNotFound, gin.H{"error": "pickup address not found"})
			return
		}
		
		log.Err(err).Msg("failed to get user pickup address")
		ctx.Status(http.StatusInternalServerError)
		return
	}
	
	ctx.JSON(http.StatusOK, address)
}
