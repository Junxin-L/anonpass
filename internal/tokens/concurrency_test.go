package tokens

import (
	"sync"
	"testing"
)

func TestMemoryReplayStoreConcurrentSpend(t *testing.T) {
	store := NewMemoryReplayStore()
	const attempts = 64

	var wg sync.WaitGroup
	inserted := make(chan bool, attempts)
	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, ok, err := store.Spend("same-token", Receipt{TokenHash: "same-token", RedeemedAt: 1})
			if err != nil {
				t.Error(err)
				return
			}
			inserted <- ok
		}()
	}
	wg.Wait()
	close(inserted)

	okCount := 0
	for ok := range inserted {
		if ok {
			okCount++
		}
	}
	if okCount != 1 {
		t.Fatalf("inserted %d times, want 1", okCount)
	}
}
