package api

import (
	"context"
	"fmt"
	
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	db "github.com/katatrina/gundam-BE/internal/db/sqlc"
	"github.com/katatrina/gundam-BE/internal/delivery"
	"github.com/katatrina/gundam-BE/internal/mailer"
	"github.com/katatrina/gundam-BE/internal/phone_number"
	"github.com/katatrina/gundam-BE/internal/storage"
	"github.com/katatrina/gundam-BE/internal/token"
	"github.com/katatrina/gundam-BE/internal/util"
	"github.com/katatrina/gundam-BE/internal/worker"
	"github.com/katatrina/gundam-BE/internal/zalopay"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog/log"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	"google.golang.org/api/idtoken"
)

type Server struct {
	router                 *gin.Engine
	dbStore                db.Store
	fileStore              storage.FileStore
	tokenMaker             token.Maker
	config                 *util.Config
	googleIDTokenValidator *idtoken.Validator
	phoneNumberService     *phone_number.PhoneNumberService
	mailService            *mailer.GmailSender
	taskDistributor        *worker.RedisTaskDistributor
	zalopayService         *zalopay.ZalopayService
	deliveryService        delivery.IDeliveryProvider
}

// NewServer creates a new HTTP server and set up routing.
func NewServer(store db.Store, redisDb *redis.Client, taskDistributor *worker.RedisTaskDistributor, config *util.Config, mailer *mailer.GmailSender, deliveryService delivery.IDeliveryProvider) (*Server, error) {
	// Create a new JWT token maker
	tokenMaker, err := token.NewJWTMaker(config.TokenSecretKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create token maker: %w", err)
	}
	log.Info().Msg("Token maker created successfully ✅")
	
	// Create a new Google ID token validator
	googleIDTokenValidator, err := idtoken.NewValidator(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to create google id token validator: %w", err)
	}
	
	// Create a new Cloudinary instance
	fileStore := storage.NewCloudinaryStore(config.CloudinaryURL)
	log.Info().Msg("Cloudinary store created successfully ✅")
	
	// Create a new OTP service
	phoneNumberService, err := phone_number.NewPhoneService(config, redisDb)
	if err != nil {
		return nil, fmt.Errorf("failed to create phone service: %w", err)
	}
	log.Info().Msg("Phone service created successfully ✅")
	
	// Create a new ZaloPay service
	zalopayService := zalopay.NewZalopayService(store, config)
	log.Info().Msg("ZaloPay service created successfully ✅")
	
	server := &Server{
		dbStore:                store,
		tokenMaker:             tokenMaker,
		config:                 config,
		googleIDTokenValidator: googleIDTokenValidator,
		fileStore:              fileStore,
		phoneNumberService:     phoneNumberService,
		mailService:            mailer,
		taskDistributor:        taskDistributor,
		zalopayService:         zalopayService,
		deliveryService:        deliveryService,
	}
	
	server.setupRouter()
	return server, nil
}

