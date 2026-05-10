//go:build !windows

package process

import (
	"bytes"
	"os/exec"
	"path/filepath"
	"strings"
)

func isRunning(name string) (bool, error) {
	out, err := exec.Command("ps", "-axo", "comm=").Output()
	if err != nil {
		return false, err
	}
	for _, line := range bytes.Split(out, []byte{'\n'}) {
		comm := strings.TrimSpace(string(line))
		if comm == "" {
			continue
		}
		base := filepath.Base(comm)
		if base == name || comm == name {
			return true, nil
		}
	}
	return false, nil
}
