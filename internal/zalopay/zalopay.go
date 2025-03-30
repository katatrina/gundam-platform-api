package zalopay

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
	
	"github.com/katatrina/gundam-BE/internal/util"
	"github.com/zpmep/hmacutil"
)

const (
	sandboxTestAppID = "2553"
	sandboxTestKey1  = "PcY4iZIKFCIdgZvA6ueMcMHHUbRLYjPL"
)

type ZalopayService struct {
	appID string
	key1  string
}

func NewZalopayService() *ZalopayService {
	return &ZalopayService{
		appID: sandboxTestAppID,
		key1:  sandboxTestKey1,
	}
}

type CreateOrderZaloPayResponse struct {
	ReturnCode       int    `json:"return_code"`
	ReturnMessage    string `json:"return_message"`
	SubReturnCode    int    `json:"sub_return_code"`
	SubReturnMessage string `json:"sub_return_message"`
	OrderURL         string `json:"order_url"`
	ZpTransToken     string `json:"zp_trans_token"`
	OrderToken       string `json:"order_token"`
	QrCode           string `json:"qr_code"`
}

func (z *ZalopayService) CreateOrder(appUser string, amount int64, items []map[string]interface{}, description string) (*CreateOrderZaloPayResponse, error) {
	appTransID := util.GenerateZalopayAppTransID()
	
	appTime := strconv.FormatInt(time.Now().UnixMilli(), 10)
	
	parsedItems, _ := json.Marshal(items)
	
	// Dữ liệu riêng của đơn hàng.
	// Dữ liệu này sẽ được callback lại cho AppServer khi thanh toán thành công (Nếu không có thì để chuỗi rỗng).
	embedData, _ := json.Marshal(map[string]interface{}{
		"preferred_payment_method": []string{},
	})
	
	// Kết quả hiển thị trên trang cổng thanh toán:
	// Danh sách tất cả các hình thức và ngân hàng được hỗ trợ (ATM, CC, ZaloPay, ZaloPay QR đa năng, Apple Pay ...).
	bankCode := ""
	
	// request data
	params := make(url.Values)
	params.Add("app_id", z.appID)
	params.Add("amount", strconv.Itoa(int(amount)))
	params.Add("app_user", appUser)
	params.Add("embed_data", string(embedData))
	params.Add("item", string(parsedItems))
	params.Add("description", description)
	params.Add("bank_code", bankCode)
	
	params.Add("app_time", appTime)
	
	params.Add("app_trans_id", appTransID)
	
	data := fmt.Sprintf("%v|%v|%v|%v|%v|%v|%v", params.Get("app_id"), params.Get("app_trans_id"), params.Get("app_user"),
		params.Get("amount"), params.Get("app_time"), params.Get("embed_data"), params.Get("item"))
	params.Add("mac", hmacutil.HexStringEncode(hmacutil.SHA256, z.key1, data))
	
	// Gọi API ZaloPay
	// Content-Type: application/x-www-form-urlencoded
	resp, err := http.PostForm("https://sb-openapi.zalopay.vn/v2/create", params)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	
	// Đọc body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	
	// Parse response vào struct
	var result CreateOrderZaloPayResponse
	if err = json.Unmarshal(body, &result); err != nil {
		return nil, err
	}
	
	if result.ReturnCode != 1 {
		fmt.Println(result)
		return nil, fmt.Errorf("ZaloPay error: %s", result.ReturnMessage)
	}
	
	return &result, nil
}
