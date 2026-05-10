//go:build darwin

package platform

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strings"
)

func detect() (Info, error) {
	u, err := user.Current()
	if err != nil {
		return Info{}, fmt.Errorf("get current user: %w", err)
	}
	home := u.HomeDir
	if home == "" {
		home = os.Getenv("HOME")
	}
	if home == "" {
		return Info{}, fmt.Errorf("home directory is not set")
	}

	dataDir := pickExistingDir([]string{
		filepath.Join(home, ".SynologyDrive", "data"),
		filepath.Join(home, "Library", "Application Support", "SynologyDrive", "data"),
	})
	dbDir := filepath.Join(dataDir, "db")

	return Info{
		OS:            "darwin",
		Username:      u.Username,
		HomeDir:       home,
		DriveDataDir:  dataDir,
		DBDir:         dbDir,
		ClientProcess: "SynologyDrive / cloud-drive-daemon",
		ClientProcesses: []string{
			"SynologyDrive",
			"Synology Drive Client",
			"cloud-drive-daemon",
			"cloud-drive-ui",
		},
	}, nil
}

func pickExistingDir(candidates []string) string {
	for _, candidate := range candidates {
		if info, err := os.Stat(filepath.Join(candidate, "db")); err == nil && info.IsDir() {
			return candidate
		}
	}
	return candidates[0]
}

func hasWALArtifacts(dbDir string) (bool, []string, error) {
	entries, err := os.ReadDir(dbDir)
	if err != nil {
		return false, nil, fmt.Errorf("read db dir: %w", err)
	}
	var found []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasSuffix(name, "-wal") || strings.HasSuffix(name, "-shm") {
			found = append(found, name)
		}
	}
	return len(found) > 0, found, nil
}
