package api

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	mathrand "math/rand"
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
func (server *Server) handleRegularOrderConfirmation(ctx *gin.Context, order db.Order) (db.CompleteRegularOrderTxResult, error) {
	// Lấy thông tin giao dịch đơn hàng
	orderTransaction, err := server.dbStore.GetOrderTransactionByOrderID(ctx, order.ID)
	if err != nil {
		if errors.Is(err, db.ErrRecordNotFound) {
			return db.CompleteRegularOrderTxResult{}, fmt.Errorf("transaction for order ID %s not found", order.ID)
		}
		return db.CompleteRegularOrderTxResult{}, err
	}
	
	// Kiểm tra xem giao dịch đã có seller_entry_id chưa
	if orderTransaction.SellerEntryID == nil {
		return db.CompleteRegularOrderTxResult{}, fmt.Errorf("seller entry not found for order %s", order.Code)
	}
	
	// Lấy bút toán của người bán
	sellerEntry, err := server.dbStore.GetWalletEntryByID(ctx, *orderTransaction.SellerEntryID)
	if err != nil {
		if errors.Is(err, db.ErrRecordNotFound) {
			return db.CompleteRegularOrderTxResult{}, fmt.Errorf("seller entry not found for order %s", order.Code)
		}
		return db.CompleteRegularOrderTxResult{}, err
	}
	
	// Lấy ví của người bán
	sellerWallet, err := server.dbStore.GetWalletForUpdate(ctx, order.SellerID)
	if err != nil {
		if errors.Is(err, db.ErrRecordNotFound) {
			return db.CompleteRegularOrderTxResult{}, fmt.Errorf("wallet not found for seller %s", order.SellerID)
		}
		return db.CompleteRegularOrderTxResult{}, err
	}
	
	// Lấy thông tin order items
	orderItems, err := server.dbStore.ListOrderItems(ctx, order.ID)
	if err != nil {
		return db.CompleteRegularOrderTxResult{}, err
	}
	
	// Kiểm tra trạng thái của các Gundam liên quan
	for _, item := range orderItems {
		if item.GundamID != nil {
			gundam, err := server.dbStore.GetGundamByID(ctx, *item.GundamID)
			if err != nil {
				if errors.Is(err, db.ErrRecordNotFound) {
					return db.CompleteRegularOrderTxResult{}, fmt.Errorf("gundam ID %d not found", *item.GundamID)
				}
				return db.CompleteRegularOrderTxResult{}, err
			}
			
			if gundam.Status != db.GundamStatusProcessing {
				return db.CompleteRegularOrderTxResult{}, fmt.Errorf("gundam ID %d is not in processing status", *item.GundamID)
			}
		} else {
			log.Warn().Msg("gundam ID is nil in order item")
		}
	}
	
	// Thực hiện transaction xác nhận đơn hàng đã nhận
	result, err := server.dbStore.CompleteRegularOrderTx(ctx, db.CompleteRegularOrderTxParams{
		Order:        &order,
		OrderItems:   orderItems,
		SellerEntry:  &sellerEntry,
		SellerWallet: &sellerWallet,
	})
	if err != nil {
		return db.CompleteRegularOrderTxResult{}, err
	}
	
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
func (server *Server) handleExchangeOrderConfirmation(ctx *gin.Context, order db.Order) (db.CompleteExchangeOrderTxResult, error) {
	// Lấy thông tin exchange từ order
	exchange, err := server.dbStore.GetExchangeByOrderID(ctx, &order.ID)
	if err != nil {
		if errors.Is(err, db.ErrRecordNotFound) {
			return db.CompleteExchangeOrderTxResult{}, fmt.Errorf("exchange for order ID %s not found", order.ID)
		}
		return db.CompleteExchangeOrderTxResult{}, err
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
		return db.CompleteExchangeOrderTxResult{}, fmt.Errorf("invalid exchange configuration")
	}
	
	// Lấy thông tin order items
	orderItems, err := server.dbStore.ListOrderItems(ctx, order.ID)
	if err != nil {
		return db.CompleteExchangeOrderTxResult{}, err
	}
	
	// Kiểm tra trạng thái của các Gundam liên quan
	for _, item := range orderItems {
		if item.GundamID != nil {
			gundam, err := server.dbStore.GetGundamByID(ctx, *item.GundamID)
			if err != nil {
				if errors.Is(err, db.ErrRecordNotFound) {
					return db.CompleteExchangeOrderTxResult{}, fmt.Errorf("gundam ID %d not found", *item.GundamID)
				}
				return db.CompleteExchangeOrderTxResult{}, err
			}
			
			if gundam.Status != db.GundamStatusExchanging {
				return db.CompleteExchangeOrderTxResult{}, fmt.Errorf("gundam ID %d is not in exchanging status", *item.GundamID)
			}
		}
	}
	
	// Lấy thông tin exchange items (tất cả các items trong exchange)
	exchangeItems, err := server.dbStore.ListExchangeItems(ctx, db.ListExchangeItemsParams{
		ExchangeID: exchange.ID,
	})
	if err != nil {
		return db.CompleteExchangeOrderTxResult{}, err
	}
	
	// Thực hiện transaction xác nhận đơn hàng trao đổi đã nhận
	result, err := server.dbStore.CompleteExchangeOrderTx(ctx, db.CompleteExchangeOrderTxParams{
		Order:          &order,
		Exchange:       &exchange,
		ExchangeItems:  exchangeItems,
		PartnerOrderID: partnerOrderID,
	})
	if err != nil {
		return db.CompleteExchangeOrderTxResult{}, err
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
			return db.CompleteExchangeOrderTxResult{}, fmt.Errorf("user ID %s not found for order %s", currentUserID, order.Code)
		}
		
		return db.CompleteExchangeOrderTxResult{}, err
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

// handleAuctionOrderConfirmation xử lý xác nhận nhận hàng cho đơn hàng đấu giá
func (server *Server) handleAuctionOrderConfirmation(ctx *gin.Context, order db.Order) (db.CompleteRegularOrderTxResult, error) {
	// Lấy thông tin giao dịch đơn hàng - giống như đơn hàng thông thường
	orderTransaction, err := server.dbStore.GetOrderTransactionByOrderID(ctx, order.ID)
	if err != nil {
		if errors.Is(err, db.ErrRecordNotFound) {
			return db.CompleteRegularOrderTxResult{}, fmt.Errorf("transaction for order ID %s not found", order.ID)
		}
		return db.CompleteRegularOrderTxResult{}, err
	}
	
	// Kiểm tra xem giao dịch đã có seller_entry_id chưa
	if orderTransaction.SellerEntryID == nil {
		return db.CompleteRegularOrderTxResult{}, fmt.Errorf("seller entry not found for order %s", order.Code)
	}
	
	// Lấy bút toán của người bán
	sellerEntry, err := server.dbStore.GetWalletEntryByID(ctx, *orderTransaction.SellerEntryID)
	if err != nil {
		if errors.Is(err, db.ErrRecordNotFound) {
			return db.CompleteRegularOrderTxResult{}, fmt.Errorf("seller entry not found for order %s", order.Code)
		}
		return db.CompleteRegularOrderTxResult{}, err
	}
	
	// Lấy ví của người bán
	sellerWallet, err := server.dbStore.GetWalletForUpdate(ctx, order.SellerID)
	if err != nil {
		if errors.Is(err, db.ErrRecordNotFound) {
			return db.CompleteRegularOrderTxResult{}, fmt.Errorf("wallet not found for seller %s", order.SellerID)
		}
		return db.CompleteRegularOrderTxResult{}, err
	}
	
	// Lấy thông tin order items
	orderItems, err := server.dbStore.ListOrderItems(ctx, order.ID)
	if err != nil {
		return db.CompleteRegularOrderTxResult{}, err
	}
	
	// Kiểm tra trạng thái của các Gundam liên quan
	for _, item := range orderItems {
		if item.GundamID != nil {
			gundam, err := server.dbStore.GetGundamByID(ctx, *item.GundamID)
			if err != nil {
				if errors.Is(err, db.ErrRecordNotFound) {
					return db.CompleteRegularOrderTxResult{}, fmt.Errorf("gundam ID %d not found", *item.GundamID)
				}
				return db.CompleteRegularOrderTxResult{}, err
			}
			
			if gundam.Status != db.GundamStatusProcessing {
				return db.CompleteRegularOrderTxResult{}, fmt.Errorf("gundam ID %d is not in processing status", *item.GundamID)
			}
		} else {
			log.Warn().Msg("gundam ID is nil in order item")
		}
	}
	
	// Lấy thông tin phiên đấu giá liên quan (để hiển thị thông tin trong thông báo)
	auction, err := server.dbStore.GetAuctionByOrderID(ctx, &order.ID)
	if err != nil && !errors.Is(err, db.ErrRecordNotFound) {
		return db.CompleteRegularOrderTxResult{}, fmt.Errorf("failed to get auction details: %w", err)
	}
	
	// Thực hiện transaction xác nhận đơn hàng đã nhận - sử dụng lại transaction của đơn hàng thông thường
	result, err := server.dbStore.CompleteRegularOrderTx(ctx, db.CompleteRegularOrderTxParams{
		Order:        &order,
		OrderItems:   orderItems,
		SellerEntry:  &sellerEntry,
		SellerWallet: &sellerWallet,
	})
	if err != nil {
		return db.CompleteRegularOrderTxResult{}, err
	}
	
	opts := []asynq.Option{
		asynq.MaxRetry(3),
		asynq.Queue(worker.QueueCritical),
	}
	
	// Gửi thông báo cho người mua - Thông báo riêng cho đơn hàng đấu giá
	var auctionInfo string
	if auction.ID != uuid.Nil {
		auctionInfo = fmt.Sprintf(" từ phiên đấu giá %s", auction.GundamSnapshot.Name)
	}
	
	err = server.taskDistributor.DistributeTaskSendNotification(ctx.Request.Context(), &worker.PayloadSendNotification{
		RecipientID: result.Order.BuyerID,
		Title:       fmt.Sprintf("Bạn đã xác nhận hoàn tất đơn hàng đấu giá %s thành công.", result.Order.Code),
		Message:     fmt.Sprintf("Mô hình Gundam%s đã được thêm vào bộ sưu tập của bạn.", auctionInfo),
		Type:        "order",
		ReferenceID: result.Order.Code,
	}, opts...)
	if err != nil {
		log.Err(err).Msg("failed to send notification to buyer")
	}
	log.Info().Msgf("Notification sent to buyer: %s", result.Order.BuyerID)
	
	// Gửi thông báo cho người bán - Thông báo riêng cho đơn hàng đấu giá
	err = server.taskDistributor.DistributeTaskSendNotification(ctx.Request.Context(), &worker.PayloadSendNotification{
		RecipientID: result.Order.SellerID,
		Title:       fmt.Sprintf("Đơn hàng đấu giá #%s đã được người mua xác nhận hoàn tất.", result.Order.Code),
		Message:     fmt.Sprintf("Quyền sở hữu mô hình Gundam%s đã được chuyển cho người mua. Số dư khả dụng của bạn đã được cộng thêm %s.", auctionInfo, util.FormatVND(result.SellerEntry.Amount)),
		Type:        "order",
		ReferenceID: result.Order.Code,
	}, opts...)
	if err != nil {
		log.Err(err).Msg("failed to send notification to seller")
	}
	log.Info().Msgf("Notification sent to seller: %s", result.Order.SellerID)
	
	return result, nil
}

// sendExchangeCancelNotifications sends notifications to both parties about the canceled exchange
func (server *Server) sendExchangeCancelNotifications(ctx context.Context, result db.CancelExchangeTxResult, canceledByID string) {
	exchange := result.Exchange
	
	// Get user who canceled
	canceledBy, err := server.dbStore.GetUserByID(ctx, canceledByID)
	if err != nil {
		log.Err(err).Msg("failed to get user who canceled exchange")
		return
	}
	
	// Create notification options
	opts := []asynq.Option{
		asynq.MaxRetry(3),
		asynq.Queue(worker.QueueCritical),
	}
	
	// Create base notification content
	baseTitle := "Giao dịch trao đổi đã bị hủy"
	exchangeCode := exchange.ID.String()[:8] // First 8 characters of UUID for reference
	baseMessage := fmt.Sprintf("Giao dịch trao đổi #%s đã bị hủy bởi %s.", exchangeCode, canceledBy.FullName)
	
	// Add reason if provided
	if exchange.CanceledReason != nil && *exchange.CanceledReason != "" {
		baseMessage += fmt.Sprintf(" Lý do: %s.", *exchange.CanceledReason)
	}
	
	// Add refund information if applicable
	var refundMessage string
	if result.RefundedCompensation {
		refundMessage = fmt.Sprintf(" Tiền bù %s đã được hoàn trả.", util.FormatVND(*exchange.CompensationAmount))
	}
	
	if result.RefundedPosterDeliveryFee {
		posterRefundMsg := fmt.Sprintf(" Phí vận chuyển %s đã được hoàn trả.", util.FormatVND(*exchange.PosterDeliveryFee))
		if exchange.PosterID == canceledByID {
			// Don't add refund message for the person who canceled
		} else {
			refundMessage += posterRefundMsg
		}
	}
	
	if result.RefundedOffererDeliveryFee {
		offererRefundMsg := fmt.Sprintf(". Phí vận chuyển %s đã được hoàn trả.", util.FormatVND(*exchange.OffererDeliveryFee))
		if exchange.OffererID == canceledByID {
			// Don't add refund message for the person who canceled
		} else {
			refundMessage += offererRefundMsg
		}
	}
	
	// Send notification to poster
	if exchange.PosterID != canceledByID {
		posterMessage := baseMessage + refundMessage
		err = server.taskDistributor.DistributeTaskSendNotification(ctx, &worker.PayloadSendNotification{
			RecipientID: exchange.PosterID,
			Title:       baseTitle,
			Message:     posterMessage,
			Type:        "exchange",
			ReferenceID: exchange.ID.String(),
		}, opts...)
		if err != nil {
			log.Err(err).Msg("failed to send notification to poster")
		}
	}
	
	// Send notification to offerer
	if exchange.OffererID != canceledByID {
		offererMessage := baseMessage + refundMessage
		err = server.taskDistributor.DistributeTaskSendNotification(ctx, &worker.PayloadSendNotification{
			RecipientID: exchange.OffererID,
			Title:       baseTitle,
			Message:     offererMessage,
			Type:        "exchange",
			ReferenceID: exchange.ID.String(),
		}, opts...)
		if err != nil {
			log.Err(err).Msg("failed to send notification to offerer")
		}
	}
	
	// Send confirmation to the person who canceled
	err = server.taskDistributor.DistributeTaskSendNotification(ctx, &worker.PayloadSendNotification{
		RecipientID: canceledByID,
		Title:       "Bạn đã hủy giao dịch trao đổi",
		Message:     fmt.Sprintf("Bạn đã hủy giao dịch trao đổi #%s thành công.", exchangeCode),
		Type:        "exchange",
		ReferenceID: exchange.ID.String(),
	}, opts...)
	if err != nil {
		log.Err(err).Msg("failed to send confirmation to canceler")
	}
}

// sendOrderCancelNotifications sends notifications about order cancellation to relevant parties
func (server *Server) sendOrderCancelNotifications(ctx context.Context, result db.CancelOrderByBuyerTxResult, canceledByID string) {
	order := result.Order
	
	// Get user who canceled
	canceledBy, err := server.dbStore.GetUserByID(ctx, canceledByID)
	if err != nil {
		log.Err(err).Msg("failed to get user who canceled order")
		return
	}
	
	opts := []asynq.Option{
		asynq.MaxRetry(3),
		asynq.Queue(worker.QueueCritical),
	}
	
	// Create base notification content
	baseTitle := "Đơn hàng đã bị hủy"
	baseMessage := fmt.Sprintf("Đơn hàng #%s đã bị hủy bởi người mua %s.", order.Code, canceledBy.FullName)
	
	// Add reason if provided
	if order.CanceledReason != nil && *order.CanceledReason != "" {
		baseMessage += fmt.Sprintf(" Lý do: %s", *order.CanceledReason)
	}
	
	// Send notification to seller using asynq + Firestore database
	err = server.taskDistributor.DistributeTaskSendNotification(ctx, &worker.PayloadSendNotification{
		RecipientID: order.SellerID,
		Title:       baseTitle,
		Message:     baseMessage,
		Type:        "order",
		ReferenceID: order.Code,
	}, opts...)
	if err != nil {
		log.Err(err).Msg("failed to send notification to seller")
	}
	
	// Send confirmation to buyer
	refundMessage := ""
	if result.RefundEntry.Amount > 0 {
		refundMessage = fmt.Sprintf(" Số tiền %s đã được hoàn lại vào ví của bạn.", util.FormatVND(result.RefundEntry.Amount))
	}
	
	err = server.taskDistributor.DistributeTaskSendNotification(ctx, &worker.PayloadSendNotification{
		RecipientID: order.BuyerID,
		Title:       "Bạn đã hủy đơn hàng",
		Message:     fmt.Sprintf("Bạn đã hủy đơn hàng #%s thành công.%s", order.Code, refundMessage),
		Type:        "order",
		ReferenceID: order.Code,
	}, opts...)
	if err != nil {
		log.Err(err).Msg("failed to send confirmation to buyer")
	}
}

// generateRandomAvatar tự động tạo avatar ngẫu nhiên cho người dùng.
func (server *Server) generateRandomAvatar(ctx context.Context, fullName string) (string, error) {
	// Danh sách các style có sẵn trong DiceBear 9.x
	styles := []string{
		"adventurer", "adventurer-neutral", "avataaars", "avataaars-neutral",
		"big-ears", "big-ears-neutral", "big-smile", "bottts", "bottts-neutral",
		"croodles", "croodles-neutral", "fun-emoji", "icons", "identicon",
		"initials", "lorelei", "lorelei-neutral", "micah", "miniavs",
		"notionists", "notionists-neutral", "open-peeps", "personas",
		"pixel-art", "pixel-art-neutral", "shapes", "thumbs",
	}
	
	// Danh sách các màu nền cơ bản và an toàn (được hỗ trợ bởi tất cả các style)
	// Sử dụng mã hex không có dấu #
	backgroundColors := []string{
		"b6e3f4",      // Xanh nhạt
		"c0aede",      // Tím nhạt
		"d1d4f9",      // Xanh dương nhạt
		"ffd5dc",      // Hồng nhạt
		"ffdfbf",      // Vàng nhạt
		"transparent", // Trong suốt
	}
	
	// Tạo số ngẫu nhiên an toàn
	var seed int64
	if err := binary.Read(rand.Reader, binary.BigEndian, &seed); err != nil {
		// Fallback nếu crypto/rand thất bại
		seed = time.Now().UnixNano()
	}
	
	// Tạo nguồn ngẫu nhiên riêng
	r := mathrand.New(mathrand.NewSource(seed))
	
	// Chọn một style ngẫu nhiên
	randomStyleIndex := r.Intn(len(styles))
	style := styles[randomStyleIndex]
	
	// Chọn một màu nền ngẫu nhiên
	randomColorIndex := r.Intn(len(backgroundColors))
	backgroundColor := backgroundColors[randomColorIndex]
	
	// Tạo URL API với định dạng SVG
	url := fmt.Sprintf("https://api.dicebear.com/9.x/%s/svg", style)
	
	// Thực hiện request đến DiceBear API
	resp, err := server.restyClient.R().
		SetQueryParams(map[string]string{
			"seed":            fullName,        // Sử dụng full_name làm seed
			"backgroundColor": backgroundColor, // Sử dụng màu nền ngẫu nhiên
			"radius":          "0",             // Không bo góc
			"scale":           "90",            // Tỷ lệ phần tử 90%
			"translateX":      "0",             // Không dịch chuyển theo X
			"translateY":      "0",             // Không dịch chuyển theo Y
			"size":            "256",           // Kích thước cố định 256px
			"flip":            "false",         // Không lật avatar
			"rotate":          "0",             // Không xoay avatar
			"randomizeIds":    "true",          // Ngẫu nhiên hóa ID trong SVG
		}).
		Get(url)
	
	if err != nil {
		return "", fmt.Errorf("failed to get avatar from DiceBear: %w", err)
	}
	
	// Kiểm tra status code
	if resp.StatusCode() != 200 {
		return "", fmt.Errorf("DiceBear API error, status code: %d", resp.StatusCode())
	}
	
	fmt.Println("Status code:", resp.StatusCode())
	fmt.Println("Content-Type:", resp.Header().Get("Content-Type"))
	
	// Tạo tên file duy nhất (chỉ để đặt tên, không tạo file thật)
	fileName := fmt.Sprintf("avatar-%s.svg", uuid.New().String())
	
	// Lấy nội dung response body như một mảng byte
	svgData := resp.Bytes()
	if svgData == nil || len(svgData) == 0 {
		return "", fmt.Errorf("empty SVG data received from DiceBear")
	}
	
	// Tải lên Cloudinary trực tiếp từ dữ liệu trong bộ nhớ
	uploadedAvatarURL, err := server.fileStore.UploadFile(svgData, fileName, "avatars")
	if err != nil {
		return "", fmt.Errorf("failed to upload avatar to Cloudinary: %w", err)
	}
	
	return uploadedAvatarURL, nil
}
