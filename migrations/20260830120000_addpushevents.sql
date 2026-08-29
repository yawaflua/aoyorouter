-- +goose Up
CREATE TABLE push_events (
    id          BIGSERIAL PRIMARY KEY,
    subject     TEXT NOT NULL,
    title       TEXT NOT NULL,
    body        TEXT NOT NULL,
    tag         TEXT NOT NULL DEFAULT '',
    provider_id TEXT NOT NULL DEFAULT '',
    url         TEXT NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX push_events_subject_id_idx ON push_events (subject, id);

-- +goose Down
DROP TABLE push_events;
