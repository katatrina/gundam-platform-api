package api

import (
	"errors"
	"fmt"
	"math"
	"net/http"
	"time"
	
	"github.com/gin-gonic/gin"
	db "github.com/katatrina/gundam-BE/internal/db/sqlc"
	"github.com/katatrina/gundam-BE/internal/util"
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
	// Validate starting_price (tối thiểu 100,000đ)
	if req.StartingPrice < 100000 {
		return fmt.Errorf("starting_price must be at least %s, provided: %s",
			util.FormatMoney(100000), util.FormatMoney(req.StartingPrice))
	}
	
	// Validate bid_increment (tối thiểu 10,000đ)
	if req.BidIncrement < 10000 {
		return fmt.Errorf("bid_increment must be at least %s, provided: %s",
			util.FormatMoney(10000), util.FormatMoney(req.BidIncrement))
	}
	
	// Lấy múi giờ Việt Nam
	vietnamLoc, err := time.LoadLocation("Asia/Ho_Chi_Minh")
	if err != nil {
		// Fallback nếu không tìm thấy timezone data
		vietnamLoc = time.FixedZone("Asia/Ho_Chi_Minh", 7*60*60)
	}
	
	// Lấy thời gian hiện tại theo múi giờ Việt Nam
	now := time.Now().In(vietnamLoc)
	
	// Tính 00:00 của ngày mai theo múi giờ Việt Nam
	tomorrow := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, vietnamLoc)
	// Thêm 1 ngày nữa - Phương án N+2
	dayAfterTomorrow := tomorrow.AddDate(0, 0, 1)
	
	// Chuyển thời gian từ request sang múi giờ Việt Nam để so sánh
	startTimeVN := req.StartTime.In(vietnamLoc)
	endTimeVN := req.EndTime.In(vietnamLoc)
	
	// Kiểm tra ràng buộc thời gian bắt đầu phải từ 00:00 ngày N+2
	if startTimeVN.Before(dayAfterTomorrow) {
		return fmt.Errorf("start_time must be from 00:00 %s or later, provided: %s",
			dayAfterTomorrow.Format("02/01/2006"),
			startTimeVN.Format("15:04 02/01/2006"),
		)
	}
	
	// Kiểm tra thời gian kết thúc sau thời gian bắt đầu
	if !endTimeVN.After(startTimeVN) {
		return fmt.Errorf("end_time must be after start_time, start_time: %s, end_time: %s",
			startTimeVN.Format("15:04 02/01/2006"),
			endTimeVN.Format("15:04 02/01/2006"),
		)
	}
	
	// Kiểm tra thời lượng đấu giá
	auctionDuration := endTimeVN.Sub(startTimeVN)
	if auctionDuration < 24*time.Hour {
		return fmt.Errorf("auction duration must be at least 24 hours, provided: %.1f hours",
			auctionDuration.Hours())
	}
	
	if auctionDuration > 14*24*time.Hour {
		return fmt.Errorf("auction duration cannot exceed 14 days, provided: %.1f days",
			auctionDuration.Hours()/24)
	}
	
	// Kiểm tra ràng buộc giá
	// Validate bid increment (3-10% giá khởi điểm, tối thiểu 10,000đ)
	minBidIncrement := int64(math.Max(10000, float64(req.StartingPrice)*0.03))
	maxBidIncrement := int64(float64(req.StartingPrice) * 0.10)
	
	if req.BidIncrement < minBidIncrement || req.BidIncrement > maxBidIncrement {
		return fmt.Errorf("bid_increment must be between %s and %s (3-10%% of starting_price), provided: %s",
			util.FormatMoney(minBidIncrement), util.FormatMoney(maxBidIncrement), util.FormatMoney(req.BidIncrement))
	}
	
	// Validate buy now price (nếu có)
	if req.BuyNowPrice != nil && *req.BuyNowPrice < int64(float64(req.StartingPrice)*1.5) {
		minBuyNowPrice := int64(float64(req.StartingPrice) * 1.5)
		return fmt.Errorf("buy_now_price must be at least 150%% of starting_price, provided: %s, minimum required: %s",
			util.FormatMoney(*req.BuyNowPrice), util.FormatMoney(minBuyNowPrice))
	}
	
	return nil
}

//	@Summary		Create a new auction request
//	@Description	Create a new auction request for a Gundam model
//	@Tags			auctions
//	@Accept			json
//	@Produce		json
//	@Security		accessToken
//	@Param			request	body		createAuctionRequestBody	true	"Auction request details"
//	@Success		201		{object}	db.AuctionRequest			"Successfully created auction request"
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
