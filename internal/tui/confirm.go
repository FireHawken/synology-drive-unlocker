package tui

import (
	"context"
	"fmt"
	"strings"

	"github.com/FireHawken/synology-drive-unlocker/internal/db"
	"github.com/FireHawken/synology-drive-unlocker/internal/paths"
	"github.com/FireHawken/synology-drive-unlocker/internal/platform"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
)

// confirmModel renders the diff of pending changes and runs the apply step
// when the user confirms. While applying, the spinner indicates progress and
// keys are ignored.
type confirmModel struct {
	info    platform.Info
	sysDB   *db.SysDB
	session db.Session
	newPath string
	version string

	openFolderWillUpdate bool
	collisionWarn        string

	cursor   int // 0 = Apply, 1 = Cancel
	applying bool
	spin     spinner.Model
}

func newConfirm(info platform.Info, sysDB *db.SysDB, session db.Session, newPath, version string) (confirmModel, error) {
	open, err := sysDB.OpenFolder(context.Background())
	if err != nil {
		return confirmModel{}, fmt.Errorf("read open_folder: %w", err)
	}
	willTouchOpen := open != "" && trimTrailingSep(open) == trimTrailingSep(session.SyncFolder)

	all, err := sysDB.AllSyncFolders(context.Background())
	if err != nil {
		return confirmModel{}, fmt.Errorf("read sync_folders: %w", err)
	}
	others := make([]string, 0, len(all))
	for _, f := range all {
		if f == session.SyncFolder {
			continue
		}
		others = append(others, f)
	}
	collisionWarn := ""
	if err := paths.CheckCollision(newPath, others); err != nil {
		collisionWarn = err.Error()
	}

	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = styleAccent

	return confirmModel{
		info:                 info,
		sysDB:                sysDB,
		session:              session,
		newPath:              newPath,
		version:              version,
		openFolderWillUpdate: willTouchOpen,
		collisionWarn:        collisionWarn,
		spin:                 sp,
	}, nil
}

func trimTrailingSep(p string) string {
	for len(p) > 0 {
		c := p[len(p)-1]
		if c != '\\' && c != '/' {
			break
		}
		p = p[:len(p)-1]
	}
	return p
}

func (m confirmModel) Init() tea.Cmd { return nil }

func (m confirmModel) Update(msg tea.Msg) (confirmModel, tea.Cmd) {
	if m.applying {
		switch msg.(type) {
		case spinner.TickMsg:
			var c tea.Cmd
			m.spin, c = m.spin.Update(msg)
			return m, c
		}
		return m, nil
	}

	if k, ok := msg.(tea.KeyMsg); ok {
		switch k.String() {
		case "ctrl+c":
			return m, func() tea.Msg { return quitAppMsg{} }
		case "esc", "q":
			return m, func() tea.Msg { return backToMenuMsg{} }
		case "left", "h":
			m.cursor = 0
		case "right", "l":
			m.cursor = 1
		case "tab":
			m.cursor = (m.cursor + 1) % 2
		case "enter", " ":
			if m.cursor == 1 {
				return m, func() tea.Msg { return backToMenuMsg{} }
			}
			if m.collisionWarn != "" {
				return m, nil
			}
			m.applying = true
			return m, tea.Batch(
				m.spin.Tick,
				applyChangesCmd(context.Background(), m.info, m.sysDB,
					m.session, m.newPath, m.version),
			)
		}
	}
	return m, nil
}

func (m confirmModel) View() string {
	if m.applying {
		var b strings.Builder
		b.WriteString(styleTitle.Render("Applying changes…"))
		b.WriteByte('\n')
		b.WriteString(m.spin.View())
		b.WriteString(" working")
		return b.String()
	}

	var b strings.Builder
	b.WriteString(styleTitle.Render("Review changes"))
	b.WriteByte('\n')

	body := strings.Builder{}
	fmt.Fprintf(&body, "Session id=%d, share=%s, remote=%s\n\n",
		m.session.ID, m.session.ShareName, m.session.RemotePath)
	body.WriteString("sys.sqlite → session_table.sync_folder:\n")
	body.WriteString("  " + styleDiffOld.Render("- "+m.session.SyncFolder) + "\n")
	body.WriteString("  " + styleDiffNew.Render("+ "+m.newPath) + "\n\n")

	body.WriteString("sys.sqlite → system_table.open_folder:\n")
	if m.openFolderWillUpdate {
		body.WriteString("  " + styleDiffOld.Render("- "+trimTrailingSep(m.session.SyncFolder)) + "\n")
		body.WriteString("  " + styleDiffNew.Render("+ "+trimTrailingSep(m.newPath)) + "\n\n")
	} else {
		body.WriteString("  " + styleMuted.Render("not affected (points elsewhere)") + "\n\n")
	}

	body.WriteString("file-status.sqlite → statinfo.path:\n")
	body.WriteString("  " + styleMuted.Render("rewritten if the table exists; skipped otherwise") + "\n\n")

	body.WriteString("Snapshots taken before write:\n")
	for _, name := range []string{"sys.sqlite", "file-status.sqlite", "filter.sqlite", "history.sqlite"} {
		body.WriteString("  • " + name + "\n")
	}
	body.WriteString("  → " + m.info.DBDir + `\.unlocker-backups\backup-<timestamp>\` + "\n")

	if m.collisionWarn != "" {
		body.WriteString("\n" + styleErr.Render("Cannot apply: ") + m.collisionWarn)
	}

	b.WriteString(stylePanel.Render(body.String()))
	b.WriteString("\n\n")

	b.WriteString(m.renderButtons())
	b.WriteString(styleHelp.Render("\n←/→ choose   enter confirm   esc back"))
	return b.String()
}

func (m confirmModel) renderButtons() string {
	disabled := m.collisionWarn != ""
	var apply string
	switch {
	case disabled:
		apply = styleMuted.Render("[ Apply ]")
	case m.cursor == 0:
		apply = styleSelectedItem.Render("[ Apply ]")
	default:
		apply = styleItem.Render("[ Apply ]")
	}
	cancel := styleItem.Render("[ Cancel ]")
	if m.cursor == 1 {
		cancel = styleSelectedItem.Render("[ Cancel ]")
	}
	return apply + "    " + cancel
}
