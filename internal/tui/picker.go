package tui

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/FireHawken/synology-drive-unlocker/internal/paths"
	"github.com/FireHawken/synology-drive-unlocker/internal/platform"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// pickerModel is a bespoke folder picker. We didn't reuse bubbles/filepicker
// because we need first-class manual-input support (for drive switching and
// pasted paths) and unambiguous "select this folder" semantics.
type pickerModel struct {
	info platform.Info

	cwd     string
	entries []dirEntry
	cursor  int
	width   int
	height  int

	// errMsg shown above the listing — typically "permission denied" from the last cd attempt.
	errMsg string

	// manual mode lets the user type/paste any absolute path.
	manual bool
	input  textinput.Model
}

// dirEntry is a row in the picker. The first row is always synthetic ("." == select cwd).
type dirEntry struct {
	name      string
	selectCwd bool
	parent    bool
}

func newPicker(info platform.Info, startHint string) pickerModel {
	start := startHint
	if start == "" {
		start = info.HomeDir
	}
	if _, err := os.Stat(start); err != nil {
		start = info.HomeDir
	}

	ti := textinput.New()
	ti.Placeholder = `e.g. C:\Users\you\.config`
	ti.Prompt = "> "
	ti.CharLimit = 500
	ti.Width = 60

	m := pickerModel{
		info:  info,
		cwd:   start,
		input: ti,
	}
	m.refresh()
	return m
}

func (m pickerModel) Init() tea.Cmd { return nil }

// refresh re-reads the cwd and rebuilds the entries list.
func (m *pickerModel) refresh() {
	m.entries = nil
	m.entries = append(m.entries, dirEntry{name: ".  (select this folder)", selectCwd: true})
	if filepath.Dir(m.cwd) != m.cwd {
		m.entries = append(m.entries, dirEntry{name: "..", parent: true})
	}
	if des, err := os.ReadDir(m.cwd); err == nil {
		var dirs []string
		for _, e := range des {
			if e.IsDir() {
				dirs = append(dirs, e.Name())
			}
		}
		sort.SliceStable(dirs, func(i, j int) bool {
			return strings.ToLower(dirs[i]) < strings.ToLower(dirs[j])
		})
		for _, d := range dirs {
			m.entries = append(m.entries, dirEntry{name: d})
		}
	}
	if m.cursor >= len(m.entries) {
		m.cursor = 0
	}
}

func (m pickerModel) Update(msg tea.Msg) (pickerModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		w := msg.Width - 10
		if w < 20 {
			w = 20
		}
		m.input.Width = w
		return m, nil
	case tea.KeyMsg:
		if m.manual {
			return m.handleManualKey(msg)
		}
		return m.handleListKey(msg)
	}
	return m, nil
}

func (m pickerModel) handleListKey(msg tea.KeyMsg) (pickerModel, tea.Cmd) {
	m.errMsg = ""
	switch msg.String() {
	case "ctrl+c", "q":
		return m, func() tea.Msg { return quitAppMsg{} }
	case "esc":
		return m, func() tea.Msg { return backToMenuMsg{} }
	case "i":
		m.manual = true
		m.input.SetValue(m.cwd)
		m.input.CursorEnd()
		return m, m.input.Focus()
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < len(m.entries)-1 {
			m.cursor++
		}
	case "home", "g":
		m.cursor = 0
	case "end", "G":
		m.cursor = len(m.entries) - 1
	case "left", "backspace", "h":
		return m.goUp(), nil
	case "right", "l":
		return m.openSelected(), nil
	case "enter":
		return m.openSelected(), m.maybeEmitSelected()
	}
	return m, nil
}

