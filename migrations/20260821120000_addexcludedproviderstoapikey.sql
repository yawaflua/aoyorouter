-- +goose Up
ALTER TABLE api_keys
ADD COLUMN excluded_providers TEXT[] NOT NULL DEFAULT '{}',
ADD COLUMN excluded_models TEXT[] NOT NULL DEFAULT '{}';

-- +goose Down
ALTER TABLE api_keys DROP COLUMN excluded_providers;
ALTER TABLE api_keys DROP COLUMN excluded_models;
