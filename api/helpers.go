package api

import (
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"time"
	
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	db "github.com/katatrina/gundam-BE/internal/db/sqlc"
	"github.com/katatrina/gundam-BE/internal/util"
	"github.com/katatrina/gundam-BE/internal/worker"
	"github.com/rs/zerolog/log"
)

func (server *Server) uploadFileToCloudinary(key string, value string, folder string, files ...*multipart.FileHeader) (uploadedFileURLs []string, err error) {
	if len(files) == 0 {
		return nil, fmt.Errorf("no files provided")
	}
	
	for _, file := range files {
		// Open and read file
		currentFile, err := file.Open()
		if err != nil {
			return nil, fmt.Errorf("failed to open file: %w", err)
		}
		defer currentFile.Close()
		
		fileBytes, err := io.ReadAll(currentFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read file: %w", err)
		}
		
		fileName := fmt.Sprintf("%s_%s_%d", key, value, time.Now().Unix())
		
		// Upload new avatar to cloudinary
		uploadedFileURL, err := server.fileStore.UploadFile(fileBytes, fileName, folder)
		if err != nil {
			return nil, fmt.Errorf("failed to upload file: %w", err)
		}
		
		uploadedFileURLs = append(uploadedFileURLs, uploadedFileURL)
	}
	
	return uploadedFileURLs, nil
}

// handleRegularOrderConfirmation xử lý xác nhận nhận hàng cho đơn hàng thông thường
func (server *Server) handleRegularOrderConfirmation(ctx *gin.Context, order db.Order) (db.ConfirmOrderReceivedByBuyerTxResult, error) {
	// Lấy thông tin giao dịch đơn hàng
	orderTransaction, err := server.dbStore.GetOrderTransactionByOrderID(ctx, order.ID)
	if err != nil {
		if errors.Is(err, db.ErrRecordNotFound) {
			return db.ConfirmOrderReceivedByBuyerTxResult{}, fmt.Errorf("transaction for order ID %s not found", order.ID)
		}
		return db.ConfirmOrderReceivedByBuyerTxResult{}, err
	}
	
	// Kiểm tra xem giao dịch đã có seller_entry_id chưa
	if orderTransaction.SellerEntryID == nil {
		return db.ConfirmOrderReceivedByBuyerTxResult{}, fmt.Errorf("seller entry not found for order %s", order.Code)
	}
	
	// Lấy bút toán của người bán
	sellerEntry, err := server.dbStore.GetWalletEntryByID(ctx, *orderTransaction.SellerEntryID)
	if err != nil {
		if errors.Is(err, db.ErrRecordNotFound) {
			return db.ConfirmOrderReceivedByBuyerTxResult{}, fmt.Errorf("seller entry not found for order %s", order.Code)
		}
		return db.ConfirmOrderReceivedByBuyerTxResult{}, err
	}
	
	// Lấy ví của người bán
	sellerWallet, err := server.dbStore.GetWalletForUpdate(ctx, order.SellerID)
	if err != nil {
		if errors.Is(err, db.ErrRecordNotFound) {
			return db.ConfirmOrderReceivedByBuyerTxResult{}, fmt.Errorf("wallet not found for seller %s", order.SellerID)
		}
		return db.ConfirmOrderReceivedByBuyerTxResult{}, err
	}
	
	// Lấy thông tin order items
	orderItems, err := server.dbStore.ListOrderItems(ctx, order.ID)
	if err != nil {
		return db.ConfirmOrderReceivedByBuyerTxResult{}, err
	}
	
	// Kiểm tra trạng thái của các Gundam liên quan
	for _, item := range orderItems {
		if item.GundamID != nil {
			gundam, err := server.dbStore.GetGundamByID(ctx, *item.GundamID)
			if err != nil {
				if errors.Is(err, db.ErrRecordNotFound) {
					return db.ConfirmOrderReceivedByBuyerTxResult{}, fmt.Errorf("gundam ID %d not found", *item.GundamID)
				}
				return db.ConfirmOrderReceivedByBuyerTxResult{}, err
			}
			
			if gundam.Status != db.GundamStatusProcessing {
				return db.ConfirmOrderReceivedByBuyerTxResult{}, fmt.Errorf("gundam ID %d is not in processing status", *item.GundamID)
			}
		} else {
			log.Warn().Msg("gundam ID is nil in order item")
		}
	}
	
	// Thực hiện transaction xác nhận đơn hàng đã nhận
	result, err := server.dbStore.ConfirmOrderReceivedByBuyerTx(ctx, db.ConfirmOrderReceivedByBuyerTxParams{
		Order:        &order,
		OrderItems:   orderItems,
		SellerEntry:  &sellerEntry,
		SellerWallet: &sellerWallet,
	})
	if err != nil {
		return db.ConfirmOrderReceivedByBuyerTxResult{}, err
	}
	
	// TODO: Hủy task "tự động xác nhận đơn hàng sau 7 ngày" vì người mua đã xác nhận đơn hàng
	
	opts := []asynq.Option{
		asynq.MaxRetry(3),
		asynq.Queue(worker.QueueCritical),
	}
	
	// Gửi thông báo cho người mua
	err = server.taskDistributor.DistributeTaskSendNotification(ctx.Request.Context(), &worker.PayloadSendNotification{
		RecipientID: result.Order.BuyerID,
		Title:       fmt.Sprintf("Bạn đã xác nhận hoàn tất đơn hàng %s thành công.", result.Order.Code),
		Message:     fmt.Sprintf("Mô hình Gundam đã được thêm vào bộ sưu tập của bạn."),
		Type:        "order",
		ReferenceID: result.Order.Code,
	}, opts...)
	if err != nil {
		log.Err(err).Msg("failed to send notification to buyer")
	}
	log.Info().Msgf("Notification sent to buyer: %s", result.Order.BuyerID)
	
	// Gửi thông báo cho người bán
	err = server.taskDistributor.DistributeTaskSendNotification(ctx.Request.Context(), &worker.PayloadSendNotification{
		RecipientID: result.Order.SellerID,
		Title:       fmt.Sprintf("Đơn hàng #%s đã được người mua xác nhận hoàn tất.", result.Order.Code),
		Message:     fmt.Sprintf("Quyền sở hữu mô hình Gundam đã được chuyển cho người mua. Số tiền khả dụng của bạn đã được cộng thêm %s.", util.FormatVND(result.SellerEntry.Amount)),
		Type:        "order",
		ReferenceID: result.Order.Code,
	}, opts...)
	if err != nil {
		log.Err(err).Msg("failed to send notification to seller")
	}
	log.Info().Msgf("Notification sent to seller: %s", result.Order.SellerID)
	
	return result, nil
}

