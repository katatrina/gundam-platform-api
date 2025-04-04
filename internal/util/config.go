package util

import (
	"errors"
	"fmt"
	"time"
	
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
)

const (
	EnvironmentDevelopment = "development"
	EnvironmentProduction  = "production"
)

// Config stores all configuration of the application.
// The values are read by viper from a config file or environment variables.
type Config struct {
	AllowedOrigins      []string      `mapstructure:"ALLOWED_ORIGINS"`
	DatabaseURL         string        `mapstructure:"DATABASE_URL"`
	HTTPServerAddress   string        `mapstructure:"HTTP_SERVER_ADDRESS"`
	TokenSecretKey      string        `mapstructure:"TOKEN_SECRET_KEY"`
	AccessTokenDuration time.Duration `mapstructure:"ACCESS_TOKEN_DURATION"`
	GoogleClientID      string        `mapstructure:"GOOGLE_CLIENT_ID"`
	CloudinaryURL       string        `mapstructure:"CLOUDINARY_URL"`
	RedisServerAddress  string        `mapstructure:"REDIS_SERVER_ADDRESS"`
	RedisServerPassword string        `mapstructure:"REDIS_SERVER_PASSWORD"`
	DiscordBotToken     string        `mapstructure:"DISCORD_BOT_TOKEN"`
	DiscordChannelID    string        `mapstructure:"DISCORD_CHANNEL_ID"`
	GmailSMTPUsername   string        `mapstructure:"GMAIL_SMTP_USERNAME"`
	GmailSMTPPassword   string        `mapstructure:"GMAIL_SMTP_PASSWORD"`
	ZalopayCallbackURL  string        `mapstructure:"ZALOPAY_CALLBACK_URL"`
	Environment         string        `mapstructure:"ENVIRONMENT"`
	NgrokAuthToken      string        `mapstructure:"NGROK_AUTH_TOKEN"`
	GHNShopID           string        `mapstructure:"GHN_SHOP_ID"`
	GHNToken            string        `mapstructure:"GHN_TOKEN"`
}

// LoadConfig reads configuration from file or environment variables.
func LoadConfig(path string) (config Config, err error) {
	// Set defaults for non-sensitive config
	viper.SetDefault("ALLOWED_ORIGINS", []string{"http://localhost:3000"})
	viper.SetDefault("HTTP_SERVER_ADDRESS", "0.0.0.0:8080")
	viper.SetDefault("ACCESS_TOKEN_DURATION", "24h")
	viper.SetDefault("ENVIRONMENT", EnvironmentDevelopment)
	
	// Prefer environment variables over config file
	viper.AutomaticEnv()
	
	// Load config file if exists
	viper.SetConfigFile(path)
	if err = viper.ReadInConfig(); err != nil {
		var configFileNotFoundError viper.ConfigFileNotFoundError
		if !errors.As(err, &configFileNotFoundError) {
			// Config file was found but another error was produced
			return config, fmt.Errorf("failed to read config file: %w", err)
		}
		
		// If config file is not found, we will use environment variables
		log.Warn().Msg("Config file not found, using environment variables")
	}
	
	// Unmarshal config into struct
	err = viper.UnmarshalExact(&config)
	if err != nil {
		return
	}
	
	// Validate required configuration
	err = validateConfig(config)
	return
}

func validateConfig(config Config) error {
	if config.DatabaseURL == "" {
		return fmt.Errorf("DATABASE_URL is required")
	}
	if config.TokenSecretKey == "" {
		return fmt.Errorf("TOKEN_SECRET_KEY is required")
	}
	if config.GoogleClientID == "" {
		return fmt.Errorf("GOOGLE_CLIENT_ID is required")
	}
	if config.CloudinaryURL == "" {
		return fmt.Errorf("CLOUDINARY_URL is required")
	}
	if config.RedisServerAddress == "" {
		return fmt.Errorf("REDIS_SERVER_ADDRESS is required")
	}
	if config.Environment != EnvironmentDevelopment && config.Environment != EnvironmentProduction {
		return fmt.Errorf("ENVIRONMENT must be either %s or %s", EnvironmentDevelopment, EnvironmentProduction)
	}
	
	return nil
}
