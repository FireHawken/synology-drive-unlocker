// Package tui implements the Bubble Tea-based interactive UI for the
// drive-unlocker tool. The root model App is a simple state machine that
// delegates rendering and key handling to one sub-model at a time.
package tui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/FireHawken/synology-drive-unlocker/internal/backup"
	"github.com/FireHawken/synology-drive-unlocker/internal/db"
	"github.com/FireHawken/synology-drive-unlocker/internal/platform"
	"github.com/FireHawken/synology-drive-unlocker/internal/process"
	tea "github.com/charmbracelet/bubbletea"
)

// PreflightResult bundles the host checks run on app start. Errors that
// occur during the checks are folded into the boolean state (e.g. a stat
// failure leaves dbDirFound=false) since the menu only shows pass/fail icons.
type PreflightResult struct {
	dbDirFound    bool
	clientRunning bool
	hasWAL        bool
	walFiles      []string
	hasBackups    bool
}

// CanProceed reports whether the host is in a safe state for editing the DB:
// the db directory exists, the Synology Drive Client is not running, and no
// WAL/SHM artifacts indicate an unclean shutdown.
func (p PreflightResult) CanProceed() bool {
	return p.dbDirFound && !p.clientRunning && !p.hasWAL
}

// blocker returns a short, human-readable reason why preflight failed.
// Empty string means CanProceed() is true.
func (p PreflightResult) blocker() string {
	switch {
	case !p.dbDirFound:
		return "database directory missing"
	case p.clientRunning:
		return "Synology Drive Client is running"
	case p.hasWAL:
		return "WAL/SHM files present — close the client cleanly first"
	}
	return ""
}

// RunPreflight performs the host checks. Called once on startup and again
// whenever we return to the main menu, since process state may have shifted.
func RunPreflight(info platform.Info) PreflightResult {
	var p PreflightResult

	if info.DBDir != "" {
		if exists, _ := dirExists(info.DBDir); exists {
			p.dbDirFound = true
		}
	}

	if running, err := process.IsAnyRunning(info.Processes()); err == nil {
		p.clientRunning = running
	}

	if p.dbDirFound {
		if hasWAL, files, err := platform.HasWALArtifacts(info.DBDir); err == nil {
			p.hasWAL = hasWAL
			p.walFiles = files
		}
		if entries, err := backup.List(info.DBDir); err == nil && len(entries) > 0 {
			p.hasBackups = true
		}
	}
	return p
}

func dirExists(p string) (bool, error) {
	info, err := os.Stat(p)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	return info.IsDir(), nil
}

// state enumerates the screens the App can be in.
type state int

const (
	stateMenu state = iota
	stateSessions
	statePicker
	stateConfirm
	stateApplying
	stateApplyResult
	stateRestoreList
	stateRestoreConfirm
	stateRestoring
	stateRestoreResult
	stateFatal
)

// App is the root tea.Model. It holds shared context (platform info, db handle)
// and the active sub-model for the current state.
type App struct {
	info      platform.Info
	preflight PreflightResult
	sysDB     *db.SysDB
	version   string

	state  state
	width  int
	height int

	// sub-models
	menu        menuModel
	sessions    sessionsModel
	picker      pickerModel
	confirm     confirmModel
	restoreList restoreListModel
	restoreConf restoreConfirmModel

	// pickedSession is kept across screens so confirm.go can show the diff
	// after the picker emits a path. Other transient selections live only
	// inside their corresponding messages.
	pickedSession db.Session

	// final results displayed on the *Result screens.
	applyResult applyDoneMsg
	restoreErr  error

	fatalErr error
}

// New constructs the App. Caller must already have detected platform info,
// run preflight, and (if preflight passed) opened sys.sqlite.
func New(info platform.Info, pre PreflightResult, sysDB *db.SysDB, version string) App {
	return App{
		info:      info,
		preflight: pre,
		sysDB:     sysDB,
		version:   version,
		state:     stateMenu,
		menu:      newMenu(info, pre),
	}
}

func (a App) Init() tea.Cmd { return nil }

