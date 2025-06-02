package main

import (
	"context"
	"crypto/tls"
	"os"
	"time"
	
	firebase "firebase.google.com/go/v4"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/katatrina/gundam-BE/api"
	db "github.com/katatrina/gundam-BE/internal/db/sqlc"
	"github.com/katatrina/gundam-BE/internal/delivery"
	"github.com/katatrina/gundam-BE/internal/event"
	"github.com/katatrina/gundam-BE/internal/mailer"
	ordertracking "github.com/katatrina/gundam-BE/internal/order_tracking"
	"github.com/katatrina/gundam-BE/internal/util"
	"github.com/katatrina/gundam-BE/internal/worker"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"
	"golang.ngrok.com/ngrok/config"
	"google.golang.org/api/option"
	
	"github.com/rs/zerolog/log"
	
	_ "github.com/katatrina/gundam-BE/docs"
	"golang.ngrok.com/ngrok"
)

//	@title			Gundam Platform API
//	@version		1.0.0
//	@description	API documentation for Gundam Platform application

//	@host		localhost:8080
//	@BasePath	/v1
//	@schemes	http https

// @securityDefinitions.apikey	accessToken
// @in							header
// @name						Authorization
// @description				Type "Bearer" followed by a space and JWT token.
func main() {
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})
	
	// Determine config path based on environment
	configPath := getConfigPath()
	
	// Load configurations
	appConfig, err := util.LoadConfig(configPath)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to load application config file 😣")
	}
	
	// Load Google service account credentials
	ctx := context.Background()
	firebaseApp, err := initializeFirebase(ctx, appConfig.Environment)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to create firebase app 😣")
	}
	log.Info().Msg("firebase client app initialized ✅")
	
	// Create connection pool với optimized settings
	config, err := pgxpool.ParseConfig(appConfig.DatabaseURL)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to parse database config string 😣")
	}
	
	// Optimize connection pool for production
	if appConfig.Environment == util.EnvironmentProduction {
		config.MaxConns = 10               // Limit concurrent connections
		config.MinConns = 2                // Keep warm connections
		config.MaxConnLifetime = time.Hour // Rotate connections
		config.MaxConnIdleTime = time.Minute * 30
		config.HealthCheckPeriod = time.Minute
	}
	
	connPool, err := pgxpool.NewWithConfig(context.Background(), config)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to validate db connection string 😣")
	}
	defer connPool.Close()
	
	pingErr := connPool.Ping(context.Background())
	if pingErr != nil {
		log.Fatal().Err(pingErr).Msg("failed to connect to db 😣")
	}
	log.Info().Msg("connected to db ✅")
	
	store := db.NewStore(connPool)
	
	redisDb := redis.NewClient(&redis.Options{
		Addr:      appConfig.RedisServerAddress,
		Password:  appConfig.RedisServerPassword,
		DB:        0,
		TLSConfig: getTLSConfig(appConfig.Environment),
	})
	defer redisDb.Close()
	
	// Test Redis connection
	_, err = redisDb.Ping(context.Background()).Result()
	if err != nil {
		log.Fatal().Err(err).Msg("failed to connect to Redis 😣")
	}
	log.Info().Msg("connected to Redis ✅")
	
	mailService, err := mailer.NewGmailSender(appConfig.GmailSMTPUsername, appConfig.GmailSMTPPassword, appConfig, redisDb)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to create mail service 😣")
	}
	
	redisOpt := asynq.RedisClientOpt{
		Addr:      appConfig.RedisServerAddress,
		Password:  appConfig.RedisServerPassword,
		TLSConfig: getTLSConfig(appConfig.Environment),
	}
	
	taskDistributor := worker.NewTaskDistributor(redisOpt)
	taskInspector := worker.NewTaskInspector(redisOpt)
	
	// Tạo GHN service
	ghnService := delivery.NewGHNService(appConfig.GHNToken, appConfig.GHNShopID)
	
	// Khởi tạo và chạy OrderTracker
	orderTracker, err := ordertracking.NewOrderTracker(store, ghnService, taskDistributor)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to create order tracker 😣")
	}
	
	err = orderTracker.Start()
	if err != nil {
		log.Fatal().Err(err).Msg("failed to start order tracker 😣")
	}
	log.Info().Msg("order tracking service started ✅")
	
	// Khởi tạo SSE server
	sseServer := event.NewSSEServer()
	go sseServer.Run() // Chạy trong goroutine riêng
	
	go runRedisTaskProcessor(redisOpt, store, firebaseApp, taskDistributor, sseServer)
	runHTTPServer(&appConfig, store, redisDb, taskDistributor, taskInspector, mailService, ghnService, sseServer)
}

