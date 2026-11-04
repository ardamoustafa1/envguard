CREATE TABLE IF NOT EXISTS entries (
    id           BIGSERIAL PRIMARY KEY,
    account      TEXT        NOT NULL,
    amount_cents BIGINT      NOT NULL,
    memo         TEXT        NOT NULL DEFAULT '',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS entries_account_idx ON entries (account, id DESC);
