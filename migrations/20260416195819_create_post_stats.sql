-- +goose Up
CREATE TABLE post_stats (
    post_id    BIGINT PRIMARY KEY REFERENCES posts(id) ON DELETE CASCADE,
    views      BIGINT NOT NULL DEFAULT 0,
    likes      BIGINT NOT NULL DEFAULT 0,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);


-- +goose Down
DROP TABLE post_stats;