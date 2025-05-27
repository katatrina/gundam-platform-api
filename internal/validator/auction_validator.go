// internal/validator/auction_validator.go
package validator

import (
	"fmt"
	"math"
	"time"
	
	"github.com/katatrina/gundam-BE/internal/util"
)

// ValidateAuctionStartingPrice validates minimum starting price
func ValidateAuctionStartingPrice(price int64) error {
	if price < 100000 {
		return fmt.Errorf("starting_price must be at least %s, provided: %s",
			util.FormatMoney(100000), util.FormatMoney(price))
	}
	return nil
}

// ValidateAuctionBidIncrement validates bid increment rules
func ValidateAuctionBidIncrement(startingPrice, bidIncrement int64) error {
	if bidIncrement < 10000 {
		return fmt.Errorf("bid_increment must be at least %s, provided: %s",
			util.FormatMoney(10000), util.FormatMoney(bidIncrement))
	}
	
	minBidIncrement := int64(math.Max(10000, float64(startingPrice)*0.03))
	maxBidIncrement := int64(float64(startingPrice) * 0.10)
	
	if bidIncrement < minBidIncrement || bidIncrement > maxBidIncrement {
		return fmt.Errorf("bid_increment must be between %s and %s (3-10%% of starting_price), provided: %s",
			util.FormatMoney(minBidIncrement), util.FormatMoney(maxBidIncrement), util.FormatMoney(bidIncrement))
	}
	
	return nil
}

// ValidateAuctionBuyNowPrice validates buy now price if provided
func ValidateAuctionBuyNowPrice(startingPrice int64, buyNowPrice *int64) error {
	if buyNowPrice != nil && *buyNowPrice < int64(float64(startingPrice)*1.5) {
		minBuyNowPrice := int64(float64(startingPrice) * 1.5)
		return fmt.Errorf("buy_now_price must be at least 150%% of starting_price, provided: %s, minimum required: %s",
			util.FormatMoney(*buyNowPrice), util.FormatMoney(minBuyNowPrice))
	}
	return nil
}

// ValidateAuctionTimesForCreate validates auction times when creating request
func ValidateAuctionTimesForCreate(startTime, endTime time.Time) error {
	vietnamLoc, _ := time.LoadLocation("Asia/Ho_Chi_Minh")
	if vietnamLoc == nil {
		vietnamLoc = time.FixedZone("Asia/Ho_Chi_Minh", 7*60*60)
	}
	
	now := time.Now().In(vietnamLoc)
	tomorrow := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, vietnamLoc)
	dayAfterTomorrow := tomorrow.AddDate(0, 0, 1)
	
	startTimeVN := startTime.In(vietnamLoc)
	endTimeVN := endTime.In(vietnamLoc)
	
	if startTimeVN.Before(dayAfterTomorrow) {
		return fmt.Errorf("start_time must be from 00:00 %s or later, provided: %s",
			dayAfterTomorrow.Format("02/01/2006"),
			startTimeVN.Format("15:04 02/01/2006"))
	}
	
	if !endTimeVN.After(startTimeVN) {
		return fmt.Errorf("end_time must be after start_time, start_time: %s, end_time: %s",
			startTimeVN.Format("15:04 02/01/2006"),
			endTimeVN.Format("15:04 02/01/2006"))
	}
	
	return ValidateAuctionDuration(startTimeVN, endTimeVN)
}

// ValidateAuctionTimesForApproval validates auction times when approving
func ValidateAuctionTimesForApproval(startTime time.Time) error {
	vietnamLoc, _ := time.LoadLocation("Asia/Ho_Chi_Minh")
	if vietnamLoc == nil {
		vietnamLoc = time.FixedZone("Asia/Ho_Chi_Minh", 7*60*60)
	}
	
	now := time.Now().In(vietnamLoc)
	startTimeVN := startTime.In(vietnamLoc)
	
	if startTimeVN.Before(now) {
		return fmt.Errorf("auction start_time is in the past, provided: %s, now: %s",
			startTimeVN.Format("15:04 02/01/2006"),
			now.Format("15:04 02/01/2006"))
	}
	
	return nil
}

// ValidateAuctionDuration validates duration constraints
func ValidateAuctionDuration(startTime, endTime time.Time) error {
	duration := endTime.Sub(startTime)
	
	if duration < 24*time.Hour {
		return fmt.Errorf("auction duration must be at least 24 hours, provided: %.1f hours",
			duration.Hours())
	}
	
	if duration > 14*24*time.Hour {
		return fmt.Errorf("auction duration cannot exceed 14 days, provided: %.1f days",
			duration.Hours()/24)
	}
	
	return nil
}
