// Package process detects whether a named process is running on the host.
package process

import "errors"

// ErrUnsupported is returned on platforms where IsRunning is not implemented.
var ErrUnsupported = errors.New("this OS is not supported yet")

// IsRunning reports whether at least one process with the given executable
// name (e.g. "cloud-drive-ui.exe") is currently running.
func IsRunning(name string) (bool, error) { return isRunning(name) }
