package backup

import (
	"crypto/sha256"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fakeDBDir creates a temp dir that mimics SynologyDrive's data\db, populated
// with byte-distinguishable copies of each known database file.
func fakeDBDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for i, name := range FilesToBackup {
		body := []byte("fixture " + name + "\n" + strings.Repeat("x", i+1))
		if err := os.WriteFile(filepath.Join(dir, name), body, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func sha(t *testing.T, p string) [32]byte {
	t.Helper()
	f, err := os.Open(p)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		t.Fatal(err)
	}
	var out [32]byte
	copy(out[:], h.Sum(nil))
	return out
}

func TestCreate_CopiesAllPresentFilesAndWritesMeta(t *testing.T) {
	dbDir := fakeDBDir(t)
	m := Meta{
		ToolVersion: "test",
		SessionID:   6,
		OldPath:     `C:\temporary_test\`,
		NewPath:     `C:\Users\demo\.config\`,
		CreatedAt:   time.Date(2026, 5, 9, 19, 23, 1, 0, time.UTC),
	}
	dir, err := Create(dbDir, m)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if !strings.Contains(dir, "backup-2026-05-09T19-23-01") {
		t.Errorf("backup dir name = %q", dir)
	}
	for _, name := range FilesToBackup {
		got := sha(t, filepath.Join(dir, name))
		want := sha(t, filepath.Join(dbDir, name))
		if got != want {
			t.Errorf("%s: backup hash differs from source", name)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "meta.json")); err != nil {
		t.Errorf("meta.json missing: %v", err)
	}
}

func TestCreate_SkipsMissingFiles(t *testing.T) {
	dbDir := fakeDBDir(t)
	if err := os.Remove(filepath.Join(dbDir, "filter.sqlite")); err != nil {
		t.Fatal(err)
	}
	dir, err := Create(dbDir, Meta{ToolVersion: "test"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "filter.sqlite")); !os.IsNotExist(err) {
		t.Errorf("expected filter.sqlite to be absent in backup, got err=%v", err)
	}
	entries, err := List(dbDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("len(entries) = %d", len(entries))
	}
	got := entries[0].Meta.FilesCopied
	for _, name := range got {
		if name == "filter.sqlite" {
			t.Error("FilesCopied should not include missing filter.sqlite")
		}
	}
}

func TestList_OrderedNewestFirst(t *testing.T) {
	dbDir := fakeDBDir(t)
	t1 := time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 5, 5, 10, 0, 0, 0, time.UTC)
	t3 := time.Date(2026, 5, 3, 10, 0, 0, 0, time.UTC)
	for _, ts := range []time.Time{t1, t2, t3} {
		if _, err := Create(dbDir, Meta{CreatedAt: ts, ToolVersion: "t"}); err != nil {
			t.Fatal(err)
		}
	}
	entries, err := List(dbDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 3 {
		t.Fatalf("len(entries) = %d", len(entries))
	}
	got := []time.Time{entries[0].Meta.CreatedAt, entries[1].Meta.CreatedAt, entries[2].Meta.CreatedAt}
	want := []time.Time{t2, t3, t1}
	for i := range got {
		if !got[i].Equal(want[i]) {
			t.Errorf("entry %d: got %v, want %v", i, got[i], want[i])
		}
	}
}

func TestList_NoBackupsRoot(t *testing.T) {
	entries, err := List(t.TempDir())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if entries != nil {
		t.Errorf("expected nil, got %v", entries)
	}
}

func TestRestore_RoundTrips(t *testing.T) {
	dbDir := fakeDBDir(t)
	originalSums := map[string][32]byte{}
	for _, name := range FilesToBackup {
		originalSums[name] = sha(t, filepath.Join(dbDir, name))
	}

	dir, err := Create(dbDir, Meta{ToolVersion: "t"})
	if err != nil {
		t.Fatal(err)
	}

	// Mutate every db file post-backup.
	for _, name := range FilesToBackup {
		if err := os.WriteFile(filepath.Join(dbDir, name), []byte("MUTATED"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	if err := Restore(dir, dbDir); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	for _, name := range FilesToBackup {
		if got := sha(t, filepath.Join(dbDir, name)); got != originalSums[name] {
			t.Errorf("%s not restored", name)
		}
	}
}
