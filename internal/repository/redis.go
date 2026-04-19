package repository

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

const viewedTTL = time.Hour * 24
const dirtyKey = "dirty"

type RedisRepo struct {
	client *redis.Client
}

type Stats struct {
	Likes int64
	Views int64
}

// Builder
func NewRedisRepo(client *redis.Client) *RedisRepo {
	return &RedisRepo{client: client}
}

// Keys
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
func (r *RedisRepo) MarkViewed(ctx context.Context, postID int64, userID string) (bool, error) {
	key := viewedKey(postID)
	added, err := r.client.SAdd(ctx, key, userID).Result()
	if err != nil {
		return false, err
	}
	if added == 1 {
		if err := r.client.Expire(ctx, key, viewedTTL).Err(); err != nil {
			return false, err
		}
	}
	return added == 1, nil
}

func (r *RedisRepo) IncrView(ctx context.Context, postID int64) error {
	return r.client.Incr(ctx, viewsKey(postID)).Err()
}

// Liked
func (r *RedisRepo) MarkLiked(ctx context.Context, postID int64, userID string) (bool, error) {
	key := likedKey(postID)
	added, err := r.client.SAdd(ctx, key, userID).Result()
	if err != nil {
		return false, err
	}
	return added == 1, nil
}

func (r *RedisRepo) UnmarkLiked(ctx context.Context, postID int64, userID string) (bool, error) {
	key := likedKey(postID)
	removed, err := r.client.SRem(ctx, key, userID).Result()
	if err != nil {
		return false, err
	}
	return removed == 1, nil
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
	if err != nil && !errors.Is(err, redis.Nil) {
		return nil, err
	}
	stat.Views = views

	likes, err := r.client.Get(ctx, likesKey(postID)).Int64()
	if err != nil && !errors.Is(err, redis.Nil) {
		return nil, err
	}
	stat.Likes = likes

	return stat, nil
}

func (r *RedisRepo) MGetStats(ctx context.Context, postID []int64) ([]*Stats, error) {
	n := len(postID)
	if n == 0 {
		return nil, nil
	}

	keys := make([]string, n*2)
	for i, id := range postID {
		keys[i] = viewsKey(id)
		keys[i+n] = likesKey(id)
	}

	vals, err := r.client.MGet(ctx, keys...).Result()
	if err != nil {
		return nil, err
	}

	stats := make([]*Stats, n)
	for i := range stats {
		stat := &Stats{}
		if s, ok := vals[i].(string); ok {
			stat.Views, _ = strconv.ParseInt(s, 10, 64)
		}
		if s, ok := vals[i+n].(string); ok {
			stat.Likes, _ = strconv.ParseInt(s, 10, 64)
		}
		stats[i] = stat
	}
	return stats, nil

}

func (r *RedisRepo) AddToDirty(ctx context.Context, postID int64) error {
	return r.client.SAdd(ctx, dirtyKey, postID).Err()
}

func (r *RedisRepo) PopDirty(ctx context.Context) ([]int64, error) {
	strs, err := r.client.SPopN(ctx, dirtyKey, 10000).Result()
	if err != nil {
		return nil, err
	}
	out := make([]int64, 0, len(strs))
	for _, s := range strs {
		id, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			continue
		}
		out = append(out, id)
	}
	return out, nil
}

func (r *RedisRepo) PopDeltasBatch(ctx context.Context, postIDs []int64) (map[int64]Stats, error) {
	if len(postIDs) == 0 {
		return nil, nil
	}

	pipe := r.client.Pipeline()
	viewCmds := make([]*redis.StringCmd, len(postIDs))
	likeCmds := make([]*redis.StringCmd, len(postIDs))

	for i, id := range postIDs {
		viewCmds[i] = pipe.GetDel(ctx, viewsKey(id))
		likeCmds[i] = pipe.GetDel(ctx, likesKey(id))
	}

	// Exec может вернуть redis.Nil, если хотя бы один ключ отсутствовал — это ок.
	if _, err := pipe.Exec(ctx); err != nil && !errors.Is(err, redis.Nil) {
		return nil, err
	}

	out := make(map[int64]Stats, len(postIDs))
	for i, id := range postIDs {
		var s Stats
		if v, err := viewCmds[i].Int64(); err == nil {
			s.Views = v
		}
		if l, err := likeCmds[i].Int64(); err == nil {
			s.Likes = l
		}
		if s.Views != 0 || s.Likes != 0 {
			out[id] = s
		}
	}
	return out, nil
}

func (r *RedisRepo) RestoreDelta(ctx context.Context, postID, views, likes int64) error {
	if views == 0 && likes == 0 {
		return nil
	}
	pipe := r.client.Pipeline()
	if views != 0 {
		pipe.IncrBy(ctx, viewsKey(postID), views)
	}
	if likes != 0 {
		pipe.IncrBy(ctx, likesKey(postID), likes)
	}
	_, err := pipe.Exec(ctx)
	return err
}
