package worker

import (
	"context"
	"log/slog"
	"time"

	"github.com/drenk83/counter-service/internal/repository"
)

type RedisRepo interface {
	AddToDirty(ctx context.Context, postID int64) error
	PopDirty(ctx context.Context) ([]int64, error)
	PopDeltasBatch(ctx context.Context, postIDs []int64) (map[int64]repository.Stats, error)
	RestoreDelta(ctx context.Context, postID, views, likes int64) error
}

type PostgresRepo interface {
	FlushStatsBatch(ctx context.Context, deltas map[int64]repository.Stats) error
}

type Flusher struct {
	redis    RedisRepo
	postgres PostgresRepo
	interval time.Duration
}

func NewFlusher(r RedisRepo, p PostgresRepo, inter time.Duration) *Flusher {
	return &Flusher{
		redis:    r,
		postgres: p,
		interval: inter,
	}
}

func (f *Flusher) Run(ctx context.Context) {
	ticker := time.NewTicker(f.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := f.Flush(ctx); err != nil {
				slog.Error("flush cycle failed", "err", err)
			}
		}
	}
}

func (f *Flusher) Flush(ctx context.Context) error {
	ids, err := f.redis.PopDirty(ctx)
	if err != nil {
		return err
	}
	if len(ids) == 0 {
		return nil
	}

	deltas, err := f.redis.PopDeltasBatch(ctx, ids)
	if err != nil {
		for _, id := range ids {
			if err := f.redis.AddToDirty(ctx, id); err != nil {
				slog.Error("re-add to dirty failed", "post_id", id, "err", err)
			}
		}
		return err
	}
	if len(deltas) == 0 {
		return nil
	}

	if err := f.postgres.FlushStatsBatch(ctx, deltas); err != nil {
		for id, del := range deltas {
			if err := f.redis.RestoreDelta(ctx, id, del.Views, del.Likes); err != nil {
				slog.Error("restore delta failed", "post_id", id, "err", err)
			}
			if err := f.redis.AddToDirty(ctx, id); err != nil {
				slog.Error("re-add to dirty failed", "post_id", id, "err", err)
			}
		}
		return err
	}
	slog.Info("flushed", "posts", len(deltas))
	return nil
}
