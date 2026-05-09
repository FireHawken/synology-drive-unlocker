package tui

import "github.com/charmbracelet/lipgloss"

// Centralized lipgloss styles. Kept here so screens render consistently
// and so we have a single place to tweak the colour palette.

var (
	colorTitle   = lipgloss.AdaptiveColor{Light: "#5A56E0", Dark: "#7D7AFF"}
	colorOK      = lipgloss.AdaptiveColor{Light: "#02BA84", Dark: "#02BA84"}
	colorErr     = lipgloss.AdaptiveColor{Light: "#FF4F4F", Dark: "#FF6E6E"}
	colorWarn    = lipgloss.AdaptiveColor{Light: "#D08700", Dark: "#FFC857"}
	colorMuted   = lipgloss.AdaptiveColor{Light: "#5C5C5C", Dark: "#9A9A9A"}
	colorAccent  = lipgloss.AdaptiveColor{Light: "#FF6E73", Dark: "#FF7AC6"}
	colorBorder  = lipgloss.AdaptiveColor{Light: "#7D7AFF", Dark: "#7D7AFF"}
	colorBgPanel = lipgloss.AdaptiveColor{Light: "#FAFAFA", Dark: "#1B1B1F"}
)

var (
	styleTitle = lipgloss.NewStyle().
			Foreground(colorTitle).
			Bold(true).
			MarginBottom(1)

	styleSubtitle = lipgloss.NewStyle().
			Foreground(colorMuted).
			MarginBottom(1)

	styleOK   = lipgloss.NewStyle().Foreground(colorOK).Bold(true)
	styleErr  = lipgloss.NewStyle().Foreground(colorErr).Bold(true)
	styleWarn = lipgloss.NewStyle().Foreground(colorWarn).Bold(true)

	styleMuted  = lipgloss.NewStyle().Foreground(colorMuted)
	styleAccent = lipgloss.NewStyle().Foreground(colorAccent).Bold(true)

	stylePanel = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorBorder).
			Padding(0, 1)

	styleHelp = lipgloss.NewStyle().
			Foreground(colorMuted).
			MarginTop(1)

	styleDiffOld = lipgloss.NewStyle().Foreground(colorErr)
	styleDiffNew = lipgloss.NewStyle().Foreground(colorOK)

	styleSelectedItem = lipgloss.NewStyle().
				Foreground(colorAccent).
				Bold(true)

	styleItem = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#222", Dark: "#DDD"})
)
