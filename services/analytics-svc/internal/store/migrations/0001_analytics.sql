-- Idempotency ledger: one row per click event ever applied. Pruned after the
-- JetStream retention window (an event older than that can't be redelivered).
CREATE TABLE processed_events (
    event_id     text        PRIMARY KEY,
    processed_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX processed_events_processed_at_idx ON processed_events (processed_at);

CREATE TABLE clicks_hourly (
    slug   text        NOT NULL,
    hour   timestamptz NOT NULL,
    clicks bigint      NOT NULL CHECK (clicks >= 0),
    PRIMARY KEY (slug, hour)
);

CREATE TABLE referrers (
    slug     text   NOT NULL,
    referrer text   NOT NULL,
    clicks   bigint NOT NULL CHECK (clicks >= 0),
    PRIMARY KEY (slug, referrer)
);

CREATE TABLE link_totals (
    slug          text        PRIMARY KEY,
    clicks        bigint      NOT NULL CHECK (clicks >= 0),
    last_click_at timestamptz
);
