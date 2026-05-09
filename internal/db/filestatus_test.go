package db

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

// freshFileStatusFixture creates an empty file-status.sqlite per test, matching
// the shape of a freshly-installed Synology Drive Client (no statinfo table yet).
func freshFileStatusFixture(t *testing.T) string {
	t.Helper()
	dst := filepath.Join(t.TempDir(), "file-status.sqlite")
	if err := MakeEmptyFixture(dst); err != nil {
		t.Fatalf("make fixture: %v", err)
	}
	return dst
}

func TestUpdatePaths_TableMissing_Skips(t *testing.T) {
	db, err := OpenFileStatus(freshFileStatusFixture(t))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()

	res, err := db.UpdatePaths(context.Background(), `C:\temporary_test\`, `C:\Users\demo\.config\`)
	if err != nil {
		t.Fatalf("UpdatePaths: %v", err)
	}
	if res.TableExisted {
		t.Error("statinfo should not exist in fixture")
	}
	if res.RowsAffected != 0 {
		t.Errorf("rows affected = %d, want 0", res.RowsAffected)
	}
	if res.Skipped == "" {
		t.Error("expected non-empty Skipped reason")
	}
}

// makeFakeStatinfo fabricates a statinfo table with the path column,
// so we can verify the update logic without depending on the real schema.
func makeFakeStatinfo(t *testing.T, path string, rows []string) {
	t.Helper()
	conn, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if _, err := conn.Exec(`CREATE TABLE statinfo (id INTEGER PRIMARY KEY, path TEXT NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	for _, p := range rows {
		if _, err := conn.Exec(`INSERT INTO statinfo (path) VALUES (?)`, p); err != nil {
			t.Fatal(err)
		}
	}
}

func TestUpdatePaths_RewritesExactAndNested(t *testing.T) {
	dst := freshFileStatusFixture(t)
	makeFakeStatinfo(t, dst, []string{
		`C:\temporary_test\`,
		`C:\temporary_test\file1.txt`,
		`C:\temporary_test\sub\file2.txt`,
		`C:\OTHER\should-not-touch.txt`,
		`C:\temporary_test_NEIGHBOR\file3.txt`, // must NOT match (underscore-escape)
	})

	db, err := OpenFileStatus(dst)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	res, err := db.UpdatePaths(context.Background(), `C:\temporary_test\`, `C:\Users\demo\.config\`)
	if err != nil {
		t.Fatalf("UpdatePaths: %v", err)
	}
	if !res.TableExisted || !res.HasPathColumn {
		t.Errorf("expected statinfo with path column, got %+v", res)
	}
	if res.RowsAffected != 3 {
		t.Errorf("rows affected = %d, want 3", res.RowsAffected)
	}

	// Verify post-state.
	conn, err := sql.Open("sqlite", dst)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	rows, err := conn.Query(`SELECT path FROM statinfo ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var got []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			t.Fatal(err)
		}
		got = append(got, p)
	}
	want := []string{
		`C:\Users\demo\.config\`,
		`C:\Users\demo\.config\file1.txt`,
		`C:\Users\demo\.config\sub\file2.txt`,
		`C:\OTHER\should-not-touch.txt`,
		`C:\temporary_test_NEIGHBOR\file3.txt`,
	}
	if len(got) != len(want) {
		t.Fatalf("rows: got %v, want %v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("row %d: got %q, want %q", i, got[i], want[i])
		}
	}
}

func TestUpdatePaths_UnexpectedSchema_Skips(t *testing.T) {
	dst := freshFileStatusFixture(t)
	conn, err := sql.Open("sqlite", dst)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(`CREATE TABLE statinfo (id INTEGER PRIMARY KEY, weird TEXT)`); err != nil {
		t.Fatal(err)
	}
	conn.Close()

	db, err := OpenFileStatus(dst)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	res, err := db.UpdatePaths(context.Background(), `C:\foo\`, `C:\bar\`)
	if err != nil {
		t.Fatalf("UpdatePaths: %v", err)
	}
	if !res.TableExisted {
		t.Error("expected TableExisted=true")
	}
	if res.HasPathColumn {
		t.Error("expected HasPathColumn=false")
	}
	if res.Skipped == "" {
		t.Error("expected Skipped reason")
	}
}
