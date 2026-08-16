package tokens

import (
	"sync"
)

type QuotaStore interface {
	Take(account string, limit int, window string) (int, bool, error)
	Close() error
}

type MemoryQuotaStore struct {
	mu   sync.Mutex
	used map[string]int
}

func NewMemoryQuotaStore() *MemoryQuotaStore {
	return &MemoryQuotaStore{used: make(map[string]int)}
}

func (s *MemoryQuotaStore) Take(account string, limit int, window string) (int, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := account + "\x00" + window
	if s.used[key] >= limit {
		return 0, false, nil
	}
	s.used[key]++
	return limit - s.used[key], true, nil
}

func (s *MemoryQuotaStore) Close() error {
	return nil
}
