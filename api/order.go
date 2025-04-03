package api

import (
	"errors"
	"fmt"
	"net/http"
	"time"
	
	"github.com/gin-gonic/gin"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgtype"
	db "github.com/katatrina/gundam-BE/internal/db/sqlc"
	"github.com/katatrina/gundam-BE/internal/token"
	"github.com/katatrina/gundam-BE/internal/worker"
	"github.com/rs/zerolog/log"
)

// CreateOrderRequest contains the necessary information to create a new order
type createOrderRequest struct {
	// ID of the seller
	// example: user123
	SellerID string `json:"seller_id" binding:"required"`
	
	// List of Gundam IDs in the order
	// example: [1, 2, 3]
	GundamIDs []int64 `json:"gundam_ids" binding:"required,dive,gt=0"`
	
	// ID of the buyer's address
	// example: 42
	BuyerAddressID int64 `json:"buyer_address_id" binding:"required"`
	
	// Delivery fee (VND)
	// minimum: 0
	// example: 30000
	DeliveryFee int64 `json:"delivery_fee" binding:"required,min=0"`
	
	// Expected delivery time
	// example: 2025-04-05T10:00:00Z
	ExpectedDeliveryTime time.Time `json:"expected_delivery_time" binding:"required"`
	
	// Payment method (wallet: pay via platform wallet, cod: cash on delivery)
	// enums: wallet,cod
	// example: wallet
	PaymentMethod string `json:"payment_method" binding:"required,oneof=wallet cod"`
	
	// Total value of all items (excluding delivery fee)
	// minimum: 0
	// example: 500000
	ItemsSubtotal int64 `json:"items_subtotal" binding:"required,min=0"`
	
	// Total order amount (including delivery fee)
	// minimum: 0
	// example: 530000
	TotalAmount int64 `json:"total_amount" binding:"required,min=0"`
	
	// Optional note for the order
	// maxLength: 255
	// example: Please deliver in the morning
	Note *string `json:"note" binding:"max=255"`
}

func (r *createOrderRequest) getNote() string {
	if r.Note != nil {
		return *r.Note
	}
	
	return ""
}

