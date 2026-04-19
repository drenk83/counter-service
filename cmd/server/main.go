package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/drenk83/counter-service/internal/config"
	"github.com/drenk83/counter-service/internal/handler"
	"github.com/drenk83/counter-service/internal/repository"
	"github.com/drenk83/counter-service/internal/service"
	"github.com/drenk83/counter-service/internal/worker"
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

	// Контекст для Flush, чтобы не потерять данные при shutdown
	rootCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	flusher := worker.NewFlusher(redisRepo, postgresRepo, cfg.FlushTick)
	flusherCtx, cancelFlusher := context.WithCancel(context.Background())
	flusherDone := make(chan struct{})
	go func() {
		defer close(flusherDone)
		flusher.Run(flusherCtx)
	}()

	// http
	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           r,
		ReadHeaderTimeout: 5 * time.Second,  // максимум 5 секунд на присылание HTTP-заголовков
		ReadTimeout:       15 * time.Second, // максимум 15 секунд на полное чтение запроса
		WriteTimeout:      15 * time.Second, // максимум 15 секунд на запись ответа
		IdleTimeout:       60 * time.Second, // максимум 60 секунд соединение без активности
	}
	serverErr := make(chan error, 1)
	go func() {
		slog.Info("starting server", "addr", cfg.HTTPAddr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()

	select {
	case <-rootCtx.Done():
		slog.Info("shutdown signal received")
		stop()
	case err := <-serverErr:
		slog.Error("server failed", "err", err)
	}

	// Shutdown
	shutdownCtx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		slog.Error("http shutdown failed", "err", err)
	}
	cancelFlusher()
	<-flusherDone

	if err := flusher.Flush(shutdownCtx); err != nil {
		slog.Error("final flush failed", "err", err)
	} else {
		slog.Info("final flush complete")
	}

	slog.Info("shutdown complete")
}
