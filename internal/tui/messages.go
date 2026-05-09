package tui

import (
	"github.com/FireHawken/synology-drive-unlocker/internal/backup"
	"github.com/FireHawken/synology-drive-unlocker/internal/db"
)

// Custom tea.Msg types used to transition between screens. These are emitted
// by sub-models and handled in the root App.Update.

type (
	// menuChoice is sent from the menu when the user picks an action.
	menuChoiceMsg struct{ choice menuChoice }

	// sessionPickedMsg is sent from the sessions list when the user confirms a session.
	sessionPickedMsg struct{ session db.Session }

	// pathPickedMsg is sent from the filepicker when the user picks a folder.
	pathPickedMsg struct{ path string }

	// applyDoneMsg is sent after the apply step finishes (success or failure).
	applyDoneMsg struct {
		backupDir  string
		sysResult  db.UpdateResult
		statResult db.StatusUpdateResult
		err        error
	}

	// restoreEntryPickedMsg is sent when the user picks a backup to restore.
	restoreEntryPickedMsg struct{ entry backup.Entry }

	// restoreDoneMsg is sent after a restore finishes.
	restoreDoneMsg struct{ err error }

	// backToMenuMsg returns to the main menu from any subscreen.
	backToMenuMsg struct{}

	// quitAppMsg cleanly shuts the program down.
	quitAppMsg struct{}
)

type menuChoice int

const (
	menuChange menuChoice = iota
	menuRestore
	menuQuit
)
