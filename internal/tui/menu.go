package tui

import (
	"fmt"
	"strings"

	"github.com/FireHawken/synology-drive-unlocker/internal/platform"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// menuModel is the landing screen with a preflight status banner and three
// action choices. We render it manually rather than via bubbles/list because
// the layout is bespoke (banner + heading + items + help line).
type menuModel struct {
	info      platform.Info
	preflight PreflightResult
	cursor    int
	items     []menuItem
}

type menuItem struct {
	label    string
	choice   menuChoice
	disabled bool
	tooltip  string
}

func newMenu(info platform.Info, pre PreflightResult) menuModel {
	change := menuItem{label: "Change a sync session's local folder", choice: menuChange}
	restore := menuItem{label: "Restore from backup", choice: menuRestore}
	quit := menuItem{label: "Quit", choice: menuQuit}

	if !pre.CanProceed() {
		change.disabled = true
		change.tooltip = pre.blocker()
		restore.disabled = true
		restore.tooltip = pre.blocker()
	}
	if !pre.hasBackups {
		restore.disabled = true
		restore.tooltip = "no backups yet"
	}

	m := menuModel{
		info:      info,
		preflight: pre,
		items:     []menuItem{change, restore, quit},
	}
	// Land cursor on the first non-disabled item if any are disabled.
	for i, it := range m.items {
		if !it.disabled {
			m.cursor = i
			break
		}
	}
	return m
}

func (m menuModel) Init() tea.Cmd { return nil }

func (m menuModel) Update(msg tea.Msg) (menuModel, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch keyMsg.String() {
	case "ctrl+c", "q", "esc":
		return m, func() tea.Msg { return quitAppMsg{} }
	case "up", "k":
		m.cursor = m.prevEnabled(m.cursor)
	case "down", "j":
		m.cursor = m.nextEnabled(m.cursor)
	case "home", "g":
		m.cursor = m.firstEnabled()
	case "end", "G":
		m.cursor = m.lastEnabled()
	case "enter", " ":
		it := m.items[m.cursor]
		if it.disabled {
			return m, nil
		}
		choice := it.choice
		return m, func() tea.Msg { return menuChoiceMsg{choice: choice} }
	}
	return m, nil
}

func (m menuModel) prevEnabled(from int) int {
	for i := from - 1; i >= 0; i-- {
		if !m.items[i].disabled {
			return i
		}
	}
	return from
}

func (m menuModel) nextEnabled(from int) int {
	for i := from + 1; i < len(m.items); i++ {
		if !m.items[i].disabled {
			return i
		}
	}
	return from
}

func (m menuModel) firstEnabled() int {
	for i := range m.items {
		if !m.items[i].disabled {
			return i
		}
	}
	return 0
}

func (m menuModel) lastEnabled() int {
	for i := len(m.items) - 1; i >= 0; i-- {
		if !m.items[i].disabled {
			return i
		}
	}
	return len(m.items) - 1
}

func (m menuModel) View() string {
	var b strings.Builder
	b.WriteString(styleTitle.Render("Synology Drive Folder Unlocker"))
	b.WriteByte('\n')
	b.WriteString(styleSubtitle.Render(
		"Reroute existing sync sessions to system or dot-prefixed folders that the Synology Drive Client refuses to pick directly."))
	b.WriteString("\n\n")
	b.WriteString(m.renderPreflight())
	b.WriteString("\n\n")
	b.WriteString(m.renderItems())
	b.WriteString(styleHelp.Render("\n↑/↓ move    enter select    esc/q quit"))
	return b.String()
}

func (m menuModel) renderPreflight() string {
	check := func(ok bool, label, detail string) string {
		mark := styleErr.Render("✗")
		if ok {
			mark = styleOK.Render("✓")
		}
		line := fmt.Sprintf("%s  %s", mark, label)
		if detail != "" {
			line += "  " + styleMuted.Render(detail)
		}
		return line
	}
	rows := []string{
		check(m.preflight.dbDirFound, "Synology Drive database directory", m.info.DBDir),
		check(!m.preflight.clientRunning, "Synology Drive Client stopped", m.info.ClientProcess),
		check(!m.preflight.hasWAL, "no WAL/SHM files in db directory", strings.Join(m.preflight.walFiles, ", ")),
	}
	body := strings.Join(rows, "\n")
	return stylePanel.Render(body)
}

func (m menuModel) renderItems() string {
	var lines []string
	for i, it := range m.items {
		marker := "  "
		label := it.label
		switch {
		case it.disabled:
			label = lipgloss.NewStyle().Foreground(colorMuted).Strikethrough(true).Render(label)
			if it.tooltip != "" {
				label += "  " + styleMuted.Render("("+it.tooltip+")")
			}
		case i == m.cursor:
			marker = styleAccent.Render("▸ ")
			label = styleSelectedItem.Render(label)
		default:
			label = styleItem.Render(label)
		}
		lines = append(lines, marker+label)
	}
	return strings.Join(lines, "\n")
}
