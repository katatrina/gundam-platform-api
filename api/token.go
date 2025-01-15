package api

import (
	"net/http"
	
	"github.com/gin-gonic/gin"
)

type verifyAccessTokenRequest struct {
	AccessToken string `json:"access_token"`
}

func (server *Server) verifyAccessToken(c *gin.Context) {
	req := new(verifyAccessTokenRequest)
	
	if err := c.ShouldBindJSON(req); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse(err))
		return
	}
	
	claims, err := server.tokenMaker.VerifyToken(req.AccessToken)
	if err != nil {
		c.JSON(http.StatusUnauthorized, errorResponse(err))
		return
	}
	
	user, err := server.store.GetUserByID(c, claims.Subject)
	if err != nil {
		c.JSON(http.StatusUnauthorized, errorResponse(err))
		return
	}
	
	c.JSON(http.StatusOK, user)
}
