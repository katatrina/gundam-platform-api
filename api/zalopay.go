package api

import (
	"net/http"
	
	"github.com/gin-gonic/gin"
	"github.com/katatrina/gundam-BE/internal/token"
)

type createZaloPayOrderRequest struct {
	Amount      int64  `json:"amount" binding:"required"`
	Description string `json:"description"`
}

//	@Summary		Create a ZaloPay order
//	@Description	Create a ZaloPay order
//	@Tags			wallet
//	@Accept			json
//	@Produce		json
//	@Security		accessToken
//	@Param			request	body		createZaloPayOrderRequest			true	"Create ZaloPay order request"
//	@Success		200		{object}	zalopay.CreateOrderZaloPayResponse	"Create ZaloPay order response"
//	@Failure		400		"Bad request"
//	@Failure		500		"Internal server error"
//	@Router			/wallet/zalopay/create [post]
func (server *Server) createZalopayOrder(c *gin.Context) {
	authPayload := c.MustGet(authorizationPayloadKey).(*token.Payload)
	appUser := authPayload.Subject
	
	var req createZaloPayOrderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse(err))
		return
	}
	
	// Tạo đơn hàng Zalopay
	// items tạm thời là nil
	result, err := server.zalopayService.CreateOrder(appUser, req.Amount, nil, req.Description)
	if err != nil {
		c.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	c.JSON(http.StatusOK, result)
}
