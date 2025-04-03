package api

import (
	"context"
	"fmt"
	
	firebase "firebase.google.com/go/v4"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	db "github.com/katatrina/gundam-BE/internal/db/sqlc"
	"github.com/katatrina/gundam-BE/internal/mailer"
	"github.com/katatrina/gundam-BE/internal/notification"
	"github.com/katatrina/gundam-BE/internal/phone_number"
	"github.com/katatrina/gundam-BE/internal/storage"
	"github.com/katatrina/gundam-BE/internal/token"
	"github.com/katatrina/gundam-BE/internal/util"
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
	redisDb                *redis.Client
	phoneNumberService     *phone_number.PhoneNumberService
	mailService            *mailer.GmailSender
	notificationService    *notification.NotificationService
	zalopayService         *zalopay.ZalopayService
}

// NewServer creates a new HTTP server and set up routing.
func NewServer(store db.Store, redisDb *redis.Client, config *util.Config, mailer *mailer.GmailSender, firebaseApp *firebase.App) (*Server, error) {
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
	
	// Create a new notification service
	notificationService, err := notification.NewNotificationService(context.Background(), firebaseApp)
	if err != nil {
		return nil, fmt.Errorf("failed to create notification service: %w", err)
	}
	log.Info().Msg("Notification service created successfully ✅")
	
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
		redisDb:                redisDb,
		mailService:            mailer,
		notificationService:    notificationService,
		zalopayService:         zalopayService,
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
	}
	
	orderGroup := v1.Group("/orders", authMiddleware(server.tokenMaker))
	{
		orderGroup.POST("", server.createOrder)
		orderGroup.GET("", server.listOrders)
	}
	
	walletGroup := v1.Group("/wallet", authMiddleware(server.tokenMaker))
	{
		zalopayGroup := walletGroup.Group("/zalopay")
		{
			zalopayGroup.POST("/create", server.createZalopayOrder)
		}
	}
	
	v1.GET("/grades", server.listGundamGrades)
	
	v1.GET("/sellers/:sellerID", server.getSeller)
	sellerGroup := v1.Group("/sellers/:sellerID", authMiddleware(server.tokenMaker), requiredSellerOrAdminRole(server.dbStore))
	{
		gundamGroup := sellerGroup.Group("gundams")
		{
			gundamGroup.POST("", server.createGundam)
			gundamGroup.GET("", server.listGundamsBySeller)
			gundamGroup.PATCH(":gundamID/publish", server.publishGundam)
			gundamGroup.PATCH(":gundamID/unpublish", server.unpublishGundam)
		}
		
		subscriptionGroup := sellerGroup.Group("subscriptions")
		{
			subscriptionGroup.GET("active", server.getCurrentActiveSubscription)
		}
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
	
	v1.POST("checkout", authMiddleware(server.tokenMaker), server.createOrder)
	
	otpGroup := v1.Group("/otp")
	{
		otpGroup.POST("/phone_number/generate", server.generatePhoneNumberOTP)
		otpGroup.POST("/phone_number/verify", server.verifyPhoneNumberOTP)
		
		otpGroup.POST("/email/generate", server.generateEmailOTP)
		otpGroup.POST("/email/verify", server.verifyEmailOTP)
	}
	
	v1.GET("/check-email", server.checkEmailExists)
	
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
