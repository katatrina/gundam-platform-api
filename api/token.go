package api

import (
	"net/http"
	"strconv"
	
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
	
	userID, err := strconv.ParseInt(claims.Subject, 10, 64)
	if err != nil {
		c.JSON(http.StatusUnauthorized, errorResponse(err))
		return
	}
	
	user, err := server.store.GetUserByID(c, userID)
	if err != nil {
		c.JSON(http.StatusUnauthorized, errorResponse(err))
		return
	}
	
	c.JSON(http.StatusOK, user)
}
