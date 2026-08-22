-- +goose Up
ALTER TABLE providers
ADD COLUMN disabled BOOLEAN NOT NULL DEFAULT false;

ALTER TABLE providers
ADD COLUMN priority INT NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE providers DROP COLUMN disabled;
ALTER TABLE providers DROP COLUMN priority;
