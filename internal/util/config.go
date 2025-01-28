package util

import (
	"fmt"
	"time"
	
	"github.com/spf13/viper"
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
	DiscordBotToken     string        `mapstructure:"DISCORD_BOT_TOKEN"`
	DiscordChannelID    string        `mapstructure:"DISCORD_CHANNEL_ID"`
}

// LoadConfig reads configuration from file or environment variables.
func LoadConfig(path string) (config Config, err error) {
	// Set defaults for non-sensitive config
	viper.SetDefault("ALLOWED_ORIGINS", []string{"http://localhost:3000"})
	viper.SetDefault("HTTP_SERVER_ADDRESS", "0.0.0.0:8080")
	viper.SetDefault("ACCESS_TOKEN_DURATION", "24h")
	
	// Prefer environment variables over config file
	viper.AutomaticEnv()
	
	// Load config file
	viper.SetConfigFile(path)
	if err = viper.ReadInConfig(); err != nil {
		return
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
	
	return nil
}
