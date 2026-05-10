// Package platform abstracts OS-specific paths and process names so the rest
// of the code stays portable.
package platform

import "errors"

// ErrUnsupported is returned by platform implementations on operating systems
// we have not validated yet.
var ErrUnsupported = errors.New("this OS is not supported yet")

// Info bundles the runtime data we need from the host.
type Info struct {
	OS            string // "windows", "darwin", "linux"
	Username      string
	HomeDir       string
	DriveDataDir  string // e.g. %LOCALAPPDATA%\SynologyDrive\data
	DBDir         string // <DriveDataDir>/db
	ClientProcess string // e.g. cloud-drive-ui.exe
	// ClientProcesses contains executable names that mean Synology Drive is
	// still running. It may contain multiple entries on platforms where the UI
	// and daemon use different process names.
	ClientProcesses []string
}

// Detect returns Info for the current host or ErrUnsupported.
func Detect() (Info, error) { return detect() }

// Processes returns ClientProcesses when set, otherwise the legacy single
// ClientProcess value. This keeps callers simple while preserving a compact UI
// label.
func (i Info) Processes() []string {
	if len(i.ClientProcesses) > 0 {
		return i.ClientProcesses
	}
	if i.ClientProcess == "" {
		return nil
	}
	return []string{i.ClientProcess}
}

// HasWALArtifacts returns true if any -wal or -shm file exists in the db dir.
// Their presence means the previous Synology Drive run did not exit cleanly.
func HasWALArtifacts(dbDir string) (bool, []string, error) {
	return hasWALArtifacts(dbDir)
}
