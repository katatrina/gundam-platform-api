package api

import (
	"net/http"
	"strconv"
	
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
//	@Failure		403	"seller can only view their own gundams"
//	@Failure		500	"Internal server error"
//	@Router			/sellers/:sellerID/gundams [get]
func (server *Server) listGundamsBySeller(ctx *gin.Context) {
	sellerID := ctx.Param("sellerID")
	userID := ctx.MustGet(authorizationPayloadKey).(*token.Payload).Subject
	if sellerID != userID {
		ctx.JSON(http.StatusForbidden, gin.H{"error": "seller can only view their own gundams"})
		return
	}
	
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
		ctx.Status(http.StatusInternalServerError)
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
//	@Failure		500	"Internal server error"
//	@Router			/sellers/:sellerID/subscriptions/active [get]
func (server *Server) getCurrentActiveSubscription(ctx *gin.Context) {
	sellerID := ctx.Param("sellerID")
	
	subscription, err := server.dbStore.GetCurrentActiveSubscriptionDetailsForSeller(ctx, sellerID)
	if err != nil {
		log.Error().Err(err).Msg("Failed to get current active subscription")
		ctx.Status(http.StatusInternalServerError)
		return
	}
	
	ctx.JSON(http.StatusOK, subscription)
}

//	@Summary		Sell a gundam
//	@Description	Start selling a gundam for the specified seller
//	@Tags			sellers
//	@Accept			json
//	@Produce		json
//	@Param			gundamID	path	int64	true	"Gundam ID"
//	@Param			sellerID	path	string	true	"Seller ID"
//	@Security		accessToken
//	@Success		200	"Successfully sold gundam"
//	@Failure		400	"Invalid gundam ID"
//	@Failure		403	"Cannot sell gundam for another user"
//	@Failure		409	"Subscription limit exceeded"
//	@Failure		409	"Gundam not available for sale"
//	@Failure		500	"Internal server error"
//	@Router			/sellers/:sellerID/gundams/:gundamID/sell [patch]
func (server *Server) sellGundam(ctx *gin.Context) {
	gundamID, err := strconv.ParseInt(ctx.Param("gundamID"), 10, 64)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid gundam ID"})
		return
	}
	
	userID := ctx.Param("sellerID")
	ownerID := ctx.MustGet(authorizationPayloadKey).(*token.Payload).Subject
	if userID != ownerID {
		ctx.JSON(http.StatusForbidden, gin.H{"message": "cannot sell gundam for another user"})
		return
	}
	
	userSubscription, err := server.dbStore.GetCurrentActiveSubscriptionDetailsForSeller(ctx, userID)
	if err != nil {
		log.Error().Err(err).Msg("Failed to get current active subscription")
		ctx.Status(http.StatusInternalServerError)
		return
	}
	
	if !userSubscription.IsUnlimited && userSubscription.ListingsUsed >= userSubscription.MaxListings.Int64 {
		ctx.JSON(http.StatusConflict, errorResponse(db.ErrSubscriptionLimitExceeded))
		return
	}
	
	gundam, err := server.dbStore.GetGundamByID(ctx, gundamID)
	if err != nil {
		log.Error().Err(err).Msg("Failed to get gundam")
		ctx.Status(http.StatusInternalServerError)
		return
	}
	if gundam.Status != db.GundamStatusAvailable {
		ctx.JSON(http.StatusConflict, errorResponse(db.ErrGundamNotAvailableForSale))
		return
	}
	
	arg := db.SellGundamTxParams{
		GundamID:             gundam.ID,
		SellerID:             userID,
		ActiveSubscriptionID: userSubscription.ID,
		ListingsUsed:         userSubscription.ListingsUsed + 1,
	}
	err = server.dbStore.SellGundamTx(ctx, arg)
	if err != nil {
		log.Error().Err(err).Msg("Failed to start selling gundam")
		ctx.Status(http.StatusInternalServerError)
		return
	}
	
	ctx.JSON(http.StatusOK, gin.H{"message": "gundam being sell successfully"})
}
