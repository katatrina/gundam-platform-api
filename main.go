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
	
	// Load configurations
	appConfig, err := util.LoadConfig("./app.env")
	if err != nil {
		log.Fatal().Err(err).Msg("failed to load application config file 😣")
	}
	
	// Load Google service account file and initialize Firebase app
	ctx := context.Background()
	opt := option.WithCredentialsFile("./service-account-file.json")
	firebaseApp, err := firebase.NewApp(ctx, nil, opt)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to create firebase app 😣")
	}
	
	log.Info().Msg("configurations loaded successfully ✅")
	
	// Create connection pool
	connPool, err := pgxpool.New(context.Background(), appConfig.DatabaseURL)
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
		Addr:     appConfig.RedisServerAddress,
		Password: "", // no password set
		DB:       0,  // use default DB
	})
	defer redisDb.Close()
	
	mailService, err := mailer.NewGmailSender(appConfig.GmailSMTPUsername, appConfig.GmailSMTPPassword, appConfig, redisDb)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to create mail service 😣")
	}
	
	redisOpt := asynq.RedisClientOpt{
		Addr: appConfig.RedisServerAddress,
	}
	if appConfig.Environment == "production" {
		redisOpt.Password = appConfig.RedisServerPassword
		redisOpt.TLSConfig = &tls.Config{
			MinVersion: tls.VersionTLS12,
		}
	}
	
	taskDistributor := worker.NewRedisTaskDistributor(redisOpt)
	
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
	
	go runRedisTaskProcessor(redisOpt, store, firebaseApp)
	runHTTPServer(&appConfig, store, redisDb, taskDistributor, mailService, ghnService)
}

func runHTTPServer(appConfig *util.Config, store db.Store, redisDb *redis.Client, taskDistributor *worker.RedisTaskDistributor, mailer *mailer.GmailSender, deliveryService delivery.IDeliveryProvider) {
	server, err := api.NewServer(store, redisDb, taskDistributor, appConfig, mailer, deliveryService)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to create HTTP server 😣")
	}
	
	// Chạy ngrok tunnel trong một goroutine riêng
	go setupNgrokTunnel(appConfig, server)
	
	// Chạy server chính trên localhost
	log.Info().Msg("Starting main server on localhost")
	if err := server.Start(appConfig.HTTPServerAddress); err != nil {
		log.Fatal().Err(err).Msg("failed to start main HTTP server 😣")
	}
}

func setupNgrokTunnel(appConfig *util.Config, server *api.Server) {
	if appConfig.Environment != util.EnvironmentDevelopment || appConfig.NgrokAuthToken == "" {
		log.Warn().Msg("Skipping ngrok tunnel setup")
		return
	}
	
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
		if err := zalopayRouter.RunListener(tunnel); err != nil {
			log.Error().Err(err).Msg("Zalopay callback server stopped")
			return
		}
	}
}

// runRedisTaskProcessor creates a new task processor and starts it.
func runRedisTaskProcessor(redisOpt asynq.RedisClientOpt, store db.Store, firebaseApp *firebase.App) {
	taskProcessor := worker.NewRedisTaskProcessor(redisOpt, store, firebaseApp)
	
	err := taskProcessor.Start()
	if err != nil {
		log.Fatal().Err(err).Msg("failed to start task processor 😣")
	}
	
	log.Info().Msg("task processor started ✅")
}
