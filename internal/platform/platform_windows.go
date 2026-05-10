//go:build windows

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
		home = os.Getenv("USERPROFILE")
	}

	localAppData := os.Getenv("LOCALAPPDATA")
	if localAppData == "" {
		localAppData = filepath.Join(home, "AppData", "Local")
	}
	dataDir := filepath.Join(localAppData, "SynologyDrive", "data")
	dbDir := filepath.Join(dataDir, "db")

	return Info{
		OS:            "windows",
		Username:      u.Username,
		HomeDir:       home,
		DriveDataDir:  dataDir,
		DBDir:         dbDir,
		ClientProcess: "cloud-drive-ui.exe",
		ClientProcesses: []string{
			"cloud-drive-ui.exe",
			"cloud-drive-daemon.exe",
		},
	}, nil
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
