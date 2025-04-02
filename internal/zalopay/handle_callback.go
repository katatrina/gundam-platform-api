package zalopay

import (
	"context"
	"encoding/json"
	
	db "github.com/katatrina/gundam-BE/internal/db/sqlc"
	"github.com/zpmep/hmacutil"
)

// VerifyCallback kiểm tra tính hợp lệ của callback từ Zalopay
func (z *ZalopayService) VerifyCallback(callbackData ZaloPayCallbackData) bool {
	reqMac := callbackData.Mac
	mac := hmacutil.HexStringEncode(hmacutil.SHA256, z.key2, callbackData.Data)
	return reqMac == mac
}

// ProcessCallback xử lý dữ liệu callback sau khi đã xác thực
func (z *ZalopayService) ProcessCallback(ctx context.Context, callbackData ZaloPayCallbackData) (*ZalopayCallbackResult, error) {
	// Parse dữ liệu giao dịch
	var transData TransactionData
	if err := json.Unmarshal([]byte(callbackData.Data), &transData); err != nil {
		return &ZalopayCallbackResult{
			ReturnCode:    -1,
			ReturnMessage: "Invalid transaction data",
		}, err
	}
	
	// Xử lý thanh toán thành công
	err := z.dbStore.HandleZalopayCallbackTx(ctx, db.HandleZalopayCallbackTxParams{
		AppTransID: transData.AppTransID,
		AppUser:    transData.AppUser,
	})
	
	if err != nil {
		return &ZalopayCallbackResult{
			ReturnCode:    -1,
			ReturnMessage: "Internal server error",
		}, err
	}
	
	return &ZalopayCallbackResult{
		ReturnCode:    1,
		ReturnMessage: "success",
	}, nil
}
