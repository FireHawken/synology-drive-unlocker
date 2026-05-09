// Package paths provides Synology Drive-compatible path normalization
// and collision validation against existing sync sessions.
package paths

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Normalize returns an absolute, cleaned path with a trailing separator,
// matching the convention used by Synology Drive's sys.sqlite (e.g. "C:\Users\demo\.ssh\").
// On Windows, separators are backslashes; on POSIX, forward slashes.
func Normalize(p string) (string, error) {
	if strings.TrimSpace(p) == "" {
		return "", errors.New("path is empty")
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", fmt.Errorf("resolve absolute path: %w", err)
	}
	cleaned := filepath.Clean(abs)
	sep := string(filepath.Separator)
	if !strings.HasSuffix(cleaned, sep) {
		cleaned += sep
	}
	return cleaned, nil
}

// Validate ensures the path exists, is a directory, and is writable.
func Validate(p string) error {
	info, err := os.Stat(p)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("path does not exist: %s", p)
		}
		return fmt.Errorf("stat path: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("path is not a directory: %s", p)
	}
	probe := filepath.Join(p, ".drive-unlocker-write-probe")
	f, err := os.Create(probe)
	if err != nil {
		return fmt.Errorf("path is not writable: %w", err)
	}
	_ = f.Close()
	_ = os.Remove(probe)
	return nil
}

// CheckCollision returns an error if target equals, contains, or is contained by
// any of the other paths. All paths must already be normalized.
func CheckCollision(target string, others []string) error {
	t := caseFold(target)
	for _, o := range others {
		if o == "" {
			continue
		}
		other := caseFold(o)
		switch {
		case t == other:
			return fmt.Errorf("path is already used by another sync session: %s", o)
		case strings.HasPrefix(t, other):
			return fmt.Errorf("path is inside another sync session's folder: %s", o)
		case strings.HasPrefix(other, t):
			return fmt.Errorf("path contains another sync session's folder: %s", o)
		}
	}
	return nil
}

// caseFold lowercases the path on Windows where the filesystem is case-insensitive.
// On POSIX it returns the path unchanged.
func caseFold(p string) string {
	if filepath.Separator == '\\' {
		return strings.ToLower(p)
	}
	return p
}