// handleExchangeOrderConfirmation xử lý xác nhận nhận hàng cho đơn hàng trao đổi
func (server *Server) handleExchangeOrderConfirmation(ctx *gin.Context, order db.Order) (db.ConfirmExchangeOrderReceivedTxResult, error) {
	// Lấy thông tin exchange từ order
	exchange, err := server.dbStore.GetExchangeByOrderID(ctx, &order.ID)
	if err != nil {
		if errors.Is(err, db.ErrRecordNotFound) {
			return db.ConfirmExchangeOrderReceivedTxResult{}, fmt.Errorf("exchange for order ID %s not found", order.ID)
		}
		return db.ConfirmExchangeOrderReceivedTxResult{}, err
	}
	
	// Xác định đơn hàng đối tác
	var partnerOrderID uuid.UUID
	isPosterOrder := exchange.PosterOrderID != nil && *exchange.PosterOrderID == order.ID
	isOffererOrder := exchange.OffererOrderID != nil && *exchange.OffererOrderID == order.ID
	
	if isPosterOrder && exchange.OffererOrderID != nil {
		partnerOrderID = *exchange.OffererOrderID
	} else if isOffererOrder && exchange.PosterOrderID != nil {
		partnerOrderID = *exchange.PosterOrderID
	} else {
		return db.ConfirmExchangeOrderReceivedTxResult{}, fmt.Errorf("invalid exchange configuration")
	}
	
	// Lấy thông tin order items
	orderItems, err := server.dbStore.ListOrderItems(ctx, order.ID)
	if err != nil {
		return db.ConfirmExchangeOrderReceivedTxResult{}, err
	}
	
	// Kiểm tra trạng thái của các Gundam liên quan
	for _, item := range orderItems {
		if item.GundamID != nil {
			gundam, err := server.dbStore.GetGundamByID(ctx, *item.GundamID)
			if err != nil {
				if errors.Is(err, db.ErrRecordNotFound) {
					return db.ConfirmExchangeOrderReceivedTxResult{}, fmt.Errorf("gundam ID %d not found", *item.GundamID)
				}
				return db.ConfirmExchangeOrderReceivedTxResult{}, err
			}
			
			if gundam.Status != db.GundamStatusExchanging {
				return db.ConfirmExchangeOrderReceivedTxResult{}, fmt.Errorf("gundam ID %d is not in exchanging status", *item.GundamID)
			}
		}
	}
	
	// Lấy thông tin exchange items (tất cả các items trong exchange)
	exchangeItems, err := server.dbStore.ListExchangeItems(ctx, db.ListExchangeItemsParams{
		ExchangeID: exchange.ID,
	})
	if err != nil {
		return db.ConfirmExchangeOrderReceivedTxResult{}, err
	}
	
	// Thực hiện transaction xác nhận đơn hàng trao đổi đã nhận
	result, err := server.dbStore.ConfirmExchangeOrderReceivedTx(ctx, db.ConfirmExchangeOrderReceivedTxParams{
		Order:          &order,
		Exchange:       &exchange,
		ExchangeItems:  exchangeItems,
		PartnerOrderID: partnerOrderID,
	})
	if err != nil {
		return db.ConfirmExchangeOrderReceivedTxResult{}, err
	}
	
	// Gửi thông báo
	opts := []asynq.Option{
		asynq.MaxRetry(3),
		asynq.Queue(worker.QueueCritical),
	}
	
	// Thông báo cho người xác nhận đơn hàng
	err = server.taskDistributor.DistributeTaskSendNotification(ctx.Request.Context(), &worker.PayloadSendNotification{
		RecipientID: order.BuyerID,
		Title:       fmt.Sprintf("Bạn đã xác nhận hoàn tất đơn hàng trao đổi %s thành công.", order.Code),
		Message:     fmt.Sprintf("Các mô hình Gundam đã được cập nhật trong bộ sưu tập của bạn."),
		Type:        "order",
		ReferenceID: order.Code,
	}, opts...)
	if err != nil {
		log.Err(err).Msg("failed to send notification to order confirmer")
	}
	
	// Gửi thông báo cho đối tác về việc đơn hàng được xác nhận
	var currentUserID, partnerID string
	if isPosterOrder {
		currentUserID = exchange.PosterID
		partnerID = exchange.OffererID
	} else {
		currentUserID = exchange.OffererID
		partnerID = exchange.PosterID
	}
	
	currentUser, err := server.dbStore.GetUserByID(ctx, currentUserID)
	if err != nil {
		if errors.Is(err, db.ErrRecordNotFound) {
			return db.ConfirmExchangeOrderReceivedTxResult{}, fmt.Errorf("user ID %s not found for order %s", currentUserID, order.Code)
		}
		
		return db.ConfirmExchangeOrderReceivedTxResult{}, err
	}
	
	message := fmt.Sprintf("%s đã xác nhận nhận hàng thành công cho đơn hàng trao đổi %s.", currentUser.FullName, order.Code)
	
	// Nếu giao dịch trao đổi đã hoàn tất, cập nhật nội dung thông báo
	if result.Exchange != nil && result.Exchange.Status == db.ExchangeStatusCompleted {
		message = fmt.Sprintf("Giao dịch trao đổi đã hoàn tất. Các mô hình Gundam đã được chuyển quyền sở hữu.")
		
		// Nếu có tiền bù, thêm thông tin
		if result.Exchange.PayerID != nil && result.Exchange.CompensationAmount != nil {
			if partnerID == *result.Exchange.PayerID {
				message += fmt.Sprintf(" Bạn đã trả %s tiền bù cho giao dịch này.", util.FormatVND(*result.Exchange.CompensationAmount))
			} else {
				message += fmt.Sprintf(" Bạn đã nhận %s tiền bù cho giao dịch này.", util.FormatVND(*result.Exchange.CompensationAmount))
			}
		}
	}
	
	err = server.taskDistributor.DistributeTaskSendNotification(ctx.Request.Context(), &worker.PayloadSendNotification{
		RecipientID: partnerID,
		Title:       "Cập nhật giao dịch trao đổi",
		Message:     message,
		Type:        "exchange",
		ReferenceID: exchange.ID.String(),
	}, opts...)
	if err != nil {
		log.Err(err).Msg("failed to send notification to exchange partner")
	}
	
	return result, nil
}
