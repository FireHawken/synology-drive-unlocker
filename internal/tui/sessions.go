package tui

import (
	"context"
	"fmt"
	"strings"

	"github.com/FireHawken/synology-drive-unlocker/internal/db"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
)

// sessionsModel renders the list of sync-type sessions read from sys.sqlite.
// Selecting one emits sessionPickedMsg.
type sessionsModel struct {
	list     list.Model
	sessions []db.Session
}

// sessionItem adapts db.Session to bubbles/list's item interface.
type sessionItem struct{ s db.Session }

func (i sessionItem) FilterValue() string { return i.s.SyncFolder + " " + i.s.RemotePath }
func (i sessionItem) Title() string {
	return fmt.Sprintf("[id=%d]  %s", i.s.ID, i.s.SyncFolder)
}
func (i sessionItem) Description() string {
	return fmt.Sprintf("    ↔  %s   (share=%s)", i.s.RemotePath, i.s.ShareName)
}

func newSessions(sysDB *db.SysDB) (sessionsModel, error) {
	ctx := context.Background()
	rows, err := sysDB.SyncSessions(ctx)
	if err != nil {
		return sessionsModel{}, err
	}
	items := make([]list.Item, 0, len(rows))
	for _, s := range rows {
		items = append(items, sessionItem{s: s})
	}

	d := list.NewDefaultDelegate()
	d.SetSpacing(0)
	d.Styles.SelectedTitle = d.Styles.SelectedTitle.Foreground(colorAccent).Bold(true)
	d.Styles.SelectedDesc = d.Styles.SelectedDesc.Foreground(colorAccent)
	d.Styles.NormalTitle = d.Styles.NormalTitle.Foreground(colorTitle)
	d.Styles.NormalDesc = d.Styles.NormalDesc.Foreground(colorMuted)

	l := list.New(items, d, 0, 0)
	l.Title = "Pick the sync session to redirect"
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(true)
	l.SetShowHelp(true)
	l.Styles.Title = styleTitle
	return sessionsModel{list: l, sessions: rows}, nil
}

func (m sessionsModel) Init() tea.Cmd { return nil }

func (m sessionsModel) Update(msg tea.Msg) (sessionsModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		// Reserve a few rows below for help.
		m.list.SetSize(msg.Width-2, msg.Height-4)
		return m, nil
	case tea.KeyMsg:
		// Don't intercept keys while the user is typing in the filter.
		if m.list.FilterState() == list.Filtering {
			break
		}
		switch msg.String() {
		case "esc":
			return m, func() tea.Msg { return backToMenuMsg{} }
		case "q", "ctrl+c":
			return m, func() tea.Msg { return quitAppMsg{} }
		case "enter":
			if it, ok := m.list.SelectedItem().(sessionItem); ok {
				picked := it.s
				return m, func() tea.Msg { return sessionPickedMsg{session: picked} }
			}
		}
	}
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m sessionsModel) View() string {
	if len(m.sessions) == 0 {
		var b strings.Builder
		b.WriteString(styleTitle.Render("No sync sessions found"))
		b.WriteString("\n")
		b.WriteString(styleMuted.Render(
			"Create a sync task in Synology Drive Client first (any empty placeholder folder will do),\n" +
				"then re-run this tool to redirect it."))
		b.WriteString(styleHelp.Render("\nesc back   q quit"))
		return b.String()
	}
	return m.list.View() + styleHelp.Render("\nenter pick   /  filter   esc back")
}
