package migrate

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestMigrationFilesSorted(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"002_second.sql": "select 2;",
		"001_first.sql":  "select 1;",
		"ignore.txt":     "nope",
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	got, err := migrationFiles(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"001_first.sql", "002_second.sql"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("files = %v, want %v", got, want)
	}
}
