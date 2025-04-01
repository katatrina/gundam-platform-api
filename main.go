package main

import (
	"context"
	"os"
	"time"
	
	firebase "firebase.google.com/go/v4"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/katatrina/gundam-BE/api"
	db "github.com/katatrina/gundam-BE/internal/db/sqlc"
	"github.com/katatrina/gundam-BE/internal/mailer"
	"github.com/katatrina/gundam-BE/internal/util"
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
	
	mailService, err := mailer.NewGmailSender(appConfig.GmailSMTPUsername, appConfig.GmailSMTPPassword, appConfig, redisDb)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to create mail service 😣")
	}
	
	runHTTPServer(&appConfig, store, redisDb, mailService, firebaseApp)
}

func runHTTPServer(appConfig *util.Config, store db.Store, redisDb *redis.Client, mailer *mailer.GmailSender, firebaseApp *firebase.App) {
	// Khởi động ngrok tunnel cho Zalopay callback nếu ở môi trường development
	if appConfig.Environment == util.EnvironmentDevelopment {
		// Kiểm tra xem NGROK_AUTHTOKEN có được cung cấp hay không
		if appConfig.NgrokAuthToken == "" {
			log.Warn().Msg("NGROK_AUTHTOKEN not set in config, skipping ngrok tunnel setup")
			log.Warn().Msg("Zalopay callback service may not work properly 😣")
		} else {
			log.Info().Msg("starting ngrok tunnel for Zalopay callback...")
			
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			
			listener, err := ngrok.Listen(ctx,
				config.HTTPEndpoint(),
				ngrok.WithAuthtoken(appConfig.NgrokAuthToken),
			)
			
			if err != nil {
				log.Warn().Err(err).Msg("failed to create ngrok tunnel, Zalopay callback service may not work properly 😣")
			} else {
				log.Info().Str("url", listener.URL()).Msg("ngrok tunnel established for Zalopay callback ✅")
				appConfig.ZalopayCallbackURL = listener.URL() + "/v1/zalopay/callback"
			}
		}
	}
	
	server, err := api.NewServer(store, redisDb, appConfig, mailer, firebaseApp)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to create HTTP server 😣")
	}
	
	// Chạy server chính bình thường
	err = server.Start(appConfig.HTTPServerAddress)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to start HTTP server 😣")
	}
}
