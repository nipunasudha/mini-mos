-- +goose Up
-- Forward-only: this is a demo ledger, we never roll schema back.

CREATE TABLE clients (
  id          TEXT PRIMARY KEY,
  name        TEXT NOT NULL,
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Ledger accounts: one per pot, plus the external accounts.
CREATE TABLE accounts (
  id            TEXT PRIMARY KEY,            -- e.g. 'c-1001:rent', 'external:income'
  client_id     TEXT REFERENCES clients(id), -- NULL for external accounts
  pot           TEXT,                        -- NULL for external accounts
  balance_cents BIGINT NOT NULL DEFAULT 0,
  is_external   BOOLEAN NOT NULL DEFAULT false,
  CHECK (is_external OR balance_cents >= 0),
  UNIQUE (client_id, pot)
);

CREATE TABLE plan_rules (
  client_id    TEXT NOT NULL REFERENCES clients(id),
  pot          TEXT NOT NULL,
  amount_cents BIGINT NOT NULL CHECK (amount_cents > 0),
  priority     INT NOT NULL,
  PRIMARY KEY (client_id, pot)
);

CREATE TABLE ledger_txns (
  id          UUID PRIMARY KEY,
  kind        TEXT NOT NULL CHECK (kind IN ('deposit','payment')),
  client_id   TEXT NOT NULL REFERENCES clients(id),
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE ledger_entries (
  id           BIGSERIAL PRIMARY KEY,
  txn_id       UUID NOT NULL REFERENCES ledger_txns(id),
  account_id   TEXT NOT NULL REFERENCES accounts(id),
  amount_cents BIGINT NOT NULL CHECK (amount_cents <> 0)  -- +credit / -debit
);
-- Invariant (tested, not a constraint): SUM(amount_cents) per txn_id = 0.

CREATE TABLE deposits (
  id           UUID PRIMARY KEY,
  client_id    TEXT NOT NULL REFERENCES clients(id),
  amount_cents BIGINT NOT NULL CHECK (amount_cents > 0),
  source       TEXT NOT NULL,
  txn_id       UUID NOT NULL REFERENCES ledger_txns(id),
  created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE bills (
  id           UUID PRIMARY KEY,
  client_id    TEXT NOT NULL REFERENCES clients(id),
  pot          TEXT NOT NULL,
  payee        TEXT NOT NULL,
  amount_cents BIGINT NOT NULL CHECK (amount_cents > 0),
  due_date     DATE NOT NULL,
  status       TEXT NOT NULL DEFAULT 'scheduled' CHECK (status IN ('scheduled','paid')),
  FOREIGN KEY (client_id, pot) REFERENCES accounts(client_id, pot)
);
CREATE INDEX bills_due_idx ON bills (client_id, status, due_date);

CREATE TABLE payments (
  id           UUID PRIMARY KEY,
  bill_id      UUID NOT NULL UNIQUE REFERENCES bills(id),  -- backstop: a bill is paid once
  txn_id       UUID NOT NULL REFERENCES ledger_txns(id),
  amount_cents BIGINT NOT NULL,
  created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE idempotency_keys (
  scope           TEXT NOT NULL,             -- 'deposit:<client_id>' or 'payment:<bill_id>'
  key             TEXT NOT NULL,
  request_hash    TEXT NOT NULL,             -- sha256 of the canonical request body
  response_status INT  NOT NULL,
  response_body   JSONB NOT NULL,
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (scope, key)
);

CREATE TABLE outbox (
  id           BIGSERIAL PRIMARY KEY,
  event_id     UUID NOT NULL UNIQUE,
  topic        TEXT NOT NULL,
  msg_key      TEXT NOT NULL,                -- client_id: keeps per-client ordering
  payload      JSONB NOT NULL,
  created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
  published_at TIMESTAMPTZ
);
CREATE INDEX outbox_unpublished_idx ON outbox (id) WHERE published_at IS NULL;