// setupRouter configures the HTTP server routes.
func (server *Server) setupRouter() *gin.Engine {
	gin.ForceConsoleColor()
	router := gin.Default()
	router.Use(cors.New(cors.Config{
		AllowOrigins:     server.config.AllowedOrigins,
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE"},
		AllowHeaders:     []string{"Origin", "Content-Length", "Content-Type", "Authorization"},
		AllowCredentials: true,
	}))
	router.Use(func(c *gin.Context) {
		c.Header("Cross-Origin-Opener-Policy", "same-origin same-origin-allow-popups")
		c.Header("Cross-Origin-Embedder-Policy", "unsafe-none")
		c.Next()
	})
	
	v1 := router.Group("/v1")
	
	v1.POST("/tokens/verify", server.verifyAccessToken)
	
	v1.POST("/auth/login", server.loginUser)
	v1.POST("/auth/google-login", server.loginUserWithGoogle)
	
	userGroup := v1.Group("/users")
	{
		userGroup.POST("", server.createUser)
		userGroup.GET(":id", server.getUser)
		userGroup.PUT(":id", server.updateUser)
		userGroup.GET("by-phone", server.getUserByPhoneNumber)
		userGroup.PATCH(":id/avatar", server.updateAvatar)
		userGroup.GET(":id/addresses/pickup", server.getUserPickupAddress)
		userGroup.GET(":id/addresses", server.listUserAddresses)
		userGroup.POST(":id/addresses", server.createUserAddress)
		userGroup.PUT(":id/addresses/:address_id", server.updateUserAddress)
		userGroup.DELETE(":id/addresses/:address_id", server.deleteUserAddress)
		
		userGroup.Use(authMiddleware(server.tokenMaker))
		userGroup.POST("become-seller", server.becomeSeller)
		
		userGroup.GET(":id/wallet", server.getUserWallet)
		
		userGundamGroup := userGroup.Group(":id/gundams")
		{
			userGundamGroup.POST("", server.createGundam)
			userGundamGroup.GET("", server.listGundamsByUser)
		}
		
		// API cho bài đăng trao đổi của người dùng hiện tại (đã đăng nhập)
		userExchangePostGroup := userGroup.Group("/me/exchange-posts")
		{
			// // Liệt kê các bài đăng trao đổi của người dùng hiện tại
			// userExchangePostGroup.GET("", server.listUserExchangePosts)
			//
			// Tạo bài đăng trao đổi mới
			userExchangePostGroup.POST("", server.createExchangePost)
			//
			// // Lấy chi tiết một bài đăng trao đổi cụ thể của người dùng hiện tại
			// userExchangePostGroup.GET("/:id", server.getMyExchangePostDetails)
			//
			// // Chỉnh sửa bài đăng trao đổi
			// userExchangePostGroup.PUT("/:id", server.updateExchangePost)
			//
			// // Đóng/hủy bài đăng trao đổi
			// userExchangePostGroup.PATCH("/:id/close", server.closeExchangePost)
			
			// API cho đề xuất trao đổi của một bài đăng
			// offerGroup := userExchangePostGroup.Group("/:id/offers")
			// {
			// 	// Liệt kê các đề xuất cho bài đăng trao đổi
			// 	offerGroup.GET("", server.listExchangeOffers)
			//
			// 	// Chấp nhận đề xuất trao đổi
			// 	offerGroup.PATCH("/:offerID/accept", server.acceptExchangeOffer)
			//
			// 	// Từ chối đề xuất trao đổi
			// 	offerGroup.PATCH("/:offerID/reject", server.rejectExchangeOffer)
			// }
		}
		
		// // API cho đề xuất trao đổi của người dùng hiện tại
		// userOffersGroup := userGroup.Group("/me/exchange-offers")
		// {
		// 	// Liệt kê đề xuất trao đổi mà người dùng đã gửi
		// 	userOffersGroup.GET("", server.listMyExchangeOffers)
		//
		// 	// Tạo đề xuất trao đổi cho một bài đăng
		// 	userOffersGroup.POST("", server.createExchangeOffer)
		//
		// 	// Hủy đề xuất trao đổi
		// 	userOffersGroup.PATCH("/:offerID/cancel", server.cancelExchangeOffer)
		// }
	}
	
	// API public cho bài đăng trao đổi - không cần đăng nhập
	// exchangePostPublicGroup := v1.Group("/exchange-posts")
	{
		// Liệt kê các bài post trao đổi đang mở trên nền tảng
		// exchangePostPublicGroup.GET("", server.listOpenExchangePosts)
		
		// Lấy chi tiết một bài post trao đổi
		// exchangePostPublicGroup.GET("/:id", server.getExchangePostDetails)
	}
	
	// Nhóm api cho các đơn hàng thông thường và đơn hàng trao đổi
	orderGroup := v1.Group("/orders", authMiddleware(server.tokenMaker))
	{
		// Tạo đơn hàng mua thông thường khi user thanh toán thành công
		// Client cần gọi api này nhiều lần để tạo nhiều đơn hàng nếu có nhiều sản phẩm thuộc nhiều seller khác nhau trong giỏ hàng
		orderGroup.POST("", server.createOrder)                            // ✅ Tạo đơn hàng thông thường
		orderGroup.GET("", server.listMemberOrders)                        // ✅ Liệt kê tất cả đơn hàng thông thường và đơn hàng trao đổi trong tab "Đơn hàng" trong trang "Tài khoản của tôi"
		orderGroup.GET(":orderID", server.getMemberOrderDetails)           // ✅ Lấy thông tin chi tiết của một đơn hàng thông thường hoặc đơn hàng trao đổi
		orderGroup.PATCH(":orderID/package", server.packageOrder)          // Người gửi đóng gói đơn hàng
		orderGroup.PATCH(":orderID/received", server.confirmOrderReceived) // Người nhận hàng xác nhận đã nhận hàng thành công
		orderGroup.PATCH(":orderID/cancel", server.cancelOrderByBuyer)     // Người mua hủy đơn hàng
	}
	
	// Nhóm các API chỉ dành cho seller
	sellerGroup := v1.Group("/sellers/:sellerID", authMiddleware(server.tokenMaker), requiredSellerRole(server.dbStore))
	{
		// Nhóm các API chỉ liên quan đến đơn bán (không bao gồm đơn hàng trao đổi)
		sellerOrderGroup := sellerGroup.Group("orders")
		{
			sellerOrderGroup.GET("", server.listSalesOrders)                      // ✅ Liệt kê tất cả đơn bán
			sellerOrderGroup.GET(":orderID", server.getSalesOrderDetails)         // ✅ Lấy thông tin chi tiết của một đơn bán
			sellerOrderGroup.PATCH(":orderID/confirm", server.confirmOrder)       // ✅ Người bán xác nhận sẽ gửi đơn hàng
			sellerOrderGroup.PATCH(":orderID/cancel", server.cancelOrderBySeller) // Người bán hủy đơn hàng
		}
		
		gundamGroup := sellerGroup.Group("gundams")
		{
			gundamGroup.PATCH(":gundamID/publish", server.publishGundam)
			gundamGroup.PATCH(":gundamID/unpublish", server.unpublishGundam)
		}
		
		subscriptionGroup := sellerGroup.Group("subscriptions")
		{
			subscriptionGroup.GET("active", server.getCurrentActiveSubscription)
		}
	}
	
	walletGroup := v1.Group("/wallet", authMiddleware(server.tokenMaker))
	{
		zalopayGroup := walletGroup.Group("/zalopay")
		{
			zalopayGroup.POST("/create", server.createZalopayOrder)
		}
	}
	
	v1.GET("/grades", server.listGundamGrades)
	
	sellerProfileGroup := v1.Group("/seller/profile")
	{
		sellerProfileGroup.POST("", server.createSellerProfile)
		sellerProfileGroup.GET("", server.getSellerProfile)
		sellerProfileGroup.PATCH("", server.updateSellerProfile)
	}
	
	gundamGroup := v1.Group("/gundams")
	{
		gundamGroup.GET("", server.listGundams)
		gundamGroup.GET(":slug", server.getGundamBySlug)
	}
	
	cartGroup := v1.Group("/cart", authMiddleware(server.tokenMaker))
	{
		cartGroup.POST("/items", server.addCartItem)
		cartGroup.GET("/items", server.listCartItems)
		cartGroup.DELETE("/items/:id", server.deleteCartItem)
	}
	
	otpGroup := v1.Group("/otp")
	{
		otpGroup.POST("/phone-number/generate", server.generatePhoneNumberOTP)
		otpGroup.POST("/phone-number/verify", server.verifyPhoneNumberOTP)
		
		otpGroup.POST("/email/generate", server.generateEmailOTP)
		otpGroup.POST("/email/verify", server.verifyEmailOTP)
	}
	
	v1.POST("/check-email", server.checkEmailExists)
	
	router.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))
	
	server.router = router
	return router
}

// Start runs the HTTP server on a specific address.
func (server *Server) Start(address string) error {
	return server.router.Run(address)
}

func (server *Server) SetupZalopayRouter() *gin.Engine {
	zalopayRouter := gin.New()
	zalopayRouter.Use(gin.Recovery())
	
	zalopayRouter.POST("/v1/zalopay/callback", server.handleZalopayCallback)
	
	return zalopayRouter
}
