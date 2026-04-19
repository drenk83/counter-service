package service

import (
	"context"

	"github.com/drenk83/counter-service/internal/repository"
)

type RedisRepository interface {
	IsViewed(ctx context.Context, postID int64, userID string) (bool, error)
	SetViewed(ctx context.Context, postID int64, userID string) error
	IncrView(ctx context.Context, postID int64) error

	IsLiked(ctx context.Context, postID int64, userID string) (bool, error)
	SetLiked(ctx context.Context, postID int64, userID string) error
	RemoveLiked(ctx context.Context, postID int64, userID string) error
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
	ok, err := s.redis.IsViewed(ctx, postID, userID)
	if err != nil {
		return err
	}
	if ok {
		return nil
	}
	if err := s.redis.SetViewed(ctx, postID, userID); err != nil {
		return err
	}
	if err := s.redis.IncrView(ctx, postID); err != nil {
		return err
	}
	if err := s.redis.AddToDirty(ctx, postID); err != nil {
		return err
	}
	return nil
}

func (s *CounterService) AddLike(ctx context.Context, postID int64, userID string) error {
	ok, err := s.redis.IsLiked(ctx, postID, userID)
	if err != nil {
		return err
	}
	if ok {
		return nil
	}
	if err := s.redis.SetLiked(ctx, postID, userID); err != nil {
		return err
	}
	if err := s.redis.IncrLike(ctx, postID); err != nil {
		return err
	}
	if err := s.redis.AddToDirty(ctx, postID); err != nil {
		return err
	}
	return nil
}

func (s *CounterService) RemoveLike(ctx context.Context, postID int64, userID string) error {
	ok, err := s.redis.IsLiked(ctx, postID, userID)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	if err := s.redis.RemoveLiked(ctx, postID, userID); err != nil {
		return err
	}
	if err := s.redis.DecrLike(ctx, postID); err != nil {
		return err
	}
	if err := s.redis.AddToDirty(ctx, postID); err != nil {
		return err
	}
	return nil
}

func (s *CounterService) GetStats(ctx context.Context, postID int64) (*repository.Stats, error) {
	redisStat, err := s.redis.GetStats(ctx, postID)
	if err != nil {
		return nil, err
	}
	postgStat, err := s.postgres.GetStats(ctx, postID)
	if err != nil {
		return nil, err
	}
	return &repository.Stats{
		Views: redisStat.Views + postgStat.Views,
		Likes: redisStat.Likes + postgStat.Likes,
	}, nil
}

func (s *CounterService) GetStatsBatch(ctx context.Context, postIDs []int64) ([]*repository.Stats, error) {
	redisStats, err := s.redis.MGetStats(ctx, postIDs)
	if err != nil {
		return nil, err
	}

	postgStats, err := s.postgres.GetStatsBatch(ctx, postIDs)
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
