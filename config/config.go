package config

import (
	"github.com/spf13/viper"
)

type Config struct {
	Environment           string `mapstructure:"ENVIRONMENT"`
	Port                  string `mapstructure:"PORT"`
	DatabaseURL           string `mapstructure:"DATABASE_URL"`
	JWTSecret             string `mapstructure:"JWT_SECRET"`
	RazorpayKeyID          string `mapstructure:"RAZORPAY_KEY_ID"`
	RazorpayKeySecret      string `mapstructure:"RAZORPAY_KEY_SECRET"`
	RazorpayWebhookSecret  string `mapstructure:"RAZORPAY_WEBHOOK_SECRET"`
	RazorpayAccountNumber  string `mapstructure:"RAZORPAY_ACCOUNT_NUMBER"` // Razorpay X account for payouts
	GoogleClientID        string `mapstructure:"GOOGLE_CLIENT_ID"`
	GoogleClientSecret    string `mapstructure:"GOOGLE_CLIENT_SECRET"`
	GoogleWebClientID     string `mapstructure:"GOOGLE_WEB_CLIENT_ID"`
	GoogleWebClientSecret string `mapstructure:"GOOGLE_WEB_CLIENT_SECRET"`
	InstagramClientID     string `mapstructure:"INSTAGRAM_CLIENT_ID"`
	InstagramClientSecret string `mapstructure:"INSTAGRAM_CLIENT_SECRET"`
	InstagramRedirectURI  string `mapstructure:"INSTAGRAM_REDIRECT_URI"`
	GeminiAPIKey          string `mapstructure:"GEMINI_API_KEY"`
}

func LoadConfig(path string) (config Config, err error) {
	// Enable automatic environment variable reading
	viper.AutomaticEnv()

	// Explicitly bind environment variables (required for AutomaticEnv to work)
	viper.BindEnv("ENVIRONMENT")
	viper.BindEnv("PORT")
	viper.BindEnv("DATABASE_URL")
	viper.BindEnv("JWT_SECRET")
	viper.BindEnv("RAZORPAY_KEY_ID")
	viper.BindEnv("RAZORPAY_KEY_SECRET")
	viper.BindEnv("RAZORPAY_WEBHOOK_SECRET")
	viper.BindEnv("RAZORPAY_ACCOUNT_NUMBER")
	viper.BindEnv("GOOGLE_CLIENT_ID")
	viper.BindEnv("GOOGLE_CLIENT_SECRET")
	viper.BindEnv("GOOGLE_WEB_CLIENT_ID")
	viper.BindEnv("GOOGLE_WEB_CLIENT_SECRET")
	viper.BindEnv("INSTAGRAM_CLIENT_ID")
	viper.BindEnv("INSTAGRAM_CLIENT_SECRET")
	viper.BindEnv("INSTAGRAM_REDIRECT_URI")
	viper.BindEnv("GEMINI_API_KEY")

	// Try to read .env file (optional for local development)
	viper.AddConfigPath(path)
	viper.SetConfigName(".env")
	viper.SetConfigType("env")

	err = viper.ReadInConfig()
	if err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			// Config file was found but another error occurred
			return
		}
		// Config file not found; ignore error and rely on env vars
		err = nil
	}

	// Unmarshal config from both file and environment variables
	err = viper.Unmarshal(&config)
	return
}
