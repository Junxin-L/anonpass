package tokens

import (
	"database/sql"
	"errors"
	"time"
)

type PostgresQuotaStore struct {
	db *sql.DB
}

func OpenPostgresQuotaStore(dsn string) (*PostgresQuotaStore, error) {
	if dsn == "" {
		return nil, errors.New("missing postgres dsn")
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, err
	}
	store := &PostgresQuotaStore{db: db}
	if err := store.init(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (s *PostgresQuotaStore) Take(account string, limit int, window string) (int, bool, error) {
	var remaining int
	err := s.db.QueryRow(`
		INSERT INTO quota_windows(account, window, used_count, quota_limit, updated_at)
		VALUES ($1, $2, 1, $3, $4)
		ON CONFLICT (account, window) DO UPDATE
		SET used_count = quota_windows.used_count + 1,
			quota_limit = EXCLUDED.quota_limit,
			updated_at = EXCLUDED.updated_at
		WHERE quota_windows.used_count < EXCLUDED.quota_limit
		RETURNING quota_limit - used_count
	`, account, window, limit, time.Now().Unix()).Scan(&remaining)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return remaining, true, nil
}

func (s *PostgresQuotaStore) Close() error {
	return s.db.Close()
}

func (s *PostgresQuotaStore) init() error {
	if err := s.db.Ping(); err != nil {
		return err
	}
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS quota_windows (
			account TEXT NOT NULL,
			window TEXT NOT NULL,
			used_count INTEGER NOT NULL,
			quota_limit INTEGER NOT NULL,
			updated_at BIGINT NOT NULL,
			PRIMARY KEY (account, window)
		)
	`)
	return err
}
