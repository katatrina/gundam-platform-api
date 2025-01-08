package main

import (
	"context"
	"os"
	
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/katatrina/gundam-BE/api"
	db "github.com/katatrina/gundam-BE/internal/db/sqlc"
	"github.com/katatrina/gundam-BE/internal/util"
	"github.com/rs/zerolog"
	
	"github.com/rs/zerolog/log"
)

func main() {
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})
	
	// Load configurations
	config, err := util.LoadConfig("./app.env")
	if err != nil {
		log.Fatal().Err(err).Msg("failed to load config file 😣")
	}
	
	log.Info().Msg("configurations loaded successfully 😎")
	
	// Create connection pool
	connPool, err := pgxpool.New(context.Background(), config.DatabaseURL)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to validate db connection string 😣")
	}
	
	pingErr := connPool.Ping(context.Background())
	if pingErr != nil {
		log.Fatal().Err(pingErr).Msg("failed to connect to db 😣")
	}
	log.Info().Msg("connected to db 😎")
	
	store := db.NewStore(connPool)
	
	runHTTPServer(config, store)
}

func runHTTPServer(config util.Config, store db.Store) {
	server, err := api.NewServer(store, config)
	
	if err != nil {
		log.Fatal().Err(err).Msg("failed to create HTTP server 😣")
	}
	
	err = server.Start(config.HTTPServerAddress)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to start HTTP server 😣")
	}
}
