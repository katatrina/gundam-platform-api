package api

import (
	"net/http"
	
	"github.com/gin-gonic/gin"
	"github.com/katatrina/gundam-BE/internal/token"
	"github.com/rs/zerolog/log"
)

//	@Summary		Retrieve a seller by ID
//	@Description	Get detailed information about a specific seller
//	@Tags			sellers
//	@Produce		json
//	@Param			id	path		string	true	"Seller ID"
//	@Success		200	{object}	db.User	"Successfully retrieved seller"
//	@Failure		500	"Internal server error"
//	@Router			/sellers/{id} [get]
func (server *Server) getSeller(ctx *gin.Context) {
	sellerID := ctx.Param("id")
	
	seller, err := server.dbStore.GetSellerByID(ctx, sellerID)
	if err != nil {
		log.Error().Err(err).Msg("Failed to get seller")
		ctx.Status(http.StatusInternalServerError)
		return
	}
	
	ctx.JSON(http.StatusOK, seller)
}

//	@Summary		List all gundams for a specific seller
//	@Description	Get all gundams that belong to the specified seller ID
//	@Tags			sellers
//	@Accept			json
//	@Produce		json
//	@Param			id	path	string	true	"Seller ID"
//	@Security		accessToken
//	@Success		200	{array}		db.Gundam
//	@Failure		403	{object}	gin.H
//	@Failure		500	{object}	nil
//	@Router			/users/:id/gundams [get]
func (server *Server) listGundamsBySeller(ctx *gin.Context) {
	sellerID := ctx.Param("id")
	userID := ctx.MustGet(authorizationPayloadKey).(*token.Payload).Subject
	if sellerID != userID {
		ctx.JSON(http.StatusForbidden, gin.H{"error": "seller can only view their own gundams"})
		return
	}
	
	gundams, err := server.dbStore.ListGundamsBySellerID(ctx, sellerID)
	if err != nil {
		log.Error().Err(err).Msg("Failed to list gundams by seller")
		ctx.Status(http.StatusInternalServerError)
		return
	}
	
	ctx.JSON(http.StatusOK, gundams)
}
