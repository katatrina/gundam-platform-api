package ghn

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Định nghĩa các cấu trúc dữ liệu riêng cho package ghn
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

type CreateGHNOrderRequest struct {
	Order           OrderInfo
	OrderItems      []OrderItemInfo
	SenderAddress   AddressInfo
	ReceiverAddress AddressInfo
}

type CreateGHNOrderResponse struct {
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

func (s *GHNService) CreateOrder(ctx context.Context, arg CreateGHNOrderRequest) (*CreateGHNOrderResponse, error) {
	// API endpoint để tạo đơn hàng
	url := GHNBaseURL + "/shipping-order/create"
	
	totalWeight := int64(0)
	for _, item := range arg.OrderItems {
		totalWeight += item.Weight * item.Quantity
	}
	
	// Thông tin đơn hàng
	orderData := map[string]interface{}{
		"from_name":            arg.SenderAddress.FullName,
		"from_phone":           arg.SenderAddress.PhoneNumber,
		"from_address":         arg.SenderAddress.Detail,
		"from_ward_name":       arg.SenderAddress.WardName,
		"from_district_name":   arg.SenderAddress.DistrictName,
		"from_province_name":   arg.SenderAddress.ProvinceName,
		"to_name":              arg.ReceiverAddress.FullName,
		"to_phone":             arg.ReceiverAddress.PhoneNumber,
		"to_address":           arg.ReceiverAddress.Detail,
		"to_ward_name":         arg.ReceiverAddress.WardName,
		"to_district_name":     arg.ReceiverAddress.DistrictName,
		"to_province_name":     arg.ReceiverAddress.ProvinceName,
		"return_phone":         arg.SenderAddress.PhoneNumber,
		"return_address":       arg.SenderAddress.Detail,
		"return_district_name": arg.SenderAddress.DistrictName,
		"return_ward_name":     arg.SenderAddress.WardName,
		"return_province_name": arg.SenderAddress.ProvinceName,
		"client_order_code":    arg.Order.Code,
		"cod_amount":           int64(0), // Đã thanh toán bằng ví
		"content":              "Mô hình Gundam",
		"weight":               totalWeight,
		// Sử dụng giá trị mặc định cho toàn bộ đơn hàng
		"length":          int64(40), // cm
		"width":           int64(30),
		"height":          int64(20),
		"service_type_id": int64(2), // Chọn loại dịch vụ "Hàng nhẹ" cho đơn giản
		"payment_type_id": int64(2), // Người mua thanh toán phí dịch vụ
		"required_note":   "CHOXEMHANGKHONGTHU",
		"insurance_value": int64(0), // Không thêm phí bảo hiểm cho môi trường test
		// TODO: Thêm các thông tin khác nếu cần thiết
	}
	
	// Chuyển đổi dữ liệu thành JSON
	jsonData, err := json.Marshal(orderData)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal GHN order data: %w", err)
	}
	
	// Tạo request
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create GHN order request: %w", err)
	}
	
	// Thiết lập header
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Token", s.Token)
	req.Header.Set("ShopId", s.ShopID)
	
	// Gửi request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()
	
	// Đọc response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}
	
	// Kiểm tra mã trạng thái HTTP
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GHN API returned non-OK status: %d, body: %s", resp.StatusCode, string(body))
	}
	
	// Parse response
	var response CreateGHNOrderResponse
	if err = json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("failed to parse GHN order response: %w", err)
	}
	
	// Kiểm tra code trong response body
	if response.Code != int64(http.StatusOK) {
		return nil, fmt.Errorf("GHN API returned business error: code=%d, message=%s",
			response.Code, response.Message)
	}
	
	return &response, nil
}
