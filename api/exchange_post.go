package api

import (
	"errors"
	"fmt"
	"mime/multipart"
	"net/http"
	
	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	db "github.com/katatrina/gundam-BE/internal/db/sqlc"
	"github.com/katatrina/gundam-BE/internal/token"
)

type createExchangePostRequest struct {
	Content     string                  `form:"content" binding:"required"`
	PostImages  []*multipart.FileHeader `form:"post_images" binding:"required"`
	PostItemIDs []int64                 `form:"post_item_id" binding:"required"`
}

//	@Summary		Create a new exchange post
//	@Description	Create a new exchange post.
//	@Tags			exchanges
//	@Accept			json
//	@Produce		json
//	@Security		accessToken
//	@Param			request	body		createExchangePostRequest		true	"Create exchange post request"
//	@Success		201		{object}	db.CreateExchangePostTxResult	"Create exchange post response"
//	@Router			/users/me/exchange-posts [post]
func (server *Server) createExchangePost(c *gin.Context) {
	authPayload := c.MustGet(authorizationPayloadKey).(*token.Payload)
	userID := authPayload.Subject
	
	_, err := server.dbStore.GetUserByID(c.Request.Context(), userID)
	if err != nil {
		if errors.Is(err, db.ErrRecordNotFound) {
			err = fmt.Errorf("user ID %s not found", userID)
			c.JSON(http.StatusNotFound, errorResponse(err))
			return
		}
		
		c.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	var req createExchangePostRequest
	if err := c.ShouldBindWith(&req, binding.FormMultipart); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse(err))
		return
	}
	
	// Kiểm tra xem người dùng có đang sở hữu các Gundam trong post_item_ids không
	// và có thể thêm các Gundam này vào bài post không.
	for _, itemID := range req.PostItemIDs {
		gundam, err := server.dbStore.GetGundamByID(c.Request.Context(), itemID)
		if err != nil {
			if errors.Is(err, db.ErrRecordNotFound) {
				err = fmt.Errorf("gundam ID %d not found", itemID)
				c.JSON(http.StatusNotFound, errorResponse(err))
				return
			}
			
			c.JSON(http.StatusInternalServerError, errorResponse(err))
			return
		}
		
		if gundam.OwnerID != userID {
			err = fmt.Errorf("user ID %s does not own gundam ID %d", userID, itemID)
			c.JSON(http.StatusUnprocessableEntity, errorResponse(err))
			return
		}
		
		if gundam.Status != db.GundamStatusInstore {
			err = fmt.Errorf("gundam ID %d is not available for exchange", itemID)
			c.JSON(http.StatusUnprocessableEntity, errorResponse(err))
			return
		}
	}
	
	result, err := server.dbStore.CreateExchangePostTx(c.Request.Context(), db.CreateExchangePostTxParams{
		UserID:           userID,
		Content:          req.Content,
		PostImages:       req.PostImages,
		PostItemIDs:      req.PostItemIDs,
		UploadImagesFunc: server.uploadFileToCloudinary,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	c.JSON(http.StatusCreated, result)
}
