package keyfile

import (
	"path/filepath"
	"testing"
)

func TestLoadOrCreateKeepsSameKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "issuer.pem")

	first, err := LoadOrCreate(path, 1024)
	if err != nil {
		t.Fatal(err)
	}
	second, err := LoadOrCreate(path, 1024)
	if err != nil {
		t.Fatal(err)
	}

	if first.N.Cmp(second.N) != 0 {
		t.Fatal("key changed after reload")
	}
}
