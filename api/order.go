package api

import (
	"errors"
	"fmt"
	"net/http"
	"time"
	
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
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
	GundamIDs []int64 `json:"gundam_ids" binding:"required,dive,min=1"`
	
	// ID of the buyer's chosen address
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

//	@Summary		Create a new order
//	@Description	Create a new order for purchasing Gundam models
//	@Tags			orders
//	@Accept			json
//	@Produce		json
//	@Param			request	body		createOrderRequest		true	"Order details"
//	@Success		201		{object}	db.CreateOrderTxResult	"Order created successfully"
//	@Security		accessToken
//	@Router			/orders [post]
func (server *Server) createOrder(c *gin.Context) {
	userID := c.MustGet(authorizationPayloadKey).(*token.Payload).Subject
	
	_, err := server.dbStore.GetUserByID(c, userID)
	if err != nil {
		if errors.Is(err, db.ErrRecordNotFound) {
			err = fmt.Errorf("authenticated user ID %s not found", userID)
			c.JSON(http.StatusNotFound, errorResponse(err))
			return
		}
		
		log.Err(err).Msg("failed to get user by ID")
		c.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	var req createOrderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse(err))
		return
	}
	
	_, err = server.dbStore.GetUserByID(c, req.SellerID)
	if err != nil {
		if errors.Is(err, db.ErrRecordNotFound) {
			err = fmt.Errorf("seller ID %s not found", userID)
			c.JSON(http.StatusNotFound, errorResponse(err))
			return
		}
		
		log.Err(err).Msg("failed to get seller by ID")
		c.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	// Tính toán tổng giá trị thực tế của các sản phẩm
	actualItemsSubtotal := int64(0)
	actualTotalAmount := int64(0)
	gundams := make([]db.Gundam, len(req.GundamIDs))
	
	// Duyệt qua từng gundam trong danh sách để kiểm tra tính hợp lệ
	for i, gundamID := range req.GundamIDs {
		gundam, err := server.dbStore.GetGundamByID(c.Request.Context(), gundamID)
		if err != nil {
			if errors.Is(err, db.ErrRecordNotFound) {
				err = fmt.Errorf("gundam ID %d not found", gundamID)
				c.JSON(http.StatusNotFound, errorResponse(err))
				return
			}
			
			log.Err(err).Msg("failed to get gundam by ID")
			c.JSON(http.StatusInternalServerError, errorResponse(err))
			return
		}
		
		// Kiểm tra người sở hữu
		if gundam.OwnerID != req.SellerID {
			err = fmt.Errorf("gundam ID %d does not belong to seller ID %s", gundamID, req.SellerID)
			c.JSON(http.StatusUnprocessableEntity, errorResponse(err))
			return
		}
		
		// Kiểm tra trạng thái gundam
		if gundam.Status != db.GundamStatusPublished {
			err = fmt.Errorf("gundam ID %d is not in published status", gundamID)
			c.JSON(http.StatusUnprocessableEntity, errorResponse(err))
			return
		}
		
		// Tránh seller mua sản phẩm của chính mình
		if gundam.OwnerID == userID {
			err = fmt.Errorf("seller ID %s cannot buy their own gundam ID %d", userID, gundamID)
			c.JSON(http.StatusUnprocessableEntity, errorResponse(err))
			return
		}
		
		actualItemsSubtotal += gundam.Price
		gundams[i] = gundam
	}
	
	// Kiểm tra xem tổng giá trị sản phẩm có khớp với tổng giá trị thực tế không
	if req.ItemsSubtotal != actualItemsSubtotal {
		err = fmt.Errorf("items subtotal mismatch: expected %d, got %d", actualItemsSubtotal, req.ItemsSubtotal)
		c.JSON(http.StatusUnprocessableEntity, errorResponse(err))
		return
	}
	
	// Kiểm tra xem tổng giá trị đơn hàng có hợp lệ không
	actualTotalAmount = actualItemsSubtotal + req.DeliveryFee
	if req.TotalAmount != actualTotalAmount {
		err = fmt.Errorf("total amount mismatch: expected %d, got %d", actualTotalAmount, req.TotalAmount)
		c.JSON(http.StatusUnprocessableEntity, errorResponse(err))
		return
	}
	
	// Lấy thông tin địa chỉ người mua
	buyerAddress, err := server.dbStore.GetUserAddressByID(c, db.GetUserAddressByIDParams{
		ID:     req.BuyerAddressID,
		UserID: userID,
	})
	if err != nil {
		if errors.Is(err, db.ErrRecordNotFound) {
			err = fmt.Errorf("cannot find user address with ID %d for buyer with ID %s", req.BuyerAddressID, userID)
			c.JSON(http.StatusUnprocessableEntity, errorResponse(err))
			return
		}
		
		log.Err(err).Msg("failed to get user address by ID")
		c.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	// Lấy thông tin địa chỉ lấy hàng của người bán
	sellerAddress, err := server.dbStore.GetUserPickupAddress(c, req.SellerID)
	if err != nil {
		if errors.Is(err, db.ErrRecordNotFound) {
			err = fmt.Errorf("seller pickup address not found for seller ID %s", req.SellerID)
			c.JSON(http.StatusUnprocessableEntity, errorResponse(err))
			return
		}
		
		log.Err(err).Msg("failed to get user pickup address")
		c.JSON(http.StatusInternalServerError, errorResponse(err))
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
		Note:                 req.Note,
		Gundams:              gundams,
	}
	
	// Thực hiện transaction tạo đơn hàng
	result, err := server.dbStore.CreateOrderTx(c, arg)
	if err != nil {
		log.Err(err).Msg("failed to create order")
		c.JSON(http.StatusUnprocessableEntity, errorResponse(err))
		return
	}
	
	log.Info().Msgf("Order created successfully: %v", result)
	
	opts := []asynq.Option{
		asynq.MaxRetry(3),
		asynq.Queue(worker.QueueCritical),
	}
	
	// Gửi thông báo cho người mua
	err = server.taskDistributor.DistributeTaskSendNotification(c.Request.Context(), &worker.PayloadSendNotification{
		RecipientID: result.Order.BuyerID,
		Title:       fmt.Sprintf("Đơn hàng %s đã được tạo thành công", result.Order.Code),
		Message:     fmt.Sprintf("Đơn hàng %s đã được tạo thành công với tổng giá trị %s. Người bán sẽ xác nhận đơn hàng của bạn trong thời gian sớm nhất. Bạn có thể theo dõi trạng thái đơn hàng trong trang Đơn Hàng.", result.Order.Code, util.FormatVND(result.Order.TotalAmount)),
		Type:        "order",
		ReferenceID: result.Order.Code,
	}, opts...)
	if err != nil {
		log.Err(err).Msg("failed to send notification to buyer")
	}
	log.Info().Msgf("Notification sent to buyer: %s", result.Order.BuyerID)
	
	// Gửi thông báo cho người bán
	err = server.taskDistributor.DistributeTaskSendNotification(c.Request.Context(), &worker.PayloadSendNotification{
		RecipientID: result.Order.SellerID,
		Title:       fmt.Sprintf("Đơn hàng mới %s cần xác nhận", result.Order.Code),
		Message:     fmt.Sprintf("Bạn có đơn hàng mới %s với giá trị %s. Vui lòng xác nhận đơn hàng trong thời gian sớm nhất để chuẩn bị giao cho đơn vị vận chuyển.", result.Order.Code, util.FormatVND(result.Order.ItemsSubtotal)),
		Type:        "order",
		ReferenceID: result.Order.Code,
	}, opts...)
	if err != nil {
		log.Err(err).Msg("failed to send notification to seller")
	}
	log.Info().Msgf("Notification sent to seller: %s", result.Order.SellerID)
	
	c.JSON(http.StatusCreated, result)
}

type listMemberOrdersRequest struct {
	Status *string `form:"status"`
}

func (req *listMemberOrdersRequest) getStatus() string {
	if req.Status != nil {
		return *req.Status
	}
	
	return ""
}

func (req *listMemberOrdersRequest) validate() error {
	if req.Status != nil {
		if err := db.IsValidOrderStatus(*req.Status); err != nil {
			return err
		}
	}
	
	return nil
}

//	@Summary		List all orders of a member
//	@Description	List all orders of a member with optional filtering by order status
//	@Tags			orders
//	@Accept			json
//	@Produce		json
//	@Security		accessToken
//	@Param			status	query	string				false	"Filter by order status"	Enums(pending, packaging, delivering, delivered, completed, canceled, failed)
//	@Success		200		array	db.MemberOrderInfo	"List of orders"
//	@Router			/orders [get]
func (server *Server) listMemberOrders(ctx *gin.Context) {
	userID := ctx.MustGet(authorizationPayloadKey).(*token.Payload).Subject
	
	_, err := server.dbStore.GetUserByID(ctx, userID)
	if err != nil {
		if errors.Is(err, db.ErrRecordNotFound) {
			err = fmt.Errorf("authenticated user ID %s not found", userID)
			ctx.JSON(http.StatusNotFound, errorResponse(err))
			return
		}
		
		log.Err(err).Msg("failed to get user by ID")
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	var req listMemberOrdersRequest
	if err := ctx.ShouldBindQuery(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, errorResponse(err))
		return
	}
	
	// Kiểm tra tính hợp lệ của tham số trạng thái
	if err := req.validate(); err != nil {
		ctx.JSON(http.StatusBadRequest, errorResponse(err))
		return
	}
	
	// Thực hiện truy vấn để lấy danh sách đơn hàng
	orders, err := server.dbStore.ListMemberOrders(ctx, db.ListMemberOrdersParams{
		BuyerID: userID,
		Status: db.NullOrderStatus{
			OrderStatus: db.OrderStatus(req.getStatus()),
			Valid:       req.Status != nil,
		},
	})
	if err != nil {
		log.Err(err).Msg("failed to list orders for member")
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	resp := make([]db.MemberOrderInfo, 0, len(orders))
	
	for _, order := range orders {
		var orderInfo db.MemberOrderInfo
		orderItems, err := server.dbStore.ListOrderItems(ctx, order.ID)
		if err != nil {
			log.Err(err).Msg("failed to get order items")
			ctx.JSON(http.StatusInternalServerError, errorResponse(err))
			return
		}
		
		orderInfo.Order = order
		orderInfo.OrderItems = orderItems
		resp = append(resp, orderInfo)
	}
	
	ctx.JSON(http.StatusOK, resp)
}

//	@Summary		Confirm order received
//	@Description	Confirm that the buyer has received the order. For regular orders, it completes the transaction and transfers payment to seller. For exchange orders, it updates exchange status and may complete the exchange if both parties have confirmed.
//	@Tags			orders
//	@Produce		json
//	@Param			orderID	path		string									true	"Order ID"	example(123e4567-e89b-12d3-a456-426614174000)
//	@Success		200		{object}	db.ConfirmOrderReceivedByBuyerTxResult	"Order received successfully"
//	@Security		accessToken
//	@Router			/orders/{orderID}/received [patch]
func (server *Server) confirmOrderReceived(ctx *gin.Context) {
	authPayload := ctx.MustGet(authorizationPayloadKey).(*token.Payload)
	userID := authPayload.Subject
	
	// Xác thực người dùng
	_, err := server.dbStore.GetUserByID(ctx, userID)
	if err != nil {
		if errors.Is(err, db.ErrRecordNotFound) {
			err = fmt.Errorf("authenticated user ID %s not found", userID)
			ctx.JSON(http.StatusNotFound, errorResponse(err))
			return
		}
		
		log.Err(err).Msg("failed to get user by ID")
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
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
	if order.BuyerID != userID {
		err = fmt.Errorf("order %s does not belong to user %s", order.Code, userID)
		ctx.JSON(http.StatusForbidden, errorResponse(err))
		return
	}
	
	// Kiểm tra trạng thái đơn hàng
	if order.Status != db.OrderStatusDelivered {
		err = fmt.Errorf("order %s is not in delivered status", order.Code)
		ctx.JSON(http.StatusUnprocessableEntity, errorResponse(err))
		return
	}
	
	// Xử lý dựa trên loại đơn hàng
	switch order.Type {
	case db.OrderTypeRegular:
		// Xử lý đơn hàng thông thường
		result, err := server.handleRegularOrderConfirmation(ctx, order)
		if err != nil {
			log.Err(err).Msg("failed to confirm regular order received")
			ctx.JSON(http.StatusInternalServerError, errorResponse(err))
			return
		}
		ctx.JSON(http.StatusOK, result)
	
	case db.OrderTypeExchange:
		// Xử lý đơn hàng trao đổi
		result, err := server.handleExchangeOrderConfirmation(ctx, order)
		if err != nil {
			log.Err(err).Msg("failed to confirm exchange order received")
			ctx.JSON(http.StatusInternalServerError, errorResponse(err))
			return
		}
		ctx.JSON(http.StatusOK, result)
	
	default:
		err = fmt.Errorf("unsupported order type: %s", order.Type)
		ctx.JSON(http.StatusUnprocessableEntity, errorResponse(err))
	}
}

//	@Summary		Get order details for a member
//	@Description	Get details of a specific order for a member
//	@Tags			orders
//	@Accept			json
//	@Produce		json
//	@Param			orderID	path		string					true	"Order ID"	example(123e4567-e89b-12d3-a456-426614174000)
//	@Success		200		{object}	db.MemberOrderDetails	"Order details"
//	@Security		accessToken
//	@Router			/orders/{orderID} [get]
func (server *Server) getMemberOrderDetails(c *gin.Context) {
	authPayload := c.MustGet(authorizationPayloadKey).(*token.Payload)
	userID := authPayload.Subject
	
	_, err := server.dbStore.GetUserByID(c.Request.Context(), userID)
	if err != nil {
		if errors.Is(err, db.ErrRecordNotFound) {
			err = fmt.Errorf("authenticated user ID %s not found", userID)
			c.JSON(http.StatusNotFound, errorResponse(err))
			return
		}
		
		log.Err(err).Msg("failed to get user by ID")
		c.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	orderID, err := uuid.Parse(c.Param("orderID"))
	if err != nil {
		c.JSON(http.StatusBadRequest, errorResponse(err))
		return
	}
	
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
	
	// Kiểm tra quyền truy cập
	if order.BuyerID != userID && order.SellerID != userID {
		err = fmt.Errorf("order %s does not belong to user ID %s", order.Code, userID)
		c.JSON(http.StatusForbidden, errorResponse(err))
		return
	}
	
	var resp db.MemberOrderDetails
	resp.Order = order
	
	orderItems, err := server.dbStore.ListOrderItems(c.Request.Context(), order.ID)
	if err != nil {
		log.Err(err).Msg("failed to get order items")
		c.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	resp.OrderItems = orderItems
	
	orderDelivery, err := server.dbStore.GetOrderDelivery(c.Request.Context(), order.ID)
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
	
	// Lấy thông tin địa chỉ giao hàng của người nhận
	deliveryInformation, err := server.dbStore.GetDeliveryInformation(c.Request.Context(), orderDelivery.ToDeliveryID)
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
	resp.ToDeliveryInformation = deliveryInformation
	
	orderTransaction, err := server.dbStore.GetOrderTransactionByOrderID(c.Request.Context(), order.ID)
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
	
	// Xác định vai trò người dùng
	isSender := order.SellerID == userID
	isReceiver := order.BuyerID == userID
	
	// Nếu người dùng là người gửi đơn hàng
	if isSender {
		// Lấy thông tin người nhận đơn hàng
		buyer, err := server.dbStore.GetUserByID(c.Request.Context(), order.BuyerID)
		if err != nil {
			if errors.Is(err, db.ErrRecordNotFound) {
				err = fmt.Errorf("buyer ID %s not found", order.BuyerID)
				c.JSON(http.StatusNotFound, errorResponse(err))
				return
			}
			
			log.Err(err).Msg("failed to get buyer by ID")
			c.JSON(http.StatusInternalServerError, errorResponse(err))
			return
		}
		resp.BuyerInfo = &buyer
	}
	
	// Nếu người dùng là người nhận đơn hàng
	if isReceiver {
		// Lấy thông tin người gửi đơn hàng
		seller, err := server.dbStore.GetSellerDetailByID(c.Request.Context(), order.SellerID)
		if err != nil {
			if errors.Is(err, db.ErrRecordNotFound) {
				err = fmt.Errorf("seller ID %s not found", order.SellerID)
				c.JSON(http.StatusNotFound, errorResponse(err))
				return
			}
			
			log.Err(err).Msg("failed to get seller by ID")
			c.JSON(http.StatusInternalServerError, errorResponse(err))
			return
		}
		resp.SellerInfo = &db.SellerInfo{
			ID:              seller.User.ID,
			GoogleAccountID: seller.User.GoogleAccountID,
			UserFullName:    seller.User.FullName,
			ShopName:        seller.SellerProfile.ShopName,
			Email:           seller.User.Email,
			PhoneNumber:     seller.User.PhoneNumber,
			Role:            string(seller.User.Role),
			AvatarURL:       seller.User.AvatarURL,
		}
	}
	
	c.JSON(http.StatusOK, resp)
}

type cancelOrderByBuyerRequest struct {
	CanceledReason string `json:"canceled_reason" binding:"required"`
}

//	@Summary		Cancel order by buyer
//	@Description	Cancel an order by the buyer
//	@Tags			orders
//	@Accept			json
//	@Produce		json
//	@Param			orderID	path		string							true	"Order ID"	example(123e4567-e89b-12d3-a456-426614174000)
//	@Param			request	body		cancelOrderByBuyerRequest		true	"Cancellation reason"
//	@Success		200		{object}	db.CancelOrderByBuyerTxResult	"Order canceled successfully"
//	@Security		accessToken
//	@Router			/orders/{orderID}/cancel [patch]
func (server *Server) cancelOrderByBuyer(c *gin.Context) {
	authPayload := c.MustGet(authorizationPayloadKey).(*token.Payload)
	userID := authPayload.Subject
	
	_, err := server.dbStore.GetUserByID(c.Request.Context(), userID)
	if err != nil {
		if errors.Is(err, db.ErrRecordNotFound) {
			err = fmt.Errorf("authenticated user ID %s not found", userID)
			c.JSON(http.StatusNotFound, errorResponse(err))
			return
		}
		
		log.Err(err).Msg("failed to get user by ID")
		c.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	orderID, err := uuid.Parse(c.Param("orderID"))
	if err != nil {
		c.JSON(http.StatusBadRequest, errorResponse(err))
		return
	}
	
	var req cancelOrderByBuyerRequest
	if err = c.ShouldBindJSON(&req); err != nil {
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
	
	// Kiểm tra xem người mua có quyền hủy đơn hàng không
	if order.BuyerID != userID {
		err = fmt.Errorf("order %s does not belong to user ID %s", order.Code, userID)
		c.JSON(http.StatusForbidden, errorResponse(err))
		return
	}
	
	// Kiểm tra trạng thái đơn hàng
	// Chỉ cho phép hủy đơn hàng khi đơn hàng đang ở trạng thái "pending"
	if order.Status != db.OrderStatusPending {
		err = fmt.Errorf("order %s cannot be canceled in status %s", order.Code, order.Status)
		c.JSON(http.StatusUnprocessableEntity, errorResponse(err))
		return
	}
	
	result, err := server.dbStore.CancelOrderByBuyerTx(c.Request.Context(), db.CancelOrderByBuyerTxParams{
		Order:          &order,
		CanceledReason: req.CanceledReason,
	})
	if err != nil {
		log.Err(err).Msg("failed to cancel order by buyer")
		c.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	opts := []asynq.Option{
		asynq.MaxRetry(3),
		asynq.Queue(worker.QueueCritical),
	}
	
	// Gửi thông báo cho người mua
	err = server.taskDistributor.DistributeTaskSendNotification(c.Request.Context(), &worker.PayloadSendNotification{
		RecipientID: result.Order.BuyerID,
		Title:       "Đơn hàng của bạn đã được hủy",
		Message: fmt.Sprintf("Đơn hàng #%s đã được hủy thành công. Số tiền %s đã được hoàn trả vào ví của bạn.",
			result.Order.Code,
			util.FormatVND(result.OrderTransaction.Amount)),
		Type:        "order",
		ReferenceID: result.Order.Code,
	}, opts...)
	if err != nil {
		log.Err(err).Msg("failed to send notification to buyer")
	}
	log.Info().Msgf("Notification sent to buyer: %s", result.Order.BuyerID)
	
	// Gửi thông báo cho người bán
	err = server.taskDistributor.DistributeTaskSendNotification(c.Request.Context(), &worker.PayloadSendNotification{
		RecipientID: result.Order.SellerID,
		Title:       "Đơn hàng đã bị hủy bởi người mua",
		Message: fmt.Sprintf("Đơn hàng #%s (giá trị %s) đã bị hủy bởi người mua với lý do: \"%s\". Các sản phẩm đã được đưa về trạng thái có thể bán lại.",
			result.Order.Code,
			util.FormatVND(result.OrderTransaction.Amount),
			req.CanceledReason),
		Type:        "order",
		ReferenceID: result.Order.Code,
	}, opts...)
	if err != nil {
		log.Err(err).Msg("failed to send notification to seller")
	}
	log.Info().Msgf("Notification sent to seller: %s", result.Order.SellerID)
	
	c.JSON(http.StatusOK, result)
}
