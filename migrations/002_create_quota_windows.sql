CREATE TABLE IF NOT EXISTS quota_windows (
  account TEXT NOT NULL,
  quota_window TEXT NOT NULL,
  used_count INTEGER NOT NULL,
  quota_limit INTEGER NOT NULL,
  updated_at BIGINT NOT NULL,
  PRIMARY KEY (account, quota_window)
);

CREATE INDEX IF NOT EXISTS quota_windows_updated_at_idx
  ON quota_windows(updated_at);
