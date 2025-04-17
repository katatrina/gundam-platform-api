package db

import (
	"time"
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
