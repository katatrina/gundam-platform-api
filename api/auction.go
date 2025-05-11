package api

import (
	"fmt"
	"net/http"
	
	"github.com/gin-gonic/gin"
	db "github.com/katatrina/gundam-BE/internal/db/sqlc"
)

//	@Summary		Get platform auctions
//	@Description	Retrieves upcoming and ongoing auctions from the platform.
//	@Tags			auctions
//	@Produce		json
//	@Param			status	query	string		false	"Filter by status"	Enums(scheduled, active)
//	@Success		200		{array}	db.Auction	"List of auctions"
//	@Router			/auctions [get]
func (server *Server) listAuctions(c *gin.Context) {
	status := db.AuctionStatus(c.Query("status"))
	if status != "" {
		if status != db.AuctionStatusScheduled && status != db.AuctionStatusActive {
			err := fmt.Errorf("invalid status: %s, allowed statuses: [scheduled, active]", status)
			c.JSON(http.StatusBadRequest, errorResponse(err))
			return
		}
	}
	
	auctions, err := server.dbStore.ListAuctions(c, db.NullAuctionStatus{
		AuctionStatus: status,
		Valid:         status != "",
	})
	if err != nil {
		err = fmt.Errorf("failed to list auctions: %w", err)
		c.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	c.JSON(http.StatusOK, auctions)
}
