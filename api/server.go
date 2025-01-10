package api

import (
	"fmt"
	
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	db "github.com/katatrina/gundam-BE/internal/db/sqlc"
	"github.com/katatrina/gundam-BE/internal/token"
	"github.com/katatrina/gundam-BE/internal/util"
)

type Server struct {
	router     *gin.Engine
	store      db.Store
	tokenMaker token.Maker
	config     util.Config
}

// NewServer creates a new HTTP server and set up routing.
func NewServer(store db.Store, config util.Config) (*Server, error) {
	tokenMaker, err := token.NewJWTMaker(config.TokenSecretKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create token maker: %w", err)
	}
	
	server := &Server{
		store:      store,
		tokenMaker: tokenMaker,
		config:     config,
	}
	
	server.setupRouter()
	return server, nil
}

// setupRouter configures the HTTP server routes.
func (server *Server) setupRouter() {
	router := gin.Default()
	router.Use(cors.New(cors.Config{
		AllowOrigins:     server.config.AllowedOrigins,
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE"},
		AllowHeaders:     []string{"Origin", "Content-Length", "Content-Type", "Authorization"},
		AllowCredentials: true,
	}))
	v1 := router.Group("/v1")
	
	v1.POST("/tokens/verify", server.verifyAccessToken)
	
	userGroup := v1.Group("/users")
	{
		userGroup.POST("", server.createUser)
		userGroup.POST("/login", server.loginUser)
	}
	
	server.router = router
}

// Start runs the HTTP server on a specific address.
func (server *Server) Start(address string) error {
	return server.router.Run(address)
}
