package api

import (
	"errors"
	"net/http"
	"strconv"
	"time"
	
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgtype"
	db "github.com/katatrina/gundam-BE/internal/db/sqlc"
	"github.com/katatrina/gundam-BE/internal/token"
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
		ctx.Status(http.StatusInternalServerError)
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
