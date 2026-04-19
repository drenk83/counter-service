package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"

	"github.com/drenk83/counter-service/internal/config"
	"github.com/drenk83/counter-service/internal/handler"
	"github.com/drenk83/counter-service/internal/repository"
	"github.com/drenk83/counter-service/internal/service"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"
)

func main() {
	// Config
	if err := godotenv.Load(); err != nil {
		slog.Warn(".env file not found")
	}
	cfg := config.Load()

	//Redis
	opt, err := redis.ParseURL(cfg.RedisURL)
	if err != nil {
		slog.Error("failed to parse redis url", "err", err)
		os.Exit(1)
	}
	rdb := redis.NewClient(opt)
	if err := rdb.Ping(context.Background()).Err(); err != nil {
		slog.Error("failed to connect to redis", "err", err)
		os.Exit(1)
	}
	defer rdb.Close()

	// Postgres
	db, err := pgxpool.New(context.Background(), cfg.PGDSN)
	if err != nil {
		slog.Error("failed to create postgres pool", "err", err)
		os.Exit(1)
	}
	if err := db.Ping(context.Background()); err != nil {
		slog.Error("failed to connect to postgres", "err", err)
		os.Exit(1)
	}
	defer db.Close()

	redisRepo := repository.NewRedisRepo(rdb)
	postgresRepo := repository.NewPostgresRepo(db)
	srv := service.NewCounterService(redisRepo, postgresRepo)
	h := handler.NewHandler(srv)

	r := chi.NewRouter()
	r.Post("/posts/{id}/view", h.HandleView)
	r.Post("/posts/{id}/like", h.HandleLike)
	r.Delete("/posts/{id}/like", h.HandleUnlike)
	r.Get("/posts/{id}/stats", h.HandleStats)
	r.Get("/posts/batch", h.HandleBatch)

	slog.Info("starting server", "addr", cfg.HTTPAddr)
	if err := http.ListenAndServe(cfg.HTTPAddr, r); err != nil {
		slog.Error("server error", "err", err)
		os.Exit(1)
	}
}
