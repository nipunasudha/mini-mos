-- +goose Up

-- The two counter-parties of every money movement. Their entries are written to
-- ledger_entries, but their balance_cents is deliberately never updated: that
-- would make one hot row that every client's transaction has to lock. Their
-- balance is derivable with SUM(ledger_entries.amount_cents) when needed.
INSERT INTO accounts (id, is_external) VALUES
  ('external:income', true),
  ('external:payees', true);
