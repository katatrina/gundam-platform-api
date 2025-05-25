package api

import (
	"fmt"
	"net/http"
	
	"github.com/gin-gonic/gin"
)

// 
//	@Summary		List all subscription plans
//	@Description	Get a list of all available subscription plans.
//	@Tags			sellers
//	@Produce		json
//	@Success		200	{array}	db.SubscriptionPlan	"All available subscription plans"
//	@Router			/subscription-plans [get]
func (server *Server) listSubscriptionPlans(c *gin.Context) () {
	plans, err := server.dbStore.ListSubscriptionPlans(c.Request.Context())
	if err != nil {
		err = fmt.Errorf("failed to list subscription plans: %w", err)
		c.JSON(http.StatusInternalServerError, err)
		return
	}
	
	c.JSON(http.StatusOK, plans)
}
