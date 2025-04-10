package api

import (
	"errors"
	"fmt"
	"net/http"
	"time"
	
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgtype"
	db "github.com/katatrina/gundam-BE/internal/db/sqlc"
	"github.com/katatrina/gundam-BE/internal/token"
	"github.com/katatrina/gundam-BE/internal/util"
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
//	@Failure		404					{object}	gin.H					"Something not found"
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
			if errors.Is(err, db.ErrRecordNotFound) {
				ctx.JSON(http.StatusNotFound, gin.H{
					"error":   "Gundam not found",
					"details": fmt.Sprintf("Gundam ID %d not found", gundamID),
				})
				return
			}
			
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
			err = fmt.Errorf("cannot find buyer address with ID %d for user with ID %s", req.BuyerAddressID, userID)
			log.Err(err).Msg("buyer address not found")
			ctx.JSON(http.StatusUnprocessableEntity, errorResponse(err))
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
		Title:       fmt.Sprintf("Đơn hàng #%s đã được tạo thành công", result.Order.Code),
		Message:     fmt.Sprintf("Đơn hàng #%s đã được tạo thành công với tổng giá trị %d VND. Người bán sẽ xác nhận đơn hàng của bạn trong thời gian sớm nhất. Bạn có thể theo dõi trạng thái đơn hàng trong mục Đơn mua.", result.Order.Code, result.Order.TotalAmount),
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
		Title:       fmt.Sprintf("Đơn hàng mới #%s cần xác nhận", result.Order.Code),
		Message:     fmt.Sprintf("Bạn có đơn hàng mới #%s với giá trị %d VND. Vui lòng xác nhận đơn hàng trong thời gian sớm nhất để chuẩn bị giao cho đơn vị vận chuyển GHN.", result.Order.Code, result.Order.ItemsSubtotal),
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

type listPurchaseOrdersRequest struct {
	Status *string `form:"status"`
}

func (status *listPurchaseOrdersRequest) getStatus() string {
	if status.Status != nil {
		return *status.Status
	}
	
	return ""
}

type OrderInfo struct {
	Order      db.Order                       `json:"order"`
	OrderItems []db.GetGundamsByOrderItemsRow `json:"order_items"`
}

//	@Summary		List all purchase orders of the normal user
//	@Description	List all purchase orders of the normal user
//	@Tags			orders
//	@Accept			json
//	@Produce		json
//	@Security		accessToken
//	@Param			status	query	string		false	"Filter by order status"	Enums(pending, packaging, delivering, delivered, completed, canceled, failed)
//	@Success		200		array	OrderInfo	"Successfully retrieved list of purchase orders"
//	@Failure		400		"Bad request"
//	@Failure		500		"Internal server error"
//	@Router			/orders [get]
func (server *Server) listPurchaseOrders(ctx *gin.Context) {
	// Lấy userID từ token xác thực
	userID := ctx.MustGet(authorizationPayloadKey).(*token.Payload).Subject
	
	var req listPurchaseOrdersRequest
	var resp []OrderInfo
	if err := ctx.ShouldBindQuery(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, errorResponse(err))
		return
	}
	
	if req.Status != nil {
		if err := util.IsOrderStatusValid(req.getStatus()); err != nil {
			ctx.JSON(http.StatusBadRequest, errorResponse(err))
			return
		}
	}
	
	// Thực hiện truy vấn để lấy danh sách đơn hàng
	orders, err := server.dbStore.ListPurchaseOrders(ctx, db.ListPurchaseOrdersParams{
		BuyerID: userID,
		Status: db.NullOrderStatus{
			OrderStatus: db.OrderStatus(req.getStatus()),
			Valid:       req.Status != nil,
		},
	})
	if err != nil {
		log.Err(err).Msg("failed to list orders")
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	// Duyệt qua từng đơn hàng để lấy thông tin chi tiết
	for _, order := range orders {
		var orderDetail OrderInfo
		orderItems, err := server.dbStore.GetGundamsByOrderItems(ctx, order.ID.String())
		if err != nil {
			log.Err(err).Msg("failed to get order items")
			ctx.JSON(http.StatusInternalServerError, errorResponse(err))
			return
		}
		
		orderDetail.Order = order
		orderDetail.OrderItems = orderItems
		resp = append(resp, orderDetail)
	}
	
	ctx.JSON(http.StatusOK, resp)
}

//	@Summary		Confirm order received
//	@Description	Confirm that the buyer has received the order
//	@Tags			orders
//	@Accept			json
//	@Produce		json
//	@Param			orderID	path		string							true	"Order ID"	example(123e4567-e89b-12d3-a456-426614174000)
//	@Success		200		{object}	db.ConfirmOrderReceivedTxResult	"Order received successfully"
//	@Failure		400		"Bad request"
//	@Failure		404		"Order not found"
//	@Failure		403		"Forbidden - User does not have permission to confirm this order"
//	@Failure		422		"Unprocessable Entity - Order is not in delivered status"
//	@Failure		500		"Internal server error"
//	@Security		accessToken
//	@Router			/orders/{orderID}/received [post]
func (server *Server) confirmOrderReceived(ctx *gin.Context) {
	// Lấy buyerID từ token xác thực
	authPayload := ctx.MustGet(authorizationPayloadKey).(*token.Payload)
	buyerID := authPayload.Subject
	
	// Lấy orderID từ tham số URL
	orderID, err := uuid.Parse(ctx.Param("orderID"))
	if err != nil {
		ctx.JSON(http.StatusBadRequest, errorResponse(err))
		return
	}
	
	// Lấy thông tin đơn hàng
	order, err := server.dbStore.GetOrderByID(ctx, orderID)
	if err != nil {
		if errors.Is(err, db.ErrRecordNotFound) {
			err = fmt.Errorf("order ID %s not found", orderID)
			ctx.JSON(http.StatusNotFound, errorResponse(err))
			return
		}
		
		log.Err(err).Msg("failed to get order by ID")
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	// Kiểm tra xem người mua có quyền xác nhận đơn hàng không
	if order.BuyerID != buyerID {
		err = fmt.Errorf("order %s does not belong to user %s", order.Code, buyerID)
		ctx.JSON(http.StatusForbidden, errorResponse(err))
		return
	}
	
	// Kiểm tra trạng thái đơn hàng
	if order.Status != db.OrderStatusDelivered {
		err = fmt.Errorf("order %s is not in delivered status", order.Code)
		ctx.JSON(http.StatusUnprocessableEntity, errorResponse(err))
		return
	}
	
	// Lấy thông tin giao dịch đơn hàng
	orderTransaction, err := server.dbStore.GetOrderTransactionByOrderID(ctx, order.ID.String())
	if err != nil {
		if errors.Is(err, db.ErrRecordNotFound) {
			err = fmt.Errorf("transaction for order ID %s not found", orderID)
			ctx.JSON(http.StatusNotFound, errorResponse(err))
			return
		}
		
		log.Err(err).Msg("failed to get order transaction by order ID")
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	// Kiểm tra xem giao dịch đã có seller_entry_id chưa
	if !orderTransaction.SellerEntryID.Valid {
		err = fmt.Errorf("seller entry not found for order %s", order.Code)
		ctx.JSON(http.StatusUnprocessableEntity, errorResponse(err))
		return
	}
	
	// Lấy bút toán của người bán
	sellerEntry, err := server.dbStore.GetWalletEntryByID(ctx, orderTransaction.SellerEntryID.Int64)
	if err != nil {
		if errors.Is(err, db.ErrRecordNotFound) {
			err = fmt.Errorf("seller entry not found for order %s", order.Code)
			ctx.JSON(http.StatusNotFound, errorResponse(err))
			return
		}
		
		log.Err(err).Msg("failed to get wallet entry by ID")
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	// Lấy ví của người bán
	sellerWallet, err := server.dbStore.GetWalletForUpdate(ctx, order.SellerID)
	if err != nil {
		if errors.Is(err, db.ErrRecordNotFound) {
			err = fmt.Errorf("wallet not found for seller %s", order.SellerID)
			ctx.JSON(http.StatusNotFound, errorResponse(err))
			return
		}
		
		log.Err(err).Msg("failed to get wallet for update")
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	// Lấy thông tin order items
	orderItems, err := server.dbStore.GetOrderItems(ctx, order.ID.String())
	if err != nil {
		if errors.Is(err, db.ErrRecordNotFound) {
			err = fmt.Errorf("order items not found for order %s", order.Code)
			ctx.JSON(http.StatusNotFound, errorResponse(err))
			return
		}
		
		log.Err(err).Msg("failed to get order items")
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	// Lấy thông tin gundam
	var gundams []db.Gundam
	for _, item := range orderItems {
		gundam, err := server.dbStore.GetGundamByID(ctx, item.GundamID)
		if err != nil {
			if errors.Is(err, db.ErrRecordNotFound) {
				err = fmt.Errorf("gundam ID %d not found", item.GundamID)
				ctx.JSON(http.StatusNotFound, errorResponse(err))
				return
			}
			
			log.Err(err).Msg("failed to get gundam by ID")
			ctx.JSON(http.StatusInternalServerError, errorResponse(err))
			return
		}
		
		if gundam.Status != db.GundamStatusProcessing {
			err = fmt.Errorf("gundam ID %d is not in processing status", item.GundamID)
			ctx.JSON(http.StatusUnprocessableEntity, errorResponse(err))
			return
		}
		
		gundams = append(gundams, gundam)
	}
	
	// Thực hiện transaction xác nhận đơn hàng đã nhận
	result, err := server.dbStore.ConfirmOrderReceivedTx(ctx, db.ConfirmOrderReceivedTxParams{
		Order:        &order,
		OrderItems:   orderItems,
		SellerEntry:  &sellerEntry,
		SellerWallet: &sellerWallet,
	})
	if err != nil {
		log.Err(err).Msg("failed to confirm order received")
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	// TODO: Hủy task "tự động xác nhận đơn hàng sau 7 ngày" vì người mua đã xác nhận đơn hàng
	
	// Gửi thông báo cho người mua và người bán
	opts := []asynq.Option{
		asynq.MaxRetry(3),
		asynq.Queue(worker.QueueCritical),
	}
	
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
	
	ctx.JSON(http.StatusOK, result)
}

type OrderDetail struct {
	Order            db.Order                       `json:"order"`
	OrderItems       []db.GetGundamsByOrderItemsRow `json:"order_items"`
	OrderDelivery    db.OrderDelivery               `json:"order_delivery"`
	DeliveryAddress  db.DeliveryInformation         `json:"to_delivery_address"`
	SellerInfo       db.User                        `json:"seller_info"`
	OrderTransaction db.OrderTransaction            `json:"order_transaction"`
}

//	@Summary		Get order details
//	@Description	Get details of a specific order
//	@Tags			orders
//	@Accept			json
//	@Produce		json
//	@Param			orderID	path		string		true	"Order ID"	example(123e4567-e89b-12d3-a456-426614174000)
//	@Success		200		{object}	OrderDetail	"Successfully retrieved order details"
//	@Failure		400		"Bad request"
//	@Failure		404		"Not Found - Order not found"
//	@Failure		403		"Forbidden - User does not have permission to access this order"
//	@Failure		500		"Internal server error"
//	@Security		accessToken
//	@Router			/orders/{orderID} [get]
func (server *Server) getOrderDetails(c *gin.Context) {
	// Lấy userID từ token xác thực
	authPayload := c.MustGet(authorizationPayloadKey).(*token.Payload)
	userID := authPayload.Subject
	
	var resp OrderDetail
	
	// Lấy orderID từ tham số URL
	orderID, err := uuid.Parse(c.Param("orderID"))
	if err != nil {
		c.JSON(http.StatusBadRequest, errorResponse(err))
		return
	}
	
	// Lấy thông tin đơn hàng
	order, err := server.dbStore.GetOrderByID(c.Request.Context(), orderID)
	if err != nil {
		if errors.Is(err, db.ErrRecordNotFound) {
			err = fmt.Errorf("order ID %s not found", orderID)
			c.JSON(http.StatusNotFound, errorResponse(err))
			return
		}
		
		log.Err(err).Msg("failed to get order by ID")
		c.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	// Kiểm tra xem người dùng có quyền truy cập đơn hàng không
	if userID != order.BuyerID {
		err = fmt.Errorf("orderID %s does not belong to user %s", order.ID, userID)
		c.JSON(http.StatusForbidden, errorResponse(err))
		return
	}
	
	resp.Order = order
	
	orderItems, err := server.dbStore.GetGundamsByOrderItems(c.Request.Context(), order.ID.String())
	if err != nil {
		log.Err(err).Msg("failed to get order items")
		c.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	resp.OrderItems = orderItems
	
	orderDelivery, err := server.dbStore.GetOrderDelivery(c.Request.Context(), order.ID.String())
	if err != nil {
		if errors.Is(err, db.ErrRecordNotFound) {
			err = fmt.Errorf("order delivery not found for order ID %s", order.ID)
			c.JSON(http.StatusNotFound, errorResponse(err))
			return
		}
		
		log.Err(err).Msg("failed to get order delivery")
		c.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	resp.OrderDelivery = orderDelivery
	
	sellerInfo, err := server.dbStore.GetUserByID(c.Request.Context(), order.SellerID)
	if err != nil {
		if errors.Is(err, db.ErrRecordNotFound) {
			err = fmt.Errorf("seller ID %s not found", order.SellerID)
			c.JSON(http.StatusNotFound, errorResponse(err))
			return
		}
		
		log.Err(err).Msg("failed to get seller info")
		c.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	resp.SellerInfo = sellerInfo
	
	orderTransaction, err := server.dbStore.GetOrderTransactionByOrderID(c.Request.Context(), order.ID.String())
	if err != nil {
		if errors.Is(err, db.ErrRecordNotFound) {
			err = fmt.Errorf("order transaction not found for order ID %s", order.ID)
			c.JSON(http.StatusNotFound, errorResponse(err))
			return
		}
		
		log.Err(err).Msg("failed to get order transaction")
		c.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	resp.OrderTransaction = orderTransaction
	
	// Lấy thông tin địa chỉ giao hàng
	deliveryAddress, err := server.dbStore.GetDeliveryInformation(c.Request.Context(), orderDelivery.ToDeliveryID)
	if err != nil {
		if errors.Is(err, db.ErrRecordNotFound) {
			err = fmt.Errorf("delivery address ID %d not found", orderDelivery.ToDeliveryID)
			c.JSON(http.StatusNotFound, errorResponse(err))
			return
		}
		
		log.Err(err).Msg("failed to get delivery address")
		c.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	resp.DeliveryAddress = deliveryAddress
	
	c.JSON(http.StatusOK, resp)
}
