package zalopay

import (
	"github.com/zpmep/hmacutil"
)

// VerifyCallback kiểm tra tính hợp lệ của callback từ Zalopay
func (z *ZalopayService) VerifyCallback(callbackData ZaloPayCallbackData) bool {
	reqMac := callbackData.Mac
	mac := hmacutil.HexStringEncode(hmacutil.SHA256, z.key2, callbackData.Data)
	return reqMac == mac
}

// ProcessCallback xử lý dữ liệu callback sau khi đã xác thực
// func (z *ZalopayService) ProcessCallback(callbackData ZaloPayCallbackData) (*ZalopayCallbackResult, error) {
// 	// Parse dữ liệu giao dịch
// 	var transData TransactionData
// 	if err := json.Unmarshal([]byte(callbackData.Data), &transData); err != nil {
// 		return &ZalopayCallbackResult{
// 			ReturnCode:    0,
// 			ReturnMessage: "Invalid transaction data",
// 		}, err
// 	}
//
// 	// Xử lý thanh toán thành công
// 	err := z.dbStore.WithTx(func(tx *sql.Tx) error {
// 		// 1. Kiểm tra và cập nhật trạng thái giao dịch
//
// 		// Kiểm tra giao dịch đã tồn tại trong DB chưa
// 		transaction, err := z.store.GetPaymentTransactionByProviderID(context.Background(), db.GetPaymentTransactionByProviderIDParams{
// 			Provider:              "zalopay",
// 			ProviderTransactionID: transData.AppTransID,
// 		})
// 		if err != nil {
// 			return err
// 		}
//
// 		// Nếu đã xử lý rồi thì không làm gì nữa
// 		if transaction.Status == "success" {
// 			return nil
// 		}
//
// 		// 2. Cập nhật trạng thái giao dịch
// 		metadata, _ := json.Marshal(map[string]interface{}{
// 			"zp_trans_id": transData.ZpTransID,
// 			"channel":     transData.Channel,
// 			"server_time": transData.ServerTime,
// 		})
//
// 		err = z.store.UpdatePaymentTransactionStatus(context.Background(), db.UpdatePaymentTransactionStatusParams{
// 			Provider:              "zalopay",
// 			ProviderTransactionID: transData.AppTransID,
// 			Status:                "success",
// 			Metadata:              metadata,
// 		})
// 		if err != nil {
// 			return err
// 		}
//
// 		// 3. Cộng tiền vào ví người dùng
// 		_, err = z.store.CreateWalletEntry(context.Background(), db.CreateWalletEntryParams{
// 			UserID:        transData.AppUser,
// 			Amount:        transData.Amount,
// 			Type:          db.WalletEntryTypeDeposit,
// 			ReferenceID:   transaction.ID,
// 			ReferenceType: db.WalletReferenceTypeThirdPartyPayment,
// 			Description:   "Nạp tiền qua Zalopay",
// 		})
//
// 		return err
// 	})
//
// 	if err != nil {
// 		return &ZalopayCallbackResult{
// 			ReturnCode:    0,
// 			ReturnMessage: "Internal server error",
// 		}, err
// 	}
//
// 	return &ZalopayCallbackResult{
// 		ReturnCode:    1,
// 		ReturnMessage: "success",
// 	}, nil
// }
