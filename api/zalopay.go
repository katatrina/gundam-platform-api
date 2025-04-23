package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	
	"github.com/gin-gonic/gin"
	"github.com/hibiken/asynq"
	db "github.com/katatrina/gundam-BE/internal/db/sqlc"
	"github.com/katatrina/gundam-BE/internal/token"
	"github.com/katatrina/gundam-BE/internal/util"
	"github.com/katatrina/gundam-BE/internal/worker"
	"github.com/katatrina/gundam-BE/internal/zalopay"
	"github.com/rs/zerolog/log"
)

type createZalopayOrderRequest struct {
	Amount      int64  `json:"amount" binding:"required,min=1000"`
	Description string `json:"description" binding:"required,max=256"`
	RedirectURL string `json:"redirect_url" binding:"required,url"`
}

//	@Summary		Create a ZaloPay order
//	@Description	Create a ZaloPay order
//	@Tags			wallet
//	@Accept			json
//	@Produce		json
//	@Security		accessToken
//	@Param			request	body		createZalopayOrderRequest			true	"Create ZaloPay order request"
//	@Success		200		{object}	zalopay.CreateOrderZalopayResponse	"Create ZaloPay order response"
//	@Failure		400		"Bad request"
//	@Failure		500		"Internal server error"
//	@Router			/wallet/zalopay/create [post]
func (server *Server) createZalopayOrder(c *gin.Context) {
	authPayload := c.MustGet(authorizationPayloadKey).(*token.Payload)
	appUser := authPayload.Subject
	
	var req createZalopayOrderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse(err))
		return
	}
	
	// Tạo đơn hàng Zalopay
	transID, result, err := server.zalopayService.CreateOrder(appUser, req.Amount, nil, req.Description, req.RedirectURL)
	if err != nil {
		log.Error().Err(err).Msg("Failed to create ZaloPay order")
		c.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	// Tạo metadata để lưu thông tin bổ sung
	metadata, err := json.Marshal(map[string]interface{}{
		"order_url":      result.OrderURL,
		"zp_trans_token": result.ZpTransToken,
		"order_token":    result.OrderToken,
		"qr_code":        result.QrCode,
	})
	if err != nil {
		log.Error().Err(err).Msg("Failed to marshal metadata")
		c.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	// Lưu thông tin giao dịch vào database
	transaction := db.CreatePaymentTransactionParams{
		UserID:                appUser,
		Amount:                req.Amount,
		TransactionType:       db.PaymentTransactionTypeWalletdeposit,
		Provider:              db.PaymentTransactionProviderZalopay,
		ProviderTransactionID: transID,
		Status:                db.PaymentTransactionStatusPending,
		Metadata:              metadata,
	}
	
	_, err = server.dbStore.CreatePaymentTransaction(c, transaction)
	if err != nil {
		c.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	c.JSON(http.StatusOK, result)
}

func (server *Server) handleZalopayCallback(c *gin.Context) {
	var callbackData zalopay.ZaloPayCallbackData
	if err := c.ShouldBindJSON(&callbackData); err != nil {
		c.JSON(http.StatusBadRequest, zalopay.ZalopayCallbackResult{
			ReturnCode:    -1,
			ReturnMessage: "Invalid request data",
		})
		return
	}
	
	// Bước 1: Xác thực callback
	if !server.zalopayService.VerifyCallback(callbackData) {
		c.JSON(http.StatusBadRequest, zalopay.ZalopayCallbackResult{
			ReturnCode:    -1,
			ReturnMessage: "mac not equal",
		})
		return
	}
	
	// Bước 2: Xử lý dữ liệu callback
	result, transData, err := server.zalopayService.ProcessCallback(c.Request.Context(), callbackData)
	if err != nil {
		log.Error().Err(err).Msg("Failed to process ZaloPay callback")
		c.JSON(http.StatusInternalServerError, result)
		return
	}
	
	opts := []asynq.Option{
		asynq.MaxRetry(3),
		asynq.Queue(worker.QueueCritical),
	}
	
	err = server.taskDistributor.DistributeTaskSendNotification(c.Request.Context(), &worker.PayloadSendNotification{
		RecipientID: transData.AppUser,
		Title:       fmt.Sprintf("Nạp tiền thành công: %s", util.FormatVND(transData.Amount)),
		Message:     fmt.Sprintf("Nạp tiền thành công: %s đã được thêm vào ví của bạn qua ZaloPay. Số dư mới có thể sử dụng để thanh toán đơn hàng hoặc tham gia đấu giá. Mã giao dịch: %s", util.FormatVND(transData.Amount), transData.AppTransID),
		Type:        "wallet",
		ReferenceID: transData.AppTransID,
	}, opts...)
	if err != nil {
		log.Err(err).Msgf("failed to send notification to user OfferID %s", transData.AppUser)
	}
	
	log.Info().Msgf("Successfully deposited %s into wallet for user OfferID %s", util.FormatVND(transData.Amount), transData.AppUser)
	
	// Trả về kết quả cho Zalopay server
	c.JSON(http.StatusOK, result)
}