func (a App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m := msg.(type) {
	case tea.WindowSizeMsg:
		a.width = m.Width
		a.height = m.Height
		// Forward to active sub-model when it cares (filepicker, lists).
		return a.forwardWindowSize(m)

	case tea.KeyMsg:
		// Global shortcuts that work in any state.
		if a.state != stateApplying && a.state != stateRestoring {
			switch m.String() {
			case "ctrl+c":
				return a, tea.Quit
			}
		}

	case quitAppMsg:
		return a, tea.Quit

	case backToMenuMsg:
		a.state = stateMenu
		a.menu = newMenu(a.info, RunPreflight(a.info))
		return a, nil

	case menuChoiceMsg:
		return a.handleMenuChoice(m.choice)

	case sessionPickedMsg:
		a.pickedSession = m.session
		a.picker = newPicker(a.info, a.pickedSession.SyncFolder)
		a.state = statePicker
		a.applySize()
		return a, a.picker.Init()

	case pathPickedMsg:
		conf, err := newConfirm(a.info, a.sysDB, a.pickedSession, m.path, a.version)
		if err != nil {
			a.fatalErr = err
			a.state = stateFatal
			return a, nil
		}
		a.confirm = conf
		a.state = stateConfirm
		return a, nil

	case applyDoneMsg:
		a.applyResult = m
		a.state = stateApplyResult
		return a, nil

	case restoreEntryPickedMsg:
		a.restoreConf = newRestoreConfirm(a.info, m.entry)
		a.state = stateRestoreConfirm
		return a, nil

	case restoreDoneMsg:
		a.restoreErr = m.err
		a.state = stateRestoreResult
		return a, nil
	}

	return a.routeToActive(msg)
}

func (a App) handleMenuChoice(c menuChoice) (tea.Model, tea.Cmd) {
	switch c {
	case menuChange:
		s, err := newSessions(a.sysDB)
		if err != nil {
			a.fatalErr = err
			a.state = stateFatal
			return a, nil
		}
		a.sessions = s
		a.state = stateSessions
		a.applySize()
		return a, nil
	case menuRestore:
		rl, err := newRestoreList(a.info)
		if err != nil {
			a.fatalErr = err
			a.state = stateFatal
			return a, nil
		}
		a.restoreList = rl
		a.state = stateRestoreList
		a.applySize()
		return a, nil
	case menuQuit:
		return a, tea.Quit
	}
	return a, nil
}

// routeToActive forwards a message to whichever sub-model owns the current state.
func (a App) routeToActive(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch a.state {
	case stateMenu:
		a.menu, cmd = a.menu.Update(msg)
	case stateSessions:
		a.sessions, cmd = a.sessions.Update(msg)
	case statePicker:
		a.picker, cmd = a.picker.Update(msg)
	case stateConfirm:
		a.confirm, cmd = a.confirm.Update(msg)
		if a.confirm.applying {
			a.state = stateApplying
		}
	case stateApplying:
		a.confirm, cmd = a.confirm.Update(msg)
	case stateApplyResult:
		// any key returns to menu
		if k, ok := msg.(tea.KeyMsg); ok {
			switch k.String() {
			case "enter", "esc", "q", " ":
				return a, func() tea.Msg { return backToMenuMsg{} }
			}
		}
	case stateRestoreList:
		a.restoreList, cmd = a.restoreList.Update(msg)
	case stateRestoreConfirm:
		a.restoreConf, cmd = a.restoreConf.Update(msg)
		if a.restoreConf.restoring {
			a.state = stateRestoring
		}
	case stateRestoring:
		a.restoreConf, cmd = a.restoreConf.Update(msg)
	case stateRestoreResult:
		if k, ok := msg.(tea.KeyMsg); ok {
			switch k.String() {
			case "enter", "esc", "q", " ":
				return a, func() tea.Msg { return backToMenuMsg{} }
			}
		}
	case stateFatal:
		if _, ok := msg.(tea.KeyMsg); ok {
			return a, tea.Quit
		}
	}
	return a, cmd
}

func (a App) forwardWindowSize(msg tea.WindowSizeMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch a.state {
	case statePicker:
		a.picker, cmd = a.picker.Update(msg)
	case stateSessions:
		a.sessions, cmd = a.sessions.Update(msg)
	case stateRestoreList:
		a.restoreList, cmd = a.restoreList.Update(msg)
	}
	return a, cmd
}

// applySize re-injects the cached terminal size into the currently active
// sub-model. Bubble Tea only emits WindowSizeMsg when the terminal actually
// resizes, so a screen entered after the initial event would otherwise
// render with width=0/height=0 — visible as an empty list bug. The mutation
// happens on the App receiver pointer fields by reassigning them.
func (a *App) applySize() {
	if a.width <= 0 || a.height <= 0 {
		return
	}
	msg := tea.WindowSizeMsg{Width: a.width, Height: a.height}
	switch a.state {
	case stateSessions:
		a.sessions, _ = a.sessions.Update(msg)
	case statePicker:
		a.picker, _ = a.picker.Update(msg)
	case stateRestoreList:
		a.restoreList, _ = a.restoreList.Update(msg)
	}
}

