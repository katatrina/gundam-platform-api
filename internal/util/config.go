package util

import (
	"errors"
	"fmt"
	"os"
	"reflect"
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
	DatabaseURL         string        `mapstructure:"DATABASE_URL" validate:"required"`
	HTTPServerAddress   string        `mapstructure:"HTTP_SERVER_ADDRESS"`
	TokenSecretKey      string        `mapstructure:"TOKEN_SECRET_KEY" validate:"required"`
	AccessTokenDuration time.Duration `mapstructure:"ACCESS_TOKEN_DURATION"`
	GoogleClientID      string        `mapstructure:"GOOGLE_CLIENT_ID" validate:"required"`
	CloudinaryURL       string        `mapstructure:"CLOUDINARY_URL" validate:"required"`
	RedisServerAddress  string        `mapstructure:"REDIS_SERVER_ADDRESS" validate:"required"`
	RedisServerPassword string        `mapstructure:"REDIS_SERVER_PASSWORD" validate:"required"`
	DiscordBotToken     string        `mapstructure:"DISCORD_BOT_TOKEN" validate:"required"`
	DiscordChannelID    string        `mapstructure:"DISCORD_CHANNEL_ID" validate:"required"`
	GmailSMTPUsername   string        `mapstructure:"GMAIL_SMTP_USERNAME" validate:"required"`
	GmailSMTPPassword   string        `mapstructure:"GMAIL_SMTP_PASSWORD" validate:"required"`
	ZalopayCallbackURL  string        `mapstructure:"ZALOPAY_CALLBACK_URL"`
	Environment         string        `mapstructure:"ENVIRONMENT"`
	NgrokAuthToken      string        `mapstructure:"NGROK_AUTH_TOKEN" validate:"required"`
	GHNShopID           string        `mapstructure:"GHN_SHOP_ID" validate:"required"`
	GHNToken            string        `mapstructure:"GHN_TOKEN" validate:"required"`
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
		
		// Only use environment variables
		viper.AutomaticEnv()
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

// validateConfig validates the configuration using struct tags
func validateConfig(config Config) error {
	v := reflect.ValueOf(config)
	t := reflect.TypeOf(config)
	
	var missingFields []string
	
	for i := 0; i < v.NumField(); i++ {
		field := t.Field(i)
		value := v.Field(i)
		
		// Check if field has "required" tag
		if validateTag := field.Tag.Get("validate"); validateTag == "required" {
			// Check if the field is empty
			if value.Kind() == reflect.String && value.String() == "" {
				fieldName := field.Tag.Get("mapstructure")
				if fieldName == "" {
					fieldName = field.Name
				}
				missingFields = append(missingFields, fieldName)
			}
		}
	}
	
	if len(missingFields) > 0 {
		return fmt.Errorf("required fields are missing: %v", missingFields)
	}
	
	if config.Environment != EnvironmentDevelopment && config.Environment != EnvironmentProduction {
		return fmt.Errorf("ENVIRONMENT must be either %s or %s", EnvironmentDevelopment, EnvironmentProduction)
	}
	
	return nil
}
