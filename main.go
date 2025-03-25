package main

import (
	"context"
	"os"
	
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/katatrina/gundam-BE/api"
	db "github.com/katatrina/gundam-BE/internal/db/sqlc"
	"github.com/katatrina/gundam-BE/internal/mailer"
	"github.com/katatrina/gundam-BE/internal/util"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"
	
	"github.com/rs/zerolog/log"
	
	_ "github.com/katatrina/gundam-BE/docs"
)

//	@title			Gundam Platform API
//	@version		1.0.0
//	@description	API documentation for Gundam Platform application

//	@host		localhost:8080
//	@BasePath	/v1
//	@schemes	http https

//	@securityDefinitions.apikey	accessToken
//	@in							header
//	@name						Authorization
//	@description				Type "Bearer" followed by a space and JWT token.
func main() {
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})
	
	// Load configurations
	config, err := util.LoadConfig("./app.env")
	if err != nil {
		log.Fatal().Err(err).Msg("failed to load config file 😣")
	}
	
	log.Info().Msg("configurations loaded successfully ✅")
	
	// Create connection pool
	connPool, err := pgxpool.New(context.Background(), config.DatabaseURL)
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
		Addr:     config.RedisServerAddress,
		Password: "", // no password set
		DB:       0,  // use default DB
	})
	
	// fmt.Println(config.GmailSMTPUsername, config.GmailSMTPPassword)
	mailService, err := mailer.NewGmailSender(config.GmailSMTPUsername, config.GmailSMTPPassword, config, redisDb)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to create mailer service 😣")
	}
	
	runHTTPServer(config, store, redisDb, mailService)
}

func runHTTPServer(config util.Config, store db.Store, redisDb *redis.Client, mailer *mailer.GmailSender) {
	server, err := api.NewServer(store, redisDb, config, mailer)
	
	if err != nil {
		log.Fatal().Err(err).Msg("failed to create HTTP server 😣")
	}
	
	err = server.Start(config.HTTPServerAddress)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to start HTTP server 😣")
	}
}
