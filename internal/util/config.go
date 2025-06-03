package util

import (
	"errors"
	"fmt"
	"os"
	"strings"
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

// LoadConfig reads configuration from file (dev) or environment variables (prod)
func LoadConfig(path string) (config Config, err error) {
	// Clear any previous viper state
	viper.Reset()
	
	// Determine environment
	environment := determineEnvironment()
	
	if environment == EnvironmentDevelopment {
		// DEVELOPMENT MODE: Load from file first, then allow env vars to overwrite
		log.Info().Msg("Development mode: Loading from config file with env var overrides")
		
		// Set defaults for development
		viper.SetDefault("ALLOWED_ORIGINS", []string{"http://localhost:5173"})
		viper.SetDefault("HTTP_SERVER_ADDRESS", "0.0.0.0:8080")
		viper.SetDefault("ACCESS_TOKEN_DURATION", "24h")
		viper.SetDefault("ENVIRONMENT", environment)
		
		// Load config file (required in development)
		if path == "" {
			return config, fmt.Errorf("config file path is required in development mode")
		}
		
		viper.SetConfigFile(path)
		if err = viper.ReadInConfig(); err != nil {
			var configFileNotFoundError viper.ConfigFileNotFoundError
			if errors.As(err, &configFileNotFoundError) {
				return config, fmt.Errorf("config file not found: %s (required in development)", path)
			}
			return config, fmt.Errorf("failed to read config file %s: %w", path, err)
		}
		
		// Allow environment variables to overwrite file values
		viper.AutomaticEnv()
		
	} else {
		// PRODUCTION MODE: Environment variables only
		log.Info().Msg("Production mode: Using environment variables only")
		
		viper.SetDefault("ENVIRONMENT", environment)
		
		// Configure viper for environment variables
		viper.AutomaticEnv()
		viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
		
		// Bind all environment variables
		envVars := []string{
			"DATABASE_URL",
			"TOKEN_SECRET_KEY",
			"GOOGLE_CLIENT_ID",
			"CLOUDINARY_URL",
			"REDIS_SERVER_ADDRESS",
			"REDIS_SERVER_PASSWORD",
			"DISCORD_BOT_TOKEN",
			"DISCORD_CHANNEL_ID",
			"GMAIL_SMTP_USERNAME",
			"GMAIL_SMTP_PASSWORD",
			"NGROK_AUTH_TOKEN",
			"GHN_SHOP_ID",
			"GHN_TOKEN",
			"ALLOWED_ORIGINS",
			"HTTP_SERVER_ADDRESS",
			"ACCESS_TOKEN_DURATION",
			"ZALOPAY_CALLBACK_URL",
		}
		
		for _, env := range envVars {
			err = viper.BindEnv(env)
			if err != nil {
				return config, fmt.Errorf("failed to bind environment variable %s: %w", env, err)
			}
		}
	}
	
	// Unmarshal config into struct
	err = viper.UnmarshalExact(&config)
	if err != nil {
		return config, fmt.Errorf("failed to unmarshal config: %w", err)
	}
	
	// Post-process configuration
	config = postProcessConfig(config)
	
	// Validate configuration
	err = validateConfig(config)
	if err != nil {
		if environment == EnvironmentProduction {
			log.Fatal().Err(err).Msg("Config validation failed in production")
		}
		return config, fmt.Errorf("config validation failed: %w", err)
	}
	
	log.Info().Msg("Configurations loaded successfully ✅")
	
	return config, nil
}

// determineEnvironment checks environment variable to determine mode
func determineEnvironment() string {
	// Check explicit environment setting
	if env := os.Getenv("ENVIRONMENT"); env != "" {
		return env
	}
	
	// Default to development
	return EnvironmentDevelopment
}

// postProcessConfig handles any post-processing of configuration values
func postProcessConfig(config Config) Config {
	// Handle ALLOWED_ORIGINS if it's provided as a comma-separated string
	if len(config.AllowedOrigins) == 1 && strings.Contains(config.AllowedOrigins[0], ",") {
		origins := strings.Split(config.AllowedOrigins[0], ",")
		for i, origin := range origins {
			origins[i] = strings.TrimSpace(origin)
		}
		config.AllowedOrigins = origins
	}
	
	return config
}

// validateConfig validates the configuration
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
	if config.RedisServerPassword == "" && config.Environment == EnvironmentProduction {
		return fmt.Errorf("REDIS_SERVER_PASSWORD is required")
	}
	if config.DiscordBotToken == "" {
		return fmt.Errorf("DISCORD_BOT_TOKEN is required")
	}
	if config.DiscordChannelID == "" {
		return fmt.Errorf("DISCORD_CHANNEL_ID is required")
	}
	if config.GmailSMTPUsername == "" {
		return fmt.Errorf("GMAIL_SMTP_USERNAME is required")
	}
	if config.GmailSMTPPassword == "" {
		return fmt.Errorf("GMAIL_SMTP_PASSWORD is required")
	}
	if config.NgrokAuthToken == "" {
		return fmt.Errorf("NGROK_AUTH_TOKEN is required")
	}
	if config.GHNShopID == "" {
		return fmt.Errorf("GHN_SHOP_ID is required")
	}
	if config.GHNToken == "" {
		return fmt.Errorf("GHN_TOKEN is required")
	}
	
	if config.Environment != EnvironmentDevelopment && config.Environment != EnvironmentProduction {
		return fmt.Errorf("ENVIRONMENT must be either %s or %s", EnvironmentDevelopment, EnvironmentProduction)
	}
	
	return nil
}
