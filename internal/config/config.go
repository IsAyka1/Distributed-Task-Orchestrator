package config

import (
	"errors"
	"os"
	"strings"
	"time"
)

type Config struct {
	DatabaseURL string
	HTTPAddress string
	Timeout     time.Duration
}

func Load() (Config, error) {
	cfg := Config{DatabaseURL: os.Getenv("DATABASE_URL"), Timeout: 10 * time.Second, HTTPAddress: "127.0.0.1:8080"}
	if strings.TrimSpace(cfg.DatabaseURL) == "" {
		return Config{}, errors.New("DATABASE_URL is required")
	}
	if value := os.Getenv("DATABASE_TIMEOUT"); value != "" {
		timeout, err := time.ParseDuration(value)
		if err != nil || timeout <= 0 || timeout > time.Minute {
			return Config{}, errors.New("DATABASE_TIMEOUT must be a duration greater than zero and at most 1m")
		}
		cfg.Timeout = timeout
	}
	if value := os.Getenv("HTTP_ADDR"); value != "" {
		cfg.HTTPAddress = value
	}
	return cfg, nil
}
