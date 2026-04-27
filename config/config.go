package config

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/spf13/viper"
)

func NewConfig() (*Config, error) {
	var cfg Config
	env := os.Getenv("APP_ENV")
	if env == "" {
		env = "dev"
	}
	fmt.Printf("APP_ENV=%s\n", env)
	v := viper.New()
	v.SetDefault("Features.dddCoreEnabled", true)
	v.SetDefault("Features.apiBreakingEnabled", true)
	v.AddConfigPath("config")
	v.SetConfigName("config." + env)
	v.SetConfigType("yml")
	err := v.ReadInConfig()
	if err != nil {
		slog.Error("fail to read config", slog.Any("error", err))
		return &cfg, err
	}
	err = v.Unmarshal(&cfg)
	if err != nil {
		slog.Error("unable to decode config into struct", slog.Any("error", err))
		return &cfg, err
	}
	return &cfg, nil
}
