package db

import (
	"context"
	"path/filepath"
	"testing"
)

// freshSysFixture builds a sys.sqlite in a per-test tempdir using the canonical
// synthetic seed. Tests never mutate a committed fixture.
func freshSysFixture(t *testing.T) string {
	t.Helper()
	dst := filepath.Join(t.TempDir(), "sys.sqlite")
	if err := MakeSysFixture(dst, DefaultFixtureSessions); err != nil {
		t.Fatalf("make fixture: %v", err)
	}
	return dst
}

func TestSyncSessions_FromExample(t *testing.T) {
	db, err := OpenSys(freshSysFixture(t))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()

	sessions, err := db.SyncSessions(context.Background())
	if err != nil {
		t.Fatalf("SyncSessions: %v", err)
	}
	// Fixture has session_type=1 for ids 2, 5, 6 and session_type=2 for id 3.
	if len(sessions) != 3 {
		t.Fatalf("expected 3 sync sessions, got %d", len(sessions))
	}
	for _, s := range sessions {
		if s.SessionType != 1 {
			t.Errorf("session id=%d has session_type=%d, want 1", s.ID, s.SessionType)
		}
	}
	// Spot-check id=6.
	var target *Session
	for i := range sessions {
		if sessions[i].ID == 6 {
			target = &sessions[i]
		}
	}
	if target == nil {
		t.Fatal("session id=6 missing")
	}
	if target.SyncFolder != `C:\temporary_test\` {
		t.Errorf("sync_folder = %q, want %q", target.SyncFolder, `C:\temporary_test\`)
	}
	if target.RemotePath != `/temporary_test_synology_side/` {
		t.Errorf("remote_path = %q", target.RemotePath)
	}
}

func TestAllSyncFolders_FromExample(t *testing.T) {
	db, err := OpenSys(freshSysFixture(t))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()

	all, err := db.AllSyncFolders(context.Background())
	if err != nil {
		t.Fatalf("AllSyncFolders: %v", err)
	}
	// Fixture has 4 rows total (3 sync + 1 backup).
	if len(all) != 4 {
		t.Errorf("expected 4 entries, got %d (%v)", len(all), all)
	}
}

func TestOpenFolder_FromExample(t *testing.T) {
	db, err := OpenSys(freshSysFixture(t))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()

	v, err := db.OpenFolder(context.Background())
	if err != nil {
		t.Fatalf("OpenFolder: %v", err)
	}
	if v != `D:\SynologyDrive` {
		t.Errorf("open_folder = %q, want %q", v, `D:\SynologyDrive`)
	}
}

func TestUpdateSessionFolder_TargetSession(t *testing.T) {
	db, err := OpenSys(freshSysFixture(t))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	res, err := db.UpdateSessionFolder(ctx, 6, `C:\temporary_test\`, `C:\Users\demo\.config\`)
	if err != nil {
		t.Fatalf("UpdateSessionFolder: %v", err)
	}
	if res.SessionRowsAffected != 1 {
		t.Errorf("session rows affected = %d, want 1", res.SessionRowsAffected)
	}
	if res.OpenFolderUpdated {
		t.Errorf("open_folder should not be updated for session 6")
	}

	sessions, err := db.SyncSessions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var got string
	for _, s := range sessions {
		if s.ID == 6 {
			got = s.SyncFolder
		}
	}
	if got != `C:\Users\demo\.config\` {
		t.Errorf("post-update sync_folder = %q", got)
	}
}

func TestUpdateSessionFolder_DefaultSessionUpdatesOpenFolder(t *testing.T) {
	db, err := OpenSys(freshSysFixture(t))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	// Session id=2 is the "default": its sync_folder D:\SynologyDrive\
	// matches system_table.open_folder D:\SynologyDrive.
	res, err := db.UpdateSessionFolder(ctx, 2, `D:\SynologyDrive\`, `E:\NewSync\`)
	if err != nil {
		t.Fatalf("UpdateSessionFolder: %v", err)
	}
	if !res.OpenFolderUpdated {
		t.Error("expected open_folder to be updated")
	}
	v, err := db.OpenFolder(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if v != `E:\NewSync` {
		t.Errorf("open_folder = %q, want E:\\NewSync (no trailing sep)", v)
	}
}

func TestUpdateSessionFolder_StaleOldPathRejected(t *testing.T) {
	db, err := OpenSys(freshSysFixture(t))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()

	_, err = db.UpdateSessionFolder(context.Background(), 6,
		`C:\WRONG\`, `C:\Users\demo\.config\`)
	if err == nil {
		t.Fatal("expected error for mismatched old path")
	}
}

func TestUpdateSessionFolder_UnknownSessionRejected(t *testing.T) {
	db, err := OpenSys(freshSysFixture(t))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()

	_, err = db.UpdateSessionFolder(context.Background(), 9999,
		`C:\nope\`, `C:\new\`)
	if err == nil {
		t.Fatal("expected error for missing session id")
	}
}
