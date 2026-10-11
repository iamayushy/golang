package config

import (
	"os"
	"time"
)

type Config struct {
	Addr            string
	ShutdownTimeout time.Duration
}

// Load reads configuration from environment variables, falling back to defaults.
func Load() Config {
	return Config{
		Addr:            ":" + getEnv("PORT", "8080"),
		ShutdownTimeout: 10 * time.Second,
	}
}

func getEnv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}
