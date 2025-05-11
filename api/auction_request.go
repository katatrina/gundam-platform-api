package api

import (
	"errors"
	"fmt"
	"net/http"
	"time"
	
	"github.com/gin-gonic/gin"
	db "github.com/katatrina/gundam-BE/internal/db/sqlc"
	"github.com/katatrina/gundam-BE/internal/validator"
)

type createAuctionRequestBody struct {
	GundamID      int64     `json:"gundam_id" binding:"required"`
	StartingPrice int64     `json:"starting_price" binding:"required"`
	BidIncrement  int64     `json:"bid_increment" binding:"required"`
	BuyNowPrice   *int64    `json:"buy_now_price,omitempty"`
	StartTime     time.Time `json:"start_time" binding:"required"`
	EndTime       time.Time `json:"end_time" binding:"required"`
}

func (req *createAuctionRequestBody) validate() error {
	if err := validator.ValidateAuctionStartingPrice(req.StartingPrice); err != nil {
		return err
	}
	
	if err := validator.ValidateAuctionBidIncrement(req.StartingPrice, req.BidIncrement); err != nil {
		return err
	}
	
	if err := validator.ValidateAuctionBuyNowPrice(req.StartingPrice, req.BuyNowPrice); err != nil {
		return err
	}
	
	if err := validator.ValidateAuctionTimesForCreate(req.StartTime, req.EndTime); err != nil {
		return err
	}
	
	return nil
}

//	@Summary		Create a new auction request by seller
//	@Description	Create a new auction request for a Gundam model
//	@Tags			auctions
//	@Accept			json
//	@Produce		json
//	@Security		accessToken
//	@Param			sellerID	path		string						true	"Seller ID"
//	@Param			request		body		createAuctionRequestBody	true	"Auction request details"
//	@Success		201			{object}	db.AuctionRequest			"Successfully created auction request"
//	@Router			/sellers/:sellerID/auction-requests [post]
func (server *Server) createAuctionRequest(c *gin.Context) {
	user := c.MustGet(sellerPayloadKey).(*db.User)
	
	var req createAuctionRequestBody
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse(err))
		return
	}
	
	// Validate business logic
	if err := req.validate(); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse(err))
		return
	}
	
	gundam, err := server.dbStore.GetGundamByID(c, req.GundamID)
	if err != nil {
		if errors.Is(err, db.ErrRecordNotFound) {
			err = fmt.Errorf("gundam ID %d not found", req.GundamID)
			c.JSON(http.StatusNotFound, errorResponse(err))
			return
		}
		
		c.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	// Check if gundam exists and belongs to the user
	if gundam.OwnerID != user.ID {
		err = fmt.Errorf("gundam ID %d does not belong to user ID %s", req.GundamID, user.ID)
		c.JSON(http.StatusForbidden, errorResponse(err))
		return
	}
	
	// Check gundam status
	if gundam.Status != db.GundamStatusInstore {
		err = fmt.Errorf("gundam ID %d is not in store, current status: %s", req.GundamID, gundam.Status)
		c.JSON(http.StatusUnprocessableEntity, errorResponse(err))
		return
	}
	
	// Check if gundam is already in the auction request
	pendingCount, err := server.dbStore.CountExistingPendingAuctionRequest(c, &gundam.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	if pendingCount > 0 {
		err = fmt.Errorf("gundam ID %d already has a pending auction request", gundam.ID)
		c.JSON(http.StatusConflict, errorResponse(err))
		return
	}
	
	// Check subscription and open auctions limit
	subscription, err := server.dbStore.GetCurrentActiveSubscriptionDetailsForSeller(c.Request.Context(), user.ID)
	if err != nil {
		if errors.Is(err, db.ErrRecordNotFound) {
			err = fmt.Errorf("user ID %s does not have an active subscription", user.ID)
			c.JSON(http.StatusForbidden, errorResponse(err))
			return
		}
		
		c.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	if !subscription.IsUnlimited {
		if subscription.MaxOpenAuctions != nil {
			if subscription.OpenAuctionsUsed >= *subscription.MaxOpenAuctions {
				err = fmt.Errorf("max open auction limit reached, used: %d, max: %d", subscription.OpenAuctionsUsed, *subscription.MaxOpenAuctions)
				c.JSON(http.StatusForbidden, errorResponse(err))
				return
			}
		} else {
			err = errors.New("max_open_auctions is nil when subscription is not unlimited")
			c.JSON(http.StatusInternalServerError, errorResponse(err))
			return
		}
	}
	
	// Lấy thông tin grade của Gundam
	grade, err := server.dbStore.GetGradeByID(c, gundam.GradeID)
	if err != nil {
		if errors.Is(err, db.ErrRecordNotFound) {
			err = fmt.Errorf("gundam ID %d has an invalid grade ID %d", req.GundamID, gundam.GradeID)
			c.JSON(http.StatusNotFound, errorResponse(err))
			return
		}
		
		c.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	// Lấy ảnh chính của Gundam
	primaryImageURL, err := server.dbStore.GetGundamPrimaryImageURL(c, gundam.ID)
	if err != nil {
		if errors.Is(err, db.ErrRecordNotFound) {
			err = fmt.Errorf("gundam ID %d does not have a primary image", gundam.ID)
			c.JSON(http.StatusNotFound, errorResponse(err))
			return
		}
		
		c.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	// Tạo snapshot
	snapshot := db.GundamSnapshot{
		ID:       gundam.ID,
		Name:     gundam.Name,
		Slug:     gundam.Slug,
		Grade:    grade.DisplayName,
		Scale:    string(gundam.Scale),
		Quantity: gundam.Quantity,
		Weight:   gundam.Weight,
		ImageURL: primaryImageURL,
	}
	
	// Thực thi transaction
	auctionRequest, err := server.dbStore.CreateAuctionRequestTx(c.Request.Context(), db.CreateAuctionRequestTxParams{
		Gundam:         gundam,
		GundamSnapshot: snapshot,
		StartingPrice:  req.StartingPrice,
		BidIncrement:   req.BidIncrement,
		BuyNowPrice:    req.BuyNowPrice,
		StartTime:      req.StartTime,
		EndTime:        req.EndTime,
		Subscription:   subscription,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	// TODO: Gửi thông báo cho tất cả morderator về yêu cầu đấu giá
	
	// Trả về thông tin yêu cầu đấu giá
	c.JSON(http.StatusCreated, auctionRequest)
}
