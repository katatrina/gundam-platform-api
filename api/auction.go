package api

import (
	"errors"
	"fmt"
	"net/http"
	
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
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

//	@Summary		Get auction details
//	@Description	Retrieves details of a specific auction by its ID.
//	@Tags			auctions
//	@Produce		json
//	@Param			auctionID	path		string		true	"ID of the auction"
//	@Success		200			{object}	db.Auction	"Details of the auction"
//	@Router			/auctions/{auctionID} [get]
func (server *Server) getAuctionDetails(c *gin.Context) {
	auctionID, err := uuid.Parse(c.Param("auctionID"))
	if err != nil {
		err = fmt.Errorf("invalid auction ID: %w", err)
		c.JSON(http.StatusBadRequest, errorResponse(err))
		return
	}
	
	auction, err := server.dbStore.GetAuctionByID(c, auctionID)
	if err != nil {
		if errors.Is(err, db.ErrRecordNotFound) {
			err = fmt.Errorf("auction ID %s not found", auctionID)
			c.JSON(http.StatusNotFound, errorResponse(err))
			return
		}
		
		err = fmt.Errorf("failed to get auction details: %w", err)
		c.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	c.JSON(http.StatusOK, auction)
}
