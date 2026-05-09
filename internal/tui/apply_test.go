package tui

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/FireHawken/synology-drive-unlocker/internal/backup"
	"github.com/FireHawken/synology-drive-unlocker/internal/db"
	"github.com/FireHawken/synology-drive-unlocker/internal/platform"
)

// makeFakeDBDir creates a synthetic Synology Drive Client db directory:
// a populated sys.sqlite plus empty file-status / filter / history dbs.
// No real-world fixtures are touched.
func makeFakeDBDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := db.MakeSysFixture(filepath.Join(dir, "sys.sqlite"), db.DefaultFixtureSessions); err != nil {
		t.Fatalf("make sys.sqlite: %v", err)
	}
	for _, name := range []string{"file-status.sqlite", "filter.sqlite", "history.sqlite"} {
		if err := db.MakeEmptyFixture(filepath.Join(dir, name)); err != nil {
			t.Fatalf("make %s: %v", name, err)
		}
	}
	return dir
}

func TestApplyChangesCmd_EndToEnd(t *testing.T) {
	dbDir := makeFakeDBDir(t)
	info := platform.Info{
		OS:            "windows",
		DriveDataDir:  filepath.Dir(dbDir),
		DBDir:         dbDir,
		ClientProcess: "cloud-drive-ui.exe",
	}

	sysDB, err := db.OpenSys(filepath.Join(dbDir, "sys.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer sysDB.Close()

	sessions, err := sysDB.SyncSessions(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var target db.Session
	for _, s := range sessions {
		if s.ID == 6 {
			target = s
		}
	}
	if target.ID == 0 {
		t.Fatal("session id=6 missing from fixture")
	}

	newPath := `C:\Users\demo\.config\`
	cmd := applyChangesCmd(context.Background(), info, sysDB, target, newPath, "test")
	msg, ok := cmd().(applyDoneMsg)
	if !ok {
		t.Fatalf("unexpected msg type %T", cmd())
	}
	if msg.err != nil {
		t.Fatalf("apply failed: %v (backup at %s)", msg.err, msg.backupDir)
	}
	if msg.sysResult.SessionRowsAffected != 1 {
		t.Errorf("session rows = %d", msg.sysResult.SessionRowsAffected)
	}
	if msg.sysResult.OpenFolderUpdated {
		t.Error("open_folder unexpectedly updated for session 6")
	}
	// statinfo doesn't exist in the fixture → must be skipped, not an error.
	if msg.statResult.TableExisted {
		t.Error("statinfo unexpectedly existed in fixture")
	}
	if msg.statResult.Skipped == "" {
		t.Error("expected statinfo skip reason")
	}

	// sys.sqlite should now reflect the new path.
	got := readSyncFolder(t, filepath.Join(dbDir, "sys.sqlite"), 6)
	if got != newPath {
		t.Errorf("post-apply sync_folder = %q, want %q", got, newPath)
	}

	// A backup directory must exist with all four files + meta.json.
	if msg.backupDir == "" {
		t.Fatal("empty backup dir")
	}
	for _, name := range backup.FilesToBackup {
		if _, err := os.Stat(filepath.Join(msg.backupDir, name)); err != nil {
			t.Errorf("backup file %s missing: %v", name, err)
		}
	}
	if _, err := os.Stat(filepath.Join(msg.backupDir, "meta.json")); err != nil {
		t.Errorf("meta.json missing: %v", err)
	}

	// And the backup should restore cleanly.
	if err := backup.Restore(msg.backupDir, dbDir); err != nil {
		t.Fatalf("restore: %v", err)
	}
	gotAfterRestore := readSyncFolder(t, filepath.Join(dbDir, "sys.sqlite"), 6)
	if gotAfterRestore != target.SyncFolder {
		t.Errorf("post-restore sync_folder = %q, want %q", gotAfterRestore, target.SyncFolder)
	}
}

func TestApplyChangesCmd_DefaultSessionUpdatesOpenFolder(t *testing.T) {
	dbDir := makeFakeDBDir(t)
	info := platform.Info{
		OS:            "windows",
		DBDir:         dbDir,
		ClientProcess: "cloud-drive-ui.exe",
	}
	sysDB, err := db.OpenSys(filepath.Join(dbDir, "sys.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer sysDB.Close()

	// Session id=2 is the default whose sync_folder matches open_folder.
	target := db.Session{ID: 2, SyncFolder: `D:\SynologyDrive\`}
	cmd := applyChangesCmd(context.Background(), info, sysDB, target, `E:\NewSync\`, "test")
	msg := cmd().(applyDoneMsg)
	if msg.err != nil {
		t.Fatalf("apply: %v", msg.err)
	}
	if !msg.sysResult.OpenFolderUpdated {
		t.Error("open_folder should have been updated")
	}
}

func readSyncFolder(t *testing.T, path string, id int64) string {
	t.Helper()
	conn, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	var v string
	if err := conn.QueryRow(`SELECT sync_folder FROM session_table WHERE id = ?`, id).Scan(&v); err != nil {
		t.Fatal(err)
	}
	return v
}
