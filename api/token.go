package api

import (
	"net/http"
	
	"github.com/gin-gonic/gin"
)

type verifyAccessTokenRequest struct {
	AccessToken string `json:"access_token"`
}

//	@Summary		Verify access token
//	@Description	Verifies a JWT access token and returns the associated user
//	@Tags			authentication
//	@Accept			json
//	@Produce		json
//	@Param			request	body		verifyAccessTokenRequest	true	"Token verification request"
//	@Success		200		{object}	db.User						"User information"
//	@Failure		400		"Invalid request format"
//	@Failure		401		"Invalid or expired token"
//	@Router			/tokens/verify [post]
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
	
	user, err := server.dbStore.GetUserByID(c, claims.Subject)
	if err != nil {
		c.JSON(http.StatusUnauthorized, errorResponse(err))
		return
	}
	
	c.JSON(http.StatusOK, user)
}
