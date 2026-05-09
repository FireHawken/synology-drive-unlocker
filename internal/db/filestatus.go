package db

import (
	"context"
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

// StatusUpdateResult describes what happened when patching file-status.sqlite.
type StatusUpdateResult struct {
	// TableExisted is true iff a "statinfo" table was found in the DB.
	TableExisted bool
	// HasPathColumn is true iff statinfo had a recognizable "path" column.
	HasPathColumn bool
	// RowsAffected counts how many rows had path rewritten.
	RowsAffected int64
	// Skipped explains why the file was not modified, if it wasn't.
	Skipped string
}

// FileStatusDB is a handle to file-status.sqlite.
type FileStatusDB struct {
	conn *sql.DB
	path string
}

// OpenFileStatus opens file-status.sqlite. Empty / fresh databases are valid;
// the caller should examine StatusUpdateResult.TableExisted to decide if anything happened.
func OpenFileStatus(path string) (*FileStatusDB, error) {
	conn, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open file-status.sqlite: %w", err)
	}
	if err := conn.Ping(); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("ping file-status.sqlite: %w", err)
	}
	return &FileStatusDB{conn: conn, path: path}, nil
}

func (f *FileStatusDB) Close() error { return f.conn.Close() }

// UpdatePaths rewrites every row in statinfo whose path equals oldPath or is
// nested under it (LIKE 'oldPath%'), substituting the prefix with newPath.
// If the table doesn't exist or lacks a path column, returns Skipped explaining why.
//
// We are defensive here because the example databases observed so far (both
// fresh and from a running client) had no statinfo table at all — the schema
// is created by the client lazily, and may differ across versions.
func (f *FileStatusDB) UpdatePaths(ctx context.Context, oldPath, newPath string) (StatusUpdateResult, error) {
	res := StatusUpdateResult{}

	exists, err := tableExists(ctx, f.conn, "statinfo")
	if err != nil {
		return res, err
	}
	if !exists {
		res.Skipped = "statinfo table not present (will be created by Synology Drive on next launch)"
		return res, nil
	}
	res.TableExisted = true

	hasPath, err := columnExists(ctx, f.conn, "statinfo", "path")
	if err != nil {
		return res, err
	}
	if !hasPath {
		res.Skipped = "statinfo has unexpected schema (no 'path' column); manual review required"
		return res, nil
	}
	res.HasPathColumn = true

	tx, err := f.conn.BeginTx(ctx, nil)
	if err != nil {
		return res, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Match either the folder itself or any descendant path. SQLite LIKE escapes:
	// % and _ are wildcards. Escape underscores in the prefix so paths like
	// "C:\foo_bar\" don't match unintended siblings.
	likePrefix := escapeLike(oldPath) + "%"
	r, err := tx.ExecContext(ctx,
		`UPDATE statinfo
		   SET path = ? || substr(path, ?)
		 WHERE path = ? OR path LIKE ? ESCAPE '\'`,
		newPath, len(oldPath)+1, oldPath, likePrefix)
	if err != nil {
		return res, fmt.Errorf("update statinfo: %w", err)
	}
	res.RowsAffected, _ = r.RowsAffected()

	if err := tx.Commit(); err != nil {
		return res, fmt.Errorf("commit: %w", err)
	}
	if _, err := f.conn.ExecContext(ctx, `PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
		return res, fmt.Errorf("wal_checkpoint: %w", err)
	}
	return res, nil
}

func tableExists(ctx context.Context, db *sql.DB, name string) (bool, error) {
	var n string
	err := db.QueryRowContext(ctx,
		`SELECT name FROM sqlite_master WHERE type='table' AND name=?`, name).Scan(&n)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("check table %q: %w", name, err)
	}
	return true, nil
}

func columnExists(ctx context.Context, db *sql.DB, table, col string) (bool, error) {
	rows, err := db.QueryContext(ctx, `PRAGMA table_info(`+quoteIdent(table)+`)`)
	if err != nil {
		return false, fmt.Errorf("table_info(%q): %w", table, err)
	}
	defer rows.Close()
	for rows.Next() {
		var (
			cid       int
			name      string
			ctype     string
			notnull   int
			dfltValue sql.NullString
			pk        int
		)
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dfltValue, &pk); err != nil {
			return false, fmt.Errorf("scan table_info: %w", err)
		}
		if name == col {
			return true, nil
		}
	}
	return false, rows.Err()
}

// quoteIdent quotes an SQL identifier with double quotes, escaping embedded quotes.
func quoteIdent(s string) string {
	out := make([]byte, 0, len(s)+2)
	out = append(out, '"')
	for i := 0; i < len(s); i++ {
		if s[i] == '"' {
			out = append(out, '"', '"')
		} else {
			out = append(out, s[i])
		}
	}
	out = append(out, '"')
	return string(out)
}

// escapeLike escapes the LIKE-pattern metacharacters %, _, and \ using \ as the escape char.
// The caller must use ESCAPE '\' in the LIKE clause.
func escapeLike(s string) string {
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '%' || c == '_' || c == '\\' {
			out = append(out, '\\')
		}
		out = append(out, c)
	}
	return string(out)
}
