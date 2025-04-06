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
	"github.com/jackc/pgx/v5/pgtype"
	db "github.com/katatrina/gundam-BE/internal/db/sqlc"
	"github.com/katatrina/gundam-BE/internal/token"
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
//	@Failure		409	"User is already a seller"
//	@Failure		500	"Internal server error"
//	@Router			/users/become-seller [post]
func (server *Server) becomeSeller(ctx *gin.Context) {
	userID := ctx.MustGet(authorizationPayloadKey).(*token.Payload).Subject
	user, err := server.dbStore.GetUserByID(ctx, userID)
	if err != nil {
		log.Error().Err(err).Msg("Failed to get user")
		ctx.Status(http.StatusInternalServerError)
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

//	@Summary		Retrieve a seller by ID
//	@Description	Get detailed information about a specific seller
//	@Tags			sellers
//	@Produce		json
//	@Param			sellerID	path		string	true	"Seller ID"
//	@Success		200			{object}	db.User	"Successfully retrieved seller"
//	@Failure		500			"Internal server error"
//	@Router			/sellers/{id} [get]
func (server *Server) getSeller(ctx *gin.Context) {
	sellerID := ctx.Param("sellerID")
	
	seller, err := server.dbStore.GetSellerByID(ctx, sellerID)
	if err != nil {
		log.Error().Err(err).Msg("Failed to get seller")
		ctx.Status(http.StatusInternalServerError)
		return
	}
	
	ctx.JSON(http.StatusOK, seller)
}

type listGundamsBySellerRequest struct {
	Name *string `form:"name"`
}

func (req *listGundamsBySellerRequest) getName() string {
	if req == nil || req.Name == nil {
		return ""
	}
	
	return *req.Name
}

//	@Summary		List all gundams for a specific seller
//	@Description	Get all gundams that belong to the specified seller ID
//	@Tags			sellers
//	@Accept			json
//	@Produce		json
//	@Param			sellerID	path	string	true	"Seller ID"
//	@Param			name		query	string	false	"Gundam name to filter by"
//	@Security		accessToken
//	@Success		200	"Successfully retrieved list of gundams"
//	@Failure		500	"Internal server error"
//	@Router			/sellers/:sellerID/gundams [get]
func (server *Server) listGundamsBySeller(ctx *gin.Context) {
	sellerID := ctx.Param("sellerID")
	
	req := new(listGundamsBySellerRequest)
	if err := ctx.ShouldBindQuery(req); err != nil {
		ctx.JSON(http.StatusBadRequest, errorResponse(err))
		return
	}
	
	arg := db.ListGundamsBySellerIDParams{
		OwnerID: sellerID,
		Name: pgtype.Text{
			String: req.getName(),
			Valid:  req.Name != nil,
		},
	}
	
	gundams, err := server.dbStore.ListGundamsBySellerID(ctx, arg)
	if err != nil {
		log.Error().Err(err).Msg("Failed to list gundams by seller")
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	ctx.JSON(http.StatusOK, gundams)
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
func (server *Server) getCurrentActiveSubscription(ctx *gin.Context) {
	sellerID := ctx.Param("sellerID")
	
	subscription, err := server.dbStore.GetCurrentActiveSubscriptionDetailsForSeller(ctx, sellerID)
	if err != nil {
		if errors.Is(err, db.ErrRecordNotFound) {
			ctx.JSON(http.StatusNotFound, gin.H{"error": "no active subscription found"})
			return
		}
		
		log.Error().Err(err).Msg("Failed to get current active subscription")
		ctx.Status(http.StatusInternalServerError)
		return
	}
	
	ctx.JSON(http.StatusOK, subscription)
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
//	@Failure		400	{object}	map[string]string		"Invalid gundam ID"
//	@Failure		403	{object}	map[string]string		"Seller does not own this gundam"
//	@Failure		404	{object}	map[string]string		"Gundam not found"
//	@Failure		409	{object}	map[string]string		"Subscription limit exceeded<br/>Subscription expired<br/>Gundam is not available for publishing"
//	@Failure		500	{object}	map[string]string		"Internal server error"
//	@Router			/sellers/{sellerID}/gundams/{gundamID}/publish [patch]
func (server *Server) publishGundam(ctx *gin.Context) {
	gundamID, err := strconv.ParseInt(ctx.Param("gundamID"), 10, 64)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid gundam ID"})
		return
	}
	
	ownerID := ctx.Param("sellerID")
	
	userSubscription, err := server.dbStore.GetCurrentActiveSubscriptionDetailsForSeller(ctx, ownerID)
	if err != nil {
		log.Error().Err(err).Msg("Failed to get current active subscription")
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	// Kiểm tra nếu gói không phải Gói Dùng Thử và đã hết hạn
	if userSubscription.EndDate.Valid &&
		userSubscription.EndDate.Time.Before(time.Now()) &&
		userSubscription.PlanName != db.TrialSellerSubscriptionName {
		ctx.JSON(http.StatusConflict, errorResponse(db.ErrSubscriptionExpired))
		return
	}
	
	// Kiểm tra nếu gói không phải Không Giới Hạn và đã vượt quá số lượt bán
	if !userSubscription.IsUnlimited &&
		userSubscription.MaxListings.Valid &&
		userSubscription.ListingsUsed >= userSubscription.MaxListings.Int64 {
		ctx.JSON(http.StatusConflict, errorResponse(db.ErrSubscriptionLimitExceeded))
		return
	}
	
	gundam, err := server.dbStore.GetGundamByID(ctx, gundamID)
	if err != nil {
		if errors.Is(err, db.ErrRecordNotFound) {
			err = errors.New("gundam not found")
			ctx.JSON(http.StatusNotFound, errorResponse(err))
			return
		}
		
		log.Error().Err(err).Msg("Failed to get gundam")
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	// Kiểm tra quyền sở hữu và trạng thái Gundam
	if gundam.OwnerID != ownerID {
		ctx.JSON(http.StatusForbidden, errorResponse(ErrSellerNotOwnGundam))
		return
	}
	if gundam.Status != db.GundamStatusInstore {
		ctx.JSON(http.StatusConflict, errorResponse(ErrGundamNotInStore))
		return
	}
	
	arg := db.PublishGundamTxParams{
		GundamID:             gundam.ID,
		SellerID:             ownerID,
		ActiveSubscriptionID: userSubscription.ID,
		ListingsUsed:         userSubscription.ListingsUsed + 1,
	}
	err = server.dbStore.PublishGundamTx(ctx, arg)
	if err != nil {
		log.Error().Err(err).Msg("Failed to publish gundam")
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "failed to start publishing process"})
		return
	}
	
	// Phản hồi thành công với thông tin chi tiết sản phẩm
	ctx.JSON(http.StatusOK, gin.H{
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
//	@Failure		409	{object}	map[string]string		"Gundam is not currently listed for sale"
//	@Failure		500	{object}	map[string]string		"Internal server error"
//	@Router			/sellers/{sellerID}/gundams/{gundamID}/unpublish [patch]
func (server *Server) unpublishGundam(ctx *gin.Context) {
	gundamID, err := strconv.ParseInt(ctx.Param("gundamID"), 10, 64)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid gundam ID"})
		return
	}
	
	gundam, err := server.dbStore.GetGundamByID(ctx, gundamID)
	if err != nil {
		log.Error().Err(err).Msg("Failed to get gundam")
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	sellerID := ctx.Param("sellerID")
	
	if gundam.OwnerID != sellerID {
		ctx.JSON(http.StatusForbidden, errorResponse(ErrSellerNotOwnGundam))
		return
	}
	if gundam.Status != db.GundamStatusPublished {
		ctx.JSON(http.StatusConflict, errorResponse(ErrGundamNotPublishing))
		return
	}
	
	err = server.dbStore.UnpublishGundamTx(ctx, db.UnpublishGundamTxParams{
		GundamID: gundam.ID,
		SellerID: gundam.OwnerID,
	})
	if err != nil {
		log.Error().Err(err).Msg("Failed to unpublish gundam")
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	ctx.JSON(http.StatusOK, gin.H{
		"message":   "gundam is no longer being published",
		"gundam_id": gundam.ID,
	})
}

//	@Summary		List all sale orders for a specific seller
//	@Description	Get all sale orders that belong to the specified seller ID
//	@Tags			sellers
//	@Accept			json
//	@Produce		json
//	@Security		accessToken
//	@Param			sellerID	path	string	true	"Seller ID"
//	@Success		200			"Successfully retrieved list of orders"
//	@Failure		500			"Internal server error"
//	@Router			/sellers/:sellerID/orders [get]
func (server *Server) listOrdersBySeller(ctx *gin.Context) {
	sellerID := ctx.Param("sellerID")
	
	orders, err := server.dbStore.ListOrdersBySellerID(ctx, sellerID)
	if err != nil {
		log.Error().Err(err).Msg("Failed to list orders by seller")
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	ctx.JSON(http.StatusOK, orders)
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
//	@Failure		400	{object}	map[string]string		"Invalid order ID or seller ID"
//	@Failure		404	{object}	map[string]string		"Order not found"
//	@Failure		403	{object}	map[string]string		"Order does not belong to this seller"
//	@Failure		409	{object}	map[string]string		"Order is not in pending status"
//	@Failure		500	{object}	map[string]string		"Internal server error"
//	@Router			/sellers/:sellerID/orders/:orderID/confirm [patch]
func (server *Server) confirmOrder(ctx *gin.Context) {
	var url confirmOrderRequestParams
	if err := ctx.ShouldBindUri(&url); err != nil {
		log.Error().Err(err).Msg("Failed to bind URI")
		ctx.JSON(http.StatusBadRequest, errorResponse(err))
		return
	}
	
	orderID, err := uuid.Parse(url.OrderID)
	if err != nil {
		log.Error().Err(err).Msg("Failed to parse order ID")
		ctx.JSON(http.StatusBadRequest, errorResponse(err))
		return
	}
	
	order, err := server.dbStore.GetSalesOrderBySellerID(ctx.Request.Context(), db.GetSalesOrderBySellerIDParams{
		OrderID:  orderID,
		SellerID: url.SellerID,
	})
	if err != nil {
		if errors.Is(err, db.ErrRecordNotFound) {
			err = fmt.Errorf("order ID %s not found", orderID)
			ctx.JSON(http.StatusNotFound, errorResponse(err))
			return
		}
		
		log.Error().Err(err).Msg("Failed to get order")
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	// Kiểm tra quyền sở hữu đơn hàng
	if order.SellerID != url.SellerID {
		err = fmt.Errorf("order %s does not belong to seller ID %s", order.Code, url.SellerID)
		ctx.JSON(http.StatusForbidden, errorResponse(err))
		return
	}
	
	// Kiểm tra trạng thái đơn hàng
	if order.Status != db.OrderStatusPending {
		err = fmt.Errorf("order %s is not in pending status", order.Code)
		ctx.JSON(http.StatusConflict, errorResponse(err))
		return
	}
	
	result, err := server.dbStore.ConfirmOrderTx(ctx, db.ConfirmOrderTxParams{
		Order:    &order,
		SellerID: url.SellerID,
	})
	if err != nil {
		log.Error().Err(err).Msg("failed to confirm order")
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	log.Info().Msgf("Order confirmed: %s", result.Order.Code)
	
	// Thông báo cho người dùng về việc đơn hàng đã được xác nhận thành công
	opts := []asynq.Option{
		asynq.ProcessIn(3 * time.Second),
		asynq.MaxRetry(3),
		asynq.Queue(worker.QueueCritical),
	}
	
	// Gửi thông báo cho người mua
	err = server.taskDistributor.DistributeTaskSendNotification(ctx.Request.Context(), &worker.PayloadSendNotification{
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
	err = server.taskDistributor.DistributeTaskSendNotification(ctx.Request.Context(), &worker.PayloadSendNotification{
		RecipientID: result.Order.SellerID,
		Title:       fmt.Sprintf("Đã xác nhận đơn hàng #%s", result.Order.Code),
		Message:     fmt.Sprintf("Bạn đã xác nhận đơn hàng #%s. Số tiền %d VNĐ đã được chuyển vào số dư tạm thời của bạn. Số tiền này sẽ được chuyển vào số dư khả dụng sau khi người mua xác nhận đã nhận hàng thành công.", result.Order.Code, result.SellerEntry.Amount),
		Type:        "order",
		ReferenceID: result.Order.Code,
	}, opts...)
	if err != nil {
		log.Err(err).Msg("failed to send notification to seller")
	}
	log.Info().Msgf("Notification sent to seller: %s", url.SellerID)
	
	ctx.JSON(http.StatusOK, result)
}

type packageOrderRequestParams struct {
	SellerID string `uri:"sellerID" binding:"required"`
	OrderID  string `uri:"orderID" binding:"required"`
}

type packageOrderRequestBody struct {
	PackageImages []*multipart.FileHeader `form:"package_images" binding:"required"`
	// Lược bỏ package weight và package size (length, width, height)
}

//	@Summary		Package an order
//	@Description	Package an order for the specified seller. This endpoint checks the order's status before proceeding.
//	@Tags			sellers
//	@Produce		json
//	@Param			sellerID	path	string	true	"Seller ID"
//	@Param			orderID		path	string	true	"Order ID"
//	@Security		accessToken
//	@Param			package_images	formData	file					true	"Package images"
//	@Success		200				{object}	db.PackageOrderTxResult	"Successfully packaged order"
//	@Failure		400				{object}	map[string]string		"Invalid order ID or seller ID<br/>At least one package image is required"
//	@Failure		404				{object}	map[string]string		"Order not found"
//	@Failure		403				{object}	map[string]string		"Order does not belong to this seller"
//	@Failure		409				{object}	map[string]string		"Order is not in packaging status<br/>Order is already packaged"
//	@Failure		500				{object}	map[string]string		"Internal server error"
//	@Router			/sellers/:sellerID/orders/:orderID/package [patch]
func (server *Server) packageOrder(c *gin.Context) {
	var url packageOrderRequestParams
	if err := c.ShouldBindUri(&url); err != nil {
		log.Error().Err(err).Msg("Failed to bind URI")
		c.JSON(http.StatusBadRequest, errorResponse(err))
		return
	}
	
	orderID, err := uuid.Parse(url.OrderID)
	if err != nil {
		log.Error().Err(err).Msg("Failed to parse order ID")
		c.JSON(http.StatusBadRequest, errorResponse(err))
		return
	}
	
	// Bind form data including multipart files
	var req packageOrderRequestBody
	if err = c.ShouldBindWith(&req, binding.FormMultipart); err != nil {
		log.Error().Err(err).Msg("Failed to bind form data")
		c.JSON(http.StatusBadRequest, errorResponse(err))
		return
	}
	
	// Validate that at least one image is provided
	if len(req.PackageImages) == 0 {
		err = errors.New("at least one package image is required")
		log.Error().Err(err).Msg("No package images provided")
		c.JSON(http.StatusBadRequest, errorResponse(err))
		return
	}
	
	// Validate that the order belongs to the seller
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
	if order.SellerID != url.SellerID {
		err = fmt.Errorf("order %s does not belong to seller ID %s", order.Code, url.SellerID)
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
	
	// Gửi thông báo cho người mua
	err = server.taskDistributor.DistributeTaskSendNotification(c.Request.Context(), &worker.PayloadSendNotification{
		RecipientID: result.Order.BuyerID,
		Title:       fmt.Sprintf("Đơn hàng #%s đã được đóng gói", result.Order.Code),
		Message: fmt.Sprintf("Đơn hàng #%s đã được đóng gói và sẽ được giao cho đơn vị vận chuyển. Mã vận đơn: %s, dự kiến giao hàng: %s.",
			result.Order.Code,
			result.OrderDelivery.GhnOrderCode,
			result.OrderDelivery.ExpectedDeliveryTime.Format("02/01/2006")),
		Type:        "order",
		ReferenceID: result.Order.Code,
	})
	
	// TODO: Có thể gửi thông báo cho người bán
	
	c.JSON(http.StatusOK, result)
}
