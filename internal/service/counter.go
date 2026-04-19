package service

import (
	"context"

	"github.com/drenk83/counter-service/internal/repository"
)

type RedisRepository interface {
	MarkViewed(ctx context.Context, postID int64, userID string) (bool, error)
	IncrView(ctx context.Context, postID int64) error

	MarkLiked(ctx context.Context, postID int64, userID string) (bool, error)
	UnmarkLiked(ctx context.Context, postID int64, userID string) (bool, error)
	IncrLike(ctx context.Context, postID int64) error
	DecrLike(ctx context.Context, postID int64) error

	GetStats(ctx context.Context, postID int64) (*repository.Stats, error)
	MGetStats(ctx context.Context, postIDs []int64) ([]*repository.Stats, error)
	AddToDirty(ctx context.Context, postID int64) error
}

type PostgresRepository interface {
	GetStats(ctx context.Context, postID int64) (*repository.Stats, error)
	GetStatsBatch(ctx context.Context, postIDs []int64) (map[int64]repository.Stats, error)
}

type CounterService struct {
	redis    RedisRepository
	postgres PostgresRepository
}

func NewCounterService(r RedisRepository, p PostgresRepository) *CounterService {
	return &CounterService{redis: r, postgres: p}
}

func (s *CounterService) AddView(ctx context.Context, postID int64, userID string) error {
	ok, err := s.redis.MarkViewed(ctx, postID, userID)
	if err != nil {
		return err
	}
	if ok {
		if err := s.redis.IncrView(ctx, postID); err != nil {
			return err
		}
		if err := s.redis.AddToDirty(ctx, postID); err != nil {
			return err
		}
	}
	return nil
}

func (s *CounterService) AddLike(ctx context.Context, postID int64, userID string) error {
	ok, err := s.redis.MarkLiked(ctx, postID, userID)
	if err != nil {
		return err
	}
	if ok {
		if err := s.redis.IncrLike(ctx, postID); err != nil {
			return err
		}
		if err := s.redis.AddToDirty(ctx, postID); err != nil {
			return err
		}
	}
	return nil
}

func (s *CounterService) RemoveLike(ctx context.Context, postID int64, userID string) error {
	ok, err := s.redis.UnmarkLiked(ctx, postID, userID)
	if err != nil {
		return err
	}
	if ok {
		if err := s.redis.DecrLike(ctx, postID); err != nil {
			return err
		}
		if err := s.redis.AddToDirty(ctx, postID); err != nil {
			return err
		}
	}
	return nil
}

func (s *CounterService) GetStats(ctx context.Context, postID int64) (*repository.Stats, error) {
	postgStat, err := s.postgres.GetStats(ctx, postID)
	if err != nil {
		return nil, err
	}
	redisStat, err := s.redis.GetStats(ctx, postID)
	if err != nil {
		return nil, err
	}
	return &repository.Stats{
		Views: redisStat.Views + postgStat.Views,
		Likes: redisStat.Likes + postgStat.Likes,
	}, nil
}

func (s *CounterService) GetStatsBatch(ctx context.Context, postIDs []int64) ([]*repository.Stats, error) {
	postgStats, err := s.postgres.GetStatsBatch(ctx, postIDs)
	if err != nil {
		return nil, err
	}
	redisStats, err := s.redis.MGetStats(ctx, postIDs)
	if err != nil {
		return nil, err
	}

	stats := make([]*repository.Stats, len(postIDs))
	for i, id := range postIDs {
		stats[i] = &repository.Stats{
			Views: redisStats[i].Views + postgStats[id].Views,
			Likes: redisStats[i].Likes + postgStats[id].Likes,
		}
	}
	return stats, nil
}
