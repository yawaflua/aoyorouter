-- +goose Up
ALTER TABLE api_keys
    ADD COLUMN quota_setted BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN quota_tokens BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN quota_period TEXT NOT NULL DEFAULT 'forever',
    ADD COLUMN quota_reset_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    ADD COLUMN reserved_tokens BIGINT DEFAULT 0;


-- +goose Down
ALTER TABLE api_keys
    DROP COLUMN quota_setted,
    DROP COLUMN quota_tokens,
    DROP COLUMN quota_period,
    DROP COLUMN quota_reset_at,
    DROP COLUMN reserved_tokens;
