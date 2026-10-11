package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Addr            string
	BaseURL         string
	MongoURI        string
	MongoDB         string
	FrontendURL     string
	AllowedOrigins  []string
	RateLimit       int
	ShutdownTimeout time.Duration
}

// Load reads configuration from environment variables, falling back to defaults.
func Load() (Config, error) {
	port := getEnv("PORT", "8080")
	rateLimit, err := strconv.Atoi(getEnv("RATE_LIMIT_PER_MIN", "20"))
	if err != nil || rateLimit < 0 {
		return Config{}, fmt.Errorf("RATE_LIMIT_PER_MIN must be a non-negative integer")
	}
	frontendURL := strings.TrimRight(os.Getenv("FRONTEND_URL"), "/")
	cfg := Config{
		Addr:            ":" + port,
		BaseURL:         strings.TrimRight(getEnv("BASE_URL", "http://localhost:"+port), "/"),
		MongoURI:        os.Getenv("MONGO_URI"),
		MongoDB:         getEnv("MONGO_DB", "urlshortener"),
		FrontendURL:     frontendURL,
		AllowedOrigins:  splitList(getEnv("ALLOWED_ORIGINS", frontendURL)),
		RateLimit:       rateLimit,
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

func splitList(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		if part = strings.TrimRight(strings.TrimSpace(part), "/"); part != "" {
			out = append(out, part)
		}
	}
	return out
}
