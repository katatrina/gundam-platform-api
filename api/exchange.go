package api

import (
	"errors"
	"fmt"
	"net/http"
	
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	db "github.com/katatrina/gundam-BE/internal/db/sqlc"
	"github.com/katatrina/gundam-BE/internal/token"
	"github.com/katatrina/gundam-BE/internal/util"
)

//	@Summary		Get exchange details
//	@Description	Retrieves detailed information about a specific exchange.
//	@Tags			exchanges
//	@Produce		json
//	@Security		accessToken
//	@Param			exchangeID	path		string					true	"Exchange ID"
//	@Success		200			{object}	db.UserExchangeDetails	"Exchange details"
//	@Router			/exchanges/{exchangeID} [get]
func (server *Server) getExchangeDetails(c *gin.Context) {
	// Lấy thông tin người dùng đã đăng nhập
	authPayload := c.MustGet(authorizationPayloadKey).(*token.Payload)
	userID := authPayload.Subject
	
	// Lấy ID của exchange từ URL
	exchangeIDStr := c.Param("exchangeID")
	exchangeID, err := uuid.Parse(exchangeIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, errorResponse(fmt.Errorf("invalid exchange ID: %s", exchangeIDStr)))
		return
	}
	
	// Lấy thông tin cơ bản của exchange
	exchange, err := server.dbStore.GetExchangeByID(c.Request.Context(), exchangeID)
	if err != nil {
		if errors.Is(err, db.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, errorResponse(fmt.Errorf("exchange ID %s not found", exchangeIDStr)))
			return
		}
		
		c.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	// Kiểm tra quyền truy cập - chỉ người tham gia trao đổi mới có thể xem chi tiết
	if exchange.PosterID != userID && exchange.OffererID != userID {
		err = fmt.Errorf("exchange ID %s does not belong to user ID %s", exchangeIDStr, userID)
		c.JSON(http.StatusForbidden, errorResponse(err))
		return
	}
	
	// Xác định vai trò của người dùng hiện tại
	isCurrentUserPoster := exchange.PosterID == userID
	
	// Khởi tạo response
	result := db.UserExchangeDetails{
		ID:                 exchange.ID,
		PosterID:           exchange.PosterID,
		OffererID:          exchange.OffererID,
		PayerID:            exchange.PayerID,
		CompensationAmount: exchange.CompensationAmount,
		Status:             string(exchange.Status),
		CreatedAt:          exchange.CreatedAt,
		UpdatedAt:          exchange.UpdatedAt,
		CompletedAt:        exchange.CompletedAt,
		CanceledBy:         exchange.CanceledBy,
		CanceledReason:     exchange.CanceledReason,
	}
	
	// Xây dựng thông tin người dùng hiện tại và đối tác
	var currentUserID, partnerID string
	var currentUserOrderID, partnerOrderID uuid.UUID
	var currentUserFromDeliveryID, currentUserToDeliveryID, partnerFromDeliveryID, partnerToDeliveryID *int64
	var currentUserDeliveryFeePaid, partnerDeliveryFeePaid bool
	var isCurrentUserItemFromPoster, isPartnerItemFromPoster bool
	
	if isCurrentUserPoster {
		currentUserID = exchange.PosterID
		partnerID = exchange.OffererID
		currentUserOrderID = exchange.PosterOrderID
		partnerOrderID = exchange.OffererOrderID
		currentUserFromDeliveryID = exchange.PosterFromDeliveryID
		currentUserToDeliveryID = exchange.PosterToDeliveryID
		partnerFromDeliveryID = exchange.OffererFromDeliveryID
		partnerToDeliveryID = exchange.OffererToDeliveryID
		currentUserDeliveryFeePaid = exchange.PosterDeliveryFeePaid
		partnerDeliveryFeePaid = exchange.OffererDeliveryFeePaid
		isCurrentUserItemFromPoster = true
		isPartnerItemFromPoster = false
	} else {
		currentUserID = exchange.OffererID
		partnerID = exchange.PosterID
		currentUserOrderID = exchange.OffererOrderID
		partnerOrderID = exchange.PosterOrderID
		currentUserFromDeliveryID = exchange.OffererFromDeliveryID
		currentUserToDeliveryID = exchange.OffererToDeliveryID
		partnerFromDeliveryID = exchange.PosterFromDeliveryID
		partnerToDeliveryID = exchange.PosterToDeliveryID
		currentUserDeliveryFeePaid = exchange.OffererDeliveryFeePaid
		partnerDeliveryFeePaid = exchange.PosterDeliveryFeePaid
		isCurrentUserItemFromPoster = false
		isPartnerItemFromPoster = true
	}
	
	// Xây dựng thông tin người dùng hiện tại
	currentUser, err := server.dbStore.GetUserByID(c.Request.Context(), currentUserID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, errorResponse(fmt.Errorf("failed to get current user info: %w", err)))
		return
	}
	
	// Xây dựng thông tin đơn hàng của người dùng hiện tại
	currentUserOrder, err := server.dbStore.GetOrderByID(c.Request.Context(), currentUserOrderID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, errorResponse(fmt.Errorf("failed to get current user order: %w", err)))
		return
	}
	
	// Xây dựng thông tin địa chỉ của người dùng hiện tại
	var currentUserFromDelivery, currentUserToDelivery *db.DeliveryInformation
	if currentUserFromDeliveryID != nil {
		fromDelivery, err := server.dbStore.GetDeliveryInformation(c.Request.Context(), *currentUserFromDeliveryID)
		if err == nil {
			currentUserFromDelivery = &fromDelivery
		}
	}
	
	if currentUserToDeliveryID != nil {
		toDelivery, err := server.dbStore.GetDeliveryInformation(c.Request.Context(), *currentUserToDeliveryID)
		if err == nil {
			currentUserToDelivery = &toDelivery
		}
	}
	
	// Lấy danh sách Gundam của người dùng hiện tại
	currentUserItems, err := server.dbStore.ListExchangeItems(c.Request.Context(), db.ListExchangeItemsParams{
		ExchangeID:   exchange.ID,
		IsFromPoster: util.BoolPointer(isCurrentUserItemFromPoster),
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, errorResponse(fmt.Errorf("failed to list current user items: %w", err)))
		return
	}
	// Xây dựng thông tin người đối tác
	partner, err := server.dbStore.GetUserByID(c.Request.Context(), partnerID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, errorResponse(fmt.Errorf("failed to get partner info: %w", err)))
		return
	}
	
	// Xây dựng thông tin đơn hàng của đối tác
	partnerOrder, err := server.dbStore.GetOrderByID(c.Request.Context(), partnerOrderID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, errorResponse(fmt.Errorf("failed to get partner order: %w", err)))
		return
	}
	
	// Xây dựng thông tin địa chỉ của đối tác
	var partnerFromDelivery, partnerToDelivery *db.DeliveryInformation
	if partnerFromDeliveryID != nil {
		fromDelivery, err := server.dbStore.GetDeliveryInformation(c.Request.Context(), *partnerFromDeliveryID)
		if err == nil {
			partnerFromDelivery = &fromDelivery
		}
	}
	
	if partnerToDeliveryID != nil {
		toDelivery, err := server.dbStore.GetDeliveryInformation(c.Request.Context(), *partnerToDeliveryID)
		if err == nil {
			partnerToDelivery = &toDelivery
		}
	}
	
	// Lấy danh sách Gundam của đối tác
	partnerItems, err := server.dbStore.ListExchangeItems(c.Request.Context(), db.ListExchangeItemsParams{
		ExchangeID:   exchange.ID,
		IsFromPoster: util.BoolPointer(isPartnerItemFromPoster),
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, errorResponse(fmt.Errorf("failed to list partner items: %w", err)))
		return
	}
	
	// Đóng gói thông tin người dùng hiện tại
	result.CurrentUser = db.ExchangeUserInfo{
		ID:              currentUser.ID,
		FullName:        currentUser.FullName,
		AvatarURL:       currentUser.AvatarURL,
		Order:           &currentUserOrder,
		FromDelivery:    currentUserFromDelivery,
		ToDelivery:      currentUserToDelivery,
		DeliveryFeePaid: currentUserDeliveryFeePaid,
		Items:           currentUserItems,
	}
	
	// Đóng gói thông tin đối tác
	result.Partner = db.ExchangeUserInfo{
		ID:              partner.ID,
		FullName:        partner.FullName,
		AvatarURL:       partner.AvatarURL,
		Order:           &partnerOrder,
		FromDelivery:    partnerFromDelivery,
		ToDelivery:      partnerToDelivery,
		DeliveryFeePaid: partnerDeliveryFeePaid,
		Items:           partnerItems,
	}
	
	c.JSON(http.StatusOK, result)
}
