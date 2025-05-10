package api

import (
	"errors"
	"fmt"
	"mime/multipart"
	"net/http"
	"strconv"
	"time"
	
	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	db "github.com/katatrina/gundam-BE/internal/db/sqlc"
	"github.com/katatrina/gundam-BE/internal/token"
	"github.com/katatrina/gundam-BE/internal/util"
	"github.com/katatrina/gundam-BE/internal/worker"
	"github.com/rs/zerolog/log"
)

//	@Summary		Become a seller
//	@Description	Upgrade the user's role to seller and create the trial subscription
//	@Tags			users
//	@Accept			json
//	@Produce		json
//	@Security		accessToken
//	@Success		200	{object}	db.User	"Successfully became seller"
//	@Failure		404	"User not found"
//	@Failure		409	"User is already a seller"
//	@Failure		500	"Internal server error"
//	@Router			/users/become-seller [post]
func (server *Server) becomeSeller(ctx *gin.Context) {
	userID := ctx.MustGet(authorizationPayloadKey).(*token.Payload).Subject
	
	user, err := server.dbStore.GetUserByID(ctx, userID)
	if err != nil {
		if errors.Is(err, db.ErrRecordNotFound) {
			err = fmt.Errorf("user ID %s not found", userID)
			ctx.JSON(http.StatusNotFound, errorResponse(err))
			return
		}
		
		log.Error().Err(err).Msg("Failed to get user")
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	if user.Role == db.UserRoleSeller {
		ctx.JSON(http.StatusConflict, gin.H{"error": "user is already a seller"})
		return
	}
	
	seller, err := server.dbStore.BecomeSellerTx(ctx, userID)
	if err != nil {
		log.Error().Err(err).Msg("Failed to become seller")
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	ctx.JSON(http.StatusOK, seller)
}

type getSellerProfileQueryString struct {
	UserID string `form:"user_id" binding:"required"`
}

//	@Summary		Get seller profile
//	@Description	Get detailed information about a specific seller
//	@Tags			seller profile
//	@Accept			json
//	@Produce		json
//	@Param			user_id	query		string			true	"User ID"
//	@Success		200		{object}	db.SellerInfo	"Seller profile details"
//	@Failure		404		"User not found"
//	@Failure		500		"Internal server error"
//	@Router			/seller/profile [get]
func (server *Server) getSellerProfile(c *gin.Context) {
	var req getSellerProfileQueryString
	if err := c.ShouldBindQuery(&req); err != nil {
		log.Error().Err(err).Msg("Failed to bind query string")
		c.JSON(http.StatusBadRequest, errorResponse(err))
		return
	}
	
	row, err := server.dbStore.GetSellerDetailByID(c.Request.Context(), req.UserID)
	if err != nil {
		if errors.Is(err, db.ErrRecordNotFound) {
			err = fmt.Errorf("user ID %s not found", req.UserID)
			c.JSON(http.StatusNotFound, errorResponse(err))
			return
		}
		
		log.Error().Err(err).Msg("failed to get seller profile")
		c.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	resp := db.SellerInfo{
		ID:              row.User.ID,
		GoogleAccountID: row.User.GoogleAccountID,
		UserFullName:    row.User.FullName,
		ShopName:        row.SellerProfile.ShopName,
		Email:           row.User.Email,
		PhoneNumber:     row.User.PhoneNumber,
		Role:            string(row.User.Role),
		AvatarURL:       row.User.AvatarURL,
	}
	
	c.JSON(http.StatusOK, resp)
}

//	@Summary		Get current active subscription
//	@Description	Get the current active subscription for the specified seller
//	@Tags			sellers
//	@Produce		json
//	@Param			sellerID	path	string	true	"Seller ID"
//	@Security		accessToken
//	@Success		200	"Successfully retrieved current active subscription"
//	@Failure		404	"Subscription not found"
//	@Failure		500	"Internal server error"
//	@Router			/sellers/:sellerID/subscriptions/active [get]
func (server *Server) getCurrentActiveSubscription(c *gin.Context) {
	seller := c.MustGet(sellerPayloadKey).(*db.User)
	
	subscription, err := server.dbStore.GetCurrentActiveSubscriptionDetailsForSeller(c, seller.ID)
	if err != nil {
		if errors.Is(err, db.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "no active subscription found"})
			return
		}
		
		log.Error().Err(err).Msg("Failed to get current active subscription")
		c.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	c.JSON(http.StatusOK, subscription)
}

//	@Summary		Publish a gundam for sale
//	@Description	Publish a gundam for sale for the specified seller. This endpoint checks the gundam's status before proceeding.
//	@Tags			sellers
//	@Accept			json
//	@Produce		json
//	@Param			gundamID	path	int64	true	"Gundam ID"
//	@Param			sellerID	path	string	true	"Seller ID"
//	@Security		accessToken
//	@Success		200	{object}	map[string]interface{}	"Successfully published gundam"
//	@Router			/sellers/{sellerID}/gundams/{gundamID}/publish [patch]
func (server *Server) publishGundam(c *gin.Context) {
	seller := c.MustGet(sellerPayloadKey).(*db.User)
	
	gundamID, err := strconv.ParseInt(c.Param("gundamID"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid gundam ID`"})
		return
	}
	
	gundam, err := server.dbStore.GetGundamByID(c, gundamID)
	if err != nil {
		if errors.Is(err, db.ErrRecordNotFound) {
			err = fmt.Errorf("gundam ID %d not found", gundamID)
			c.JSON(http.StatusNotFound, errorResponse(err))
			return
		}
		
		log.Error().Err(err).Msg("Failed to get gundam")
		c.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	// Kiểm tra quyền sở hữu và trạng thái Gundam
	if gundam.OwnerID != seller.ID {
		err = fmt.Errorf("gundam ID %d does not belong to seller ID %s", gundam.ID, seller.ID)
		c.JSON(http.StatusForbidden, errorResponse(err))
		return
	}
	
	if gundam.Status != db.GundamStatusInstore {
		err = fmt.Errorf("gundam ID %d is not in store", gundam.ID)
		c.JSON(http.StatusConflict, errorResponse(err))
		return
	}
	
	if gundam.Price == nil {
		err = fmt.Errorf("gundam ID %d does not have a price", gundam.ID)
		c.JSON(http.StatusConflict, errorResponse(err))
		return
	}
	
	userSubscription, err := server.dbStore.GetCurrentActiveSubscriptionDetailsForSeller(c, seller.ID)
	if err != nil {
		if errors.Is(err, db.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "no active subscription found"})
			return
		}
		
		log.Error().Err(err).Msg("Failed to get current active subscription")
		c.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	// Kiểm tra nếu gói không phải Gói Dùng Thử và đã hết hạn
	if userSubscription.EndDate != nil &&
		userSubscription.EndDate.Before(time.Now()) &&
		userSubscription.SubscriptionName != db.TrialSellerSubscriptionName {
		c.JSON(http.StatusConflict, errorResponse(db.ErrSubscriptionExpired))
		return
	}
	
	// Kiểm tra nếu gói không phải gói "Không Giới Hạn" và đã vượt quá số lượt bán
	if !userSubscription.IsUnlimited &&
		(userSubscription.MaxListings != nil) &&
		userSubscription.ListingsUsed >= *userSubscription.MaxListings {
		c.JSON(http.StatusConflict, errorResponse(db.ErrSubscriptionLimitExceeded))
		return
	}
	
	arg := db.PublishGundamTxParams{
		GundamID:             gundam.ID,
		SellerID:             seller.ID,
		ActiveSubscriptionID: userSubscription.ID,
		ListingsUsed:         userSubscription.ListingsUsed + 1,
	}
	err = server.dbStore.PublishGundamTx(c, arg)
	if err != nil {
		log.Error().Err(err).Msg("Failed to publish gundam")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to start publishing process"})
		return
	}
	
	// Phản hồi thành công với thông tin chi tiết sản phẩm
	c.JSON(http.StatusOK, gin.H{
		"message":   "gundam is now listed for sale",
		"gundam_id": gundam.ID,
		"status":    db.GundamStatusPublished,
	})
}

//	@Summary		Unpublish a gundam
//	@Description	Unpublish a gundam for the specified seller. This endpoint checks the gundam's status before proceeding.
//	@Tags			sellers
//	@Accept			json
//	@Produce		json
//	@Param			gundamID	path	int64	true	"Gundam ID"
//	@Param			sellerID	path	string	true	"Seller ID"
//	@Security		accessToken
//	@Success		200	{object}	map[string]interface{}	"Successfully unsold gundam with details"
//	@Failure		400	{object}	map[string]string		"Invalid gundam ID"
//	@Failure		403	{object}	map[string]string		"Seller does not own this gundam"
//	@Failure		404	{object}	map[string]string		"Seller not found<br/>Gundam not found"
//	@Failure		409	{object}	map[string]string		"Gundam is not currently listed for sale"
//	@Failure		500	{object}	map[string]string		"Internal server error"
//	@Router			/sellers/{sellerID}/gundams/{gundamID}/unpublish [patch]
func (server *Server) unpublishGundam(c *gin.Context) {
	seller := c.MustGet(sellerPayloadKey).(*db.User)
	
	gundamID, err := strconv.ParseInt(c.Param("gundamID"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid gundam ID"})
		return
	}
	
	gundam, err := server.dbStore.GetGundamByID(c, gundamID)
	if err != nil {
		if errors.Is(err, db.ErrRecordNotFound) {
			err = fmt.Errorf("gundam ID %d not found", gundamID)
			c.JSON(http.StatusNotFound, errorResponse(err))
			return
		}
		
		log.Error().Err(err).Msg("Failed to get gundam")
		c.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	if gundam.OwnerID != seller.ID {
		err = fmt.Errorf("gundam ID %d does not belong to seller ID %s", gundam.ID, seller.ID)
		c.JSON(http.StatusForbidden, errorResponse(err))
		return
	}
	if gundam.Status != db.GundamStatusPublished {
		err = fmt.Errorf("gundam ID %d is not currently listed for sale", gundam.ID)
		c.JSON(http.StatusConflict, errorResponse(err))
		return
	}
	
	err = server.dbStore.UnpublishGundamTx(c, db.UnpublishGundamTxParams{
		GundamID: gundam.ID,
		SellerID: gundam.OwnerID,
	})
	if err != nil {
		log.Error().Err(err).Msg("Failed to unpublish gundam")
		c.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	c.JSON(http.StatusOK, gin.H{
		"message":   "gundam is no longer being published",
		"gundam_id": gundam.ID,
	})
}

//	@Summary		List all sales orders (excluding exchange orders) for a specific seller
//	@Description	Get all sales orders that belong to the specified seller ID
//	@Tags			sellers
//	@Accept			json
//	@Produce		json
//	@Security		accessToken
//	@Param			sellerID	path	string				true	"Seller ID"
//	@Param			status		query	string				false	"Filter by order status"	Enums(pending, packaging, delivering, delivered, completed, canceled, failed)
//	@Success		200			array	db.SalesOrderInfo	"List of sales orders"
//	@Router			/sellers/:sellerID/orders [get]
func (server *Server) listSalesOrders(c *gin.Context) {
	user := c.MustGet(sellerPayloadKey).(*db.User)
	status := c.Query("status")
	
	if status != "" {
		if err := db.IsValidOrderStatus(status); err != nil {
			log.Error().Err(err).Msg("Invalid order status")
			c.JSON(http.StatusBadRequest, errorResponse(err))
			return
		}
	}
	
	var resp []db.SalesOrderInfo
	
	arg := db.ListSalesOrdersParams{
		SellerID: user.ID,
		Status: db.NullOrderStatus{
			OrderStatus: db.OrderStatus(status),
			Valid:       status != "",
		},
	}
	
	orders, err := server.dbStore.ListSalesOrders(c, arg)
	if err != nil {
		log.Error().Err(err).Msg("Failed to list orders by user")
		c.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	for _, order := range orders {
		var orderInfo db.SalesOrderInfo
		orderItems, err := server.dbStore.ListOrderItems(c, order.ID)
		if err != nil {
			log.Err(err).Msg("failed to get order items")
			c.JSON(http.StatusInternalServerError, errorResponse(err))
			return
		}
		
		orderInfo.Order = order
		orderInfo.OrderItems = orderItems
		resp = append(resp, orderInfo)
	}
	
	c.JSON(http.StatusOK, resp)
}

type confirmOrderRequestParams struct {
	SellerID string `uri:"sellerID" binding:"required"`
	OrderID  string `uri:"orderID" binding:"required"`
}

//	@Summary		Confirm an order
//	@Description	Confirm an order for the specified seller. This endpoint checks the order's status before proceeding.
//	@Tags			sellers
//	@Accept			json
//	@Produce		json
//	@Param			sellerID	path	string	true	"Seller ID"
//	@Param			orderID		path	string	true	"Order ID"
//	@Security		accessToken
//	@Success		200	{object}	db.ConfirmOrderTxResult	"Successfully confirmed order"
//	@Router			/sellers/:sellerID/orders/:orderID/confirm [patch]
func (server *Server) confirmOrder(c *gin.Context) {
	user := c.MustGet(sellerPayloadKey).(*db.User)
	
	orderID, err := uuid.Parse(c.Param("orderID"))
	if err != nil {
		log.Error().Err(err).Msg("Failed to parse order ID")
		c.JSON(http.StatusBadRequest, errorResponse(err))
		return
	}
	
	order, err := server.dbStore.GetSalesOrder(c.Request.Context(), db.GetSalesOrderParams{
		OrderID:  orderID,
		SellerID: user.ID,
	})
	if err != nil {
		if errors.Is(err, db.ErrRecordNotFound) {
			err = fmt.Errorf("order ID %s not found", orderID)
			c.JSON(http.StatusNotFound, errorResponse(err))
			return
		}
		
		log.Error().Err(err).Msg("Failed to get order")
		c.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	// Kiểm tra quyền sở hữu đơn hàng
	if order.SellerID != user.ID {
		err = fmt.Errorf("order %s does not belong to seller ID %s", order.Code, user.ID)
		c.JSON(http.StatusForbidden, errorResponse(err))
		return
	}
	
	// Kiểm tra trạng thái đơn hàng
	if order.Status != db.OrderStatusPending {
		err = fmt.Errorf("order %s is not in pending status", order.Code)
		c.JSON(http.StatusConflict, errorResponse(err))
		return
	}
	
	result, err := server.dbStore.ConfirmOrderBySellerTx(c, db.ConfirmOrderTxParams{
		Order:    &order,
		SellerID: user.ID,
	})
	if err != nil {
		log.Error().Err(err).Msg("failed to confirm order")
		c.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	log.Info().Msgf("Order confirmed: %s", result.Order.Code)
	
	// Thông báo cho người dùng về việc đơn hàng đã được xác nhận thành công
	opts := []asynq.Option{
		asynq.MaxRetry(3),
		asynq.Queue(worker.QueueCritical),
	}
	
	// Gửi thông báo cho người mua
	err = server.taskDistributor.DistributeTaskSendNotification(c.Request.Context(), &worker.PayloadSendNotification{
		RecipientID: result.Order.BuyerID,
		Title:       fmt.Sprintf("Đơn hàng #%s đã được xác nhận", result.Order.Code),
		Message:     fmt.Sprintf("Đơn hàng #%s của bạn đã được người bán xác nhận và đang được chuẩn bị. Chúng tôi sẽ thông báo khi đơn hàng được giao cho đơn vị vận chuyển."),
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
		Title:       fmt.Sprintf("Đã xác nhận đơn hàng #%s", result.Order.Code),
		Message:     fmt.Sprintf("Bạn đã xác nhận đơn hàng #%s. Tổng tiền hàng %s đã được chuyển vào số dư tạm thời của bạn. Số tiền này sẽ được chuyển vào số dư khả dụng sau khi người mua xác nhận đã nhận hàng thành công.", result.Order.Code, util.FormatVND(result.SellerEntry.Amount)),
		Type:        "order",
		ReferenceID: result.Order.Code,
	}, opts...)
	if err != nil {
		log.Err(err).Msg("failed to send notification to seller")
	}
	log.Info().Msgf("Notification sent to seller: %s", user.ID)
	
	c.JSON(http.StatusOK, result)
}

type packageOrderRequestBody struct {
	PackageImages []*multipart.FileHeader `form:"package_images" binding:"required"`
	// Lược bỏ package weight và package size (length, width, height)
}

//	@Summary		Package an order for delivery
//	@Description	Upload package images, create a delivery order, and update order status for a specified order.
//	@Description	Handles packaging for regular orders, exchange orders, and auction orders (future).
//	@Tags			orders
//	@Accept			multipart/form-data
//	@Produce		json
//	@Param			orderID			path		string					true	"Order ID in UUID format"
//	@Param			package_images	formData	file					true	"Package images (at least one image required)"
//	@Success		200				{object}	db.PackageOrderTxResult	"Successfully packaged order with delivery details"
//	@Security		accessToken
//	@Router			/orders/{orderID}/package [patch]
func (server *Server) packageOrder(c *gin.Context) {
	authPayload := c.MustGet(authorizationPayloadKey).(*token.Payload)
	userID := authPayload.Subject
	
	var req packageOrderRequestBody
	if err := c.ShouldBindWith(&req, binding.FormMultipart); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse(err))
		return
	}
	
	orderID, err := uuid.Parse(c.Param("orderID"))
	if err != nil {
		c.JSON(http.StatusBadRequest, errorResponse(err))
		return
	}
	
	if len(req.PackageImages) == 0 {
		err = errors.New("at least one package image is required")
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
		
		log.Error().Err(err).Msg("Failed to get order")
		c.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	// Kiểm tra quyền sở hữu đơn hàng
	if order.SellerID != userID {
		err = fmt.Errorf("order %s does not belong to user ID %s", order.Code, userID)
		c.JSON(http.StatusForbidden, errorResponse(err))
		return
	}
	
	// Kiểm tra trạng thái đơn hàng
	if order.Status != db.OrderStatusPackaging {
		err = fmt.Errorf("order %s is not in packaging status", order.Code)
		c.JSON(http.StatusConflict, errorResponse(err))
		return
	}
	
	// Kiểm tra trạng thái đã đóng gói
	if order.IsPackaged {
		err = fmt.Errorf("order %s is already packaged", order.Code)
		c.JSON(http.StatusConflict, errorResponse(err))
		return
	}
	
	arg := db.PackageOrderTxParams{
		Order:               &order,
		PackageImages:       req.PackageImages,
		UploadImagesFunc:    server.uploadFileToCloudinary,
		CreateDeliveryOrder: server.deliveryService.CreateOrder,
	}
	
	result, err := server.dbStore.PackageOrderTx(c.Request.Context(), arg)
	if err != nil {
		log.Error().Err(err).Msg("Failed to package order")
		c.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	// Chuẩn bị nội dung thông báo cơ bản
	title := fmt.Sprintf("Đơn hàng %s đã được đóng gói", result.Order.Code)
	message := fmt.Sprintf("Đơn hàng %s đã được đóng gói và sẽ được giao cho đơn vị vận chuyển. Mã vận đơn: %s, dự kiến giao hàng: %s.",
		result.Order.Code,
		result.OrderDelivery.DeliveryTrackingCode,
		result.OrderDelivery.ExpectedDeliveryTime.Format("02/01/2006"))
	
	// Tùy chỉnh thông báo dựa trên loại đơn hàng
	switch result.Order.Type {
	case db.OrderTypeExchange:
		title = fmt.Sprintf("Đơn hàng trao đổi %s đã được đóng gói", result.Order.Code)
		// Các tùy chỉnh khác cho đơn hàng trao đổi nếu cần
	
	case db.OrderTypeAuction:
		title = fmt.Sprintf("Đơn hàng đấu giá %s đã được đóng gói", result.Order.Code)
		// Các tùy chỉnh khác cho đơn hàng đấu giá nếu cần
	
	case db.OrderTypeRegular:
		// Đã là mặc định - không cần tùy chỉnh thêm
	
	default:
		log.Warn().Msgf("Unknown order type %s for order ID %s", result.Order.Type, result.Order.ID)
	}
	
	// Gửi thông báo cho người mua
	err = server.taskDistributor.DistributeTaskSendNotification(c.Request.Context(), &worker.PayloadSendNotification{
		RecipientID: result.Order.BuyerID,
		Title:       title,
		Message:     message,
		Type:        "order",
		ReferenceID: result.Order.Code,
	}, []asynq.Option{
		asynq.MaxRetry(3),
		asynq.Queue(worker.QueueCritical)}...)
	
	c.JSON(http.StatusOK, result)
}

type updateSellerProfileRequest struct {
	UserID   string  `json:"user_id" binding:"required"`
	ShopName *string `json:"shop_name"`
}

//	@Summary		Update seller profile
//	@Description	Update the seller's profile information
//	@Tags			seller profile
//	@Accept			json
//	@Produce		json
//	@Param			request	body		updateSellerProfileRequest	true	"Request body"
//	@Success		200		{object}	db.SellerProfile			"Successfully updated seller profile"
//	@Failure		400		"Invalid request body"
//	@Failure		404		"Seller not found"
//	@Failure		500		"Internal server error"
//	@Router			/seller/profile [patch]
func (server *Server) updateSellerProfile(c *gin.Context) {
	var req updateSellerProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		log.Error().Err(err).Msg("Failed to bind JSON")
		c.JSON(http.StatusBadRequest, errorResponse(err))
		return
	}
	
	profile, err := server.dbStore.UpdateSellerProfileByID(c.Request.Context(), db.UpdateSellerProfileByIDParams{
		SellerID: req.UserID,
		ShopName: req.ShopName,
	})
	if err != nil {
		log.Error().Err(err).Msg("Failed to update seller profile")
		c.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	c.JSON(http.StatusOK, profile)
}

//	@Summary		Get sales order details (excluding exchange orders)
//	@Description	Get details of a specific sales order for the seller
//	@Tags			sellers
//	@Accept			json
//	@Produce		json
//	@Param			sellerID	path	string	true	"Seller ID"
//	@Param			orderID		path	string	true	"Order ID"
//	@Security		accessToken
//	@Success		200	{object}	db.SalesOrderDetails	"Sales order details"
//	@Router			/sellers/:sellerID/orders/:orderID [get]
func (server *Server) getSalesOrderDetails(c *gin.Context) {
	user := c.MustGet(sellerPayloadKey).(*db.User)
	
	orderID, err := uuid.Parse(c.Param("orderID"))
	if err != nil {
		log.Error().Err(err).Msg("Failed to parse order ID")
		c.JSON(http.StatusBadRequest, errorResponse(err))
		return
	}
	
	var resp db.SalesOrderDetails
	
	order, err := server.dbStore.GetSalesOrder(c.Request.Context(), db.GetSalesOrderParams{
		OrderID:  orderID,
		SellerID: user.ID,
	})
	if err != nil {
		if errors.Is(err, db.ErrRecordNotFound) {
			err = fmt.Errorf("order ID %s not found", orderID)
			c.JSON(http.StatusNotFound, errorResponse(err))
			return
		}
		
		log.Error().Err(err).Msg("Failed to get order")
		c.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	// Kiểm tra xem người dùng có quyền truy cập đơn hàng không
	if user.ID != order.SellerID {
		err = fmt.Errorf("order ID %s does not belong to user %s", order.ID, user.ID)
		c.JSON(http.StatusForbidden, errorResponse(err))
		return
	}
	
	resp.Order = order
	
	// Lấy thông tin người mua
	buyer, err := server.dbStore.GetUserByID(c.Request.Context(), order.BuyerID)
	if err != nil {
		if errors.Is(err, db.ErrRecordNotFound) {
			err = fmt.Errorf("buyer ID %s not found", order.BuyerID)
			c.JSON(http.StatusNotFound, errorResponse(err))
			return
		}
		
		log.Err(err).Msg("failed to get buyer")
		c.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	resp.BuyerInfo = buyer
	
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
	
	c.JSON(http.StatusOK, resp)
}

//	@Summary		Cancel order by seller
//	@Description	Cancel a pending order by the seller
//	@Tags			sellers
//	@Accept			json
//	@Produce		json
//	@Param			sellerID	path		string							true	"Seller ID"	example(s123e456-e789-45d0-9876-54321abcdef)
//	@Param			orderID		path		string							true	"Order ID"	example(123e4567-e89b-12d3-a456-426614174000)
//	@Param			request		body		cancelOrderRequest				false	"Cancellation reason"
//	@Success		200			{object}	db.CancelOrderBySellerTxResult	"Order canceled successfully"
//	@Security		accessToken
//	@Router			/sellers/{sellerID}/orders/{orderID}/cancel [patch]
func (server *Server) cancelOrderBySeller(c *gin.Context) {
	seller := c.MustGet(sellerPayloadKey).(*db.User)
	
	orderID, err := uuid.Parse(c.Param("orderID"))
	if err != nil {
		log.Error().Err(err).Msg("Failed to parse order ID")
		c.JSON(http.StatusBadRequest, errorResponse(err))
		return
	}
	
	var req cancelOrderRequest
	if err = c.ShouldBindJSON(&req); err != nil {
		log.Error().Err(err).Msg("Failed to bind JSON")
		c.JSON(http.StatusBadRequest, errorResponse(err))
		return
	}
	
	order, err := server.dbStore.GetSalesOrder(c.Request.Context(), db.GetSalesOrderParams{
		OrderID:  orderID,
		SellerID: seller.ID,
	})
	if err != nil {
		if errors.Is(err, db.ErrRecordNotFound) {
			err = fmt.Errorf("order ID %s not found", orderID)
			c.JSON(http.StatusNotFound, errorResponse(err))
			return
		}
		
		log.Error().Err(err).Msg("Failed to get order")
		c.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	if order.SellerID != seller.ID {
		err = fmt.Errorf("order ID %s does not belong to seller ID %s", order.ID, seller.ID)
		c.JSON(http.StatusForbidden, errorResponse(err))
		return
	}
	
	if order.Status != db.OrderStatusPending {
		err = fmt.Errorf("order ID %s is not in pending status", order.ID)
		c.JSON(http.StatusConflict, errorResponse(err))
		return
	}
	
	arg := db.CancelOrderBySellerTxParams{
		Order:  &order,
		Reason: req.Reason,
	}
	result, err := server.dbStore.CancelOrderBySellerTx(c.Request.Context(), arg)
	if err != nil {
		log.Error().Err(err).Msg("failed to cancel order")
		c.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	// Thiết lập options cho task notification
	opts := []asynq.Option{
		asynq.MaxRetry(3),
		asynq.Queue(worker.QueueCritical),
	}
	
	// Gửi thông báo cho người mua
	err = server.taskDistributor.DistributeTaskSendNotification(c.Request.Context(), &worker.PayloadSendNotification{
		RecipientID: result.Order.BuyerID,
		Title:       "Đơn hàng của bạn đã bị hủy bởi người bán",
		Message: fmt.Sprintf("Đơn hàng #%s đã bị hủy bởi người bán. Số tiền %s đã được hoàn trả vào ví của bạn.",
			result.Order.Code,
			util.FormatVND(result.OrderTransaction.Amount)),
		Type:        "order",
		ReferenceID: result.Order.Code,
	}, opts...)
	if err != nil {
		log.Error().Err(err).Msg("failed to send notification to buyer")
	}
	log.Info().Msgf("Notification sent to buyer: %s", result.Order.BuyerID)
	
	// Gửi thông báo cho người bán (xác nhận hủy đơn)
	err = server.taskDistributor.DistributeTaskSendNotification(c.Request.Context(), &worker.PayloadSendNotification{
		RecipientID: result.Order.SellerID,
		Title:       "Bạn đã hủy đơn hàng thành công",
		Message: fmt.Sprintf("Đơn hàng %s (giá trị %s) đã được hủy thành công. Các sản phẩm đã được đưa về trạng thái có thể bán lại.",
			result.Order.Code,
			util.FormatVND(result.OrderTransaction.Amount)),
		Type:        "order",
		ReferenceID: result.Order.Code,
	}, opts...)
	if err != nil {
		log.Error().Err(err).Msg("failed to send notification to seller")
	}
	log.Info().Msgf("Notification sent to seller: %s", result.Order.SellerID)
	
	c.JSON(http.StatusOK, result)
}

//	@Summary		List auction requests of a seller
//	@Description	List all auction requests that belong to the specified seller, optionally filtered by status
//	@Tags			auctions
//	@Produce		json
//	@Param			sellerID	path	string				true	"Seller ID"
//	@Param			status		query	string				false	"Filter by status"	Enums(pending,approved,rejected)
//	@Success		200			{array}	db.AuctionRequest	"List of auction requests"
//	@Security		accessToken
//	@Router			/sellers/:sellerID/auction-requests [get]
func (server *Server) listSellerAuctionRequests(c *gin.Context) {
	user := c.MustGet(sellerPayloadKey).(*db.User)
	
	status := c.Query("status")
	if status != "" {
		if err := db.IsValidAuctionRequestStatus(status); err != nil {
			c.JSON(http.StatusBadRequest, errorResponse(err))
			return
		}
	}
	
	auctionRequests, err := server.dbStore.ListSellerAuctionRequests(c.Request.Context(), db.ListSellerAuctionRequestsParams{
		SellerID: user.ID,
		Status: db.NullAuctionRequestStatus{
			AuctionRequestStatus: db.AuctionRequestStatus(status),
			Valid:                status != "",
		},
	})
	if err != nil {
		log.Error().Err(err).Msg("failed to list auction requests")
		c.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	c.JSON(http.StatusOK, auctionRequests)
}

//	@Summary		Delete an auction request by seller
//	@Description	Delete an auction request. Only requests with 'pending' or 'rejected' status can be deleted.
//	@Tags			auctions
//	@Produce		json
//	@Security		accessToken
//	@Param			sellerID	path	string	true	"Seller ID"
//	@Param			requestID	path	string	true	"Auction Request ID (UUID format)"
//	@Success		204			"Successfully deleted auction request"
//	@Router			/sellers/{sellerID}/auction-requests/{requestID} [delete]
func (server *Server) deleteAuctionRequest(c *gin.Context) {
	user := c.MustGet(sellerPayloadKey).(*db.User)
	
	requestID, err := uuid.Parse(c.Param("requestID"))
	if err != nil {
		err = fmt.Errorf("failed to parse auction request ID: %w", err)
		c.JSON(http.StatusBadRequest, errorResponse(err))
		return
	}
	
	// Lấy thông tin yêu cầu để kiểm tra
	request, err := server.dbStore.GetAuctionRequestByID(c, requestID)
	if err != nil {
		if errors.Is(err, db.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, errorResponse(err))
			return
		}
		c.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	// Kiểm tra quyền sở hữu yêu cầu
	if request.SellerID != user.ID {
		err = fmt.Errorf("auction request ID %s does not belong to seller ID %s", request.ID, user.ID)
		c.JSON(http.StatusForbidden, errorResponse(err))
		return
	}
	
	// Chỉ cho phép xóa nếu đang pending hoặc rejected
	if request.Status != db.AuctionRequestStatusPending && request.Status != db.AuctionRequestStatusRejected {
		err = fmt.Errorf("only 'pending' or 'rejected' requests can be deleted, current: %s", request.Status)
		c.JSON(http.StatusConflict, errorResponse(err))
		return
	}
	
	// Thực hiện xóa trong transaction
	err = server.dbStore.DeleteAuctionRequestTx(c.Request.Context(), request)
	if err != nil {
		log.Error().Err(err).Msg("failed to delete auction request")
		c.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	c.Status(http.StatusNoContent)
}
