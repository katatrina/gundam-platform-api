package api

import (
	"fmt"
	"math"
	"net/http"
	"time"
	
	"github.com/gin-gonic/gin"
	"github.com/katatrina/gundam-BE/internal/util"
)

type createAuctionRequestBody struct {
	GundamID      int64     `json:"gundam_id" binding:"required"`
	StartingPrice int64     `json:"starting_price" binding:"required,min=100000"`
	BidIncrement  int64     `json:"bid_increment" binding:"required,min=10000"`
	BuyNowPrice   *int64    `json:"buy_now_price,omitempty"`
	StartTime     time.Time `json:"start_time" binding:"required"`
	EndTime       time.Time `json:"end_time" binding:"required"`
}

func (req *createAuctionRequestBody) validate() error {
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

func (server *Server) createAuctionRequest(c *gin.Context) {
	// user := c.MustGet(sellerPayloadKey).(*db.User)
	
	var req createAuctionRequestBody
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse(err))
		return
	}
	
}
