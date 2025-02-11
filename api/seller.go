package api

import (
	"net/http"
	
	"github.com/gin-gonic/gin"
)

func (server *Server) getSeller(ctx *gin.Context) {
	sellerID := ctx.Param("id")
	
	seller, err := server.dbStore.GetSellerByID(ctx, sellerID)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	ctx.JSON(http.StatusOK, seller)
}
