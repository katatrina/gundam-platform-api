package api

import (
	"net/http"
	
	"github.com/gin-gonic/gin"
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