func (a App) View() string {
	switch a.state {
	case stateMenu:
		return a.menu.View()
	case stateSessions:
		return a.sessions.View()
	case statePicker:
		return a.picker.View()
	case stateConfirm, stateApplying:
		return a.confirm.View()
	case stateApplyResult:
		return a.renderApplyResult()
	case stateRestoreList:
		return a.restoreList.View()
	case stateRestoreConfirm, stateRestoring:
		return a.restoreConf.View()
	case stateRestoreResult:
		return a.renderRestoreResult()
	case stateFatal:
		return styleErr.Render("Fatal: ") + fmt.Sprintf("%v", a.fatalErr) +
			styleHelp.Render("\nPress any key to exit.")
	}
	return ""
}

func (a App) renderApplyResult() string {
	var b strings.Builder
	r := a.applyResult
	if r.err != nil {
		b.WriteString(styleErr.Render("Update failed."))
		b.WriteString("\n\n")
		b.WriteString(fmt.Sprintf("%v", r.err))
		if r.backupDir != "" {
			b.WriteString("\n\nBackup is intact at: ")
			b.WriteString(r.backupDir)
			b.WriteString("\nUse \"Restore from backup\" if anything looks wrong.")
		}
	} else {
		b.WriteString(styleOK.Render("Done."))
		b.WriteString("\n\n")
		fmt.Fprintf(&b, "session_table updated:    %d row(s)\n", r.sysResult.SessionRowsAffected)
		if r.sysResult.OpenFolderUpdated {
			fmt.Fprintf(&b, "system_table.open_folder: rewritten\n")
		}
		if r.statResult.TableExisted && r.statResult.HasPathColumn {
			fmt.Fprintf(&b, "file-status.statinfo:     %d row(s)\n", r.statResult.RowsAffected)
		} else if r.statResult.Skipped != "" {
			fmt.Fprintf(&b, "file-status.statinfo:     skipped (%s)\n", r.statResult.Skipped)
		}
		fmt.Fprintf(&b, "\nBackup saved to:\n  %s\n", r.backupDir)
		b.WriteString("\nLaunch Synology Drive Client to resume syncing.")
	}
	b.WriteString(styleHelp.Render("\nPress enter to return to menu."))
	return b.String()
}

func (a App) renderRestoreResult() string {
	var b strings.Builder
	if a.restoreErr != nil {
		b.WriteString(styleErr.Render("Restore failed."))
		b.WriteString("\n\n")
		b.WriteString(fmt.Sprintf("%v", a.restoreErr))
	} else {
		b.WriteString(styleOK.Render("Restore complete."))
		b.WriteString("\nDatabase files have been replaced with the backup contents.")
	}
	b.WriteString(styleHelp.Render("\nPress enter to return to menu."))
	return b.String()
}

// applyChangesCmd performs the backup + db updates and emits applyDoneMsg.
// Returned as a tea.Cmd so the UI can show a "working..." view while it runs.
func applyChangesCmd(ctx context.Context, info platform.Info, sysDB *db.SysDB,
	session db.Session, newPath, version string) tea.Cmd {
	return func() tea.Msg {
		bDir, err := backup.Create(info.DBDir, backup.Meta{
			ToolVersion: version,
			SessionID:   session.ID,
			OldPath:     session.SyncFolder,
			NewPath:     newPath,
		})
		if err != nil {
			return applyDoneMsg{err: fmt.Errorf("backup: %w", err)}
		}

		sysRes, err := sysDB.UpdateSessionFolder(ctx, session.ID, session.SyncFolder, newPath)
		if err != nil {
			return applyDoneMsg{backupDir: bDir, err: fmt.Errorf("update sys.sqlite: %w", err)}
		}

		statPath := filepath.Join(info.DBDir, "file-status.sqlite")
		var statRes db.StatusUpdateResult
		if exists, _ := dirExists(info.DBDir); exists {
			fs, err := db.OpenFileStatus(statPath)
			if err == nil {
				statRes, err = fs.UpdatePaths(ctx, session.SyncFolder, newPath)
				_ = fs.Close()
				if err != nil {
					return applyDoneMsg{
						backupDir:  bDir,
						sysResult:  sysRes,
						statResult: statRes,
						err:        fmt.Errorf("update file-status.sqlite: %w", err),
					}
				}
			}
		}
		return applyDoneMsg{
			backupDir:  bDir,
			sysResult:  sysRes,
			statResult: statRes,
		}
	}
}

// restoreCmd performs the restore and emits restoreDoneMsg.
func restoreCmd(info platform.Info, entry backup.Entry) tea.Cmd {
	return func() tea.Msg {
		err := backup.Restore(entry.Dir, info.DBDir)
		return restoreDoneMsg{err: err}
	}
}
