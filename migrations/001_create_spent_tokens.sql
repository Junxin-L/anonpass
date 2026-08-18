CREATE TABLE IF NOT EXISTS spent_tokens (
  token_hash TEXT PRIMARY KEY,
  receipt_json JSONB NOT NULL,
  redeemed_at BIGINT NOT NULL
);

CREATE INDEX IF NOT EXISTS spent_tokens_redeemed_at_idx
  ON spent_tokens(redeemed_at);
