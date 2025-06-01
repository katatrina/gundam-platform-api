package api

import (
	"fmt"
	"net/http"
	
	"github.com/gin-gonic/gin"
	db "github.com/katatrina/gundam-BE/internal/db/sqlc"
	"golang.org/x/sync/errgroup"
)

//	@Summary		Get admin dashboard statistics
//	@Description	Get comprehensive dashboard statistics for admin including business metrics with order type breakdowns
//	@Tags			admin
//	@Produce		json
//	@Security		accessToken
//	@Success		200	{object}	db.AdminDashboard	"Dashboard statistics with order type details"
//	@Router			/admin/dashboard [get]
func (server *Server) getAdminDashboard(c *gin.Context) {
	_ = c.MustGet(adminPayloadKey).(*db.User)
	
	var resp db.AdminDashboard
	
	g, ctx := errgroup.WithContext(c.Request.Context())
	
	// Goroutine 1: Get total business users (members + sellers)
	g.Go(func() error {
		totalBusinessUsers, err := server.dbStore.GetAdminTotalBusinessUsers(ctx)
		if err != nil {
			return fmt.Errorf("failed to get total active users: %w", err)
		}
		resp.TotalBusinessUsers = totalBusinessUsers
		return nil
	})
	
	// Goroutine 2: Get total regular orders this month
	g.Go(func() error {
		totalRegularOrders, err := server.dbStore.GetAdminTotalRegularOrdersThisMonth(ctx)
		if err != nil {
			return fmt.Errorf("failed to get total regular orders this month: %w", err)
		}
		resp.TotalRegularOrdersThisMonth = totalRegularOrders
		return nil
	})
	
	// Goroutine 3: Get total exchange orders this month
	g.Go(func() error {
		totalExchangeOrders, err := server.dbStore.GetAdminTotalExchangeOrdersThisMonth(ctx)
		if err != nil {
			return fmt.Errorf("failed to get total exchange orders this month: %w", err)
		}
		resp.TotalExchangeOrdersThisMonth = totalExchangeOrders
		return nil
	})
	
	// Goroutine 4: Get total auction orders this month
	g.Go(func() error {
		totalAuctionOrders, err := server.dbStore.GetAdminTotalAuctionOrdersThisMonth(ctx)
		if err != nil {
			return fmt.Errorf("failed to get total auction orders this month: %w", err)
		}
		resp.TotalAuctionOrdersThisMonth = totalAuctionOrders
		return nil
	})
	
	// Goroutine 5: Get total revenue this month
	g.Go(func() error {
		totalRevenue, err := server.dbStore.GetAdminTotalRevenueThisMonth(ctx)
		if err != nil {
			return fmt.Errorf("failed to get total revenue this month: %w", err)
		}
		resp.TotalRevenueThisMonth = totalRevenue
		return nil
	})
	
	// Goroutine 6: Get completed exchanges this month
	g.Go(func() error {
		completedExchanges, err := server.dbStore.GetAdminCompletedExchangesThisMonth(ctx)
		if err != nil {
			return fmt.Errorf("failed to get completed exchanges this month: %w", err)
		}
		resp.CompletedExchangesThisMonth = completedExchanges
		return nil
	})
	
	// Goroutine 7: Get completed auctions this week
	g.Go(func() error {
		completedAuctions, err := server.dbStore.GetAdminCompletedAuctionsThisWeek(ctx)
		if err != nil {
			return fmt.Errorf("failed to get completed auctions this week: %w", err)
		}
		resp.CompletedAuctionsThisWeek = completedAuctions
		return nil
	})
	
	// Goroutine 8: Get total wallet volume this week
	g.Go(func() error {
		totalWalletVolume, err := server.dbStore.GetAdminTotalWalletVolumeThisWeek(ctx)
		if err != nil {
			return fmt.Errorf("failed to get total wallet volume this week: %w", err)
		}
		resp.TotalWalletVolumeThisWeek = totalWalletVolume
		return nil
	})
	
	// Goroutine 9: Get total published gundams
	g.Go(func() error {
		totalPublishedGundams, err := server.dbStore.GetAdminTotalPublishedGundams(ctx)
		if err != nil {
			return fmt.Errorf("failed to get total published gundams: %w", err)
		}
		resp.TotalPublishedGundams = totalPublishedGundams
		return nil
	})
	
	// Goroutine 10: Get new users this week
	g.Go(func() error {
		newUsers, err := server.dbStore.GetAdminNewUsersThisWeek(ctx)
		if err != nil {
			return fmt.Errorf("failed to get new users this week: %w", err)
		}
		resp.NewUsersThisWeek = newUsers
		return nil
	})
	
	// Wait for all goroutines to complete
	if err := g.Wait(); err != nil {
		c.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	c.JSON(http.StatusOK, resp)
}
