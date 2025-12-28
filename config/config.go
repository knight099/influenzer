package config

import (
	"github.com/spf13/viper"
)

type Config struct {
	Environment       string `mapstructure:"ENVIRONMENT"`
	Port              string `mapstructure:"PORT"`
	DatabaseURL       string `mapstructure:"DATABASE_URL"`
	JWTSecret         string `mapstructure:"JWT_SECRET"`
	RazorpayKeyID     string `mapstructure:"RAZORPAY_KEY_ID"`
	RazorpayKeySecret string `mapstructure:"RAZORPAY_KEY_SECRET"`
}

func LoadConfig(path string) (config Config, err error) {
	viper.AddConfigPath(path)
	viper.SetConfigName(".env")
	viper.SetConfigType("env")

	viper.AutomaticEnv()

	err = viper.ReadInConfig()
	if err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return
		}
		// Config file not found; ignore error if desired and rely on env vars
		err = nil
	}

	err = viper.Unmarshal(&config)
	return
}
