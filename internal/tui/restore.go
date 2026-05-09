package tui

import (
	"fmt"
	"strings"

	"github.com/FireHawken/synology-drive-unlocker/internal/backup"
	"github.com/FireHawken/synology-drive-unlocker/internal/platform"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
)

// restoreListModel shows every backup found in the db dir, newest first.
type restoreListModel struct {
	info    platform.Info
	entries []backup.Entry
	cursor  int
	width   int
	height  int
}

func newRestoreList(info platform.Info) (restoreListModel, error) {
	entries, err := backup.List(info.DBDir)
	if err != nil {
		return restoreListModel{}, err
	}
	return restoreListModel{info: info, entries: entries}, nil
}

func (m restoreListModel) Init() tea.Cmd { return nil }

func (m restoreListModel) Update(msg tea.Msg) (restoreListModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, func() tea.Msg { return quitAppMsg{} }
		case "esc":
			return m, func() tea.Msg { return backToMenuMsg{} }
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.entries)-1 {
				m.cursor++
			}
		case "enter":
			if len(m.entries) == 0 {
				return m, nil
			}
			picked := m.entries[m.cursor]
			return m, func() tea.Msg { return restoreEntryPickedMsg{entry: picked} }
		}
	}
	return m, nil
}

func (m restoreListModel) View() string {
	var b strings.Builder
	b.WriteString(styleTitle.Render("Restore from backup"))
	b.WriteByte('\n')

	if len(m.entries) == 0 {
		b.WriteString(styleMuted.Render("No backups found in " + m.info.DBDir + `\.unlocker-backups\`))
		b.WriteString(styleHelp.Render("\nesc back   q quit"))
		return b.String()
	}

	for i, e := range m.entries {
		marker := "  "
		ts := e.Meta.CreatedAt.Local().Format("2006-01-02 15:04:05")
		var line string
		if e.MetaError != nil {
			line = fmt.Sprintf("%s   (corrupt meta: %v)   %s", ts, e.MetaError, e.Dir)
		} else {
			line = fmt.Sprintf("%s   session id=%d   %s  →  %s",
				ts, e.Meta.SessionID, e.Meta.OldPath, e.Meta.NewPath)
		}
		if i == m.cursor {
			marker = styleAccent.Render("▸ ")
			line = styleSelectedItem.Render(line)
		} else {
			line = styleItem.Render(line)
		}
		b.WriteString(marker + line + "\n")
	}
	b.WriteString(styleHelp.Render("\n↑/↓ move   enter pick   esc back"))
	return b.String()
}

// restoreConfirmModel asks for confirmation before overwriting the live db files.
type restoreConfirmModel struct {
	info  platform.Info
	entry backup.Entry

	cursor    int // 0 = Restore, 1 = Cancel
	restoring bool
	spin      spinner.Model
}

func newRestoreConfirm(info platform.Info, entry backup.Entry) restoreConfirmModel {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = styleAccent
	return restoreConfirmModel{info: info, entry: entry, cursor: 1, spin: sp}
}

func (m restoreConfirmModel) Init() tea.Cmd { return nil }

func (m restoreConfirmModel) Update(msg tea.Msg) (restoreConfirmModel, tea.Cmd) {
	if m.restoring {
		if _, ok := msg.(spinner.TickMsg); ok {
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
			m.restoring = true
			return m, tea.Batch(m.spin.Tick, restoreCmd(m.info, m.entry))
		}
	}
	return m, nil
}

func (m restoreConfirmModel) View() string {
	if m.restoring {
		var b strings.Builder
		b.WriteString(styleTitle.Render("Restoring…"))
		b.WriteByte('\n')
		b.WriteString(m.spin.View())
		b.WriteString(" working")
		return b.String()
	}
	var b strings.Builder
	b.WriteString(styleTitle.Render("Confirm restore"))
	b.WriteByte('\n')

	body := strings.Builder{}
	fmt.Fprintf(&body, "Backup:        %s\n", m.entry.Dir)
	fmt.Fprintf(&body, "Created at:    %s\n", m.entry.Meta.CreatedAt.Local().Format("2006-01-02 15:04:05"))
	if m.entry.Meta.SessionID != 0 {
		fmt.Fprintf(&body, "Captured for:  session id=%d (%s → %s)\n",
			m.entry.Meta.SessionID, m.entry.Meta.OldPath, m.entry.Meta.NewPath)
	}
	body.WriteString("\nFiles to overwrite in " + m.info.DBDir + ":\n")
	for _, name := range m.entry.Meta.FilesCopied {
		body.WriteString("  • " + name + "\n")
	}
	body.WriteString("\n" + styleWarn.Render("Synology Drive Client must be stopped before restoring."))

	b.WriteString(stylePanel.Render(body.String()))
	b.WriteString("\n\n")

	restore := styleItem.Render("[ Restore ]")
	cancel := styleItem.Render("[ Cancel ]")
	if m.cursor == 0 {
		restore = styleSelectedItem.Render("[ Restore ]")
	} else {
		cancel = styleSelectedItem.Render("[ Cancel ]")
	}
	b.WriteString(restore + "    " + cancel)
	b.WriteString(styleHelp.Render("\n←/→ choose   enter confirm   esc back"))
	return b.String()
}
