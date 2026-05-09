//go:build windows

package process

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
	"syscall"
)

// isRunning shells out to tasklist with a filter on IMAGENAME and parses the
// CSV output. Using PowerShell's Get-Process would be heavier and slower.
func isRunning(name string) (bool, error) {
	cmd := exec.Command("tasklist", "/FI", "IMAGENAME eq "+name, "/FO", "CSV", "/NH")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return false, fmt.Errorf("run tasklist: %w", err)
	}
	out := stdout.String()
	// When no process matches, tasklist prints either an info line containing
	// "No tasks are running" (in current locale) or nothing at all. The reliable
	// signal is whether the executable name appears in the CSV output.
	lower := strings.ToLower(out)
	return strings.Contains(lower, strings.ToLower(name)), nil
}
