package tokens

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"

	bolt "go.etcd.io/bbolt"
)

var spentBucket = []byte("spent_tokens")

type ReplayStore interface {
	Spend(tokenHash string, receipt Receipt) (Receipt, bool, error)
	Close() error
}

type MemoryReplayStore struct {
	mu    sync.Mutex
	spent map[string]Receipt
}

func NewMemoryReplayStore() *MemoryReplayStore {
	return &MemoryReplayStore{spent: make(map[string]Receipt)}
}

func (s *MemoryReplayStore) Spend(tokenHash string, receipt Receipt) (Receipt, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if old, ok := s.spent[tokenHash]; ok {
		return old, false, nil
	}
	s.spent[tokenHash] = receipt
	return receipt, true, nil
}

func (s *MemoryReplayStore) Close() error {
	return nil
}

type BoltReplayStore struct {
	db *bolt.DB
}

func OpenBoltReplayStore(path string) (*BoltReplayStore, error) {
	if path == "" {
		return nil, errors.New("missing replay db path")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}

	db, err := bolt.Open(path, 0600, &bolt.Options{Timeout: 2 * time.Second})
	if err != nil {
		return nil, err
	}

	store := &BoltReplayStore{db: db}
	if err := store.init(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (s *BoltReplayStore) Spend(tokenHash string, receipt Receipt) (Receipt, bool, error) {
	var out Receipt
	var inserted bool

	err := s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(spentBucket)
		key := []byte(tokenHash)

		if old := b.Get(key); old != nil {
			inserted = false
			return json.Unmarshal(old, &out)
		}

		data, err := json.Marshal(receipt)
		if err != nil {
			return err
		}
		if err := b.Put(key, data); err != nil {
			return err
		}
		out = receipt
		inserted = true
		return nil
	})
	return out, inserted, err
}

func (s *BoltReplayStore) Close() error {
	return s.db.Close()
}

func (s *BoltReplayStore) init() error {
	return s.db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists(spentBucket)
		return err
	})
}
