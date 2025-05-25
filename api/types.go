package api

import (
	"time"
)

// SubscriptionDetailsResponse represents the detailed information of a seller's subscription
type SubscriptionDetailsResponse struct {
	// ID là ID duy nhất của subscription
	ID int64 `json:"id" example:"1"`
	
	// PlanID là ID của gói đăng ký
	PlanID int64 `json:"plan_id" example:"2"`
	
	// SubscriptionName là tên của gói đăng ký
	SubscriptionName string `json:"subscription_name" example:"GÓI NÂNG CẤP"`
	
	// SubscriptionPrice là giá của gói đăng ký (VND)
	SubscriptionPrice int64 `json:"subscription_price" example:"359000"`
	
	// SellerID là ID của người bán
	SellerID string `json:"seller_id" example:"550e8400-e29b-41d4-a716-446655440000"`
	
	// Listing information
	// MaxListings là số lượt đăng bán tối đa (null nếu unlimited)
	MaxListings *int64 `json:"max_listings" example:"50"`
	
	// ListingsUsed là số lượt đăng bán đã sử dụng
	ListingsUsed int64 `json:"listings_used" example:"12"`
	
	// ListingsRemaining là số lượt đăng bán còn lại (null nếu unlimited)
	ListingsRemaining *int64 `json:"listings_remaining" example:"38"`
	
	// Auction information
	// MaxOpenAuctions là số phiên đấu giá tối đa có thể mở cùng lúc (null nếu unlimited)
	MaxOpenAuctions *int64 `json:"max_open_auctions" example:"30"`
	
	// OpenAuctionsUsed là số phiên đấu giá đang mở
	OpenAuctionsUsed int64 `json:"open_auctions_used" example:"5"`
	
	// AuctionsRemaining là số phiên đấu giá còn có thể mở (null nếu unlimited)
	AuctionsRemaining *int64 `json:"auctions_remaining" example:"25"`
	
	// Plan details
	// IsActive cho biết subscription có đang hoạt động không
	IsActive bool `json:"is_active" example:"true"`
	
	// IsUnlimited cho biết có phải gói không giới hạn không
	IsUnlimited bool `json:"is_unlimited" example:"false"`
	
	// DurationDays là thời hạn của gói (ngày), null nếu vĩnh viễn
	DurationDays *int64 `json:"duration_days" example:"90"`
	
	// Time information
	// StartDate là thời gian bắt đầu subscription
	StartDate time.Time `json:"start_date" example:"2023-12-01T10:00:00Z"`
	
	// EndDate là thời gian kết thúc subscription (null nếu vĩnh viễn)
	EndDate *time.Time `json:"end_date" example:"2024-03-01T10:00:00Z"`
	
	// DaysRemaining là số ngày còn lại của subscription (null nếu vĩnh viễn)
	DaysRemaining *int `json:"days_remaining" example:"45"`
	
	// Status indicators
	// IsExpiringSoon cho biết subscription có sắp hết hạn không (< 7 ngày)
	IsExpiringSoon bool `json:"is_expiring_soon" example:"false"`
	
	// CanUpgrade cho biết có thể nâng cấp lên gói cao hơn không
	CanUpgrade bool `json:"can_upgrade" example:"true"`
}
