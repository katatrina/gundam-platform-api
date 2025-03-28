package api

import (
	"net/http"
	
	"github.com/gin-gonic/gin"
	db "github.com/katatrina/gundam-BE/internal/db/sqlc"
	"github.com/katatrina/gundam-BE/internal/token"
	"github.com/rs/zerolog/log"
)

type addCartItemRequest struct {
	GundamID int64 `json:"gundam_id" binding:"required"`
}

//	@Summary		Add Item to Cart
//	@Description	Adds a Gundam model to the user's shopping cart
//	@Tags			cart
//	@Accept			json
//	@Produce		json
//	@Security		accessToken
//	@Param			request	body		addCartItemRequest	true	"Gundam to add to cart"
//	@Success		200		{object}	db.AddCartItemRow	"Successfully added item to cart"
//	@Failure		400		"Bad Request - Invalid input"
//	@Failure		401		"Unauthorized - Authentication required"
//	@Failure		500		"Internal Server Error - Failed to add cart item"
//	@Router			/cart/items [post]
func (server *Server) addCartItem(ctx *gin.Context) {
	req := new(addCartItemRequest)
	
	if err := ctx.ShouldBindJSON(req); err != nil {
		log.Error().Err(err).Msg("failed to bind JSON")
		ctx.JSON(http.StatusBadRequest, errorResponse(err))
		return
	}
	
	userID := ctx.MustGet(authorizationPayloadKey).(*token.Payload).Subject
	
	cartID, err := server.dbStore.GetOrCreateCartIfNotExists(ctx, userID)
	if err != nil {
		log.Err(err).Msg("failed to get or create cart")
		ctx.Status(http.StatusInternalServerError)
		return
	}
	
	// Check if Gundam exists
	gundam, err := server.dbStore.GetGundamByID(ctx, req.GundamID)
	if err != nil {
		log.Err(err).Msg("failed to get Gundam by ID")
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	if gundam.Status != db.GundamStatusPublished || gundam.DeletedAt.Valid == true {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "this gundam has been deleted or is not being published"})
		return
	}
	
	arg := db.AddCartItemParams{
		CartID:   cartID,
		GundamID: req.GundamID,
	}
	
	// Check if Gundam is already in the cart
	exists, err := server.dbStore.CheckCartItemExists(ctx, db.CheckCartItemExistsParams{
		CartID:   cartID,
		GundamID: req.GundamID,
	})
	if err != nil {
		log.Err(err).Msg("failed to check if Gundam is already in the cart")
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	if exists {
		ctx.JSON(http.StatusBadRequest, errorResponse(db.ErrCartItemExists))
		return
	}
	
	cartItem, err := server.dbStore.AddCartItem(ctx, arg)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	ctx.JSON(http.StatusOK, cartItem)
}

//	@Summary		List Cart Items
//	@Description	Retrieves all items in the user's shopping cart with detailed information
//	@Tags			cart
//	@Produce		json
//	@Security		accessToken
//	@Success		200	{array}	db.ListCartItemsWithDetailsRow	"Successfully retrieved cart items"
//	@Failure		500	"Internal Server Error - Failed to retrieve cart items"
//	@Router			/cart/items [get]
func (server *Server) listCartItems(ctx *gin.Context) {
	userID := ctx.MustGet(authorizationPayloadKey).(*token.Payload).Subject
	
	cartID, err := server.dbStore.GetOrCreateCartIfNotExists(ctx, userID)
	if err != nil {
		log.Err(err).Msg("failed to get or create cart")
		ctx.Status(http.StatusInternalServerError)
		return
	}
	
	items, err := server.dbStore.ListCartItemsWithDetails(ctx, cartID)
	if err != nil {
		log.Error().Err(err).Msg("failed to list cart items")
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	ctx.JSON(http.StatusOK, items)
}

//	@Summary		Delete Cart Item
//	@Description	Removes a specific item from the user's shopping cart
//	@Tags			cart
//	@Security		accessToken
//	@Param			id	path	string	true	"Cart Item ID to delete"	example(1)
//	@Success		204	"Successfully deleted cart item"
//	@Failure		400	"Bad Request - Invalid cart item ID"
//	@Failure		500	"Internal Server Error - Failed to delete cart item"
//	@Router			/cart/items/{id} [delete]
func (server *Server) deleteCartItem(ctx *gin.Context) {
	cartItemID := ctx.Param("id")
	
	userID := ctx.MustGet(authorizationPayloadKey).(*token.Payload).Subject
	
	cartID, err := server.dbStore.GetCartByUserID(ctx, userID)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	err = server.dbStore.RemoveCartItem(ctx, db.RemoveCartItemParams{
		ID:     cartItemID,
		CartID: cartID,
	})
	if err != nil {
		log.Error().Err(err).Msg("failed to remove cart item")
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	ctx.Status(http.StatusNoContent)
}
