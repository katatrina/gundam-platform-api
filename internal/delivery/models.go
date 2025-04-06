package delivery

import (
	"time"
)

type OrderInfo struct {
	ID            string
	Code          string
	BuyerID       string
	SellerID      string
	ItemsSubtotal int64
	DeliveryFee   int64
	TotalAmount   int64
	Status        string
	PaymentMethod string
	Note          string
}

type OrderItemInfo struct {
	OrderID  string
	GundamID int64
	Price    int64
	Quantity int64
	Weight   int64
}

type AddressInfo struct {
	UserID        string
	FullName      string
	PhoneNumber   string
	ProvinceName  string
	DistrictName  string
	GhnDistrictID int64
	WardName      string
	GhnWardCode   string
	Detail        string
}

type CreateOrderRequest struct {
	Order           OrderInfo
	OrderItems      []OrderItemInfo
	SenderAddress   AddressInfo
	ReceiverAddress AddressInfo
}

type CreateOrderResponse struct {
	Code    int64  `json:"code"`
	Message string `json:"message"`
	Data    struct {
		OrderCode            string    `json:"order_code"`
		SortCode             string    `json:"sort_code"`
		TransType            string    `json:"trans_type"`
		WardEncode           string    `json:"ward_encode"`
		DistrictEncode       string    `json:"district_encode"`
		ExpectedDeliveryTime time.Time `json:"expected_delivery_time"`
		TotalFee             int64     `json:"total_fee"`
		Fee                  struct {
			MainService  int64 `json:"main_service"`
			Insurance    int64 `json:"insurance"`
			StationDo    int64 `json:"station_do"`
			StationPu    int64 `json:"station_pu"`
			Return       int64 `json:"return"`
			R2S          int64 `json:"r2s"`
			Coupon       int64 `json:"coupon"`
			CodFailedFee int64 `json:"cod_failed_fee"`
		} `json:"fee"`
	} `json:"data"`
}
