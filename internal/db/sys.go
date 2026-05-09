// Package db provides read/write access to Synology Drive Client's SQLite databases.
package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	_ "modernc.org/sqlite"
)

// Session represents a row from sys.sqlite session_table that the user can edit.
// We expose only the fields relevant to the unlock workflow.
type Session struct {
	ID          int64
	ConnID      int64
	ShareName   string
	RemotePath  string
	SyncFolder  string
	SessionType int64
	Status      int64
}

const openFolderKey = "open_folder"

// SysDB is a handle to sys.sqlite.
type SysDB struct {
	conn *sql.DB
	path string
}

// OpenSys opens the given sys.sqlite file. The caller must Close() it.
func OpenSys(path string) (*SysDB, error) {
	conn, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sys.sqlite: %w", err)
	}
	if err := conn.Ping(); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("ping sys.sqlite: %w", err)
	}
	return &SysDB{conn: conn, path: path}, nil
}

func (s *SysDB) Close() error { return s.conn.Close() }

// SyncSessions returns all sync-type sessions (session_type = 1), ordered by id.
func (s *SysDB) SyncSessions(ctx context.Context) ([]Session, error) {
	const q = `SELECT id, conn_id, share_name, remote_path, sync_folder, session_type, status
	           FROM session_table WHERE session_type = 1 ORDER BY id`
	rows, err := s.conn.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("query sessions: %w", err)
	}
	defer rows.Close()

	var out []Session
	for rows.Next() {
		var ss Session
		if err := rows.Scan(&ss.ID, &ss.ConnID, &ss.ShareName, &ss.RemotePath,
			&ss.SyncFolder, &ss.SessionType, &ss.Status); err != nil {
			return nil, fmt.Errorf("scan session row: %w", err)
		}
		out = append(out, ss)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate session rows: %w", err)
	}
	return out, nil
}

// AllSyncFolders returns sync_folder values for every row in session_table
// regardless of session_type. Used for collision checks against new paths.
func (s *SysDB) AllSyncFolders(ctx context.Context) ([]string, error) {
	rows, err := s.conn.QueryContext(ctx, `SELECT sync_folder FROM session_table`)
	if err != nil {
		return nil, fmt.Errorf("query sync_folders: %w", err)
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, fmt.Errorf("scan sync_folder: %w", err)
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// OpenFolder returns the system_table.open_folder value, or empty string if absent.
func (s *SysDB) OpenFolder(ctx context.Context) (string, error) {
	var v string
	err := s.conn.QueryRowContext(ctx,
		`SELECT value FROM system_table WHERE key = ?`, openFolderKey).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("query open_folder: %w", err)
	}
	return v, nil
}

// UpdateResult describes what UpdateSessionFolder actually changed.
type UpdateResult struct {
	SessionRowsAffected    int64
	OpenFolderUpdated      bool
	OpenFolderRowsAffected int64
}

// UpdateSessionFolder atomically:
//  1. updates session_table.sync_folder for the given session id;
//  2. updates system_table.open_folder iff its current value equals the old path;
//  3. runs PRAGMA wal_checkpoint(TRUNCATE) so no -wal/-shm files linger.
//
// It verifies the existing sync_folder matches oldPath before writing, to catch
// stale state (someone modified the DB between read and write).
func (s *SysDB) UpdateSessionFolder(ctx context.Context, sessionID int64, oldPath, newPath string) (UpdateResult, error) {
	var res UpdateResult

	tx, err := s.conn.BeginTx(ctx, nil)
	if err != nil {
		return res, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var current string
	if err := tx.QueryRowContext(ctx,
		`SELECT sync_folder FROM session_table WHERE id = ?`, sessionID).Scan(&current); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return res, fmt.Errorf("session id=%d not found", sessionID)
		}
		return res, fmt.Errorf("read current sync_folder: %w", err)
	}
	if current != oldPath {
		return res, fmt.Errorf("sync_folder for session id=%d changed since read: db has %q, expected %q",
			sessionID, current, oldPath)
	}

	r, err := tx.ExecContext(ctx,
		`UPDATE session_table SET sync_folder = ? WHERE id = ?`, newPath, sessionID)
	if err != nil {
		return res, fmt.Errorf("update session_table: %w", err)
	}
	res.SessionRowsAffected, _ = r.RowsAffected()

	var openFolder string
	err = tx.QueryRowContext(ctx,
		`SELECT value FROM system_table WHERE key = ?`, openFolderKey).Scan(&openFolder)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return res, fmt.Errorf("read open_folder: %w", err)
	}

	// open_folder is stored without a trailing separator (e.g. "D:\SynologyDrive"),
	// while sync_folder has one ("D:\SynologyDrive\"). Compare with both forms stripped.
	if openFolder != "" && trimSep(openFolder) == trimSep(oldPath) {
		newOpen := trimSep(newPath)
		r2, err := tx.ExecContext(ctx,
			`UPDATE system_table SET value = ? WHERE key = ?`, newOpen, openFolderKey)
		if err != nil {
			return res, fmt.Errorf("update open_folder: %w", err)
		}
		res.OpenFolderUpdated = true
		res.OpenFolderRowsAffected, _ = r2.RowsAffected()
	}

	if err := tx.Commit(); err != nil {
		return res, fmt.Errorf("commit: %w", err)
	}

	if _, err := s.conn.ExecContext(ctx, `PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
		return res, fmt.Errorf("wal_checkpoint: %w", err)
	}
	return res, nil
}

func trimSep(p string) string {
	for len(p) > 0 {
		last := p[len(p)-1]
		if last != '\\' && last != '/' {
			break
		}
		p = p[:len(p)-1]
	}
	return p
}
