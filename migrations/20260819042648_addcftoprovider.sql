-- +goose Up
ALTER TABLE providers
    ADD COLUMN is_cloudflare BOOLEAN NOT NULL DEFAULT FALSE;

-- +goose Down
ALTER TABLE providers
    DROP COLUMN is_cloudflare;
