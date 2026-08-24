-- +goose Up
ALTER TABLE api_keys ADD COLUMN is_admin BOOLEAN NOT NULL DEFAULT false;

-- +goose Down
ALTER TABLE api_keys DROP COLUMN is_admin;