//	@Summary		Create a new order
//	@Description	Create a new order for purchasing Gundam models
//	@Tags			orders
//	@Accept			json
//	@Produce		json
//	@Param			createOrderRequest	body		createOrderRequest		true	"Order details"
//	@Success		201					{object}	db.CreateOrderTxResult	"Order created successfully"
//	@Failure		400					{object}	gin.H					"Invalid request data"
//	@Failure		401					{object}	gin.H					"Unauthorized"
//	@Failure		422					{object}	gin.H					"Invalid items or price mismatch"
//	@Failure		500					{object}	gin.H					"Internal server error"
//	@Security		accessToken
//	@Router			/orders [post]
func (server *Server) createOrder(ctx *gin.Context) {
	// Lấy userID từ token xác thực
	userID := ctx.MustGet(authorizationPayloadKey).(*token.Payload).Subject
	
	var req createOrderRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, errorResponse(err))
		return
	}
	
	// Tính toán tổng giá trị thực tế của các sản phẩm
	realItemsSubtotal := int64(0)
	gundams := make([]db.Gundam, len(req.GundamIDs))
	
	// Duyệt qua từng gundam trong danh sách để kiểm tra tính hợp lệ
	for i, gundamID := range req.GundamIDs {
		// Kiểm tra xem gundam có hợp lệ để thanh toán không
		result, err := server.dbStore.ValidateGundamBeforeCheckout(ctx, gundamID)
		if err != nil {
			log.Err(err).Msg("failed to validate gundam before checkout")
			ctx.JSON(http.StatusInternalServerError, errorResponse(err))
			return
		}
		
		// Valid = true khi gundam tồn tại và đang published
		if !result.Valid {
			ctx.JSON(http.StatusUnprocessableEntity, gin.H{
				"error":   "One or more items in your cart are no longer available",
				"details": fmt.Sprintf("Gundam ID %d is not available for purchasing", gundamID),
			})
			return
		}
		
		// Kiểm tra người sở hữu
		if result.Gundam.OwnerID != req.SellerID {
			ctx.JSON(http.StatusUnprocessableEntity, gin.H{
				"error":   "Seller does not own one or more items",
				"details": fmt.Sprintf("Gundam ID %d is not owned by the specified seller", gundamID),
			})
			return
		}
		
		// Tránh seller mua sản phẩm của chính mình
		if result.Gundam.OwnerID == userID {
			ctx.JSON(http.StatusUnprocessableEntity, gin.H{
				"error":   "Seller cannot purchase their own items",
				"details": fmt.Sprintf("Gundam ID %d is owned by the seller", gundamID),
			})
			return
		}
		
		realItemsSubtotal += result.Gundam.Price
		gundams[i] = result.Gundam
	}
	
	// Kiểm tra xem tổng giá trị sản phẩm có khớp với tổng giá trị thực tế không
	if req.ItemsSubtotal != realItemsSubtotal {
		ctx.JSON(http.StatusUnprocessableEntity, gin.H{
			"error":   "Price mismatch",
			"details": fmt.Sprintf("Submitted subtotal (%d) does not match actual subtotal (%d)", req.ItemsSubtotal, realItemsSubtotal),
		})
		return
	}
	
	// Kiểm tra xem tổng giá trị đơn hàng có hợp lệ không
	if req.TotalAmount != (realItemsSubtotal + req.DeliveryFee) {
		ctx.JSON(http.StatusUnprocessableEntity, gin.H{
			"error":   "Total amount mismatch",
			"details": fmt.Sprintf("Submitted total amount (%d) does not match calculated total amount (%d)", req.TotalAmount, realItemsSubtotal+req.DeliveryFee),
		})
		return
	}
	
	// Lấy thông tin địa chỉ người mua
	buyerAddress, err := server.dbStore.GetUserAddressByID(ctx, db.GetUserAddressByIDParams{
		ID:     req.BuyerAddressID,
		UserID: userID,
	})
	if err != nil {
		if errors.Is(err, db.ErrRecordNotFound) {
			log.Err(err).Msg("buyer address not found")
			ctx.JSON(http.StatusUnprocessableEntity, errorResponse(errors.New("buyer address not found")))
			return
		}
		
		log.Err(err).Msg("failed to get user address by ID")
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	// Lấy thông tin địa chỉ lấy hàng của người bán
	sellerAddress, err := server.dbStore.GetUserPickupAddress(ctx, req.SellerID)
	if err != nil {
		if errors.Is(err, db.ErrRecordNotFound) {
			log.Err(err).Msg("seller pickup address not found")
			ctx.JSON(http.StatusUnprocessableEntity, errorResponse(errors.New("seller pickup address not found")))
			return
		}
		
		log.Err(err).Msg("failed to get user pickup address")
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	// Chuẩn bị tham số cho transaction createOrder
	arg := db.CreateOrderTxParams{
		BuyerID:              userID,
		BuyerAddress:         buyerAddress,
		SellerID:             req.SellerID,
		SellerAddress:        sellerAddress,
		ItemsSubtotal:        req.ItemsSubtotal, // Tổng giá trị các sản phẩm
		DeliveryFee:          req.DeliveryFee,   // Phí vận chuyển
		TotalAmount:          req.TotalAmount,   // Tổng giá trị đơn hàng (bao gồm phí vận chuyển)
		ExpectedDeliveryTime: req.ExpectedDeliveryTime,
		PaymentMethod:        db.PaymentMethod(req.PaymentMethod),
		Note: pgtype.Text{
			String: req.getNote(),
			Valid:  req.Note != nil,
		},
		Gundams: gundams,
	}
	
	// Thực hiện transaction tạo đơn hàng
	result, err := server.dbStore.CreateOrderTx(ctx, arg)
	if err != nil {
		log.Err(err).Msg("failed to create order")
		ctx.JSON(http.StatusUnprocessableEntity, errorResponse(err))
		return
	}
	
	log.Info().Msgf("Order created successfully: %v", result)
	
	opts := []asynq.Option{
		asynq.ProcessIn(5 * time.Second),
		asynq.MaxRetry(3),
		asynq.Queue(worker.QueueCritical),
	}
	
	// Gửi thông báo cho người mua
	err = server.taskDistributor.DistributeTaskSendNotification(ctx.Request.Context(), &worker.PayloadSendNotification{
		RecipientID: result.Order.BuyerID,
		Title:       fmt.Sprintf("Đơn hàng mới #%s", result.Order.Code),
		Message:     fmt.Sprintf("Đơn hàng của bạn đã được tạo thành công với mã #%s. Tổng giá trị đơn hàng là %d VND.", result.Order.Code, result.Order.TotalAmount),
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
		Title:       fmt.Sprintf("Đơn hàng mới #%s", result.Order.Code),
		Message:     fmt.Sprintf("Bạn đã nhận được một đơn hàng mới với mã #%s. Tổng giá trị đơn hàng là %d VND.", result.Order.Code, result.Order.ItemsSubtotal),
		Type:        "order",
		ReferenceID: result.Order.Code,
	}, opts...)
	if err != nil {
		log.Err(err).Msg("failed to send notification to seller")
	}
	log.Info().Msgf("Notification sent to seller: %s", result.Order.SellerID)
	
	// Trả về kết quả cho client
	ctx.JSON(http.StatusCreated, result)
}

func (server *Server) listOrders(ctx *gin.Context) {
	// Lấy userID từ token xác thực
	userID := ctx.MustGet(authorizationPayloadKey).(*token.Payload).Subject
	
	// Thực hiện truy vấn để lấy danh sách đơn hàng
	orders, err := server.dbStore.ListOrdersByUserID(ctx, userID)
	if err != nil {
		log.Err(err).Msg("failed to list orders")
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	ctx.JSON(http.StatusOK, orders)
}
