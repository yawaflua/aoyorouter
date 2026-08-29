-- +goose Up
CREATE TABLE push_subscriptions (
    id              UUID PRIMARY KEY,
    endpoint        TEXT NOT NULL UNIQUE,
    p256dh          TEXT NOT NULL,
    auth            TEXT NOT NULL,
    expiration_time BIGINT,
    user_agent      TEXT NOT NULL DEFAULT '',
    labels          JSONB NOT NULL DEFAULT '{}',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE push_subscription_topics (
    subscription_id UUID NOT NULL REFERENCES push_subscriptions (id) ON DELETE CASCADE,
    subject         TEXT NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (subscription_id, subject)
);

CREATE INDEX push_subscription_topics_subject_idx ON push_subscription_topics (subject);

-- Single-row table holding the generated VAPID keypair, so subscriptions stay
-- valid across restarts when the operator has not pinned keys in the env.
CREATE TABLE push_vapid_keys (
    id          BOOLEAN PRIMARY KEY DEFAULT true,
    public_key  TEXT NOT NULL,
    private_key TEXT NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT push_vapid_keys_single_row CHECK (id)
);

-- +goose Down
DROP TABLE push_vapid_keys;
DROP TABLE push_subscription_topics;
DROP TABLE push_subscriptions;
