package main

import (
	"log/slog"

	"github.com/drenk83/counter-service/internal/config"
	"github.com/joho/godotenv"
)

func main() {
	if err := godotenv.Load(); err != nil {
		slog.Warn(".env file not found")
	}
	cfg := config.Load()
	_ = cfg
}
