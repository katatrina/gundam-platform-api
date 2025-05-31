package api

import (
	"context"
	"fmt"
	
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	db "github.com/katatrina/gundam-BE/internal/db/sqlc"
	"github.com/katatrina/gundam-BE/internal/delivery"
	"github.com/katatrina/gundam-BE/internal/event"
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
	"resty.dev/v3"
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
	taskDistributor        worker.TaskDistributor
	taskInspector          worker.TaskInspector
	zalopayService         *zalopay.ZalopayService
	deliveryService        delivery.IDeliveryProvider
	restyClient            *resty.Client
	eventSender            event.EventSender
}

// NewServer creates a new HTTP server and set up routing.
func NewServer(store db.Store, redisClient *redis.Client, taskDistributor worker.TaskDistributor, taskInspector worker.TaskInspector, config *util.Config, mailer *mailer.GmailSender, deliveryService delivery.IDeliveryProvider, eventSender event.EventSender) (*Server, error) {
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
	phoneNumberService, err := phone_number.NewPhoneService(config, redisClient)
	if err != nil {
		return nil, fmt.Errorf("failed to create phone service: %w", err)
	}
	log.Info().Msg("Phone service created successfully ✅")
	
	// Create a new ZaloPay service
	zalopayService := zalopay.NewZalopayService(store, config)
	log.Info().Msg("ZaloPay service created successfully ✅")
	
	// Khởi tạo resty client
	restyClient := resty.New()
	log.Info().Msg("Resty client created successfully ✅")
	
	server := &Server{
		dbStore:                store,
		tokenMaker:             tokenMaker,
		config:                 config,
		googleIDTokenValidator: googleIDTokenValidator,
		fileStore:              fileStore,
		phoneNumberService:     phoneNumberService,
		mailService:            mailer,
		taskDistributor:        taskDistributor,
		taskInspector:          taskInspector,
		zalopayService:         zalopayService,
		deliveryService:        deliveryService,
		restyClient:            restyClient,
		eventSender:            eventSender,
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
	
	// API cho member thông thường
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
			userGundamGroup.GET(":gundamID", server.getUserGundamDetails)
			userGundamGroup.PATCH(":gundamID", server.updateGundamBasisInfo)
			userGundamGroup.PUT(":gundamID/accessories", server.updateGundamAccessories)
			userGundamGroup.PATCH(":gundamID/primary-image", server.updateGundamPrimaryImage)
			userGundamGroup.POST(":gundamID/images", server.addGundamSecondaryImages)
			userGundamGroup.DELETE(":gundamID/images", server.deleteGundamSecondaryImage)
			userGundamGroup.DELETE(":gundamID", server.hardDeleteGundam)
			userGundamGroup.GET("", server.listGundamsByUser)
		}
		
		// API cho bài đăng trao đổi của người dùng (đã đăng nhập)
		userExchangePostGroup := userGroup.Group("/me/exchange-posts")
		{
			// Liệt kê thông tin chi tiết của các bài đăng trao đổi
			userExchangePostGroup.GET("", server.listUserExchangePosts) // ✅
			
			// Tạo bài đăng trao đổi mới
			userExchangePostGroup.POST("", server.createExchangePost) // ✅
			
			// Lấy thông tin chi tiết của một bài đăng trao đổi
			userExchangePostGroup.GET("/:postID", server.getUserExchangePost) // ✅
			
			// Chỉnh sửa bài đăng trao đổi (hiện tại chưa cho phép chỉnh sửa sau khi đã tạo bài đăng)
			// Bỏ qua vì nhiều lí do
			// userExchangePostGroup.PUT("/:id", server.updateExchangePost)
			
			// Xóa bài đăng trao đổi
			userExchangePostGroup.DELETE(":postID", server.deleteExchangePost) // ✅
			
			// API cho đề xuất trao đổi của một bài đăng
			offerGroup := userExchangePostGroup.Group("/:postID/offers")
			{
				// Thêm endpoint cho yêu cầu thương lượng
				offerGroup.PATCH("/:offerID/negotiate", server.requestNegotiationForOffer) // ✅
				
				// Chấp nhận đề xuất trao đổi
				offerGroup.PATCH("/:offerID/accept", server.acceptExchangeOffer) // ✅
				
				// Từ chối đề xuất trao đổi
				// offerGroup.PATCH("/:offerID/reject", server.rejectExchangeOffer)
			}
		}
		
		// API cho đề xuất trao đổi của người dùng đã đăng nhập
		userOffersGroup := userGroup.Group("/me/exchange-offers")
		{
			// Liệt kê tất cả đề xuất trao đổi mà người dùng đã gửi
			userOffersGroup.GET("", server.listUserExchangeOffers) // ✅
			
			// Lấy thông tin chi tiết của một đề xuất
			userOffersGroup.GET(":offerID", server.getUserExchangeOffer) // ✅
			
			// Tạo đề xuất trao đổi cho một bài đăng
			userOffersGroup.POST("", server.createExchangeOffer) // ✅
			
			// Thêm endpoint cập nhật đề xuất (phản hồi thương lượng)
			userOffersGroup.PATCH("/:offerID", server.updateExchangeOffer) // ✅
			
			// Xóa đề xuất trao đổi
			userOffersGroup.DELETE("/:offerID", server.deleteExchangeOffer) // ✅
		}
	}
	
	// Nhóm các API liên quan đến cuộc trao đổi
	exchangeGroup := v1.Group("/exchanges", authMiddleware(server.tokenMaker))
	{
		exchangeGroup.GET("", server.listUserExchanges)                                              // ✅ Liệt kê các giao dịch trao đổi của người dùng
		exchangeGroup.GET(":exchangeID", server.getExchangeDetails)                                  // ✅ Lấy chi tiết giao dịch trao đổi
		exchangeGroup.PUT(":exchangeID/delivery-addresses", server.provideExchangeDeliveryAddresses) // ✅ Cung cấp địa chỉ gửi và nhận hàng
		exchangeGroup.POST(":exchangeID/pay-delivery-fee", server.payExchangeDeliveryFee)            // ✅ Thanh toán phí vận chuyển
		exchangeGroup.PATCH(":exchangeID/cancel", server.cancelExchange)                             // ✅ Hủy giao dịch trao đổi
	}
	
	// API public cho bài đăng trao đổi - không cần đăng nhập
	exchangePostPublicGroup := v1.Group("/exchange-posts")
	{
		// Liệt kê các bài post trao đổi đang mở trên nền tảng
		exchangePostPublicGroup.GET("", optionalAuthMiddleware(server.tokenMaker), server.listOpenExchangePosts) // ✅
		
		// Lấy chi tiết một bài post trao đổi (bỏ - không cần thiết)
		// exchangePostPublicGroup.GET("/:id", server.getExchangePostDetails)
	}
	
	// Nhóm api cho các đơn hàng thông thường và đơn hàng trao đổi
	orderGroup := v1.Group("/orders", authMiddleware(server.tokenMaker))
	{
		// Tạo đơn hàng mua thông thường khi user thanh toán thành công
		// Client cần gọi api này nhiều lần để tạo nhiều đơn hàng nếu có nhiều sản phẩm thuộc nhiều seller khác nhau trong giỏ hàng
		orderGroup.POST("", server.createOrder)                        // ✅ Tạo đơn hàng thông thường
		orderGroup.GET("", server.listMemberOrders)                    // ✅ Liệt kê tất cả đơn hàng thông thường và đơn hàng trao đổi trong tab "Đơn hàng" trong trang "Tài khoản của tôi"
		orderGroup.GET(":orderID", server.getMemberOrderDetails)       // ✅ Lấy thông tin chi tiết của một đơn hàng thông thường hoặc đơn hàng trao đổi
		orderGroup.PATCH(":orderID/package", server.packageOrder)      // ✅ Người gửi đóng gói đơn hàng
		orderGroup.PATCH(":orderID/complete", server.completeOrder)    // ✅ Người nhận hàng xác nhận đã nhận hàng thành công
		orderGroup.PATCH(":orderID/cancel", server.cancelOrderByBuyer) // ✅ Người mua hủy đơn hàng
	}
	
	// API công khai cho phiên đấu giá (không cần đăng nhập)
	auctionPublicGroup := v1.Group("/auctions")
	{
		// Liệt kê các phiên đấu giá (sắp diễn ra, đang diễn ra)
		auctionPublicGroup.GET("", server.listAuctions) // ✅
		
		// Lấy thông tin chi tiết của một phiên đấu giá
		auctionPublicGroup.GET(":auctionID", server.getAuctionDetails) // ✅
		
		auctionPublicGroup.GET(":auctionID/stream", server.streamAuctionEvents) // ✅ Endpoint SSE
	}
	
	// API cho người dùng tham gia đấu giá (cần đăng nhập)
	userAuctionGroup := v1.Group("/users/me/auctions", authMiddleware(server.tokenMaker))
	{
		// Tham gia đấu giá (đặt cọc)
		userAuctionGroup.POST("/:auctionID/participate", server.participateInAuction) // ✅
		
		// Đặt giá
		userAuctionGroup.POST("/:auctionID/bids", server.placeBid) // ✅
		
		// Xem danh sách các lượt đặt giá của bản thân của một phiên đấu giá cụ thể
		userAuctionGroup.GET("/:auctionID/bids", server.listUserBids) // ✅
		
		// Thanh toán sau khi thắng
		userAuctionGroup.POST("/:auctionID/payment", server.payAuctionWinningBid) // ✅
		
		// Xem danh sách các phiên đấu giá đã tham gia của bản thân
		userAuctionGroup.GET("", server.listUserParticipatedAuctions) // ✅
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
			sellerOrderGroup.PATCH(":orderID/cancel", server.cancelOrderBySeller) // ✅ Người bán hủy đơn hàng
		}
		
		// Nhóm các API liên quan đến việc quản lý sản phẩm của người bán
		gundamGroup := sellerGroup.Group("gundams")
		{
			gundamGroup.PATCH(":gundamID/publish", server.publishGundam)     // ✅
			gundamGroup.PATCH(":gundamID/unpublish", server.unpublishGundam) // ✅
		}
		
		// Nhóm các API liên quan đến việc quản lý gói đăng ký của người bán
		subscriptionGroup := sellerGroup.Group("subscriptions")
		{
			// Lấy thông tin gói đăng ký hiện tại của người bán
			subscriptionGroup.GET("active", server.getCurrentActiveSubscription) // ✅
			
			// Đăng ký/nâng cấp gói subscription
			subscriptionGroup.POST("upgrade", server.upgradeSubscription) // ✅
		}
		
		// Nhóm các API cho yêu cầu đấu giá
		auctionRequestGroup := sellerGroup.Group("auction-requests")
		{
			// Tạo yêu cầu đấu giá
			auctionRequestGroup.POST("", server.createAuctionRequest) // ✅
			
			// Xem danh sách yêu cầu đấu giá của mình
			auctionRequestGroup.GET("", server.listSellerAuctionRequests) // ✅
			
			// Xem chi tiết yêu cầu đấu giá
			// auctionRequestGroup.GET(":requestID", server.getAuctionRequestDetails)
			
			// Xóa yêu cầu đấu giá (pending hoặc rejected)
			auctionRequestGroup.DELETE(":requestID", server.deleteAuctionRequest) // ✅
		}
		
		// API cho phiên đấu giá của seller
		sellerAuctionGroup := sellerGroup.Group("auctions")
		{
			// Xem danh sách phiên đấu giá của mình
			sellerAuctionGroup.GET("", server.listSellerAuctions) // ✅
			
			// Xem chi tiết phiên đấu giá của mình
			sellerAuctionGroup.GET(":auctionID", server.getSellerAuctionDetails) // ✅
			
			// Hủy phiên đấu giá (tạm thời không cho hủy)
			// sellerAuctionGroup.PATCH(":auctionID/cancel", server.cancelAuction)
		}
	}
	
	walletGroup := v1.Group("/wallet", authMiddleware(server.tokenMaker))
	{
		zalopayGroup := walletGroup.Group("/zalopay")
		{
			zalopayGroup.POST("/create", server.createZalopayOrder)
		}
	}
	
	userWalletGroup := v1.Group("/users/me/wallet", authMiddleware(server.tokenMaker))
	{
		// Liệt kê tất cả các bút toán ví của người dùng
		userWalletGroup.GET("/entries", server.listUserWalletEntries)
		
		userWalletGroup.POST("/withdrawal-requests", server.createWithdrawalRequest)             // ✅
		userWalletGroup.GET("/withdrawal-requests", server.listUserWithdrawalRequests)           // ✅
		userWalletGroup.PATCH("/withdrawal-requests/:requestID", server.cancelWithdrawalRequest) // ✅
	}
	
	userBankAccountGroup := v1.Group("/users/me/bank-accounts", authMiddleware(server.tokenMaker))
	{
		userBankAccountGroup.POST("", server.addBankAccount)      // ✅
		userBankAccountGroup.GET("", server.listUserBankAccounts) // ✅
	}
	
	v1.GET("/grades", server.listGundamGrades)                  // Liệt kê tất cả các cấp độ Gundam
	v1.GET("/subscription-plans", server.listSubscriptionPlans) // Liệt kê tất cả gói subscription
	
	sellerProfileGroup := v1.Group("/seller/profile")
	{
		sellerProfileGroup.POST("", server.createSellerProfile)
		sellerProfileGroup.GET("", server.getSellerProfile)
		sellerProfileGroup.PATCH("", server.updateSellerProfile)
	}
	
	gundamGroup := v1.Group("/gundams")
	{
		gundamGroup.GET("", server.listGundams)
		gundamGroup.GET(":gundamID", server.getGundamDetails)
		gundamGroup.GET("/by-slug/:slug", server.getGundamBySlug)
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
	
	// API cho moderator
	moderatorGroup := v1.Group("/mod", authMiddleware(server.tokenMaker), requiredModeratorRole(server.dbStore))
	{
		moderatorAuctionRequestGroup := moderatorGroup.Group("auction-requests")
		{
			// Xem tất cả yêu cầu đấu giá (pending, approved, rejected)
			moderatorAuctionRequestGroup.GET("", server.listAuctionRequestsForModerator) // ✅
			
			// Phê duyệt yêu cầu đấu giá
			moderatorAuctionRequestGroup.PATCH(":requestID/approve", server.approveAuctionRequest) // ✅
			
			// Từ chối yêu cầu đấu giá
			moderatorAuctionRequestGroup.PATCH(":requestID/reject", server.rejectAuctionRequest) // ✅
		}
		
		moderatorAuctionGroup := moderatorGroup.Group("auctions")
		{
			// Chỉnh sửa thông tin của một phiên đấu giá
			moderatorAuctionGroup.PATCH(":auctionID", server.updateAuctionDetailsByModerator)
		}
		
		moderatorWithdrawalRequestGroup := moderatorGroup.Group("withdrawal-requests")
		{
			moderatorWithdrawalRequestGroup.GET("", server.listWithdrawalRequests)                         // ✅
			moderatorWithdrawalRequestGroup.PATCH(":requestID/complete", server.completeWithdrawalRequest) // ✅
			// moderatorWithdrawalRequestGroup.PATCH(":requestID/reject", server.rejectWithdrawalRequest)
		}
	}
	
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
