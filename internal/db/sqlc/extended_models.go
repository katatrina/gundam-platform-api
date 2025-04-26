package db

import (
	"time"
	
	"github.com/google/uuid"
)

type GundamDetails struct {
	ID                   int64                `json:"gundam_id"`
	OwnerID              string               `json:"owner_id"`
	Name                 string               `json:"name"`
	Slug                 string               `json:"slug"`
	Grade                string               `json:"grade"`
	Series               string               `json:"series"`
	PartsTotal           int64                `json:"parts_total"`
	Material             string               `json:"material"`
	Version              string               `json:"version"`
	Quantity             int64                `json:"quantity"`
	Condition            string               `json:"condition"`
	ConditionDescription *string              `json:"condition_description"`
	Manufacturer         string               `json:"manufacturer"`
	Weight               int64                `json:"weight"`
	Scale                string               `json:"scale"`
	Description          string               `json:"description"`
	Price                int64                `json:"price"`
	ReleaseYear          *int64               `json:"release_year"`
	Status               string               `json:"status"`
	Accessories          []GundamAccessoryDTO `json:"accessories"`
	PrimaryImageURL      string               `json:"primary_image_url"`
	SecondaryImageURLs   []string             `json:"secondary_image_urls"`
	CreatedAt            time.Time            `json:"created_at"`
	UpdatedAt            time.Time            `json:"updated_at"`
}

type GundamAccessoryDTO struct {
	Name     string `json:"name"`
	Quantity int64  `json:"quantity"`
}

func ConvertGundamAccessoryToDTO(accessory GundamAccessory) GundamAccessoryDTO {
	return GundamAccessoryDTO{
		Name:     accessory.Name,
		Quantity: accessory.Quantity,
	}
}

type MemberOrderInfo struct {
	Order      Order       `json:"order"`
	OrderItems []OrderItem `json:"order_items"`
}

type MemberOrderDetails struct {
	SellerInfo            *SellerInfo         `json:"seller_info"`             // Thông tin người gửi (null nếu là người gửi)
	BuyerInfo             *User               `json:"buyer_info"`              // Thông tin người nhận (null nếu là người nhận)
	Order                 Order               `json:"order"`                   // Thông tin đơn hàng
	OrderItems            []OrderItem         `json:"order_items"`             // Danh sách sản phẩm trong đơn hàng
	OrderDelivery         OrderDelivery       `json:"order_delivery"`          // Thông tin vận chuyển
	OrderTransaction      OrderTransaction    `json:"order_transaction"`       // Thông tin giao dịch thanh toán của đơn hàng
	ToDeliveryInformation DeliveryInformation `json:"to_delivery_information"` // Địa chỉ nhận hàng của người mua
	// FromDeliveryInformation *DeliveryInformation `json:"from_delivery_information"` // Địa chỉ gửi hàng của người bán
}

type SalesOrderInfo struct {
	Order      Order       `json:"order"`
	OrderItems []OrderItem `json:"order_items"`
}

type SalesOrderDetails struct {
	BuyerInfo             User                `json:"buyer_info"`              // Thông tin người mua
	Order                 Order               `json:"order"`                   // Thông tin đơn hàng
	OrderItems            []OrderItem         `json:"order_items"`             // Danh sách sản phẩm trong đơn hàng
	OrderDelivery         OrderDelivery       `json:"order_delivery"`          // Thông tin vận chuyển
	ToDeliveryInformation DeliveryInformation `json:"to_delivery_information"` // Địa chỉ nhận hàng của người mua
	OrderTransaction      OrderTransaction    `json:"order_transaction"`       // Thông tin giao dịch thanh toán của đơn hàng
}

type SellerInfo struct {
	ID              string  `json:"id"`
	GoogleAccountID *string `json:"google_account_id"`
	UserFullName    string  `json:"user_full_name"`
	ShopName        string  `json:"shop_name"`
	Email           string  `json:"email"`
	PhoneNumber     *string `json:"phone_number"`
	Role            string  `json:"role"`
	AvatarURL       *string `json:"avatar_url"`
}

type OpenExchangePostInfo struct {
	ExchangePost      ExchangePost    `json:"exchange_post"`       // Thông tin bài đăng
	ExchangePostItems []GundamDetails `json:"exchange_post_items"` // Danh sách Gundam mà Người đăng bài cho phép trao đổi
	Poster            User            `json:"poster"`              // Thông tin Người đăng bài
	OfferCount        int64           `json:"offer_count"`         // Số lượng offer của bài đăng
	// AuthenticatedUserOffer       *ExchangeOffer  `json:"authenticated_user_offer"`        // Offer của người dùng đã đăng nhập (nếu có)
	// AuthenticatedUserOfferItems  []GundamDetails `json:"authenticated_user_offer_items"`  // Danh sách Gundam trong offer của người dùng đã đăng nhập (nếu có)
	// AuthenticatedUserWantedItems []GundamDetails `json:"authenticated_user_wanted_items"` // Danh sách Gundam mà người dùng đã đăng nhập muốn nhận (nếu có)
}

type UserExchangePostDetails struct {
	ExchangePost      ExchangePost        `json:"exchange_post"`       // Thông tin bài đăng
	ExchangePostItems []GundamDetails     `json:"exchange_post_items"` // Danh sách Gundam mà Người đăng bài cho phép trao đổi
	OfferCount        int64               `json:"offer_count"`         // Số lượng offer của bài đăng
	Offers            []ExchangeOfferInfo `json:"offers"`              // Danh sách các offer của bài đăng
}

type ExchangeOfferInfo struct {
	ID      uuid.UUID `json:"id"`      // ID đề xuất
	PostID  uuid.UUID `json:"post_id"` // ID bài đăng trao đổi
	Offerer User      `json:"offerer"` // Thông tin người đề xuất
	
	PayerID            *string `json:"payer_id"`            // ID người bù tiền
	CompensationAmount *int64  `json:"compensation_amount"` // Số tiền bù
	Note               *string `json:"note"`                // Ghi chú của đề xuất
	
	OffererExchangeItems []GundamDetails `json:"offerer_exchange_items"` // Danh sách Gundam của người đề xuất
	PosterExchangeItems  []GundamDetails `json:"poster_exchange_items"`  // Danh sách Gundam của người đăng bài mà người đề xuất muốn trao đổi
	
	NegotiationsCount    int64               `json:"negotiations_count"`    // Số lần đã thương lượng
	MaxNegotiations      int64               `json:"max_negotiations"`      // Số lần thương lượng tối đa
	NegotiationRequested bool                `json:"negotiation_requested"` // Đã yêu cầu thương lượng chưa
	LastNegotiationAt    *time.Time          `json:"last_negotiation_at"`   // Thời gian thương lượng gần nhất
	NegotiationNotes     []ExchangeOfferNote `json:"negotiation_notes"`     // Các ghi chú/tin nhắn thương lượng
	
	CreatedAt time.Time `json:"created_at"` // Thời gian tạo đề xuất
	UpdatedAt time.Time `json:"updated_at"` // Thời gian cập nhật đề xuất gần nhất
}

type UserExchangeOfferDetails struct {
	ExchangePost      ExchangePost      `json:"exchange_post"`       // Thông tin bài đăng
	Poster            User              `json:"poster"`              // Thông tin Người đăng bài
	ExchangePostItems []GundamDetails   `json:"exchange_post_items"` // Danh sách Gundam mà Người đăng bài cho phép trao đổi
	Offer             ExchangeOfferInfo `json:"offer"`               // Chi tiết đề xuất
}
