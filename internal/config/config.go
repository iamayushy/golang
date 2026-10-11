package config

import (
	"errors"
	"os"
	"strings"
	"time"
)

type Config struct {
	Addr            string
	BaseURL         string
	MongoURI        string
	MongoDB         string
	ShutdownTimeout time.Duration
}

// Load reads configuration from environment variables, falling back to defaults.
func Load() (Config, error) {
	port := getEnv("PORT", "8080")
	cfg := Config{
		Addr:            ":" + port,
		BaseURL:         strings.TrimRight(getEnv("BASE_URL", "http://localhost:"+port), "/"),
		MongoURI:        os.Getenv("MONGO_URI"),
		MongoDB:         getEnv("MONGO_DB", "urlshortener"),
		ShutdownTimeout: 10 * time.Second,
	}
	if cfg.MongoURI == "" {
		return Config{}, errors.New("MONGO_URI is required")
	}
	return cfg, nil
}

func getEnv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}