func (m pickerModel) handleManualKey(msg tea.KeyMsg) (pickerModel, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, func() tea.Msg { return quitAppMsg{} }
	case "esc":
		m.manual = false
		m.input.Blur()
		m.errMsg = ""
		return m, nil
	case "enter":
		raw := strings.TrimSpace(m.input.Value())
		if raw == "" {
			m.errMsg = "path is empty"
			return m, nil
		}
		norm, err := paths.Normalize(raw)
		if err != nil {
			m.errMsg = err.Error()
			return m, nil
		}
		if err := paths.Validate(norm); err != nil {
			m.errMsg = err.Error()
			return m, nil
		}
		picked := norm
		return m, func() tea.Msg { return pathPickedMsg{path: picked} }
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

// goUp navigates one directory level toward the filesystem root.
func (m pickerModel) goUp() pickerModel {
	parent := filepath.Dir(m.cwd)
	if parent == m.cwd {
		return m
	}
	m.cwd = parent
	m.cursor = 0
	m.refresh()
	return m
}

// openSelected handles cursor activation: parent → up; subdir → cd into it.
// The synthetic "selectCwd" row is a no-op here — Enter on it is handled by
// the caller via maybeEmitSelected, which emits pathPickedMsg.
func (m pickerModel) openSelected() pickerModel {
	if m.cursor < 0 || m.cursor >= len(m.entries) {
		return m
	}
	e := m.entries[m.cursor]
	switch {
	case e.parent:
		return m.goUp()
	case e.selectCwd:
		return m
	default:
		next := filepath.Join(m.cwd, e.name)
		if _, err := os.ReadDir(next); err != nil {
			m.errMsg = humanizeError(err)
			return m
		}
		m.cwd = next
		m.cursor = 0
		m.refresh()
		return m
	}
}

func (m pickerModel) maybeEmitSelected() tea.Cmd {
	if m.cursor < 0 || m.cursor >= len(m.entries) {
		return nil
	}
	if !m.entries[m.cursor].selectCwd {
		return nil
	}
	picked, err := paths.Normalize(m.cwd)
	if err != nil {
		return nil
	}
	if err := paths.Validate(picked); err != nil {
		return nil
	}
	return func() tea.Msg { return pathPickedMsg{path: picked} }
}

func (m pickerModel) View() string {
	var b strings.Builder
	b.WriteString(styleTitle.Render("Pick the new local folder"))
	b.WriteByte('\n')
	b.WriteString(styleSubtitle.Render(
		"This is the folder Synology Drive will sync to. System and dot-prefixed folders are allowed."))
	b.WriteString("\n\n")

	b.WriteString(styleAccent.Render("cwd: "))
	b.WriteString(m.cwd)
	b.WriteString("\n\n")

	if m.errMsg != "" {
		b.WriteString(styleErr.Render("error: ") + m.errMsg + "\n\n")
	}

	if m.manual {
		b.WriteString("Type or paste an absolute path:\n")
		b.WriteString(m.input.View())
		b.WriteString(styleHelp.Render("\nenter accept   esc cancel"))
		return b.String()
	}

	maxRows := m.height - 12
	if maxRows < 5 {
		maxRows = 5
	}
	start, end := windowAroundCursor(m.cursor, len(m.entries), maxRows)
	for i := start; i < end; i++ {
		e := m.entries[i]
		marker := "  "
		label := e.name
		if !e.selectCwd && !e.parent {
			label += string(filepath.Separator)
		}
		switch {
		case i == m.cursor:
			marker = styleAccent.Render("▸ ")
			label = styleSelectedItem.Render(label)
		case e.selectCwd:
			label = styleOK.Render(label)
		default:
			label = styleItem.Render(label)
		}
		b.WriteString(marker + label + "\n")
	}
	if end < len(m.entries) {
		b.WriteString(styleMuted.Render(fmt.Sprintf("  … %d more\n", len(m.entries)-end)))
	}
	b.WriteString(styleHelp.Render(
		"\n↑/↓ move    →/enter open folder (or select '.')    ←/backspace up    i type path    esc back"))
	return b.String()
}

// windowAroundCursor returns [start,end) bounds for a list slice that keeps
// the cursor visible inside a viewport of the requested size.
func windowAroundCursor(cursor, total, size int) (int, int) {
	if total <= size {
		return 0, total
	}
	half := size / 2
	start := cursor - half
	if start < 0 {
		start = 0
	}
	end := start + size
	if end > total {
		end = total
		start = end - size
	}
	return start, end
}

func humanizeError(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, os.ErrPermission) {
		return "permission denied"
	}
	return err.Error()
}
