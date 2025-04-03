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

//	@Summary		Check Email Exists
//	@Description	Checks if an email already exists in the database
//	@Tags			authentication
//	@Accept			json
//	@Produce		json
//	@Param			request	body		checkEmailRequest	true	"Check email request"
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
