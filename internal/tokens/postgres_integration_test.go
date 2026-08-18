package tokens

import (
	"fmt"
	"os"
	"sync"
	"testing"
	"time"
)

func TestPostgresStoresConcurrentBehavior(t *testing.T) {
	dsn := os.Getenv("ANONPASS_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("set ANONPASS_TEST_POSTGRES_DSN to run PostgreSQL integration tests")
	}

	replay, err := OpenPostgresReplayStore(dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer replay.Close()

	quota, err := OpenPostgresQuotaStore(dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer quota.Close()

	suffix := time.Now().UnixNano()
	tokenHash := fmt.Sprintf("token-%d", suffix)

	const replayAttempts = 32
	var replayWG sync.WaitGroup
	replayInserted := make(chan bool, replayAttempts)
	for i := 0; i < replayAttempts; i++ {
		replayWG.Add(1)
		go func() {
			defer replayWG.Done()
			_, ok, err := replay.Spend(tokenHash, Receipt{TokenHash: tokenHash, RedeemedAt: time.Now().Unix()})
			if err != nil {
				t.Error(err)
				return
			}
			replayInserted <- ok
		}()
	}
	replayWG.Wait()
	close(replayInserted)

	replayOK := 0
	for ok := range replayInserted {
		if ok {
			replayOK++
		}
	}
	if replayOK != 1 {
		t.Fatalf("postgres replay inserted %d times, want 1", replayOK)
	}

	account := fmt.Sprintf("account-%d@example.com", suffix)
	window := fmt.Sprintf("window-%d", suffix)
	const limit = 7
	const quotaAttempts = 40
	var quotaWG sync.WaitGroup
	quotaAllowed := make(chan bool, quotaAttempts)
	for i := 0; i < quotaAttempts; i++ {
		quotaWG.Add(1)
		go func() {
			defer quotaWG.Done()
			_, ok, err := quota.Take(account, limit, window)
			if err != nil {
				t.Error(err)
				return
			}
			quotaAllowed <- ok
		}()
	}
	quotaWG.Wait()
	close(quotaAllowed)

	allowed := 0
	for ok := range quotaAllowed {
		if ok {
			allowed++
		}
	}
	if allowed != limit {
		t.Fatalf("postgres quota allowed %d, want %d", allowed, limit)
	}
}
