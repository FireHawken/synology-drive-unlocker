package db

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

// This file holds fixture builders used by tests across packages.
// They are kept in a non-_test.go file so internal/tui tests can import
// them without duplicating the schema. None of these symbols are intended
// for production use; the names are prefixed with "Fixture" to make that
// clear.

// fixtureSysSchema mirrors the session_table and system_table schemas
// observed in real Synology Drive Client installations. It is verbatim
// (down to the column order, types, defaults, and the typo on
// "DATATIME") so behaviour against synthetic fixtures matches behaviour
// against the real thing.
const fixtureSysSchema = `
CREATE TABLE session_table (
	id INTEGER primary key autoincrement,
	conn_id INTEGER DEFAULT 0,
	share_name TEXT COLLATE NOCASE DEFAULT '',
	remote_path TEXT DEFAULT '',
	ctime DATATIME DEFAULT (strftime('%s','now')),
	view_id INTEGER DEFAULT 0,
	node_id INTEGER DEFAULT 0,
	status INTEGER DEFAULT 0,
	error INTEGER DEFAULT 0,
	share_version INTEGER DEFAULT 0,
	sync_folder TEXT DEFAULT '',
	perm_mode INTEGER DEFAULT 0,
	is_read_only INTEGER DEFAULT 0,
	is_daemon_enable INTEGER DEFAULT 0,
	sync_direction INTEGER DEFAULT 0,
	ignore_local_remove INTEGER DEFAULT 0,
	conflict_policy TEXT DEFAULT 'compare_mtime',
	rename_conflict INTEGER DEFAULT 1,
	is_encryption INTEGER DEFAULT 0,
	is_mounted INTEGER DEFAULT 1,
	attribute_check_strength INTEGER DEFAULT 0,
	sync_temp_file INTEGER DEFAULT 0,
	use_windows_cloud_file_api INTEGER DEFAULT 0,
	is_shared_with_me INTEGER DEFAULT 0,
	session_type INTEGER DEFAULT 0,
	with_c2share INTEGER DEFAULT 0,
	c2_share_id TEXT DEFAULT '',
	c2_hash_key TEXT DEFAULT '',
	is_file_filter_enable INTEGER DEFAULT 0,
	is_selective_sync_enable INTEGER DEFAULT 0,
	is_index_home INTEGER DEFAULT 0,
	is_mac_on_demand_sync_enable INTEGER DEFAULT 0,
	custom_session_name TEXT DEFAULT '',
	file_id TEXT DEFAULT '',
	symbolic_link_path TEXT DEFAULT '',
	enable_node_locking INTEGER DEFAULT 0,
	enable_auto_node_locking INTEGER DEFAULT 0,
	with_tiering INTEGER DEFAULT 0
);

CREATE TABLE system_table (
	key VARCHAR PRIMARY KEY ON CONFLICT IGNORE,
	value VARCHAR NOT NULL
);
`

// FixtureSession is the subset of session_table columns the tests care about.
// All other columns are written with their schema defaults.
type FixtureSession struct {
	ID          int64
	ConnID      int64
	ShareName   string
	RemotePath  string
	SyncFolder  string
	SessionType int64
	Status      int64
}

// DefaultFixtureSessions is the canonical seed used across tests:
//   - id=2 is the "default" sync, whose sync_folder matches system_table.open_folder
//   - id=3 is a backup-type task (session_type=2) that must be filtered out
//   - id=5 is a regular sync session
//   - id=6 is the empty placeholder we use as the redirect target
//
// All paths are synthetic; no production usernames or device identifiers leak in.
var DefaultFixtureSessions = []FixtureSession{
	{ID: 2, ConnID: 2, ShareName: "home", RemotePath: "/Drive/", SyncFolder: `D:\SynologyDrive\`, SessionType: 1, Status: 1},
	{ID: 3, ConnID: 3, ShareName: "home", RemotePath: "/Backup/HOST/C/Users/demo/.ssh/", SyncFolder: `C:\Users\demo\.ssh\`, SessionType: 2, Status: 1},
	{ID: 5, ConnID: 2, ShareName: "home", RemotePath: "/cmdtest/", SyncFolder: `C:\Users\demo\.ssh\`, SessionType: 1, Status: 1},
	{ID: 6, ConnID: 2, ShareName: "home", RemotePath: "/temporary_test_synology_side/", SyncFolder: `C:\temporary_test\`, SessionType: 1, Status: 1},
}

// MakeSysFixture creates a fresh sys.sqlite at path, populated with the
// session_table schema, the given sessions, and an open_folder entry
// pointing at session id=2's sync_folder (matching real-world layout).
//
// If path already exists, it is truncated.
func MakeSysFixture(path string, sessions []FixtureSession) error {
	conn, err := sql.Open("sqlite", path)
	if err != nil {
		return fmt.Errorf("open: %w", err)
	}
	defer conn.Close()

	if _, err := conn.Exec(fixtureSysSchema); err != nil {
		return fmt.Errorf("create schema: %w", err)
	}

	const insSession = `INSERT INTO session_table
		(id, conn_id, share_name, remote_path, sync_folder, session_type, status)
		VALUES (?, ?, ?, ?, ?, ?, ?)`
	for _, s := range sessions {
		if _, err := conn.Exec(insSession,
			s.ID, s.ConnID, s.ShareName, s.RemotePath,
			s.SyncFolder, s.SessionType, s.Status); err != nil {
			return fmt.Errorf("insert session id=%d: %w", s.ID, err)
		}
	}

	// open_folder: stored without trailing separator, matching the live client.
	openFolder := ""
	for _, s := range sessions {
		if s.ID == 2 {
			openFolder = trimSepFixture(s.SyncFolder)
			break
		}
	}
	if openFolder == "" && len(sessions) > 0 {
		openFolder = trimSepFixture(sessions[0].SyncFolder)
	}
	if _, err := conn.Exec(
		`INSERT INTO system_table (key, value) VALUES ('open_folder', ?)`,
		openFolder); err != nil {
		return fmt.Errorf("insert open_folder: %w", err)
	}
	return nil
}

// MakeEmptyFixture creates a valid but empty SQLite database at path,
// matching the shape of file-status.sqlite / filter.sqlite / history.sqlite
// when no sync activity has populated them yet.
func MakeEmptyFixture(path string) error {
	conn, err := sql.Open("sqlite", path)
	if err != nil {
		return fmt.Errorf("open: %w", err)
	}
	if err := conn.Ping(); err != nil {
		_ = conn.Close()
		return fmt.Errorf("ping: %w", err)
	}
	return conn.Close()
}

func trimSepFixture(p string) string {
	for len(p) > 0 {
		c := p[len(p)-1]
		if c != '\\' && c != '/' {
			break
		}
		p = p[:len(p)-1]
	}
	return p
}
