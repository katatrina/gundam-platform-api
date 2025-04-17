package api

import (
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

func (server *Server) createExchangePost(c *gin.Context) {
	authPayload := c.MustGet(authorizationPayloadKey).(*token.Payload)
	userID := authPayload.Subject
	
	var req createExchangePostRequest
	if err := c.ShouldBindWith(&req, binding.FormMultipart); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse(err))
		return
	}
	
	// TODO: Kiểm tra xem người dùng có đang sở hữu các Gundam trong post_item_ids không
	// và có thể thêm các Gundam này vào post không
	
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
