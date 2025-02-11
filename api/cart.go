package api

import (
	"net/http"
	
	"github.com/gin-gonic/gin"
	db "github.com/katatrina/gundam-BE/internal/db/sqlc"
	"github.com/katatrina/gundam-BE/internal/token"
	"github.com/rs/zerolog/log"
)

type addCartItemRequest struct {
	GundamID int64 `json:"gundam_id"`
}

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
		ctx.JSON(http.StatusInternalServerError, errorResponse(ErrInternalServer))
		return
	}
	
	arg := db.AddCartItemParams{
		CartID:   cartID,
		GundamID: req.GundamID,
	}
	
	cartItem, err := server.dbStore.AddCartItem(ctx, arg)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	ctx.JSON(http.StatusOK, cartItem)
}

func (server *Server) listCartItems(ctx *gin.Context) {
	userID := ctx.MustGet(authorizationPayloadKey).(*token.Payload).Subject
	
	cartID, err := server.dbStore.GetOrCreateCartIfNotExists(ctx, userID)
	if err != nil {
		log.Err(err).Msg("failed to get or create cart")
		ctx.JSON(http.StatusInternalServerError, errorResponse(ErrInternalServer))
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
