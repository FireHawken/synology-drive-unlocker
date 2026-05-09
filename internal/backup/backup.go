// Package backup creates timestamped snapshots of Synology Drive's SQLite
// databases and restores them on demand.
package backup

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// FilesToBackup lists every database we copy. The unlock workflow only writes
// to the first two, but we snapshot all four for safety so restore is total.
var FilesToBackup = []string{
	"sys.sqlite",
	"file-status.sqlite",
	"filter.sqlite",
	"history.sqlite",
}

// Meta describes a single backup directory. It is persisted as meta.json.
type Meta struct {
	CreatedAt   time.Time `json:"created_at"`
	ToolVersion string    `json:"tool_version"`
	SessionID   int64     `json:"session_id"`
	OldPath     string    `json:"old_path"`
	NewPath     string    `json:"new_path"`
	// FilesCopied is the list of database files actually present at backup time.
	FilesCopied []string `json:"files_copied"`
}

// dirTimestampFormat is the directory name pattern: backup-YYYY-MM-DDTHH-MM-SS
// (colons are not portable on Windows, so we use dashes).
const dirTimestampFormat = "2006-01-02T15-04-05"

// Create snapshots dbDir into a sibling directory and writes meta.json.
// Returns the absolute path of the created backup directory.
func Create(dbDir string, m Meta) (string, error) {
	if m.CreatedAt.IsZero() {
		m.CreatedAt = time.Now()
	}
	parent := backupsRoot(dbDir)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return "", fmt.Errorf("create backups root: %w", err)
	}
	dir := filepath.Join(parent, "backup-"+m.CreatedAt.Format(dirTimestampFormat))
	if err := os.Mkdir(dir, 0o755); err != nil {
		return "", fmt.Errorf("create backup dir: %w", err)
	}

	var copied []string
	for _, name := range FilesToBackup {
		src := filepath.Join(dbDir, name)
		if _, err := os.Stat(src); errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err := copyFile(src, filepath.Join(dir, name)); err != nil {
			// Roll back partially-written backup so callers don't see a corrupt one.
			_ = os.RemoveAll(dir)
			return "", fmt.Errorf("copy %s: %w", name, err)
		}
		copied = append(copied, name)
	}
	m.FilesCopied = copied

	if err := writeMeta(dir, m); err != nil {
		_ = os.RemoveAll(dir)
		return "", err
	}
	return dir, nil
}

// List returns every backup found under dbDir/.unlocker-backups, newest first.
func List(dbDir string) ([]Entry, error) {
	root := backupsRoot(dbDir)
	entries, err := os.ReadDir(root)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read backups root: %w", err)
	}
	var out []Entry
	for _, e := range entries {
		if !e.IsDir() || !strings.HasPrefix(e.Name(), "backup-") {
			continue
		}
		dir := filepath.Join(root, e.Name())
		m, err := readMeta(dir)
		if err != nil {
			// Treat unreadable meta.json as a corrupt backup; surface it but keep going.
			out = append(out, Entry{Dir: dir, MetaError: err})
			continue
		}
		out = append(out, Entry{Dir: dir, Meta: m})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Meta.CreatedAt.After(out[j].Meta.CreatedAt)
	})
	return out, nil
}

// Entry pairs a backup directory with its parsed meta.json.
type Entry struct {
	Dir       string
	Meta      Meta
	MetaError error
}

// Restore copies every .sqlite file from the backup dir back into dbDir,
// overwriting whatever is there. The caller MUST ensure Synology Drive is
// not running before invoking this, the same as for Create.
func Restore(backupDir, dbDir string) error {
	m, err := readMeta(backupDir)
	if err != nil {
		return fmt.Errorf("read meta: %w", err)
	}
	for _, name := range m.FilesCopied {
		src := filepath.Join(backupDir, name)
		if _, err := os.Stat(src); err != nil {
			return fmt.Errorf("missing backup file %s: %w", name, err)
		}
	}
	for _, name := range m.FilesCopied {
		src := filepath.Join(backupDir, name)
		dst := filepath.Join(dbDir, name)
		if err := copyFile(src, dst); err != nil {
			return fmt.Errorf("restore %s: %w", name, err)
		}
	}
	return nil
}

func backupsRoot(dbDir string) string {
	return filepath.Join(dbDir, ".unlocker-backups")
}

func writeMeta(dir string, m Meta) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal meta: %w", err)
	}
	return os.WriteFile(filepath.Join(dir, "meta.json"), data, 0o644)
}

func readMeta(dir string) (Meta, error) {
	var m Meta
	data, err := os.ReadFile(filepath.Join(dir, "meta.json"))
	if err != nil {
		return m, err
	}
	if err := json.Unmarshal(data, &m); err != nil {
		return m, err
	}
	return m, nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}
