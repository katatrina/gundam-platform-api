package util

import (
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
}

// LoadConfig reads configuration from file or environment variables.
func LoadConfig(path string) (config Config, err error) {
	viper.AutomaticEnv()
	viper.SetConfigFile(path)
	
	err = viper.ReadInConfig()
	if err != nil {
		return
	}
	
	err = viper.UnmarshalExact(&config)
	if err != nil {
		return
	}
	
	return
}
