package tokens

import (
	"database/sql"
	"encoding/json"
	"errors"

	_ "github.com/jackc/pgx/v5/stdlib"
)

type PostgresReplayStore struct {
	db *sql.DB
}

func OpenPostgresReplayStore(dsn string) (*PostgresReplayStore, error) {
	if dsn == "" {
		return nil, errors.New("missing postgres dsn")
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, err
	}
	store := &PostgresReplayStore{db: db}
	if err := store.init(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (s *PostgresReplayStore) Spend(tokenHash string, receipt Receipt) (Receipt, bool, error) {
	data, err := json.Marshal(receipt)
	if err != nil {
		return Receipt{}, false, err
	}

	res, err := s.db.Exec(`
		INSERT INTO spent_tokens(token_hash, receipt_json, redeemed_at)
		VALUES ($1, $2, $3)
		ON CONFLICT (token_hash) DO NOTHING
	`, tokenHash, data, receipt.RedeemedAt)
	if err != nil {
		return Receipt{}, false, err
	}

	rows, err := res.RowsAffected()
	if err != nil {
		return Receipt{}, false, err
	}
	if rows == 1 {
		return receipt, true, nil
	}

	var oldData []byte
	err = s.db.QueryRow(`
		SELECT receipt_json
		FROM spent_tokens
		WHERE token_hash = $1
	`, tokenHash).Scan(&oldData)
	if err != nil {
		return Receipt{}, false, err
	}

	var old Receipt
	if err := json.Unmarshal(oldData, &old); err != nil {
		return Receipt{}, false, err
	}
	return old, false, nil
}

func (s *PostgresReplayStore) Close() error {
	return s.db.Close()
}

func (s *PostgresReplayStore) init() error {
	if err := s.db.Ping(); err != nil {
		return err
	}
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS spent_tokens (
			token_hash TEXT PRIMARY KEY,
			receipt_json JSONB NOT NULL,
			redeemed_at BIGINT NOT NULL
		)
	`)
	return err
}
