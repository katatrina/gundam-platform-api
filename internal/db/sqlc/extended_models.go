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

type PurchaseOrderInfo struct {
	Order      Order       `json:"order"`
	OrderItems []OrderItem `json:"order_items"`
}

type PurchaseOrderDetails struct {
	SellerInfo            SellerInfo          `json:"seller_info"`
	Order                 Order               `json:"order"`
	OrderItems            []OrderItem         `json:"order_items"`
	OrderDelivery         OrderDelivery       `json:"order_delivery"`
	ToDeliveryInformation DeliveryInformation `json:"to_delivery_information"` // Thông tin nhận hàng
	OrderTransaction      OrderTransaction    `json:"order_transaction"`
}

type SalesOrderInfo struct {
	Order      Order       `json:"order"`
	OrderItems []OrderItem `json:"order_items"`
}

type SalesOrderDetails struct {
	BuyerInfo             User                `json:"buyer_info"`
	Order                 Order               `json:"order"`
	OrderItems            []OrderItem         `json:"order_items"`
	OrderDelivery         OrderDelivery       `json:"order_delivery"`
	ToDeliveryInformation DeliveryInformation `json:"to_delivery_information"` // Thông tin nhận hàng
	OrderTransaction      OrderTransaction    `json:"order_transaction"`
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
