package zalopay

type CreateOrderZalopayResponse struct {
	ReturnCode       int    `json:"return_code"`
	ReturnMessage    string `json:"return_message"`
	SubReturnCode    int    `json:"sub_return_code"`
	SubReturnMessage string `json:"sub_return_message"`
	OrderURL         string `json:"order_url"`
	ZpTransToken     string `json:"zp_trans_token"`
	OrderToken       string `json:"order_token"`
	QrCode           string `json:"qr_code"`
}

type ZaloPayCallbackData struct {
	Data string `json:"data"`
	Mac  string `json:"mac"`
	Type int    `json:"type"`
}

type ZalopayCallbackResult struct {
	ReturnCode    int    `json:"return_code"`
	ReturnMessage string `json:"return_message"`
}

type TransactionData struct {
	AppID          int    `json:"app_id"`
	AppTransID     string `json:"app_trans_id"`
	AppTime        int64  `json:"app_time"`
	AppUser        string `json:"app_user"`
	Amount         int64  `json:"amount"`
	EmbedData      string `json:"embed_data"`
	Item           string `json:"item"`
	ZpTransID      int64  `json:"zp_trans_id"`
	ServerTime     int64  `json:"server_time"`
	Channel        int    `json:"channel"`
	MerchantUserID string `json:"merchant_user_id"`
	UserFeeAmount  int64  `json:"user_fee_amount"`
	DiscountAmount int64  `json:"discount_amount"`
}
