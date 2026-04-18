package repository

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresRepo struct {
	db *pgxpool.Pool
}

func NewPostgresRepo(db *pgxpool.Pool) *PostgresRepo {
	return &PostgresRepo{
		db: db,
	}
}

func (p *PostgresRepo) FlushStats(ctx context.Context, postID, deltaViews, deltaLikes int64) error {
	_, err := p.db.Exec(ctx, `
		INSERT INTO post_stats (post_id, views, likes, updated_at)
		VALUES ($1, $2, $3, now())
		ON CONFLICT (post_id)
		DO UPDATE SET
			views      = post_stats.views + EXCLUDED.views,
			likes      = post_stats.likes + EXCLUDED.likes,
			updated_at = now()
	`, postID, deltaViews, deltaLikes)
	return err
}

func (p *PostgresRepo) FlushStatsBatch(ctx context.Context, deltas map[int64]Stats) error {
	if len(deltas) == 0 {
		return nil
	}
	batch := &pgx.Batch{}
	for postID, s := range deltas {
		batch.Queue(`
            INSERT INTO post_stats (post_id, views, likes, updated_at)
            VALUES ($1, $2, $3, now())
            ON CONFLICT (post_id) DO UPDATE SET
                views      = post_stats.views + EXCLUDED.views,
                likes      = post_stats.likes + EXCLUDED.likes,
                updated_at = now()
        `, postID, s.Views, s.Likes)
	}
	br := p.db.SendBatch(ctx, batch)
	defer br.Close()
	for i := 0; i < len(deltas); i++ {
		if _, err := br.Exec(); err != nil {
			return err
		}
	}
	return nil
}

func (p *PostgresRepo) GetStats(ctx context.Context, postID int64) (*Stats, error) {
	stat := &Stats{}
	err := p.db.QueryRow(ctx, `
		SELECT views, likes FROM post_stats WHERE post_id = $1
	`, postID).Scan(&stat.Views, &stat.Likes)

	if err == pgx.ErrNoRows {
		return &Stats{}, nil
	}
	if err != nil {
		return nil, err
	}

	return stat, nil
}

func (p *PostgresRepo) GetStatsBatch(ctx context.Context, postIDs []int64) (map[int64]Stats, error) {
	if len(postIDs) == 0 {
		return nil, nil
	}
	rows, err := p.db.Query(ctx, `
        SELECT post_id, views, likes
        FROM post_stats
        WHERE post_id = ANY($1)
    `, postIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make(map[int64]Stats, len(postIDs))
	for rows.Next() {
		var id int64
		var s Stats
		if err := rows.Scan(&id, &s.Views, &s.Likes); err != nil {
			return nil, err
		}
		out[id] = s
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}
