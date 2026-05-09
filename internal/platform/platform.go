// Package platform abstracts OS-specific paths and process names so
// the rest of the code stays portable. Only Windows is fully implemented;
// Darwin and Linux return ErrUnsupported until we add their layouts.
package platform

import "errors"

// ErrUnsupported is returned by platform implementations on operating systems
// we have not validated yet (macOS, Linux).
var ErrUnsupported = errors.New("this OS is not supported yet")

// Info bundles the runtime data we need from the host.
type Info struct {
	OS            string // "windows", "darwin", "linux"
	Username      string
	HomeDir       string
	DriveDataDir  string // e.g. %LOCALAPPDATA%\SynologyDrive\data
	DBDir         string // <DriveDataDir>/db
	ClientProcess string // e.g. cloud-drive-ui.exe
}

// Detect returns Info for the current host or ErrUnsupported.
func Detect() (Info, error) { return detect() }

// HasWALArtifacts returns true if any -wal or -shm file exists in the db dir.
// Their presence means the previous Synology Drive run did not exit cleanly.
func HasWALArtifacts(dbDir string) (bool, []string, error) {
	return hasWALArtifacts(dbDir)
}
