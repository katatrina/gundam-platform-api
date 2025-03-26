package api

import (
	"errors"
	"net/http"
	
	"github.com/gin-gonic/gin"
	db "github.com/katatrina/gundam-BE/internal/db/sqlc"
)

type checkEmailRequest struct {
	Email string `json:"email" binding:"required,email"`
}

//	@Summary		Kiểm tra sự tồn tại của email
//	@Description	Kiểm tra xem email đã được đăng ký trong hệ thống chưa
//	@Tags			authentication
//	@Accept			json
//	@Produce		json
//	@Param			email	query		string	true	"Email cần kiểm tra"
//	@Success		200		{object}	map[string]bool
//	@Router			/check-email [get]
func (server *Server) checkEmailExists(ctx *gin.Context) {
	var req checkEmailRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, errorResponse(err))
		return
	}
	
	_, err := server.dbStore.GetUserByEmail(ctx, req.Email)
	if err != nil {
		if errors.Is(err, db.ErrRecordNotFound) {
			ctx.JSON(http.StatusOK, gin.H{
				"exists": false,
			})
			
			return
		}
		
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	ctx.JSON(http.StatusOK, gin.H{
		"exists": true,
	})
}
