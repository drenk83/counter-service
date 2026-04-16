package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const viewedTTL = time.Hour * 24

type RedisRepo struct {
	client *redis.Client
}

type Stats struct {
	Likes int64
	Views int64
}

func NewRedisRepo(client *redis.Client) *RedisRepo {
	return &RedisRepo{client: client}
}

func viewsKey(postID int64) string {
	return fmt.Sprintf("views:%d", postID)
}

func likesKey(postID int64) string {
	return fmt.Sprintf("likes:%d", postID)
}

func viewedKey(postID int64) string {
	return fmt.Sprintf("viewed:%d", postID)
}

func likedKey(postID int64) string {
	return fmt.Sprintf("liked:%d", postID)
}

// Viewed
func (r *RedisRepo) IsViewed(ctx context.Context, postID int64, userID string) (bool, error) {
	return r.client.SIsMember(ctx, viewedKey(postID), userID).Result()
}

func (r *RedisRepo) SetViewed(ctx context.Context, postID int64, userID string) error {
	key := viewedKey(postID)
	if err := r.client.SAdd(ctx, key, userID).Err(); err != nil {
		return err
	}
	return r.client.Expire(ctx, key, viewedTTL).Err()
}

func (r *RedisRepo) IncrView(ctx context.Context, postID int64) error {
	return r.client.Incr(ctx, viewsKey(postID)).Err()
}

// Liked
func (r *RedisRepo) IsLiked(ctx context.Context, postID int64, userID string) (bool, error) {
	return r.client.SIsMember(ctx, likedKey(postID), userID).Result()
}

func (r *RedisRepo) SetLiked(ctx context.Context, postID int64, userID string) error {
	return r.client.SAdd(ctx, likedKey(postID), userID).Err()
}

func (r *RedisRepo) RemoveLiked(ctx context.Context, postID int64, userID string) error {
	return r.client.SRem(ctx, likedKey(postID), userID).Err()
}

func (r *RedisRepo) IncrLike(ctx context.Context, postID int64) error {
	return r.client.Incr(ctx, likesKey(postID)).Err()
}

func (r *RedisRepo) DecrLike(ctx context.Context, postID int64) error {
	return r.client.Decr(ctx, likesKey(postID)).Err()
}

// Stats
func (r *RedisRepo) GetStats(ctx context.Context, postID int64) (*Stats, error) {
	stat := &Stats{}

	views, err := r.client.Get(ctx, viewsKey(postID)).Int64()
	if err != nil && err != redis.Nil {
		return nil, err
	}
	stat.Views = views

	likes, err := r.client.Get(ctx, likesKey(postID)).Int64()
	if err != nil && err != redis.Nil {
		return nil, err
	}
	stat.Likes = likes

	return stat, nil
}

func (r *RedisRepo) MGetStats(ctx context.Context, postID []int64) ([]*Stats, error) {
	// soon
}