// getConfigPath determines the config file path based on environment
func getConfigPath() string {
	if os.Getenv("ENVIRONMENT") == "production" {
		return "" // No config file in production
	}
	return "./app.env" // Development: use app.env file
}

// getTLSConfig returns TLS config for Redis based on environment
func getTLSConfig(environment string) *tls.Config {
	if environment == util.EnvironmentProduction {
		return &tls.Config{
			MinVersion: tls.VersionTLS12,
		}
	}
	return nil
}

// initializeFirebase initializes Firebase app by loading credentials from file
func initializeFirebase(ctx context.Context, environment string) (*firebase.App, error) {
	opt := option.WithCredentialsFile("./service-account-file.json")
	return firebase.NewApp(ctx, nil, opt)
}

func runHTTPServer(appConfig *util.Config, store db.Store, redisDb *redis.Client, taskDistributor worker.TaskDistributor, taskInspector worker.TaskInspector, mailer *mailer.GmailSender, deliveryService delivery.IDeliveryProvider, eventSender event.EventSender) {
	server, err := api.NewServer(store, redisDb, taskDistributor, taskInspector, appConfig, mailer, deliveryService, eventSender)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to create HTTP server 😣")
	}
	
	go setupNgrokTunnel(appConfig, server)
	
	// Chạy server chính
	log.Info().Msg("Starting main server")
	if err = server.Start(appConfig.HTTPServerAddress); err != nil {
		log.Fatal().Err(err).Msg("failed to start main HTTP server 😣")
	}
}

// Always run ngrok tunnel setup in a separate goroutine in any environment
func setupNgrokTunnel(appConfig *util.Config, server *api.Server) {
	maxRetries := 3
	for attempt := 0; attempt < maxRetries; attempt++ {
		ctx := context.Background()
		tunnel, err := ngrok.Listen(ctx,
			config.HTTPEndpoint(),
			ngrok.WithAuthtoken(appConfig.NgrokAuthToken),
		)
		if err != nil {
			log.Error().Err(err).Int("attempt", attempt+1).Int("maxRetries", maxRetries).Msg("failed to create ngrok tunnel, retrying in 5 seconds...")
			if attempt < maxRetries-1 {
				time.Sleep(5 * time.Second)
				continue
			}
			log.Error().Msg("Max retries reached, giving up on ngrok tunnel setup")
			return
		}
		
		appConfig.ZalopayCallbackURL = tunnel.URL() + "/v1/zalopay/callback"
		log.Info().Msg("ngrok tunnel established ✅")
		log.Info().Str("url", tunnel.URL()).Msg("ngrok tunnel URL")
		log.Info().Str("zalopay_callback_url", appConfig.ZalopayCallbackURL).Msg("Zalopay callback URL")
		
		zalopayRouter := server.SetupZalopayRouter()
		if err = zalopayRouter.RunListener(tunnel); err != nil {
			log.Error().Err(err).Msg("Zalopay callback server stopped")
			return
		}
	}
}

// runRedisTaskProcessor creates a new task processor and starts it.
func runRedisTaskProcessor(redisOpt asynq.RedisClientOpt, store db.Store, firebaseApp *firebase.App, taskDistributor worker.TaskDistributor, eventSender event.EventSender) {
	taskProcessor := worker.NewRedisTaskProcessor(redisOpt, store, firebaseApp, taskDistributor, eventSender)
	
	err := taskProcessor.Start()
	if err != nil {
		log.Fatal().Err(err).Msg("failed to start task processor 😣")
	}
	
	log.Info().Msg("task processor started ✅")
}
