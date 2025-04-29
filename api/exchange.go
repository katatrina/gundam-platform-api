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
	var currentUserOrderID, partnerOrderID *uuid.UUID
	var currentUserFromDeliveryID, currentUserToDeliveryID, partnerFromDeliveryID, partnerToDeliveryID *int64
	var currentUserDeliveryFee, partnerDeliveryFee *int64
	var currentUserDeliveryFeePaid, partnerDeliveryFeePaid bool
	var currentUserOrderNote, partnerOrderNote *string
	var currentUserOrderExpectedDeliveryTime, partnerOrderExpectedDeliveryTime *time.Time
	var isCurrentUserItemFromPoster, isPartnerItemFromPoster bool
	
	if isCurrentUserPoster {
		currentUserID = exchange.PosterID
		partnerID = exchange.OffererID
		if exchange.PosterOrderID != nil {
			currentUserOrderID = exchange.PosterOrderID
		}
		if exchange.OffererOrderID != nil {
			partnerOrderID = exchange.OffererOrderID
		}
		currentUserFromDeliveryID = exchange.PosterFromDeliveryID
		currentUserToDeliveryID = exchange.PosterToDeliveryID
		partnerFromDeliveryID = exchange.OffererFromDeliveryID
		partnerToDeliveryID = exchange.OffererToDeliveryID
		currentUserDeliveryFee = exchange.PosterDeliveryFee
		partnerDeliveryFee = exchange.OffererDeliveryFee
		currentUserDeliveryFeePaid = exchange.PosterDeliveryFeePaid
		partnerDeliveryFeePaid = exchange.OffererDeliveryFeePaid
		currentUserOrderNote = exchange.PosterOrderNote
		partnerOrderNote = exchange.OffererOrderNote
		currentUserOrderExpectedDeliveryTime = exchange.PosterOrderExpectedDeliveryTime
		partnerOrderExpectedDeliveryTime = exchange.OffererOrderExpectedDeliveryTime
		isCurrentUserItemFromPoster = true
		isPartnerItemFromPoster = false
	} else {
		currentUserID = exchange.OffererID
		partnerID = exchange.PosterID
		if exchange.OffererOrderID != nil {
			currentUserOrderID = exchange.OffererOrderID
		}
		if exchange.PosterOrderID != nil {
			partnerOrderID = exchange.PosterOrderID
		}
		currentUserFromDeliveryID = exchange.OffererFromDeliveryID
		currentUserToDeliveryID = exchange.OffererToDeliveryID
		partnerFromDeliveryID = exchange.PosterFromDeliveryID
		partnerToDeliveryID = exchange.PosterToDeliveryID
		currentUserDeliveryFee = exchange.OffererDeliveryFee
		partnerDeliveryFee = exchange.PosterDeliveryFee
		currentUserDeliveryFeePaid = exchange.OffererDeliveryFeePaid
		partnerDeliveryFeePaid = exchange.PosterDeliveryFeePaid
		currentUserOrderNote = exchange.OffererOrderNote
		partnerOrderNote = exchange.PosterOrderNote
		currentUserOrderExpectedDeliveryTime = exchange.OffererOrderExpectedDeliveryTime
		partnerOrderExpectedDeliveryTime = exchange.PosterOrderExpectedDeliveryTime
		isCurrentUserItemFromPoster = false
		isPartnerItemFromPoster = true
	}
	
	// Xây dựng thông tin người dùng hiện tại
	currentUser, err := server.dbStore.GetUserByID(c.Request.Context(), currentUserID)
	if err != nil {
		if errors.Is(err, db.ErrRecordNotFound) {
			err = fmt.Errorf("user ID %s not found", currentUserID)
			c.JSON(http.StatusNotFound, errorResponse(err))
			return
		}
		
		c.JSON(http.StatusInternalServerError, errorResponse(fmt.Errorf("failed to get current user info: %w", err)))
		return
	}
	
	// Xây dựng thông tin đơn hàng của người dùng hiện tại
	var currentUserOrder *db.Order
	if currentUserOrderID != nil {
		order, err := server.dbStore.GetOrderByID(c.Request.Context(), *currentUserOrderID)
		if err != nil {
			if !errors.Is(err, db.ErrRecordNotFound) {
				c.JSON(http.StatusInternalServerError, errorResponse(fmt.Errorf("failed to get current user order: %w", err)))
				return
			}
			// Nếu là ErrRecordNotFound, không làm gì cả
		} else {
			currentUserOrder = &order
		}
	}
	
	// Xây dựng thông tin địa chỉ của người dùng hiện tại
	var currentUserFromDelivery, currentUserToDelivery *db.DeliveryInformation
	if currentUserFromDeliveryID != nil {
		fromDelivery, err := server.dbStore.GetDeliveryInformation(c.Request.Context(), *currentUserFromDeliveryID)
		if err != nil {
			if !errors.Is(err, db.ErrRecordNotFound) {
				c.JSON(http.StatusInternalServerError, errorResponse(fmt.Errorf("failed to get current user from delivery: %w", err)))
				return
			}
			// Nếu là ErrRecordNotFound, không làm gì cả
		} else {
			currentUserFromDelivery = &fromDelivery
		}
	}
	
	if currentUserToDeliveryID != nil {
		toDelivery, err := server.dbStore.GetDeliveryInformation(c.Request.Context(), *currentUserToDeliveryID)
		if err != nil {
			if !errors.Is(err, db.ErrRecordNotFound) {
				c.JSON(http.StatusInternalServerError, errorResponse(fmt.Errorf("failed to get current user to delivery: %w", err)))
				return
			}
			// Nếu là ErrRecordNotFound, không làm gì cả
		} else {
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
		if errors.Is(err, db.ErrRecordNotFound) {
			err = fmt.Errorf("user ID %s not found", partnerID)
			c.JSON(http.StatusNotFound, errorResponse(err))
			return
		}
		
		c.JSON(http.StatusInternalServerError, errorResponse(fmt.Errorf("failed to get partner info: %w", err)))
		return
	}
	
	// Xây dựng thông tin đơn hàng của đối tác
	var partnerOrder *db.Order
	if partnerOrderID != nil {
		order, err := server.dbStore.GetOrderByID(c.Request.Context(), *partnerOrderID)
		if err != nil {
			if !errors.Is(err, db.ErrRecordNotFound) {
				c.JSON(http.StatusInternalServerError, errorResponse(fmt.Errorf("failed to get partner order: %w", err)))
				return
			}
			// Nếu là ErrRecordNotFound, không làm gì cả
		} else {
			partnerOrder = &order
		}
	}
	
	// Xây dựng thông tin địa chỉ của đối tác
	var partnerFromDelivery, partnerToDelivery *db.DeliveryInformation
	if partnerFromDeliveryID != nil {
		fromDelivery, err := server.dbStore.GetDeliveryInformation(c.Request.Context(), *partnerFromDeliveryID)
		if err != nil {
			if !errors.Is(err, db.ErrRecordNotFound) {
				c.JSON(http.StatusInternalServerError, errorResponse(fmt.Errorf("failed to get partner from delivery: %w", err)))
				return
			}
			// Nếu là ErrRecordNotFound, không làm gì cả
		} else {
			partnerFromDelivery = &fromDelivery
		}
	}
	
	if partnerToDeliveryID != nil {
		toDelivery, err := server.dbStore.GetDeliveryInformation(c.Request.Context(), *partnerToDeliveryID)
		if err != nil {
			if !errors.Is(err, db.ErrRecordNotFound) {
				c.JSON(http.StatusInternalServerError, errorResponse(fmt.Errorf("failed to get partner to delivery: %w", err)))
				return
			}
			// Nếu là ErrRecordNotFound, không làm gì cả
		} else {
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
		ID:                   currentUser.ID,
		FullName:             currentUser.FullName,
		AvatarURL:            currentUser.AvatarURL,
		Order:                currentUserOrder,
		FromDelivery:         currentUserFromDelivery,
		ToDelivery:           currentUserToDelivery,
		DeliveryFee:          currentUserDeliveryFee,
		DeliveryFeePaid:      currentUserDeliveryFeePaid,
		ExpectedDeliveryTime: currentUserOrderExpectedDeliveryTime,
		Note:                 currentUserOrderNote,
		Items:                currentUserItems,
	}
	
	// Đóng gói thông tin đối tác
	result.Partner = db.ExchangeUserInfo{
		ID:                   partner.ID,
		FullName:             partner.FullName,
		AvatarURL:            partner.AvatarURL,
		Order:                partnerOrder,
		FromDelivery:         partnerFromDelivery,
		ToDelivery:           partnerToDelivery,
		DeliveryFee:          partnerDeliveryFee,
		DeliveryFeePaid:      partnerDeliveryFeePaid,
		ExpectedDeliveryTime: partnerOrderExpectedDeliveryTime,
		Note:                 partnerOrderNote,
		Items:                partnerItems,
	}
	
	c.JSON(http.StatusOK, result)
}

//	@Summary		List user's exchanges
//	@Description	Retrieves a list of all exchanges that the authenticated user is participating in.
//	@Tags			exchanges
//	@Produce		json
//	@Security		accessToken
//	@Param			status	query	string					false	"Filter by status (pending, packaging, delivering, delivered, completed, canceled, failed)"
//	@Success		200		{array}	db.UserExchangeDetails	"List of user's exchanges"
//	@Router			/exchanges [get]
func (server *Server) listUserExchanges(c *gin.Context) {
	// Lấy thông tin người dùng đã đăng nhập
	authPayload := c.MustGet(authorizationPayloadKey).(*token.Payload)
	userID := authPayload.Subject
	
	// Xử lý tham số lọc theo trạng thái
	status := c.Query("status")
	var exchangeStatus db.ExchangeStatus
	var validStatus bool
	if status != "" {
		if err := db.IsValidExchangeStatus(status); err != nil {
			c.JSON(http.StatusBadRequest, errorResponse(fmt.Errorf("invalid exchange status: %s", status)))
			return
		}
		
		exchangeStatus = db.ExchangeStatus(status)
		validStatus = true
	}
	
	// Lấy danh sách các exchange của người dùng
	exchanges, err := server.dbStore.ListUserExchanges(c.Request.Context(), db.ListUserExchangesParams{
		UserID: userID,
		Status: db.NullExchangeStatus{
			ExchangeStatus: exchangeStatus,
			Valid:          validStatus,
		},
	})
	if err != nil {
		err = fmt.Errorf("failed to list user exchanges: %w", err)
		c.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	result := make([]db.UserExchangeDetails, 0, len(exchanges))
	for _, exchange := range exchanges {
		// Xác định vai trò của người dùng hiện tại
		isCurrentUserPoster := exchange.PosterID == userID
		
		// Khởi tạo response
		details := db.UserExchangeDetails{
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
		var currentUserOrderID, partnerOrderID *uuid.UUID
		var currentUserFromDeliveryID, currentUserToDeliveryID, partnerFromDeliveryID, partnerToDeliveryID *int64
		var currentUserDeliveryFee, partnerDeliveryFee *int64
		var currentUserDeliveryFeePaid, partnerDeliveryFeePaid bool
		var currentUserOrderNote, partnerOrderNote *string
		var currentUserOrderExpectedDeliveryTime, partnerOrderExpectedDeliveryTime *time.Time
		var isCurrentUserItemFromPoster, isPartnerItemFromPoster bool
		
		if isCurrentUserPoster {
			currentUserID = exchange.PosterID
			partnerID = exchange.OffererID
			if exchange.PosterOrderID != nil {
				currentUserOrderID = exchange.PosterOrderID
			}
			if exchange.OffererOrderID != nil {
				partnerOrderID = exchange.OffererOrderID
			}
			currentUserFromDeliveryID = exchange.PosterFromDeliveryID
			currentUserToDeliveryID = exchange.PosterToDeliveryID
			partnerFromDeliveryID = exchange.OffererFromDeliveryID
			partnerToDeliveryID = exchange.OffererToDeliveryID
			currentUserDeliveryFee = exchange.PosterDeliveryFee
			partnerDeliveryFee = exchange.OffererDeliveryFee
			currentUserDeliveryFeePaid = exchange.PosterDeliveryFeePaid
			partnerDeliveryFeePaid = exchange.OffererDeliveryFeePaid
			currentUserOrderNote = exchange.PosterOrderNote
			partnerOrderNote = exchange.OffererOrderNote
			currentUserOrderExpectedDeliveryTime = exchange.PosterOrderExpectedDeliveryTime
			partnerOrderExpectedDeliveryTime = exchange.OffererOrderExpectedDeliveryTime
			isCurrentUserItemFromPoster = true
			isPartnerItemFromPoster = false
		} else {
			currentUserID = exchange.OffererID
			partnerID = exchange.PosterID
			if exchange.OffererOrderID != nil {
				currentUserOrderID = exchange.OffererOrderID
			}
			if exchange.PosterOrderID != nil {
				partnerOrderID = exchange.PosterOrderID
			}
			currentUserFromDeliveryID = exchange.OffererFromDeliveryID
			currentUserToDeliveryID = exchange.OffererToDeliveryID
			partnerFromDeliveryID = exchange.PosterFromDeliveryID
			partnerToDeliveryID = exchange.PosterToDeliveryID
			currentUserDeliveryFee = exchange.OffererDeliveryFee
			partnerDeliveryFee = exchange.PosterDeliveryFee
			currentUserDeliveryFeePaid = exchange.OffererDeliveryFeePaid
			partnerDeliveryFeePaid = exchange.PosterDeliveryFeePaid
			currentUserOrderNote = exchange.OffererOrderNote
			partnerOrderNote = exchange.PosterOrderNote
			currentUserOrderExpectedDeliveryTime = exchange.OffererOrderExpectedDeliveryTime
			partnerOrderExpectedDeliveryTime = exchange.PosterOrderExpectedDeliveryTime
			isCurrentUserItemFromPoster = false
			isPartnerItemFromPoster = true
		}
		
		// Xây dựng thông tin người dùng hiện tại
		currentUser, err := server.dbStore.GetUserByID(c.Request.Context(), currentUserID)
		if err != nil {
			if errors.Is(err, db.ErrRecordNotFound) {
				c.JSON(http.StatusNotFound, errorResponse(fmt.Errorf("user ID %s not found", currentUserID)))
				return
			}
			
			c.JSON(http.StatusInternalServerError, errorResponse(fmt.Errorf("failed to get current user info: %w", err)))
			return
		}
		
		// Xây dựng thông tin đơn hàng của người dùng hiện tại
		var currentUserOrder *db.Order
		if currentUserOrderID != nil {
			order, err := server.dbStore.GetOrderByID(c.Request.Context(), *currentUserOrderID)
			if err != nil {
				if !errors.Is(err, db.ErrRecordNotFound) {
					c.JSON(http.StatusInternalServerError, errorResponse(fmt.Errorf("failed to get current user order: %w", err)))
					return
				}
				// Nếu là ErrRecordNotFound, không làm gì cả
			} else {
				currentUserOrder = &order
			}
		}
		
		// Xây dựng thông tin địa chỉ của người dùng hiện tại
		var currentUserFromDelivery, currentUserToDelivery *db.DeliveryInformation
		if currentUserFromDeliveryID != nil {
			fromDelivery, err := server.dbStore.GetDeliveryInformation(c.Request.Context(), *currentUserFromDeliveryID)
			if err != nil {
				if !errors.Is(err, db.ErrRecordNotFound) {
					c.JSON(http.StatusInternalServerError, errorResponse(fmt.Errorf("failed to get current user from delivery: %w", err)))
					return
				}
				// Nếu là ErrRecordNotFound, không làm gì cả
			} else {
				currentUserFromDelivery = &fromDelivery
			}
		}
		
		if currentUserToDeliveryID != nil {
			toDelivery, err := server.dbStore.GetDeliveryInformation(c.Request.Context(), *currentUserToDeliveryID)
			if err != nil {
				if !errors.Is(err, db.ErrRecordNotFound) {
					c.JSON(http.StatusInternalServerError, errorResponse(fmt.Errorf("failed to get current user to delivery: %w", err)))
					return
				}
				// Nếu là ErrRecordNotFound, không làm gì cả
			} else {
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
			if errors.Is(err, db.ErrRecordNotFound) {
				c.JSON(http.StatusNotFound, errorResponse(fmt.Errorf("partner ID %s not found", partnerID)))
				return
			}
			
			c.JSON(http.StatusInternalServerError, errorResponse(fmt.Errorf("failed to get partner info: %w", err)))
			return
		}
		
		// Xây dựng thông tin đơn hàng của đối tác
		var partnerOrder *db.Order
		if partnerOrderID != nil {
			order, err := server.dbStore.GetOrderByID(c.Request.Context(), *partnerOrderID)
			if err != nil {
				if !errors.Is(err, db.ErrRecordNotFound) {
					c.JSON(http.StatusInternalServerError, errorResponse(fmt.Errorf("failed to get partner order: %w", err)))
					return
				}
				// Nếu là ErrRecordNotFound, không làm gì cả
			} else {
				partnerOrder = &order
			}
		}
		
		// Xây dựng thông tin địa chỉ của đối tác
		var partnerFromDelivery, partnerToDelivery *db.DeliveryInformation
		if partnerFromDeliveryID != nil {
			fromDelivery, err := server.dbStore.GetDeliveryInformation(c.Request.Context(), *partnerFromDeliveryID)
			if err != nil {
				if !errors.Is(err, db.ErrRecordNotFound) {
					c.JSON(http.StatusInternalServerError, errorResponse(fmt.Errorf("failed to get partner from delivery: %w", err)))
					return
				}
				// Nếu là ErrRecordNotFound, không làm gì cả
			} else {
				partnerFromDelivery = &fromDelivery
			}
		}
		
		if partnerToDeliveryID != nil {
			toDelivery, err := server.dbStore.GetDeliveryInformation(c.Request.Context(), *partnerToDeliveryID)
			if err != nil {
				if !errors.Is(err, db.ErrRecordNotFound) {
					c.JSON(http.StatusInternalServerError, errorResponse(fmt.Errorf("failed to get partner to delivery: %w", err)))
					return
				}
				// Nếu là ErrRecordNotFound, không làm gì cả
			} else {
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
		details.CurrentUser = db.ExchangeUserInfo{
			ID:                   currentUser.ID,
			FullName:             currentUser.FullName,
			AvatarURL:            currentUser.AvatarURL,
			Order:                currentUserOrder,
			FromDelivery:         currentUserFromDelivery,
			ToDelivery:           currentUserToDelivery,
			DeliveryFee:          currentUserDeliveryFee,
			DeliveryFeePaid:      currentUserDeliveryFeePaid,
			ExpectedDeliveryTime: currentUserOrderExpectedDeliveryTime,
			Note:                 currentUserOrderNote,
			Items:                currentUserItems,
		}
		
		// Đóng gói thông tin đối tác
		details.Partner = db.ExchangeUserInfo{
			ID:                   partner.ID,
			FullName:             partner.FullName,
			AvatarURL:            partner.AvatarURL,
			Order:                partnerOrder,
			FromDelivery:         partnerFromDelivery,
			ToDelivery:           partnerToDelivery,
			DeliveryFee:          partnerDeliveryFee,
			DeliveryFeePaid:      partnerDeliveryFeePaid,
			ExpectedDeliveryTime: partnerOrderExpectedDeliveryTime,
			Note:                 partnerOrderNote,
			Items:                partnerItems,
		}
		
		result = append(result, details)
	}
	
	c.JSON(http.StatusOK, result)
}

// provideExchangeDeliveryAddressesRequest định nghĩa request để cung cấp thông tin vận chuyển
type provideExchangeDeliveryAddressesRequest struct {
	// ID địa chỉ gửi đã được lưu trong bảng user_addresses
	FromAddressID int64 `json:"from_address_id" binding:"required"`
	
	// ID địa chỉ nhận đã được lưu trong bảng user_addresses
	ToAddressID int64 `json:"to_address_id" binding:"required"`
}

//	@Summary		Provide delivery addresses for exchange
//	@Description	Provides shipping addresses (from and to) for an exchange transaction. Both participants must provide their addresses before proceeding.
//	@Tags			exchanges
//	@Accept			json
//	@Produce		json
//	@Security		accessToken
//	@Param			exchangeID	path		string									true	"Exchange ID"
//	@Param			request		body		provideExchangeDeliveryAddressesRequest	true	"Delivery addresses information"
//	@Success		200			{object}	db.ProvideDeliveryAddressesForExchangeTxResult
//	@Router			/exchanges/{exchangeID}/delivery-addresses [put]
func (server *Server) provideExchangeDeliveryAddresses(c *gin.Context) {
	authPayload := c.MustGet(authorizationPayloadKey).(*token.Payload)
	userID := authPayload.Subject
	
	var req provideExchangeDeliveryAddressesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse(err))
		return
	}
	
	exchangeIDStr := c.Param("exchangeID")
	exchangeID, err := uuid.Parse(exchangeIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, errorResponse(fmt.Errorf("invalid exchange ID: %s", exchangeIDStr)))
		return
	}
	
	// Kiểm tra xem exchange có tồn tại không
	exchange, err := server.dbStore.GetExchangeByID(c.Request.Context(), exchangeID)
	if err != nil {
		if errors.Is(err, db.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, errorResponse(fmt.Errorf("exchange ID %s not found", exchangeIDStr)))
			return
		}
		
		c.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	// Phần 1: Kiểm tra business rules
	
	// Kiểm tra xem người dùng có tham gia vào exchange này không
	isPoster := exchange.PosterID == userID
	isOfferer := exchange.OffererID == userID
	if !isPoster && !isOfferer {
		err = fmt.Errorf("exchange ID %s does not belong to user ID %s", exchangeIDStr, userID)
		c.JSON(http.StatusForbidden, errorResponse(err))
		return
	}
	
	// Kiểm tra trạng thái exchange
	if exchange.Status != "pending" {
		err = fmt.Errorf("exchange ID %s is not in pending status", exchangeIDStr)
		c.JSON(http.StatusUnprocessableEntity, errorResponse(err))
		return
	}
	
	// Kiểm tra xem người dùng đã cung cấp thông tin vận chuyển chưa
	// Chỉ cho phép cung cấp khi các cột thông tin vận chuyển là null
	if isPoster && (exchange.PosterFromDeliveryID != nil || exchange.PosterToDeliveryID != nil) {
		err = fmt.Errorf("user ID %s has already provided shipping information for exchange ID %s", userID, exchangeIDStr)
		c.JSON(http.StatusUnprocessableEntity, errorResponse(err))
		return
	}
	
	if isOfferer && (exchange.OffererFromDeliveryID != nil || exchange.OffererToDeliveryID != nil) {
		err = fmt.Errorf("user ID %s has already provided shipping information for exchange ID %s", userID, exchangeIDStr)
		c.JSON(http.StatusUnprocessableEntity, errorResponse(err))
		return
	}
	
	// Kiểm tra từng địa chỉ
	
	// 1. Kiểm tra địa chỉ gửi
	fromAddress, err := server.dbStore.GetUserAddressByID(c.Request.Context(), db.GetUserAddressByIDParams{
		ID:     req.FromAddressID,
		UserID: userID,
	})
	if err != nil {
		if errors.Is(err, db.ErrRecordNotFound) {
			err = fmt.Errorf("from address ID %d not found for user ID %s", req.FromAddressID, userID)
			c.JSON(http.StatusNotFound, errorResponse(err))
			return
		}
		
		c.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	// Kiểm tra xem địa chỉ có thuộc về người dùng không
	if fromAddress.UserID != userID {
		err = fmt.Errorf("from address ID %d does not belong to user ID %s", req.FromAddressID, userID)
		c.JSON(http.StatusForbidden, errorResponse(err))
		return
	}
	
	// 2. Kiểm tra địa chỉ nhận
	toAddress, err := server.dbStore.GetUserAddressByID(c.Request.Context(), db.GetUserAddressByIDParams{
		ID:     req.ToAddressID,
		UserID: userID,
	})
	if err != nil {
		if errors.Is(err, db.ErrRecordNotFound) {
			err = fmt.Errorf("to address ID %d not found for user ID %s", req.ToAddressID, userID)
			c.JSON(http.StatusNotFound, errorResponse(err))
			return
		}
		
		c.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	// Kiểm tra xem địa chỉ có thuộc về người dùng không
	if toAddress.UserID != userID {
		err = fmt.Errorf("to address ID %d does not belong to user ID %s", req.ToAddressID, userID)
		c.JSON(http.StatusForbidden, errorResponse(err))
		return
	}
	
	// Phần 2: Xử lý transaction
	
	// Chuẩn bị tham số cho transaction
	arg := db.ProvideDeliveryAddressesForExchangeTxParams{
		ExchangeID:  exchangeID,
		UserID:      userID,
		IsPoster:    isPoster,
		FromAddress: fromAddress,
		ToAddress:   toAddress,
	}
	
	// Gọi transaction
	result, err := server.dbStore.ProvideDeliveryAddressesForExchangeTx(c.Request.Context(), arg)
	if err != nil {
		c.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	// Xác định ID của đối tác
	partnerID := ""
	if isPoster {
		partnerID = exchange.OffererID
	} else {
		partnerID = exchange.PosterID
	}
	
	// Kiểm tra xem đối tác đã cung cấp thông tin chưa
	partnerHasProvidedInfo := false
	if isPoster {
		partnerHasProvidedInfo = exchange.OffererFromDeliveryID != nil && exchange.OffererToDeliveryID != nil
	} else {
		partnerHasProvidedInfo = exchange.PosterFromDeliveryID != nil && exchange.PosterToDeliveryID != nil
	}
	
	// Gửi thông báo cho đối tác trong mọi trường hợp
	opts := []asynq.Option{
		asynq.MaxRetry(3),
		asynq.ProcessIn(time.Second * 1),
		asynq.Queue(worker.QueueCritical),
	}
	
	// Chọn nội dung thông báo phù hợp dựa trên việc đối tác đã cung cấp thông tin chưa
	title := "Đối tác đã cung cấp thông tin vận chuyển"
	var message string
	
	if partnerHasProvidedInfo {
		message = fmt.Sprintf("Đối tác đã cung cấp thông tin vận chuyển cho giao dịch trao đổi %s. Bây giờ bạn có thể xem chi tiết và tiến hành thanh toán phí vận chuyển.", exchangeID)
	} else {
		message = fmt.Sprintf("Đối tác đã cung cấp thông tin vận chuyển cho giao dịch trao đổi %s. Vui lòng cung cấp thông tin vận chuyển của bạn để tiếp tục quá trình trao đổi.", exchangeID)
	}
	
	err = server.taskDistributor.DistributeTaskSendNotification(c.Request.Context(), &worker.PayloadSendNotification{
		RecipientID: partnerID,
		Title:       title,
		Message:     message,
		Type:        "exchange",
		ReferenceID: exchangeID.String(),
	}, opts...)
	if err != nil {
		log.Error().Err(err).Msg("failed to send notification to partner")
	} else {
		log.Info().Msgf("Notification sent to partner: %s", partnerID)
	}
	
	c.JSON(http.StatusOK, result)
}

type payExchangeDeliveryFeeRequest struct {
	DeliveryFee          int64     `json:"delivery_fee" binding:"required,min=1"`
	ExpectedDeliveryTime time.Time `json:"expected_delivery_time" binding:"required"`
	Note                 *string   `json:"note"`
}

//	@Summary		Pay delivery fee for exchange
//	@Description	Pays the delivery fee for an exchange transaction. When both parties have paid, the system creates two orders.
//	@Tags			exchanges
//	@Accept			json
//	@Produce		json
//	@Security		accessToken
//	@Param			exchangeID	path		string							true	"Exchange ID"
//	@Param			request		body		payExchangeDeliveryFeeRequest	true	"Delivery fee information"
//	@Success		200			{object}	db.PayExchangeDeliveryFeeTxResult
//	@Router			/exchanges/{exchangeID}/pay-delivery-fee [post]
func (server *Server) payExchangeDeliveryFee(c *gin.Context) {
	authPayload := c.MustGet(authorizationPayloadKey).(*token.Payload)
	userID := authPayload.Subject
	
	var req payExchangeDeliveryFeeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse(err))
		return
	}
	
	// Parse exchange ID from path
	exchangeIDStr := c.Param("exchangeID")
	exchangeID, err := uuid.Parse(exchangeIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, errorResponse(fmt.Errorf("invalid exchange ID: %s", exchangeIDStr)))
		return
	}
	
	// Get exchange details
	exchange, err := server.dbStore.GetExchangeByID(c.Request.Context(), exchangeID)
	if err != nil {
		if errors.Is(err, db.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, errorResponse(fmt.Errorf("exchange ID %s not found", exchangeIDStr)))
			return
		}
		
		c.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	// Verify user is part of the exchange
	isPoster := exchange.PosterID == userID
	isOfferer := exchange.OffererID == userID
	if !isPoster && !isOfferer {
		err = fmt.Errorf("exchange ID %s does not belong to user ID %s", exchangeIDStr, userID)
		c.JSON(http.StatusForbidden, errorResponse(err))
		return
	}
	
	// Check exchange status
	if exchange.Status != db.ExchangeStatusPending {
		err = fmt.Errorf("exchange ID %s is not in pending status", exchangeIDStr)
		c.JSON(http.StatusUnprocessableEntity, errorResponse(err))
		return
	}
	
	// Check if delivery addresses have been provided by both parties
	if exchange.PosterFromDeliveryID == nil || exchange.PosterToDeliveryID == nil ||
		exchange.OffererFromDeliveryID == nil || exchange.OffererToDeliveryID == nil {
		err = fmt.Errorf("both parties must provide delivery addresses before payment")
		c.JSON(http.StatusUnprocessableEntity, errorResponse(err))
		return
	}
	
	// Check if user has already paid
	if (isPoster && exchange.PosterDeliveryFeePaid) || (isOfferer && exchange.OffererDeliveryFeePaid) {
		err = fmt.Errorf("user ID %s has already paid the delivery fee for exchange ID %s", userID, exchangeIDStr)
		c.JSON(http.StatusUnprocessableEntity, errorResponse(err))
		return
	}
	
	// Get delivery fee amount from request
	deliveryFee := req.DeliveryFee
	
	// Check user's wallet balance
	wallet, err := server.dbStore.GetWalletByUserID(c.Request.Context(), userID)
	if err != nil {
		if errors.Is(err, db.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, errorResponse(fmt.Errorf("wallet not found for user ID %s", userID)))
			return
		}
		
		c.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	if wallet.Balance < deliveryFee {
		err = fmt.Errorf("insufficient balance to pay delivery fee: required %d, available %d",
			deliveryFee, wallet.Balance)
		c.JSON(http.StatusUnprocessableEntity, errorResponse(err))
		return
	}
	
	// Execute transaction
	arg := db.PayExchangeDeliveryFeeTxParams{
		ExchangeID:           exchangeID,
		UserID:               userID,
		IsPoster:             isPoster,
		DeliveryFee:          deliveryFee,
		Note:                 req.Note,
		ExpectedDeliveryTime: req.ExpectedDeliveryTime,
	}
	
	result, err := server.dbStore.PayExchangeDeliveryFeeTx(c.Request.Context(), arg)
	if err != nil {
		c.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	// Gửi thông báo về việc thanh toán phí vận chuyển thành công cho các bên liên quan
	opts := []asynq.Option{
		asynq.MaxRetry(3),
		asynq.ProcessIn(time.Second * 1),
		asynq.Queue(worker.QueueCritical),
	}
	
	// Tạo mã cuộc trao đổi ngắn gọn để hiển thị trong thông báo
	exchangeCode := arg.ExchangeID.String()[:8] // Lấy 8 ký tự đầu của UUID làm mã tham chiếu
	
	// Khởi tạo tên người dùng mặc định
	currentUserName := "Đối tác của bạn"
	
	// Lấy thông tin người dùng hiện tại
	currentUser, err := server.dbStore.GetUserByID(c.Request.Context(), userID)
	if err == nil {
		currentUserName = currentUser.FullName
	} else {
		log.Error().Err(err).Msgf("failed to get current user %s", userID)
	}
	
	// 1. Gửi thông báo cho người thanh toán về việc thanh toán thành công
	paymentSuccessMsg := fmt.Sprintf("Bạn đã thanh toán phí vận chuyển %s cho cuộc trao đổi #%s thành công.",
		util.FormatVND(arg.DeliveryFee), exchangeCode)
	err = server.taskDistributor.DistributeTaskSendNotification(c.Request.Context(), &worker.PayloadSendNotification{
		RecipientID: userID,
		Title:       "Thanh toán phí vận chuyển thành công",
		Message:     paymentSuccessMsg,
		Type:        "exchange",
		ReferenceID: arg.ExchangeID.String(),
	}, opts...)
	if err != nil {
		log.Error().Err(err).Msg("failed to send payment success notification")
	} else {
		log.Info().Msgf("payment success notification sent to user: %s", userID)
	}
	
	// 2. Xác định đối tác và gửi thông báo cho họ nếu chưa thanh toán
	var partnerID string
	if arg.IsPoster {
		partnerID = result.Exchange.OffererID
	} else {
		partnerID = result.Exchange.PosterID
	}
	
	if !result.PartnerHasPaid {
		// Đối tác chưa thanh toán, thông báo cho họ biết để thanh toán
		partnerMsg := fmt.Sprintf("%s đã thanh toán phí vận chuyển %s cho cuộc trao đổi #%s. Vui lòng thanh toán phí vận chuyển của bạn để tiếp tục quá trình trao đổi.",
			currentUserName, util.FormatVND(arg.DeliveryFee), exchangeCode)
		
		err = server.taskDistributor.DistributeTaskSendNotification(c.Request.Context(), &worker.PayloadSendNotification{
			RecipientID: partnerID,
			Title:       "Đối tác đã thanh toán phí vận chuyển",
			Message:     partnerMsg,
			Type:        "exchange",
			ReferenceID: arg.ExchangeID.String(),
		}, opts...)
		if err != nil {
			log.Error().Err(err).Msg("failed to send notification to partner")
		} else {
			log.Info().Msgf("notification sent to partner: %s", partnerID)
		}
	}
	
	// 3. Nếu cả hai bên đã thanh toán, gửi thông báo về việc tạo đơn hàng
	if result.BothPartiesPaid {
		// Kiểm tra xem các đơn hàng đã được tạo chưa
		if result.PosterOrder == nil || result.OffererOrder == nil {
			log.Error().Msg("order information is missing in the result")
			return
		}
		
		// Lấy thông tin đơn hàng từ result
		posterOrderCode := result.PosterOrder.Code
		offererOrderCode := result.OffererOrder.Code
		
		// Thông báo cho người đăng bài (poster)
		posterOrderMsg := fmt.Sprintf("Cả hai bên đã thanh toán phí vận chuyển cho cuộc trao đổi #%s. Đơn hàng #%s của bạn đã được tạo. Vui lòng đóng gói để đơn vị vận chuyển lại lấy hàng.",
			exchangeCode, posterOrderCode)
		
		err = server.taskDistributor.DistributeTaskSendNotification(c.Request.Context(), &worker.PayloadSendNotification{
			RecipientID: result.Exchange.PosterID,
			Title:       "Đơn hàng trao đổi đã được tạo",
			Message:     posterOrderMsg,
			Type:        "exchange",
			ReferenceID: result.OffererOrder.ID.String(), // OrderID mà poster cần gửi hàng
		}, opts...)
		if err != nil {
			log.Error().Err(err).Msg("failed to send order creation notification to poster")
		} else {
			log.Info().Msgf("order creation notification sent to poster: %s", result.Exchange.PosterID)
		}
		
		// Thông báo cho người đề xuất (offerer)
		offererOrderMsg := fmt.Sprintf("Cả hai bên đã thanh toán phí vận chuyển cho cuộc trao đổi #%s. Đơn hàng #%s của bạn đã được tạo. Vui lòng đóng gói để đơn vị vận chuyển lại lấy hàng.",
			exchangeCode, offererOrderCode)
		
		err = server.taskDistributor.DistributeTaskSendNotification(c.Request.Context(), &worker.PayloadSendNotification{
			RecipientID: result.Exchange.OffererID,
			Title:       "Đơn hàng trao đổi đã được tạo",
			Message:     offererOrderMsg,
			Type:        "exchange",
			ReferenceID: result.PosterOrder.ID.String(), // OrderID mà offerer cần gửi hàng
		}, opts...)
		if err != nil {
			log.Error().Err(err).Msg("failed to send order creation notification to offerer")
		} else {
			log.Info().Msgf("order creation notification sent to offerer: %s", result.Exchange.OffererID)
		}
	}
	
	c.JSON(http.StatusOK, result)
}

type cancelExchangeRequest struct {
	Reason *string `json:"reason"`
}

//	@Summary		Cancel an exchange
//	@Description	Cancel an exchange transaction that is in pending or packaging status
//	@Tags			exchanges
//	@Accept			json
//	@Produce		json
//	@Param			exchangeID	path		string						true	"Exchange ID"	example(123e4567-e89b-12d3-a456-426614174000)
//	@Param			request		body		cancelExchangeRequest		true	"Cancel exchange request"
//	@Success		200			{object}	db.CancelExchangeTxResult	"Exchange canceled successfully"
//	@Security		accessToken
//	@Router			/exchanges/{exchangeID}/cancel [patch]
func (server *Server) cancelExchange(c *gin.Context) {
	authPayload := c.MustGet(authorizationPayloadKey).(*token.Payload)
	userID := authPayload.Subject
	
	// Parse exchange ID
	exchangeIDStr := c.Param("exchangeID")
	exchangeID, err := uuid.Parse(exchangeIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, errorResponse(fmt.Errorf("invalid exchange ID format: %s", exchangeIDStr)))
		return
	}
	
	// Parse request body
	var req cancelExchangeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse(err))
		return
	}
	
	// Validate reason if provided
	if req.Reason != nil && *req.Reason == "" {
		c.JSON(http.StatusBadRequest, errorResponse(fmt.Errorf("reason cannot be empty if provided")))
		return
	}
	
	// Get exchange
	exchange, err := server.dbStore.GetExchangeByID(c, exchangeID)
	if err != nil {
		if errors.Is(err, db.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, errorResponse(fmt.Errorf("exchange ID %s not found", exchangeID)))
			return
		}
		
		log.Err(err).Msg("failed to get exchange by ID")
		c.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	// Check if user is part of the exchange
	if exchange.PosterID != userID && exchange.OffererID != userID {
		c.JSON(http.StatusForbidden, errorResponse(fmt.Errorf("exchange %s does not belong to user %s", exchangeID, userID)))
		return
	}
	
	// Check if exchange can be canceled (only in "pending" status)
	if exchange.Status != db.ExchangeStatusPending {
		c.JSON(http.StatusUnprocessableEntity, errorResponse(fmt.Errorf("exchange can only be canceled in 'pending' status, current status: %s", exchange.Status)))
		return
	}
	
	// Execute transaction to cancel the exchange
	result, err := server.dbStore.CancelExchangeTx(c, db.CancelExchangeTxParams{
		ExchangeID: exchangeID,
		UserID:     userID,
		Reason:     req.Reason,
	})
	if err != nil {
		log.Err(err).Msg("failed to cancel exchange")
		c.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	// Send notifications to both parties
	server.sendExchangeCancelNotifications(c, result, userID)
	
	c.JSON(http.StatusOK, result)
}
