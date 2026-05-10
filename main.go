// drive-unlocker is a console utility that reroutes existing Synology Drive
// sync sessions to system or dot-prefixed folders the official client refuses
// to pick directly. It does so by editing the client's local SQLite databases
// (with backups) while the client is stopped.
package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/FireHawken/synology-drive-unlocker/internal/db"
	"github.com/FireHawken/synology-drive-unlocker/internal/platform"
	"github.com/FireHawken/synology-drive-unlocker/internal/tui"
	tea "github.com/charmbracelet/bubbletea"
)

const version = "0.1.0"

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}

func run() error {
	info, err := platform.Detect()
	if err != nil {
		if errors.Is(err, platform.ErrUnsupported) {
			return fmt.Errorf("this OS is not supported yet — Windows and macOS only for now")
		}
		return fmt.Errorf("detect platform: %w", err)
	}

	pre := tui.RunPreflight(info)

	var sysDB *db.SysDB
	if pre.CanProceed() {
		sysPath := filepath.Join(info.DBDir, "sys.sqlite")
		s, err := db.OpenSys(sysPath)
		if err != nil {
			return fmt.Errorf("open sys.sqlite: %w", err)
		}
		sysDB = s
		defer sysDB.Close()
	}

	app := tui.New(info, pre, sysDB, version)
	prog := tea.NewProgram(app, tea.WithAltScreen())
	if _, err := prog.Run(); err != nil {
		return fmt.Errorf("tui: %w", err)
	}
	return nil
}
