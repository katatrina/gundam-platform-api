package api

import (
	"net/http"
	
	"github.com/gin-gonic/gin"
	db "github.com/katatrina/gundam-BE/internal/db/sqlc"
	"github.com/rs/zerolog/log"
)

//	@Summary		List all auction requests for moderator
//	@Description	Get a list of all auction requests with optional status filter.
//	@Tags			moderator
//	@Produce		json
//	@Param			status	query	string				false	"Filter by auction request status"	Enums(pending,approved,rejected)
//	@Success		200		{array}	db.AuctionRequest	"List of auction requests"
//	@Router			/mod/auction-requests [get]
func (server *Server) listAuctionRequestsForModerator(c *gin.Context) {
	status := c.Query("status")
	if status != "" {
		if err := db.IsValidAuctionRequestStatus(status); err != nil {
			c.JSON(http.StatusBadRequest, errorResponse(err))
			return
		}
	}
	
	auctionRequests, err := server.dbStore.ListAuctionRequests(c.Request.Context(), db.NullAuctionRequestStatus{
		AuctionRequestStatus: db.AuctionRequestStatus(status),
		Valid:                true,
	})
	if err != nil {
		log.Error().Err(err).Msg("failed to list auction requests")
		c.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	c.JSON(http.StatusOK, auctionRequests)
}
