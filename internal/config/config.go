package config

import (
	"log/slog"
	"os"
	"time"
)

type Config struct {
	HTTPAddr  string
	RedisURL  string
	PGDSN     string
	FlushTick time.Duration
}

func Load() *Config {
	return &Config{
		HTTPAddr:  getEnv("HTTP_ADDR", ":8080"),
		RedisURL:  getEnv("REDIS_URL", "redis://localhost:6379"),
		PGDSN:     getEnv("PG_DSN", "postgres://pg:pg@localhost:5432/counter?sslmode=disable"),
		FlushTick: getDuration("FLUSH_TICK", time.Second*10),
	}
}

func getEnv(key, fallback string) string {
	if out := os.Getenv(key); out != "" {
		return out
	}
	slog.Debug("use default settings", key, fallback)
	return fallback
}

func getDuration(key string, fallback time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		out, err := time.ParseDuration(v)
		if err == nil {
			return out
		}
	}
	slog.Debug("use default settings", key, fallback)
	return fallback
}
